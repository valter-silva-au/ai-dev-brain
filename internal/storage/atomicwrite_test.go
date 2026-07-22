package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// TestAtomicWriteFile_ConcurrentOpenReader exercises the Windows rename retry
// deterministically — no timing hold. It holds the target open for reading (which,
// without FILE_SHARE_DELETE, makes MoveFileEx fail with a sharing violation) so the
// first os.Rename is guaranteed to fail, then uses the testHookRenameAttempt seam to
// synchronize: the reader is not released until the test has OBSERVED that first
// failed attempt, and the failing attempt blocks in the hook until the reader is
// released, so the retry is guaranteed to run against a freed target. The write must
// take at least two attempts and ultimately succeed with the new content, never
// corrupting the target. Guarded to Windows because POSIX rename(2) has no such
// sharing semantics (and the POSIX renameWithRetry never calls the hook).
func TestAtomicWriteFile_ConcurrentOpenReader(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific rename sharing-violation behavior")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(target, []byte("old: true\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	// Open the target for reading; the handle stays open until the test has observed
	// the first failed rename (below), so that first failure is guaranteed rather
	// than timing-dependent.
	reader, err := os.Open(target)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}

	var attempts atomic.Int32
	firstFailureObserved := make(chan struct{})
	readerClosed := make(chan struct{})
	var once sync.Once

	testHookRenameAttempt = func(_ int, rerr error) {
		attempts.Add(1)
		if rerr != nil {
			// First failed attempt: signal the test to release the reader, then block
			// here until it has, so the NEXT attempt necessarily runs against a freed
			// target. This removes all timing dependence from the retry path.
			once.Do(func() {
				close(firstFailureObserved)
				<-readerClosed
			})
		}
	}
	defer func() { testHookRenameAttempt = nil }()

	writeErr := make(chan error, 1)
	go func() {
		writeErr <- atomicWriteFile(target, []byte("new: true\n"), 0o644)
	}()

	select {
	case <-firstFailureObserved:
		// Expected: the first rename hit a sharing violation and is parked in the hook.
		// Release the reader so the retry can land, then let the write complete.
		_ = reader.Close()
		close(readerClosed)
		if err := <-writeErr; err != nil {
			t.Fatalf("atomicWriteFile with a concurrent open reader: %v", err)
		}
	case err := <-writeErr:
		// The write finished without ever failing a rename — the open reader did not
		// block the first attempt, so the retry path was NOT exercised. Fail loudly
		// rather than passing vacuously.
		_ = reader.Close()
		close(readerClosed)
		if err != nil {
			t.Fatalf("atomicWriteFile failed before the retry path was reached: %v", err)
		}
		t.Fatalf("first rename did not hit a sharing violation; retry path not exercised (attempts=%d)", attempts.Load())
	}

	if got := attempts.Load(); got < 2 {
		t.Fatalf("expected at least 2 rename attempts (one failure + one success), got %d", got)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after write: %v", err)
	}
	if string(got) != "new: true\n" {
		t.Fatalf("target content = %q, want the new content (write lost or torn)", got)
	}
}

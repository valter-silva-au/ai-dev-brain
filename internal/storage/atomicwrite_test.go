package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestAtomicWriteFile_ConcurrentOpenReader exercises the Windows rename retry: it
// holds the target open for reading (which, without FILE_SHARE_DELETE, makes
// MoveFileEx transiently fail with a sharing violation) while atomicWriteFile
// replaces it, then releases the handle so a retry can land. The write must
// ultimately succeed with the new content and never corrupt the target. It is
// guarded to Windows because POSIX rename(2) has no such sharing semantics; on
// non-Windows the write simply succeeds immediately, which this test also
// tolerates if the guard is ever relaxed.
func TestAtomicWriteFile_ConcurrentOpenReader(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific rename sharing-violation behavior")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(target, []byte("old: true\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	// Hold the target open for reading long enough that even a slow temp-file
	// write can't reach the rename before the reader blocks it — this guarantees
	// the first os.Rename hits a sharing violation and the retry path is actually
	// exercised. 60ms sits comfortably inside renameWithRetry's ~225ms budget
	// (10 attempts, 5ms-stepped backoff), so a retry still lands well before the
	// budget is spent: fast and non-flaky.
	reader, err := os.Open(target)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	closed := make(chan struct{})
	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = reader.Close()
		close(closed)
	}()

	if err := atomicWriteFile(target, []byte("new: true\n"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile with a concurrent open reader: %v", err)
	}
	<-closed

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after write: %v", err)
	}
	if string(got) != "new: true\n" {
		t.Fatalf("target content = %q, want the new content (write lost or torn)", got)
	}
}

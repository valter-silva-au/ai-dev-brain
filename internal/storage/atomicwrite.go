package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// testHookAfterTempWrite, when non-nil, is invoked inside atomicWriteFile after
// the temp file has been fully written, synced, and closed but BEFORE the rename
// that publishes it over the target. It is a test-only seam (the stdlib testHook*
// convention): it is nil in every production build and is only ever assigned by
// the crash-safety test in a re-exec'd child process (registry_crash_test.go),
// where it simulates a writer dying mid-write to prove the target file is left as
// its previous, valid contents. Production code never sets it, so the branch below
// is inert.
var testHookAfterTempWrite func()

// testHookRenameAttempt, when non-nil, is invoked by the Windows renameWithRetry
// after each os.Rename attempt with the 0-based attempt index and that attempt's
// result. Like testHookAfterTempWrite it is a test-only seam — nil in every
// production build, assigned only by the Windows rename-retry test to observe and
// deterministically synchronize with the retry path. Only the Windows
// renameWithRetry calls it; the POSIX path does not retry, so it never fires there.
var testHookRenameAttempt func(attempt int, err error)

// atomicWriteFile writes data to path via a temp-file-plus-rename replace. It
// creates the parent directory if needed (0o755), writes the full contents to a
// temp file IN THE SAME directory, fsyncs and closes it, then renames it over
// path. Because the whole file is written before the swap, the commit is a single
// rename rather than an in-place truncate-then-write (unlike os.WriteFile).
//
// The replace is ATOMIC ON POSIX: rename(2) either fully replaces the target or
// does nothing, so a concurrent reader sees the whole old or the whole new file.
// On WINDOWS it is a BEST-EFFORT REPLACE: os.Rename maps to
// MoveFileEx(MOVEFILE_REPLACE_EXISTING), which is not a documented atomicity
// guarantee and can transiently fail with a sharing violation while another
// process holds the target open. renameWithRetry (per-platform in
// atomicwrite_windows.go / atomicwrite_other.go) absorbs those transient errors
// with a bounded retry on Windows; it does NOT promote the Windows replace to a
// POSIX-grade atomic operation.
//
// It is the shared save primitive for the mutable YAML registries; callers pair
// it with a cross-process lock (see acquireRegistryLock) held around the whole
// load-modify-save cycle so writers never interleave.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// A dotfile temp in the same directory keeps the rename on one filesystem and
	// keeps the partial file out of any registry glob.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// If anything below fails before the rename lands, remove the temp file so a
	// failed write never litters the directory.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if testHookAfterTempWrite != nil {
		testHookAfterTempWrite()
	}
	if err := renameWithRetry(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file into place: %w", err)
	}
	committed = true
	return nil
}

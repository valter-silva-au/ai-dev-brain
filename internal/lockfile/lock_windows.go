//go:build windows

package lockfile

import (
	"os"

	"golang.org/x/sys/windows"
)

// Lock acquires a mandatory exclusive lock on the given file using LockFileEx.
// Returns a release function that unlocks the file. Blocks until the lock is
// available. On Windows the lock is not automatically released on handle close
// in all cases, so callers must invoke the release function.
func Lock(f *os.File) (func(), error) {
	handle := windows.Handle(f.Fd())
	var ol windows.Overlapped
	// LOCKFILE_EXCLUSIVE_LOCK without LOCKFILE_FAIL_IMMEDIATELY => blocking exclusive lock.
	// Lock the entire file (max 64-bit range).
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol); err != nil {
		return func() {}, err
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol)
	}, nil
}

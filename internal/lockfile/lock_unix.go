//go:build !windows

package lockfile

import (
	"os"

	"golang.org/x/sys/unix"
)

// Lock acquires an advisory exclusive lock on the given file using flock(2).
// Returns a release function that unlocks the file. Blocks until the lock is
// available. On Unix this uses BSD-style flock, which is released automatically
// when the file descriptor is closed, so the release function is idempotent.
func Lock(f *os.File) (func(), error) {
	fd := int(f.Fd())
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		return func() {}, err
	}
	return func() {
		// Nothing is lost by not reporting this: LOCK_UN on a live descriptor
		// fails only with EBADF or EINVAL, and EBADF means the descriptor is
		// already closed — which, for BSD flock, is itself a release. The
		// release contract is func() with no channel to report an error, and
		// widening it would change every caller of Lock.
		//nolint:errcheck // unlock cannot fail while still holding the lock.
		_ = unix.Flock(fd, unix.LOCK_UN)
	}, nil
}

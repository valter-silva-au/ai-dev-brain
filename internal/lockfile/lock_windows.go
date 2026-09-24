//go:build windows

package lockfile

import (
	"os"

	"golang.org/x/sys/windows"
)

// Lock acquires a mandatory exclusive lock on the given file using LockFileEx.
// Returns a release function that unlocks the file. Blocks until the lock is
// available. On Windows the lock is not automatically released on handle close
// in all cases, so callers must invoke the release function. Release is safe to
// invoke more than once: a second unlock of an already-unlocked region reports
// ERROR_NOT_LOCKED, which is ignored (see the release closure below).
func Lock(f *os.File) (func(), error) {
	handle := windows.Handle(f.Fd())
	var ol windows.Overlapped
	// LOCKFILE_EXCLUSIVE_LOCK without LOCKFILE_FAIL_IMMEDIATELY => blocking exclusive lock.
	// Lock the entire file (max 64-bit range).
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol); err != nil {
		return func() {}, err
	}
	return func() {
		// Unchecked deliberately, and NOT for lock_unix.go's reason — the
		// argument has to be made again here, because a Windows byte-range lock
		// is MANDATORY (kernel-enforced), not advisory. A leaked one does not
		// merely fail to signal cooperating processes: it blocks their reads and
		// writes, and because Lock omits LOCKFILE_FAIL_IMMEDIATELY, the next adb
		// process does not get an error — it blocks in LockFileEx forever.
		//
		// So the question is whether any reachable failure leaves the region
		// still locked. Enumerating what UnlockFileEx can return for THIS call:
		//
		//   ERROR_NOT_LOCKED (158)     the region is not locked — so nothing is
		//                              held. This is the double-release case.
		//   ERROR_INVALID_HANDLE (6)   the handle is already closed, and Windows
		//                              drops a FILE_OBJECT's byte-range locks
		//                              with it. Same shape as flock's EBADF: the
		//                              close WAS the release.
		//   ERROR_INVALID_PARAMETER    the unlock region must match the locked
		//                              region exactly. Unreachable: both calls
		//                              use the same captured `ol` (offset 0) and
		//                              the same 0xFFFFFFFF/0xFFFFFFFF length, and
		//                              `ol` is a local captured by this one
		//                              closure, so nothing can mutate it.
		//   ERROR_IO_PENDING (997)     only on a handle opened for asynchronous
		//                              I/O, where completion arrives through the
		//                              OVERLAPPED. Unreachable: every caller
		//                              opens the lock file with os.OpenFile /
		//                              os.Root.OpenFile and no
		//                              windows.O_FILE_FLAG_OVERLAPPED, and Go
		//                              only puts a handle in overlapped mode when
		//                              that flag is set (os/file_windows.go,
		//                              openFileNolog), so this handle is
		//                              synchronous.
		//
		// Every reachable failure therefore means the lock is ALREADY not held.
		// There is no case in which reporting it would be acted on: the caller
		// must not retry the unlock (that is ERROR_NOT_LOCKED) and must not skip
		// its Close (that is the recovery). And the close is a real backstop
		// rather than a hope — all eight call sites (internal/core/taskid.go,
		// internal/storage/backlog.go, internal/journal/rooted.go,
		// internal/foundation, internal/organization, internal/repository, and
		// two in internal/ticket) close the handle after invoking release.
		// Seven do it as adjacent statements; internal/core/taskid.go does it
		// with `defer file.Close()` registered BEFORE `defer unlock()`, so LIFO
		// ordering runs the unlock first — stated because that is the one site
		// where a reader auditing this claim cannot see the ordering locally.
		// Microsoft documents closing a file with outstanding locks as undefined
		// and recommends unlocking explicitly, which is why release exists and
		// runs first; the close only has to cover the case where it did not.
		//
		// Unreportable, and this is where it differs from lock_unix.go's
		// constraint rather than sharing it: the release contract is func() with
		// no error channel, and this package is a leaf with no logger. Widening
		// the signature was measured rather than estimated, because "it reaches
		// a few packages" is the kind of premise that turns out to be wrong: it
		// is 25 call sites across 7 packages — the 8 above, the 6 wrapper
		// contracts that themselves return func() (acquireWorkspaceLock in
		// internal/foundation, internal/organization and internal/repository,
		// acquireRootedLock in internal/journal, FileBacklogManager.
		// acquireFileLock, and this Lock), and the 17 sites calling those. Each
		// of the 17 `defer unlock()`s would need a named return plus an
		// errors.Join, to surface a value that is benign in every reachable
		// case. That is the trade being declined, not merely a signature.
		//nolint:errcheck // every reachable failure means the lock is already released.
		_ = windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol)
	}, nil
}

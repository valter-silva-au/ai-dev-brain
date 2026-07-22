//go:build windows

package storage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// renameWithRetry renames oldpath over newpath, retrying briefly on the transient
// Windows errors MoveFileEx can return when another process momentarily holds the
// target open (a concurrent reader opened without FILE_SHARE_DELETE). Unlike POSIX
// rename(2), the Windows replace is best-effort and can fail with
// ERROR_SHARING_VIOLATION / ERROR_ACCESS_DENIED, so a short bounded retry makes
// the swap robust. It is NOT an unbounded wait: after the attempt budget is spent
// the last error is returned. (This mirrors how renameio-style helpers and Go's
// own os tests treat the Windows replace.)
func renameWithRetry(oldpath, newpath string) error {
	const attempts = 10
	var err error
	for i := 0; i < attempts; i++ {
		err = os.Rename(oldpath, newpath)
		if testHookRenameAttempt != nil {
			testHookRenameAttempt(i, err)
		}
		if err == nil || !isTransientReplaceErr(err) {
			return err
		}
		// A read-only TARGET is a permanent condition, not the momentary sharing
		// violation the retry exists to absorb — MoveFileEx keeps returning
		// ACCESS_DENIED, so retrying only burns the whole budget (~225ms) before
		// failing anyway. Fail fast instead. Genuine transient ACCESS_DENIED (an
		// AV scanner / indexer briefly holding the file) and SHARING_VIOLATION are
		// NOT read-only, so they still get their retries.
		if targetIsReadOnly(newpath) {
			return err
		}
		// Back off before the NEXT attempt only — no point sleeping after the last
		// one, whose transient error we are about to return.
		if i < attempts-1 {
			time.Sleep(time.Duration(i+1) * 5 * time.Millisecond)
		}
	}
	return err
}

// targetIsReadOnly reports whether newpath exists and carries the read-only
// attribute (no owner-write bit). os.Stat maps Windows FILE_ATTRIBUTE_READONLY to
// a cleared 0o200 mode bit. A stat error (e.g. the target does not exist) returns
// false, so a genuine transient failure still gets its retry.
func targetIsReadOnly(newpath string) bool {
	info, err := os.Stat(newpath)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o200 == 0
}

// isTransientReplaceErr reports whether err is a Windows sharing/access error that
// a brief retry can clear (another process still has the target open). os.Rename
// returns a *os.LinkError wrapping a syscall.Errno, which errors.Is unwraps to the
// windows.Errno constants below.
func isTransientReplaceErr(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

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
		if err = os.Rename(oldpath, newpath); err == nil || !isTransientReplaceErr(err) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 5 * time.Millisecond)
	}
	return err
}

// isTransientReplaceErr reports whether err is a Windows sharing/access error that
// a brief retry can clear (another process still has the target open). os.Rename
// returns a *os.LinkError wrapping a syscall.Errno, which errors.Is unwraps to the
// windows.Errno constants below.
func isTransientReplaceErr(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

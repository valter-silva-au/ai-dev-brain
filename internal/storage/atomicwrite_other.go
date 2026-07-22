//go:build !windows

package storage

import "os"

// renameWithRetry renames oldpath over newpath. On POSIX rename(2) is atomic and
// does not exhibit the transient sharing-violation failures Windows can produce,
// so no retry is needed — this is a straight os.Rename.
func renameWithRetry(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

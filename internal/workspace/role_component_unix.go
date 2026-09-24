//go:build !windows

package workspace

import (
	"errors"
	"io/fs"
	"os"
)

func inspectRoleComponent(
	info fs.FileInfo,
	final bool,
) (RoleViolationReason, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return RoleViolationSymlink, errors.New("component is a symbolic link")
	}
	if final {
		if info.IsDir() || info.Mode().IsRegular() {
			return "", nil
		}
		return RoleViolationUninspectable, errors.New(
			"final component is neither a regular file nor a directory",
		)
	}
	if !info.IsDir() {
		return RoleViolationUninspectable, errors.New(
			"intermediate component is not a directory",
		)
	}
	return "", nil
}

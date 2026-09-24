//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

func inspectRoleComponent(
	info fs.FileInfo,
	final bool,
) (RoleViolationReason, error) {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return RoleViolationUninspectable, fmt.Errorf(
			"inspect Windows file attributes: unexpected stat data %T",
			info.Sys(),
		)
	}
	if data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return RoleViolationSymlink, errors.New(
			"component is a Windows reparse point",
		)
	}
	if final {
		if info.IsDir() || info.Mode().IsRegular() {
			return "", nil
		}
		return RoleViolationUninspectable, errors.New(
			"final component is neither a regular file nor a directory",
		)
	}
	if data.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		!info.IsDir() {
		return RoleViolationUninspectable, errors.New(
			"intermediate component is not a directory",
		)
	}
	return "", nil
}

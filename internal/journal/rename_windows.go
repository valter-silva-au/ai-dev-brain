//go:build windows

package journal

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func renameRootedNoReplace(root *os.Root, oldName string, newName string) error {
	oldPath := filepath.Join(root.Name(), oldName)
	newPath := filepath.Join(root.Name(), newName)
	oldPointer, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPointer, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	if err := windows.MoveFile(oldPointer, newPointer); err != nil {
		return &os.LinkError{
			Op:  "rename",
			Old: oldPath,
			New: newPath,
			Err: err,
		}
	}
	return nil
}

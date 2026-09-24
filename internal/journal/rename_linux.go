//go:build linux

package journal

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameRootedNoReplace(root *os.Root, oldName string, newName string) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close()
	}()

	return unix.Renameat2(
		int(directory.Fd()),
		oldName,
		int(directory.Fd()),
		newName,
		unix.RENAME_NOREPLACE,
	)
}

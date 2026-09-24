//go:build !darwin && !linux && !windows

package journal

import (
	"fmt"
	"os"
)

func renameRootedNoReplace(root *os.Root, oldName string, newName string) error {
	if err := root.Link(oldName, newName); err != nil {
		return err
	}
	if err := root.Remove(oldName); err != nil {
		return fmt.Errorf(
			"remove source after exclusive journal publication link: %w",
			err,
		)
	}
	return nil
}

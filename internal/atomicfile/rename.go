package atomicfile

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Rename moves source to target and durably records the directory entry change.
func Rename(source string, target string) error {
	if source == "" {
		return errors.New("atomic rename source is required")
	}
	if target == "" {
		return errors.New("atomic rename target is required")
	}

	if err := renamePath(source, target); err != nil {
		return fmt.Errorf(
			"rename %q to %q: %w",
			source,
			target,
			err,
		)
	}
	if err := syncRenameParents(source, target, syncDirectory); err != nil {
		return err
	}
	return nil
}

func syncRenameParents(
	source string,
	target string,
	sync func(string) error,
) error {
	sourceParent := filepath.Clean(filepath.Dir(source))
	targetParent := filepath.Clean(filepath.Dir(target))
	parents := []string{sourceParent}
	if targetParent != sourceParent {
		parents = append(parents, targetParent)
	}

	for _, parent := range parents {
		if err := sync(parent); err != nil {
			return fmt.Errorf(
				"sync rename parent directory %q: %w",
				parent,
				err,
			)
		}
	}
	return nil
}

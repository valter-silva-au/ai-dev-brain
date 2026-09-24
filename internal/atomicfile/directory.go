package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// MkdirAllWithin creates path beneath an existing trusted root and durably
// records every directory entry from path back to root on every call.
func MkdirAllWithin(root string, path string, mode fs.FileMode) error {
	return mkdirAllWithin(root, path, mode, nil)
}

func mkdirAllWithin(
	root string,
	path string,
	mode fs.FileMode,
	sync func(string) error,
) error {
	if root == "" {
		return errors.New("directory durability root is required")
	}
	if path == "" {
		return errors.New("directory path is required")
	}
	if mode.Perm() == 0 {
		return errors.New("directory mode is required")
	}

	cleanRoot, relative, err := relativeWithinRoot(
		root,
		path,
		"directory path",
	)
	if err != nil {
		return err
	}
	openedRoot, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return fmt.Errorf("open directory durability root %q: %w", cleanRoot, err)
	}
	defer func() {
		_ = openedRoot.Close()
	}()

	if err := openedRoot.MkdirAll(relative, mode.Perm()); err != nil {
		return fmt.Errorf("create directory tree %q: %w", path, err)
	}

	for _, parentRelative := range directoryEntryParents(relative) {
		parent := cleanRoot
		if parentRelative != "." {
			parent = filepath.Join(cleanRoot, parentRelative)
		}
		var syncErr error
		if sync != nil {
			syncErr = sync(parent)
		} else {
			syncErr = syncRootDirectory(openedRoot, parentRelative)
		}
		if syncErr != nil {
			return fmt.Errorf(
				"sync created directory parent %q: %w",
				parent,
				syncErr,
			)
		}
	}
	return nil
}

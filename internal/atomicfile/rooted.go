package atomicfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const temporaryCreateAttempts = 100

// WriteWithin atomically writes target beneath an existing trusted root.
func WriteWithin(
	root string,
	target string,
	options Options,
	write func(io.Writer) error,
) (returnErr error) {
	if target == "" {
		return errors.New("atomic file path is required")
	}
	if options.Mode.Perm() == 0 {
		return errors.New("atomic file mode is required")
	}
	if write == nil {
		return errors.New("atomic file writer is required")
	}

	cleanRoot, relativeTarget, err := relativeWithinRoot(
		root,
		target,
		"atomic file target",
	)
	if err != nil {
		return err
	}
	openedRoot, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return fmt.Errorf("open atomic file root %q: %w", cleanRoot, err)
	}
	defer func() {
		if closeErr := openedRoot.Close(); closeErr != nil {
			returnErr = errors.Join(
				returnErr,
				fmt.Errorf("close atomic file root %q: %w", cleanRoot, closeErr),
			)
		}
	}()

	parentRelative := filepath.Dir(relativeTarget)
	if options.CreateParents && parentRelative != "." {
		if err := openedRoot.MkdirAll(parentRelative, 0o755); err != nil {
			return fmt.Errorf("create parent directory %q: %w", filepath.Dir(target), err)
		}
		if err := syncRootDirectoryParents(
			openedRoot,
			cleanRoot,
			parentRelative,
		); err != nil {
			return fmt.Errorf("sync created parent directories for %q: %w", target, err)
		}
	}

	temporary, temporaryRelative, err := createTemporaryWithin(
		openedRoot,
		parentRelative,
		filepath.Base(relativeTarget),
		options.Mode.Perm(),
	)
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", target, err)
	}

	closed := false
	published := false
	defer func() {
		var cleanupErr error
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil {
				cleanupErr = errors.Join(
					cleanupErr,
					fmt.Errorf(
						"close temporary file for %q during cleanup: %w",
						target,
						closeErr,
					),
				)
			}
		}
		if !published {
			if removeErr := openedRoot.Remove(temporaryRelative); removeErr != nil {
				cleanupErr = errors.Join(
					cleanupErr,
					fmt.Errorf(
						"remove temporary file for %q during cleanup: %w",
						target,
						removeErr,
					),
				)
			}
		}
		if cleanupErr != nil {
			returnErr = errors.Join(returnErr, cleanupErr)
		}
	}()

	if err := temporary.Chmod(options.Mode.Perm()); err != nil {
		return fmt.Errorf("set temporary file mode for %q: %w", target, err)
	}
	if err := write(temporary); err != nil {
		return fmt.Errorf("write temporary file for %q: %w", target, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %q: %w", target, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", target, err)
	}
	closed = true

	if err := openedRoot.Rename(temporaryRelative, relativeTarget); err != nil {
		return fmt.Errorf("publish atomic file %q: %w", target, err)
	}
	published = true

	if err := syncRootDirectory(openedRoot, parentRelative); err != nil {
		return fmt.Errorf("sync parent directory %q: %w", filepath.Dir(target), err)
	}
	return nil
}

// RenameWithin moves source to target beneath one existing trusted root.
func RenameWithin(root string, source string, target string) error {
	if source == "" {
		return errors.New("atomic rename source is required")
	}
	if target == "" {
		return errors.New("atomic rename target is required")
	}

	cleanRoot, relativeSource, err := relativeWithinRoot(
		root,
		source,
		"atomic rename source",
	)
	if err != nil {
		return err
	}
	_, relativeTarget, err := relativeWithinRoot(
		cleanRoot,
		target,
		"atomic rename target",
	)
	if err != nil {
		return err
	}
	openedRoot, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return fmt.Errorf("open atomic rename root %q: %w", cleanRoot, err)
	}
	defer func() {
		_ = openedRoot.Close()
	}()

	if err := openedRoot.Rename(relativeSource, relativeTarget); err != nil {
		return fmt.Errorf("rename %q to %q: %w", source, target, err)
	}

	sourceParent := filepath.Dir(relativeSource)
	targetParent := filepath.Dir(relativeTarget)
	if err := syncRootDirectory(openedRoot, sourceParent); err != nil {
		return fmt.Errorf(
			"sync rename parent directory %q: %w",
			filepath.Dir(source),
			err,
		)
	}
	if targetParent != sourceParent {
		if err := syncRootDirectory(openedRoot, targetParent); err != nil {
			return fmt.Errorf(
				"sync rename parent directory %q: %w",
				filepath.Dir(target),
				err,
			)
		}
	}
	return nil
}

func relativeWithinRoot(
	root string,
	target string,
	targetDescription string,
) (string, string, error) {
	if root == "" {
		return "", "", errors.New("trusted root is required")
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("trusted root must be absolute: %q", root)
	}
	if !filepath.IsAbs(target) {
		return "", "", fmt.Errorf(
			"%s must be absolute: %q",
			targetDescription,
			target,
		)
	}

	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return "", "", fmt.Errorf(
			"resolve path %q within trusted root %q: %w",
			cleanTarget,
			cleanRoot,
			err,
		)
	}
	if relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf(
			"path %q is outside trusted root %q",
			cleanTarget,
			cleanRoot,
		)
	}
	return cleanRoot, relative, nil
}

func directoryEntryParents(relative string) []string {
	if relative == "." {
		return nil
	}

	parents := make([]string, 0)
	for parent := filepath.Dir(relative); ; parent = filepath.Dir(parent) {
		parents = append(parents, parent)
		if parent == "." {
			return parents
		}
	}
}

func syncRootDirectoryParents(
	root *os.Root,
	rootPath string,
	relative string,
) error {
	for _, parentRelative := range directoryEntryParents(relative) {
		if err := syncRootDirectory(root, parentRelative); err != nil {
			parent := rootPath
			if parentRelative != "." {
				parent = filepath.Join(rootPath, parentRelative)
			}
			return fmt.Errorf("sync directory parent %q: %w", parent, err)
		}
	}
	return nil
}

func createTemporaryWithin(
	root *os.Root,
	parent string,
	base string,
	mode fs.FileMode,
) (*os.File, string, error) {
	for attempt := 0; attempt < temporaryCreateAttempts; attempt++ {
		suffix, err := temporarySuffix()
		if err != nil {
			return nil, "", err
		}
		name := "." + base + ".tmp-" + suffix
		relative := filepath.Join(parent, name)
		file, err := root.OpenFile(
			relative,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			mode,
		)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return file, relative, nil
	}
	return nil, "", errors.New("exhausted temporary file name attempts")
}

func temporarySuffix() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate temporary file suffix: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

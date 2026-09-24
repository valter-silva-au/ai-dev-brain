package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Options struct {
	Mode          fs.FileMode
	CreateParents bool
}

func SyncDirectory(path string) error {
	return syncDirectory(path)
}

func Write(
	path string,
	options Options,
	write func(io.Writer) error,
) error {
	if path == "" {
		return errors.New("atomic file path is required")
	}
	if options.Mode.Perm() == 0 {
		return errors.New("atomic file mode is required")
	}
	if write == nil {
		return errors.New("atomic file writer is required")
	}

	parent := filepath.Dir(path)
	if options.CreateParents {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create parent directory %q: %w", parent, err)
		}
	}

	temporary, err := os.CreateTemp(parent, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}

	temporaryPath := temporary.Name()
	closed := false
	published := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(options.Mode.Perm()); err != nil {
		return fmt.Errorf("set temporary file mode for %q: %w", path, err)
	}
	if err := write(temporary); err != nil {
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	closed = true

	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("publish atomic file %q: %w", path, err)
	}
	published = true

	if err := syncDirectory(parent); err != nil {
		return fmt.Errorf("sync parent directory %q: %w", parent, err)
	}

	return nil
}

//go:build !windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
)

func syncRootDirectory(root *os.Root, relative string) error {
	directory, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close()
	}()

	if err := directory.Sync(); err != nil &&
		!errors.Is(err, syscall.EINVAL) &&
		!errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}

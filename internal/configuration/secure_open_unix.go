//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package configuration

import (
	"os"

	"golang.org/x/sys/unix"
)

func openConfigurationFile(path string) (*os.File, error) {
	descriptor, err := unix.Open(
		path,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), path), nil
}

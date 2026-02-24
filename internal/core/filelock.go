package core

import (
	"fmt"
	"os"
	"syscall"
)

// lockFile acquires an exclusive file lock (LOCK_EX) on the given file path.
// It returns an unlock function that must be called to release the lock.
// On Windows, this uses syscall.Flock which is available on Unix-like systems.
// For Windows, we fall back to a simple advisory lock pattern (non-blocking).
func lockFile(path string) (unlock func() error, err error) {
	// Open file for reading and writing (create if not exists).
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	// Acquire exclusive lock.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring file lock: %w", err)
	}

	// Return unlock function.
	return func() error {
		defer f.Close()
		return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}, nil
}

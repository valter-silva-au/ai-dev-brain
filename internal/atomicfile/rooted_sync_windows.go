//go:build windows

package atomicfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var reOpenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

func syncRootDirectory(root *os.Root, relative string) (returnErr error) {
	directory, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, closeErr)
		}
	}()

	reopened, err := reOpenDirectoryHandle(windows.Handle(directory.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := windows.CloseHandle(reopened); closeErr != nil {
			returnErr = errors.Join(returnErr, closeErr)
		}
	}()
	return windows.FlushFileBuffers(reopened)
}

func reOpenDirectoryHandle(handle windows.Handle) (windows.Handle, error) {
	reopened, _, callErr := reOpenFile.Call(
		uintptr(handle),
		uintptr(windows.GENERIC_READ|windows.GENERIC_WRITE),
		uintptr(
			windows.FILE_SHARE_READ|
				windows.FILE_SHARE_WRITE|
				windows.FILE_SHARE_DELETE,
		),
		uintptr(windows.FILE_FLAG_BACKUP_SEMANTICS),
	)
	reopenedHandle := windows.Handle(reopened)
	if reopenedHandle == windows.InvalidHandle {
		if callErr == windows.ERROR_SUCCESS {
			callErr = errors.New("ReOpenFile returned an invalid handle")
		}
		return windows.InvalidHandle, fmt.Errorf(
			"reopen rooted directory handle: %w",
			callErr,
		)
	}
	return reopenedHandle, nil
}

//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

func renamePath(source string, target string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPath, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePath,
		targetPath,
		windows.MOVEFILE_WRITE_THROUGH,
	)
}

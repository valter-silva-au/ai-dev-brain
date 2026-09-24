//go:build !windows

package atomicfile

import "os"

func renamePath(source string, target string) error {
	return os.Rename(source, target)
}

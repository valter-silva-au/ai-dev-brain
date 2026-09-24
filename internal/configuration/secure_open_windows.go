//go:build windows

package configuration

import "os"

func openConfigurationFile(path string) (*os.File, error) {
	return os.Open(path)
}

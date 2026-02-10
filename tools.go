//go:build tools

// Package tools tracks tool and library dependencies that are required by the project
// but not yet directly imported in source code. This ensures go mod tidy retains them.
package tools

import (
	_ "github.com/spf13/viper"
	_ "gopkg.in/yaml.v3"
	_ "pgregory.net/rapid"
)

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
)

// Build-time variables set via ldflags
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	// Set version info in CLI package
	cli.Version = Version
	cli.Commit = Commit
	cli.Date = Date

	// Resolve base path
	basePath, err := resolveBasePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve base path: %v\n", err)
		os.Exit(1)
	}

	// Create App with resolved base path
	app, err := internal.NewApp(basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize app: %v\n", err)
		os.Exit(1)
	}
	defer app.Cleanup()

	// Inject App into CLI package
	cli.App = app

	// Execute root command
	rootCmd := cli.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// resolveBasePath resolves the base path for the workspace
// Priority:
// 1. ADB_HOME environment variable
// 2. Walk up from current directory looking for .taskconfig
// 3. Current directory as fallback
func resolveBasePath() (string, error) {
	// Check ADB_HOME environment variable
	if adbHome := os.Getenv("ADB_HOME"); adbHome != "" {
		absPath, err := filepath.Abs(adbHome)
		if err != nil {
			return "", fmt.Errorf("invalid ADB_HOME path: %w", err)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return "", fmt.Errorf("ADB_HOME path does not exist: %s", absPath)
		}
		return absPath, nil
	}

	// Walk up from current directory looking for .taskconfig
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	dir := currentDir
	for {
		// Check if .taskconfig exists in this directory
		configPath := filepath.Join(dir, ".taskconfig")
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}

		// Check if .taskrc exists (repo-level config)
		configPath = filepath.Join(dir, ".taskrc")
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}

		// Move up one directory
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// Reached root, stop
			break
		}
		dir = parentDir
	}

	// Fallback to current directory
	return currentDir, nil
}

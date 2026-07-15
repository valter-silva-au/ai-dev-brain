package cli

import "github.com/valter-silva-au/ai-dev-brain/internal"

// Package-level variables set by main.go for dependency injection
// This approach avoids circular imports while allowing commands to access the App
var (
	// App is the application container with all dependencies wired
	App *internal.App

	// Version information (set via ldflags at build time)
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

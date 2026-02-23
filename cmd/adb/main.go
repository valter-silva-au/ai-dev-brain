package main

import (
	"fmt"
	"os"

	app "github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
)

// Set by goreleaser ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.SetVersionInfo(version, commit, date)
	basePath := app.ResolveBasePath()

	if _, err := app.NewApp(basePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing adb: %v\n", err)
		os.Exit(1)
	}

	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

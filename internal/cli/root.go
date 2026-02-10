package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	appVersion = "dev"
	appCommit  = "none"
	appDate    = "unknown"
)

// SetVersionInfo sets the version information injected via ldflags.
func SetVersionInfo(version, commit, date string) {
	appVersion = version
	appCommit = commit
	appDate = date
}

var rootCmd = &cobra.Command{
	Use:   "adb",
	Short: "AI Dev Brain - AI-powered developer productivity system",
	Long: `AI Dev Brain (adb) is a developer productivity system that wraps AI coding
assistants with persistent context management, task lifecycle automation,
and knowledge accumulation.

It provides CLI commands for managing tasks, bootstrapping worktrees,
tracking communications, and maintaining organizational knowledge.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("adb %s\ncommit: %s\nbuilt:  %s\n", appVersion, appCommit, appDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCmd creates the version command
func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		Long:  `Display version, commit, and build date information`,
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "AI Dev Brain (adb)\n")
			fmt.Fprintf(out, "Version: %s\n", Version)
			fmt.Fprintf(out, "Commit:  %s\n", Commit)
			fmt.Fprintf(out, "Built:   %s\n", Date)
		},
	}

	return cmd
}

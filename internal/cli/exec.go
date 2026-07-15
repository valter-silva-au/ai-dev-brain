package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// NewExecCmd creates the exec command
func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <cli> [args...]",
		Short: "Execute CLI command with env injection",
		Long:  `Execute a CLI command with ADB environment variables injected`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Build command string
			command := strings.Join(args, " ")

			// Create CLI executor with empty aliases and task env
			taskEnv := integration.TaskEnv{
				TaskID:       "", // Could be detected from current directory
				Branch:       "",
				WorktreePath: App.BasePath,
				TicketPath:   "",
			}

			executor := integration.NewCLIExecutor(
				make(map[string]string),
				taskEnv,
				"",
			)

			// Execute command
			fmt.Printf("Executing: %s\n", command)
			stdout, stderr, err := executor.Execute(command, App.BasePath)

			// Print output
			if stdout != "" {
				fmt.Print(stdout)
			}
			if stderr != "" {
				fmt.Fprint(cmd.ErrOrStderr(), stderr)
			}

			if err != nil {
				return fmt.Errorf("command failed: %w", err)
			}

			return nil
		},
	}

	return cmd
}

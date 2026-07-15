package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// NewRunCmd creates the run command
func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <task-name>",
		Short: "Run a task from Taskfile",
		Long:  `Execute a task defined in Taskfile.yml`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskName := args[0]

			// Create Taskfile runner
			runner := core.NewTaskfileRunner(App.BasePath)

			// Execute task
			fmt.Printf("Running task '%s' from Taskfile...\n", taskName)
			if err := runner.Run(taskName); err != nil {
				return fmt.Errorf("task execution failed: %w", err)
			}

			fmt.Printf("\n✓ Task '%s' completed\n", taskName)
			return nil
		},
	}

	return cmd
}

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// Runner is the TaskfileRunner used by the run command.
// Set during application wiring (Task #43).
var Runner integration.TaskfileRunner

// RunTaskCtx holds the active task context for run, if any.
// Populated during application wiring when a task is active.
var RunTaskCtx *integration.TaskEnvContext

var runCmd = &cobra.Command{
	Use:   "run [task] [args...]",
	Short: "Execute a task from Taskfile.yaml",
	Long: `Execute a named task from Taskfile.yaml in the current directory,
injecting task environment variables when a task is active.

Use --list to display all available Taskfile tasks.`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle --help / -h manually since DisableFlagParsing is true.
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}

		// Handle --simple flag: set CLAUDE_CODE_SIMPLE=1 for reduced output.
		if len(args) > 0 && args[0] == "--simple" {
			_ = os.Setenv("CLAUDE_CODE_SIMPLE", "1")
			args = args[1:]
		}

		if Runner == nil {
			return fmt.Errorf("taskfile runner not initialized")
		}

		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		// Check for --list flag manually since we use DisableFlagParsing.
		if len(args) > 0 && (args[0] == "--list" || args[0] == "-l") {
			tasks, err := Runner.ListTasks(dir)
			if err != nil {
				return fmt.Errorf("listing tasks: %w", err)
			}
			if len(tasks) == 0 {
				fmt.Println("No tasks found in Taskfile.yaml.")
				return nil
			}
			fmt.Println("Available Taskfile tasks:")
			for _, task := range tasks {
				if task.Description != "" {
					fmt.Printf("  %-20s %s\n", task.Name, task.Description)
				} else {
					fmt.Printf("  %s\n", task.Name)
				}
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("task name required; use --list to see available tasks")
		}

		taskName := args[0]
		taskArgs := args[1:]

		config := integration.TaskfileRunConfig{
			TaskName: taskName,
			Args:     taskArgs,
			TaskCtx:  RunTaskCtx,
			Dir:      dir,
			Stdout:   os.Stdout,
			Stderr:   os.Stderr,
		}

		result, err := Runner.Run(config)
		if err != nil {
			return fmt.Errorf("run %s: %w", taskName, err)
		}

		if result.ExitCode != 0 {
			osExit(result.ExitCode)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

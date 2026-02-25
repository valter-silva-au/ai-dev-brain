package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var priorityCmd = &cobra.Command{
	Use:   "priority <task-id> [task-id...]",
	Short: "Reorder task priorities",
	Long: `Reorder task priorities by specifying task IDs in priority order.

The first task gets P0 (highest priority), the second P1, the third P2,
and subsequent tasks get P3. This is a convenient way to reprioritize
multiple tasks at once.

Examples:
  adb priority TASK-00003 TASK-00001 TASK-00005
  # TASK-00003 -> P0, TASK-00001 -> P1, TASK-00005 -> P2`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		if err := TaskMgr.ReorderPriorities(args); err != nil {
			return fmt.Errorf("reordering priorities: %w", err)
		}

		fmt.Println("Priorities updated:")
		priorities := []string{"P0", "P1", "P2", "P3"}
		for i, taskID := range args {
			p := "P3"
			if i < len(priorities) {
				p = priorities[i]
			}
			fmt.Printf("  %s -> %s\n", taskID, p)
		}

		return nil
	},
}

func init() {
	priorityCmd.Deprecated = "use 'adb task priority'"
	priorityCmd.ValidArgsFunction = completeTaskIDs()
	rootCmd.AddCommand(priorityCmd)
}

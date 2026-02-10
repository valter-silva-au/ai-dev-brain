package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var unarchiveCmd = &cobra.Command{
	Use:   "unarchive <task-id>",
	Short: "Restore an archived task to a resumable state",
	Long: `Restore a previously archived task. The task is returned to its
pre-archive status, allowing work to continue where it left off.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		taskID := args[0]
		task, err := TaskMgr.UnarchiveTask(taskID)
		if err != nil {
			return fmt.Errorf("unarchiving task: %w", err)
		}

		fmt.Printf("Unarchived task %s\n", task.ID)
		fmt.Printf("  Type:     %s\n", task.Type)
		fmt.Printf("  Status:   %s\n", task.Status)
		fmt.Printf("  Branch:   %s\n", task.Branch)
		if task.WorktreePath != "" {
			fmt.Printf("  Worktree: %s\n", task.WorktreePath)
		}
		fmt.Printf("  Ticket:   %s\n", task.TicketPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(unarchiveCmd)
}

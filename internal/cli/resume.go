package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <task-id>",
	Short: "Resume working on an existing task",
	Long: `Resume working on a previously created task. This loads the task's context,
restores the working environment, and promotes the task to in_progress status
if it was in backlog.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		taskID := args[0]
		task, err := TaskMgr.ResumeTask(taskID)
		if err != nil {
			return fmt.Errorf("resuming task: %w", err)
		}

		fmt.Printf("Resumed task %s\n", task.ID)
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
	rootCmd.AddCommand(resumeCmd)
}

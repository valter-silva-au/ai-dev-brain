package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup <task-id>",
	Short: "Remove the git worktree for a task",
	Long: `Remove the git worktree associated with a task and clear the worktree
path from the task's status.yaml. The task itself is not archived or deleted.

Use this to reclaim disk space when you no longer need the worktree for a task.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		taskID := args[0]

		task, err := TaskMgr.GetTask(taskID)
		if err != nil {
			return fmt.Errorf("cleaning up worktree: %w", err)
		}

		if task.WorktreePath == "" {
			fmt.Printf("Task %s has no worktree to clean up.\n", taskID)
			return nil
		}

		fmt.Printf("Removing worktree: %s\n", task.WorktreePath)

		if err := TaskMgr.CleanupWorktree(taskID); err != nil {
			return fmt.Errorf("cleaning up worktree: %w", err)
		}

		fmt.Printf("Worktree removed for task %s\n", taskID)
		return nil
	},
}

func init() {
	cleanupCmd.Deprecated = "use 'adb task cleanup'"
	cleanupCmd.ValidArgsFunction = completeTaskIDs()
	rootCmd.AddCommand(cleanupCmd)
}

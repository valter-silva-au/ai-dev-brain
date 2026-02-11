package cli

import (
	"fmt"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

var archiveForce bool

var archiveCmd = &cobra.Command{
	Use:   "archive <task-id>",
	Short: "Archive a completed task",
	Long: `Archive a task, generating a handoff document that captures learnings,
decisions, and open items for future reference.

Use --force to archive a task that is still in active status (in_progress, blocked).
Without --force, archiving an active task will return an error.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		taskID := args[0]

		// Check task status if --force is not set.
		if !archiveForce {
			task, err := TaskMgr.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("archiving task: %w", err)
			}
			if task.Status == "in_progress" || task.Status == "blocked" {
				return fmt.Errorf("task %s is %s; use --force to archive an active task", taskID, task.Status)
			}
		}

		handoff, err := TaskMgr.ArchiveTask(taskID)
		if err != nil {
			return fmt.Errorf("archiving task: %w", err)
		}

		fmt.Printf("Archived task %s\n", handoff.TaskID)
		fmt.Printf("  Summary: %s\n", handoff.Summary)
		if len(handoff.CompletedWork) > 0 {
			fmt.Println("  Completed:")
			for _, item := range handoff.CompletedWork {
				fmt.Printf("    - %s\n", item)
			}
		}
		if len(handoff.OpenItems) > 0 {
			fmt.Println("  Open items:")
			for _, item := range handoff.OpenItems {
				fmt.Printf("    - %s\n", item)
			}
		}
		if len(handoff.Learnings) > 0 {
			fmt.Println("  Learnings:")
			for _, item := range handoff.Learnings {
				fmt.Printf("    - %s\n", item)
			}
		}

		return nil
	},
}

func init() {
	archiveCmd.Flags().BoolVar(&archiveForce, "force", false, "Force archive an active task")
	archiveCmd.ValidArgsFunction = completeTaskIDs(models.StatusArchived)
	rootCmd.AddCommand(archiveCmd)
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

var archiveForce bool
var archiveKeepWorktree bool

var archiveCmd = &cobra.Command{
	Use:   "archive <task-id>",
	Short: "Archive a completed task",
	Long: `Archive a task, generating a handoff document that captures learnings,
decisions, and open items for future reference.

By default, the task's git worktree is also removed. Use --keep-worktree to
preserve it.

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

		// Clean up worktree before archiving unless --keep-worktree is set.
		if !archiveKeepWorktree {
			task, err := TaskMgr.GetTask(taskID)
			if err == nil && task.WorktreePath != "" {
				fmt.Printf("Removing worktree: %s\n", task.WorktreePath)
				if cleanupErr := TaskMgr.CleanupWorktree(taskID); cleanupErr != nil {
					fmt.Printf("  Warning: worktree cleanup failed: %v\n", cleanupErr)
				}
			}
		}

		// Extract and ingest knowledge before archiving (non-fatal).
		if KnowledgeX != nil && KnowledgeMgr != nil {
			if extracted, extractErr := KnowledgeX.ExtractFromTask(taskID); extractErr == nil && extracted != nil {
				ids, ingestErr := KnowledgeMgr.IngestFromExtraction(extracted)
				if ingestErr != nil {
					fmt.Printf("  Warning: knowledge ingestion failed: %v\n", ingestErr)
				} else if len(ids) > 0 {
					fmt.Printf("Extracted %d knowledge entries from %s\n", len(ids), taskID)
				}
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
	archiveCmd.Deprecated = "use 'adb task archive'"
	archiveCmd.Flags().BoolVar(&archiveForce, "force", false, "Force archive an active task")
	archiveCmd.Flags().BoolVar(&archiveKeepWorktree, "keep-worktree", false, "Do not remove the git worktree when archiving")
	archiveCmd.ValidArgsFunction = completeTaskIDs(models.StatusArchived)
	rootCmd.AddCommand(archiveCmd)
}

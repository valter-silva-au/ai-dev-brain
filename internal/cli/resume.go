package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <task-id>",
	Short: "Resume working on an existing task",
	Long: `Resume working on a previously created task. This loads the task's context,
restores the working environment, promotes the task to in_progress status
if it was in backlog, and launches Claude Code in the worktree directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		var taskID string
		if len(args) > 0 {
			taskID = args[0]
		} else {
			// No task ID provided: show interactive picker.
			picked, err := pickResumableTask()
			if err != nil {
				return err
			}
			taskID = picked
		}
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

		// Refresh accumulated project knowledge, status, and inline content in task context.
		if task.WorktreePath != "" {
			appendKnowledgeToTaskContext(task.WorktreePath)
			refreshTaskContextMetadata(task.WorktreePath, string(task.Status))
			enrichTaskContext(task.WorktreePath, task.TicketPath)
		}

		// Post-resume workflow: rename terminal tab and launch Claude Code with --resume.
		if task.WorktreePath != "" {
			// Set ADB_TASK_TYPE and ADB_REPO_SHORT so the status line script
			// can display them without re-parsing status.yaml.
			if task.Type != "" {
				_ = os.Setenv("ADB_TASK_TYPE", string(task.Type))
			}
			if task.Repo != "" {
				_ = os.Setenv("ADB_REPO_SHORT", repoShortName(task.Repo))
			}
			launchWorkflow(task.ID, task.Branch, task.WorktreePath, task.TicketPath, true)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: task %s has no worktree. Use 'adb feat %s' to create a new task with a worktree.\n", task.ID, task.Branch)
		}

		return nil
	},
}

func init() {
	resumeCmd.Deprecated = "use 'adb task resume'"
	resumeCmd.ValidArgsFunction = completeTaskIDs(models.StatusArchived, models.StatusDone)
	rootCmd.AddCommand(resumeCmd)
}

package cli

import (
	"fmt"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// TaskMgr is the TaskManager used by task lifecycle commands.
// Set during application wiring (Task #43).
var TaskMgr core.TaskManager

// taskCreateFlags holds the optional flags shared by feat/bug/spike/refactor commands.
type taskCreateFlags struct {
	repo     string
	priority string
	owner    string
	tags     []string
}

// newTaskCommand creates a Cobra command for a given task type (feat, bug, spike, refactor).
// All four commands share the same logic, differing only in the TaskType passed to CreateTask.
func newTaskCommand(taskType models.TaskType) *cobra.Command {
	var flags taskCreateFlags

	cmd := &cobra.Command{
		Use:   string(taskType) + " <branch-name>",
		Short: fmt.Sprintf("Create a new %s task", taskType),
		Long: fmt.Sprintf(`Create a new %s task with the given branch name.

This bootstraps a ticket folder, creates a git worktree, initializes context
files, and registers the task in the backlog.`, taskType),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if TaskMgr == nil {
				return fmt.Errorf("task manager not initialized")
			}

			branchName := args[0]

			task, err := TaskMgr.CreateTask(taskType, branchName, flags.repo)
			if err != nil {
				return fmt.Errorf("creating %s task: %w", taskType, err)
			}

			fmt.Printf("Created task %s\n", task.ID)
			fmt.Printf("  Type:     %s\n", task.Type)
			fmt.Printf("  Branch:   %s\n", task.Branch)
			if task.Repo != "" {
				fmt.Printf("  Repo:     %s\n", task.Repo)
			}
			if task.WorktreePath != "" {
				fmt.Printf("  Worktree: %s\n", task.WorktreePath)
			}
			fmt.Printf("  Ticket:   %s\n", task.TicketPath)

			return nil
		},
	}

	cmd.Flags().StringVar(&flags.repo, "repo", "", "Repository path (e.g. github.com/org/repo)")
	cmd.Flags().StringVar(&flags.priority, "priority", "", "Task priority (P0, P1, P2, P3)")
	cmd.Flags().StringVar(&flags.owner, "owner", "", "Task owner (e.g. @username)")
	cmd.Flags().StringSliceVar(&flags.tags, "tags", nil, "Comma-separated tags for the task")

	return cmd
}

func init() {
	rootCmd.AddCommand(newTaskCommand(models.TaskTypeFeat))
	rootCmd.AddCommand(newTaskCommand(models.TaskTypeBug))
	rootCmd.AddCommand(newTaskCommand(models.TaskTypeSpike))
	rootCmd.AddCommand(newTaskCommand(models.TaskTypeRefactor))
}

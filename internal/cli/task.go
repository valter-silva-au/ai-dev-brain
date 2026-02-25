package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks (create, resume, archive, status, priority, update)",
	Long: `Unified task management commands.

Create new tasks, resume existing ones, archive completed work,
check status, reprioritize, and generate stakeholder updates.`,
}

// taskCreateTypeFlag holds the --type flag value for "task create".
var taskCreateTypeFlag string

var taskCreateCmd = &cobra.Command{
	Use:   "create <branch-name>",
	Short: "Create a new task",
	Long: `Create a new task with the given branch name.

This bootstraps a ticket folder, creates a git worktree, initializes context
files, and registers the task in the backlog. Use --type to specify the task
type (default: feat).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		taskType := models.TaskType(taskCreateTypeFlag)
		branchName := args[0]

		repoFlag, _ := cmd.Flags().GetString("repo")
		priorityFlag, _ := cmd.Flags().GetString("priority")
		ownerFlag, _ := cmd.Flags().GetString("owner")
		tagsFlag, _ := cmd.Flags().GetStringSlice("tags")
		prefixFlag, _ := cmd.Flags().GetString("prefix")

		repoPath := repoFlag
		if repoPath == "" {
			if detected := detectGitRoot(); detected != "" {
				repoPath = detected
			}
		}

		prefix := prefixFlag
		if prefix == "" && repoPath != "" {
			prefix = detectRepoPrefix(repoPath, BasePath)
		}

		task, err := TaskMgr.CreateTask(taskType, branchName, repoPath, core.CreateTaskOpts{
			Priority:      models.Priority(priorityFlag),
			Owner:         ownerFlag,
			Tags:          tagsFlag,
			BranchPattern: BranchPattern,
			Prefix:        prefix,
		})
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

		if task.WorktreePath != "" {
			appendKnowledgeToTaskContext(task.WorktreePath)
		}

		if task.WorktreePath != "" {
			if task.Type != "" {
				_ = os.Setenv("ADB_TASK_TYPE", string(task.Type))
			}
			if task.Repo != "" {
				_ = os.Setenv("ADB_REPO_SHORT", repoShortName(task.Repo))
			}
			launchWorkflow(task.ID, task.Branch, task.WorktreePath, task.TicketPath, false)
		}

		return nil
	},
}

var taskResumeCmd = &cobra.Command{
	Use:   "resume <task-id>",
	Short: "Resume working on an existing task",
	Long: `Resume working on a previously created task. This loads the task's context,
restores the working environment, promotes the task to in_progress status
if it was in backlog, and launches Claude Code in the worktree directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: resumeCmd.RunE,
}

var taskArchiveForce bool
var taskArchiveKeepWorktree bool

var taskArchiveCmd = &cobra.Command{
	Use:   "archive <task-id>",
	Short: "Archive a completed task",
	Long: `Archive a task, generating a handoff document that captures learnings,
decisions, and open items for future reference.

By default, the task's git worktree is also removed. Use --keep-worktree to
preserve it.

Use --force to archive a task that is still in active status (in_progress, blocked).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Temporarily set the package-level flags used by archiveCmd.RunE.
		origForce := archiveForce
		origKeep := archiveKeepWorktree
		defer func() {
			archiveForce = origForce
			archiveKeepWorktree = origKeep
		}()
		archiveForce = taskArchiveForce
		archiveKeepWorktree = taskArchiveKeepWorktree
		return archiveCmd.RunE(archiveCmd, args)
	},
}

var taskUnarchiveCmd = &cobra.Command{
	Use:   "unarchive <task-id>",
	Short: "Restore an archived task to a resumable state",
	Long: `Restore a previously archived task. The task is returned to its
pre-archive status, allowing work to continue where it left off.`,
	Args: cobra.ExactArgs(1),
	RunE: unarchiveCmd.RunE,
}

var taskCleanupCmd = &cobra.Command{
	Use:   "cleanup <task-id>",
	Short: "Remove the git worktree for a task",
	Long: `Remove the git worktree associated with a task and clear the worktree
path from the task's status.yaml. The task itself is not archived or deleted.`,
	Args: cobra.ExactArgs(1),
	RunE: cleanupCmd.RunE,
}

var taskStatusFilter string

var taskStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display tasks grouped by status",
	Long: `Display all tasks organized by their lifecycle status.

Optionally filter to a single status using --filter (e.g. --filter in_progress).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Temporarily set the package-level filter used by statusCmd.RunE.
		origFilter := statusFilter
		defer func() { statusFilter = origFilter }()
		statusFilter = taskStatusFilter
		return statusCmd.RunE(statusCmd, args)
	},
}

var taskPriorityCmd = &cobra.Command{
	Use:   "priority <task-id> [task-id...]",
	Short: "Reorder task priorities",
	Long: `Reorder task priorities by specifying task IDs in priority order.

The first task gets P0 (highest priority), the second P1, the third P2,
and subsequent tasks get P3.`,
	Args: cobra.MinimumNArgs(1),
	RunE: priorityCmd.RunE,
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <task-id>",
	Short: "Generate stakeholder communication updates for a task",
	Long: `Generate a plan of stakeholder communication updates based on the task's
current context, recent progress, blockers, and communication history.

This command DOES NOT send any messages. It only generates content for
your review.`,
	Args: cobra.ExactArgs(1),
	RunE: updateCmd.RunE,
}

// completeTaskTypes returns valid task type values for shell completion.
func completeTaskTypes(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"feat\tNew feature work",
		"bug\tBug fix",
		"spike\tResearch or investigation",
		"refactor\tCode restructuring or cleanup",
	}, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// task create flags
	taskCreateCmd.Flags().StringVar(&taskCreateTypeFlag, "type", "feat", "Task type: feat, bug, spike, or refactor")
	taskCreateCmd.Flags().String("repo", "", "Repository path (e.g. github.com/org/repo)")
	taskCreateCmd.Flags().String("priority", "", "Task priority (P0, P1, P2, P3)")
	taskCreateCmd.Flags().String("owner", "", "Task owner (e.g. @username)")
	taskCreateCmd.Flags().StringSlice("tags", nil, "Comma-separated tags for the task")
	taskCreateCmd.Flags().String("prefix", "", "Custom prefix for organizing task folders (e.g. 'finance')")
	_ = taskCreateCmd.RegisterFlagCompletionFunc("type", completeTaskTypes)
	_ = taskCreateCmd.RegisterFlagCompletionFunc("repo", completeRepoPaths)
	_ = taskCreateCmd.RegisterFlagCompletionFunc("priority", completePriorities)

	// task resume completions
	taskResumeCmd.ValidArgsFunction = completeTaskIDs(models.StatusArchived, models.StatusDone)

	// task archive flags and completions
	taskArchiveCmd.Flags().BoolVar(&taskArchiveForce, "force", false, "Force archive an active task")
	taskArchiveCmd.Flags().BoolVar(&taskArchiveKeepWorktree, "keep-worktree", false, "Do not remove the git worktree when archiving")
	taskArchiveCmd.ValidArgsFunction = completeTaskIDs(models.StatusArchived)

	// task unarchive completions
	taskUnarchiveCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return unarchiveCmd.ValidArgsFunction(cmd, args, toComplete)
	}

	// task cleanup completions
	taskCleanupCmd.ValidArgsFunction = completeTaskIDs()

	// task status flags
	taskStatusCmd.Flags().StringVar(&taskStatusFilter, "filter", "", "Filter by status (backlog, in_progress, blocked, review, done, archived)")
	_ = taskStatusCmd.RegisterFlagCompletionFunc("filter", completeStatuses)

	// task priority completions
	taskPriorityCmd.ValidArgsFunction = completeTaskIDs()

	// task update completions
	taskUpdateCmd.ValidArgsFunction = completeTaskIDs()

	// Register all subcommands
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskResumeCmd)
	taskCmd.AddCommand(taskArchiveCmd)
	taskCmd.AddCommand(taskUnarchiveCmd)
	taskCmd.AddCommand(taskCleanupCmd)
	taskCmd.AddCommand(taskStatusCmd)
	taskCmd.AddCommand(taskPriorityCmd)
	taskCmd.AddCommand(taskUpdateCmd)

	rootCmd.AddCommand(taskCmd)
}

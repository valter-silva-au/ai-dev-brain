package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewTaskCmd creates the task command with all subcommands
func NewTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage task lifecycle",
		Long:  `Commands for creating, resuming, archiving, and managing tasks`,
	}

	// Add subcommands
	taskCmd.AddCommand(
		newTaskCreateCmd(),
		newTaskResumeCmd(),
		newTaskStartCmd(),
		newTaskArchiveCmd(),
		newTaskUnarchiveCmd(),
		newTaskCleanupCmd(),
		newTaskDeleteCmd(),
		newTaskStatusCmd(),
		newTaskPriorityCmd(),
		newTaskUpdateCmd(),
		newTaskStartAllCmd(),
		newTaskCloseAllCmd(),
		newTaskRunWithRufloCmd(),
		newTaskNormalizeTitlesCmd(),
		newTaskMigrateTypesCmd(),
		newTaskMigrateBlockedByCmd(),
	)

	return taskCmd
}

// newTaskCreateCmd creates the 'task create' command
func newTaskCreateCmd() *cobra.Command {
	var (
		taskType    string
		repo        string
		priority    string
		owner       string
		tags        []string
		description string
		acceptance  []string
		initiative  string
		noLaunch    bool
	)

	cmd := &cobra.Command{
		Use:   "create <branch>",
		Short: "Create a new task",
		Long:  `Create a new task with worktree and branch isolation`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			branch := args[0]

			// Validate task type against the canonical Conventional set.
			tt, err := validateTaskType(taskType)
			if err != nil {
				return err
			}

			// Validate priority
			var p models.Priority
			switch priority {
			case "P0":
				p = models.PriorityP0
			case "P1":
				p = models.PriorityP1
			case "P2":
				p = models.PriorityP2
			case "P3":
				p = models.PriorityP3
			default:
				return fmt.Errorf("invalid priority: %s (must be P0, P1, P2, or P3)", priority)
			}

			// Create task options. Store the raw branch as the title;
			// the type prefix (e.g. `[feat]`) is added by the renderer
			// in `adb task status`. Pre-baking it here produced
			// `[feat] [feat] branch` double-prefixes in the output.
			opts := core.CreateTaskOpts{
				Title:              branch,
				Description:        description,
				AcceptanceCriteria: acceptance,
				TaskType:           tt,
				Priority:           p,
				Owner:              owner,
				Tags:               tags,
				Repo:               repo,
				Initiative:         initiative,
			}

			task, err := App.TaskManager.Create(opts)
			if err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}

			fmt.Printf("✓ Task %s created\n", task.ID)
			fmt.Printf("  Branch: %s\n", task.Branch)
			fmt.Printf("  Worktree: %s\n", task.WorktreePath)
			fmt.Printf("  Ticket: %s\n", task.TicketPath)

			// Launch workflow if a worktree was created — unless the caller
			// opted out via --no-launch / ADB_NO_LAUNCH. Suppressing the
			// launch makes `task create` safe for scripting, CI, and the MCP
			// server, none of which can drive an interactive Claude Code
			// session (which would otherwise block here indefinitely).
			if task.WorktreePath != "" && !suppressLaunch(noLaunch) {
				fmt.Println("\nLaunching workflow...")
				return launchWorkflow(taskLaunchInfo{
					TaskID:       task.ID,
					TaskType:     string(task.Type),
					Priority:     string(task.Priority),
					Status:       string(task.Status),
					WorktreePath: task.WorktreePath,
					Branch:       task.Branch,
				})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&taskType, "type", "feat", "Task type (feat, fix, refactor, docs, chore, test, perf, spike)")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name")
	cmd.Flags().StringVar(&priority, "priority", "P2", "Priority (P0, P1, P2, P3)")
	cmd.Flags().StringVar(&owner, "owner", "", "Task owner")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Task tags (comma-separated)")
	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringSliceVar(&acceptance, "acceptance", []string{}, "Acceptance criteria (comma-separated)")
	cmd.Flags().StringVar(&initiative, "initiative", "", "Associate the task with an initiative id (must exist; see `adb initiative list`)")
	cmd.Flags().BoolVar(&noLaunch, "no-launch", false, "Create the task and worktree without launching Claude Code (for scripting, CI, MCP)")

	return cmd
}

// suppressLaunch reports whether the post-create Claude Code launch should
// be skipped. The --no-launch flag wins; otherwise ADB_NO_LAUNCH=1 (set by
// CI or the MCP server) opts out globally so non-interactive callers never
// block on an interactive session.
func suppressLaunch(noLaunchFlag bool) bool {
	return noLaunchFlag || os.Getenv("ADB_NO_LAUNCH") == "1"
}

// validateTaskType parses a --type flag value into a models.TaskType, accepting
// only the canonical Conventional set (models.ValidTaskTypes). The legacy "bug"
// alias is rejected with a hint to use "fix" instead — bug-typed tasks are no
// longer minted, though ConventionalType still maps existing ones for branch
// names and `adb task migrate-types` rewrites them. Any other unknown value is
// rejected with the list of accepted types.
func validateTaskType(s string) (models.TaskType, error) {
	if s == string(models.TaskTypeBug) {
		return "", fmt.Errorf("task type %q is retired; use `fix` instead", s)
	}
	tt := models.TaskType(s)
	if !tt.IsValid() {
		return "", fmt.Errorf("invalid task type: %s (must be one of %s)", s, joinTaskTypes(models.ValidTaskTypes))
	}
	return tt, nil
}

// joinTaskTypes renders a comma-separated list of task types for error hints.
func joinTaskTypes(types []models.TaskType) string {
	parts := make([]string, len(types))
	for i, t := range types {
		parts[i] = string(t)
	}
	return strings.Join(parts, ", ")
}

// newTaskResumeCmd creates the 'task resume' command
func newTaskResumeCmd() *cobra.Command {
	var here bool

	cmd := &cobra.Command{
		Use:   "resume <task-id>",
		Short: "Resume a task",
		Long:  `Resume a task, promoting it from backlog to in_progress and launching the workflow`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			task, err := App.TaskManager.Resume(taskID)
			if err != nil {
				return fmt.Errorf("failed to resume task: %w", err)
			}

			fmt.Printf("✓ Task %s resumed\n", task.ID)
			fmt.Printf("  Status: %s\n", task.Status)
			fmt.Printf("  Worktree: %s\n", task.WorktreePath)

			// Decide where to launch Claude Code:
			//   - worktree on disk → run there (repo/worktree task)
			//   - else a ticket dir → run there (repo-less / no-worktree task:
			//     the planning docs live in the ticket dir)
			//   - neither → nothing to launch from; just report the status flip
			// This mirrors the VS Code extension's resolveCwd contract and means
			// a repo-less task now opens a Claude session instead of silently
			// doing nothing. launchWorkflow + launchClaudeCode start a NEW
			// conversation when none exists yet (no unconditional --continue).
			launchDir := task.WorktreePath
			if launchDir == "" {
				launchDir = task.TicketPath
			}
			if launchDir != "" {
				fmt.Println("\nLaunching workflow...")
				return launchWorkflow(taskLaunchInfo{
					TaskID:       task.ID,
					TaskType:     string(task.Type),
					Priority:     string(task.Priority),
					Status:       string(task.Status),
					WorktreePath: launchDir,
					Branch:       task.Branch,
					Resume:       true,
					Here:         here,
				})
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&here, "here", false,
		"Run the workflow in the current terminal (exec claude in-place), skipping the VS Code launch-file handoff")

	return cmd
}

// newTaskArchiveCmd creates the 'task archive' command
func newTaskArchiveCmd() *cobra.Command {
	var (
		force        bool
		keepWorktree bool
		pruneBranch  bool
	)

	cmd := &cobra.Command{
		Use:   "archive <task-id>",
		Short: "Archive a task",
		Long: `Archive a task by moving it to _archived/ and removing its worktree.

By default a worktree with uncommitted or unpushed work is left in place (its
work preserved) and a warning is printed; pass --force to remove it anyway, or
--keep-worktree to always keep it. --prune-branch also deletes the task's local
branch once its worktree is gone.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			if err := App.TaskManager.Archive(taskID, core.ArchiveOptions{
				Force:        force,
				KeepWorktree: keepWorktree,
				PruneBranch:  pruneBranch,
			}); err != nil {
				if !force {
					return fmt.Errorf("failed to archive task: %w", err)
				}
				fmt.Fprintf(os.Stderr, "Warning: archive completed with errors: %v\n", err)
			}

			fmt.Printf("✓ Task %s archived\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force worktree removal even with uncommitted/unpushed work (and archive despite errors)")
	cmd.Flags().BoolVar(&keepWorktree, "keep-worktree", false, "Keep the worktree after archiving")
	cmd.Flags().BoolVar(&pruneBranch, "prune-branch", false, "Delete the task's local branch after its worktree is removed")

	return cmd
}

// newTaskUnarchiveCmd creates the 'task unarchive' command
func newTaskUnarchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unarchive <task-id>",
		Short: "Unarchive a task",
		Long:  `Unarchive a task by moving it back from _archived/ to active tickets`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			if err := App.TaskManager.Unarchive(taskID); err != nil {
				return fmt.Errorf("failed to unarchive task: %w", err)
			}

			fmt.Printf("✓ Task %s unarchived\n", taskID)
			return nil
		},
	}

	return cmd
}

// newTaskCleanupCmd creates the 'task cleanup' command
func newTaskCleanupCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "cleanup <task-id>",
		Short: "Clean up a task's worktree",
		Long: `Remove only the worktree for a task, leaving ticket data intact.

Refuses to remove a worktree with uncommitted or unpushed work unless --force
is passed, so in-flight changes are never discarded silently.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			if err := App.TaskManager.Cleanup(taskID, force); err != nil {
				return fmt.Errorf("failed to cleanup task: %w", err)
			}

			fmt.Printf("✓ Task %s worktree cleaned up\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Remove the worktree even with uncommitted/unpushed work")

	return cmd
}

// newTaskStartCmd creates the 'task start' command — the singular counterpart
// to start-all (#210).
func newTaskStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <task-id>",
		Short: "Promote a task to in_progress (no session launch)",
		Long: `Promote a single backlog task to in_progress without launching a session
— the singular counterpart to start-all. Idempotent: a task that is not in
backlog is left unchanged. Use 'adb task resume' to also launch a session.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			taskID := args[0]
			if err := App.TaskManager.Start(taskID); err != nil {
				return fmt.Errorf("failed to start task: %w", err)
			}
			fmt.Printf("✓ Task %s is in_progress\n", taskID)
			return nil
		},
	}
	return cmd
}

// newTaskDeleteCmd wires the orphaned TaskManager.Delete to a subcommand (#210).
func newTaskDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <task-id>",
		Short: "Delete a task (worktree + ticket dir + backlog entry)",
		Long: `Permanently delete a task: removes its worktree, ticket directory, and
backlog entry. Destructive and irreversible — requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			taskID := args[0]
			if !yes {
				return fmt.Errorf("refusing to delete %s without --yes (removes worktree, ticket dir, and backlog entry)", taskID)
			}
			if err := App.TaskManager.Delete(taskID); err != nil {
				return fmt.Errorf("failed to delete task: %w", err)
			}
			fmt.Printf("✓ Task %s deleted\n", taskID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm deletion (required)")
	return cmd
}

// taskStatusJSON is the machine-readable shape emitted by
// `adb task status --json`. Field set mirrors the MCP server's task view so
// CLI and MCP consumers (e.g. the VS Code extension) see identical data.
type taskStatusJSON struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Owner    string   `json:"owner,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Repo     string   `json:"repo,omitempty"`
	// WorktreePath and TicketPath let machine consumers (the VS Code extension's
	// Start All) decide launchability and the terminal's working directory
	// without re-reading backlog.yaml. Branch is included for completeness.
	WorktreePath string `json:"worktree_path,omitempty"`
	TicketPath   string `json:"ticket_path,omitempty"`
	Branch       string `json:"branch,omitempty"`
	// Initiative is the associated founder-playbook initiative id (empty when
	// the ticket has no association). Metadata only — see pkg/models/task.go.
	Initiative string `json:"initiative,omitempty"`
}

// newTaskStatusCmd creates the 'task status' command
func newTaskStatusCmd() *cobra.Command {
	var filterStatus string
	var jsonOut bool
	var gitStatus bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "List tasks by status",
		Long:  `List all tasks, optionally filtered by status`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}

			// Filter tasks by status if specified
			tasks := backlog.Tasks
			if filterStatus != "" {
				var filtered []models.Task
				for _, task := range tasks {
					if string(task.Status) == filterStatus {
						filtered = append(filtered, task)
					}
				}
				tasks = filtered
			}

			// --git enriches the listing with live per-worktree git state,
			// reusing the `adb status` cross-repo report over the (optionally
			// filtered) task set (#209).
			if gitStatus {
				rows := buildStatusRows(tasks, App.GitWorktreeManager.WorktreeStatus)
				orphans := findOrphanedWorktrees(tasks)
				if jsonOut {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(statusReport{Tickets: rows, Orphaned: orphans})
				}
				printStatusTable(cmd.OutOrStdout(), rows, orphans)
				return nil
			}

			// JSON output: stable, always an array (possibly empty), for
			// machine consumers. Must run before the human-readable
			// "No tasks found" early return.
			if jsonOut {
				out := make([]taskStatusJSON, 0, len(tasks))
				for _, t := range tasks {
					out = append(out, taskStatusJSON{
						ID:           t.ID,
						Title:        t.Title,
						Type:         string(t.Type),
						Status:       string(t.Status),
						Priority:     string(t.Priority),
						Owner:        t.Owner,
						Tags:         t.Tags,
						Repo:         t.Repo,
						WorktreePath: t.WorktreePath,
						TicketPath:   t.TicketPath,
						Branch:       t.Branch,
						Initiative:   t.Initiative,
					})
				}
				data, err := json.MarshalIndent(out, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to encode tasks as JSON: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if len(tasks) == 0 {
				fmt.Println("No tasks found")
				return nil
			}

			// Group by status
			byStatus := make(map[models.TaskStatus][]models.Task)
			for _, task := range tasks {
				byStatus[task.Status] = append(byStatus[task.Status], task)
			}

			// Print grouped by status
			statuses := []models.TaskStatus{
				models.TaskStatusInProgress,
				models.TaskStatusReview,
				models.TaskStatusBlocked,
				models.TaskStatusBacklog,
				models.TaskStatusDone,
				models.TaskStatusArchived,
			}

			for _, status := range statuses {
				tasks := byStatus[status]
				if len(tasks) == 0 {
					continue
				}

				fmt.Printf("\n%s (%d):\n", strings.ToUpper(string(status)), len(tasks))
				for _, task := range tasks {
					fmt.Printf("  %s: [%s] %s [%s] (owner: %s)\n",
						task.ID, task.Type, task.Title, task.Priority, task.Owner)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&filterStatus, "filter", "", "Filter by status (backlog, in_progress, blocked, review, done, archived)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON (stable shape for machine consumers)")
	cmd.Flags().BoolVar(&gitStatus, "git", false, "Join with live per-worktree git state (branch, dirty, ahead/behind, missing)")

	return cmd
}

// newTaskPriorityCmd creates the 'task priority' command
func newTaskPriorityCmd() *cobra.Command {
	var newPriority string

	cmd := &cobra.Command{
		Use:   "priority <task-id>...",
		Short: "Update task priority",
		Long:  `Update the priority of one or more tasks`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Validate priority
			var p models.Priority
			switch newPriority {
			case "P0":
				p = models.PriorityP0
			case "P1":
				p = models.PriorityP1
			case "P2":
				p = models.PriorityP2
			case "P3":
				p = models.PriorityP3
			default:
				return fmt.Errorf("invalid priority: %s (must be P0, P1, P2, or P3)", newPriority)
			}

			// Update each task
			for _, taskID := range args {
				if err := App.TaskManager.UpdatePriority(taskID, p); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to update %s: %v\n", taskID, err)
					continue
				}
				fmt.Printf("✓ Task %s priority updated to %s\n", taskID, newPriority)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&newPriority, "priority", "", "New priority (P0, P1, P2, P3)")
	cmd.MarkFlagRequired("priority")

	return cmd
}

// printBulkResults renders the outcome of a bulk start/close operation and
// returns the number of failures so the caller can set a non-zero exit code.
func printBulkResults(verb string, results []core.BulkResult) int {
	if len(results) == 0 {
		fmt.Printf("No eligible tasks to %s\n", verb)
		return 0
	}
	failures := 0
	for _, r := range results {
		if r.Err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", r.TaskID, r.Err)
			continue
		}
		fmt.Printf("✓ %s: %s → %s\n", r.TaskID, r.OldStatus, r.NewStatus)
	}
	fmt.Printf("\n%d %sd, %d failed\n", len(results)-failures, verb, failures)
	return failures
}

// newTaskStartAllCmd creates the 'task start-all' command
func newTaskStartAllCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "start-all",
		Short: "Promote all backlog tasks to in_progress",
		Long: `Promote every task currently in the backlog to in_progress in one shot.

Only backlog tasks are affected. Tasks already in_progress, blocked, review,
done, or archived are left untouched, so the operation is safe to re-run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			if !yes {
				backlog, err := App.BacklogManager.Load()
				if err != nil {
					return fmt.Errorf("failed to load backlog: %w", err)
				}
				count := 0
				for _, t := range backlog.Tasks {
					if t.Status == models.TaskStatusBacklog {
						count++
					}
				}
				if count == 0 {
					fmt.Println("No backlog tasks to start")
					return nil
				}
				if !confirm(fmt.Sprintf("Start %d backlog task(s)? [y/N] ", count)) {
					fmt.Println("Aborted")
					return nil
				}
			}

			results, err := App.TaskManager.StartAll()
			if err != nil {
				return fmt.Errorf("failed to start tasks: %w", err)
			}
			if printBulkResults("start", results) > 0 {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// newTaskCloseAllCmd creates the 'task close-all' command
func newTaskCloseAllCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "close-all",
		Short: "Mark all active tasks as done",
		Long: `Mark every active task (in_progress, blocked, or review) as done in one shot.

Closing only flips the status to done; it does not archive tickets or remove
worktrees, so history stays intact and the change is reversible via
'adb task update --status'. Backlog, done, and archived tasks are untouched.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			if !yes {
				backlog, err := App.BacklogManager.Load()
				if err != nil {
					return fmt.Errorf("failed to load backlog: %w", err)
				}
				count := 0
				for _, t := range backlog.Tasks {
					if t.IsActive() {
						count++
					}
				}
				if count == 0 {
					fmt.Println("No active tasks to close")
					return nil
				}
				if !confirm(fmt.Sprintf("Close %d active task(s)? [y/N] ", count)) {
					fmt.Println("Aborted")
					return nil
				}
			}

			results, err := App.TaskManager.CloseAll()
			if err != nil {
				return fmt.Errorf("failed to close tasks: %w", err)
			}
			if printBulkResults("close", results) > 0 {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

// confirm prints a prompt to stderr and reads a yes/no answer from stdin.
// Returns true only for an explicit y/yes (case-insensitive). A closed or
// empty stdin returns false (safe default).
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	var answer string
	if _, err := fmt.Scanln(&answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// newTaskUpdateCmd creates the 'task update' command
func newTaskUpdateCmd() *cobra.Command {
	var (
		status     string
		priority   string
		owner      string
		initiative string
	)

	cmd := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update task properties",
		Long:  `Update various properties of a task`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			// Load task
			task, err := App.BacklogManager.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("failed to load task: %w", err)
			}

			updated := false

			// Update status if provided
			if status != "" {
				var s models.TaskStatus
				switch status {
				case "backlog":
					s = models.TaskStatusBacklog
				case "in_progress":
					s = models.TaskStatusInProgress
				case "blocked":
					s = models.TaskStatusBlocked
				case "review":
					s = models.TaskStatusReview
				case "done":
					s = models.TaskStatusDone
				case "archived":
					s = models.TaskStatusArchived
				default:
					return fmt.Errorf("invalid status: %s", status)
				}

				if err := App.TaskManager.UpdateStatus(taskID, s); err != nil {
					return fmt.Errorf("failed to update status: %w", err)
				}
				fmt.Printf("✓ Status updated to %s\n", status)
				updated = true
			}

			// Update priority if provided
			if priority != "" {
				var p models.Priority
				switch priority {
				case "P0":
					p = models.PriorityP0
				case "P1":
					p = models.PriorityP1
				case "P2":
					p = models.PriorityP2
				case "P3":
					p = models.PriorityP3
				default:
					return fmt.Errorf("invalid priority: %s", priority)
				}

				if err := App.TaskManager.UpdatePriority(taskID, p); err != nil {
					return fmt.Errorf("failed to update priority: %w", err)
				}
				fmt.Printf("✓ Priority updated to %s\n", priority)
				updated = true
			}

			// Update owner if provided
			if owner != "" {
				task.Owner = owner
				task.UpdateTimestamp()
				if err := App.BacklogManager.UpdateTask(*task); err != nil {
					return fmt.Errorf("failed to update owner: %w", err)
				}
				fmt.Printf("✓ Owner updated to %s\n", owner)
				updated = true
			}

			// Update the initiative association if the flag was set. Using
			// Changed (not initiative != "") lets `--initiative ""` explicitly
			// CLEAR the association while omitting the flag leaves it untouched.
			if cmd.Flags().Changed("initiative") {
				if _, err := App.TaskManager.SetInitiative(taskID, initiative); err != nil {
					return fmt.Errorf("failed to update initiative: %w", err)
				}
				if initiative == "" {
					fmt.Println("✓ Initiative association cleared")
				} else {
					fmt.Printf("✓ Initiative set to %s\n", initiative)
				}
				updated = true
			}

			if !updated {
				fmt.Println("No updates specified. Use --status, --priority, --owner, or --initiative flags.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().StringVar(&priority, "priority", "", "New priority")
	cmd.Flags().StringVar(&owner, "owner", "", "New owner")
	cmd.Flags().StringVar(&initiative, "initiative", "", "Associate with an initiative id (must exist); pass \"\" to clear")

	return cmd
}

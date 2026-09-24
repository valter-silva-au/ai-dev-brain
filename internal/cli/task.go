package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewTaskCmd creates the task command with all subcommands.
//
// The vocabulary was settled in TASK-00039 (Q7): the visible verbs are
// create/list/show/start/resume/update/close/archive/remove/validate/timeline
// plus the `worktree` namespace. Five older spellings survive as HIDDEN
// deprecated aliases, and they split into two kinds:
//
//   - renames (status→list, delete→remove, cleanup→worktree remove) are built
//     from the NEW command's constructor, so the two spellings cannot drift;
//   - merges (priority→update --priority, unarchive→update --status backlog)
//     cannot be, because the shapes differ — `priority` takes many ids and a
//     required flag where `update` takes one id and four optional ones. Those
//     two keep their own retired constructor and delegate to the same core
//     method the new spelling uses, so the duplication is at the CLI edge only.
func NewTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage task lifecycle",
		Long:  `Commands for creating, listing, working, closing, and retiring tasks`,
	}

	taskCmd.AddCommand(
		newTaskCreateCmd(),
		newTaskListCmd(),
		newTaskShowCmd(),
		newTaskStartCmd(),
		newTaskResumeCmd(),
		newTaskUpdateCmd(),
		newTaskCloseCmd(),
		newTaskArchiveCmd(),
		newTaskRemoveCmd(),
		newTaskValidateCmd(),
		newTaskTimelineCmd(),
		newTaskWorktreeCmd(),
		newTaskMigrateBlockedByCmd(),
	)

	// The retired spellings. Hidden, still reachable, each naming where it went.
	taskCmd.AddCommand(
		deprecatedAlias(newTaskListCmd, "status", "adb task list"),
		deprecatedAlias(newTaskRemoveCmd, "delete", "adb task remove"),
		deprecatedAlias(newTaskWorktreeRemoveCmd, "cleanup", "adb task worktree remove"),
		deprecatedAlias(newTaskPriorityCmd, "priority", "adb task update --priority"),
		deprecatedAlias(newTaskUnarchiveCmd, "unarchive", "adb task update --status backlog"),
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
		agentFlag   string
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

			// Validate the agent BEFORE creating anything. A repo-less task never
			// reaches the launch block below, so a bad --agent used to be silently
			// ignored and the create still reported success.
			if err := validateAgentFlag(agentFlag); err != nil {
				return err
			}

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
			// in `adb task list`. Pre-baking it here produced
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
			// server, none of which can drive an interactive agent session
			// (which would otherwise block here indefinitely).
			if task.WorktreePath != "" && !suppressLaunch(noLaunch) {
				agent, err := resolveAgent(agentFlag)
				if err != nil {
					return err
				}
				fmt.Println("\nLaunching workflow...")
				return launchWorkflow(taskLaunchInfo{
					TaskID:       task.ID,
					TaskType:     string(task.Type),
					Priority:     string(task.Priority),
					Status:       string(task.Status),
					WorktreePath: task.WorktreePath,
					Branch:       task.Branch,
					Agent:        agent,
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
	cmd.Flags().StringVar(&initiative, "initiative", "", "Associate the task with an initiative id (must exist; see 'adb initiative list')")
	cmd.Flags().BoolVar(&noLaunch, "no-launch", false, "Create the task and worktree without launching the coding agent (for scripting, CI, MCP)")
	cmd.Flags().StringVar(&agentFlag, "agent", "", agentFlagUsage())

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
// names so a pre-existing row keeps working. Any other unknown value is
// rejected with the list of accepted types.
//
// There is no longer a migration command: `adb task migrate-types` was removed
// with the rest of the one-shot migrations. backlog.yaml is the sole
// authoritative type store, so retyping an old `bug` row is a one-line edit
// there.
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
	var agentFlag string

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

			// Validate the agent before promoting: a bad --agent must not leave the
			// task in_progress with nothing launched.
			if err := validateAgentFlag(agentFlag); err != nil {
				return err
			}

			task, err := App.TaskManager.Resume(taskID)
			if err != nil {
				return fmt.Errorf("failed to resume task: %w", err)
			}

			fmt.Printf("✓ Task %s resumed\n", task.ID)
			fmt.Printf("  Status: %s\n", task.Status)
			fmt.Printf("  Worktree: %s\n", task.WorktreePath)

			// Decide where to launch the coding agent: the worktree when there is
			// one, else the ticket dir (a repo-less task's planning docs live
			// there), else nothing to launch from — just report the status flip.
			// This mirrors the VS Code extension's resolveCwd contract.
			launchDir := launchDirFor(task)
			if launchDir != "" {
				agent, err := resolveAgent(agentFlag)
				if err != nil {
					return err
				}

				// ADB_NO_LAUNCH=1 is a SESSION-WIDE opt-out — you export it once and
				// every adb command in that shell stops opening interactive sessions.
				// `task create` and `task start` both honour it and resume did not,
				// so a script that set it still got handed an interactive session it
				// had explicitly disabled (TASK-00039 Batch 4, follow-up 2).
				//
				// Resume deliberately has NO --no-launch flag: "promote without
				// launching" is already `adb task start --no-launch`, and a flag here
				// would reduce to `adb task update --status in_progress`. The env var
				// is different in kind — it is a global the user already set, and
				// honouring it in two of three launch paths was the inconsistency.
				//
				// Say so rather than going quiet: somebody who forgot the variable is
				// exported needs to know why no session opened. It goes to STDERR, so
				// a caller parsing stdout is unaffected.
				if suppressLaunch(false) {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"\nADB_NO_LAUNCH=1 is set — promoted only, not launching %s in %s\n",
						agentDisplayName(agent), launchDir)
					return nil
				}
				// Say which of the two things is about to happen. The launchers
				// pick continue-vs-fresh silently, and "did it remember?" is the
				// first thing you need to know before typing into the session.
				if priorSessionExists(agent, launchDir) {
					fmt.Printf("\nContinuing the previous %s session...\n", agentDisplayName(agent))
				} else {
					fmt.Printf("\nNo previous %s session here — starting a new one...\n",
						agentDisplayName(agent))
				}
				return launchWorkflow(taskLaunchInfo{
					TaskID:       task.ID,
					TaskType:     string(task.Type),
					Priority:     string(task.Priority),
					Status:       string(task.Status),
					WorktreePath: launchDir,
					Branch:       task.Branch,
					Resume:       true,
					Agent:        agent,
				})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", agentFlagUsage())

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

// newTaskStartCmd creates the 'task start' command — promote and launch a NEW
// session, the counterpart to `resume`'s continue-or-fall-back (TASK-00039).
func newTaskStartCmd() *cobra.Command {
	var agentFlag string
	var noLaunch bool

	cmd := &cobra.Command{
		Use:   "start <task-id>",
		Short: "Start a task: promote it and launch a NEW agent session",
		Long: `Promote a task to in_progress and launch a fresh coding-agent session in its
worktree.

'start' always begins a NEW conversation; 'adb task resume' continues the last
one for that directory and falls back to starting fresh when there is none. That
is the whole difference between the two — pick 'start' when you want the agent to
come in cold, 'resume' when you want it to remember.

Promotion is idempotent: a task that is not in backlog keeps its current status,
and the session is launched either way. Pass --no-launch to promote only (also
honours ADB_NO_LAUNCH=1), which is what scripts and CI want.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			taskID := args[0]
			// Validate the agent before promoting: a bad --agent must not leave the
			// task in_progress with nothing launched.
			if err := validateAgentFlag(agentFlag); err != nil {
				return err
			}
			if err := App.TaskManager.Start(taskID); err != nil {
				return fmt.Errorf("failed to start task: %w", err)
			}
			fmt.Printf("✓ Task %s is in_progress\n", taskID)

			if noLaunch || os.Getenv("ADB_NO_LAUNCH") == "1" {
				return nil
			}

			task, err := App.BacklogManager.GetTask(taskID)
			if err != nil || task == nil {
				return fmt.Errorf("failed to load task %s for launch: %w", taskID, err)
			}

			launchDir := launchDirFor(task)
			if launchDir == "" {
				// Nothing to launch from — a repo-less task with no ticket dir.
				return nil
			}

			agent, err := resolveAgent(agentFlag)
			if err != nil {
				return err
			}
			fmt.Printf("\nStarting a new %s session...\n", agentDisplayName(agent))
			return launchWorkflow(taskLaunchInfo{
				TaskID:       task.ID,
				TaskType:     string(task.Type),
				Priority:     string(task.Priority),
				Status:       string(task.Status),
				WorktreePath: launchDir,
				Branch:       task.Branch,
				Resume:       false,
				Agent:        agent,
			})
		},
	}

	cmd.Flags().StringVar(&agentFlag, "agent", "", agentFlagUsage())
	cmd.Flags().BoolVar(&noLaunch, "no-launch", false, "Promote only; do not launch a session (also honours ADB_NO_LAUNCH=1)")

	return cmd
}

// launchDirFor picks the directory a coding agent should be started in:
// the worktree when the task has one on disk, else the ticket directory (a
// repo-less task's planning docs live there), else nothing.
func launchDirFor(task *models.Task) string {
	if task == nil {
		return ""
	}
	if task.WorktreePath != "" {
		return task.WorktreePath
	}
	return task.TicketPath
}

// newTaskRemoveCmd wires TaskManager.Delete to a subcommand (#210). Named
// `remove` rather than `delete` for consistency with every other remove verb on
// the surface (`task worktree remove`, `adb schedule remove`) — the underlying
// core method keeps its own name.
func newTaskRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <task-id>",
		Short: "Remove a task entirely (worktree + ticket dir + backlog entry)",
		Long: `Permanently remove a task: its worktree, its ticket directory, and its
backlog entry. Destructive and irreversible — requires --yes.

To retire a ticket while keeping its record, use 'adb task archive'. To reclaim
only the worktree, 'adb task worktree remove'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			taskID := args[0]
			if !yes {
				return fmt.Errorf("refusing to remove %s without --yes (removes worktree, ticket dir, and backlog entry)", taskID)
			}
			if err := App.TaskManager.Delete(taskID); err != nil {
				return fmt.Errorf("failed to remove task: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Task %s removed\n", taskID)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm removal (required)")
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
	requireFlag(cmd, "priority")

	return cmd
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
				s := models.TaskStatus(status)
				if !models.IsValidTaskStatus(s) {
					return fmt.Errorf(
						"invalid status: %s (want one of: %s)",
						status, strings.Join(models.ValidTaskStatusNames(), ", "),
					)
				}

				// Archiving is not a field write. It moves the ticket directory
				// under _archived/ and removes the worktree, and it owns the
				// flags that govern both (--force / --keep-worktree /
				// --prune-branch). Writing the field alone produced a workspace
				// that claimed a task was archived while its worktree and ticket
				// dir were still live — so refuse, and name the command that
				// does the job properly.
				if s == models.TaskStatusArchived {
					return fmt.Errorf(
						"refusing to set status archived directly: archiving also moves the ticket directory and removes the worktree — use `adb task archive %s`",
						taskID,
					)
				}

				// The mirror image: `--status backlog` on an ALREADY-archived
				// task is an unarchive, and unarchiving is likewise a directory
				// move rather than a field write. Route it through Unarchive so
				// this flag is genuinely equivalent to the retired
				// `adb task unarchive`, instead of leaving the ticket in
				// _archived/ with a backlog status.
				switch {
				case task.Status == models.TaskStatusArchived && s == models.TaskStatusBacklog:
					if err := App.TaskManager.Unarchive(taskID); err != nil {
						return fmt.Errorf("failed to unarchive task: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "✓ Task %s unarchived (status backlog)\n", taskID)
				default:
					if err := App.TaskManager.UpdateStatus(taskID, s); err != nil {
						return fmt.Errorf("failed to update status: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "✓ Status updated to %s\n", status)
				}
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

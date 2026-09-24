package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// newTaskWorktreeCmd creates the `adb task worktree` namespace: the worktree
// behind a task, as opposed to `adb repo worktree`, which manages a repository's
// registered worktrees.
//
// ONE SAFETY RULE spans both namespaces:
//
//	a worktree mutation that NAMES its target acts immediately;
//	one that SWEEPS a set previews unless --apply.
//
// So `task worktree remove <task-id>` acts, while `task worktree prune` and
// `task worktree reconcile` preview. This is what de-conflicts the two
// `worktree prune` verbs: v3's `repo worktree prune` already previewed unless
// --apply AND required an exact `--path`, while `adb work prune` removed on
// sight. Neither sweeps-and-removes now, so the remaining difference between
// them is scope, not safety. Read this before changing a default here.
func newTaskWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Operate over task worktrees (list, switch, remove, prune, reconcile)",
		Long: `Inspect and manage the git worktrees behind repo-backed tasks.

A verb that names its target acts immediately: 'remove <task-id>'. A verb that
sweeps a set previews by default and mutates only with --apply: 'prune',
'reconcile'. For a repository's registered worktrees rather than a task's, see
'adb repo worktree'.`,
	}
	cmd.AddCommand(
		newTaskWorktreeListCmd(),
		newTaskWorktreeSwitchCmd(),
		newTaskWorktreeRemoveCmd(),
		newTaskWorktreePruneCmd(),
		newTaskWorktreeReconcileCmd(),
	)
	return cmd
}

// worktreeReport is the machine-readable shape of `adb task worktree list --json`.
//
// This is the one task listing with an envelope, and the reason is structural:
// an orphaned worktree has no task row to hang off, so it cannot be a key on an
// element. `adb task list --json` stays a bare array precisely because
// everything it reports IS a task.
type worktreeReport struct {
	Worktrees []statusRow `json:"worktrees"`
	Orphaned  []string    `json:"orphaned"`
}

// newTaskWorktreeListCmd lists task worktrees with branch, on-disk presence, and
// the worktrees no active ticket owns.
func newTaskWorktreeListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List task worktrees (branch, present/missing) and orphans",
		Long: `List the worktree behind every repo-backed task, with its live branch and
whether it is present on disk — plus the worktrees found on disk that no active
ticket owns.

Only worktree-bearing tasks appear. For every task, worktree or not, use
'adb task list --git'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			rows := buildStatusRows(backlog.Tasks, App.GitWorktreeManager.WorktreeStatus)
			orphans := findOrphanedWorktrees(backlog.Tasks)

			if asJSON {
				// Both slices must marshal as [] rather than null, so
				// `jq '.orphaned | length'` works on a clean workspace.
				if rows == nil {
					rows = []statusRow{}
				}
				if orphans == nil {
					orphans = []string{}
				}
				return writeIndentedJSON(cmd.OutOrStdout(), worktreeReport{Worktrees: rows, Orphaned: orphans})
			}
			printWorktreeTable(cmd.OutOrStdout(), rows, orphans)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit as JSON")
	return cmd
}

// newTaskWorktreeSwitchCmd prints a task's worktree path so a shell can cd into it:
//
//	cd "$(adb task worktree switch TASK-00042)"
func newTaskWorktreeSwitchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <task-id>",
		Short: "Print a task's worktree path (a cd target)",
		Long: `Resolve a task's worktree path and print it so a shell can switch to it:

  cd "$(adb task worktree switch TASK-00042)"

Errors if the task has no worktree or the worktree is missing on disk (run
'adb task worktree reconcile --apply' to rebuild it).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			task, err := App.BacklogManager.GetTask(args[0])
			if err != nil {
				return fmt.Errorf("failed to load task: %w", err)
			}
			if task.WorktreePath == "" {
				return fmt.Errorf("task %s has no worktree", task.ID)
			}
			if _, err := os.Stat(task.WorktreePath); os.IsNotExist(err) {
				return fmt.Errorf(
					"worktree for %s is missing at %s (run 'adb task worktree reconcile --apply')",
					task.ID, task.WorktreePath,
				)
			}
			fmt.Fprintln(cmd.OutOrStdout(), task.WorktreePath)
			return nil
		},
	}
	return cmd
}

// newTaskWorktreeRemoveCmd removes ONE task's worktree, leaving ticket data
// intact. Formerly `adb task cleanup` — a name that said when you would run it
// rather than what it did.
//
// It names its target, so per the namespace rule it acts immediately; the #207
// dirty guard is what keeps that safe.
func newTaskWorktreeRemoveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "remove <task-id>",
		Short: "Remove one task's worktree, keeping its ticket data",
		Long: `Remove only the worktree for a task, leaving the ticket directory, notes, and
backlog entry intact.

Refuses to remove a worktree with uncommitted or unpushed work unless --force is
passed, so in-flight changes are never discarded silently.

NOTE: this also CLEARS the task's recorded worktree_path, so
'adb task worktree reconcile' will not rebuild it — reconcile restores worktrees
that went missing, not ones you deliberately removed. Re-create it with
'adb task resume <task-id>'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			taskID := args[0]

			// Read the path BEFORE removing it, so the report can distinguish
			// "removed it" from "there was nothing to remove". Cleanup no-ops on
			// a task with no worktree, and reporting that as a removal is a
			// small lie that reads as confirmation the caller did not earn.
			task, err := App.BacklogManager.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("failed to load task: %w", err)
			}
			hadWorktree := task != nil && task.WorktreePath != ""

			// Not wrapped: TaskManager.Cleanup's own error already begins
			// "failed to remove worktree", and a second identical prefix reads
			// as a stutter rather than as context.
			if err := App.TaskManager.Cleanup(taskID, force); err != nil {
				return err
			}
			if !hadWorktree {
				fmt.Fprintf(cmd.OutOrStdout(), "Task %s has no worktree; nothing to remove.\n", taskID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Task %s worktree removed\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Remove the worktree even with uncommitted/unpushed work")

	return cmd
}

// addSweepFlags wires the preview-by-default contract onto a sweeping verb.
//
// --dry-run is kept as an accepted no-op, hidden: preview IS the default now, so
// `adb work prune --dry-run` (the retired spelling, which deprecatedAlias builds
// from this constructor) keeps parsing and keeps meaning what it always meant.
// Removing the flag would break that invocation for no gain.
func addSweepFlags(cmd *cobra.Command, apply, force, dryRun *bool) {
	cmd.Flags().BoolVar(apply, "apply", false, "Actually remove/rebuild (default: preview only)")
	cmd.Flags().BoolVar(force, "force", false, "Include orphans with uncommitted/unpushed work")
	cmd.Flags().BoolVar(dryRun, "dry-run", false, "Deprecated no-op: previewing is the default")
	hideFlag(cmd, "dry-run")
}

// newTaskWorktreePruneCmd removes worktrees that no active ticket owns.
//
// It sweeps a set the caller did not enumerate, so it previews unless --apply.
func newTaskWorktreePruneCmd() *cobra.Command {
	var apply, force, dryRun bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees that no active ticket owns (preview unless --apply)",
		Long: `Find worktrees present on disk that no active ticket owns and remove them.

PREVIEWS BY DEFAULT — pass --apply to actually remove. This verb sweeps a set you
did not enumerate, which is why it does not act on sight; 'adb task worktree
remove <task-id>' names its target and therefore does.

Respects the safe-teardown guard: a dirty or unpushed orphan is kept (with a
warning) unless --force.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			orphans := findOrphanedWorktrees(backlog.Tasks)
			if len(orphans) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No orphaned worktrees.")
				return nil
			}
			for _, p := range orphans {
				if !apply {
					fmt.Fprintf(cmd.OutOrStdout(), "would prune: %s\n", p)
					continue
				}
				// Through TaskManager, not GitWorktreeManager: removal has one
				// home in core and that is where worktree.removed is emitted.
				// Calling the git remover directly here is what made a prune
				// invisible to `adb metrics` (#206's created/removed balance).
				if err := App.TaskManager.RemoveOrphanWorktree(p, force); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "kept %s: %v\n", p, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "pruned: %s\n", p)
			}
			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d orphan(s) would be pruned. Re-run with --apply to remove them.\n", len(orphans))
			}
			return nil
		},
	}

	addSweepFlags(cmd, &apply, &force, &dryRun)
	return cmd
}

// newTaskWorktreeReconcileCmd rebuilds missing worktrees from backlog.yaml so
// work/ and repos/ are fully rebuildable — a fresh-machine bootstrap (#211).
//
// It sweeps every ticket, so it previews unless --apply.
func newTaskWorktreeReconcileCmd() *cobra.Command {
	var apply, prune, force, dryRun bool

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Rebuild missing worktrees recorded in backlog.yaml (preview unless --apply)",
		Long: `For every non-archived ticket with a repo + branch + worktree_path whose
worktree is missing on disk, clone the repo on demand and rebuild the worktree
(attaching the existing branch, or recreating it from the base branch). Makes
work/ and repos/ rebuildable — delete work/ and reconcile restores it with no
manual git.

PREVIEWS BY DEFAULT — pass --apply to actually rebuild. With --prune, also remove
worktrees no active ticket owns (respecting the safe-teardown dirty guard;
--force to override).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return errNoApp
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			baseBranch := ""
			if App.MergedConfig != nil && App.MergedConfig.Repo != nil {
				baseBranch = App.MergedConfig.Repo.BaseBranch
			}

			restored, failed, pending := 0, 0, 0
			for _, task := range backlog.Tasks {
				if task.Status == models.TaskStatusArchived || task.Repo == "" || task.Branch == "" || task.WorktreePath == "" {
					continue
				}
				if _, err := os.Stat(task.WorktreePath); err == nil {
					continue // already present
				}
				if !apply {
					fmt.Fprintf(cmd.OutOrStdout(), "would restore: %s → %s\n", task.ID, task.WorktreePath)
					pending++
					continue
				}
				if _, err := App.GitWorktreeManager.RestoreWorktree(task.Repo, task.Branch, task.WorktreePath, baseBranch); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "failed to restore %s: %v\n", task.ID, err)
					failed++
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "restored: %s → %s\n", task.ID, task.WorktreePath)
				restored++
			}

			if prune {
				for _, p := range findOrphanedWorktrees(backlog.Tasks) {
					if !apply {
						fmt.Fprintf(cmd.OutOrStdout(), "would prune: %s\n", p)
						pending++
						continue
					}
					// Same sweep as `task worktree prune`, so it goes through the
					// same core method and logs the same worktree.removed event.
					if err := App.TaskManager.RemoveOrphanWorktree(p, force); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "kept %s: %v\n", p, err)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "pruned: %s\n", p)
				}
			}

			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d change(s) planned. Re-run with --apply to write them.\n", pending)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reconcile: %d restored, %d failed\n", restored, failed)
			if failed > 0 {
				return fmt.Errorf("%d worktree(s) failed to restore", failed)
			}
			return nil
		},
	}

	addSweepFlags(cmd, &apply, &force, &dryRun)
	cmd.Flags().BoolVar(&prune, "prune", false, "Also remove worktrees no active ticket owns")
	return cmd
}

// printWorktreeTable renders the human `task worktree list` view, including the
// orphan section — this command owns orphans, so it is the only one that prints
// them.
func printWorktreeTable(w interface{ Write([]byte) (int, error) }, rows []statusRow, orphans []string) {
	if len(rows) == 0 && len(orphans) == 0 {
		// A bare header with no rows reads as a rendering failure rather than as
		// an answer.
		fmt.Fprintln(w, "No task worktrees.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "TASK\tBRANCH\tSTATE\tWORKTREE")
	for _, r := range rows {
		state := "present"
		if r.WorktreeMissing {
			state = "MISSING"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Branch, state, r.Worktree)
	}
	_ = tw.Flush()
	if len(orphans) > 0 {
		fmt.Fprintf(w, "\nOrphaned worktrees (no active ticket owns these):\n")
		for _, p := range orphans {
			fmt.Fprintf(w, "  %s\n", p)
		}
	}
}

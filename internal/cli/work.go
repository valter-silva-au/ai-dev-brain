package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewWorkCmd creates the `adb work` namespace, surfacing the worktree
// primitives: list, switch (print a cd target), and prune orphans (#210).
func NewWorkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Operate over task worktrees (list, switch, prune)",
		Long:  `Inspect and manage the git worktrees behind repo-backed tasks.`,
	}
	cmd.AddCommand(newWorkListCmd(), newWorkSwitchCmd(), newWorkPruneCmd(), newWorkReconcileCmd())
	return cmd
}

// newWorkReconcileCmd rebuilds missing worktrees from backlog.yaml so work/ and
// repos/ are fully rebuildable — a fresh-machine bootstrap (#211).
func newWorkReconcileCmd() *cobra.Command {
	var prune, force, dryRun bool
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Rebuild missing worktrees recorded in backlog.yaml",
		Long: `For every non-archived ticket with a repo + branch + worktree_path whose
worktree is missing on disk, clone the repo on demand and rebuild the worktree
(attaching the existing branch, or recreating it from the base branch). Makes
work/ and repos/ rebuildable — delete work/ and reconcile restores it with no
manual git. With --prune, also remove worktrees no active ticket owns
(respecting the safe-teardown dirty guard; --force to override).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			baseBranch := ""
			if App.MergedConfig != nil && App.MergedConfig.Repo != nil {
				baseBranch = App.MergedConfig.Repo.BaseBranch
			}

			restored, failed := 0, 0
			for _, task := range backlog.Tasks {
				if task.Status == models.TaskStatusArchived || task.Repo == "" || task.Branch == "" || task.WorktreePath == "" {
					continue
				}
				if _, err := os.Stat(task.WorktreePath); err == nil {
					continue // already present
				}
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "would restore: %s → %s\n", task.ID, task.WorktreePath)
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
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "would prune: %s\n", p)
						continue
					}
					if err := App.GitWorktreeManager.RemoveWorktree(p, force); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "kept %s: %v\n", p, err)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "pruned: %s\n", p)
				}
			}

			if !dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "reconcile: %d restored, %d failed\n", restored, failed)
			}
			if failed > 0 {
				return fmt.Errorf("%d worktree(s) failed to restore", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "Also remove worktrees no active ticket owns")
	cmd.Flags().BoolVar(&force, "force", false, "With --prune, remove even dirty/unpushed orphans")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored/pruned without doing it")
	return cmd
}

// newWorkListCmd lists task worktrees with their branch and on-disk presence.
func newWorkListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List task worktrees (branch, present/missing) across every repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			rows := buildStatusRows(backlog.Tasks, App.GitWorktreeManager.WorktreeStatus)

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "TASK\tBRANCH\tSTATE\tWORKTREE")
			for _, r := range rows {
				state := "present"
				if r.Missing {
					state = "MISSING"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Branch, state, r.Worktree)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit as JSON")
	return cmd
}

// newWorkSwitchCmd prints a task's worktree path so a shell can cd into it:
//
//	cd "$(adb work switch TASK-00042)"
func newWorkSwitchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <task-id>",
		Short: "Print a task's worktree path (a cd target)",
		Long: `Resolve a task's worktree path and print it so a shell can switch to it:

  cd "$(adb work switch TASK-00042)"

Errors if the task has no worktree or the worktree is missing on disk (run
'adb work reconcile' to rebuild it).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			task, err := App.BacklogManager.GetTask(args[0])
			if err != nil {
				return fmt.Errorf("failed to load task: %w", err)
			}
			if task.WorktreePath == "" {
				return fmt.Errorf("task %s has no worktree", task.ID)
			}
			if _, err := os.Stat(task.WorktreePath); os.IsNotExist(err) {
				return fmt.Errorf("worktree for %s is missing at %s (run 'adb work reconcile')", task.ID, task.WorktreePath)
			}
			fmt.Fprintln(cmd.OutOrStdout(), task.WorktreePath)
			return nil
		},
	}
	return cmd
}

// newWorkPruneCmd removes worktrees that no active ticket owns, respecting the
// #207 dirty guard (a dirty/unpushed orphan is kept unless --force).
func newWorkPruneCmd() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees that no active ticket owns",
		Long: `Find worktrees present on disk that no active ticket owns and remove them.
Respects the safe-teardown guard: a dirty or unpushed orphan is kept (with a
warning) unless --force. Use --dry-run to preview.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
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
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "would prune: %s\n", p)
					continue
				}
				if err := App.GitWorktreeManager.RemoveWorktree(p, force); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "kept %s: %v\n", p, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "pruned: %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove even orphans with uncommitted/unpushed work")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be pruned without removing")
	return cmd
}

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// statusRow is one line of the cross-repo status report: a ticket joined with
// the live git state of its worktree (#209).
type statusRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Repo     string `json:"repo,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Worktree string `json:"worktree_path,omitempty"`
	Exists   bool   `json:"worktree_exists"`
	Dirty    bool   `json:"dirty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Missing  bool   `json:"missing"` // worktree_path recorded but absent on disk
}

// statusReport is the machine-readable shape of `adb status --json`.
type statusReport struct {
	Tickets  []statusRow `json:"tickets"`
	Orphaned []string    `json:"orphaned_worktrees"`
}

// buildStatusRows joins each non-archived, worktree-bearing task with its live
// git state (via the injected gitStatus func, so it is testable without git).
// A recorded worktree that is absent on disk is flagged Missing. Rows are
// returned in stable ID order.
func buildStatusRows(tasks []models.Task, gitStatus func(worktreePath string) (integration.WorktreeStatus, error)) []statusRow {
	rows := make([]statusRow, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == models.TaskStatusArchived || task.WorktreePath == "" {
			continue
		}
		row := statusRow{
			ID:       task.ID,
			Title:    task.Title,
			Status:   string(task.Status),
			Repo:     task.Repo,
			Branch:   task.Branch,
			Worktree: task.WorktreePath,
		}
		st, err := gitStatus(task.WorktreePath)
		switch {
		case err != nil:
			// The directory may exist but git couldn't report — don't claim
			// missing; keep the backlog-recorded branch.
			row.Exists = true
		case !st.Exists:
			row.Missing = true
		default:
			row.Exists = true
			if st.Branch != "" {
				row.Branch = st.Branch
			}
			row.Dirty = st.Dirty
			row.Ahead = st.Ahead
			row.Behind = st.Behind
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// NewStatusCmd creates the top-level `adb status` command: every active ticket
// across every repo joined with live git state (#209).
func NewStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Cross-repo status: every active ticket joined with live git state",
		Long: `Join backlog.yaml with per-worktree git state — branch, dirty/clean,
ahead/behind, and whether the worktree exists on disk — across every repo.
Flags tickets whose worktree is missing and worktrees no ticket owns.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			rows := buildStatusRows(backlog.Tasks, App.GitWorktreeManager.WorktreeStatus)
			orphans := findOrphanedWorktrees(backlog.Tasks)

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(statusReport{Tickets: rows, Orphaned: orphans})
			}
			printStatusTable(cmd.OutOrStdout(), rows, orphans)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the report as JSON")
	return cmd
}

// printStatusTable renders the human-readable status table.
func printStatusTable(w io.Writer, rows []statusRow, orphans []string) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "TASK\tSTATUS\tBRANCH\tGIT\tREPO")
	for _, r := range rows {
		git := "clean"
		switch {
		case r.Missing:
			git = "MISSING"
		case r.Dirty:
			git = "dirty"
		}
		if r.Ahead > 0 || r.Behind > 0 {
			git = fmt.Sprintf("%s ↑%d↓%d", git, r.Ahead, r.Behind)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Status, r.Branch, git, r.Repo)
	}
	_ = tw.Flush()
	if len(orphans) > 0 {
		fmt.Fprintf(w, "\nOrphaned worktrees (no active ticket owns these):\n")
		for _, p := range orphans {
			fmt.Fprintf(w, "  %s\n", p)
		}
	}
}

// findOrphanedWorktrees lists, per repo referenced by an active ticket, the
// worktrees present on disk that no active ticket owns — best-effort (a repo
// whose clone or git listing can't be read is skipped). Surfaces the otherwise
// unexposed ListWorktrees (#209).
func findOrphanedWorktrees(tasks []models.Task) []string {
	if App == nil || App.GitWorktreeManager == nil {
		return nil
	}
	owned := make(map[string]bool)
	for _, t := range tasks {
		if t.WorktreePath != "" {
			owned[canonPath(t.WorktreePath)] = true
		}
	}

	orphans := []string{}
	seenRepo := make(map[string]bool)
	for _, t := range tasks {
		if t.Repo == "" || t.Status == models.TaskStatusArchived {
			continue
		}
		norm, err := App.GitWorktreeManager.NormalizeRepoPath(t.Repo)
		if err != nil || seenRepo[norm] {
			continue
		}
		seenRepo[norm] = true
		cloneDir := norm
		if !filepath.IsAbs(norm) {
			cloneDir = filepath.Join(App.BasePath, "repos", norm)
		}
		infos, err := App.GitWorktreeManager.ListWorktrees(cloneDir)
		if err != nil {
			continue
		}
		for _, wt := range infos {
			p := canonPath(wt.Path)
			if p == canonPath(cloneDir) { // the primary clone, not a task worktree
				continue
			}
			if !owned[p] {
				orphans = append(orphans, wt.Path)
			}
		}
	}
	sort.Strings(orphans)
	return orphans
}

// canonPath resolves symlinks + cleans a path so ListWorktrees' resolved paths
// compare equal to a task's stored worktree path across platforms.
func canonPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}

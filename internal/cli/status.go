package cli

import (
	"path/filepath"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// statusRow is one line of the worktree report: a ticket joined with the live
// git state of its worktree (#209).
//
// The git half is the embedded taskGitJSON, shared with `adb task list --git`
// (see internal/cli/task_list.go) so the two commands cannot come to spell the
// same five facts differently. The embedded struct is anonymous and by value, so
// its fields are promoted into one flat JSON object.
type statusRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Repo     string `json:"repo,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Worktree string `json:"worktree_path,omitempty"`
	taskGitJSON
}

// buildStatusRows joins each non-archived, WORKTREE-BEARING task with its live
// git state (via the injected gitStatus func, so it is testable without git).
// A recorded worktree that is absent on disk is flagged WorktreeMissing. Rows are
// returned in stable ID order.
//
// Note the filter: this is the WORKTREE listing, so a task without one has no
// row here by definition. `adb task list --git` deliberately does NOT use this —
// it enriches every task via gitStateFor, because there a flag must not change
// which tasks are listed.
func buildStatusRows(tasks []models.Task, gitStatus func(worktreePath string) (integration.WorktreeStatus, error)) []statusRow {
	rows := make([]statusRow, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == models.TaskStatusArchived || task.WorktreePath == "" {
			continue
		}
		state, branch := gitStateFor(task, gitStatus)
		rows = append(rows, statusRow{
			ID:          task.ID,
			Title:       task.Title,
			Status:      string(task.Status),
			Repo:        task.Repo,
			Branch:      branch,
			Worktree:    task.WorktreePath,
			taskGitJSON: state,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
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

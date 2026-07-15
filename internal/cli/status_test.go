package cli

import (
	"fmt"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestBuildStatusRows(t *testing.T) {
	tasks := []models.Task{
		{ID: "TASK-2", Status: models.TaskStatusInProgress, Repo: "r", Branch: "feat/b2", WorktreePath: "/wt/2"},
		{ID: "TASK-1", Status: models.TaskStatusInProgress, Repo: "r", Branch: "feat/b1", WorktreePath: "/wt/1"},
		{ID: "TASK-3", Status: models.TaskStatusArchived, WorktreePath: "/wt/3"}, // skipped: archived
		{ID: "TASK-4", Status: models.TaskStatusBacklog, WorktreePath: ""},       // skipped: no worktree
		{ID: "TASK-5", Status: models.TaskStatusInProgress, WorktreePath: "/wt/5"},
	}

	fake := func(p string) (integration.WorktreeStatus, error) {
		switch p {
		case "/wt/1":
			return integration.WorktreeStatus{Path: p, Exists: true, Branch: "feat/b1", Dirty: true, Ahead: 2, Behind: 1}, nil
		case "/wt/2":
			return integration.WorktreeStatus{Path: p, Exists: true, Branch: "feat/b2"}, nil
		case "/wt/5":
			return integration.WorktreeStatus{Path: p, Exists: false}, nil
		}
		return integration.WorktreeStatus{}, fmt.Errorf("unexpected path %s", p)
	}

	rows := buildStatusRows(tasks, fake)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (archived + no-worktree skipped), got %d: %+v", len(rows), rows)
	}
	// Stable ID order.
	if rows[0].ID != "TASK-1" || rows[1].ID != "TASK-2" || rows[2].ID != "TASK-5" {
		t.Errorf("rows not in ID order: %s, %s, %s", rows[0].ID, rows[1].ID, rows[2].ID)
	}
	// TASK-1: dirty, ahead/behind reflected from git state.
	if !rows[0].Dirty || rows[0].Ahead != 2 || rows[0].Behind != 1 || rows[0].Missing {
		t.Errorf("TASK-1 row = %+v, want dirty ahead2 behind1 not-missing", rows[0])
	}
	// TASK-2: clean.
	if rows[1].Dirty || rows[1].Missing || !rows[1].Exists {
		t.Errorf("TASK-2 row = %+v, want clean/exists", rows[1])
	}
	// TASK-5: worktree recorded but absent → missing.
	if !rows[2].Missing || rows[2].Exists {
		t.Errorf("TASK-5 row = %+v, want missing", rows[2])
	}
}

// TestBuildStatusRows_GitErrorKeepsRow verifies a git-query failure doesn't drop
// the ticket or falsely flag it missing — the backlog branch is retained.
func TestBuildStatusRows_GitErrorKeepsRow(t *testing.T) {
	tasks := []models.Task{
		{ID: "TASK-1", Status: models.TaskStatusInProgress, Repo: "r", Branch: "feat/b1", WorktreePath: "/wt/1"},
	}
	failing := func(string) (integration.WorktreeStatus, error) {
		return integration.WorktreeStatus{}, fmt.Errorf("git exploded")
	}
	rows := buildStatusRows(tasks, failing)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Missing {
		t.Errorf("a git error must not be reported as missing: %+v", rows[0])
	}
	if rows[0].Branch != "feat/b1" {
		t.Errorf("expected backlog branch retained on git error, got %q", rows[0].Branch)
	}
}

package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestTaskManager_Create_NestedCorrelationLayout exercises the ADR-mandated
// layout end-to-end: a spike task with a fully-qualified --repo argument
// must land its ticket and worktree at
//
//	tickets/<platform>/<org>/<repo>/TASK-id-slug
//	work/<platform>/<org>/<repo>/TASK-id-slug
//
// on a `chore/<slug>` branch (ADR mapping: spike -> chore). The leaves on
// both planes must be identical.
func TestTaskManager_Create_NestedCorrelationLayout(t *testing.T) {
	tm, _, _, worktreeCreator, _, tempDir := createTestTaskManager(t)

	opts := CreateTaskOpts{
		Title:    "platonic g0 insurability probe",
		TaskType: models.TaskTypeSpike,
		Repo:     "github.com/awslabs/mcp",
	}

	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	const wantSlug = "platonic-g0-insurability-probe"
	if task.Slug != wantSlug {
		t.Errorf("Slug = %q, want %q", task.Slug, wantSlug)
	}

	wantBranch := "chore/" + wantSlug
	if task.Branch != wantBranch {
		t.Errorf("Branch = %q, want %q (ADR: spike maps to chore)", task.Branch, wantBranch)
	}

	wantTicket := filepath.Join(tempDir, "tickets", "github.com", "awslabs", "mcp", task.ID+"-"+wantSlug)
	if task.TicketPath != wantTicket {
		t.Errorf("TicketPath = %q, want %q", task.TicketPath, wantTicket)
	}

	wantWorktree := filepath.Join(tempDir, "worktrees", "github.com", "awslabs", "mcp", task.ID+"-"+wantSlug)
	if task.WorktreePath != wantWorktree {
		t.Errorf("WorktreePath = %q, want %q", task.WorktreePath, wantWorktree)
	}

	// Both planes must share the same leaf — that's the whole point of
	// the correlation layout (so a glob TASK-id-* resolves on both).
	if filepath.Base(task.TicketPath) != filepath.Base(task.WorktreePath) {
		t.Errorf("ticket leaf %q != worktree leaf %q",
			filepath.Base(task.TicketPath), filepath.Base(task.WorktreePath))
	}

	// And the directory structure on disk should reflect the prefix:
	// the ticket path must contain "/github.com/awslabs/mcp/" between
	// tickets/ and the leaf.
	rel, err := filepath.Rel(filepath.Join(tempDir, "tickets"), task.TicketPath)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if !strings.HasPrefix(filepath.ToSlash(rel), "github.com/awslabs/mcp/") {
		t.Errorf("ticket sub-path %q does not start with platform/org/repo prefix", rel)
	}

	// The mock worktree creator records what the manager asked for, so
	// we can verify the exact branch+path that would have been passed to
	// `git worktree add -b <branch> <path>`.
	if got, want := worktreeCreator.worktrees[task.ID], wantWorktree; got != want {
		t.Errorf("MockWorktreeCreator stored worktree path %q, want %q", got, want)
	}
}

// TestTaskManager_Create_BugMapsToFix exercises the bug -> fix half of the
// ADR's type-to-Conventional-prefix mapping. Spike was the chore mapping;
// this one ensures the other non-identity mapping is also correctly wired
// from CreateTask through to the branch the worktree gets.
func TestTaskManager_Create_BugMapsToFix(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "stickler optional field",
		TaskType: models.TaskTypeBug,
		Repo:     "github.com/example/repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.Branch != "fix/stickler-optional-field" {
		t.Errorf("Branch = %q, want %q (ADR: bug maps to fix)", task.Branch, "fix/stickler-optional-field")
	}
}

// TestTaskManager_Create_RepoLessParkedUnderLocal asserts repo-less tasks
// land under tickets/_local/TASK-id-slug per the ADR's repo-less rule.
func TestTaskManager_Create_RepoLessParkedUnderLocal(t *testing.T) {
	tm, _, _, _, _, tempDir := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "wiki cleanup pass",
		TaskType: models.TaskTypeFeat,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	wantTicket := filepath.Join(tempDir, "tickets", "_local", task.ID+"-wiki-cleanup-pass")
	if task.TicketPath != wantTicket {
		t.Errorf("TicketPath = %q, want %q", task.TicketPath, wantTicket)
	}
	// Repo-less tasks have no worktree and no branch.
	if task.WorktreePath != "" {
		t.Errorf("WorktreePath = %q, want empty for repo-less task", task.WorktreePath)
	}
	if task.Branch != "" {
		t.Errorf("Branch = %q, want empty for repo-less task", task.Branch)
	}
}

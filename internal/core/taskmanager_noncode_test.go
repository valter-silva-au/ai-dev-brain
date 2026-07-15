package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestCreate_WorkType_NoWorktreeNoBranch proves the non-code `work` type gets
// NO worktree and NO branch even when a repo is supplied (D10) — it is an
// artifact/graph deliverable, not code. The ticket is still created and the
// task lands in the backlog.
func TestCreate_WorkType_NoWorktreeNoBranch(t *testing.T) {
	tm, backlogStore, _, worktreeCreator, _, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "positioning doc",
		TaskType: models.TaskTypeWork,
		Repo:     "github.com/acme/widgets", // supplied, but must be ignored for worktree
	})
	if err != nil {
		t.Fatalf("Create work task: %v", err)
	}

	if len(worktreeCreator.worktrees) != 0 {
		t.Errorf("work task created a worktree: %v", worktreeCreator.worktrees)
	}
	if task.WorktreePath != "" {
		t.Errorf("work task has WorktreePath=%q, want empty", task.WorktreePath)
	}
	if task.Branch != "" {
		t.Errorf("work task has Branch=%q, want empty", task.Branch)
	}
	// Still a real ticket in the backlog.
	if _, err := backlogStore.GetTask(task.ID); err != nil {
		t.Errorf("work task not in backlog: %v", err)
	}
	if task.Status != models.TaskStatusBacklog {
		t.Errorf("status=%q want backlog", task.Status)
	}
}

// TestCreate_PrototypeType_GetsWorktreeAndChoreBranch proves `prototype` is
// code-shaped: with a repo it gets a worktree and a chore/<slug> branch (like
// spike).
func TestCreate_PrototypeType_GetsWorktreeAndChoreBranch(t *testing.T) {
	tm, _, _, worktreeCreator, _, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "spike the ranking idea",
		TaskType: models.TaskTypePrototype,
		Repo:     "github.com/acme/widgets",
	})
	if err != nil {
		t.Fatalf("Create prototype task: %v", err)
	}

	if _, ok := worktreeCreator.worktrees[task.ID]; !ok {
		t.Errorf("prototype task did not create a worktree")
	}
	if task.Branch != "chore/spike-the-ranking-idea" {
		t.Errorf("prototype branch=%q want chore/spike-the-ranking-idea", task.Branch)
	}
}

// TestCreate_WorkType_StageAgnostic confirms the type carries no stage coupling:
// a work task created with an initiative association is unaffected by / does not
// affect stage — it simply records the metadata (stage discipline is the gate's
// job, not the type's).
func TestCreate_WorkType_RepoLessAlsoNoWorktree(t *testing.T) {
	tm, _, _, worktreeCreator, _, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "market research",
		TaskType: models.TaskTypeWork, // no repo at all
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(worktreeCreator.worktrees) != 0 || task.Branch != "" {
		t.Errorf("repo-less work task should have no worktree/branch: worktrees=%v branch=%q", worktreeCreator.worktrees, task.Branch)
	}
}

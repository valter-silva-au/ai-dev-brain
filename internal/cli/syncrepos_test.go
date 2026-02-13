package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/internal/integration"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestSyncReposCommand_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "sync-repos" {
			found = true
			break
		}
	}
	if !found {
		t.Error("sync-repos command not registered on root")
	}
}

func TestSyncReposCommand_NilRepoSyncMgr(t *testing.T) {
	origMgr := RepoSyncMgr
	defer func() { RepoSyncMgr = origMgr }()
	RepoSyncMgr = nil

	err := syncReposCmd.RunE(syncReposCmd, nil)
	if err == nil {
		t.Fatal("expected error when RepoSyncMgr is nil")
	}
	if !strings.Contains(err.Error(), "repo sync manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncReposCommand_NoRepos(t *testing.T) {
	origMgr := RepoSyncMgr
	origTaskMgr := TaskMgr
	defer func() {
		RepoSyncMgr = origMgr
		TaskMgr = origTaskMgr
	}()

	// Use a temp dir with no repos/ subdirectory.
	tmpDir := t.TempDir()
	RepoSyncMgr = integration.NewRepoSyncManager(tmpDir)
	TaskMgr = nil // buildProtectedBranches returns empty map for nil TaskMgr

	err := syncReposCmd.RunE(syncReposCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncReposCommand_BuildProtectedBranchesError(t *testing.T) {
	origMgr := RepoSyncMgr
	origTaskMgr := TaskMgr
	defer func() {
		RepoSyncMgr = origMgr
		TaskMgr = origTaskMgr
	}()

	tmpDir := t.TempDir()
	RepoSyncMgr = integration.NewRepoSyncManager(tmpDir)
	TaskMgr = &syncReposMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	err := syncReposCmd.RunE(syncReposCmd, nil)
	if err == nil {
		t.Fatal("expected error from buildProtectedBranches")
	}
	if !strings.Contains(err.Error(), "loading backlog") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildProtectedBranches_NilTaskMgr(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	protected, err := buildProtectedBranches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(protected) != 0 {
		t.Errorf("expected empty map, got %v", protected)
	}
}

func TestBuildProtectedBranches_GetAllTasksError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &syncReposMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	_, err := buildProtectedBranches()
	if err == nil {
		t.Fatal("expected error from GetAllTasks")
	}
}

func TestBuildProtectedBranches_FiltersCorrectly(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &syncReposMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Repo: "github.com/org/repo", Branch: "feat/auth", Status: models.StatusInProgress},
				{ID: "TASK-00002", Repo: "github.com/org/repo", Branch: "bug/fix", Status: models.StatusDone},
				{ID: "TASK-00003", Repo: "github.com/org/repo", Branch: "spike/cache", Status: models.StatusArchived},
				{ID: "TASK-00004", Repo: "github.com/org/other", Branch: "feat/ui", Status: models.StatusBacklog},
				{ID: "TASK-00005", Repo: "", Branch: "feat/no-repo", Status: models.StatusInProgress},
				{ID: "TASK-00006", Repo: "github.com/org/repo", Branch: "", Status: models.StatusInProgress},
				{ID: "TASK-00007", Repo: "github.com/org/repo", Branch: "refactor/db", Status: models.StatusBlocked},
			}, nil
		},
	}

	protected, err := buildProtectedBranches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TASK-00001 (in_progress) should be protected.
	// TASK-00002 (done) should NOT be protected.
	// TASK-00003 (archived) should NOT be protected.
	// TASK-00004 (backlog, different repo) should be protected.
	// TASK-00005 (no repo) should be skipped.
	// TASK-00006 (no branch) should be skipped.
	// TASK-00007 (blocked) should be protected.

	// org/repo should have feat/auth and refactor/db.
	repoKey := "github.com/org/repo"
	if protected[repoKey] == nil {
		t.Fatal("expected protected branches for github.com/org/repo")
	}
	if !protected[repoKey]["feat/auth"] {
		t.Error("feat/auth should be protected")
	}
	if !protected[repoKey]["refactor/db"] {
		t.Error("refactor/db should be protected")
	}
	if protected[repoKey]["bug/fix"] {
		t.Error("bug/fix (done) should NOT be protected")
	}
	if protected[repoKey]["spike/cache"] {
		t.Error("spike/cache (archived) should NOT be protected")
	}

	// org/other should have feat/ui.
	otherKey := "github.com/org/other"
	if protected[otherKey] == nil {
		t.Fatal("expected protected branches for github.com/org/other")
	}
	if !protected[otherKey]["feat/ui"] {
		t.Error("feat/ui should be protected")
	}
}

func TestBuildProtectedBranches_EmptyTasks(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &syncReposMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{}, nil
		},
	}

	protected, err := buildProtectedBranches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(protected) != 0 {
		t.Errorf("expected empty map for empty tasks, got %v", protected)
	}
}

// syncReposMock implements core.TaskManager for testing buildProtectedBranches.
type syncReposMock struct {
	getAllTasksFn func() ([]*models.Task, error)
}

func (m *syncReposMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) GetAllTasks() ([]*models.Task, error) {
	if m.getAllTasksFn != nil {
		return m.getAllTasksFn()
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *syncReposMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *syncReposMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *syncReposMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *syncReposMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

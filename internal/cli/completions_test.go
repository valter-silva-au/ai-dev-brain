package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// --- completeTaskIDs tests ---

func TestCompleteTaskIDs_NilTaskMgr(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	fn := completeTaskIDs()
	ids, directive := fn(&cobra.Command{}, nil, "")
	if ids != nil {
		t.Errorf("expected nil ids, got %v", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestCompleteTaskIDs_GetAllTasksError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &completionsMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	fn := completeTaskIDs()
	ids, directive := fn(&cobra.Command{}, nil, "")
	if ids != nil {
		t.Errorf("expected nil ids on error, got %v", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestCompleteTaskIDs_ReturnsMatchingTasks(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &completionsMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Type: models.TaskTypeFeat, Branch: "feat/auth", Status: models.StatusInProgress},
				{ID: "TASK-00002", Type: models.TaskTypeBug, Branch: "bug/crash", Status: models.StatusBacklog},
				{ID: "TASK-00003", Type: models.TaskTypeSpike, Branch: "spike/cache", Status: models.StatusArchived},
			}, nil
		},
	}

	fn := completeTaskIDs()

	// No filter, empty prefix -> all tasks.
	ids, _ := fn(&cobra.Command{}, nil, "")
	if len(ids) != 3 {
		t.Errorf("expected 3 ids, got %d", len(ids))
	}

	// Filter by prefix.
	ids, _ = fn(&cobra.Command{}, nil, "TASK-0000")
	if len(ids) != 3 {
		t.Errorf("expected 3 ids matching TASK-0000, got %d", len(ids))
	}

	ids, _ = fn(&cobra.Command{}, nil, "TASK-00002")
	if len(ids) != 1 {
		t.Errorf("expected 1 id matching TASK-00002, got %d", len(ids))
	}

	ids, _ = fn(&cobra.Command{}, nil, "NONEXISTENT")
	if len(ids) != 0 {
		t.Errorf("expected 0 ids for NONEXISTENT prefix, got %d", len(ids))
	}
}

func TestCompleteTaskIDs_WithExcludeStatuses(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &completionsMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Type: models.TaskTypeFeat, Branch: "auth", Status: models.StatusInProgress},
				{ID: "TASK-00002", Type: models.TaskTypeBug, Branch: "crash", Status: models.StatusArchived},
				{ID: "TASK-00003", Type: models.TaskTypeFeat, Branch: "ui", Status: models.StatusDone},
			}, nil
		},
	}

	// Exclude archived.
	fn := completeTaskIDs(models.StatusArchived)
	ids, _ := fn(&cobra.Command{}, nil, "")
	if len(ids) != 2 {
		t.Errorf("expected 2 ids after excluding archived, got %d", len(ids))
	}

	// Exclude archived and done.
	fn = completeTaskIDs(models.StatusArchived, models.StatusDone)
	ids, _ = fn(&cobra.Command{}, nil, "")
	if len(ids) != 1 {
		t.Errorf("expected 1 id after excluding archived+done, got %d", len(ids))
	}
}

// --- completeRepoPaths tests ---

func TestCompleteRepoPaths_EmptyBasePath(t *testing.T) {
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()
	BasePath = ""

	repos, directive := completeRepoPaths(&cobra.Command{}, nil, "")
	if repos != nil {
		t.Errorf("expected nil repos, got %v", repos)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestCompleteRepoPaths_WithRepos(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not supported on Windows: filepath.Rel uses backslashes, breaking forward-slash prefix matching")
	}
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()

	tmpDir := t.TempDir()
	BasePath = tmpDir

	// Create repos/github.com/org/repo1 and repos/github.com/org/repo2.
	repo1 := filepath.Join(tmpDir, "repos", "github.com", "org", "repo1")
	repo2 := filepath.Join(tmpDir, "repos", "github.com", "org", "repo2")
	if err := os.MkdirAll(repo1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo2, 0o755); err != nil {
		t.Fatal(err)
	}

	repos, directive := completeRepoPaths(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(repos), repos)
	}

	// Filter by prefix.
	repos, _ = completeRepoPaths(&cobra.Command{}, nil, "github.com/org/repo1")
	if len(repos) != 1 {
		t.Errorf("expected 1 repo matching prefix, got %d", len(repos))
	}

	repos, _ = completeRepoPaths(&cobra.Command{}, nil, "nonexistent")
	if len(repos) != 0 {
		t.Errorf("expected 0 repos for nonexistent prefix, got %d", len(repos))
	}
}

func TestCompleteRepoPaths_FileNotDir(t *testing.T) {
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()

	tmpDir := t.TempDir()
	BasePath = tmpDir

	// Create a file instead of a directory at the repo level.
	filePath := filepath.Join(tmpDir, "repos", "github.com", "org")
	if err := os.MkdirAll(filePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filePath, "not-a-dir"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos, _ := completeRepoPaths(&cobra.Command{}, nil, "")
	// The file "not-a-dir" should be skipped since it's not a directory.
	if len(repos) != 0 {
		t.Errorf("expected 0 repos (file should be skipped), got %d: %v", len(repos), repos)
	}
}

// --- completePriorities tests ---

func TestCompletePriorities(t *testing.T) {
	priorities, directive := completePriorities(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
	if len(priorities) != 4 {
		t.Fatalf("expected 4 priorities, got %d", len(priorities))
	}
	expected := []string{"P0", "P1", "P2", "P3"}
	for i, p := range priorities {
		if p[:2] != expected[i] {
			t.Errorf("priority[%d] = %q, want prefix %q", i, p, expected[i])
		}
	}
}

func TestCompleteRepoPaths_ReposDirDoesNotExist(t *testing.T) {
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()

	// Set BasePath to a temp dir without creating repos/ subdirectory.
	tmpDir := t.TempDir()
	BasePath = tmpDir

	repos, directive := completeRepoPaths(&cobra.Command{}, nil, "")
	// When repos/ doesn't exist, Glob returns no matches.
	if len(repos) != 0 {
		t.Errorf("expected 0 repos when repos dir doesn't exist, got %d", len(repos))
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestCompleteRepoPaths_GlobError(t *testing.T) {
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()

	// filepath.Glob returns an error for malformed patterns (unmatched '[').
	// By setting BasePath to a string containing '[', the constructed pattern
	// becomes invalid.
	BasePath = "/tmp/bad[path"

	repos, directive := completeRepoPaths(&cobra.Command{}, nil, "")
	if repos != nil {
		t.Errorf("expected nil repos on glob error, got %v", repos)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestCompleteRepoPaths_RelError(t *testing.T) {
	origBasePath := BasePath
	defer func() { BasePath = origBasePath }()

	tmpDir := t.TempDir()
	BasePath = tmpDir

	// Create a repo directory structure.
	repoPath := filepath.Join(tmpDir, "repos", "github.com", "org", "repo1")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Test passes normally.
	repos, directive := completeRepoPaths(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}

	// The line "if err != nil { continue }" after filepath.Rel at line 67 is
	// hard to trigger because filepath.Rel only fails if one path is not absolute
	// or when hitting internal errors. In normal operation with Glob results,
	// filepath.Rel should always succeed. This is effectively unreachable without
	// mocking or extreme filesystem manipulation.
}

// --- completeStatuses tests ---

func TestCompleteStatuses(t *testing.T) {
	statuses, directive := completeStatuses(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
	if len(statuses) != 6 {
		t.Fatalf("expected 6 statuses, got %d", len(statuses))
	}
	expectedPrefixes := []string{"backlog", "in_progress", "blocked", "review", "done", "archived"}
	for i, s := range statuses {
		if s[:len(expectedPrefixes[i])] != expectedPrefixes[i] {
			t.Errorf("status[%d] = %q, want prefix %q", i, s, expectedPrefixes[i])
		}
	}
}

// --- completionsMock ---

type completionsMock struct {
	getAllTasksFn    func() ([]*models.Task, error)
	getTasksByStatFn func(status models.TaskStatus) ([]*models.Task, error)
}

func (m *completionsMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	if m.getTasksByStatFn != nil {
		return m.getTasksByStatFn(status)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) GetAllTasks() ([]*models.Task, error) {
	if m.getAllTasksFn != nil {
		return m.getAllTasksFn()
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *completionsMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *completionsMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *completionsMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *completionsMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

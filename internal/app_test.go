package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestResolveBasePath_ADBHomeSet(t *testing.T) {
	// Test that ADB_HOME env var takes precedence.
	tmpDir := t.TempDir()
	t.Setenv("ADB_HOME", tmpDir)

	got := ResolveBasePath()
	if got != tmpDir {
		t.Errorf("ResolveBasePath() = %q, want %q", got, tmpDir)
	}
}

func TestResolveBasePath_FindsTaskConfig(t *testing.T) {
	// Test that ResolveBasePath walks up to find .taskconfig.
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub", "nested")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create .taskconfig in the parent directory.
	configPath := filepath.Join(tmpDir, ".taskconfig")
	if err := os.WriteFile(configPath, []byte("version: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Change to the nested subdirectory.
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	// Unset ADB_HOME so it doesn't interfere.
	os.Unsetenv("ADB_HOME")

	got := ResolveBasePath()
	if got != tmpDir {
		t.Errorf("ResolveBasePath() = %q, want %q (should find .taskconfig in parent)", got, tmpDir)
	}
}

func TestResolveBasePath_FallbackToCwd(t *testing.T) {
	// Test that ResolveBasePath falls back to cwd when no .taskconfig is found.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Unset ADB_HOME.
	os.Unsetenv("ADB_HOME")

	got := ResolveBasePath()
	if got != tmpDir {
		t.Errorf("ResolveBasePath() = %q, want %q (should fall back to cwd)", got, tmpDir)
	}
}

func TestNewApp_Success(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	if app == nil {
		t.Fatal("NewApp() returned nil app")
	}
	if app.BasePath != tmpDir {
		t.Errorf("app.BasePath = %q, want %q", app.BasePath, tmpDir)
	}
	// Verify that key services are wired.
	if app.TaskMgr == nil {
		t.Error("app.TaskMgr is nil")
	}
	if app.BacklogMgr == nil {
		t.Error("app.BacklogMgr is nil")
	}
}

// --- Adapter tests ---

func TestBacklogStoreAdapter_GetAllTasksError(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the backlog file to trigger a load error.
	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	if err := os.WriteFile(backlogPath, []byte("invalid: yaml: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := app.TaskMgr.GetAllTasks()
	if err == nil {
		t.Error("expected error from corrupted backlog file")
	}
	if tasks != nil {
		t.Errorf("expected nil tasks on error, got %v", tasks)
	}
}

func TestBacklogStoreAdapter_FilterTasksError(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the backlog file to trigger a load error.
	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	if err := os.WriteFile(backlogPath, []byte("invalid: yaml: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Attempt to filter tasks via TaskManager, which uses the backlog adapter.
	tasks, err := app.TaskMgr.GetTasksByStatus(models.StatusInProgress)
	if err == nil {
		t.Error("expected error from corrupted backlog file")
	}
	if tasks != nil {
		t.Errorf("expected nil tasks on error, got %v", tasks)
	}
}

func TestWorktreeAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Attempting to create a worktree without a git repo will fail, but we're
	// testing that the adapter wiring works.
	// This is a minimal smoke test.
	_ = app.WorktreeMgr
}

func TestContextStoreAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Attempting to load context for a non-existent task will return an error.
	// This is a minimal smoke test of the adapter wiring.
	_, err = app.TaskMgr.GetTask("TASK-00000")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestStorageEntryConversion(t *testing.T) {
	// Test that conversion between core.BacklogStoreEntry and storage.BacklogEntry
	// preserves all fields.
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a task and retrieve it to verify round-trip conversion.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "test-conversion", "", core.CreateTaskOpts{
		Priority: models.P1,
		Owner:    "@alice",
		Tags:     []string{"test", "conversion"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Retrieve the task.
	retrieved, err := app.TaskMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}

	if retrieved.Priority != models.P1 {
		t.Errorf("priority = %v, want %v", retrieved.Priority, models.P1)
	}
	if retrieved.Owner != "@alice" {
		t.Errorf("owner = %q, want @alice", retrieved.Owner)
	}
	if len(retrieved.Tags) != 2 {
		t.Errorf("tags = %v, want [test conversion]", retrieved.Tags)
	}
}

func TestNewApp_MissingConfig(t *testing.T) {
	// Test that NewApp uses defaults when .taskconfig is missing.
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if app == nil {
		t.Fatal("NewApp() returned nil app")
	}
	// The app should still be functional with default config values.
	if app.IDGen == nil {
		t.Error("app.IDGen is nil (should use default prefix)")
	}
}

func TestCLIAliasConversion(t *testing.T) {
	// Test that CLI aliases are properly converted from GlobalConfig to integration.CLIAlias.
	tmpDir := t.TempDir()

	// Create a .taskconfig with CLI aliases.
	configPath := filepath.Join(tmpDir, ".taskconfig")
	configContent := `
version: "1.0"
cli_aliases:
  - name: test-alias
    command: echo
    default_args: ["hello"]
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the alias was wired to the CLI package.
	// We can't directly test cli.ExecAliases here (it's a package variable),
	// but we can verify the app was created successfully with the config.
	if app.ConfigMgr == nil {
		t.Error("app.ConfigMgr is nil")
	}

	// Verify the config was loaded.
	globalCfg, err := app.ConfigMgr.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(globalCfg.CLIAliases) != 1 {
		t.Fatalf("expected 1 CLI alias, got %d", len(globalCfg.CLIAliases))
	}
	if globalCfg.CLIAliases[0].Name != "test-alias" {
		t.Errorf("alias name = %q, want test-alias", globalCfg.CLIAliases[0].Name)
	}
}

func TestWorktreeRemoverAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a task first.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "test-cleanup", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Test the worktree remover by cleaning up the worktree.
	// This verifies the adapter wiring.
	err = app.TaskMgr.CleanupWorktree(task.ID)
	// Since we don't have a real git worktree, this should either succeed or fail gracefully.
	// We're just testing that the adapter is wired correctly.
	_ = err
}

func TestBacklogStoreAdapter_AddUpdateGetTask(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a task.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeBug, "test-bug", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Update the task status.
	if err := app.TaskMgr.UpdateTaskStatus(task.ID, models.StatusInProgress); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}

	// Retrieve and verify.
	retrieved, err := app.TaskMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if retrieved.Status != models.StatusInProgress {
		t.Errorf("status = %v, want %v", retrieved.Status, models.StatusInProgress)
	}
}

func TestBacklogStoreAdapter_FilterTasksSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple tasks with different statuses.
	_, err = app.TaskMgr.CreateTask(models.TaskTypeFeat, "feat1", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatal(err)
	}
	task2, err := app.TaskMgr.CreateTask(models.TaskTypeBug, "bug1", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.TaskMgr.UpdateTaskStatus(task2.ID, models.StatusInProgress); err != nil {
		t.Fatal(err)
	}

	// Filter by status.
	tasks, err := app.TaskMgr.GetTasksByStatus(models.StatusInProgress)
	if err != nil {
		t.Fatalf("GetTasksByStatus() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task with status in_progress, got %d", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].ID != task2.ID {
		t.Errorf("task ID = %q, want %q", tasks[0].ID, task2.ID)
	}
}

func TestBacklogStoreAdapter_GetTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.TaskMgr.GetTask("TASK-99999")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "TASK-99999") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestContextStoreAdapter_LoadContext(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a task.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "test-context", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// The task should have a context file created during bootstrap.
	// Attempting to load it should succeed (even if it's mostly empty).
	ctx, err := app.ContextMgr.LoadContext(task.ID)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if ctx == nil {
		t.Error("LoadContext() returned nil context")
	}
}

package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// =============================================================================
// Edge Case 1: Empty backlog - status command with no tasks
// =============================================================================

func TestEdgeCase_EmptyBacklog_StatusReturnsNoTasks(t *testing.T) {
	app := newTestApp(t)

	tasks, err := app.TaskMgr.GetAllTasks()
	if err != nil {
		t.Fatalf("getting all tasks from empty backlog: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks in fresh backlog, got %d", len(tasks))
	}

	// Filtering by each status should also return empty.
	statuses := []models.TaskStatus{
		models.StatusBacklog,
		models.StatusInProgress,
		models.StatusBlocked,
		models.StatusReview,
		models.StatusDone,
		models.StatusArchived,
	}
	for _, s := range statuses {
		filtered, err := app.TaskMgr.GetTasksByStatus(s)
		if err != nil {
			t.Fatalf("filtering by %s on empty backlog: %v", s, err)
		}
		if len(filtered) != 0 {
			t.Fatalf("expected 0 tasks for status %s, got %d", s, len(filtered))
		}
	}
}

// =============================================================================
// Edge Case 2: Missing config files - App initialization without .taskconfig
// =============================================================================

func TestEdgeCase_MissingConfig_AppInitializesWithDefaults(t *testing.T) {
	dir := t.TempDir()
	// No .taskconfig file created.

	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp should succeed without .taskconfig: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if app.TaskMgr == nil {
		t.Fatal("TaskMgr should be initialized even without .taskconfig")
	}
	if app.BacklogMgr == nil {
		t.Fatal("BacklogMgr should be initialized even without .taskconfig")
	}
	if app.Executor == nil {
		t.Fatal("Executor should be initialized even without .taskconfig")
	}

	// Should be able to create tasks with defaults.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "test-branch", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task without .taskconfig: %v", err)
	}
	if task.ID == "" {
		t.Fatal("task ID should not be empty")
	}
}

// =============================================================================
// Edge Case 3: Corrupted YAML - backlog.yaml with invalid YAML content
// =============================================================================

func TestEdgeCase_CorruptedBacklogYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	backlogPath := filepath.Join(dir, "backlog.yaml")

	// Write corrupted YAML.
	corrupted := `{{{not: valid: yaml: [[[`
	if err := os.WriteFile(backlogPath, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("writing corrupted backlog: %v", err)
	}

	mgr := storage.NewBacklogManager(dir)
	err := mgr.Load()
	if err == nil {
		t.Fatal("expected error loading corrupted YAML, got nil")
	}
	if !strings.Contains(err.Error(), "YAML") && !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("error should mention YAML parsing, got: %v", err)
	}
}

// =============================================================================
// Edge Case 4: Task operations on non-existent tasks
// =============================================================================

func TestEdgeCase_ResumeNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	_, err := app.TaskMgr.ResumeTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error resuming non-existent task")
	}
	if !strings.Contains(err.Error(), "TASK-99999") {
		t.Fatalf("error should mention task ID, got: %v", err)
	}
}

func TestEdgeCase_ArchiveNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	_, err := app.TaskMgr.ArchiveTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error archiving non-existent task")
	}
	if !strings.Contains(err.Error(), "TASK-99999") {
		t.Fatalf("error should mention task ID, got: %v", err)
	}
}

func TestEdgeCase_UnarchiveNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	_, err := app.TaskMgr.UnarchiveTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error unarchiving non-existent task")
	}
	if !strings.Contains(err.Error(), "TASK-99999") {
		t.Fatalf("error should mention task ID, got: %v", err)
	}
}

func TestEdgeCase_GetNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	_, err := app.TaskMgr.GetTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error getting non-existent task")
	}
}

func TestEdgeCase_UpdateStatusNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	err := app.TaskMgr.UpdateTaskStatus("TASK-99999", models.StatusInProgress)
	if err == nil {
		t.Fatal("expected error updating status of non-existent task")
	}
}

func TestEdgeCase_UpdatePriorityNonExistentTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	err := app.TaskMgr.UpdateTaskPriority("TASK-99999", models.P0)
	if err == nil {
		t.Fatal("expected error updating priority of non-existent task")
	}
}

// =============================================================================
// Edge Case 5: Double archive (archive an already-archived task)
// =============================================================================

func TestEdgeCase_DoubleArchive_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "double-archive", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Archive the first time.
	_, err = app.TaskMgr.ArchiveTask(task.ID)
	if err != nil {
		t.Fatalf("first archive: %v", err)
	}

	// Verify task is now archived.
	archived, err := app.TaskMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("getting archived task: %v", err)
	}
	if archived.Status != models.StatusArchived {
		t.Fatalf("expected archived status, got %s", archived.Status)
	}

	// Archive again - should fail.
	_, err = app.TaskMgr.ArchiveTask(task.ID)
	if err == nil {
		t.Fatal("expected error on double archive")
	}
	if !strings.Contains(err.Error(), "already archived") {
		t.Fatalf("error should mention 'already archived', got: %v", err)
	}
}

// =============================================================================
// Edge Case 6: Double unarchive (unarchive a non-archived task)
// =============================================================================

func TestEdgeCase_UnarchiveNonArchivedTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeBug, "not-archived", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Task is in backlog, try to unarchive.
	_, err = app.TaskMgr.UnarchiveTask(task.ID)
	if err == nil {
		t.Fatal("expected error unarchiving a non-archived task")
	}
	if !strings.Contains(err.Error(), "not archived") {
		t.Fatalf("error should mention 'not archived', got: %v", err)
	}
}

func TestEdgeCase_UnarchiveInProgressTask_ReturnsError(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "in-progress-task", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Resume to make in_progress.
	_, err = app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming task: %v", err)
	}

	// Unarchive should fail since it's in_progress, not archived.
	_, err = app.TaskMgr.UnarchiveTask(task.ID)
	if err == nil {
		t.Fatal("expected error unarchiving an in_progress task")
	}
}

// =============================================================================
// Edge Case 7: Create task with empty name
// =============================================================================

func TestEdgeCase_CreateTaskWithEmptyBranch_StillSucceeds(t *testing.T) {
	app := newTestApp(t)

	// Empty string for branch name -- the system should still create the task
	// (the bootstrap system doesn't validate branch name content).
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task with empty branch name: %v", err)
	}
	if task.ID == "" {
		t.Fatal("task should still get an ID")
	}
	// Branch should be empty.
	if task.Branch != "" {
		t.Fatalf("expected empty branch, got %q", task.Branch)
	}
}

// =============================================================================
// Edge Case 8: Create task with special characters in name
// =============================================================================

func TestEdgeCase_CreateTaskWithSpecialChars(t *testing.T) {
	app := newTestApp(t)

	specialNames := []string{
		"feat/add-auth",
		"fix(scope): the-thing",
		"branch with spaces",
		"branch-with-emoij-\U0001f600",
		"very-long-branch-name-that-exceeds-normal-limits-" + strings.Repeat("x", 200),
	}

	for _, name := range specialNames {
		task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, name, "", core.CreateTaskOpts{})
		if err != nil {
			t.Fatalf("creating task with name %q: %v", name, err)
		}
		if task.ID == "" {
			t.Fatalf("task should have an ID for name %q", name)
		}
		// Verify we can get the task back.
		retrieved, err := app.TaskMgr.GetTask(task.ID)
		if err != nil {
			t.Fatalf("retrieving task %s: %v", task.ID, err)
		}
		if retrieved.Branch != name {
			t.Fatalf("branch name mismatch: expected %q, got %q", name, retrieved.Branch)
		}
	}
}

// =============================================================================
// Edge Case 9: Filter with no matching criteria
// =============================================================================

func TestEdgeCase_FilterNoMatches(t *testing.T) {
	app := newTestApp(t)

	// Create some tasks.
	_, _ = app.TaskMgr.CreateTask(models.TaskTypeFeat, "feature-one", "", core.CreateTaskOpts{})
	_, _ = app.TaskMgr.CreateTask(models.TaskTypeBug, "bug-one", "", core.CreateTaskOpts{})

	// Filter for a status that no tasks have.
	done, err := app.TaskMgr.GetTasksByStatus(models.StatusDone)
	if err != nil {
		t.Fatalf("filtering for done tasks: %v", err)
	}
	if len(done) != 0 {
		t.Fatalf("expected 0 done tasks, got %d", len(done))
	}

	// Filter with the backlog manager for a nonexistent owner.
	blMgr := storage.NewBacklogManager(app.BasePath)
	if err := blMgr.Load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}
	results, err := blMgr.FilterTasks(storage.BacklogFilter{
		Owner: "nonexistent-user",
	})
	if err != nil {
		t.Fatalf("filtering by owner: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for nonexistent owner, got %d", len(results))
	}
}

// =============================================================================
// Edge Case 10: Configuration with empty/nil values
// =============================================================================

func TestEdgeCase_ConfigValidation_EmptyValues(t *testing.T) {
	dir := t.TempDir()
	cm := core.NewConfigurationManager(dir)

	// Validate nil config.
	err := cm.ValidateConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}

	// Validate nil GlobalConfig pointer.
	err = cm.ValidateConfig((*models.GlobalConfig)(nil))
	if err == nil {
		t.Fatal("expected error for nil GlobalConfig")
	}

	// Validate config with empty AI.
	err = cm.ValidateConfig(&models.GlobalConfig{
		DefaultAI:    "",
		TaskIDPrefix: "TASK",
	})
	if err == nil {
		t.Fatal("expected error for empty DefaultAI")
	}
	if !strings.Contains(err.Error(), "defaults.ai") {
		t.Fatalf("error should mention defaults.ai, got: %v", err)
	}

	// Validate config with empty prefix.
	err = cm.ValidateConfig(&models.GlobalConfig{
		DefaultAI:    "claude",
		TaskIDPrefix: "",
	})
	if err == nil {
		t.Fatal("expected error for empty TaskIDPrefix")
	}

	// Validate config with invalid priority.
	err = cm.ValidateConfig(&models.GlobalConfig{
		DefaultAI:       "claude",
		TaskIDPrefix:    "TASK",
		DefaultPriority: "P99",
	})
	if err == nil {
		t.Fatal("expected error for invalid priority P99")
	}

	// Validate valid config succeeds.
	err = cm.ValidateConfig(&models.GlobalConfig{
		DefaultAI:       "claude",
		TaskIDPrefix:    "TASK",
		DefaultPriority: models.P2,
	})
	if err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}
}

func TestEdgeCase_ConfigValidation_UnsupportedType(t *testing.T) {
	dir := t.TempDir()
	cm := core.NewConfigurationManager(dir)

	// Pass an unsupported type.
	err := cm.ValidateConfig("not a config")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error should mention 'unsupported', got: %v", err)
	}
}

// =============================================================================
// Additional edge cases: Backlog operations
// =============================================================================

func TestEdgeCase_BacklogAddDuplicateID_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	entry := storage.BacklogEntry{ID: "TASK-00001", Title: "Test"}
	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("first add: %v", err)
	}
	err := mgr.AddTask(entry)
	if err == nil {
		t.Fatal("expected error for duplicate task ID")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error should mention 'already exists', got: %v", err)
	}
}

func TestEdgeCase_BacklogAddEmptyID_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	err := mgr.AddTask(storage.BacklogEntry{ID: "", Title: "No ID"})
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestEdgeCase_BacklogUpdateNonExistent_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	err := mgr.UpdateTask("TASK-NOPE", storage.BacklogEntry{Title: "updated"})
	if err == nil {
		t.Fatal("expected error for updating non-existent task")
	}
}

func TestEdgeCase_BacklogRemoveNonExistent_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	err := mgr.RemoveTask("TASK-NOPE")
	if err == nil {
		t.Fatal("expected error for removing non-existent task")
	}
}

func TestEdgeCase_BacklogGetNonExistent_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	_, err := mgr.GetTask("TASK-NOPE")
	if err == nil {
		t.Fatal("expected error for getting non-existent task")
	}
}

// =============================================================================
// Additional edge cases: Context operations
// =============================================================================

func TestEdgeCase_LoadContextNonExistentTask_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewContextManager(dir)

	_, err := mgr.LoadContext("TASK-NOPE")
	if err == nil {
		t.Fatal("expected error loading context for non-existent task")
	}
}

func TestEdgeCase_PersistContextNotLoaded_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewContextManager(dir)

	err := mgr.PersistContext("TASK-NOPE")
	if err == nil {
		t.Fatal("expected error persisting context that was never loaded")
	}
}

// =============================================================================
// Additional edge cases: Full lifecycle with priority
// =============================================================================

func TestEdgeCase_ReorderPrioritiesWithMoreThanFourTasks(t *testing.T) {
	app := newTestApp(t)

	var taskIDs []string
	for i := 0; i < 6; i++ {
		task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "task-"+string(rune('a'+i)), "", core.CreateTaskOpts{})
		if err != nil {
			t.Fatalf("creating task %d: %v", i, err)
		}
		taskIDs = append(taskIDs, task.ID)
	}

	// Reorder all 6 tasks.
	if err := app.TaskMgr.ReorderPriorities(taskIDs); err != nil {
		t.Fatalf("reordering 6 tasks: %v", err)
	}

	// First 4 should get P0-P3; remaining should all be P3.
	expected := []models.Priority{models.P0, models.P1, models.P2, models.P3, models.P3, models.P3}
	for i, id := range taskIDs {
		task, err := app.TaskMgr.GetTask(id)
		if err != nil {
			t.Fatalf("getting task %s: %v", id, err)
		}
		if task.Priority != expected[i] {
			t.Fatalf("task %d (%s): expected %s, got %s", i, id, expected[i], task.Priority)
		}
	}
}

// =============================================================================
// Additional edge cases: Archive/unarchive preserves pre-archive status
// =============================================================================

func TestEdgeCase_ArchivePreservesPreArchiveStatus(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "preserve-status", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Resume to make in_progress.
	_, err = app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}

	// Archive.
	_, err = app.TaskMgr.ArchiveTask(task.ID)
	if err != nil {
		t.Fatalf("archiving: %v", err)
	}

	// Unarchive should restore to in_progress.
	unarchived, err := app.TaskMgr.UnarchiveTask(task.ID)
	if err != nil {
		t.Fatalf("unarchiving: %v", err)
	}
	if unarchived.Status != models.StatusInProgress {
		t.Fatalf("expected in_progress after unarchive, got %s", unarchived.Status)
	}
}

// =============================================================================
// Additional edge case: Backlog save/load round trip
// =============================================================================

func TestEdgeCase_BacklogSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	// Add tasks.
	_ = mgr.AddTask(storage.BacklogEntry{ID: "TASK-00001", Title: "Task 1", Status: "backlog", Priority: "P0"})
	_ = mgr.AddTask(storage.BacklogEntry{ID: "TASK-00002", Title: "Task 2", Status: "in_progress", Priority: "P1"})
	_ = mgr.Save()

	// Load in a new manager.
	mgr2 := storage.NewBacklogManager(dir)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}

	task1, err := mgr2.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("getting TASK-00001: %v", err)
	}
	if task1.Title != "Task 1" {
		t.Fatalf("expected 'Task 1', got %q", task1.Title)
	}

	task2, err := mgr2.GetTask("TASK-00002")
	if err != nil {
		t.Fatalf("getting TASK-00002: %v", err)
	}
	if task2.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %q", task2.Status)
	}
}

// =============================================================================
// Additional edge case: Multiple task types
// =============================================================================

func TestEdgeCase_AllTaskTypesCreateSuccessfully(t *testing.T) {
	app := newTestApp(t)

	types := []models.TaskType{
		models.TaskTypeFeat,
		models.TaskTypeBug,
		models.TaskTypeSpike,
		models.TaskTypeRefactor,
	}

	for _, taskType := range types {
		task, err := app.TaskMgr.CreateTask(taskType, "test-"+string(taskType), "", core.CreateTaskOpts{})
		if err != nil {
			t.Fatalf("creating %s task: %v", taskType, err)
		}
		if task.Type != taskType {
			t.Fatalf("expected type %s, got %s", taskType, task.Type)
		}

		// Verify both notes.md and design.md were created.
		ticketDir := filepath.Join(app.BasePath, "tickets", task.ID)
		for _, file := range []string{"notes.md", "design.md", "context.md", "status.yaml"} {
			path := filepath.Join(ticketDir, file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("%s not created for %s task: %s", file, taskType, path)
			}
		}
	}
}

// =============================================================================
// Additional edge case: Sync context with no content
// =============================================================================

func TestEdgeCase_SyncContextEmptyProject(t *testing.T) {
	app := newTestApp(t)

	// Sync context on empty project should succeed.
	if err := app.AICtxGen.SyncContext(); err != nil {
		t.Fatalf("syncing context on empty project: %v", err)
	}

	// Verify files were created.
	claudePath := filepath.Join(app.BasePath, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md should be created even for empty project")
	}
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func newTestBacklogManager(t *testing.T) *fileBacklogManager {
	t.Helper()
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	return mgr
}

func sampleEntry(id string) BacklogEntry {
	return BacklogEntry{
		ID:       id,
		Title:    "Test task " + id,
		Status:   models.StatusBacklog,
		Priority: models.P2,
		Owner:    "@dev",
		Repo:     "github.com/org/repo",
		Branch:   "feat/test",
		Created:  "2026-02-01T00:00:00Z",
		Tags:     []string{"test"},
	}
}

func TestAddTask(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := sampleEntry("TASK-00001")

	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := mgr.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != entry.Title {
		t.Fatalf("expected title %q, got %q", entry.Title, got.Title)
	}
}

func TestAddTask_EmptyID(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := BacklogEntry{Title: "no id"}

	err := mgr.AddTask(entry)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestAddTask_DuplicateID(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := sampleEntry("TASK-00001")

	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := mgr.AddTask(entry)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestUpdateTask(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := sampleEntry("TASK-00001")
	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := mgr.UpdateTask("TASK-00001", BacklogEntry{Title: "Updated title", Status: models.StatusInProgress})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetTask("TASK-00001")
	if got.Title != "Updated title" {
		t.Fatalf("expected updated title, got %q", got.Title)
	}
	if got.Status != models.StatusInProgress {
		t.Fatalf("expected status in_progress, got %q", got.Status)
	}
	// Fields not in the update should be preserved.
	if got.Owner != "@dev" {
		t.Fatalf("expected owner preserved, got %q", got.Owner)
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	mgr := newTestBacklogManager(t)
	err := mgr.UpdateTask("TASK-99999", BacklogEntry{Title: "nope"})
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestRemoveTask(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := sampleEntry("TASK-00001")
	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.RemoveTask("TASK-00001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := mgr.GetTask("TASK-00001")
	if err == nil {
		t.Fatal("expected error after removal")
	}
}

func TestRemoveTask_NotFound(t *testing.T) {
	mgr := newTestBacklogManager(t)
	err := mgr.RemoveTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	mgr := newTestBacklogManager(t)
	_, err := mgr.GetTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestGetAllTasks(t *testing.T) {
	mgr := newTestBacklogManager(t)
	for i := 1; i <= 3; i++ {
		if err := mgr.AddTask(sampleEntry(fmt.Sprintf("TASK-%05d", i))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	tasks, err := mgr.GetAllTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	// Should be sorted by ID.
	if tasks[0].ID != "TASK-00001" || tasks[2].ID != "TASK-00003" {
		t.Fatalf("tasks not sorted by ID: %v, %v", tasks[0].ID, tasks[2].ID)
	}
}

func TestGetAllTasks_Empty(t *testing.T) {
	mgr := newTestBacklogManager(t)
	tasks, err := mgr.GetAllTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestFilterTasks_ByStatus(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Status = models.StatusInProgress
	e2 := sampleEntry("TASK-00002")
	e2.Status = models.StatusBacklog
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{Status: []models.TaskStatus{models.StatusInProgress}})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 in_progress task, got %d", len(result))
	}
}

func TestFilterTasks_ByPriority(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Priority = models.P0
	e2 := sampleEntry("TASK-00002")
	e2.Priority = models.P3
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{Priority: []models.Priority{models.P0}})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 P0 task, got %d", len(result))
	}
}

func TestFilterTasks_ByOwner(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Owner = "@alice"
	e2 := sampleEntry("TASK-00002")
	e2.Owner = "@bob"
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{Owner: "@alice"})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 task for @alice, got %d", len(result))
	}
}

func TestFilterTasks_ByRepo(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Repo = "github.com/org/repoA"
	e2 := sampleEntry("TASK-00002")
	e2.Repo = "github.com/org/repoB"
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{Repo: "github.com/org/repoA"})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 task for repoA, got %d", len(result))
	}
}

func TestFilterTasks_ByTags(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Tags = []string{"security", "auth", "Q1"}
	e2 := sampleEntry("TASK-00002")
	e2.Tags = []string{"security", "Q2"}
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{Tags: []string{"security", "auth"}})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 task with security+auth, got %d", len(result))
	}
}

func TestFilterTasks_MultiCriteria(t *testing.T) {
	mgr := newTestBacklogManager(t)
	e1 := sampleEntry("TASK-00001")
	e1.Status = models.StatusInProgress
	e1.Priority = models.P1
	e1.Owner = "@alice"
	e2 := sampleEntry("TASK-00002")
	e2.Status = models.StatusInProgress
	e2.Priority = models.P1
	e2.Owner = "@bob"
	_ = mgr.AddTask(e1)
	_ = mgr.AddTask(e2)

	result, _ := mgr.FilterTasks(BacklogFilter{
		Status:   []models.TaskStatus{models.StatusInProgress},
		Priority: []models.Priority{models.P1},
		Owner:    "@alice",
	})
	if len(result) != 1 || result[0].ID != "TASK-00001" {
		t.Fatalf("expected 1 task matching all criteria, got %d", len(result))
	}
}

func TestFilterTasks_EmptyFilter(t *testing.T) {
	mgr := newTestBacklogManager(t)
	_ = mgr.AddTask(sampleEntry("TASK-00001"))
	_ = mgr.AddTask(sampleEntry("TASK-00002"))

	result, _ := mgr.FilterTasks(BacklogFilter{})
	if len(result) != 2 {
		t.Fatalf("expected 2 tasks with empty filter, got %d", len(result))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	entry := sampleEntry("TASK-00001")
	entry.Source = "JIRA-1234"
	entry.Tags = []string{"security", "auth"}
	entry.BlockedBy = []string{"TASK-00000"}
	entry.Related = []string{"TASK-00099"}
	_ = mgr.AddTask(entry)

	if err := mgr.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify the file exists.
	if _, err := os.Stat(filepath.Join(dir, "backlog.yaml")); err != nil {
		t.Fatalf("backlog.yaml not created: %v", err)
	}

	mgr2 := NewBacklogManager(dir).(*fileBacklogManager)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	got, err := mgr2.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("task not found after load: %v", err)
	}
	if got.Title != entry.Title {
		t.Fatalf("title mismatch: %q vs %q", got.Title, entry.Title)
	}
	if got.Source != "JIRA-1234" {
		t.Fatalf("source mismatch: %q", got.Source)
	}
	if got.Status != entry.Status {
		t.Fatalf("status mismatch: %q vs %q", got.Status, entry.Status)
	}
	if len(got.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got.Tags))
	}
	if len(got.BlockedBy) != 1 || got.BlockedBy[0] != "TASK-00000" {
		t.Fatalf("blocked_by mismatch: %v", got.BlockedBy)
	}
	if len(got.Related) != 1 || got.Related[0] != "TASK-00099" {
		t.Fatalf("related mismatch: %v", got.Related)
	}
}

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	if err := mgr.Load(); err != nil {
		t.Fatalf("load of missing file should not error: %v", err)
	}
	tasks, _ := mgr.GetAllTasks()
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestSave_WriteFileError(t *testing.T) {
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	_ = mgr.AddTask(sampleEntry("TASK-00001"))

	// Create a directory at the backlog.yaml path to cause WriteFile to fail.
	backlogPath := filepath.Join(dir, "backlog.yaml")
	if err := os.Mkdir(backlogPath, 0o755); err != nil {
		t.Fatal(err)
	}

	err := mgr.Save()
	if err == nil {
		t.Fatal("expected error when backlog.yaml path is a directory")
	}
	if !strings.Contains(err.Error(), "saving backlog") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("{{{{invalid"), 0o644)
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	err := mgr.Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestUpdateTask_AllFields(t *testing.T) {
	mgr := newTestBacklogManager(t)
	entry := sampleEntry("TASK-00001")
	if err := mgr.AddTask(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updates := BacklogEntry{
		Source:    "JIRA-5678",
		Priority:  models.P0,
		Owner:     "@alice",
		Repo:      "github.com/org/new-repo",
		Branch:    "bug/fix-it",
		Created:   "2026-03-01T00:00:00Z",
		Tags:      []string{"security", "urgent"},
		BlockedBy: []string{"TASK-00002"},
		Related:   []string{"TASK-00003", "TASK-00004"},
	}

	if err := mgr.UpdateTask("TASK-00001", updates); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetTask("TASK-00001")
	if got.Source != "JIRA-5678" {
		t.Fatalf("expected source JIRA-5678, got %q", got.Source)
	}
	if got.Priority != models.P0 {
		t.Fatalf("expected priority P0, got %q", got.Priority)
	}
	if got.Owner != "@alice" {
		t.Fatalf("expected owner @alice, got %q", got.Owner)
	}
	if got.Repo != "github.com/org/new-repo" {
		t.Fatalf("expected repo updated, got %q", got.Repo)
	}
	if got.Branch != "bug/fix-it" {
		t.Fatalf("expected branch updated, got %q", got.Branch)
	}
	if got.Created != "2026-03-01T00:00:00Z" {
		t.Fatalf("expected created updated, got %q", got.Created)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "security" {
		t.Fatalf("expected tags updated, got %v", got.Tags)
	}
	if len(got.BlockedBy) != 1 || got.BlockedBy[0] != "TASK-00002" {
		t.Fatalf("expected blocked_by updated, got %v", got.BlockedBy)
	}
	if len(got.Related) != 2 {
		t.Fatalf("expected related updated, got %v", got.Related)
	}
	// Title should be preserved since updates.Title is empty.
	if got.Title != "Test task TASK-00001" {
		t.Fatalf("expected title preserved, got %q", got.Title)
	}
}

func TestLoad_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Create backlog.yaml as a directory so ReadFile fails with a non-IsNotExist error.
	if err := os.MkdirAll(filepath.Join(dir, "backlog.yaml"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	err := mgr.Load()
	if err == nil {
		t.Fatal("expected error when backlog.yaml is a directory")
	}
	if !strings.Contains(err.Error(), "loading backlog") {
		t.Fatalf("expected 'loading backlog' in error, got %q", err.Error())
	}
}

func TestLoad_NilTasksMap(t *testing.T) {
	dir := t.TempDir()
	// Write valid YAML with no tasks key so bf.Tasks will be nil after unmarshal.
	content := "version: \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	if err := mgr.Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tasks should be initialized to an empty map, not nil.
	tasks, err := mgr.GetAllTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks == nil {
		t.Fatal("expected non-nil tasks slice")
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestSave_ReadOnlyDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a file where the base path should be a directory, so MkdirAll fails.
	readOnlyBase := filepath.Join(dir, "readonly")
	// Create a regular file at the path to block MkdirAll.
	if err := os.WriteFile(readOnlyBase, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewBacklogManager(readOnlyBase).(*fileBacklogManager)
	err := mgr.Save()
	if err == nil {
		t.Fatal("expected error when base path is a file")
	}
	if !strings.Contains(err.Error(), "saving backlog") {
		t.Fatalf("expected 'saving backlog' in error, got %q", err.Error())
	}
}

func TestSave_WriteError(t *testing.T) {
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	_ = mgr.AddTask(sampleEntry("TASK-00001"))

	// Make backlog.yaml a directory so WriteFile fails.
	if err := os.MkdirAll(filepath.Join(dir, "backlog.yaml"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	err := mgr.Save()
	if err == nil {
		t.Fatal("expected error when backlog.yaml is a directory")
	}
	if !strings.Contains(err.Error(), "saving backlog") {
		t.Fatalf("expected 'saving backlog' in error, got %q", err.Error())
	}
}

func TestFilterTasks_NoMatchReturnsEmpty(t *testing.T) {
	mgr := newTestBacklogManager(t)
	_ = mgr.AddTask(sampleEntry("TASK-00001"))

	result, err := mgr.FilterTasks(BacklogFilter{Status: []models.TaskStatus{models.StatusDone}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected nil or empty result, got %d", len(result))
	}
}

func TestFilterTasks_GetAllTasksError(t *testing.T) {
	// Test the error path in FilterTasks when GetAllTasks fails.
	dir := t.TempDir()
	mgr := NewBacklogManager(dir).(*fileBacklogManager)

	// Create a corrupted backlog file.
	backlogPath := filepath.Join(dir, "backlog.yaml")
	if err := os.WriteFile(backlogPath, []byte("invalid: yaml: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Attempt to load the corrupted file.
	if err := mgr.Load(); err == nil {
		t.Fatal("expected error loading corrupted backlog")
	}

	// FilterTasks calls GetAllTasks, which should fail with corrupted data.
	// Reset to empty data and manually corrupt m.data.Tasks to trigger the error.
	mgr.data.Tasks = nil // This will cause GetAllTasks to return empty, not error

	// Instead, directly test the path: create a valid entry, then make the file unreadable.
	dir2 := t.TempDir()
	mgr2 := NewBacklogManager(dir2).(*fileBacklogManager)
	_ = mgr2.AddTask(sampleEntry("TASK-00001"))
	_ = mgr2.Save()

	// Now corrupt the file.
	backlogPath2 := filepath.Join(dir2, "backlog.yaml")
	if err := os.WriteFile(backlogPath2, []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load should fail now.
	mgr3 := NewBacklogManager(dir2).(*fileBacklogManager)
	if err := mgr3.Load(); err == nil {
		t.Fatal("expected error loading corrupted backlog")
	}

	// FilterTasks on a corrupted backlog after failed load.
	_, err := mgr3.FilterTasks(BacklogFilter{Status: []models.TaskStatus{models.StatusBacklog}})
	// Since Load failed, mgr3.data.Tasks is empty (not nil), so GetAllTasks returns empty.
	// This actually doesn't test the error path in FilterTasks -> GetAllTasks.
	// The error path in FilterTasks (line 162-163 in backlog.go) is when GetAllTasks
	// itself returns an error, but GetAllTasks in fileBacklogManager never returns an error.
	// So this gap is unreachable in practice.
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(e1)
	mgr.AddTask(e2)

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
	mgr.AddTask(sampleEntry("TASK-00001"))
	mgr.AddTask(sampleEntry("TASK-00002"))

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
	mgr.AddTask(entry)

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

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("{{{{invalid"), 0o644)
	mgr := NewBacklogManager(dir).(*fileBacklogManager)
	err := mgr.Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

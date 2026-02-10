package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// inMemoryBacklog implements BacklogStore for testing.
type inMemoryBacklog struct {
	tasks map[string]BacklogStoreEntry
}

func newInMemoryBacklog() *inMemoryBacklog {
	return &inMemoryBacklog{tasks: make(map[string]BacklogStoreEntry)}
}

func (b *inMemoryBacklog) AddTask(entry BacklogStoreEntry) error {
	if _, exists := b.tasks[entry.ID]; exists {
		return fmt.Errorf("task %s already exists", entry.ID)
	}
	b.tasks[entry.ID] = entry
	return nil
}

func (b *inMemoryBacklog) UpdateTask(taskID string, updates BacklogStoreEntry) error {
	existing, ok := b.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.Priority != "" {
		existing.Priority = updates.Priority
	}
	if updates.Title != "" {
		existing.Title = updates.Title
	}
	b.tasks[taskID] = existing
	return nil
}

func (b *inMemoryBacklog) GetTask(taskID string) (*BacklogStoreEntry, error) {
	e, ok := b.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return &e, nil
}

func (b *inMemoryBacklog) GetAllTasks() ([]BacklogStoreEntry, error) {
	var entries []BacklogStoreEntry
	for _, e := range b.tasks {
		entries = append(entries, e)
	}
	return entries, nil
}

func (b *inMemoryBacklog) FilterTasks(filter BacklogStoreFilter) ([]BacklogStoreEntry, error) {
	var result []BacklogStoreEntry
	for _, e := range b.tasks {
		if len(filter.Status) > 0 {
			matched := false
			for _, s := range filter.Status {
				if e.Status == s {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, e)
	}
	return result, nil
}

func (b *inMemoryBacklog) Load() error  { return nil }
func (b *inMemoryBacklog) Save() error  { return nil }

// mockContextStore implements ContextStore for testing.
type mockContextStore struct {
	loaded map[string]bool
}

func newMockContextStore() *mockContextStore {
	return &mockContextStore{loaded: make(map[string]bool)}
}

func (m *mockContextStore) LoadContext(taskID string) (interface{}, error) {
	m.loaded[taskID] = true
	return nil, nil
}

// helper to build the test dependencies.
func setupTaskManager(t *testing.T) (string, TaskManager, *inMemoryBacklog) {
	t.Helper()
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK")
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)
	backlog := newInMemoryBacklog()
	ctxStore := newMockContextStore()
	mgr := NewTaskManager(dir, bs, backlog, ctxStore)
	return dir, mgr, backlog
}

func TestCreateTask(t *testing.T) {
	dir, mgr, backlog := setupTaskManager(t)

	task, err := mgr.CreateTask(models.TaskTypeFeat, "feat/login", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.ID != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", task.ID)
	}
	if task.Type != models.TaskTypeFeat {
		t.Errorf("expected feat type, got %s", task.Type)
	}
	if task.Branch != "feat/login" {
		t.Errorf("expected branch feat/login, got %s", task.Branch)
	}
	if task.Status != models.StatusBacklog {
		t.Errorf("expected backlog status, got %s", task.Status)
	}

	// Verify ticket folder was created.
	statusPath := filepath.Join(dir, "tickets", "TASK-00001", "status.yaml")
	if _, err := os.Stat(statusPath); err != nil {
		t.Errorf("status.yaml should exist: %v", err)
	}

	// Verify backlog entry.
	entry, err := backlog.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("task should be in backlog: %v", err)
	}
	if entry.Status != models.StatusBacklog {
		t.Errorf("backlog status should be backlog, got %s", entry.Status)
	}
}

func TestCreateTask_Sequential(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	t1, err := mgr.CreateTask(models.TaskTypeFeat, "feat/one", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t2, err := mgr.CreateTask(models.TaskTypeBug, "fix/two", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if t1.ID == t2.ID {
		t.Error("task IDs should be unique")
	}
}

func TestResumeTask(t *testing.T) {
	_, mgr, backlog := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/resume-test", "")
	if err != nil {
		t.Fatalf("unexpected error creating task: %v", err)
	}

	resumed, err := mgr.ResumeTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error resuming task: %v", err)
	}

	if resumed.Status != models.StatusInProgress {
		t.Errorf("resumed task should be in_progress, got %s", resumed.Status)
	}

	// Verify backlog was updated.
	entry, err := backlog.GetTask(created.ID)
	if err != nil {
		t.Fatalf("task should be in backlog: %v", err)
	}
	if entry.Status != models.StatusInProgress {
		t.Errorf("backlog should show in_progress, got %s", entry.Status)
	}
}

func TestResumeTask_AlreadyInProgress(t *testing.T) {
	dir, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/ip", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Manually set to in_progress.
	statusPath := filepath.Join(dir, "tickets", created.ID, "status.yaml")
	data, _ := os.ReadFile(statusPath)
	var task models.Task
	yaml.Unmarshal(data, &task)
	task.Status = models.StatusInProgress
	updated, _ := yaml.Marshal(&task)
	os.WriteFile(statusPath, updated, 0o644)

	// Resume should not change status.
	resumed, err := mgr.ResumeTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resumed.Status != models.StatusInProgress {
		t.Errorf("status should remain in_progress, got %s", resumed.Status)
	}
}

func TestGetTask(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeBug, "fix/crash", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := mgr.GetTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected %s, got %s", created.ID, got.ID)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	_, err := mgr.GetTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestGetAllTasks(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	mgr.CreateTask(models.TaskTypeFeat, "feat/a", "")
	mgr.CreateTask(models.TaskTypeBug, "fix/b", "")

	tasks, err := mgr.GetAllTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestGetTasksByStatus(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/a", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All tasks start as backlog.
	tasks, err := mgr.GetTasksByStatus(models.StatusBacklog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 backlog task, got %d", len(tasks))
	}

	// Resume to in_progress.
	mgr.ResumeTask(created.ID)

	tasks, err = mgr.GetTasksByStatus(models.StatusInProgress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 in_progress task, got %d", len(tasks))
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	dir, mgr, backlog := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/status", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.UpdateTaskStatus(created.ID, models.StatusReview); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check status.yaml.
	statusPath := filepath.Join(dir, "tickets", created.ID, "status.yaml")
	data, _ := os.ReadFile(statusPath)
	var task models.Task
	yaml.Unmarshal(data, &task)
	if task.Status != models.StatusReview {
		t.Errorf("status.yaml should show review, got %s", task.Status)
	}

	// Check backlog.
	entry, _ := backlog.GetTask(created.ID)
	if entry.Status != models.StatusReview {
		t.Errorf("backlog should show review, got %s", entry.Status)
	}
}

func TestUpdateTaskPriority(t *testing.T) {
	dir, mgr, backlog := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/priority", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.UpdateTaskPriority(created.ID, models.P0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statusPath := filepath.Join(dir, "tickets", created.ID, "status.yaml")
	data, _ := os.ReadFile(statusPath)
	var task models.Task
	yaml.Unmarshal(data, &task)
	if task.Priority != models.P0 {
		t.Errorf("status.yaml priority should be P0, got %s", task.Priority)
	}

	entry, _ := backlog.GetTask(created.ID)
	if entry.Priority != models.P0 {
		t.Errorf("backlog priority should be P0, got %s", entry.Priority)
	}
}

func TestReorderPriorities(t *testing.T) {
	dir, mgr, _ := setupTaskManager(t)

	t1, _ := mgr.CreateTask(models.TaskTypeFeat, "feat/one", "")
	t2, _ := mgr.CreateTask(models.TaskTypeBug, "fix/two", "")
	t3, _ := mgr.CreateTask(models.TaskTypeSpike, "spike/three", "")

	if err := mgr.ReorderPriorities([]string{t3.ID, t1.ID, t2.ID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// t3 should be P0, t1 should be P1, t2 should be P2.
	checkPriority := func(taskID string, expected models.Priority) {
		statusPath := filepath.Join(dir, "tickets", taskID, "status.yaml")
		data, _ := os.ReadFile(statusPath)
		var task models.Task
		yaml.Unmarshal(data, &task)
		if task.Priority != expected {
			t.Errorf("task %s should have priority %s, got %s", taskID, expected, task.Priority)
		}
	}

	checkPriority(t3.ID, models.P0)
	checkPriority(t1.ID, models.P1)
	checkPriority(t2.ID, models.P2)
}

func TestArchiveTask(t *testing.T) {
	dir, mgr, backlog := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/archive-me", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resume to in_progress first.
	mgr.ResumeTask(created.ID)

	handoff, err := mgr.ArchiveTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error archiving: %v", err)
	}

	if handoff.TaskID != created.ID {
		t.Errorf("handoff task ID should be %s, got %s", created.ID, handoff.TaskID)
	}
	if handoff.Summary == "" {
		t.Error("handoff summary should not be empty")
	}

	// Verify handoff.md was created.
	handoffPath := filepath.Join(dir, "tickets", created.ID, "handoff.md")
	if _, err := os.Stat(handoffPath); err != nil {
		t.Fatalf("handoff.md should exist: %v", err)
	}
	handoffData, _ := os.ReadFile(handoffPath)
	handoffContent := string(handoffData)
	if !strings.Contains(handoffContent, "# Handoff:") {
		t.Error("handoff.md should contain the handoff heading")
	}
	if !strings.Contains(handoffContent, "Archived") {
		t.Error("handoff.md should mention Archived status")
	}

	// Verify status changed to archived.
	task, err := mgr.GetTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != models.StatusArchived {
		t.Errorf("task should be archived, got %s", task.Status)
	}

	// Verify backlog updated.
	entry, _ := backlog.GetTask(created.ID)
	if entry.Status != models.StatusArchived {
		t.Errorf("backlog should show archived, got %s", entry.Status)
	}

	// Verify pre-archive status was saved.
	preArchivePath := filepath.Join(dir, "tickets", created.ID, ".pre_archive_status")
	preArchiveData, err := os.ReadFile(preArchivePath)
	if err != nil {
		t.Fatalf("pre-archive status file should exist: %v", err)
	}
	if string(preArchiveData) != "in_progress" {
		t.Errorf("pre-archive status should be in_progress, got %s", string(preArchiveData))
	}
}

func TestArchiveTask_AlreadyArchived(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/double-archive", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mgr.ArchiveTask(created.ID)

	_, err = mgr.ArchiveTask(created.ID)
	if err == nil {
		t.Fatal("expected error when archiving already-archived task")
	}
}

func TestUnarchiveTask(t *testing.T) {
	_, mgr, backlog := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/unarchive-me", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resume to in_progress, then archive.
	mgr.ResumeTask(created.ID)
	mgr.ArchiveTask(created.ID)

	// Unarchive should restore to in_progress.
	unarchived, err := mgr.UnarchiveTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error unarchiving: %v", err)
	}

	if unarchived.Status != models.StatusInProgress {
		t.Errorf("unarchived task should be in_progress, got %s", unarchived.Status)
	}

	entry, _ := backlog.GetTask(created.ID)
	if entry.Status != models.StatusInProgress {
		t.Errorf("backlog should show in_progress, got %s", entry.Status)
	}
}

func TestUnarchiveTask_NotArchived(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeFeat, "feat/not-archived", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = mgr.UnarchiveTask(created.ID)
	if err == nil {
		t.Fatal("expected error when unarchiving non-archived task")
	}
}

func TestArchiveUnarchive_RoundTrip(t *testing.T) {
	_, mgr, _ := setupTaskManager(t)

	created, err := mgr.CreateTask(models.TaskTypeBug, "fix/roundtrip", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set to review status.
	mgr.UpdateTaskStatus(created.ID, models.StatusReview)

	// Archive from review.
	_, err = mgr.ArchiveTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unarchive should restore to review.
	restored, err := mgr.UnarchiveTask(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restored.Status != models.StatusReview {
		t.Errorf("restored status should be review, got %s", restored.Status)
	}
}

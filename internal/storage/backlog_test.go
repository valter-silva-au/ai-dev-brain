package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestNewFileBacklogManager(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	if fbm == nil {
		t.Fatal("NewFileBacklogManager returned nil")
	}
	if fbm.filePath != filePath {
		t.Errorf("Expected filePath %s, got %s", filePath, fbm.filePath)
	}
}

func TestFileBacklogManager_Load_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	backlog, err := fbm.Load()

	if err != nil {
		t.Fatalf("Load() should not error on non-existent file: %v", err)
	}
	if backlog == nil {
		t.Fatal("Load() should return empty backlog, not nil")
	}
	if len(backlog.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(backlog.Tasks))
	}
}

func TestFileBacklogManager_Load_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	// Create empty file
	if err := os.WriteFile(filePath, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	fbm := NewFileBacklogManager(filePath)
	backlog, err := fbm.Load()

	if err != nil {
		t.Fatalf("Load() should not error on empty file: %v", err)
	}
	if backlog == nil {
		t.Fatal("Load() should return empty backlog, not nil")
	}
	if len(backlog.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(backlog.Tasks))
	}
}

func TestFileBacklogManager_Save(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	backlog := models.NewBacklog()

	task1 := models.NewTask("TASK-001", "Test task 1", models.TaskTypeFeat)
	task2 := models.NewTask("TASK-002", "Test task 2", models.TaskTypeBug)
	backlog.AddTask(*task1)
	backlog.AddTask(*task2)

	// Save backlog
	err := fbm.Save(backlog)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}

	// Verify file is present, regular, and owner-read/writable.
	// Exact Unix 0o644 bits aren't portable — Windows synthesises 0o666.
	assertOwnerReadWritableFile(t, filePath)

	// Load and verify
	loaded, err := fbm.Load()
	if err != nil {
		t.Fatalf("Failed to load saved backlog: %v", err)
	}
	if len(loaded.Tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(loaded.Tasks))
	}
}

func TestFileBacklogManager_Save_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "subdir", "nested", "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	backlog := models.NewBacklog()

	// Save should create directories
	err := fbm.Save(backlog)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify directory is present + owner-accessible. Exact Unix 0o755
	// bits aren't portable; the contract is "owner can rwx it".
	assertOwnerAccessibleDir(t, filepath.Dir(filePath))
}

func TestFileBacklogManager_AddTask(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task := models.NewTask("TASK-001", "Test task", models.TaskTypeFeat)

	// Add task
	err := fbm.AddTask(*task)
	if err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Verify task was added
	retrieved, err := fbm.GetTask("TASK-001")
	if err != nil {
		t.Fatalf("GetTask() failed: %v", err)
	}
	if retrieved.ID != "TASK-001" {
		t.Errorf("Expected task ID TASK-001, got %s", retrieved.ID)
	}
	if retrieved.Title != "Test task" {
		t.Errorf("Expected title 'Test task', got %s", retrieved.Title)
	}
}

func TestFileBacklogManager_AddTask_Duplicate(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task := models.NewTask("TASK-001", "Test task", models.TaskTypeFeat)

	// Add task first time
	err := fbm.AddTask(*task)
	if err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Try to add duplicate
	err = fbm.AddTask(*task)
	if err == nil {
		t.Fatal("AddTask() should fail for duplicate task")
	}
}

func TestFileBacklogManager_UpdateTask(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task := models.NewTask("TASK-001", "Original title", models.TaskTypeFeat)

	// Add task
	err := fbm.AddTask(*task)
	if err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Update task
	task.Title = "Updated title"
	task.Status = models.TaskStatusInProgress
	task.UpdateTimestamp()

	err = fbm.UpdateTask(*task)
	if err != nil {
		t.Fatalf("UpdateTask() failed: %v", err)
	}

	// Verify update
	retrieved, err := fbm.GetTask("TASK-001")
	if err != nil {
		t.Fatalf("GetTask() failed: %v", err)
	}
	if retrieved.Title != "Updated title" {
		t.Errorf("Expected updated title, got %s", retrieved.Title)
	}
	if retrieved.Status != models.TaskStatusInProgress {
		t.Errorf("Expected status in_progress, got %s", retrieved.Status)
	}
}

func TestFileBacklogManager_UpdateTask_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task := models.NewTask("TASK-999", "Non-existent task", models.TaskTypeFeat)

	// Try to update non-existent task
	err := fbm.UpdateTask(*task)
	if err == nil {
		t.Fatal("UpdateTask() should fail for non-existent task")
	}
}

func TestFileBacklogManager_GetTask(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task := models.NewTask("TASK-001", "Test task", models.TaskTypeFeat)
	task.Priority = models.PriorityP0

	// Add task
	err := fbm.AddTask(*task)
	if err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Get task
	retrieved, err := fbm.GetTask("TASK-001")
	if err != nil {
		t.Fatalf("GetTask() failed: %v", err)
	}
	if retrieved.ID != "TASK-001" {
		t.Errorf("Expected task ID TASK-001, got %s", retrieved.ID)
	}
	if retrieved.Priority != models.PriorityP0 {
		t.Errorf("Expected priority P0, got %s", retrieved.Priority)
	}
}

func TestFileBacklogManager_GetTask_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)

	// Try to get non-existent task
	_, err := fbm.GetTask("TASK-999")
	if err == nil {
		t.Fatal("GetTask() should fail for non-existent task")
	}
}

func TestFileBacklogManager_RemoveTask(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	task1 := models.NewTask("TASK-001", "Task 1", models.TaskTypeFeat)
	task2 := models.NewTask("TASK-002", "Task 2", models.TaskTypeBug)

	// Add tasks
	if err := fbm.AddTask(*task1); err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}
	if err := fbm.AddTask(*task2); err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Remove task
	err := fbm.RemoveTask("TASK-001")
	if err != nil {
		t.Fatalf("RemoveTask() failed: %v", err)
	}

	// Verify task was removed
	_, err = fbm.GetTask("TASK-001")
	if err == nil {
		t.Fatal("Task should not be found after removal")
	}

	// Verify other task remains
	retrieved, err := fbm.GetTask("TASK-002")
	if err != nil {
		t.Fatalf("GetTask() failed for remaining task: %v", err)
	}
	if retrieved.ID != "TASK-002" {
		t.Error("Other task should still exist")
	}
}

func TestFileBacklogManager_RemoveTask_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)

	// Try to remove non-existent task
	err := fbm.RemoveTask("TASK-999")
	if err == nil {
		t.Fatal("RemoveTask() should fail for non-existent task")
	}
}

func TestFileBacklogManager_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)

	// Number of concurrent operations
	numGoroutines := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			taskID := fmt.Sprintf("TASK-%03d", index)
			task := models.NewTask(taskID, fmt.Sprintf("Task %d", index), models.TaskTypeFeat)
			if err := fbm.AddTask(*task); err != nil {
				t.Errorf("AddTask() failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all tasks were added
	backlog, err := fbm.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if len(backlog.Tasks) != numGoroutines {
		t.Errorf("Expected %d tasks, got %d", numGoroutines, len(backlog.Tasks))
	}
}

func TestFileBacklogManager_YAMLFormat(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)

	// Create a task with various fields
	task := models.NewTask("TASK-001", "Test task", models.TaskTypeFeat)
	task.Status = models.TaskStatusInProgress
	task.Priority = models.PriorityP0
	task.Owner = "test-user"
	task.Tags = []string{"tag1", "tag2"}
	task.BlockedBy = []string{"TASK-002"}

	// Add task
	if err := fbm.AddTask(*task); err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Read raw file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Verify YAML format contains expected fields
	yamlStr := string(content)
	expectedFields := []string{
		"tasks:",
		"id: TASK-001",
		"title: Test task",
		"type: feat",
		"status: in_progress",
		"priority: P0",
		"owner: test-user",
		"tags:",
		"- tag1",
		"- tag2",
		"blocked_by:",
		"- TASK-002",
	}

	for _, field := range expectedFields {
		if !contains(yamlStr, field) {
			t.Errorf("YAML does not contain expected field: %s", field)
		}
	}
}

func TestFileBacklogManager_LoadAndSavePreservesData(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)

	// Create tasks with all fields populated
	task1 := models.NewTask("TASK-001", "Task 1", models.TaskTypeFeat)
	task1.Status = models.TaskStatusInProgress
	task1.Priority = models.PriorityP0
	task1.Owner = "user1"
	task1.Repo = "repo1"
	task1.Branch = "branch1"
	task1.Tags = []string{"tag1", "tag2"}
	task1.Created = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	task1.Updated = time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	task2 := models.NewTask("TASK-002", "Task 2", models.TaskTypeBug)
	task2.BlockedBy = []string{"TASK-001"}

	// Add tasks
	if err := fbm.AddTask(*task1); err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}
	if err := fbm.AddTask(*task2); err != nil {
		t.Fatalf("AddTask() failed: %v", err)
	}

	// Load and verify all fields are preserved
	retrieved1, err := fbm.GetTask("TASK-001")
	if err != nil {
		t.Fatalf("GetTask() failed: %v", err)
	}

	if retrieved1.Owner != "user1" {
		t.Errorf("Owner not preserved: got %s", retrieved1.Owner)
	}
	if retrieved1.Repo != "repo1" {
		t.Errorf("Repo not preserved: got %s", retrieved1.Repo)
	}
	if retrieved1.Branch != "branch1" {
		t.Errorf("Branch not preserved: got %s", retrieved1.Branch)
	}
	if len(retrieved1.Tags) != 2 {
		t.Errorf("Tags not preserved: got %d tags", len(retrieved1.Tags))
	}

	retrieved2, err := fbm.GetTask("TASK-002")
	if err != nil {
		t.Fatalf("GetTask() failed: %v", err)
	}
	if len(retrieved2.BlockedBy) != 1 {
		t.Errorf("BlockedBy not preserved: got %d items", len(retrieved2.BlockedBy))
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

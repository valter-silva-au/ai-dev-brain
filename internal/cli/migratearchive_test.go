package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type migrateArchiveMock struct {
	getTasksByStatusFn func(status models.TaskStatus) ([]*models.Task, error)
}

func (m *migrateArchiveMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	if m.getTasksByStatusFn != nil {
		return m.getTasksByStatusFn(status)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}
func (m *migrateArchiveMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

func TestMigrateArchiveCmd_NilTaskManager(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()
	TaskMgr = nil

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateArchiveCmd_NoArchivedTasks(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	TaskMgr = &migrateArchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return nil, nil
		},
	}

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrateArchiveCmd_ListError(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	TaskMgr = &migrateArchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing archived tasks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateArchiveCmd_DryRun(t *testing.T) {
	orig := TaskMgr
	origBase := BasePath
	origDryRun := migrateArchiveDryRun
	defer func() {
		TaskMgr = orig
		BasePath = origBase
		migrateArchiveDryRun = origDryRun
	}()

	tmpDir := t.TempDir()
	BasePath = tmpDir
	migrateArchiveDryRun = true

	// Create a ticket directory that appears to be in the active location.
	taskID := "TASK-00001"
	activeDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		t.Fatalf("creating active dir: %v", err)
	}

	TaskMgr = &migrateArchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return []*models.Task{
				{ID: taskID, Status: models.StatusArchived},
			}, nil
		},
	}

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the directory was NOT moved in dry-run mode.
	if _, err := os.Stat(activeDir); os.IsNotExist(err) {
		t.Error("dry-run should not move directories")
	}
}

func TestMigrateArchiveCmd_MigrateSuccess(t *testing.T) {
	orig := TaskMgr
	origBase := BasePath
	origDryRun := migrateArchiveDryRun
	defer func() {
		TaskMgr = orig
		BasePath = origBase
		migrateArchiveDryRun = origDryRun
	}()

	tmpDir := t.TempDir()
	BasePath = tmpDir
	migrateArchiveDryRun = false

	taskID := "TASK-00002"
	activeDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		t.Fatalf("creating active dir: %v", err)
	}
	// Write a status.yaml so the command can update it.
	statusYAML := fmt.Sprintf("id: %s\nstatus: archived\n", taskID)
	if err := os.WriteFile(filepath.Join(activeDir, "status.yaml"), []byte(statusYAML), 0o644); err != nil {
		t.Fatalf("writing status.yaml: %v", err)
	}

	TaskMgr = &migrateArchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return []*models.Task{
				{ID: taskID, Status: models.StatusArchived},
			}, nil
		},
	}

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the directory was moved.
	archivedDir := filepath.Join(tmpDir, "tickets", "_archived", taskID)
	if _, err := os.Stat(archivedDir); os.IsNotExist(err) {
		t.Error("task should have been moved to _archived/")
	}
	if _, err := os.Stat(activeDir); !os.IsNotExist(err) {
		t.Error("original task directory should have been removed")
	}
}

func TestMigrateArchiveCmd_AlreadyArchived(t *testing.T) {
	orig := TaskMgr
	origBase := BasePath
	origDryRun := migrateArchiveDryRun
	defer func() {
		TaskMgr = orig
		BasePath = origBase
		migrateArchiveDryRun = origDryRun
	}()

	tmpDir := t.TempDir()
	BasePath = tmpDir
	migrateArchiveDryRun = false

	// Task directory only exists in _archived/ (already migrated).
	taskID := "TASK-00003"
	archivedDir := filepath.Join(tmpDir, "tickets", "_archived", taskID)
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatalf("creating archived dir: %v", err)
	}

	TaskMgr = &migrateArchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return []*models.Task{
				{ID: taskID, Status: models.StatusArchived},
			}, nil
		},
	}

	err := migrateArchiveCmd.RunE(migrateArchiveCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

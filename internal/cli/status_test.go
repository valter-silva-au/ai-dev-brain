package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestStatusCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'status' command to be registered")
	}
}

func TestStatusCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := statusCmd.RunE(statusCmd, []string{})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStatusCommand_NoTasks(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
	}()
	statusFilter = ""

	TaskMgr = &statusMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, nil
		},
	}

	err := statusCmd.RunE(statusCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatusCommand_AllTasksGrouped(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
	}()
	statusFilter = ""

	TaskMgr = &statusMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Priority: models.P0, Type: models.TaskTypeFeat, Status: models.StatusInProgress, Branch: "feat/auth"},
				{ID: "TASK-00002", Priority: models.P1, Type: models.TaskTypeBug, Status: models.StatusBacklog, Branch: "bug/fix-login"},
				{ID: "TASK-00003", Priority: models.P2, Type: models.TaskTypeSpike, Status: models.StatusInProgress, Branch: "spike/research"},
			}, nil
		},
	}

	err := statusCmd.RunE(statusCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatusCommand_FilterByStatus(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
	}()
	statusFilter = "in_progress"

	var capturedStatus models.TaskStatus
	TaskMgr = &statusMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			capturedStatus = status
			return []*models.Task{
				{ID: "TASK-00001", Priority: models.P0, Type: models.TaskTypeFeat, Status: models.StatusInProgress, Branch: "feat/auth"},
			}, nil
		},
	}

	err := statusCmd.RunE(statusCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStatus != models.StatusInProgress {
		t.Errorf("capturedStatus = %q, want %q", capturedStatus, models.StatusInProgress)
	}
}

func TestStatusCommand_GetAllTasksError(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
	}()
	statusFilter = ""

	TaskMgr = &statusMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	err := statusCmd.RunE(statusCmd, []string{})
	if err == nil {
		t.Fatal("expected error from GetAllTasks")
	}
	if !strings.Contains(err.Error(), "backlog corrupted") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStatusCommand_GetTasksByStatusError(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
	}()
	statusFilter = "blocked"

	TaskMgr = &statusMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return nil, fmt.Errorf("filter failed")
		},
	}

	err := statusCmd.RunE(statusCmd, []string{})
	if err == nil {
		t.Fatal("expected error from GetTasksByStatus")
	}
	if !strings.Contains(err.Error(), "filter failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// statusMock supports GetAllTasks and GetTasksByStatus.
type statusMock struct {
	getAllTasksFn      func() ([]*models.Task, error)
	getTasksByStatusFn func(status models.TaskStatus) ([]*models.Task, error)
}

func (m *statusMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	if m.getTasksByStatusFn != nil {
		return m.getTasksByStatusFn(status)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) GetAllTasks() ([]*models.Task, error) {
	if m.getAllTasksFn != nil {
		return m.getAllTasksFn()
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *statusMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *statusMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *statusMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *statusMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

// Suppress unused import warning.
var _ = time.Now

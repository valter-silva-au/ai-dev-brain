package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// pickerMock implements core.TaskManager with configurable GetAllTasks.
type pickerMock struct {
	getAllTasksFn func() ([]*models.Task, error)
}

func (m *pickerMock) CreateTask(models.TaskType, string, string, core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) ResumeTask(string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) ArchiveTask(string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) UnarchiveTask(string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) GetTasksByStatus(models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) GetAllTasks() ([]*models.Task, error) {
	if m.getAllTasksFn != nil {
		return m.getAllTasksFn()
	}
	return nil, nil
}
func (m *pickerMock) GetTask(string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *pickerMock) UpdateTaskStatus(string, models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}
func (m *pickerMock) UpdateTaskPriority(string, models.Priority) error {
	return fmt.Errorf("not implemented")
}
func (m *pickerMock) ReorderPriorities([]string) error { return fmt.Errorf("not implemented") }
func (m *pickerMock) CleanupWorktree(string) error     { return fmt.Errorf("not implemented") }

func TestPickResumableTask_NilTaskMgr(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	_, err := pickResumableTask()
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPickResumableTask_NoResumableTasks(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &pickerMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Status: models.StatusDone},
				{ID: "TASK-00002", Status: models.StatusArchived},
			}, nil
		},
	}

	_, err := pickResumableTask()
	if err == nil {
		t.Fatal("expected error when no resumable tasks")
	}
	if !strings.Contains(err.Error(), "no resumable tasks found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPickResumableTask_GetAllTasksError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &pickerMock{
		getAllTasksFn: func() ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog corrupted")
		},
	}

	_, err := pickResumableTask()
	if err == nil {
		t.Fatal("expected error from GetAllTasks failure")
	}
	if !strings.Contains(err.Error(), "listing tasks") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStatusIndex(t *testing.T) {
	tests := []struct {
		status models.TaskStatus
		want   int
	}{
		{models.StatusInProgress, 0},
		{models.StatusBlocked, 1},
		{models.StatusReview, 2},
		{models.StatusBacklog, 3},
		{models.StatusDone, 4},     // beyond statusOrder length
		{models.StatusArchived, 4}, // beyond statusOrder length
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := statusIndex(tt.status)
			if got != tt.want {
				t.Errorf("statusIndex(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestResumableStatuses_FiltersCorrectly(t *testing.T) {
	// Verify which statuses are considered resumable.
	for _, s := range []models.TaskStatus{
		models.StatusInProgress,
		models.StatusBlocked,
		models.StatusReview,
		models.StatusBacklog,
	} {
		if !resumableStatuses[s] {
			t.Errorf("expected %q to be resumable", s)
		}
	}

	for _, s := range []models.TaskStatus{
		models.StatusDone,
		models.StatusArchived,
	} {
		if resumableStatuses[s] {
			t.Errorf("expected %q to NOT be resumable", s)
		}
	}
}

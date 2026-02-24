package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type cleanupMock struct {
	getTaskFn         func(taskID string) (*models.Task, error)
	cleanupWorktreeFn func(taskID string) error
}

func (m *cleanupMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) GetTask(taskID string) (*models.Task, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *cleanupMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}
func (m *cleanupMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}
func (m *cleanupMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}
func (m *cleanupMock) CleanupWorktree(taskID string) error {
	if m.cleanupWorktreeFn != nil {
		return m.cleanupWorktreeFn(taskID)
	}
	return fmt.Errorf("not implemented")
}

func TestCleanupCmd_NilTaskManager(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()
	TaskMgr = nil

	err := cleanupCmd.RunE(cleanupCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCleanupCmd_TaskNotFound(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	TaskMgr = &cleanupMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return nil, fmt.Errorf("task not found: %s", taskID)
		},
	}

	err := cleanupCmd.RunE(cleanupCmd, []string{"TASK-99999"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cleaning up worktree") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCleanupCmd_NoWorktree(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	TaskMgr = &cleanupMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{ID: taskID, WorktreePath: ""}, nil
		},
	}

	err := cleanupCmd.RunE(cleanupCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanupCmd_Success(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	var cleanedUp bool
	TaskMgr = &cleanupMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{ID: taskID, WorktreePath: "/tmp/work/TASK-00001"}, nil
		},
		cleanupWorktreeFn: func(taskID string) error {
			cleanedUp = true
			return nil
		},
	}

	err := cleanupCmd.RunE(cleanupCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleanedUp {
		t.Error("expected CleanupWorktree to be called")
	}
}

func TestCleanupCmd_CleanupError(t *testing.T) {
	orig := TaskMgr
	defer func() { TaskMgr = orig }()

	TaskMgr = &cleanupMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{ID: taskID, WorktreePath: "/tmp/work/TASK-00001"}, nil
		},
		cleanupWorktreeFn: func(taskID string) error {
			return fmt.Errorf("worktree locked")
		},
	}

	err := cleanupCmd.RunE(cleanupCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error from CleanupWorktree")
	}
	if !strings.Contains(err.Error(), "cleaning up worktree") {
		t.Errorf("unexpected error: %v", err)
	}
}

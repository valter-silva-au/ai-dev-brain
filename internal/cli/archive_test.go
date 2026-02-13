package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestArchiveCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "archive" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'archive' command to be registered")
	}
}

func TestArchiveCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := archiveCmd.RunE(archiveCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArchiveCommand_ActiveTaskWithoutForce(t *testing.T) {
	origTaskMgr := TaskMgr
	origForce := archiveForce
	defer func() {
		TaskMgr = origTaskMgr
		archiveForce = origForce
	}()
	archiveForce = false

	TaskMgr = &archiveMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{
				ID:     taskID,
				Status: models.StatusInProgress,
			}, nil
		},
	}

	err := archiveCmd.RunE(archiveCmd, []string{"TASK-00042"})
	if err == nil {
		t.Fatal("expected error for active task without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force: %v", err)
	}
}

func TestArchiveCommand_ActiveTaskWithForce(t *testing.T) {
	origTaskMgr := TaskMgr
	origForce := archiveForce
	defer func() {
		TaskMgr = origTaskMgr
		archiveForce = origForce
	}()
	archiveForce = true

	var archiveCalled bool
	TaskMgr = &archiveMock{
		archiveTaskFn: func(taskID string) (*models.HandoffDocument, error) {
			archiveCalled = true
			return &models.HandoffDocument{
				TaskID:      taskID,
				Summary:     "Task completed successfully",
				GeneratedAt: time.Now(),
			}, nil
		},
	}

	err := archiveCmd.RunE(archiveCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !archiveCalled {
		t.Error("ArchiveTask was not called")
	}
}

func TestArchiveCommand_DoneTaskWithoutForce(t *testing.T) {
	origTaskMgr := TaskMgr
	origForce := archiveForce
	defer func() {
		TaskMgr = origTaskMgr
		archiveForce = origForce
	}()
	archiveForce = false

	var archiveCalled bool
	TaskMgr = &archiveMock{
		getTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{
				ID:     taskID,
				Status: models.StatusDone,
			}, nil
		},
		archiveTaskFn: func(taskID string) (*models.HandoffDocument, error) {
			archiveCalled = true
			return &models.HandoffDocument{
				TaskID:      taskID,
				Summary:     "Done",
				GeneratedAt: time.Now(),
			}, nil
		},
	}

	err := archiveCmd.RunE(archiveCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !archiveCalled {
		t.Error("ArchiveTask should be called for done tasks without --force")
	}
}

func TestArchiveCommand_ArchiveError(t *testing.T) {
	origTaskMgr := TaskMgr
	origForce := archiveForce
	defer func() {
		TaskMgr = origTaskMgr
		archiveForce = origForce
	}()
	archiveForce = true

	TaskMgr = &archiveMock{
		archiveTaskFn: func(taskID string) (*models.HandoffDocument, error) {
			return nil, fmt.Errorf("worktree cleanup failed")
		},
	}

	err := archiveCmd.RunE(archiveCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error from ArchiveTask")
	}
	if !strings.Contains(err.Error(), "worktree cleanup failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// archiveMock supports GetTask and ArchiveTask.
type archiveMock struct {
	getTaskFn     func(taskID string) (*models.Task, error)
	archiveTaskFn func(taskID string) (*models.HandoffDocument, error)
}

func (m *archiveMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	if m.archiveTaskFn != nil {
		return m.archiveTaskFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) GetTask(taskID string) (*models.Task, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *archiveMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *archiveMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *archiveMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *archiveMock) CleanupWorktree(taskID string) error {
	return nil
}

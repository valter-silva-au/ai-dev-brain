package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestPriorityCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "priority" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'priority' command to be registered")
	}
}

func TestPriorityCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := priorityCmd.RunE(priorityCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPriorityCommand_Success(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedIDs []string
	TaskMgr = &priorityMock{
		reorderPrioritiesFn: func(taskIDs []string) error {
			capturedIDs = taskIDs
			return nil
		},
	}

	err := priorityCmd.RunE(priorityCmd, []string{"TASK-00003", "TASK-00001", "TASK-00005"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 3 {
		t.Fatalf("expected 3 task IDs, got %d", len(capturedIDs))
	}
	if capturedIDs[0] != "TASK-00003" || capturedIDs[1] != "TASK-00001" || capturedIDs[2] != "TASK-00005" {
		t.Errorf("capturedIDs = %v, want [TASK-00003 TASK-00001 TASK-00005]", capturedIDs)
	}
}

func TestPriorityCommand_SingleTask(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedIDs []string
	TaskMgr = &priorityMock{
		reorderPrioritiesFn: func(taskIDs []string) error {
			capturedIDs = taskIDs
			return nil
		},
	}

	err := priorityCmd.RunE(priorityCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 1 || capturedIDs[0] != "TASK-00001" {
		t.Errorf("capturedIDs = %v, want [TASK-00001]", capturedIDs)
	}
}

func TestPriorityCommand_ReorderError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &priorityMock{
		reorderPrioritiesFn: func(taskIDs []string) error {
			return fmt.Errorf("task TASK-99999 not found")
		},
	}

	err := priorityCmd.RunE(priorityCmd, []string{"TASK-99999"})
	if err == nil {
		t.Fatal("expected error from ReorderPriorities")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPriorityCommand_RequiresArg(t *testing.T) {
	// Cobra's MinimumNArgs(1) validator should reject empty args.
	validator := priorityCmd.Args
	err := validator(priorityCmd, []string{})
	if err == nil {
		t.Fatal("expected error when no task ID provided")
	}
}

// priorityMock supports ReorderPriorities.
type priorityMock struct {
	reorderPrioritiesFn func(taskIDs []string) error
}

func (m *priorityMock) CreateTask(taskType models.TaskType, branchName string, repoPath string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *priorityMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *priorityMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *priorityMock) ReorderPriorities(taskIDs []string) error {
	if m.reorderPrioritiesFn != nil {
		return m.reorderPrioritiesFn(taskIDs)
	}
	return fmt.Errorf("not implemented")
}

func (m *priorityMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

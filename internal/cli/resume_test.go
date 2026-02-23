package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestResumeCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "resume" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'resume' command to be registered")
	}
}

func TestResumeCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := resumeCmd.RunE(resumeCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResumeCommand_Success(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedID string
	TaskMgr = &mockTaskManager{
		createTaskFn: nil,
	}
	// Override ResumeTask via a custom mock.
	TaskMgr = &resumeMock{
		resumeTaskFn: func(taskID string) (*models.Task, error) {
			capturedID = taskID
			return &models.Task{
				ID:         taskID,
				Type:       models.TaskTypeFeat,
				Status:     models.StatusInProgress,
				Branch:     "feat/oauth-flow",
				TicketPath: "tickets/" + taskID,
			}, nil
		},
	}

	err := resumeCmd.RunE(resumeCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != "TASK-00042" {
		t.Errorf("capturedID = %q, want %q", capturedID, "TASK-00042")
	}
}

func TestResumeCommand_TaskNotFound(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &resumeMock{
		resumeTaskFn: func(taskID string) (*models.Task, error) {
			return nil, fmt.Errorf("task %s not found, run 'adb status' to list valid tasks", taskID)
		},
	}

	err := resumeCmd.RunE(resumeCmd, []string{"TASK-99999"})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResumeCommand_AcceptsZeroArgs(t *testing.T) {
	// resume now accepts zero args (interactive picker) or one arg.
	validator := resumeCmd.Args
	err := validator(resumeCmd, []string{})
	if err != nil {
		t.Fatalf("expected no error for zero args, got: %v", err)
	}
	err = validator(resumeCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("expected no error for one arg, got: %v", err)
	}
	err = validator(resumeCmd, []string{"TASK-00001", "TASK-00002"})
	if err == nil {
		t.Fatal("expected error for two args")
	}
}

// resumeMock is a mock that supports ResumeTask properly.
type resumeMock struct {
	resumeTaskFn func(taskID string) (*models.Task, error)
}

func (m *resumeMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) ResumeTask(taskID string) (*models.Task, error) {
	if m.resumeTaskFn != nil {
		return m.resumeTaskFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *resumeMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *resumeMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *resumeMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *resumeMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

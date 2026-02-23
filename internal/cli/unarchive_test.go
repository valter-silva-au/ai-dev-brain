package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

func TestUnarchiveCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "unarchive" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'unarchive' command to be registered")
	}
}

func TestUnarchiveCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := unarchiveCmd.RunE(unarchiveCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnarchiveCommand_Success(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedID string
	TaskMgr = &unarchiveMock{
		unarchiveTaskFn: func(taskID string) (*models.Task, error) {
			capturedID = taskID
			return &models.Task{
				ID:         taskID,
				Type:       models.TaskTypeFeat,
				Status:     models.StatusDone,
				Branch:     "feat/oauth-flow",
				TicketPath: "tickets/" + taskID,
			}, nil
		},
	}

	err := unarchiveCmd.RunE(unarchiveCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != "TASK-00042" {
		t.Errorf("capturedID = %q, want %q", capturedID, "TASK-00042")
	}
}

func TestUnarchiveCommand_Error(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &unarchiveMock{
		unarchiveTaskFn: func(taskID string) (*models.Task, error) {
			return nil, fmt.Errorf("task %s not found in archive", taskID)
		},
	}

	err := unarchiveCmd.RunE(unarchiveCmd, []string{"TASK-99999"})
	if err == nil {
		t.Fatal("expected error from UnarchiveTask")
	}
	if !strings.Contains(err.Error(), "not found in archive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnarchiveCommand_RequiresArg(t *testing.T) {
	// Cobra's ExactArgs(1) validator should reject empty args.
	validator := unarchiveCmd.Args
	err := validator(unarchiveCmd, []string{})
	if err == nil {
		t.Fatal("expected error when no task ID provided")
	}
}

// --- ValidArgsFunction tests for unarchive init ---

func TestUnarchiveCommand_ValidArgsFunction_NilTaskMgr(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	if unarchiveCmd.ValidArgsFunction == nil {
		t.Fatal("expected ValidArgsFunction to be set")
	}

	ids, directive := unarchiveCmd.ValidArgsFunction(unarchiveCmd, nil, "")
	if ids != nil {
		t.Errorf("expected nil ids when TaskMgr is nil, got %v", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestUnarchiveCommand_ValidArgsFunction_GetTasksByStatusError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &unarchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return nil, fmt.Errorf("backlog error")
		},
	}

	ids, directive := unarchiveCmd.ValidArgsFunction(unarchiveCmd, nil, "")
	if ids != nil {
		t.Errorf("expected nil ids on error, got %v", ids)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected NoFileComp directive, got %d", directive)
	}
}

func TestUnarchiveCommand_ValidArgsFunction_ReturnsArchivedTasks(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &unarchiveMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			if status != models.StatusArchived {
				t.Errorf("expected StatusArchived, got %s", status)
			}
			return []*models.Task{
				{ID: "TASK-00001", Type: models.TaskTypeFeat, Branch: "feat/auth"},
				{ID: "TASK-00002", Type: models.TaskTypeBug, Branch: "bug/crash"},
			}, nil
		},
	}

	ids, _ := unarchiveCmd.ValidArgsFunction(unarchiveCmd, nil, "")
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	// Test filtering by prefix.
	ids, _ = unarchiveCmd.ValidArgsFunction(unarchiveCmd, nil, "TASK-00001")
	if len(ids) != 1 {
		t.Errorf("expected 1 id matching prefix, got %d", len(ids))
	}

	ids, _ = unarchiveCmd.ValidArgsFunction(unarchiveCmd, nil, "NONEXIST")
	if len(ids) != 0 {
		t.Errorf("expected 0 ids for NONEXIST prefix, got %d", len(ids))
	}
}

func TestUnarchiveCommand_SuccessWithWorktree(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &unarchiveMock{
		unarchiveTaskFn: func(taskID string) (*models.Task, error) {
			return &models.Task{
				ID:           taskID,
				Type:         models.TaskTypeFeat,
				Status:       models.StatusDone,
				Branch:       "feat/oauth-flow",
				WorktreePath: "/tmp/work/" + taskID,
				TicketPath:   "tickets/" + taskID,
			}, nil
		},
	}

	err := unarchiveCmd.RunE(unarchiveCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// unarchiveMock supports UnarchiveTask and GetTasksByStatus.
type unarchiveMock struct {
	unarchiveTaskFn    func(taskID string) (*models.Task, error)
	getTasksByStatusFn func(status models.TaskStatus) ([]*models.Task, error)
}

func (m *unarchiveMock) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) UnarchiveTask(taskID string) (*models.Task, error) {
	if m.unarchiveTaskFn != nil {
		return m.unarchiveTaskFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	if m.getTasksByStatusFn != nil {
		return m.getTasksByStatusFn(status)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *unarchiveMock) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *unarchiveMock) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *unarchiveMock) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *unarchiveMock) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

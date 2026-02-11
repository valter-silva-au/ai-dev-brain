package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// mockTaskManager implements core.TaskManager for testing.
type mockTaskManager struct {
	createTaskFn func(taskType models.TaskType, branchName string, repoPath string) (*models.Task, error)
}

func (m *mockTaskManager) CreateTask(taskType models.TaskType, branchName string, repoPath string) (*models.Task, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(taskType, branchName, repoPath)
	}
	return &models.Task{
		ID:         "TASK-00001",
		Type:       taskType,
		Branch:     fmt.Sprintf("%s/%s", taskType, branchName),
		Repo:       repoPath,
		TicketPath: "tickets/TASK-00001",
		Created:    time.Now(),
	}, nil
}

func (m *mockTaskManager) ResumeTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) ArchiveTask(taskID string) (*models.HandoffDocument, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) UnarchiveTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) GetTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) GetAllTasks() ([]*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) GetTask(taskID string) (*models.Task, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) UpdateTaskStatus(taskID string, status models.TaskStatus) error {
	return fmt.Errorf("not implemented")
}

func (m *mockTaskManager) UpdateTaskPriority(taskID string, priority models.Priority) error {
	return fmt.Errorf("not implemented")
}

func (m *mockTaskManager) ReorderPriorities(taskIDs []string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockTaskManager) CleanupWorktree(taskID string) error {
	return fmt.Errorf("not implemented")
}

// --- Tests ---

func TestFeatCommand_RegistrationInRoot(t *testing.T) {
	// Verify that feat, bug, spike, and refactor are registered.
	subcommands := rootCmd.Commands()
	names := make(map[string]bool)
	for _, cmd := range subcommands {
		names[cmd.Name()] = true
	}

	for _, expected := range []string{"feat", "bug", "spike", "refactor"} {
		if !names[expected] {
			t.Errorf("expected command %q to be registered, but it was not", expected)
		}
	}
}

func TestFeatCommand_RequiresBranchArg(t *testing.T) {
	// Save and restore TaskMgr.
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = &mockTaskManager{}

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no branch argument provided")
	}
}

func TestFeatCommand_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{"my-branch"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFeatCommand_PassesCorrectTaskType(t *testing.T) {
	for _, taskType := range []models.TaskType{models.TaskTypeFeat, models.TaskTypeBug, models.TaskTypeSpike, models.TaskTypeRefactor} {
		t.Run(string(taskType), func(t *testing.T) {
			origTaskMgr := TaskMgr
			defer func() { TaskMgr = origTaskMgr }()

			var capturedType models.TaskType
			var capturedBranch string
			TaskMgr = &mockTaskManager{
				createTaskFn: func(tt models.TaskType, bn string, rp string) (*models.Task, error) {
					capturedType = tt
					capturedBranch = bn
					return &models.Task{
						ID:         "TASK-00042",
						Type:       tt,
						Branch:     fmt.Sprintf("%s/%s", tt, bn),
						TicketPath: "tickets/TASK-00042",
					}, nil
				},
			}

			cmd := newTaskCommand(taskType)
			cmd.SetArgs([]string{"test-branch"})
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedType != taskType {
				t.Errorf("task type = %q, want %q", capturedType, taskType)
			}
			if capturedBranch != "test-branch" {
				t.Errorf("branch = %q, want %q", capturedBranch, "test-branch")
			}
		})
	}
}

func TestFeatCommand_PassesRepoFlag(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedRepo string
	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string) (*models.Task, error) {
			capturedRepo = rp
			return &models.Task{
				ID:         "TASK-00001",
				Type:       tt,
				Branch:     bn,
				Repo:       rp,
				TicketPath: "tickets/TASK-00001",
			}, nil
		},
	}

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{"--repo", "github.com/org/repo", "my-feature"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedRepo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", capturedRepo, "github.com/org/repo")
	}
}

func TestFeatCommand_CreateTaskError(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string) (*models.Task, error) {
			return nil, fmt.Errorf("task already exists, run 'adb resume' to continue")
		},
	}

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{"existing-branch"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from CreateTask")
	}
	if !strings.Contains(err.Error(), "task already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

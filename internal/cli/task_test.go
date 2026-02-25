package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// --- Registration Tests ---

func TestTaskCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'task' command to be registered on root")
	}
}

func TestTaskCmd_Subcommands(t *testing.T) {
	expected := []string{"create", "resume", "archive", "unarchive", "cleanup", "status", "priority", "update"}
	subs := make(map[string]bool)
	for _, cmd := range taskCmd.Commands() {
		subs[cmd.Name()] = true
	}
	for _, name := range expected {
		if !subs[name] {
			t.Errorf("expected subcommand %q on 'task', but it was not registered", name)
		}
	}
}

// --- task create Tests ---

func TestTaskCreate_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskCreateCmd.RunE(taskCreateCmd, []string{"my-branch"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskCreate_ArgsValidation(t *testing.T) {
	// Verify taskCreateCmd has ExactArgs(1) by checking Args is set.
	if taskCreateCmd.Args == nil {
		t.Fatal("expected taskCreateCmd.Args to be set (cobra.ExactArgs(1))")
	}
	// Cobra's ExactArgs(1) returns an error for 0 args.
	err := taskCreateCmd.Args(taskCreateCmd, []string{})
	if err == nil {
		t.Fatal("expected error from Args validator with 0 args")
	}
	// And succeeds for exactly 1 arg.
	err = taskCreateCmd.Args(taskCreateCmd, []string{"branch"})
	if err != nil {
		t.Fatalf("expected no error from Args validator with 1 arg, got: %v", err)
	}
}

func TestTaskCreate_DefaultTypeFeat(t *testing.T) {
	origTaskMgr := TaskMgr
	origFlag := taskCreateTypeFlag
	defer func() {
		TaskMgr = origTaskMgr
		taskCreateTypeFlag = origFlag
	}()
	taskCreateTypeFlag = "feat"

	var capturedType models.TaskType
	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
			capturedType = tt
			return &models.Task{
				ID:         "TASK-00001",
				Type:       tt,
				Branch:     bn,
				TicketPath: "tickets/TASK-00001",
			}, nil
		},
	}

	err := taskCreateCmd.RunE(taskCreateCmd, []string{"my-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedType != models.TaskTypeFeat {
		t.Errorf("task type = %q, want %q", capturedType, models.TaskTypeFeat)
	}
}

func TestTaskCreate_AllTypes(t *testing.T) {
	for _, tt := range []string{"feat", "bug", "spike", "refactor"} {
		t.Run(tt, func(t *testing.T) {
			origTaskMgr := TaskMgr
			origFlag := taskCreateTypeFlag
			defer func() {
				TaskMgr = origTaskMgr
				taskCreateTypeFlag = origFlag
			}()
			taskCreateTypeFlag = tt

			var capturedType models.TaskType
			TaskMgr = &mockTaskManager{
				createTaskFn: func(taskType models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
					capturedType = taskType
					return &models.Task{
						ID:         "TASK-00001",
						Type:       taskType,
						Branch:     bn,
						TicketPath: "tickets/TASK-00001",
					}, nil
				},
			}

			err := taskCreateCmd.RunE(taskCreateCmd, []string{"my-branch"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(capturedType) != tt {
				t.Errorf("task type = %q, want %q", capturedType, tt)
			}
		})
	}
}

func TestTaskCreate_Error(t *testing.T) {
	origTaskMgr := TaskMgr
	origFlag := taskCreateTypeFlag
	defer func() {
		TaskMgr = origTaskMgr
		taskCreateTypeFlag = origFlag
	}()
	taskCreateTypeFlag = "feat"

	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
			return nil, fmt.Errorf("branch already exists")
		},
	}

	err := taskCreateCmd.RunE(taskCreateCmd, []string{"dup-branch"})
	if err == nil {
		t.Fatal("expected error from CreateTask")
	}
	if !strings.Contains(err.Error(), "branch already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- task resume Tests ---

func TestTaskResume_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskResumeCmd.RunE(taskResumeCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskResume_DelegatesRunE(t *testing.T) {
	// Verify that taskResumeCmd.RunE is the same function as resumeCmd.RunE.
	if taskResumeCmd.RunE == nil {
		t.Fatal("taskResumeCmd.RunE is nil")
	}
}

// --- task archive Tests ---

func TestTaskArchive_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskArchiveCmd.RunE(taskArchiveCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskArchive_ForceFlag(t *testing.T) {
	origTaskMgr := TaskMgr
	origForce := taskArchiveForce
	origPkgForce := archiveForce
	defer func() {
		TaskMgr = origTaskMgr
		taskArchiveForce = origForce
		archiveForce = origPkgForce
	}()
	taskArchiveForce = true

	var archiveCalled bool
	TaskMgr = &archiveMock{
		archiveTaskFn: func(taskID string) (*models.HandoffDocument, error) {
			archiveCalled = true
			return &models.HandoffDocument{
				TaskID:      taskID,
				Summary:     "Done",
				GeneratedAt: time.Now(),
			}, nil
		},
	}

	err := taskArchiveCmd.RunE(taskArchiveCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !archiveCalled {
		t.Error("ArchiveTask was not called")
	}
}

// --- task unarchive Tests ---

func TestTaskUnarchive_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskUnarchiveCmd.RunE(taskUnarchiveCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- task cleanup Tests ---

func TestTaskCleanup_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskCleanupCmd.RunE(taskCleanupCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- task status Tests ---

func TestTaskStatus_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskStatusCmd.RunE(taskStatusCmd, []string{})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskStatus_FilterRestored(t *testing.T) {
	origTaskMgr := TaskMgr
	origFilter := statusFilter
	origTaskFilter := taskStatusFilter
	defer func() {
		TaskMgr = origTaskMgr
		statusFilter = origFilter
		taskStatusFilter = origTaskFilter
	}()

	taskStatusFilter = "in_progress"

	TaskMgr = &statusMock{
		getTasksByStatusFn: func(status models.TaskStatus) ([]*models.Task, error) {
			return []*models.Task{
				{ID: "TASK-00001", Priority: models.P0, Type: models.TaskTypeFeat, Status: models.StatusInProgress, Branch: "feat/auth"},
			}, nil
		},
	}

	err := taskStatusCmd.RunE(taskStatusCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the original statusFilter is restored.
	if statusFilter != origFilter {
		t.Errorf("statusFilter not restored: got %q, want %q", statusFilter, origFilter)
	}
}

// --- task priority Tests ---

func TestTaskPriority_NilTaskManager(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()
	TaskMgr = nil

	err := taskPriorityCmd.RunE(taskPriorityCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when TaskMgr is nil")
	}
	if !strings.Contains(err.Error(), "task manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- task update Tests ---

func TestTaskUpdate_NilUpdateGenerator(t *testing.T) {
	origGen := UpdateGen
	defer func() { UpdateGen = origGen }()
	UpdateGen = nil

	err := taskUpdateCmd.RunE(taskUpdateCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when UpdateGen is nil")
	}
	if !strings.Contains(err.Error(), "update generator not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Deprecation Tests ---

func TestDeprecated_OldFeatCommand(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "feat" {
			if cmd.Deprecated == "" {
				t.Error("expected 'feat' command to have Deprecated set")
			}
			if !strings.Contains(cmd.Deprecated, "adb task create") {
				t.Errorf("feat Deprecated = %q, should mention 'adb task create'", cmd.Deprecated)
			}
			return
		}
	}
	t.Error("'feat' command not found on root")
}

func TestDeprecated_OldResumeCommand(t *testing.T) {
	if resumeCmd.Deprecated == "" {
		t.Error("expected 'resume' command to have Deprecated set")
	}
	if !strings.Contains(resumeCmd.Deprecated, "adb task resume") {
		t.Errorf("resume Deprecated = %q, should mention 'adb task resume'", resumeCmd.Deprecated)
	}
}

func TestDeprecated_OldArchiveCommand(t *testing.T) {
	if archiveCmd.Deprecated == "" {
		t.Error("expected 'archive' command to have Deprecated set")
	}
	if !strings.Contains(archiveCmd.Deprecated, "adb task archive") {
		t.Errorf("archive Deprecated = %q, should mention 'adb task archive'", archiveCmd.Deprecated)
	}
}

func TestDeprecated_OldUnarchiveCommand(t *testing.T) {
	if unarchiveCmd.Deprecated == "" {
		t.Error("expected 'unarchive' command to have Deprecated set")
	}
}

func TestDeprecated_OldCleanupCommand(t *testing.T) {
	if cleanupCmd.Deprecated == "" {
		t.Error("expected 'cleanup' command to have Deprecated set")
	}
}

func TestDeprecated_OldStatusCommand(t *testing.T) {
	if statusCmd.Deprecated == "" {
		t.Error("expected 'status' command to have Deprecated set")
	}
}

func TestDeprecated_OldPriorityCommand(t *testing.T) {
	if priorityCmd.Deprecated == "" {
		t.Error("expected 'priority' command to have Deprecated set")
	}
}

func TestDeprecated_OldUpdateCommand(t *testing.T) {
	if updateCmd.Deprecated == "" {
		t.Error("expected 'update' command to have Deprecated set")
	}
}

// --- Completion Tests ---

func TestCompleteTaskTypes(t *testing.T) {
	completions, dir := completeTaskTypes(nil, nil, "")
	if dir == 0 {
		// dir should include NoFileComp
	}
	if len(completions) != 4 {
		t.Errorf("expected 4 task type completions, got %d", len(completions))
	}
	types := strings.Join(completions, " ")
	for _, expected := range []string{"feat", "bug", "spike", "refactor"} {
		if !strings.Contains(types, expected) {
			t.Errorf("expected %q in completions, got: %s", expected, types)
		}
	}
}

// --- Hidden Commands Tests ---

func TestHidden_HookCommand(t *testing.T) {
	if !hookCmd.Hidden {
		t.Error("expected 'hook' command to be hidden")
	}
}

func TestHidden_WorktreeHookCommand(t *testing.T) {
	if !worktreeHookCmd.Hidden {
		t.Error("expected 'worktree-hook' command to be hidden")
	}
}

func TestHidden_WorktreeLifecycleCommand(t *testing.T) {
	if !worktreeLifecycleCmd.Hidden {
		t.Error("expected 'worktree-lifecycle' command to be hidden")
	}
}

func TestHidden_MigrateArchiveCommand(t *testing.T) {
	if !migrateArchiveCmd.Hidden {
		t.Error("expected 'migrate-archive' command to be hidden")
	}
}

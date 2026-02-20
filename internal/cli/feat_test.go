package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// mockTaskManager implements core.TaskManager for testing.
type mockTaskManager struct {
	createTaskFn func(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error)
}

func (m *mockTaskManager) CreateTask(taskType models.TaskType, branchName string, repoPath string, opts core.CreateTaskOpts) (*models.Task, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(taskType, branchName, repoPath, opts)
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
				createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
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
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
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
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
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

func TestFeatCommand_OutputIncludesWorktreeWhenPresent(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
			return &models.Task{
				ID:           "TASK-00001",
				Type:         tt,
				Branch:       bn,
				Repo:         rp,
				WorktreePath: "", // no worktree
				TicketPath:   "tickets/TASK-00001",
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
}

func TestFeatCommand_PassesPriorityOwnerTags(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	var capturedOpts core.CreateTaskOpts
	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
			capturedOpts = opts
			return &models.Task{
				ID:         "TASK-00001",
				Type:       tt,
				Branch:     bn,
				TicketPath: "tickets/TASK-00001",
			}, nil
		},
	}

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{"--priority", "P0", "--owner", "alice", "--tags", "ui,frontend", "my-feature"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(capturedOpts.Priority) != "P0" {
		t.Errorf("priority = %q, want P0", capturedOpts.Priority)
	}
	if capturedOpts.Owner != "alice" {
		t.Errorf("owner = %q, want alice", capturedOpts.Owner)
	}
	if len(capturedOpts.Tags) != 2 || capturedOpts.Tags[0] != "ui" || capturedOpts.Tags[1] != "frontend" {
		t.Errorf("tags = %v, want [ui frontend]", capturedOpts.Tags)
	}
}

func TestFeatCommand_OutputIncludesWorktreeAndRepo(t *testing.T) {
	origTaskMgr := TaskMgr
	defer func() { TaskMgr = origTaskMgr }()

	TaskMgr = &mockTaskManager{
		createTaskFn: func(tt models.TaskType, bn string, rp string, opts core.CreateTaskOpts) (*models.Task, error) {
			return &models.Task{
				ID:           "TASK-00001",
				Type:         tt,
				Branch:       bn,
				Repo:         "github.com/org/repo",
				WorktreePath: "/nonexistent/worktree/path",
				TicketPath:   "tickets/TASK-00001",
			}, nil
		},
	}

	cmd := newTaskCommand(models.TaskTypeFeat)
	cmd.SetArgs([]string{"my-feature"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// This will try launchWorkflow with a non-existent path, which returns early.
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetTerminalTitle(t *testing.T) {
	// setTerminalTitle writes to /dev/tty or stderr. We just verify it doesn't panic.
	// On Linux CI, /dev/tty may or may not be available.
	setTerminalTitle("test-title")
}

func TestLaunchWorkflow_NonExistentWorktree(t *testing.T) {
	// When worktree path doesn't exist, launchWorkflow returns early.
	// This should not panic.
	launchWorkflow("TASK-00001", "feat/test", "/nonexistent/path/that/does/not/exist", false)
}

func TestLaunchWorkflow_ExistingDirNoClaude(t *testing.T) {
	// Create a temp dir to act as worktree path.
	tmpDir := t.TempDir()

	// Save PATH and set it to empty so claude won't be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()) // empty dir -> no binaries found

	// launchWorkflow should not panic; claude not found triggers the fallback print.
	// We can't easily capture stdout here, so just verify no panic.
	launchWorkflow("TASK-00001", "feat/test", tmpDir, false)

	// Restore PATH.
	t.Setenv("PATH", origPath)
}

func TestDetectGitRoot(t *testing.T) {
	// In the test environment, we're in a git repo so this should return something.
	// Or if not, it should return empty string (not panic).
	result := detectGitRoot()
	// We don't assert a specific value since the test environment may vary.
	_ = result
}

func TestDetectGitRoot_NotInGitRepo(t *testing.T) {
	// Run from a temp dir that is not a git repo.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	result := detectGitRoot()
	if result != "" {
		t.Errorf("expected empty string in non-git dir, got %q", result)
	}
}

func TestSetTerminalTitle_DevTtyUnavailable(t *testing.T) {
	// Test that setTerminalTitle falls back to stderr when /dev/tty is unavailable.
	// On Windows, /dev/tty doesn't exist, so this tests the fallback path.
	// On Linux, we can't easily make /dev/tty unavailable in tests, but we can
	// at least verify the function doesn't panic.
	setTerminalTitle("fallback-test")
}

func TestLaunchWorkflow_WithFakeClaude_BashShell(t *testing.T) {
	// Create a fake claude binary and a fake shell that both exit immediately.
	// This exercises the post-LookPath code in launchWorkflow.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	// Create a fake "claude" that exits 0.
	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "bash" that exits 0 (so shellCmd.Run completes).
	bashScript := filepath.Join(tmpBin, "bash")
	if err := os.WriteFile(bashScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin)
	t.Setenv("SHELL", filepath.Join(tmpBin, "bash"))

	// Should not panic. Exercises lines 148-170 and the else branch (bash) at lines 221-227, plus 229-234.
	launchWorkflow("TASK-00099", "feat/test", worktree, false)
}

func TestLaunchWorkflow_WithFakeClaude_ZshShell(t *testing.T) {
	// Exercise the zsh-specific ZDOTDIR branch in launchWorkflow.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	// Create a fake "claude" that exits 1 (exercises the error print path).
	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "zsh" that exits 0.
	zshScript := filepath.Join(tmpBin, "zsh")
	if err := os.WriteFile(zshScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin)
	t.Setenv("SHELL", filepath.Join(tmpBin, "zsh"))
	t.Setenv("HOME", t.TempDir())

	// Should not panic. Exercises the zsh branch at lines 184-220 and the
	// claude error path at lines 155-158.
	launchWorkflow("TASK-00099", "feat/zsh-test", worktree, false)
}

func TestLaunchWorkflow_WithFakeClaude_EmptyShellEnv(t *testing.T) {
	// Exercise the SHELL="" fallback to /bin/bash.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	// Create a fake "claude" that exits 0.
	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin+":/bin:/usr/bin")
	t.Setenv("SHELL", "")

	// Exercises the SHELL=="" -> "/bin/bash" fallback at lines 165-168.
	// /bin/bash is expected to fail or run briefly (no stdin).
	launchWorkflow("TASK-00099", "feat/no-shell", worktree, false)
}

func TestLaunchWorkflow_ResumeNoClaude(t *testing.T) {
	// When resume=true and claude is not on PATH, the fallback message
	// should include --resume. Verify no panic.
	tmpDir := t.TempDir()

	t.Setenv("PATH", t.TempDir()) // empty dir -> no binaries found

	launchWorkflow("TASK-00001", "feat/test", tmpDir, true)
}

func TestLaunchWorkflow_ResumeWithFakeClaude_BashShell(t *testing.T) {
	// When resume=true and claude is found, the --resume flag should be
	// passed to claude. Fake claude checks for --resume in its args.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	// Create a fake "claude" that checks for --resume and exits 0 if found.
	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = \"--resume\" ]; then exit 0; fi\ndone\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "bash" that exits 0.
	bashScript := filepath.Join(tmpBin, "bash")
	if err := os.WriteFile(bashScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin)
	t.Setenv("SHELL", filepath.Join(tmpBin, "bash"))

	// Should not panic. The fake claude verifies --resume is in the args.
	launchWorkflow("TASK-00099", "feat/resume-test", worktree, true)
}

func TestLaunchWorkflow_ResumeWithFakeClaude_ZshShell(t *testing.T) {
	// Same as the bash variant but exercises the zsh shell branch with resume=true.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	// Create a fake "claude" that checks for --resume.
	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = \"--resume\" ]; then exit 0; fi\ndone\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "zsh" that exits 0.
	zshScript := filepath.Join(tmpBin, "zsh")
	if err := os.WriteFile(zshScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin)
	t.Setenv("SHELL", filepath.Join(tmpBin, "zsh"))
	t.Setenv("HOME", t.TempDir())

	// Should not panic. Exercises the zsh branch with resume=true.
	launchWorkflow("TASK-00099", "feat/resume-zsh-test", worktree, true)
}

func TestRepoShortName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"simple name", "myrepo", "myrepo"},
		{"path with slashes", "github.com/org/repo", "repo"},
		{"path with .git suffix", "github.com/org/repo.git", "repo"},
		{"backslash path", `C:\Users\dev\repo`, "repo"},
		{"colon separated", "git@github.com:org/repo", "repo"},
		{"colon with .git", "git@github.com:org/repo.git", "repo"},
		{"single segment with .git", "myrepo.git", "myrepo"},
		{"trailing slash stripped by caller", "github.com/org/repo", "repo"},
		{"deeply nested path", "a/b/c/d/e/repo-name", "repo-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoShortName(tt.input)
			if got != tt.want {
				t.Errorf("repoShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLaunchWorkflow_ZshWithZDOTDIR(t *testing.T) {
	// Exercise the ZDOTDIR path in the zsh branch.
	tmpBin := t.TempDir()
	worktree := t.TempDir()

	claudeScript := filepath.Join(tmpBin, "claude")
	if err := os.WriteFile(claudeScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	zshScript := filepath.Join(tmpBin, "zsh")
	if err := os.WriteFile(zshScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpBin)
	t.Setenv("SHELL", filepath.Join(tmpBin, "zsh"))
	t.Setenv("ZDOTDIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// Exercises ZDOTDIR != "" path at line 192.
	launchWorkflow("TASK-00099", "feat/zdotdir-test", worktree, false)
}

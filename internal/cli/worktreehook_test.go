package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeHookCreate_MissingEnvVars(t *testing.T) {
	// Ensure env vars are not set
	os.Unsetenv("ADB_TASK_ID")
	os.Unsetenv("ADB_WORKTREE_PATH")

	// Should exit gracefully without error
	err := worktreeHookCreateCmd.RunE(worktreeHookCreateCmd, []string{})
	if err != nil {
		t.Errorf("Expected nil error with missing env vars, got: %v", err)
	}
}

func TestWorktreeHookCreate_ValidEnvVars(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up a fake worktree with task-context.md
	worktreePath := filepath.Join(tmpDir, "worktree")
	taskContextPath := filepath.Join(worktreePath, ".claude", "rules")
	if err := os.MkdirAll(taskContextPath, 0o755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskContextPath, "task-context.md"), []byte("test context"), 0o644); err != nil {
		t.Fatalf("Failed to create task-context.md: %v", err)
	}

	// Set env vars
	os.Setenv("ADB_TASK_ID", "TASK-00001")
	os.Setenv("ADB_WORKTREE_PATH", worktreePath)
	defer func() {
		os.Unsetenv("ADB_TASK_ID")
		os.Unsetenv("ADB_WORKTREE_PATH")
	}()

	// Should succeed
	err := worktreeHookCreateCmd.RunE(worktreeHookCreateCmd, []string{})
	if err != nil {
		t.Errorf("Expected nil error with valid env vars, got: %v", err)
	}
}

func TestWorktreeHookCreate_MissingTaskContext(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up a worktree WITHOUT task-context.md
	worktreePath := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Set env vars
	os.Setenv("ADB_TASK_ID", "TASK-00001")
	os.Setenv("ADB_WORKTREE_PATH", worktreePath)
	defer func() {
		os.Unsetenv("ADB_TASK_ID")
		os.Unsetenv("ADB_WORKTREE_PATH")
	}()

	// Should succeed (graceful handling)
	err := worktreeHookCreateCmd.RunE(worktreeHookCreateCmd, []string{})
	if err != nil {
		t.Errorf("Expected nil error with missing task-context.md, got: %v", err)
	}
}

func TestWorktreeHookRemove_MissingEnvVar(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("ADB_TASK_ID")

	// Should exit gracefully without error
	err := worktreeHookRemoveCmd.RunE(worktreeHookRemoveCmd, []string{})
	if err != nil {
		t.Errorf("Expected nil error with missing env var, got: %v", err)
	}
}

func TestWorktreeHookRemove_ValidEnvVar(t *testing.T) {
	// Set env var
	os.Setenv("ADB_TASK_ID", "TASK-00001")
	defer os.Unsetenv("ADB_TASK_ID")

	// Should succeed
	err := worktreeHookRemoveCmd.RunE(worktreeHookRemoveCmd, []string{})
	if err != nil {
		t.Errorf("Expected nil error with valid env var, got: %v", err)
	}
}

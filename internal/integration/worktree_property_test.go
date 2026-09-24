package integration

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_RepoPathNormalizationHTTPS verifies HTTPS URL normalization
func TestProperty_RepoPathNormalizationHTTPS(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		platform := rapid.SampledFrom([]string{"github.com", "gitlab.com", "bitbucket.org"}).Draw(t, "platform")
		org := rapid.StringMatching(`^[a-z][a-z0-9-]+$`).Draw(t, "org")
		repo := rapid.StringMatching(`^[a-z][a-z0-9-]+$`).Draw(t, "repo")

		mgr := NewGitWorktreeManager(".")
		httpsURL := "https://" + platform + "/" + org + "/" + repo + ".git"

		normalized, err := mgr.NormalizeRepoPath(httpsURL)
		if err != nil {
			t.Fatalf("Failed to normalize HTTPS URL: %v", err)
		}

		expected := platform + "/" + org + "/" + repo
		if normalized != expected {
			t.Fatalf("Expected %s, got %s", expected, normalized)
		}

		// Verify .git suffix is removed
		if strings.HasSuffix(normalized, ".git") {
			t.Fatal("Normalized path should not have .git suffix")
		}
	})
}

// TestProperty_RepoPathNormalizationSSH verifies SSH URL normalization
func TestProperty_RepoPathNormalizationSSH(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		platform := rapid.SampledFrom([]string{"github.com", "gitlab.com", "bitbucket.org"}).Draw(t, "platform")
		org := rapid.StringMatching(`^[a-z][a-z0-9-]+$`).Draw(t, "org")
		repo := rapid.StringMatching(`^[a-z][a-z0-9-]+$`).Draw(t, "repo")

		mgr := NewGitWorktreeManager(".")
		sshURL := "git@" + platform + ":" + org + "/" + repo + ".git"

		normalized, err := mgr.NormalizeRepoPath(sshURL)
		if err != nil {
			t.Fatalf("Failed to normalize SSH URL: %v", err)
		}

		expected := platform + "/" + org + "/" + repo
		if normalized != expected {
			t.Fatalf("Expected %s, got %s", expected, normalized)
		}

		// Verify .git suffix is removed
		if strings.HasSuffix(normalized, ".git") {
			t.Fatal("Normalized path should not have .git suffix")
		}
	})
}

// TestProperty_RepoPathNormalizationLocalPath verifies local path handling
func TestProperty_RepoPathNormalizationLocalPath(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(".")

		// Test relative paths
		relativePath := "./" + rapid.StringMatching(`^[a-z][a-z0-9/]+$`).Draw(t, "path")
		normalized, err := mgr.NormalizeRepoPath(relativePath)
		if err != nil {
			t.Fatalf("Failed to normalize relative path: %v", err)
		}

		if normalized != relativePath {
			t.Fatalf("Relative path should be unchanged: expected %s, got %s", relativePath, normalized)
		}

		// Test absolute paths
		absolutePath := "/" + rapid.StringMatching(`^[a-z][a-z0-9/]+$`).Draw(t, "abspath")
		normalized, err = mgr.NormalizeRepoPath(absolutePath)
		if err != nil {
			t.Fatalf("Failed to normalize absolute path: %v", err)
		}

		if normalized != absolutePath {
			t.Fatalf("Absolute path should be unchanged: expected %s, got %s", absolutePath, normalized)
		}
	})
}

// TestProperty_RepoPathNormalizationEmpty verifies empty path handling
func TestProperty_RepoPathNormalizationEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(".")

		_, err := mgr.NormalizeRepoPath("")
		if err == nil {
			t.Fatal("Empty path should return error")
		}

		if !strings.Contains(err.Error(), "empty") {
			t.Fatalf("Error message should mention empty path, got: %v", err)
		}
	})
}

// TestProperty_WorktreeTaskIDValidation verifies task ID validation
func TestProperty_WorktreeTaskIDValidation(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(baseDir)

		// Test empty task ID
		_, err := mgr.CreateWorktree("", "test/repo", "main")
		if err == nil {
			t.Fatal("Empty task ID should return error")
		}

		if !strings.Contains(err.Error(), "taskID") && !strings.Contains(err.Error(), "empty") {
			t.Fatalf("Error should mention taskID, got: %v", err)
		}
	})
}

// TestProperty_WorktreeRepoPathValidation verifies repo path validation
func TestProperty_WorktreeRepoPathValidation(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(baseDir)
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		// Test empty repo path
		_, err := mgr.CreateWorktree(taskID, "", "main")
		if err == nil {
			t.Fatal("Empty repo path should return error")
		}

		if !strings.Contains(err.Error(), "repoPath") && !strings.Contains(err.Error(), "empty") {
			t.Fatalf("Error should mention repoPath, got: %v", err)
		}
	})
}

// TestProperty_WorktreeBaseBranchDefault verifies default base branch
func TestProperty_WorktreeBaseBranchDefault(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(baseDir)
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		// This will fail because we don't have an actual git repo,
		// but we're testing that empty baseBranch doesn't cause a panic
		// and that the validation happens before any git operations
		_, err := mgr.CreateWorktree(taskID, "/nonexistent/repo", "")

		// Should fail but not panic
		if err == nil {
			t.Fatal("Non-existent repo should return error")
		}
	})
}

// TestProperty_WorktreeGetForTaskConsistency verifies GetWorktreeForTask consistency
func TestProperty_WorktreeGetForTaskConsistency(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(baseDir)
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		// Check non-existent worktree
		_, exists, err := mgr.GetWorktreeForTask(taskID)
		if err != nil {
			t.Fatalf("GetWorktreeForTask should not error for non-existent task: %v", err)
		}

		if exists {
			t.Fatal("Non-existent worktree should not exist")
		}
	})
}

// TestProperty_WorktreeTaskIDEmpty verifies empty task ID handling
func TestProperty_WorktreeTaskIDEmpty(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		mgr := NewGitWorktreeManager(baseDir)

		_, _, err := mgr.GetWorktreeForTask("")
		if err == nil {
			t.Fatal("Empty task ID should return error")
		}

		if !strings.Contains(err.Error(), "taskID") && !strings.Contains(err.Error(), "empty") {
			t.Fatalf("Error should mention taskID, got: %v", err)
		}
	})
}

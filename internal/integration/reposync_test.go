package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepoSyncManager(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a test git repository
	repoPath := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Skipf("Git not available, skipping test: %v", err)
	}

	// Configure git
	exec.Command("git", "config", "user.email", "test@example.com").Run()
	exec.Command("git", "config", "user.name", "Test User").Run()

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	cmd.Run()

	t.Run("SyncRepo", func(t *testing.T) {
		rsm := NewRepoSyncManager(2)

		result, err := rsm.SyncRepo(repoPath)
		if err != nil {
			t.Errorf("SyncRepo failed: %v", err)
		}

		if result == nil {
			t.Fatal("Expected non-nil result")
		}

		if !result.Success {
			t.Errorf("Expected success, got: %v", result.Error)
		}

		if len(result.Actions) == 0 {
			t.Error("Expected at least one action")
		}
	})

	t.Run("SyncRepo_NotARepo", func(t *testing.T) {
		rsm := NewRepoSyncManager(2)

		notRepoPath := filepath.Join(tmpDir, "not-a-repo")
		os.MkdirAll(notRepoPath, 0o755)

		result, err := rsm.SyncRepo(notRepoPath)
		if err == nil {
			t.Error("Expected error for non-git directory")
		}

		if result == nil || result.Success {
			t.Error("Expected failed result for non-git directory")
		}
	})

	t.Run("SyncAll", func(t *testing.T) {
		rsm := NewRepoSyncManager(2)

		// Create repos directory
		reposDir := filepath.Join(tmpDir, "repos")
		if err := os.MkdirAll(reposDir, 0o755); err != nil {
			t.Fatalf("Failed to create repos directory: %v", err)
		}

		// Move test repo to repos directory
		newRepoPath := filepath.Join(reposDir, "test-repo")
		if err := os.Rename(repoPath, newRepoPath); err != nil {
			t.Fatalf("Failed to move repo: %v", err)
		}

		results, err := rsm.SyncAll(reposDir)
		if err != nil {
			t.Errorf("SyncAll failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if len(results) > 0 && !results[0].Success {
			t.Errorf("Expected successful sync, got: %v", results[0].Error)
		}
	})

	t.Run("SyncAll_EmptyDirectory", func(t *testing.T) {
		rsm := NewRepoSyncManager(2)

		emptyDir := filepath.Join(tmpDir, "empty-repos")
		os.MkdirAll(emptyDir, 0o755)

		results, err := rsm.SyncAll(emptyDir)
		if err != nil {
			t.Errorf("SyncAll failed on empty directory: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("Expected 0 results for empty directory, got %d", len(results))
		}
	})

	t.Run("SyncAll_NonexistentDirectory", func(t *testing.T) {
		rsm := NewRepoSyncManager(2)

		nonexistent := filepath.Join(tmpDir, "nonexistent")

		_, err := rsm.SyncAll(nonexistent)
		if err == nil {
			t.Error("Expected error for nonexistent directory")
		}
	})
}

func TestRepoSyncManager_Concurrency(t *testing.T) {
	t.Run("MaxConcurrency", func(t *testing.T) {
		rsm := NewRepoSyncManager(0)
		defaultRsm := rsm.(*DefaultRepoSyncManager)
		if defaultRsm.maxConcurrency != 4 {
			t.Errorf("Expected default maxConcurrency of 4, got %d", defaultRsm.maxConcurrency)
		}

		rsm = NewRepoSyncManager(8)
		customRsm := rsm.(*DefaultRepoSyncManager)
		if customRsm.maxConcurrency != 8 {
			t.Errorf("Expected maxConcurrency of 8, got %d", customRsm.maxConcurrency)
		}
	})
}

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeRepoPath(t *testing.T) {
	manager := NewGitWorktreeManager("")

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "HTTPS URL",
			input:    "https://github.com/org/repo.git",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "HTTPS URL without .git",
			input:    "https://github.com/org/repo",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "HTTP URL",
			input:    "http://github.com/org/repo.git",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "SSH URL",
			input:    "git@github.com:org/repo.git",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "SSH URL without .git",
			input:    "git@github.com:org/repo",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "Absolute local path",
			input:    "/home/user/repos/myrepo",
			expected: "/home/user/repos/myrepo",
			wantErr:  false,
		},
		{
			name:     "Relative local path",
			input:    "./local/repo",
			expected: "./local/repo",
			wantErr:  false,
		},
		{
			name:     "Relative parent path",
			input:    "../parent/repo",
			expected: "../parent/repo",
			wantErr:  false,
		},
		{
			// Absolute Windows path. On Windows, filepath.IsAbs returns
			// true so the early-return path is taken. On Unix, IsAbs is
			// false but none of the URL/SSH prefixes match, so the default
			// branch still returns the input unchanged — making the assert
			// pass identically on all three OSes.
			name:     "Absolute Windows path",
			input:    `C:\Users\user\repos\myrepo`,
			expected: `C:\Users\user\repos\myrepo`,
			wantErr:  false,
		},
		{
			name:     "Platform/org/repo format",
			input:    "github.com/org/repo",
			expected: "github.com/org/repo",
			wantErr:  false,
		},
		{
			name:     "Empty path",
			input:    "",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.NormalizeRepoPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NormalizeRepoPath() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NormalizeRepoPath() unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("NormalizeRepoPath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCreateWorktreeLocalRepo(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to init git repo: %v: %s", err, string(output))
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to config git user email: %v: %s", err, string(output))
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to config git user name: %v: %s", err, string(output))
	}

	// Create initial commit
	testFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add file: %v: %s", err, string(output))
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to commit: %v: %s", err, string(output))
	}

	// Create main branch (git init creates 'master' by default in some versions)
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create main branch: %v: %s", err, string(output))
	}

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create worktree for a task
	taskID := "TASK-001"
	worktreePath, err := manager.CreateWorktree(taskID, repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	// Verify worktree was created
	expectedPath := filepath.Join(workBase, "work", taskID)
	if worktreePath != expectedPath {
		t.Errorf("Expected worktree at %s, got %s", expectedPath, worktreePath)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Errorf("Worktree directory does not exist: %s", worktreePath)
	}

	// Verify .git file exists
	gitFile := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitFile); os.IsNotExist(err) {
		t.Errorf(".git file does not exist: %s", gitFile)
	}

	// Verify README.md exists in worktree
	readmeInWorktree := filepath.Join(worktreePath, "README.md")
	if _, err := os.Stat(readmeInWorktree); os.IsNotExist(err) {
		t.Errorf("README.md does not exist in worktree: %s", readmeInWorktree)
	}
}

// TestCreateWorktree_WindowsAbsoluteRepoPath is a regression test for the
// pre-existing Windows bug where absolute Windows repoPath values (like
// `C:\Users\...\repo`) were treated as remote repo paths and concatenated
// under `basePath/repos/`, producing invalid paths like
// `workspace\repos\C:\Users\...` that git cannot chdir into.
//
// The fix at internal/integration/worktree.go lines 71 and 133 swaps
// `strings.HasPrefix(p, "/")` for `filepath.IsAbs(p)` so Windows drive-
// letter paths are correctly recognised as absolute local paths.
func TestCreateWorktree_WindowsAbsoluteRepoPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only regression test — filepath.IsAbs semantics differ per OS.")
	}

	tempDir := t.TempDir()

	// Init a git repo at an absolute Windows path (t.TempDir returns one
	// on Windows by construction: `C:\Users\...\TempN\TestN\001`).
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}
	setupGitRepo(t, repoDir)

	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	taskID := "TASK-WIN-001"
	worktreePath, err := manager.CreateWorktree(taskID, repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() with Windows absolute repoPath failed: %v", err)
	}

	// The worktree must live under workBase/work/<taskID>, with no
	// embedded drive letter in the middle of the path.
	expectedPath := filepath.Join(workBase, "work", taskID)
	if worktreePath != expectedPath {
		t.Errorf("Expected worktree at %s, got %s", expectedPath, worktreePath)
	}

	// Sanity: repoDir's drive letter must NOT appear inside worktreePath
	// (guards against the specific bug shape `workspace\repos\C:\...`).
	if strings.Contains(worktreePath, ":") && strings.Index(worktreePath, ":") != 1 {
		t.Errorf("worktreePath contains embedded drive letter after position 1: %s", worktreePath)
	}

	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Errorf("Worktree directory does not exist: %s", worktreePath)
	}
}

func TestCreateWorktreeEmptyParams(t *testing.T) {
	manager := NewGitWorktreeManager("")

	tests := []struct {
		name        string
		taskID      string
		repoPath    string
		baseBranch  string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Empty taskID",
			taskID:      "",
			repoPath:    "/some/repo",
			baseBranch:  "main",
			wantErr:     true,
			errContains: "taskID cannot be empty",
		},
		{
			name:        "Empty repoPath",
			taskID:      "TASK-001",
			repoPath:    "",
			baseBranch:  "main",
			wantErr:     true,
			errContains: "repoPath cannot be empty",
		},
		{
			name:        "Path traversal in taskID",
			taskID:      "../../tmp/evil",
			repoPath:    "/some/repo",
			baseBranch:  "main",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "Semicolon in taskID",
			taskID:      "TASK;rm -rf",
			repoPath:    "/some/repo",
			baseBranch:  "main",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "Space in taskID",
			taskID:      "TASK 001",
			repoPath:    "/some/repo",
			baseBranch:  "main",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.CreateWorktree(tt.taskID, tt.repoPath, tt.baseBranch)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateWorktree() expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("CreateWorktree() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CreateWorktree() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoveWorktree(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo with initial commit
	setupGitRepo(t, repoDir)

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create worktree
	taskID := "TASK-002"
	worktreePath, err := manager.CreateWorktree(taskID, repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatalf("Worktree was not created: %s", worktreePath)
	}

	// Remove worktree (clean, no remote → the non-forced dirty guard passes)
	if err := manager.RemoveWorktree(worktreePath, false); err != nil {
		t.Fatalf("RemoveWorktree() failed: %v", err)
	}

	// Verify worktree was removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("Worktree still exists after removal: %s", worktreePath)
	}
}

func TestRemoveWorktreeErrors(t *testing.T) {
	manager := NewGitWorktreeManager("")

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Empty path",
			path:        "",
			wantErr:     true,
			errContains: "worktreePath cannot be empty",
		},
		{
			// Use a guaranteed-nonexistent subpath of t.TempDir() so
			// the error shape is portable: `/nonexistent/path` worked on
			// Unix (Stat returns ENOENT) but Windows either canonicalised
			// it to the current drive's root and found something, or
			// produced a Windows-specific error string. A subdir of
			// TempDir guarantees Stat returns os.ErrNotExist everywhere.
			name:        "Non-existent path",
			path:        filepath.Join(t.TempDir(), "definitely-not-here"),
			wantErr:     true,
			errContains: "worktree does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RemoveWorktree(tt.path, false)
			if tt.wantErr {
				if err == nil {
					t.Errorf("RemoveWorktree() expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RemoveWorktree() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("RemoveWorktree() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRemoveWorktree_RefusesDirtyUnlessForce verifies the #207 safety guard: a
// worktree holding uncommitted/untracked work is refused unless force is set.
func TestRemoveWorktree_RefusesDirtyUnlessForce(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}
	setupGitRepo(t, repoDir)

	manager := NewGitWorktreeManager(filepath.Join(tempDir, "workspace"))
	worktreePath, err := manager.CreateWorktree("TASK-207", repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	// Dirty the worktree with an untracked file.
	if err := os.WriteFile(filepath.Join(worktreePath, "scratch.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatalf("failed to write scratch file: %v", err)
	}

	// Non-forced removal must refuse and leave the worktree intact.
	if err := manager.RemoveWorktree(worktreePath, false); err == nil {
		t.Fatal("RemoveWorktree(force=false) should refuse a dirty worktree")
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("dirty worktree was removed despite the guard")
	}

	// Forced removal proceeds.
	if err := manager.RemoveWorktree(worktreePath, true); err != nil {
		t.Fatalf("RemoveWorktree(force=true) failed: %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree still exists after forced removal")
	}
}

// TestRemoveBranch verifies opt-in orphan-branch cleanup (#207): once a
// worktree is gone, its branch can be pruned from the repo.
func TestRemoveBranch(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}
	setupGitRepo(t, repoDir)

	manager := NewGitWorktreeManager(filepath.Join(tempDir, "workspace"))
	worktreePath, err := manager.CreateWorktree("TASK-208", repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}
	branch := "task/TASK-208" // legacy CreateWorktree branch shape

	// The branch is checked out in the worktree; remove the worktree first.
	if err := manager.RemoveWorktree(worktreePath, true); err != nil {
		t.Fatalf("RemoveWorktree() failed: %v", err)
	}
	if !branchExists(t, repoDir, branch) {
		t.Fatalf("branch %q should exist before prune", branch)
	}
	if err := manager.RemoveBranch(repoDir, branch); err != nil {
		t.Fatalf("RemoveBranch() failed: %v", err)
	}
	if branchExists(t, repoDir, branch) {
		t.Errorf("branch %q should be gone after RemoveBranch", branch)
	}
}

func branchExists(t *testing.T, repoDir, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list failed: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// TestBranchExists verifies the branch-uniqueness lookup behind the #208 guard:
// unknown branches report false, a worktree's branch reports true, and an
// un-cloned repo reports false rather than erroring.
func TestBranchExists(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}
	setupGitRepo(t, repoDir)
	manager := NewGitWorktreeManager(filepath.Join(tempDir, "workspace"))

	if exists, err := manager.BranchExists(repoDir, "feat/nope"); err != nil || exists {
		t.Errorf("BranchExists(feat/nope) = %v, %v; want false, nil", exists, err)
	}

	if _, err := manager.CreateWorktree("TASK-209", repoDir, "main"); err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}
	if exists, err := manager.BranchExists(repoDir, "task/TASK-209"); err != nil || !exists {
		t.Errorf("BranchExists(task/TASK-209) = %v, %v; want true, nil", exists, err)
	}

	// A repo that was never cloned reports false, not an error.
	if exists, err := manager.BranchExists("github.com/never/cloned", "main"); err != nil || exists {
		t.Errorf("BranchExists(uncloned) = %v, %v; want false, nil", exists, err)
	}
}

func TestParseStatusBranchLine(t *testing.T) {
	cases := []struct {
		in            string
		branch        string
		ahead, behind int
	}{
		{"main...origin/main [ahead 1, behind 2]", "main", 1, 2},
		{"main...origin/main [ahead 3]", "main", 3, 0},
		{"main...origin/main [behind 5]", "main", 0, 5},
		{"main...origin/main", "main", 0, 0},
		{"feat/x", "feat/x", 0, 0},
		{"No commits yet on main", "main", 0, 0},
	}
	for _, tc := range cases {
		b, a, be := parseStatusBranchLine(tc.in)
		if b != tc.branch || a != tc.ahead || be != tc.behind {
			t.Errorf("parseStatusBranchLine(%q) = (%q,%d,%d), want (%q,%d,%d)", tc.in, b, a, be, tc.branch, tc.ahead, tc.behind)
		}
	}
}

// TestWorktreeStatus exercises the live git-state read behind `adb status`
// (#209): a clean worktree, a dirtied one, and a missing path.
func TestWorktreeStatus(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}
	setupGitRepo(t, repoDir)
	manager := NewGitWorktreeManager(filepath.Join(tempDir, "workspace"))

	worktreePath, err := manager.CreateWorktree("TASK-209", repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	st, err := manager.WorktreeStatus(worktreePath)
	if err != nil {
		t.Fatalf("WorktreeStatus() failed: %v", err)
	}
	if !st.Exists || st.Dirty {
		t.Errorf("fresh worktree: got Exists=%v Dirty=%v, want true/false", st.Exists, st.Dirty)
	}
	if st.Branch != "task/TASK-209" {
		t.Errorf("branch = %q, want task/TASK-209", st.Branch)
	}

	// Dirty it.
	if err := os.WriteFile(filepath.Join(worktreePath, "scratch.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatalf("write scratch: %v", err)
	}
	if st, err := manager.WorktreeStatus(worktreePath); err != nil || !st.Dirty {
		t.Errorf("dirtied worktree: got Dirty=%v err=%v, want true/nil", st.Dirty, err)
	}

	// Missing worktree → Exists=false, no error.
	st, err = manager.WorktreeStatus(filepath.Join(tempDir, "does-not-exist"))
	if err != nil {
		t.Errorf("missing worktree should not error, got %v", err)
	}
	if st.Exists {
		t.Errorf("missing worktree: Exists should be false")
	}
}

// TestRestoreWorktree covers the #211 rebuild primitive: after a worktree is
// removed, RestoreWorktree re-creates it — attaching the surviving branch, or
// recreating a vanished branch from the base.
func TestRestoreWorktree(t *testing.T) {
	setup := func(t *testing.T) (GitWorktreeManager, string, string) {
		t.Helper()
		tempDir := t.TempDir()
		repoDir := filepath.Join(tempDir, "test-repo")
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatalf("mkdir repo: %v", err)
		}
		setupGitRepo(t, repoDir)
		return NewGitWorktreeManager(filepath.Join(tempDir, "workspace")), repoDir, tempDir
	}

	t.Run("attaches surviving branch", func(t *testing.T) {
		manager, repoDir, _ := setup(t)
		wt, err := manager.CreateWorktree("TASK-211", repoDir, "main")
		if err != nil {
			t.Fatalf("CreateWorktree: %v", err)
		}
		// Remove the worktree (branch task/TASK-211 survives).
		if err := manager.RemoveWorktree(wt, true); err != nil {
			t.Fatalf("RemoveWorktree: %v", err)
		}
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Fatalf("precondition: worktree should be gone")
		}
		if !branchExists(t, repoDir, "task/TASK-211") {
			t.Fatalf("precondition: branch should survive worktree removal")
		}

		if _, err := manager.RestoreWorktree(repoDir, "task/TASK-211", wt, "main"); err != nil {
			t.Fatalf("RestoreWorktree: %v", err)
		}
		if _, err := os.Stat(wt); err != nil {
			t.Errorf("worktree not restored: %v", err)
		}
		st, _ := manager.WorktreeStatus(wt)
		if st.Branch != "task/TASK-211" {
			t.Errorf("restored worktree on branch %q, want task/TASK-211", st.Branch)
		}
	})

	t.Run("recreates vanished branch from base", func(t *testing.T) {
		manager, repoDir, _ := setup(t)
		wt, err := manager.CreateWorktree("TASK-212", repoDir, "main")
		if err != nil {
			t.Fatalf("CreateWorktree: %v", err)
		}
		if err := manager.RemoveWorktree(wt, true); err != nil {
			t.Fatalf("RemoveWorktree: %v", err)
		}
		if err := manager.RemoveBranch(repoDir, "task/TASK-212"); err != nil {
			t.Fatalf("RemoveBranch: %v", err)
		}
		if branchExists(t, repoDir, "task/TASK-212") {
			t.Fatalf("precondition: branch should be gone")
		}

		if _, err := manager.RestoreWorktree(repoDir, "task/TASK-212", wt, "main"); err != nil {
			t.Fatalf("RestoreWorktree: %v", err)
		}
		if _, err := os.Stat(wt); err != nil {
			t.Errorf("worktree not restored: %v", err)
		}
		if !branchExists(t, repoDir, "task/TASK-212") {
			t.Errorf("branch not recreated")
		}
	})
}

func TestListWorktrees(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	// Canonicalise the temp root before deriving any expected paths. On macOS
	// t.TempDir() hands back a /var/folders/... path, but that lives under the
	// /var → /private/var symlink, and ListWorktrees now returns EvalSymlinks-
	// resolved paths (matching what `git worktree list --porcelain` reports).
	// Resolving here keeps the expected paths in that same canonical space, so
	// the equality checks below hold on macOS instead of spuriously failing.
	if resolved, err := filepath.EvalSymlinks(tempDir); err == nil {
		tempDir = resolved
	}

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo with initial commit
	setupGitRepo(t, repoDir)

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create multiple worktrees
	taskIDs := []string{"TASK-003", "TASK-004"}
	for _, taskID := range taskIDs {
		if _, err := manager.CreateWorktree(taskID, repoDir, "main"); err != nil {
			t.Fatalf("CreateWorktree(%s) failed: %v", taskID, err)
		}
	}

	// List worktrees
	worktrees, err := manager.ListWorktrees(repoDir)
	if err != nil {
		t.Fatalf("ListWorktrees() failed: %v", err)
	}

	// Verify we have at least 3 worktrees (main repo + 2 task worktrees)
	if len(worktrees) < 3 {
		t.Errorf("Expected at least 3 worktrees, got %d", len(worktrees))
	}

	// Verify main repo is listed
	foundMain := false
	for _, wt := range worktrees {
		if wt.Path == repoDir {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Errorf("Main repository not found in worktree list")
	}

	// Verify task worktrees are listed
	for _, taskID := range taskIDs {
		expectedPath := filepath.Join(workBase, "work", taskID)
		found := false
		for _, wt := range worktrees {
			if wt.Path == expectedPath {
				found = true
				// Verify branch name
				expectedBranch := "refs/heads/task/" + taskID
				if wt.Branch != expectedBranch {
					t.Errorf("Expected branch %s for worktree %s, got %s", expectedBranch, taskID, wt.Branch)
				}
				// Verify commit is set
				if wt.Commit == "" {
					t.Errorf("Commit hash not set for worktree %s", taskID)
				}
				break
			}
		}
		if !found {
			t.Errorf("Worktree for %s not found in list", taskID)
		}
	}
}

func TestListWorktreesErrors(t *testing.T) {
	manager := NewGitWorktreeManager("")

	tests := []struct {
		name        string
		repoPath    string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Empty repo path",
			repoPath:    "",
			wantErr:     true,
			errContains: "repoPath cannot be empty",
		},
		{
			name:        "Non-git directory",
			repoPath:    os.TempDir(),
			wantErr:     true,
			errContains: "failed to list worktrees",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ListWorktrees(tt.repoPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ListWorktrees() expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ListWorktrees() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ListWorktrees() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGetWorktreeForTask(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo with initial commit
	setupGitRepo(t, repoDir)

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create worktree for a task
	taskID := "TASK-005"
	expectedPath, err := manager.CreateWorktree(taskID, repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	// Test GetWorktreeForTask with existing task
	path, exists, err := manager.GetWorktreeForTask(taskID)
	if err != nil {
		t.Errorf("GetWorktreeForTask() unexpected error: %v", err)
	}
	if !exists {
		t.Errorf("GetWorktreeForTask() exists = false, want true")
	}
	if path != expectedPath {
		t.Errorf("GetWorktreeForTask() path = %s, want %s", path, expectedPath)
	}

	// Test GetWorktreeForTask with non-existent task
	nonExistentTaskID := "TASK-999"
	path, exists, err = manager.GetWorktreeForTask(nonExistentTaskID)
	if err != nil {
		t.Errorf("GetWorktreeForTask() unexpected error: %v", err)
	}
	if exists {
		t.Errorf("GetWorktreeForTask() exists = true, want false")
	}
	if path != "" {
		t.Errorf("GetWorktreeForTask() path = %s, want empty string", path)
	}
}

func TestGetWorktreeForTaskErrors(t *testing.T) {
	manager := NewGitWorktreeManager("")

	// Test with empty taskID
	_, _, err := manager.GetWorktreeForTask("")
	if err == nil {
		t.Errorf("GetWorktreeForTask() expected error with empty taskID, got nil")
	}
	if !strings.Contains(err.Error(), "taskID cannot be empty") {
		t.Errorf("GetWorktreeForTask() error = %v, want error containing 'taskID cannot be empty'", err)
	}
}

func TestCreateWorktreeDefaultBaseBranch(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo with initial commit
	setupGitRepo(t, repoDir)

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create worktree with empty baseBranch (should default to "main")
	taskID := "TASK-006"
	_, err := manager.CreateWorktree(taskID, repoDir, "")
	if err != nil {
		t.Fatalf("CreateWorktree() with empty baseBranch failed: %v", err)
	}

	// Verify worktree was created
	expectedPath := filepath.Join(workBase, "work", taskID)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Worktree was not created with default baseBranch: %s", expectedPath)
	}
}

func TestCreateWorktreeAlreadyExists(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Initialize a git repo
	repoDir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo directory: %v", err)
	}

	// Initialize git repo with initial commit
	setupGitRepo(t, repoDir)

	// Create worktree manager
	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	// Create worktree
	taskID := "TASK-007"
	_, err := manager.CreateWorktree(taskID, repoDir, "main")
	if err != nil {
		t.Fatalf("CreateWorktree() failed: %v", err)
	}

	// Try to create worktree again with same taskID
	_, err = manager.CreateWorktree(taskID, repoDir, "main")
	if err == nil {
		t.Errorf("CreateWorktree() expected error when worktree already exists, got nil")
	}
	if !strings.Contains(err.Error(), "worktree already exists") {
		t.Errorf("CreateWorktree() error = %v, want error containing 'worktree already exists'", err)
	}
}

// gitIn runs a git command in dir and fails the test on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v: %s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestCreateWorktree_FetchesBeforeBranching is the regression test for the
// freshness gap: a worktree created after upstream advances must branch from
// the fetched origin/<base>, not the stale local clone. It also asserts
// ADB_NO_FETCH=1 falls back to the local base (no network).
func TestCreateWorktree_FetchesBeforeBranching(t *testing.T) {
	tempDir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(tempDir); err == nil {
		tempDir = resolved
	}

	// Bare "remote" with an initial commit on main.
	remote := filepath.Join(tempDir, "remote.git")
	gitIn(t, tempDir, "init", "--bare", "-b", "main", remote)

	// Seed the remote via a scratch clone.
	seed := filepath.Join(tempDir, "seed")
	gitIn(t, tempDir, "clone", remote, seed)
	gitIn(t, seed, "config", "user.email", "seed@test.local")
	gitIn(t, seed, "config", "user.name", "Seed")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, seed, "add", "README.md")
	gitIn(t, seed, "commit", "-m", "initial")
	gitIn(t, seed, "push", "origin", "main")

	// The clone adb would branch from. Capture its (now stale) HEAD.
	clone := filepath.Join(tempDir, "clone")
	gitIn(t, tempDir, "clone", remote, clone)
	staleHead := gitIn(t, clone, "rev-parse", "HEAD")

	// Advance upstream from the seed clone — the local `clone` does NOT
	// know about this commit until a fetch happens.
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, seed, "commit", "-am", "upstream advance")
	gitIn(t, seed, "push", "origin", "main")
	freshHead := gitIn(t, seed, "rev-parse", "HEAD")
	if staleHead == freshHead {
		t.Fatal("test setup error: upstream did not advance")
	}

	workBase := filepath.Join(tempDir, "workspace")
	manager := NewGitWorktreeManager(workBase)

	t.Run("default fetches and branches from fresh upstream", func(t *testing.T) {
		os.Unsetenv("ADB_NO_FETCH")
		wtPath, err := manager.CreateWorktree("TASK-100", clone, "main")
		if err != nil {
			t.Fatalf("CreateWorktree: %v", err)
		}
		got := gitIn(t, wtPath, "rev-parse", "HEAD")
		if got != freshHead {
			t.Errorf("worktree HEAD = %s, want fresh upstream %s (stale was %s)", got, freshHead, staleHead)
		}
	})

	t.Run("ADB_NO_FETCH=1 branches from stale local base", func(t *testing.T) {
		t.Setenv("ADB_NO_FETCH", "1")
		// Re-clone so the local base is stale again (independent of the
		// fetch the previous subtest performed).
		clone2 := filepath.Join(tempDir, "clone2")
		gitIn(t, tempDir, "clone", remote, clone2)
		// Make clone2's local main stale by resetting to the first commit,
		// while origin/main (cached at clone time) points at fresh.
		gitIn(t, clone2, "reset", "--hard", staleHead)

		wtPath, err := manager.CreateWorktree("TASK-101", clone2, "main")
		if err != nil {
			t.Fatalf("CreateWorktree: %v", err)
		}
		got := gitIn(t, wtPath, "rev-parse", "HEAD")
		if got != staleHead {
			t.Errorf("with ADB_NO_FETCH, worktree HEAD = %s, want local base %s", got, staleHead)
		}
	})
}

// setupGitRepo is a helper function to initialize a git repo with an initial commit
func setupGitRepo(t *testing.T, repoDir string) {
	t.Helper()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to init git repo: %v: %s", err, string(output))
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to config git user email: %v: %s", err, string(output))
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to config git user name: %v: %s", err, string(output))
	}

	// Create initial commit
	testFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to add file: %v: %s", err, string(output))
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to commit: %v: %s", err, string(output))
	}

	// Create main branch
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create main branch: %v: %s", err, string(output))
	}
}

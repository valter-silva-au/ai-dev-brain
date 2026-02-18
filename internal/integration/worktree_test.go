package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// setupTestGitRepo creates a git repo with an initial commit in the given directory.
func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "user.email", "test@example.com"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("running %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("running %v: %v\n%s", args, err, out)
		}
	}
}

// =============================================================================
// Unit tests: parseRepoPath
// =============================================================================

func TestParseRepoPath_Valid(t *testing.T) {
	tests := []struct {
		input                           string
		wantPlatform, wantOrg, wantRepo string
	}{
		{"github.com/org/repo", "github.com", "org", "repo"},
		{"gitlab.com/team/project", "gitlab.com", "team", "project"},
		{"code.aws.dev/myorg/myrepo", "code.aws.dev", "myorg", "myrepo"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			platform, org, repo, err := parseRepoPath(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if platform != tc.wantPlatform {
				t.Errorf("platform = %q, want %q", platform, tc.wantPlatform)
			}
			if org != tc.wantOrg {
				t.Errorf("org = %q, want %q", org, tc.wantOrg)
			}
			if repo != tc.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tc.wantRepo)
			}
		})
	}
}

func TestParseRepoPath_Invalid(t *testing.T) {
	tests := []string{
		"",
		"github.com",
		"github.com/only-org",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, _, _, err := parseRepoPath(input)
			if err == nil {
				t.Fatal("expected error for invalid repo path, got nil")
			}
		})
	}
}

func TestParseRepoPath_TrailingSlash(t *testing.T) {
	platform, org, repo, err := parseRepoPath("github.com/org/repo/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != "github.com" || org != "org" || repo != "repo" {
		t.Errorf("got (%q, %q, %q), want (github.com, org, repo)", platform, org, repo)
	}
}

// =============================================================================
// Unit tests: worktreePath
// =============================================================================

func TestWorktreePath_Structure(t *testing.T) {
	mgr := &gitWorktreeManager{basePath: "/home/user/.adb"}

	path := mgr.worktreePath("TASK-00001")

	expected := filepath.Join("/home/user/.adb", "work", "TASK-00001")
	if path != expected {
		t.Errorf("worktreePath = %q, want %q", path, expected)
	}
}

// =============================================================================
// Unit tests: parseWorktreeListOutput
// =============================================================================

func TestParseWorktreeListOutput_Basic(t *testing.T) {
	output := `worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /base/repos/github.com/org/repo/work/TASK-00001
HEAD def456
branch refs/heads/feat/task-00001
`

	worktrees := parseWorktreeListOutput(output, "/repo")
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}

	// First worktree: main (no task ID since parent dir is not "work").
	if worktrees[0].Branch != "main" {
		t.Errorf("worktrees[0].Branch = %q, want %q", worktrees[0].Branch, "main")
	}
	if worktrees[0].TaskID != "" {
		t.Errorf("worktrees[0].TaskID = %q, want empty", worktrees[0].TaskID)
	}

	// Second worktree: task worktree.
	if worktrees[1].Branch != "feat/task-00001" {
		t.Errorf("worktrees[1].Branch = %q, want %q", worktrees[1].Branch, "feat/task-00001")
	}
	if worktrees[1].TaskID != "TASK-00001" {
		t.Errorf("worktrees[1].TaskID = %q, want %q", worktrees[1].TaskID, "TASK-00001")
	}
}

func TestParseWorktreeListOutput_Empty(t *testing.T) {
	worktrees := parseWorktreeListOutput("", "/repo")
	if len(worktrees) != 0 {
		t.Errorf("got %d worktrees for empty output, want 0", len(worktrees))
	}
}

func TestParseWorktreeListOutput_PathBasedTaskID(t *testing.T) {
	output := `worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /base/work/github.com/org/finance/new-feature
HEAD def456
branch refs/heads/feat/finance/new-feature
`

	worktrees := parseWorktreeListOutput(output, "/repo")
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}

	// Second worktree should have the full path-based task ID.
	if worktrees[1].TaskID != "github.com/org/finance/new-feature" {
		t.Errorf("worktrees[1].TaskID = %q, want %q", worktrees[1].TaskID, "github.com/org/finance/new-feature")
	}
	if worktrees[1].Branch != "feat/finance/new-feature" {
		t.Errorf("worktrees[1].Branch = %q, want %q", worktrees[1].Branch, "feat/finance/new-feature")
	}
}

func TestParseWorktreeListOutput_ShortPrefixTaskID(t *testing.T) {
	output := `worktree /base/work/finance/add-auth
HEAD abc123
branch refs/heads/feat/finance/add-auth
`

	worktrees := parseWorktreeListOutput(output, "/repo")
	if len(worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(worktrees))
	}

	if worktrees[0].TaskID != "finance/add-auth" {
		t.Errorf("worktrees[0].TaskID = %q, want %q", worktrees[0].TaskID, "finance/add-auth")
	}
}

func TestParseWorktreeListOutput_Bare(t *testing.T) {
	output := `worktree /repo
HEAD abc123
branch refs/heads/main
bare
`
	worktrees := parseWorktreeListOutput(output, "/repo")
	if len(worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(worktrees))
	}
	if worktrees[0].Path != "/repo" {
		t.Errorf("Path = %q, want %q", worktrees[0].Path, "/repo")
	}
}

// =============================================================================
// Unit tests: CreateWorktree validation
// =============================================================================

func TestCreateWorktree_EmptyRepoPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   "",
		BranchName: "feat/test",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Fatal("expected error for empty RepoPath")
	}
}

func TestCreateWorktree_EmptyTaskID_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   "github.com/org/repo",
		BranchName: "feat/test",
		TaskID:     "",
	})
	if err == nil {
		t.Fatal("expected error for empty TaskID")
	}
}

func TestCreateWorktree_EmptyBranchName_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   "github.com/org/repo",
		BranchName: "",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Fatal("expected error for empty BranchName")
	}
}

func TestCreateWorktree_InvalidRepoPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   "invalid",
		BranchName: "feat/test",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Fatal("expected error for invalid RepoPath")
	}
}

func TestCreateWorktree_LocalAbsRepoPath_Success(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	baseDir := filepath.Join(tmp, "adb")
	mgr := NewGitWorktreeManager(baseDir)

	wtPath, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/test-branch",
		TaskID:     "TASK-00001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(baseDir, "work", "TASK-00001")
	if wtPath != expectedPath {
		t.Errorf("worktree path = %q, want %q", wtPath, expectedPath)
	}

	// Verify worktree directory exists.
	if info, statErr := os.Stat(wtPath); statErr != nil || !info.IsDir() {
		t.Error("worktree directory does not exist")
	}
}

func TestCreateWorktree_LocalAbsRepoPath_WithBaseBranch(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	baseDir := filepath.Join(tmp, "adb")
	mgr := NewGitWorktreeManager(baseDir)

	wtPath, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/from-main",
		TaskID:     "TASK-00002",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(baseDir, "work", "TASK-00002")
	if wtPath != expectedPath {
		t.Errorf("worktree path = %q, want %q", wtPath, expectedPath)
	}
}

func TestCreateWorktree_GitWorktreeAddFails(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	baseDir := filepath.Join(tmp, "adb")
	mgr := NewGitWorktreeManager(baseDir)

	// Create the first worktree.
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/branch-a",
		TaskID:     "TASK-00001",
	})
	if err != nil {
		t.Fatalf("first worktree creation failed: %v", err)
	}

	// Creating a second worktree with the same branch name should fail.
	_, err = mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/branch-a",
		TaskID:     "TASK-00002",
	})
	if err == nil {
		t.Fatal("expected error for duplicate branch name")
	}
	if !strings.Contains(err.Error(), "git worktree add failed") {
		t.Errorf("error = %q, want to contain 'git worktree add failed'", err.Error())
	}
}

// =============================================================================
// Unit tests: RemoveWorktree validation
// =============================================================================

func TestRemoveWorktree_EmptyPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	err := mgr.RemoveWorktree("")
	if err == nil {
		t.Fatal("expected error for empty worktree path")
	}
}

// =============================================================================
// Unit tests: ListWorktrees validation
// =============================================================================

func TestListWorktrees_EmptyRepoPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.ListWorktrees("")
	if err == nil {
		t.Fatal("expected error for empty repoPath")
	}
}

// =============================================================================
// Unit tests: GetWorktreeForTask validation
// =============================================================================

func TestGetWorktreeForTask_EmptyTaskID_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.GetWorktreeForTask("")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestGetWorktreeForTask_NotFound_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.GetWorktreeForTask("TASK-99999")
	if err == nil {
		t.Fatal("expected error when no worktree matches")
	}
}

// =============================================================================
// Property 12: Multi-Repository Worktree Creation
// =============================================================================

// Feature: ai-dev-brain, Property 12: Worktree Path Consistency
// *For any* task, the worktree path SHALL be basePath/work/{taskID} regardless
// of which repository is used.
//
// **Validates: Requirements 9.1**
//
// We test that the worktree path is always basePath/work/{taskID} and that
// different task IDs produce different paths.
func TestProperty12_WorktreePathConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		basePath := "/home/user/.adb"
		mgr := &gitWorktreeManager{basePath: basePath}

		numTasks := rapid.IntRange(1, 5).Draw(rt, "numTasks")

		seen := map[string]bool{}
		paths := make(map[string]bool, numTasks)

		for i := 0; i < numTasks; i++ {
			taskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(1, 99999).Draw(rt, fmt.Sprintf("taskNum_%d", i)))
			if seen[taskID] {
				continue
			}
			seen[taskID] = true

			p := mgr.worktreePath(taskID)

			// Verify path structure: basePath/work/{taskID}.
			expectedFull := filepath.Join(basePath, "work", taskID)
			if p != expectedFull {
				rt.Errorf("path = %q, want %q", p, expectedFull)
			}

			if paths[p] {
				rt.Fatalf("duplicate worktree path %q for different tasks", p)
			}
			paths[p] = true
		}

		// Exactly len(seen) distinct paths.
		if len(paths) != len(seen) {
			rt.Errorf("expected %d distinct paths, got %d", len(seen), len(paths))
		}
	})
}

// =============================================================================
// Property 13: Repository Path Structure
// =============================================================================

// Feature: ai-dev-brain, Property 13: Worktree Path Structure
// *For any* task ID, the worktree path SHALL follow the structure:
// basePath/work/TASK-XXXXX, independent of the repository.
//
// **Validates: Requirements 9.4**
func TestProperty13_WorktreePathStructure(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Use t.TempDir() as a base to avoid OS-specific path separator issues.
		baseDir := t.TempDir()
		subDir := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "subDir")
		basePath := filepath.Join(baseDir, subDir)
		mgr := &gitWorktreeManager{basePath: basePath}

		taskNum := rapid.IntRange(1, 99999).Draw(rt, "taskNum")
		taskID := fmt.Sprintf("TASK-%05d", taskNum)

		// Generate path.
		p := mgr.worktreePath(taskID)

		// Verify the path starts with basePath.
		if !strings.HasPrefix(p, basePath) {
			rt.Errorf("path %q does not start with basePath %q", p, basePath)
		}

		// Verify path contains the expected directory hierarchy.
		expectedSuffix := filepath.Join("work", taskID)
		if !strings.HasSuffix(p, expectedSuffix) {
			rt.Errorf("path %q does not end with expected %q", p, expectedSuffix)
		}

		// Verify full expected path matches exactly.
		expectedFull := filepath.Join(basePath, "work", taskID)
		if p != expectedFull {
			rt.Errorf("path = %q, want %q", p, expectedFull)
		}

		// Verify parseRepoPath still works correctly for repo path parsing.
		platforms := []string{"github.com", "gitlab.com", "code.aws.dev"}
		platform := platforms[rapid.IntRange(0, len(platforms)-1).Draw(rt, "platform")]
		org := rapid.StringMatching(`[a-z][a-z0-9]{1,10}`).Draw(rt, "org")
		repo := rapid.StringMatching(`[a-z][a-z0-9]{1,10}`).Draw(rt, "repo")
		repoPathInput := platform + "/" + org + "/" + repo
		gotPlatform, gotOrg, gotRepo, err := parseRepoPath(repoPathInput)
		if err != nil {
			rt.Fatalf("parseRepoPath(%q) failed: %v", repoPathInput, err)
		}
		if gotPlatform != platform || gotOrg != org || gotRepo != repo {
			rt.Errorf("parseRepoPath(%q) = (%q, %q, %q), want (%q, %q, %q)",
				repoPathInput, gotPlatform, gotOrg, gotRepo, platform, org, repo)
		}
	})
}

// =============================================================================
// Unit tests: NormalizeRepoPath edge cases
// =============================================================================

func TestNormalizeRepoPath_SSHColonSeparator(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com:org/repo", "github.com/org/repo"},
		{"git@github.com:org/repo.git", "github.com/org/repo"},
		{"https://github.com/org/repo.git", "github.com/org/repo"},
		{"http://github.com/org/repo", "github.com/org/repo"},
		{"repos/github.com/org/repo", "github.com/org/repo"},
		{"  github.com/org/repo  ", "github.com/org/repo"},
		{`github.com\org\repo`, "github.com/org/repo"},
		{"github.com/org/repo/", "github.com/org/repo"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeRepoPath(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeRepoPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// =============================================================================
// Unit tests: repoURLFromPath and repoSSHURLFromPath
// =============================================================================

func TestRepoURLFromPath(t *testing.T) {
	got := repoURLFromPath("github.com/org/repo")
	want := "https://github.com/org/repo.git"
	if got != want {
		t.Errorf("repoURLFromPath = %q, want %q", got, want)
	}
}

func TestRepoSSHURLFromPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/org/repo", "git@github.com:org/repo.git"},
		{`github.com\org\repo`, "git@github.com:org/repo.git"},
		{"singlepart", "git@singlepart"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := repoSSHURLFromPath(tc.input)
			if got != tc.want {
				t.Errorf("repoSSHURLFromPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// =============================================================================
// Unit tests: detectDefaultBranch
// =============================================================================

func TestDetectDefaultBranch_WithMain(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	// Create a bare remote and push main to it.
	bareDir := filepath.Join(tmp, "remote.git")
	cmd := exec.Command("git", "init", "--bare", "-b", "main")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}

	// Add remote and push.
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	got := detectDefaultBranch(repoDir)
	if got != "main" {
		t.Errorf("detectDefaultBranch = %q, want %q", got, "main")
	}
}

func TestDetectDefaultBranch_NoRemote(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	// No origin remote, so detectDefaultBranch should return empty.
	got := detectDefaultBranch(repoDir)
	if got != "" {
		t.Errorf("detectDefaultBranch = %q, want empty string", got)
	}
}

// =============================================================================
// Unit tests: RemoveWorktree
// =============================================================================

func TestRemoveWorktree_NonExistentPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	err := mgr.RemoveWorktree("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for non-existent worktree path")
	}
	if !strings.Contains(err.Error(), "git worktree remove failed") {
		t.Errorf("error = %q, want to contain 'git worktree remove failed'", err.Error())
	}
}

func TestRemoveWorktree_Success(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	// Create a worktree using git directly from the repo directory.
	wtPath := filepath.Join(tmp, "worktree")
	cmd := exec.Command("git", "worktree", "add", "-b", "feat/to-remove", wtPath)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// Verify the worktree exists and has a .git file pointing back.
	gitFile := filepath.Join(wtPath, ".git")
	if _, statErr := os.Stat(gitFile); os.IsNotExist(statErr) {
		t.Fatal("expected .git file in worktree")
	}

	// Remove the worktree using RemoveWorktree. The implementation runs
	// `git worktree remove <path>` without setting Dir, which works because
	// git resolves the parent repo from the .git pointer in the worktree.
	// However, this requires the cwd to be within a git repo, so we change
	// to the repo directory for this test.
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(repoDir)

	mgr := NewGitWorktreeManager(tmp)
	if err := mgr.RemoveWorktree(wtPath); err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	// Verify directory is gone.
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Error("expected worktree directory to be removed")
	}
}

// =============================================================================
// Unit tests: ListWorktrees
// =============================================================================

func TestListWorktrees_ValidRepo(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	baseDir := filepath.Join(tmp, "adb")
	mgr := NewGitWorktreeManager(baseDir)

	// Create a worktree.
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/list-test",
		TaskID:     "TASK-00020",
	})
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	// List worktrees from the repo.
	worktrees, listErr := mgr.ListWorktrees(repoDir)
	if listErr != nil {
		t.Fatalf("ListWorktrees failed: %v", listErr)
	}

	// Should have at least 2 worktrees (main + the one we created).
	if len(worktrees) < 2 {
		t.Errorf("expected at least 2 worktrees, got %d", len(worktrees))
	}

	// Find the one we created.
	found := false
	for _, wt := range worktrees {
		if wt.Branch == "feat/list-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find worktree with branch 'feat/list-test'")
	}
}

func TestListWorktrees_InvalidRepoPath_ReturnsError(t *testing.T) {
	mgr := NewGitWorktreeManager(t.TempDir())
	_, err := mgr.ListWorktrees("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for invalid repo path")
	}
	if !strings.Contains(err.Error(), "git worktree list failed") {
		t.Errorf("error = %q, want to contain 'git worktree list failed'", err.Error())
	}
}

// =============================================================================
// Unit tests: GetWorktreeForTask (success path)
// =============================================================================

func TestGetWorktreeForTask_Found(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	baseDir := filepath.Join(tmp, "adb")
	mgr := NewGitWorktreeManager(baseDir)

	// Create a worktree.
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   repoDir,
		BranchName: "feat/find-me",
		TaskID:     "TASK-00030",
	})
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	wt, findErr := mgr.GetWorktreeForTask("TASK-00030")
	if findErr != nil {
		t.Fatalf("GetWorktreeForTask failed: %v", findErr)
	}
	if wt.TaskID != "TASK-00030" {
		t.Errorf("TaskID = %q, want %q", wt.TaskID, "TASK-00030")
	}
	expectedPath := filepath.Join(baseDir, "work", "TASK-00030")
	if wt.Path != expectedPath {
		t.Errorf("Path = %q, want %q", wt.Path, expectedPath)
	}
}

func TestGetWorktreeForTask_FileNotDir_ReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	// Create a file (not a directory) at the expected worktree path.
	workDir := filepath.Join(baseDir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "TASK-00001"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewGitWorktreeManager(baseDir)
	_, err := mgr.GetWorktreeForTask("TASK-00001")
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

// =============================================================================
// Unit tests: ensureRepoReady
// =============================================================================

func TestEnsureRepoReady_ExistingRepoWithCommits(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")
	setupTestGitRepo(t, repoDir)

	// Create a bare remote and add it as origin.
	bareDir := filepath.Join(tmp, "remote.git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--bare", "-b", "main")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	mgr := &gitWorktreeManager{basePath: tmp}
	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err != nil {
		t.Fatalf("ensureRepoReady failed: %v", err)
	}
}

func TestEnsureRepoReady_NoGitDir_CloneFails(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "nonexistent-repo")

	mgr := &gitWorktreeManager{basePath: tmp}
	// Clone will fail because there's no real remote.
	err := mgr.ensureRepoReady(repoDir, "invalid.example.com/no/repo")
	if err == nil {
		t.Fatal("expected error when clone fails")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Errorf("error = %q, want to contain 'git clone failed'", err.Error())
	}
}

func TestEnsureRepoReady_EmptyRepoNoCommits(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")

	// Create a git repo with no commits.
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	// Add an origin remote (pointing to a bare repo that also has no commits).
	bareDir := filepath.Join(tmp, "remote.git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "init", "--bare", "-b", "main")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	mgr := &gitWorktreeManager{basePath: tmp}
	// Should not error -- empty repo with no branches from remote is OK.
	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err != nil {
		t.Fatalf("ensureRepoReady failed: %v", err)
	}
}

func TestEnsureRepoReady_EmptyRepoWithRemoteBranch(t *testing.T) {
	tmp := t.TempDir()

	// Create a bare remote with a commit.
	bareDir := filepath.Join(tmp, "remote.git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--bare", "-b", "main")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}

	// Seed the bare remote with a commit.
	seedDir := filepath.Join(tmp, "seed")
	setupTestGitRepo(t, seedDir)
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = seedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = seedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	// Create an empty git repo pointing to the bare remote.
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "init", "-b", "orphan")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	mgr := &gitWorktreeManager{basePath: tmp}
	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err != nil {
		t.Fatalf("ensureRepoReady failed: %v", err)
	}

	// After ensureRepoReady, the main branch should be checked out.
	cmd = exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Error("expected 'main' branch to exist after ensureRepoReady")
	}
}

// =============================================================================
// Unit tests: CreateWorktree with non-absolute repo path
// =============================================================================

func TestCreateWorktree_NonAbsRepoPath_EnsureRepoReadyFails(t *testing.T) {
	baseDir := t.TempDir()
	mgr := NewGitWorktreeManager(baseDir)

	// Use a repo path that will pass parseRepoPath but fail ensureRepoReady
	// because there's no actual repo to clone.
	_, err := mgr.CreateWorktree(WorktreeConfig{
		RepoPath:   "invalid.example.com/org/repo",
		BranchName: "feat/test",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Fatal("expected error when ensureRepoReady fails")
	}
	if !strings.Contains(err.Error(), "preparing repository") {
		t.Errorf("error = %q, want to contain 'preparing repository'", err.Error())
	}
}

func TestEnsureRepoReady_ParentDirCreationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	tmp := t.TempDir()

	// Make the repos directory read-only so MkdirAll fails.
	reposDir := filepath.Join(tmp, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(reposDir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(reposDir, 0o755) }()

	mgr := &gitWorktreeManager{basePath: tmp}
	repoDir := filepath.Join(reposDir, "github.com", "org", "repo")

	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err == nil {
		t.Fatal("expected error when parent directory creation fails")
	}
	if !strings.Contains(err.Error(), "creating parent directory") {
		t.Errorf("error = %q, want to contain 'creating parent directory'", err.Error())
	}
}

func TestEnsureRepoReady_CheckoutFails(t *testing.T) {
	tmp := t.TempDir()

	// Create a bare remote with a commit.
	bareDir := filepath.Join(tmp, "remote.git")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--bare", "-b", "main")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}

	// Seed it.
	seedDir := filepath.Join(tmp, "seed")
	setupTestGitRepo(t, seedDir)
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = seedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = seedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	// Create an empty git repo pointing to the bare remote.
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "init", "-b", "orphan")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	// Fetch so origin/main exists.
	cmd = exec.Command("git", "fetch", "origin")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git fetch: %v\n%s", err, out)
	}
	// Now create a local branch "main" so the checkout -b main will fail
	// (branch already exists).
	cmd = exec.Command("git", "checkout", "-b", "main", "origin/main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, out)
	}
	// Delete the branch ref file so rev-parse HEAD fails (to trigger the no-commits path),
	// but we already have a main branch locally, which will cause checkout -b main to fail.
	// Actually, let's take a different approach: just make HEAD point to a non-existent ref.
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/nonexistent")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git symbolic-ref: %v\n%s", err, out)
	}

	// Now ensureRepoReady should:
	// 1. See .git exists
	// 2. rev-parse HEAD fails (no such ref)
	// 3. fetch origin (succeeds, already fetched)
	// 4. detectDefaultBranch returns "main"
	// 5. checkout -b main origin/main fails because "main" branch already exists
	mgr := &gitWorktreeManager{basePath: tmp}
	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err == nil {
		t.Fatal("expected error when checkout fails for existing branch")
	}
	if !strings.Contains(err.Error(), "git checkout") {
		t.Errorf("error = %q, want to contain 'git checkout'", err.Error())
	}
}

func TestEnsureRepoReady_FetchError_NoCommits(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "repo")

	// Create a git repo with no commits and no valid origin.
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// Add a bogus origin that will fail to fetch.
	cmd = exec.Command("git", "remote", "add", "origin", "/nonexistent/path")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	mgr := &gitWorktreeManager{basePath: tmp}
	err := mgr.ensureRepoReady(repoDir, "github.com/org/repo")
	if err == nil {
		t.Fatal("expected error when fetch fails for empty repo")
	}
	if !strings.Contains(err.Error(), "git fetch origin failed") {
		t.Errorf("error = %q, want to contain 'git fetch origin failed'", err.Error())
	}
}

package integration

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// =============================================================================
// Unit tests: parseRepoPath
// =============================================================================

func TestParseRepoPath_Valid(t *testing.T) {
	tests := []struct {
		input                          string
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

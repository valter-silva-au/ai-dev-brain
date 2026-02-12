package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// =============================================================================
// Test helpers
// =============================================================================

// setupTestRepo initialises a git repository in dir with an initial commit.
// Returns the absolute path to the repository.
func setupTestRepo(t *testing.T, dir string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating test repo dir: %v", err)
	}

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "user.email", "test@example.com"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("running %v: %v\n%s", args, err, out)
		}
	}

	// Create a file and commit it.
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

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolving abs path: %v", err)
	}
	return abs
}

// setupBareRemote creates a bare git repository in dir.
// Returns the absolute path to the bare repository.
func setupBareRemote(t *testing.T, dir string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating bare remote dir: %v", err)
	}

	cmd := exec.Command("git", "init", "--bare", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolving abs path: %v", err)
	}
	return abs
}

// gitRun runs a git command in the given directory and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// cloneFromBare clones a bare remote into dir, configures user, and returns
// the absolute path.
func cloneFromBare(t *testing.T, bare, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "clone", bare, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	gitRun(t, dir, "config", "user.name", "Test User")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolving abs path: %v", err)
	}
	return abs
}

// =============================================================================
// Tests
// =============================================================================

func TestSyncAll_NoReposDir(t *testing.T) {
	base := t.TempDir()
	// No repos/ directory created at all.
	mgr := NewRepoSyncManager(base)

	results, err := mgr.SyncAll(nil)
	if err != nil {
		t.Fatalf("SyncAll returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSyncAll_EmptyReposDir(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "repos"), 0o755); err != nil {
		t.Fatalf("creating repos dir: %v", err)
	}
	mgr := NewRepoSyncManager(base)

	results, err := mgr.SyncAll(nil)
	if err != nil {
		t.Fatalf("SyncAll returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSyncAll_DiscoversRepos(t *testing.T) {
	base := t.TempDir()
	reposDir := filepath.Join(base, "repos")

	// Create three repos with the platform/org/repo structure.
	repoPaths := []string{
		filepath.Join(reposDir, "github.com", "orgA", "repo1"),
		filepath.Join(reposDir, "github.com", "orgA", "repo2"),
		filepath.Join(reposDir, "gitlab.com", "orgB", "repo3"),
	}

	for _, rp := range repoPaths {
		setupTestRepo(t, rp)
	}

	mgr := NewRepoSyncManager(base)
	results, err := mgr.SyncAll(nil)
	if err != nil {
		t.Fatalf("SyncAll returned error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Collect relative paths from results and verify all repos were discovered.
	var relPaths []string
	for _, r := range results {
		relPaths = append(relPaths, r.RepoPath)
	}
	sort.Strings(relPaths)

	expected := []string{
		"github.com/orgA/repo1",
		"github.com/orgA/repo2",
		"gitlab.com/orgB/repo3",
	}
	sort.Strings(expected)

	for i, want := range expected {
		if relPaths[i] != want {
			t.Errorf("result[%d].RepoPath = %q, want %q", i, relPaths[i], want)
		}
	}
}

func TestSyncRepo_FetchAndPrune(t *testing.T) {
	tmp := t.TempDir()

	// Set up a bare remote with an initial commit.
	bareDir := filepath.Join(tmp, "remote.git")
	bare := setupBareRemote(t, bareDir)

	// Create a "seed" repo, push initial commit to the bare remote.
	seedDir := filepath.Join(tmp, "seed")
	setupTestRepo(t, seedDir)
	gitRun(t, seedDir, "remote", "add", "origin", bare)
	gitRun(t, seedDir, "push", "-u", "origin", "main")

	// Clone from the bare remote into the actual test repo.
	cloneDir := filepath.Join(tmp, "clone")
	clonePath := cloneFromBare(t, bare, cloneDir)

	// Create a new branch on the remote (via the seed repo).
	gitRun(t, seedDir, "checkout", "-b", "feature-x")
	if err := os.WriteFile(filepath.Join(seedDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, seedDir, "add", ".")
	gitRun(t, seedDir, "commit", "-m", "add feature")
	gitRun(t, seedDir, "push", "origin", "feature-x")

	// Before sync, the clone should NOT know about origin/feature-x.
	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(clonePath, "clone", nil)

	if result.Error != nil {
		t.Fatalf("SyncRepo returned error: %v", result.Error)
	}
	if !result.Fetched {
		t.Error("expected Fetched to be true")
	}

	// After fetch, origin/feature-x should exist in the clone.
	verifyCmd := exec.Command("git", "-C", clonePath, "rev-parse", "--verify", "origin/feature-x")
	if err := verifyCmd.Run(); err != nil {
		t.Error("expected origin/feature-x to exist after fetch")
	}
}

func TestSyncRepo_DeletesMergedBranches(t *testing.T) {
	tmp := t.TempDir()

	// Set up bare remote and clone.
	bareDir := filepath.Join(tmp, "remote.git")
	bare := setupBareRemote(t, bareDir)

	seedDir := filepath.Join(tmp, "seed")
	setupTestRepo(t, seedDir)
	gitRun(t, seedDir, "remote", "add", "origin", bare)
	gitRun(t, seedDir, "push", "-u", "origin", "main")

	cloneDir := filepath.Join(tmp, "clone")
	clonePath := cloneFromBare(t, bare, cloneDir)

	// Set origin/HEAD so detectDefaultBranchFromHead works.
	gitRun(t, clonePath, "remote", "set-head", "origin", "main")

	// Create a branch and merge it into main.
	gitRun(t, clonePath, "checkout", "-b", "merged-branch")
	if err := os.WriteFile(filepath.Join(clonePath, "merged.txt"), []byte("merged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, clonePath, "add", ".")
	gitRun(t, clonePath, "commit", "-m", "merged branch commit")
	gitRun(t, clonePath, "checkout", "main")
	gitRun(t, clonePath, "merge", "merged-branch")

	// Verify branch exists before sync.
	branches := gitRun(t, clonePath, "branch")
	if !strings.Contains(branches, "merged-branch") {
		t.Fatal("expected merged-branch to exist before sync")
	}

	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(clonePath, "clone", nil)
	if result.Error != nil {
		t.Fatalf("SyncRepo returned error: %v", result.Error)
	}

	// The merged branch should have been deleted.
	found := false
	for _, b := range result.BranchesDeleted {
		if b == "merged-branch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected merged-branch to be deleted, got BranchesDeleted=%v", result.BranchesDeleted)
	}

	// Verify branch is gone.
	branchesAfter := gitRun(t, clonePath, "branch")
	if strings.Contains(branchesAfter, "merged-branch") {
		t.Error("merged-branch still exists after sync")
	}
}

func TestSyncRepo_PreservesUnmergedBranches(t *testing.T) {
	tmp := t.TempDir()

	// Set up bare remote and clone.
	bareDir := filepath.Join(tmp, "remote.git")
	bare := setupBareRemote(t, bareDir)

	seedDir := filepath.Join(tmp, "seed")
	setupTestRepo(t, seedDir)
	gitRun(t, seedDir, "remote", "add", "origin", bare)
	gitRun(t, seedDir, "push", "-u", "origin", "main")

	cloneDir := filepath.Join(tmp, "clone")
	clonePath := cloneFromBare(t, bare, cloneDir)

	// Set origin/HEAD so detectDefaultBranchFromHead works.
	gitRun(t, clonePath, "remote", "set-head", "origin", "main")

	// Create a branch with unique commits NOT merged into main.
	gitRun(t, clonePath, "checkout", "-b", "unmerged-branch")
	if err := os.WriteFile(filepath.Join(clonePath, "unmerged.txt"), []byte("unmerged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, clonePath, "add", ".")
	gitRun(t, clonePath, "commit", "-m", "unmerged commit")
	gitRun(t, clonePath, "checkout", "main")

	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(clonePath, "clone", nil)
	if result.Error != nil {
		t.Fatalf("SyncRepo returned error: %v", result.Error)
	}

	// Unmerged branch should NOT be deleted.
	for _, b := range result.BranchesDeleted {
		if b == "unmerged-branch" {
			t.Errorf("unmerged-branch was deleted but should have been preserved")
		}
	}

	// Verify branch still exists.
	branches := gitRun(t, clonePath, "branch")
	if !strings.Contains(branches, "unmerged-branch") {
		t.Error("unmerged-branch no longer exists, but should have been preserved")
	}
}

func TestSyncRepo_HandlesOrphanedHead(t *testing.T) {
	tmp := t.TempDir()

	// Set up bare remote and clone.
	bareDir := filepath.Join(tmp, "remote.git")
	bare := setupBareRemote(t, bareDir)

	seedDir := filepath.Join(tmp, "seed")
	setupTestRepo(t, seedDir)
	gitRun(t, seedDir, "remote", "add", "origin", bare)
	gitRun(t, seedDir, "push", "-u", "origin", "main")

	cloneDir := filepath.Join(tmp, "clone")
	clonePath := cloneFromBare(t, bare, cloneDir)

	// Set origin/HEAD so detectDefaultBranchFromHead works.
	gitRun(t, clonePath, "remote", "set-head", "origin", "main")

	// Point HEAD to a non-existent branch via symbolic-ref.
	gitRun(t, clonePath, "symbolic-ref", "HEAD", "refs/heads/nonexistent-branch")

	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(clonePath, "clone", nil)
	if result.Error != nil {
		t.Fatalf("SyncRepo returned error: %v", result.Error)
	}

	// The default branch should have been detected.
	if result.DefaultBranch != "main" {
		t.Errorf("expected DefaultBranch = 'main', got %q", result.DefaultBranch)
	}

	// HEAD should now point to main (orphaned head recovered).
	head := gitRun(t, clonePath, "symbolic-ref", "HEAD")
	if head != "refs/heads/main" {
		t.Errorf("HEAD = %q, expected refs/heads/main", head)
	}
}

func TestSyncRepo_SkipsProtectedBranches(t *testing.T) {
	tmp := t.TempDir()

	// Set up bare remote and clone.
	bareDir := filepath.Join(tmp, "remote.git")
	bare := setupBareRemote(t, bareDir)

	seedDir := filepath.Join(tmp, "seed")
	setupTestRepo(t, seedDir)
	gitRun(t, seedDir, "remote", "add", "origin", bare)
	gitRun(t, seedDir, "push", "-u", "origin", "main")

	cloneDir := filepath.Join(tmp, "clone")
	clonePath := cloneFromBare(t, bare, cloneDir)

	gitRun(t, clonePath, "remote", "set-head", "origin", "main")

	// Create two branches and merge both into main.
	for _, branch := range []string{"task-branch", "stale-branch"} {
		gitRun(t, clonePath, "checkout", "-b", branch)
		if err := os.WriteFile(filepath.Join(clonePath, branch+".txt"), []byte(branch+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, clonePath, "add", ".")
		gitRun(t, clonePath, "commit", "-m", "commit on "+branch)
		gitRun(t, clonePath, "checkout", "main")
		gitRun(t, clonePath, "merge", branch)
	}

	// Protect "task-branch" (simulating an active backlog task).
	protected := map[string]bool{"task-branch": true}

	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(clonePath, "clone", protected)
	if result.Error != nil {
		t.Fatalf("SyncRepo returned error: %v", result.Error)
	}

	// task-branch should be skipped, not deleted.
	for _, b := range result.BranchesDeleted {
		if b == "task-branch" {
			t.Error("task-branch was deleted but should have been protected")
		}
	}
	foundSkipped := false
	for _, b := range result.BranchesSkipped {
		if b == "task-branch" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Errorf("expected task-branch in BranchesSkipped, got %v", result.BranchesSkipped)
	}

	// stale-branch should be deleted.
	foundDeleted := false
	for _, b := range result.BranchesDeleted {
		if b == "stale-branch" {
			foundDeleted = true
		}
	}
	if !foundDeleted {
		t.Errorf("expected stale-branch in BranchesDeleted, got %v", result.BranchesDeleted)
	}
}

func TestSyncRepo_NonExistentPath(t *testing.T) {
	tmp := t.TempDir()

	mgr := NewRepoSyncManager(tmp)
	result := mgr.SyncRepo(filepath.Join(tmp, "does-not-exist"), "does-not-exist", nil)

	if result.Error == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

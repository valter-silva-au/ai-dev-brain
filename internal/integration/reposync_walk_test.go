package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeBareOrigin creates a bare repo at bareDir with an initial commit on
// its default branch. Returns the bare repo path.
func makeBareOrigin(t *testing.T, bareDir string, defaultBranch string) {
	t.Helper()
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	mustGit(t, "", "init", "--bare", "--initial-branch="+defaultBranch, bareDir)

	// Seed via a scratch working clone.
	scratch := t.TempDir()
	mustGit(t, "", "clone", bareDir, scratch)
	mustGit(t, scratch, "config", "user.email", "test@example.com")
	mustGit(t, scratch, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(scratch, "README.md"), "seed\n")
	mustGit(t, scratch, "add", "README.md")
	mustGit(t, scratch, "commit", "-m", "seed")
	mustGit(t, scratch, "push", "origin", defaultBranch)
}

// cloneFrom clones bareDir to workDir, configures identity, sets
// origin/HEAD so `symbolic-ref refs/remotes/origin/HEAD` works, and
// returns the clone path.
func cloneFrom(t *testing.T, bareDir, workDir, defaultBranch string) string {
	t.Helper()
	mustGit(t, "", "clone", bareDir, workDir)
	mustGit(t, workDir, "config", "user.email", "test@example.com")
	mustGit(t, workDir, "config", "user.name", "Test User")
	// Ensure origin/HEAD points at the default branch (not always set by old git).
	mustGit(t, workDir, "remote", "set-head", "origin", defaultBranch)
	return workDir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q failed: %v\n%s", args, dir, err, string(out))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// gitAvailable reports whether `git` is on PATH.
func gitAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
		return false
	}
	return true
}

func TestFindGitRepos_Nested(t *testing.T) {
	if !gitAvailable(t) {
		return
	}

	root := t.TempDir()
	reposDir := filepath.Join(root, "repos")
	// Mimic the real layout: repos/github.com/<org>/<repo>
	layout := []string{
		filepath.Join(reposDir, "github.com", "org1", "repo-a"),
		filepath.Join(reposDir, "github.com", "org1", "repo-b"),
		filepath.Join(reposDir, "github.com", "org2", "repo-c"),
	}
	for _, p := range layout {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		mustGit(t, "", "init", "--initial-branch=main", p)
	}

	// A non-repo sibling that should be ignored.
	if err := os.MkdirAll(filepath.Join(reposDir, "github.com", "org2", "notarepo"), 0o755); err != nil {
		t.Fatalf("mkdir notarepo: %v", err)
	}

	got, err := FindGitRepos(reposDir)
	if err != nil {
		t.Fatalf("FindGitRepos: %v", err)
	}
	if len(got) != len(layout) {
		t.Fatalf("expected %d repos, got %d: %v", len(layout), len(got), got)
	}
}

func TestPullAllRepos_PullsCleanFastForward(t *testing.T) {
	if !gitAvailable(t) {
		return
	}

	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	makeBareOrigin(t, bare, "main")

	reposDir := filepath.Join(root, "repos")
	clone := cloneFrom(t, bare, filepath.Join(reposDir, "github.com", "org", "repo"), "main")

	// Advance origin by a commit via a scratch clone.
	scratch := t.TempDir()
	mustGit(t, "", "clone", bare, scratch)
	mustGit(t, scratch, "config", "user.email", "a@b")
	mustGit(t, scratch, "config", "user.name", "A")
	writeFile(t, filepath.Join(scratch, "new.txt"), "x\n")
	mustGit(t, scratch, "add", "new.txt")
	mustGit(t, scratch, "commit", "-m", "advance")
	mustGit(t, scratch, "push", "origin", "main")

	summary, err := PullAllRepos(root, PullOpts{ReposRoot: reposDir})
	if err != nil {
		t.Fatalf("PullAllRepos: %v", err)
	}
	if summary.Pulled != 1 {
		t.Fatalf("expected 1 pulled, got %d (%#v)", summary.Pulled, summary.Repos)
	}

	if _, err := os.Stat(filepath.Join(clone, "new.txt")); err != nil {
		t.Fatalf("expected new.txt pulled into clone: %v", err)
	}
}

func TestPullAllRepos_SkipsDirty(t *testing.T) {
	if !gitAvailable(t) {
		return
	}

	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	makeBareOrigin(t, bare, "main")

	reposDir := filepath.Join(root, "repos")
	clone := cloneFrom(t, bare, filepath.Join(reposDir, "github.com", "org", "dirty"), "main")

	// Dirty the working tree.
	writeFile(t, filepath.Join(clone, "dirty.txt"), "dirty\n")

	summary, err := PullAllRepos(root, PullOpts{ReposRoot: reposDir})
	if err != nil {
		t.Fatalf("PullAllRepos: %v", err)
	}
	if summary.Pulled != 0 {
		t.Fatalf("expected 0 pulled, got %d", summary.Pulled)
	}
	if summary.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d (%#v)", summary.Skipped, summary.Repos)
	}
	if got := summary.Repos[0].Action; got != "skipped-dirty" {
		t.Fatalf("expected skipped-dirty, got %q", got)
	}
}

func TestPullAllRepos_SkipsNonDefaultBranch(t *testing.T) {
	if !gitAvailable(t) {
		return
	}

	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	makeBareOrigin(t, bare, "main")

	reposDir := filepath.Join(root, "repos")
	clone := cloneFrom(t, bare, filepath.Join(reposDir, "github.com", "org", "featbranch"), "main")

	mustGit(t, clone, "checkout", "-b", "feature/x")

	summary, err := PullAllRepos(root, PullOpts{ReposRoot: reposDir})
	if err != nil {
		t.Fatalf("PullAllRepos: %v", err)
	}
	if got := summary.Repos[0].Action; got != "skipped-branch" {
		t.Fatalf("expected skipped-branch, got %q", got)
	}
}

func TestPullAllRepos_FetchOnly(t *testing.T) {
	if !gitAvailable(t) {
		return
	}

	root := t.TempDir()
	bare := filepath.Join(root, "bare.git")
	makeBareOrigin(t, bare, "main")

	reposDir := filepath.Join(root, "repos")
	_ = cloneFrom(t, bare, filepath.Join(reposDir, "github.com", "org", "fo"), "main")

	summary, err := PullAllRepos(root, PullOpts{ReposRoot: reposDir, FetchOnly: true})
	if err != nil {
		t.Fatalf("PullAllRepos: %v", err)
	}
	if summary.Pulled != 0 {
		t.Fatalf("expected 0 pulled in FetchOnly, got %d", summary.Pulled)
	}
	if summary.Fetched != 1 {
		t.Fatalf("expected 1 fetched, got %d", summary.Fetched)
	}
}

func TestPullAllRepos_MissingRoot(t *testing.T) {
	_, err := PullAllRepos(t.TempDir(), PullOpts{ReposRoot: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

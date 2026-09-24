package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// excludeFromWorktreeVCS had exactly one caller (the Serena provisioner) and one
// assertion, inside that provisioner's happy-path test. It now has two callers and
// is the mechanism that keeps a fresh worktree out of `git status`, so it gets
// tests of its own — including the two properties nothing covered: idempotency
// across repeated calls, and not destroying the user's own exclude entries.

// A LINKED worktree is the case that matters, and the one an earlier attempt got
// wrong: git reads info/exclude from the COMMON dir, so writing to the
// per-worktree $GIT_DIR/info/exclude has no effect at all.
func TestExcludeFromWorktreeVCS_LinkedWorktreeWritesToTheCommonDir(t *testing.T) {
	root := t.TempDir()
	repoGit := filepath.Join(root, "repo", ".git")
	worktreeGitDir := filepath.Join(repoGit, "worktrees", "TASK-00001")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	worktree := filepath.Join(root, "work", "TASK-00001")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+worktreeGitDir+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}

	if err := excludeFromWorktreeVCS(worktree, "/AGENTS.md"); err != nil {
		t.Fatalf("excludeFromWorktreeVCS: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(repoGit, "info", "exclude"))
	if err != nil {
		t.Fatalf("common-dir exclude not written: %v", err)
	}
	if !strings.Contains(string(body), "/AGENTS.md") {
		t.Errorf("exclude does not carry the pattern:\n%s", body)
	}

	// And it must NOT have written the ineffective per-worktree copy, since a
	// file there would read as "this is handled" while changing nothing.
	if _, err := os.Stat(filepath.Join(worktreeGitDir, "info", "exclude")); err == nil {
		t.Error("wrote the per-worktree exclude, which git does not read")
	}
}

// A plain clone's `.git` is a directory and is itself the common dir.
func TestExcludeFromWorktreeVCS_PlainClone(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := excludeFromWorktreeVCS(root, "/.adb/"); err != nil {
		t.Fatalf("excludeFromWorktreeVCS: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("exclude not written: %v", err)
	}
	if !strings.Contains(string(body), "/.adb/") {
		t.Errorf("exclude does not carry the pattern:\n%s", body)
	}
}

// It runs on every task create AND every resume, so appending the same pattern
// repeatedly would grow the file without bound.
func TestExcludeFromWorktreeVCS_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := excludeFromWorktreeVCS(root, "/AGENTS.md"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	body, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if got := strings.Count(string(body), "/AGENTS.md"); got != 1 {
		t.Errorf("pattern appears %d times after 3 calls, want 1:\n%s", got, body)
	}
}

// This file belongs to the user's clone, not to adb. Whatever they already had in
// it must survive.
func TestExcludeFromWorktreeVCS_PreservesTheUsersEntries(t *testing.T) {
	root := t.TempDir()
	infoDir := filepath.Join(root, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Deliberately with no trailing newline: appending to a file that does not end
	// in one would otherwise splice adb's pattern onto the user's last line.
	existing := "# my own excludes\n/scratch.txt"
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte(existing), 0o644); err != nil {
		t.Fatalf("seed exclude: %v", err)
	}

	if err := excludeFromWorktreeVCS(root, "/AGENTS.md"); err != nil {
		t.Fatalf("excludeFromWorktreeVCS: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(infoDir, "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(body), "/scratch.txt") {
		t.Errorf("the user's own entry was lost:\n%s", body)
	}
	if strings.Contains(string(body), "/scratch.txt/AGENTS.md") {
		t.Errorf("the pattern was spliced onto the user's last line:\n%s", body)
	}
	if !strings.Contains(string(body), "\n/AGENTS.md") {
		t.Errorf("the pattern is not on its own line:\n%s", body)
	}
}

// A repo-less task's ticket directory has no .git at all. The caller ignores the
// error, so this only pins that it IS an error rather than a panic or a silently
// created stray directory.
func TestExcludeFromWorktreeVCS_NoGitDirectory(t *testing.T) {
	root := t.TempDir()

	if err := excludeFromWorktreeVCS(root, "/AGENTS.md"); err == nil {
		t.Error("expected an error for a directory that is not a checkout")
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		t.Errorf("left %d stray entries behind: %v", len(entries), entries)
	}
}

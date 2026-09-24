package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// worktreeDirtyReason's "unpushed commits" probe runs `git remote` first,
// because a purely local repo has nowhere to push and its commits are therefore
// not unpushed. That branch used to be spelled `if err != nil || out == ""`,
// which conflated "this repo has no remotes" (safe) with "the probe failed"
// (unknown) — and answered "safe to remove" for both, contradicting the
// function's own doc comment and defeating the #207 guard in the destructive
// direction. Splitting the two must not change the no-remote verdict, which is
// what these tests pin.

func newLocalRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init")
	gitIn(t, dir, "config", "user.email", "test@example.com")
	gitIn(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitIn(t, dir, "add", "README.md")
	gitIn(t, dir, "commit", "-m", "initial commit")
	return dir
}

// TestWorktreeDirtyReason_NoRemoteIsSafe is the branch that must survive: a
// remote-less repo holding committed work is clean, not "unpushed".
func TestWorktreeDirtyReason_NoRemoteIsSafe(t *testing.T) {
	dir := newLocalRepoWithCommit(t)
	if got := gitIn(t, dir, "remote"); got != "" {
		t.Fatalf("precondition: expected no remotes, got %q", got)
	}

	reason, err := worktreeDirtyReason(dir)
	if err != nil {
		t.Fatalf("worktreeDirtyReason on a clean remote-less repo: %v", err)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty: a repo with no remote has nowhere to push, so its commits are not unpushed", reason)
	}
}

// TestWorktreeDirtyReason_NoRemoteStillDetectsDirtyTree proves the no-remote
// early return does not short-circuit the uncommitted-changes check that runs
// before it — otherwise the test above could pass for the wrong reason.
func TestWorktreeDirtyReason_NoRemoteStillDetectsDirtyTree(t *testing.T) {
	dir := newLocalRepoWithCommit(t)
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	reason, err := worktreeDirtyReason(dir)
	if err != nil {
		t.Fatalf("worktreeDirtyReason: %v", err)
	}
	if reason == "" {
		t.Error("reason is empty, want a dirty reason: an untracked file must be reported even with no remote")
	}
}

// TestWorktreeDirtyReason_UnverifiableIsNotSafe pins the fail-safe contract at
// the level the caller depends on: when the probe cannot answer, the answer is
// an error (RemoveWorktree turns that into a refusal naming force), never an
// empty reason. A non-repo makes the very first probe fail, which is the only
// probe failure reachable without stubbing git.
func TestWorktreeDirtyReason_UnverifiableIsNotSafe(t *testing.T) {
	dir := t.TempDir() // a directory, but not a git repo

	reason, err := worktreeDirtyReason(dir)
	if err == nil {
		t.Fatalf("want an error for an unverifiable worktree, got reason=%q err=nil", reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty alongside the error", reason)
	}
}

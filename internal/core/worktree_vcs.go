package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Worktree VCS hygiene: keeping adb's own generated files out of `git status`.
//
// THE BUG THIS EXISTS FOR is worse than it sounds. adb writes generated files into
// a repo's working tree, so in any repo that does not happen to gitignore them a
// BRAND-NEW worktree was immediately dirty: `adb task list --git` reported `dirty`
// for every ticket, and `adb task worktree remove` / `adb task archive` REFUSED to
// tear the worktree down via the #207 safe-teardown guard — over a file adb itself
// had just written. Reproduced against the installed binary. It only looked fine in
// development because ai-dev-brain's own .gitignore happens to list the file.
//
// $GIT_COMMON_DIR/info/exclude is the right place, and every alternative is worse:
//
//   - the repo's tracked .gitignore is the USER'S file and may be committed — adb
//     must not edit it;
//   - the per-worktree $GIT_DIR/info/exclude does NOT work: git reads info/exclude
//     from the COMMON dir, so a linked worktree's own copy is silently ignored
//     (verified empirically — it cost a wrong first implementation here);
//   - core.excludesFile is a global user setting, far too broad a blast radius.
//
// The common-dir exclude lives inside .git: never committed, invisible to `git
// status`, and shared by every worktree of that clone — which is exactly right,
// since adb writes the same filenames into each one.
//
// This started life inside serena_provision.go, excluding `.serena/`. It moved here
// when the worktree task context needed the same thing (TASK-00039 Q5): two
// independent appenders to one git exclude file is precisely how two spellings of
// one rule drift apart.

// excludeFromWorktreeVCS appends pattern to the worktree repo's git exclude file
// (idempotently) so an adb-generated path doesn't surface as an untracked
// change. It resolves the common git dir from the worktree's .git (a dir for a
// normal checkout, a "gitdir:" pointer file for a linked worktree). Pure file
// IO — no git binary is invoked.
func excludeFromWorktreeVCS(worktreePath, pattern string) error {
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return fmt.Errorf("no .git in worktree: %w", err)
	}

	var commonGitDir string
	if info.IsDir() {
		commonGitDir = gitPath
	} else {
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return fmt.Errorf("failed to read .git pointer: %w", err)
		}
		gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(content)), "gitdir:"))
		// gitdir points at <repo>/.git/worktrees/<name>; the shared exclude
		// lives in the common <repo>/.git.
		commonGitDir = filepath.Dir(filepath.Dir(gitdir))
	}

	// gosec flags excludePath as a tainted path below (G703), because commonGitDir
	// is derived from the CONTENTS of the worktree's `.git` pointer file. That is
	// not a trust boundary: the file is written by git itself inside a checkout adb
	// created under its own `work/` tree, and anyone able to rewrite it could
	// instead drop an executable hook in the same .git directory — git would run
	// that, so a redirected exclude path buys an attacker nothing they don't
	// already have. adb also runs with the invoking user's own privileges, so no
	// escalation is available either. The three sites are annotated individually
	// because a nolint only covers the line it sits on.
	excludePath := filepath.Join(commonGitDir, "info", "exclude")
	//nolint:gosec // G703: path derives from git's own .git pointer inside an adb-created worktree; same-user, no privilege boundary
	if existing, err := os.ReadFile(excludePath); err == nil {
		// Already excluded — nothing to do.
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == pattern {
				return nil
			}
		}
		if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
			pattern = "\n" + pattern
		}
	}

	//nolint:gosec // G703: see the excludePath note above — git's own pointer, same-user, no privilege boundary
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("failed to create git info dir: %w", err)
	}
	//nolint:gosec // G703: see the excludePath note above — git's own pointer, same-user, no privilege boundary
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open git exclude: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(pattern + "\n"); err != nil {
		return fmt.Errorf("failed to append to git exclude: %w", err)
	}
	return nil
}

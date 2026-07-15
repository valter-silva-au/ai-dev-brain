package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// validTaskID matches safe task IDs: alphanumeric with dashes and underscores
var validTaskID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// validBranchName matches Conventional-style branch names: lowercase alnum,
// dashes, slashes (one slash for the type/slug separator). Restrictive on
// purpose so a malicious caller can't slip `;` or `..` through to the
// `git worktree add -b <branchName>` command line.
var validBranchName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// GitWorktreeManager manages git worktrees for multi-repo task isolation
type GitWorktreeManager interface {
	// CreateWorktree creates a new git worktree for a task using the legacy
	// flat layout: worktree at basePath/work/{taskID} on a `task/<taskID>`
	// branch. Kept for backward compatibility (older callers and integration
	// tests). New callers should use CreateWorktreeAt instead so the branch
	// and worktree path are determined by the caller (CreateTask now uses the
	// nested correlation layout and a Conventional branch name).
	//
	// repoPath can be a local path, HTTPS URL, SSH URL, or platform/org/repo
	// identifier.
	CreateWorktree(taskID, repoPath, baseBranch string) (string, error)

	// CreateWorktreeAt creates a new git worktree at an explicit worktreePath
	// on an explicit branchName, branched from baseBranch. The caller is
	// responsible for picking a worktree path that doesn't collide with
	// other tasks; intermediate parent directories are created. This is the
	// path used by the nested correlation layout: the TaskManager passes
	// `<basePath>/work/<platform>/<org>/<repo>/TASK-<id>-<slug>` and
	// `<conv-type>/<slug>` (e.g. `chore/platonic-g0-insurability-probe`).
	CreateWorktreeAt(taskID, repoPath, baseBranch, branchName, worktreePath string) (string, error)

	// RemoveWorktree removes a worktree, resolving the parent repo from its
	// .git file. When force is false it refuses to remove a worktree that has
	// uncommitted/untracked changes or unpushed commits (returning a descriptive
	// error); force skips that guard — the historical always-force behaviour.
	RemoveWorktree(worktreePath string, force bool) error

	// RemoveBranch deletes a local branch in the repo identified by repoPath.
	// It backs the opt-in orphan-branch cleanup on archive, so a stale
	// <type>/<slug> branch doesn't linger after its worktree is gone.
	RemoveBranch(repoPath, branch string) error

	// ListWorktrees returns all worktrees by parsing 'git worktree list --porcelain'
	ListWorktrees(repoPath string) ([]WorktreeInfo, error)

	// GetWorktreeForTask checks if a worktree exists for a task at basePath/work/{taskID}
	GetWorktreeForTask(taskID string) (string, bool, error)

	// NormalizeRepoPath converts various URL formats to canonical platform/org/repo
	NormalizeRepoPath(repoPath string) (string, error)

	// BranchExists reports whether a local branch (refs/heads/<branch>) already
	// exists in the repo. Backs the branch-uniqueness guard (#208); a repo that
	// isn't cloned yet reports false.
	BranchExists(repoPath, branch string) (bool, error)

	// WorktreeStatus reports the live git state of the worktree at worktreePath:
	// whether it exists on disk, its branch, whether it is dirty, and how far
	// ahead/behind its upstream it is. A missing worktree returns {Exists:false}
	// with a nil error so callers can flag it rather than fail (#209).
	WorktreeStatus(worktreePath string) (WorktreeStatus, error)

	// RestoreWorktree rebuilds a missing worktree at worktreePath on branchName,
	// cloning the repo on demand. An existing branch is attached; a vanished one
	// is recreated from baseBranch. Backs `adb work reconcile` (#211).
	RestoreWorktree(repoPath, branchName, worktreePath, baseBranch string) (string, error)
}

// WorktreeStatus is the live git state of a task worktree, joining what
// backlog.yaml records with what git reports on disk (#209).
type WorktreeStatus struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

// WorktreeInfo represents information about a git worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	Commit string
	Bare   bool
}

// DefaultGitWorktreeManager implements GitWorktreeManager
type DefaultGitWorktreeManager struct {
	basePath string // Base path for repos and worktrees
}

// NewGitWorktreeManager creates a new GitWorktreeManager
// basePath is the base directory where repos/ and work/ subdirectories will be created
func NewGitWorktreeManager(basePath string) GitWorktreeManager {
	if basePath == "" {
		basePath = "."
	}
	return &DefaultGitWorktreeManager{
		basePath: basePath,
	}
}

// NormalizeRepoPath converts various URL formats to canonical platform/org/repo
// Handles:
// - HTTPS URLs: https://github.com/org/repo.git -> github.com/org/repo
// - SSH URLs: git@github.com:org/repo.git -> github.com/org/repo
// - Relative paths: ./local/repo -> ./local/repo (unchanged)
// - Absolute paths: /path/to/repo -> /path/to/repo (unchanged)
func (m *DefaultGitWorktreeManager) NormalizeRepoPath(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repoPath cannot be empty")
	}

	// Handle local paths (relative or absolute). filepath.IsAbs is OS-aware
	// (recognises Windows drive letters and UNC paths in addition to `/`),
	// so a repo path like `C:\Users\me\repo` is treated as local on Windows
	// rather than falling through to the URL / platform-path branches below.
	if filepath.IsAbs(repoPath) || strings.HasPrefix(repoPath, "./") || strings.HasPrefix(repoPath, "../") {
		return repoPath, nil
	}

	// Handle HTTPS URLs: https://github.com/org/repo.git
	if strings.HasPrefix(repoPath, "https://") || strings.HasPrefix(repoPath, "http://") {
		// Remove protocol
		path := strings.TrimPrefix(repoPath, "https://")
		path = strings.TrimPrefix(path, "http://")
		// Remove .git suffix
		path = strings.TrimSuffix(path, ".git")
		return path, nil
	}

	// Handle SSH URLs: git@github.com:org/repo.git
	if strings.HasPrefix(repoPath, "git@") {
		// Format: git@platform:org/repo.git
		parts := strings.SplitN(repoPath, "@", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid SSH URL format: %s", repoPath)
		}
		// Split by colon
		hostRepo := strings.SplitN(parts[1], ":", 2)
		if len(hostRepo) != 2 {
			return "", fmt.Errorf("invalid SSH URL format: %s", repoPath)
		}
		platform := hostRepo[0]
		repo := strings.TrimSuffix(hostRepo[1], ".git")
		return fmt.Sprintf("%s/%s", platform, repo), nil
	}

	// Assume it's already in platform/org/repo format
	return repoPath, nil
}

// CreateWorktree creates a new git worktree using the legacy flat layout. See
// the interface comment for which callers should use this versus
// CreateWorktreeAt. This is now a thin wrapper around createWorktreeImpl with
// empty explicit values so the impl picks legacy defaults.
func (m *DefaultGitWorktreeManager) CreateWorktree(taskID, repoPath, baseBranch string) (string, error) {
	return m.createWorktreeImpl(taskID, repoPath, baseBranch, "", "")
}

// CreateWorktreeAt creates a worktree at an explicit path on an explicit
// branch. See interface comment for the layout the TaskManager uses.
func (m *DefaultGitWorktreeManager) CreateWorktreeAt(taskID, repoPath, baseBranch, branchName, worktreePath string) (string, error) {
	if branchName == "" {
		return "", fmt.Errorf("branchName cannot be empty for CreateWorktreeAt; use CreateWorktree for the legacy default")
	}
	if worktreePath == "" {
		return "", fmt.Errorf("worktreePath cannot be empty for CreateWorktreeAt; use CreateWorktree for the legacy default")
	}
	return m.createWorktreeImpl(taskID, repoPath, baseBranch, branchName, worktreePath)
}

// createWorktreeImpl is the shared implementation. If branchName or
// worktreePath is empty, it falls back to the legacy default
// (`task/<taskID>` and `<basePath>/work/<taskID>` respectively) so the
// pre-existing public CreateWorktree behavior is preserved exactly.
func (m *DefaultGitWorktreeManager) createWorktreeImpl(taskID, repoPath, baseBranch, branchName, worktreePath string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("taskID cannot be empty")
	}
	if repoPath == "" {
		return "", fmt.Errorf("repoPath cannot be empty")
	}
	// Validate taskID to prevent path traversal
	if !validTaskID.MatchString(taskID) {
		return "", fmt.Errorf("taskID contains invalid characters (must be alphanumeric with dashes/underscores): %s", taskID)
	}
	if strings.Contains(taskID, "..") {
		return "", fmt.Errorf("taskID contains path traversal sequence: %s", taskID)
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Normalize the repo path
	normalizedPath, err := m.NormalizeRepoPath(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to normalize repo path: %w", err)
	}

	// Determine if this is a local path or remote repo. Using filepath.IsAbs
	// so Windows absolute paths (drive letters, UNC) are kept as-is rather
	// than being joined under `basePath/repos/` — that join produces invalid
	// paths like `workspace\repos\C:\Users\...` which git cannot chdir into.
	var repoDir string
	if filepath.IsAbs(normalizedPath) || strings.HasPrefix(normalizedPath, "./") || strings.HasPrefix(normalizedPath, "../") {
		// Local path - use as-is
		repoDir = normalizedPath
	} else {
		// Remote repo - clone to repos/{platform}/{org}/{repo}
		repoDir = filepath.Join(m.basePath, "repos", normalizedPath)

		// Check if repo already exists
		if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
			// Repo doesn't exist, clone it
			if err := m.cloneRepo(repoPath, repoDir); err != nil {
				return "", fmt.Errorf("failed to clone repo: %w", err)
			}
		}
	}

	// Verify repo directory exists and is a git repo
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		return "", fmt.Errorf("not a git repository: %s", repoDir)
	}

	// Resolve worktreePath: use the caller's value if given, else fall back
	// to the legacy flat layout (basePath/work/<taskID>).
	if worktreePath == "" {
		workDir := filepath.Join(m.basePath, "work")
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create work directory: %w", err)
		}
		worktreePath = filepath.Join(workDir, taskID)
	} else {
		// Caller-supplied path may be nested several directories deep
		// (work/<platform>/<org>/<repo>/TASK-id-slug). Create intermediate
		// parents but NOT the leaf itself — git worktree add insists on
		// creating the leaf.
		if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
			return "", fmt.Errorf("failed to create worktree parent directory: %w", err)
		}
	}

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return "", fmt.Errorf("worktree already exists at: %s", worktreePath)
	}

	// Resolve branch name: use the caller's value if given, else fall back
	// to the legacy `task/<taskID>` form.
	if branchName == "" {
		branchName = fmt.Sprintf("task/%s", taskID)
	} else if !validBranchName.MatchString(branchName) {
		// Caller-supplied branch name flows directly into a `git worktree
		// add -b <branchName>` argv, so reject anything outside the
		// Conventional shape we expect.
		return "", fmt.Errorf("branchName contains invalid characters: %s", branchName)
	}

	// Resolve the ref to branch from. By default we fetch the remote and
	// branch off the freshly-fetched origin/<base> so a new task always
	// starts from current upstream, regardless of when the local clone was
	// last pulled. Falls back to the local base branch when offline, when
	// there is no upstream, or when ADB_NO_FETCH=1.
	baseRef := resolveWorktreeBase(repoDir, baseBranch)

	// Create worktree with new branch
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, baseRef)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		out := strings.TrimSpace(string(output))
		// Backstop for the branch-uniqueness guard (#208): if git rejects the
		// add because the branch already exists or is checked out elsewhere,
		// surface a clear, actionable message rather than raw git stderr.
		if strings.Contains(out, "already exists") ||
			strings.Contains(out, "already used by worktree") ||
			strings.Contains(out, "already checked out") {
			return "", fmt.Errorf("cannot create worktree: branch %q already exists or is checked out in another worktree (repo %s)", branchName, repoDir)
		}
		return "", fmt.Errorf("failed to create worktree: %w: %s", err, out)
	}

	return worktreePath, nil
}

// resolveWorktreeBase decides which ref a new task worktree should branch
// from. It fetches the repo's default remote (best-effort) and, if
// origin/<baseBranch> resolves, returns that fully-fresh ref; otherwise it
// returns the local baseBranch unchanged. Fetching is skipped when
// ADB_NO_FETCH=1 (offline / scripted runs that must not hit the network).
// All git calls here are non-fatal: any failure degrades to the local base
// so worktree creation never breaks just because a fetch couldn't run.
func resolveWorktreeBase(repoDir, baseBranch string) string {
	if os.Getenv("ADB_NO_FETCH") == "1" {
		return baseBranch
	}
	remote := defaultRemote(repoDir)
	if remote == "" {
		// No upstream configured (e.g. a fresh local repo) — nothing to
		// fetch; branch from the local base.
		return baseBranch
	}
	// Best-effort fetch of just the base branch. Ignore errors (offline,
	// auth prompt suppressed, etc.) — we still try to branch from whatever
	// origin/<base> we already have.
	fetch := exec.Command("git", "fetch", "--quiet", remote, baseBranch)
	fetch.Dir = repoDir
	_ = fetch.Run()

	remoteRef := remote + "/" + baseBranch
	if refExists(repoDir, remoteRef) {
		return remoteRef
	}
	return baseBranch
}

// defaultRemote returns the repo's remote to fetch from: "origin" when it
// exists, else the first configured remote, else "" when the repo has none.
func defaultRemote(repoDir string) string {
	cmd := exec.Command("git", "remote")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	remotes := strings.Fields(string(out))
	for _, r := range remotes {
		if r == "origin" {
			return "origin"
		}
	}
	if len(remotes) > 0 {
		return remotes[0]
	}
	return ""
}

// refExists reports whether ref resolves in repoDir (git rev-parse --verify).
func refExists(repoDir, ref string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	cmd.Dir = repoDir
	return cmd.Run() == nil
}

// cloneRepo clones a repository with HTTPS first, SSH fallback
func (m *DefaultGitWorktreeManager) cloneRepo(repoPath, targetDir string) error {
	// Create parent directory
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Try HTTPS first
	var httpsURL string
	if strings.HasPrefix(repoPath, "https://") || strings.HasPrefix(repoPath, "http://") {
		httpsURL = repoPath
	} else if strings.HasPrefix(repoPath, "git@") {
		// Convert SSH to HTTPS
		// git@github.com:org/repo.git -> https://github.com/org/repo.git
		parts := strings.SplitN(repoPath, "@", 2)
		if len(parts) == 2 {
			hostRepo := strings.Replace(parts[1], ":", "/", 1)
			httpsURL = fmt.Sprintf("https://%s", hostRepo)
		}
	} else {
		// Assume platform/org/repo format, default to GitHub HTTPS
		httpsURL = fmt.Sprintf("https://%s.git", repoPath)
	}

	// Try cloning with HTTPS
	cmd := exec.Command("git", "clone", httpsURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// HTTPS failed, try SSH
	var sshURL string
	if strings.HasPrefix(repoPath, "git@") {
		sshURL = repoPath
	} else if strings.HasPrefix(repoPath, "https://") || strings.HasPrefix(repoPath, "http://") {
		// Convert HTTPS to SSH
		// https://github.com/org/repo.git -> git@github.com:org/repo.git
		url := strings.TrimPrefix(repoPath, "https://")
		url = strings.TrimPrefix(url, "http://")
		parts := strings.SplitN(url, "/", 2)
		if len(parts) == 2 {
			sshURL = fmt.Sprintf("git@%s:%s", parts[0], parts[1])
		}
	} else {
		// Assume platform/org/repo format, default to GitHub SSH
		parts := strings.SplitN(repoPath, "/", 2)
		if len(parts) == 2 {
			sshURL = fmt.Sprintf("git@%s:%s.git", parts[0], parts[1])
		}
	}

	if sshURL == "" {
		return fmt.Errorf("failed to clone with HTTPS: %w: %s", err, string(output))
	}

	// Try cloning with SSH
	cmd = exec.Command("git", "clone", sshURL, targetDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clone with HTTPS and SSH: %s", string(output))
	}

	return nil
}

// RemoveWorktree removes a worktree, resolving parent repo from its .git file.
// When force is false it refuses to remove a worktree holding uncommitted work
// or unpushed commits; force restores the historical always-force behaviour.
func (m *DefaultGitWorktreeManager) RemoveWorktree(worktreePath string, force bool) error {
	if worktreePath == "" {
		return fmt.Errorf("worktreePath cannot be empty")
	}

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return fmt.Errorf("worktree does not exist: %s", worktreePath)
	}

	// Safety: unless forced, refuse to discard uncommitted work or unpushed
	// commits (#207). The caller can pass force to override.
	if !force {
		if reason, err := worktreeDirtyReason(worktreePath); err != nil {
			return fmt.Errorf("could not verify worktree is safe to remove (%s): %w; pass force to override", worktreePath, err)
		} else if reason != "" {
			return fmt.Errorf("refusing to remove worktree with %s: %s; commit/push or pass force to override", reason, worktreePath)
		}
	}

	// Read .git file to find parent repo
	gitFile := filepath.Join(worktreePath, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return fmt.Errorf("failed to read .git file: %w", err)
	}

	// Parse .git file: "gitdir: /path/to/parent/repo/.git/worktrees/name"
	gitdirLine := strings.TrimSpace(string(content))
	if !strings.HasPrefix(gitdirLine, "gitdir: ") {
		return fmt.Errorf("invalid .git file format: %s", gitdirLine)
	}

	gitdir := strings.TrimPrefix(gitdirLine, "gitdir: ")
	// Navigate up from .git/worktrees/name to parent repo
	// gitdir is something like: /path/to/parent/.git/worktrees/taskname
	parentGitDir := filepath.Dir(filepath.Dir(gitdir))
	parentRepo := filepath.Dir(parentGitDir)

	// Remove worktree using git
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = parentRepo
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %w: %s", err, string(output))
	}

	return nil
}

// worktreeDirtyReason returns a non-empty human-readable reason when the
// worktree has uncommitted/untracked changes or unpushed commits, so a
// non-forced teardown can refuse instead of silently discarding work (#207).
// An empty reason with a nil error means the worktree is safe to remove.
func worktreeDirtyReason(worktreePath string) (string, error) {
	// Uncommitted, staged, or untracked changes.
	status := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := status.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "" {
		return "uncommitted or untracked changes", nil
	}

	// Unpushed commits only make sense when a remote exists — a purely local
	// repo has nowhere to push, so its commits are not "unpushed".
	remotes, err := exec.Command("git", "-C", worktreePath, "remote").Output()
	if err != nil || strings.TrimSpace(string(remotes)) == "" {
		return "", nil
	}

	// Commits reachable from HEAD but not from any remote-tracking branch.
	rev := exec.Command("git", "-C", worktreePath, "rev-list", "HEAD", "--not", "--remotes", "--count")
	revOut, err := rev.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-list failed: %w: %s", err, strings.TrimSpace(string(revOut)))
	}
	if n := strings.TrimSpace(string(revOut)); n != "" && n != "0" {
		return fmt.Sprintf("%s unpushed commit(s)", n), nil
	}
	return "", nil
}

// repoCloneDir resolves the local clone directory for repoPath the same way
// createWorktreeImpl does: absolute/relative paths are used as-is; a canonical
// platform/org/repo is joined under <basePath>/repos.
func (m *DefaultGitWorktreeManager) repoCloneDir(repoPath string) (string, error) {
	normalized, err := m.NormalizeRepoPath(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to normalize repo path: %w", err)
	}
	if filepath.IsAbs(normalized) || strings.HasPrefix(normalized, "./") || strings.HasPrefix(normalized, "../") {
		return normalized, nil
	}
	return filepath.Join(m.basePath, "repos", normalized), nil
}

// RemoveBranch deletes a local branch in the repo identified by repoPath. Used
// for opt-in orphan-branch cleanup after archiving (#207).
func (m *DefaultGitWorktreeManager) RemoveBranch(repoPath, branch string) error {
	if repoPath == "" || branch == "" {
		return fmt.Errorf("repoPath and branch are required")
	}
	repoDir, err := m.repoCloneDir(repoPath)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch %q: %w: %s", branch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BranchExists reports whether refs/heads/<branch> exists in repoPath's clone.
// A repo that isn't cloned yet reports false — there is nothing to collide with
// (#208).
func (m *DefaultGitWorktreeManager) BranchExists(repoPath, branch string) (bool, error) {
	if repoPath == "" || branch == "" {
		return false, nil
	}
	repoDir, err := m.repoCloneDir(repoPath)
	if err != nil {
		return false, err
	}
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(statErr) {
		return false, nil
	}
	return refExists(repoDir, "refs/heads/"+branch), nil
}

// RestoreWorktree rebuilds a missing worktree at worktreePath on branchName,
// cloning the repo on demand. If the branch survived (its worktree was removed
// but the branch kept) it is attached with `git worktree add <path> <branch>`;
// if the branch is gone too it is recreated from baseBranch (default "main")
// with `git worktree add -b`. An already-present worktree is a no-op. This is
// the rebuild primitive behind `adb work reconcile` (#211).
func (m *DefaultGitWorktreeManager) RestoreWorktree(repoPath, branchName, worktreePath, baseBranch string) (string, error) {
	if repoPath == "" || branchName == "" || worktreePath == "" {
		return "", fmt.Errorf("repoPath, branchName and worktreePath are required")
	}
	if !validBranchName.MatchString(branchName) {
		return "", fmt.Errorf("branchName contains invalid characters: %s", branchName)
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	repoDir, err := m.repoCloneDir(repoPath)
	if err != nil {
		return "", err
	}
	// Clone on demand so a fresh machine (empty repos/) can rebuild from nothing.
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(statErr) {
		if err := m.cloneRepo(repoPath, repoDir); err != nil {
			return "", fmt.Errorf("failed to clone repo: %w", err)
		}
	}

	// Already present — nothing to restore.
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		return worktreePath, nil
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	var cmd *exec.Cmd
	if refExists(repoDir, "refs/heads/"+branchName) {
		// The branch survived the worktree removal — attach it.
		cmd = exec.Command("git", "worktree", "add", worktreePath, branchName)
	} else {
		// Branch gone too — recreate it from the (freshly fetched) base.
		cmd = exec.Command("git", "worktree", "add", "-b", branchName, worktreePath, resolveWorktreeBase(repoDir, baseBranch))
	}
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to restore worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return worktreePath, nil
}

// WorktreeStatus reports the live git state of the worktree at worktreePath.
// A non-existent worktree yields {Exists:false} with a nil error, so a caller
// can flag it as missing rather than treat it as a failure (#209).
func (m *DefaultGitWorktreeManager) WorktreeStatus(worktreePath string) (WorktreeStatus, error) {
	st := WorktreeStatus{Path: worktreePath}
	if worktreePath == "" {
		return st, fmt.Errorf("worktreePath cannot be empty")
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return st, nil // Exists stays false — a missing worktree.
	}
	st.Exists = true

	// `git status --porcelain=v1 --branch` prints a "## <branch>...<upstream>
	// [ahead N, behind M]" header followed by one line per changed path.
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain=v1", "--branch")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return st, fmt.Errorf("git status failed for %s: %w: %s", worktreePath, err, strings.TrimSpace(string(out)))
	}
	for i, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if i == 0 && strings.HasPrefix(line, "## ") {
			st.Branch, st.Ahead, st.Behind = parseStatusBranchLine(strings.TrimPrefix(line, "## "))
			continue
		}
		if strings.TrimSpace(line) != "" {
			st.Dirty = true
		}
	}
	return st, nil
}

// parseStatusBranchLine parses the "## " header of `git status --branch`:
//
//	"main...origin/main [ahead 1, behind 2]" → ("main", 1, 2)
//	"main...origin/main"                     → ("main", 0, 0)
//	"feat/x"                                 → ("feat/x", 0, 0)  (no upstream)
//	"No commits yet on main"                 → ("main", 0, 0)
func parseStatusBranchLine(s string) (branch string, ahead, behind int) {
	if i := strings.Index(s, " ["); i >= 0 {
		bracket := strings.TrimSuffix(s[i+2:], "]")
		for _, part := range strings.Split(bracket, ", ") {
			if n, ok := strings.CutPrefix(strings.TrimSpace(part), "ahead "); ok {
				fmt.Sscanf(n, "%d", &ahead)
			} else if n, ok := strings.CutPrefix(strings.TrimSpace(part), "behind "); ok {
				fmt.Sscanf(n, "%d", &behind)
			}
		}
		s = s[:i]
	}
	if i := strings.Index(s, "..."); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "No commits yet on ")
	return strings.TrimSpace(s), ahead, behind
}

// ListWorktrees returns all worktrees by parsing 'git worktree list --porcelain'
func (m *DefaultGitWorktreeManager) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath cannot be empty")
	}

	// Run git worktree list --porcelain
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w: %s", err, string(output))
	}

	// Parse porcelain output
	worktrees := []WorktreeInfo{}
	lines := strings.Split(string(output), "\n")

	var current *WorktreeInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line marks end of a worktree entry
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			// Start of a new worktree entry. Normalise the path via
			// filepath.FromSlash so Windows callers comparing against
			// filepath.Join-produced expectations don't get spurious
			// mismatches — `git worktree list --porcelain` emits
			// forward slashes even on Windows.
			path := filepath.FromSlash(strings.TrimPrefix(line, "worktree "))
			// Also canonicalise via EvalSymlinks so the returned path matches
			// what a caller gets from resolving its own paths. On macOS the
			// temp/home trees live under symlinks (/var → /private/var,
			// /tmp → /private/tmp); git already reports the resolved form, so a
			// caller comparing an *unresolved* path (e.g. a raw t.TempDir())
			// would never match. Resolving here makes the contract explicit:
			// ListWorktrees returns canonical absolute paths. Fall back to the
			// raw path when resolution fails — a stale/prunable worktree whose
			// directory has been removed can't be resolved, and dropping it
			// would be worse than reporting the last-known path.
			if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
				path = resolved
			}
			current = &WorktreeInfo{
				Path: path,
			}
		} else if strings.HasPrefix(line, "HEAD ") && current != nil {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			current.Branch = strings.TrimPrefix(line, "branch ")
		} else if line == "bare" && current != nil {
			current.Bare = true
		}
	}

	// Add last entry if exists
	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees, nil
}

// GetWorktreeForTask checks if a worktree exists for a task at basePath/work/{taskID}
func (m *DefaultGitWorktreeManager) GetWorktreeForTask(taskID string) (string, bool, error) {
	if taskID == "" {
		return "", false, fmt.Errorf("taskID cannot be empty")
	}

	worktreePath := filepath.Join(m.basePath, "work", taskID)

	// Check if path exists
	info, err := os.Stat(worktreePath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to check worktree path: %w", err)
	}

	// Verify it's a directory
	if !info.IsDir() {
		return "", false, fmt.Errorf("worktree path exists but is not a directory: %s", worktreePath)
	}

	// Verify it has a .git file (characteristic of a worktree)
	gitFile := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitFile); os.IsNotExist(err) {
		return "", false, fmt.Errorf("worktree path exists but is not a git worktree: %s", worktreePath)
	}

	return worktreePath, true, nil
}

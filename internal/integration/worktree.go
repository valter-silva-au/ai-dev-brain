package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// isAbsRepoPath returns true if the repo path is an absolute filesystem path
// (i.e., a local git repository) rather than a platform/org/repo identifier.
func isAbsRepoPath(repoPath string) bool {
	return filepath.IsAbs(repoPath)
}

// WorktreeConfig holds the parameters needed to create a new git worktree.
type WorktreeConfig struct {
	RepoPath   string
	BranchName string
	TaskID     string
	BaseBranch string
}

// Worktree represents an active git worktree associated with a task.
type Worktree struct {
	Path     string
	Branch   string
	TaskID   string
	RepoPath string
}

// GitWorktreeManager defines operations for managing git worktrees across
// multiple repositories.
type GitWorktreeManager interface {
	CreateWorktree(config WorktreeConfig) (string, error)
	RemoveWorktree(worktreePath string) error
	ListWorktrees(repoPath string) ([]*Worktree, error)
	GetWorktreeForTask(taskID string) (*Worktree, error)
}

// gitWorktreeManager implements GitWorktreeManager using git CLI commands.
type gitWorktreeManager struct {
	basePath string
}

// NewGitWorktreeManager creates a new GitWorktreeManager that stores worktrees
// under the given basePath.
func NewGitWorktreeManager(basePath string) GitWorktreeManager {
	return &gitWorktreeManager{basePath: basePath}
}

// normalizeRepoPath converts various git URL and path formats into the
// canonical platform/org/repo format. It handles:
//   - github.com/org/repo (already canonical)
//   - github.com:org/repo (SSH-style)
//   - git@github.com:org/repo.git (full SSH URL)
//   - https://github.com/org/repo.git (full HTTPS URL)
//   - repos/github.com/org/repo (with repos/ prefix)
//   - github.com/org/repo.git (with .git suffix)
func normalizeRepoPath(repoPath string) string {
	cleaned := strings.TrimSpace(repoPath)

	// Strip common URL prefixes.
	cleaned = strings.TrimPrefix(cleaned, "https://")
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimPrefix(cleaned, "git@")

	// Strip repos/ prefix (user might copy from filesystem path).
	cleaned = strings.TrimPrefix(cleaned, "repos/")

	// Convert SSH-style colon separator to slash: github.com:org/repo -> github.com/org/repo
	if idx := strings.Index(cleaned, ":"); idx > 0 && !strings.Contains(cleaned[:idx], "/") {
		cleaned = cleaned[:idx] + "/" + cleaned[idx+1:]
	}

	// Strip .git suffix.
	cleaned = strings.TrimSuffix(cleaned, ".git")

	// Normalise separators and trim trailing slashes.
	cleaned = strings.TrimRight(strings.ReplaceAll(cleaned, `\`, "/"), "/")

	return cleaned
}

// parseRepoPath splits a repository path like "github.com/org/repo" into
// platform, org, and repo components. The input is normalized first to handle
// SSH URLs, HTTPS URLs, .git suffixes, and repos/ prefixes.
func parseRepoPath(repoPath string) (platform, org, repo string, err error) {
	cleaned := normalizeRepoPath(repoPath)

	parts := strings.Split(cleaned, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid repo path %q: expected format github.com/org/repo", repoPath)
	}

	// Take the last three segments to handle leading paths gracefully.
	n := len(parts)
	return parts[n-3], parts[n-2], parts[n-1], nil
}

// worktreePath builds the canonical worktree directory for a task:
//
//	basePath/work/{taskID}
func (m *gitWorktreeManager) worktreePath(taskID string) string {
	return filepath.Join(m.basePath, "work", taskID)
}

// CreateWorktree creates a new git worktree for the given task.
//
// The worktree is always placed at basePath/work/{taskID}. If RepoPath is an
// absolute path, it is used directly as the git directory. Otherwise, RepoPath
// is treated as a platform/org/repo identifier: the repo is cloned to
// repos/{platform}/{org}/{repo}/ if not already present, and the worktree is
// created from that local mirror.
//
// It returns the absolute path of the created worktree.
func (m *gitWorktreeManager) CreateWorktree(config WorktreeConfig) (string, error) {
	if config.RepoPath == "" {
		return "", fmt.Errorf("WorktreeConfig.RepoPath must not be empty")
	}
	if config.TaskID == "" {
		return "", fmt.Errorf("WorktreeConfig.TaskID must not be empty")
	}
	if config.BranchName == "" {
		return "", fmt.Errorf("WorktreeConfig.BranchName must not be empty")
	}

	var wtPath string
	var gitDir string

	if isAbsRepoPath(config.RepoPath) {
		// Local git repository: place worktree in basePath/work/{taskID}.
		wtPath = filepath.Join(m.basePath, "work", config.TaskID)
		gitDir = config.RepoPath
	} else {
		// Normalize the repo path to handle SSH URLs, .git suffix, repos/ prefix, etc.
		normalized := normalizeRepoPath(config.RepoPath)
		platform, org, repo, err := parseRepoPath(normalized)
		if err != nil {
			return "", err
		}
		wtPath = m.worktreePath(config.TaskID)
		// Resolve the repo path to the actual directory on disk.
		gitDir = filepath.Join(m.basePath, "repos", platform, org, repo)

		// Ensure the repository is cloned and has commits.
		canonicalPath := platform + "/" + org + "/" + repo
		if err := m.ensureRepoReady(gitDir, canonicalPath); err != nil {
			return "", fmt.Errorf("preparing repository %s: %w", canonicalPath, err)
		}
	}

	args := []string{"worktree", "add"}
	if config.BaseBranch != "" {
		args = append(args, "-b", config.BranchName, wtPath, config.BaseBranch)
	} else {
		args = append(args, "-b", config.BranchName, wtPath)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = gitDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return wtPath, nil
}

// ensureRepoReady ensures the git repository at repoDir is cloned and ready.
// If the directory does not exist, the repository is cloned from the remote
// URL derived from repoPath. If it exists but has no commits, a fetch from
// origin is attempted.
func (m *gitWorktreeManager) ensureRepoReady(repoDir, repoPath string) error {
	_, statErr := os.Stat(filepath.Join(repoDir, ".git"))

	if statErr != nil {
		// Directory doesn't exist or isn't a git repo — clone it.
		cloneURL := repoURLFromPath(repoPath)
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o750); err != nil {
			return fmt.Errorf("creating parent directory: %w", err)
		}
		cmd := exec.Command("git", "clone", cloneURL, repoDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Clone via HTTPS failed — try SSH.
			sshURL := repoSSHURLFromPath(repoPath)
			sshCmd := exec.Command("git", "clone", sshURL, repoDir)
			if sshOutput, sshErr := sshCmd.CombinedOutput(); sshErr != nil {
				return fmt.Errorf("git clone failed (tried HTTPS and SSH):\n  HTTPS: %s\n  SSH: %s: %w",
					strings.TrimSpace(string(output)),
					strings.TrimSpace(string(sshOutput)), sshErr)
			}
		}
		return nil
	}

	// Directory exists and is a git repo. Check if it has any commits.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		// No commits — try fetching from origin.
		fetchCmd := exec.Command("git", "fetch", "origin")
		fetchCmd.Dir = repoDir
		if output, fetchErr := fetchCmd.CombinedOutput(); fetchErr != nil {
			return fmt.Errorf("git fetch origin failed: %s: %w", strings.TrimSpace(string(output)), fetchErr)
		}

		// Check if origin has any branches after fetching.
		defaultBranch := detectDefaultBranch(repoDir)
		if defaultBranch != "" {
			checkoutCmd := exec.Command("git", "checkout", "-b", defaultBranch, "origin/"+defaultBranch)
			checkoutCmd.Dir = repoDir
			if output, checkoutErr := checkoutCmd.CombinedOutput(); checkoutErr != nil {
				return fmt.Errorf("git checkout %s failed: %s: %w", defaultBranch, strings.TrimSpace(string(output)), checkoutErr)
			}
		}
		// If no branches found, the remote repo is empty. That's OK — worktree
		// will be created as an orphan branch.
	} else {
		// Repo has commits — fetch latest branches from origin.
		fetchCmd := exec.Command("git", "fetch", "origin")
		fetchCmd.Dir = repoDir
		_, _ = fetchCmd.CombinedOutput() // best-effort, don't fail if offline
	}

	return nil
}

// repoURLFromPath constructs an HTTPS clone URL from a platform/org/repo path.
func repoURLFromPath(repoPath string) string {
	return "https://" + repoPath + ".git"
}

// repoSSHURLFromPath constructs an SSH clone URL from a platform/org/repo path.
func repoSSHURLFromPath(repoPath string) string {
	cleaned := strings.TrimRight(strings.ReplaceAll(repoPath, `\`, "/"), "/")
	parts := strings.SplitN(cleaned, "/", 2)
	if len(parts) < 2 {
		return "git@" + repoPath
	}
	return "git@" + parts[0] + ":" + parts[1] + ".git"
}

// detectDefaultBranch determines the default branch name from a remote by
// checking origin/main and origin/master. Returns empty string if no remote
// branches are found.
func detectDefaultBranch(repoDir string) string {
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", "origin/"+branch)
		cmd.Dir = repoDir
		if err := cmd.Run(); err == nil {
			return branch
		}
	}
	return ""
}

// RemoveWorktree removes a git worktree at the given path.
func (m *gitWorktreeManager) RemoveWorktree(worktreePath string) error {
	if worktreePath == "" {
		return fmt.Errorf("worktree path must not be empty")
	}

	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// ListWorktrees lists all worktrees for the given repository path by parsing
// the porcelain output of `git worktree list --porcelain`.
func (m *gitWorktreeManager) ListWorktrees(repoPath string) ([]*Worktree, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath must not be empty")
	}

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return parseWorktreeListOutput(string(output), repoPath), nil
}

// parseWorktreeListOutput parses the porcelain output of `git worktree list`.
// Each worktree block is separated by a blank line and contains lines like:
//
//	worktree /path/to/worktree
//	HEAD <sha>
//	branch refs/heads/branch-name
func parseWorktreeListOutput(output, repoPath string) []*Worktree {
	var worktrees []*Worktree

	blocks := strings.Split(strings.TrimSpace(output), "\n\n")
	for _, block := range blocks {
		if block == "" {
			continue
		}

		wt := &Worktree{RepoPath: repoPath}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch refs/heads/"):
				wt.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			}
		}

		// Extract task ID from path: the last segment of
		// .../work/TASK-XXXXX is the task ID.
		if wt.Path != "" {
			dir := filepath.Dir(wt.Path)
			if filepath.Base(dir) == "work" {
				wt.TaskID = filepath.Base(wt.Path)
			}
		}

		worktrees = append(worktrees, wt)
	}

	return worktrees
}

// GetWorktreeForTask searches for the worktree directory associated with the
// given task ID. All worktrees are stored at basePath/work/{taskID}.
func (m *gitWorktreeManager) GetWorktreeForTask(taskID string) (*Worktree, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskID must not be empty")
	}

	workDir := filepath.Join(m.basePath, "work", taskID)
	if info, err := os.Stat(workDir); err == nil && info.IsDir() {
		return &Worktree{
			Path:   workDir,
			TaskID: taskID,
		}, nil
	}

	return nil, fmt.Errorf("no worktree found for task %q", taskID)
}

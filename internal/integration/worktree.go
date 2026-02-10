package integration

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

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

// parseRepoPath splits a repository path like "github.com/org/repo" into
// platform, org, and repo components.
func parseRepoPath(repoPath string) (platform, org, repo string, err error) {
	// Normalise separators and trim trailing slashes.
	cleaned := strings.TrimRight(strings.ReplaceAll(repoPath, `\`, "/"), "/")

	parts := strings.Split(cleaned, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid repo path %q: expected {platform}/{org}/{repo}", repoPath)
	}

	// Take the last three segments to handle leading paths gracefully.
	n := len(parts)
	return parts[n-3], parts[n-2], parts[n-1], nil
}

// worktreePath builds the canonical worktree directory for a task:
//
//	basePath/repos/{platform}/{org}/{repo}/work/{taskID}
func (m *gitWorktreeManager) worktreePath(platform, org, repo, taskID string) string {
	return filepath.Join(m.basePath, "repos", platform, org, repo, "work", taskID)
}

// CreateWorktree creates a new git worktree for the given task. The worktree
// is placed at repos/{platform}/{org}/{repo}/work/{taskID} under the base path.
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

	platform, org, repo, err := parseRepoPath(config.RepoPath)
	if err != nil {
		return "", err
	}

	wtPath := m.worktreePath(platform, org, repo, config.TaskID)

	args := []string{"worktree", "add"}
	if config.BaseBranch != "" {
		args = append(args, "-b", config.BranchName, wtPath, config.BaseBranch)
	} else {
		args = append(args, "-b", config.BranchName, wtPath)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = config.RepoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return wtPath, nil
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

// GetWorktreeForTask searches all worktrees under the base path for one whose
// task ID matches. It walks the repos directory looking for repositories and
// queries each one.
func (m *gitWorktreeManager) GetWorktreeForTask(taskID string) (*Worktree, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskID must not be empty")
	}

	// Search the repos directory structure for work/{taskID} directories.
	pattern := filepath.Join(m.basePath, "repos", "*", "*", "*")
	repoDirs, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("searching for repos: %w", err)
	}

	for _, repoDir := range repoDirs {
		worktrees, err := m.ListWorktrees(repoDir)
		if err != nil {
			// Skip repos that fail to list (not a git repo, etc.).
			continue
		}
		for _, wt := range worktrees {
			if wt.TaskID == taskID {
				return wt, nil
			}
		}
	}

	return nil, fmt.Errorf("no worktree found for task %q", taskID)
}

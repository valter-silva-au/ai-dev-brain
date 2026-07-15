package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// RepoSyncResult represents the result of syncing a single repository
type RepoSyncResult struct {
	RepoPath string
	Success  bool
	Error    error
	Actions  []string // List of actions performed (fetch, prune, ff-merge, branch-cleanup)
}

// RepoSyncManager manages parallel repository synchronization operations
type RepoSyncManager interface {
	// SyncAll synchronizes all repositories under the repos directory
	// Performs: fetch, prune, ff-merge, and branch cleanup in parallel
	SyncAll(reposDir string) ([]RepoSyncResult, error)

	// SyncRepo synchronizes a single repository
	SyncRepo(repoPath string) (*RepoSyncResult, error)
}

// DefaultRepoSyncManager implements RepoSyncManager
type DefaultRepoSyncManager struct {
	maxConcurrency int // Maximum number of repos to sync in parallel
}

// NewRepoSyncManager creates a new repository sync manager
func NewRepoSyncManager(maxConcurrency int) RepoSyncManager {
	if maxConcurrency <= 0 {
		maxConcurrency = 4 // Default to 4 parallel operations
	}
	return &DefaultRepoSyncManager{
		maxConcurrency: maxConcurrency,
	}
}

// SyncAll synchronizes all repositories under the repos directory
func (rsm *DefaultRepoSyncManager) SyncAll(reposDir string) ([]RepoSyncResult, error) {
	// Check if repos directory exists
	if _, err := os.Stat(reposDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("repos directory does not exist: %s", reposDir)
	}

	// Find all subdirectories in repos/
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read repos directory: %w", err)
	}

	// Filter for directories that contain .git
	var repoPaths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoPath := filepath.Join(reposDir, entry.Name())
		gitPath := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			repoPaths = append(repoPaths, repoPath)
		}
	}

	if len(repoPaths) == 0 {
		return []RepoSyncResult{}, nil
	}

	// Sync repos in parallel with concurrency limit
	results := make([]RepoSyncResult, len(repoPaths))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, rsm.maxConcurrency)

	for i, repoPath := range repoPaths {
		wg.Add(1)
		go func(index int, path string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Sync the repo
			result, err := rsm.SyncRepo(path)
			if err != nil {
				results[index] = RepoSyncResult{
					RepoPath: path,
					Success:  false,
					Error:    err,
					Actions:  []string{},
				}
			} else {
				results[index] = *result
			}
		}(i, repoPath)
	}

	wg.Wait()

	return results, nil
}

// SyncRepo synchronizes a single repository
func (rsm *DefaultRepoSyncManager) SyncRepo(repoPath string) (*RepoSyncResult, error) {
	result := &RepoSyncResult{
		RepoPath: repoPath,
		Success:  false,
		Error:    nil,
		Actions:  []string{},
	}

	// Check if it's a git repository
	gitPath := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		result.Error = fmt.Errorf("not a git repository")
		return result, result.Error
	}

	// 1. Fetch with prune
	if err := rsm.gitFetch(repoPath); err != nil {
		result.Error = fmt.Errorf("fetch failed: %w", err)
		return result, result.Error
	}
	result.Actions = append(result.Actions, "fetch")

	// 2. Get current branch
	currentBranch, err := rsm.getCurrentBranch(repoPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to get current branch: %w", err)
		return result, result.Error
	}

	// 3. Fast-forward merge if possible
	if currentBranch != "" && currentBranch != "HEAD" {
		if err := rsm.gitFFMerge(repoPath, currentBranch); err != nil {
			// FF merge failure is not critical, just note it
			result.Actions = append(result.Actions, "ff-merge-skipped")
		} else {
			result.Actions = append(result.Actions, "ff-merge")
		}
	}

	// 4. Clean up merged branches
	if err := rsm.gitCleanupBranches(repoPath); err != nil {
		// Cleanup failure is not critical
		result.Actions = append(result.Actions, "branch-cleanup-skipped")
	} else {
		result.Actions = append(result.Actions, "branch-cleanup")
	}

	result.Success = true
	return result, nil
}

// gitFetch performs git fetch --prune
func (rsm *DefaultRepoSyncManager) gitFetch(repoPath string) error {
	cmd := exec.Command("git", "fetch", "--prune", "--all")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	return nil
}

// getCurrentBranch gets the current branch name
func (rsm *DefaultRepoSyncManager) getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// gitFFMerge performs a fast-forward merge
func (rsm *DefaultRepoSyncManager) gitFFMerge(repoPath, branch string) error {
	// Check if there's a tracking branch
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", fmt.Sprintf("%s@{upstream}", branch))
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// No upstream tracking branch
		return fmt.Errorf("no upstream tracking branch")
	}
	upstream := strings.TrimSpace(string(output))

	// Check if we can fast-forward
	cmd = exec.Command("git", "merge-base", "--is-ancestor", "HEAD", upstream)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		// Cannot fast-forward
		return fmt.Errorf("cannot fast-forward")
	}

	// Perform fast-forward merge
	cmd = exec.Command("git", "merge", "--ff-only", upstream)
	cmd.Dir = repoPath
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}

	return nil
}

// gitCleanupBranches removes local branches that have been merged
func (rsm *DefaultRepoSyncManager) gitCleanupBranches(repoPath string) error {
	// Get list of merged branches (excluding current branch and main/master)
	cmd := exec.Command("git", "branch", "--merged")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	branches := strings.Split(string(output), "\n")
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		// Skip current branch (marked with *)
		if strings.HasPrefix(branch, "*") {
			continue
		}
		// Skip main/master branches
		if branch == "main" || branch == "master" || branch == "" {
			continue
		}

		// Delete the branch
		cmd = exec.Command("git", "branch", "-d", branch)
		cmd.Dir = repoPath
		_ = cmd.Run() // Ignore errors for individual branch deletions
	}

	return nil
}

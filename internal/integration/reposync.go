package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// RepoSyncManager synchronises all git repositories under the workspace's
// repos/ directory by fetching updates, fast-forwarding the default branch,
// and cleaning up merged branches.
type RepoSyncManager struct {
	basePath string
}

// NewRepoSyncManager creates a new RepoSyncManager rooted at the given
// workspace base path.
func NewRepoSyncManager(basePath string) *RepoSyncManager {
	return &RepoSyncManager{basePath: basePath}
}

// RepoSyncResult captures the outcome of synchronising a single repository.
type RepoSyncResult struct {
	RepoPath        string   // Relative path like "github.com/org/repo"
	DefaultBranch   string
	Fetched         bool
	BranchesDeleted []string
	BranchesSkipped []string // Branches that were merged but protected by the backlog
	Error           error
}

// SyncAll discovers all repositories under basePath/repos/ and synchronises
// each one in parallel. protectedBranches maps normalised repo paths
// (e.g. "github.com/org/repo") to a set of branch names that must not be
// deleted because they are associated with active tasks in the backlog.
func (m *RepoSyncManager) SyncAll(protectedBranches map[string]map[string]bool) ([]RepoSyncResult, error) {
	reposDir := filepath.Join(m.basePath, "repos")

	// Walk three levels: platform/org/repo.
	pattern := filepath.Join(reposDir, "*", "*", "*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	type repoEntry struct {
		absPath string
		relPath string
	}

	var repos []repoEntry
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || !info.IsDir() {
			continue
		}
		// Confirm it is a git repository.
		if _, gitErr := os.Stat(filepath.Join(match, ".git")); gitErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(reposDir, match)
		if relErr != nil {
			continue
		}
		repos = append(repos, repoEntry{absPath: match, relPath: rel})
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []RepoSyncResult
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(r repoEntry) {
			defer wg.Done()
			// Look up protected branches for this repo using the normalised
			// relative path (which is already in platform/org/repo form).
			protected := protectedBranches[r.relPath]
			result := m.SyncRepo(r.absPath, r.relPath, protected)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(repo)
	}

	wg.Wait()
	return results, nil
}

// SyncRepo synchronises a single git repository by fetching all remotes,
// fast-forwarding the default branch, and deleting local branches that have
// been merged into it. Branches listed in protected are skipped during
// deletion because they are associated with active backlog tasks.
func (m *RepoSyncManager) SyncRepo(repoAbsPath, repoRelPath string, protected map[string]bool) RepoSyncResult {
	result := RepoSyncResult{RepoPath: repoRelPath}

	// Fetch all remotes and prune deleted remote branches.
	cmd := exec.Command("git", "-C", repoAbsPath, "fetch", "--all", "--prune")
	if err := cmd.Run(); err != nil {
		result.Error = err
		return result
	}
	result.Fetched = true

	// Detect default branch.
	defaultBranch := detectDefaultBranchFromHead(repoAbsPath)
	if defaultBranch == "" {
		defaultBranch = detectDefaultBranch(repoAbsPath)
	}
	result.DefaultBranch = defaultBranch

	if defaultBranch == "" {
		return result
	}

	// If local HEAD is orphaned (points to a branch that doesn't exist),
	// checkout the default branch.
	headCmd := exec.Command("git", "-C", repoAbsPath, "symbolic-ref", "HEAD")
	if headOutput, err := headCmd.Output(); err == nil {
		headBranch := strings.TrimSpace(string(headOutput))
		headBranch = strings.TrimPrefix(headBranch, "refs/heads/")

		// Check if the branch the HEAD points to actually exists.
		verifyCmd := exec.Command("git", "-C", repoAbsPath, "rev-parse", "--verify", headBranch)
		if verifyErr := verifyCmd.Run(); verifyErr != nil {
			// HEAD is orphaned -- checkout the default branch.
			checkoutCmd := exec.Command("git", "-C", repoAbsPath, "checkout", defaultBranch)
			_ = checkoutCmd.Run()
		}
	}

	// Fast-forward the default branch if behind origin.
	ffCmd := exec.Command("git", "-C", repoAbsPath, "merge", "--ff-only", "origin/"+defaultBranch)
	_ = ffCmd.Run() // ignore error: may already be up to date or not on that branch

	// Find branches merged into the default branch.
	mergedCmd := exec.Command("git", "-C", repoAbsPath, "branch", "--merged", defaultBranch)
	mergedOutput, err := mergedCmd.Output()
	if err != nil {
		return result
	}

	for _, line := range strings.Split(string(mergedOutput), "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}
		// Skip current branch (marked with *) and the default branch itself.
		if strings.HasPrefix(branch, "* ") {
			continue
		}
		if branch == defaultBranch {
			continue
		}

		// Skip branches that are associated with active backlog tasks.
		if protected[branch] {
			result.BranchesSkipped = append(result.BranchesSkipped, branch)
			continue
		}

		delCmd := exec.Command("git", "-C", repoAbsPath, "branch", "-d", branch)
		if delErr := delCmd.Run(); delErr == nil {
			result.BranchesDeleted = append(result.BranchesDeleted, branch)
		}
	}

	return result
}

// detectDefaultBranchFromHead checks the symbolic-ref of origin/HEAD to
// determine the default branch. Returns empty string if origin/HEAD is not
// set.
func detectDefaultBranchFromHead(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output is like "refs/remotes/origin/main"
	ref := strings.TrimSpace(string(output))
	const prefix = "refs/remotes/origin/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ""
}

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PullRepoResult is the outcome of syncing a single repo via PullAllRepos.
type PullRepoResult struct {
	Path    string
	Action  string // "pulled" | "fetched" | "skipped-dirty" | "skipped-branch" | "skipped-no-upstream" | "error"
	Branch  string
	Default string
	Err     error
}

// PullSummary aggregates results across all synced repos.
type PullSummary struct {
	Repos    []PullRepoResult
	Fetched  int
	Pulled   int
	Skipped  int
	Errors   int
	Duration time.Duration
}

// PullOpts configures PullAllRepos.
type PullOpts struct {
	// ReposRoot is the directory to walk for repos. Defaults to
	// <basePath>/repos when empty.
	ReposRoot string
	// PerRepoTimeout bounds each git operation. Defaults to 60s.
	PerRepoTimeout time.Duration
	// FetchOnly skips the pull step and only runs `git fetch`.
	FetchOnly bool
}

// PullAllRepos walks the repos root (default: <basePath>/repos) looking
// for directories that contain `.git`, runs `git fetch --all --prune` on
// each, and fast-forwards only when the working tree is clean, HEAD is
// on the default branch, and an upstream is configured.
//
// Errors inside a single repo never abort the walk — they're recorded in
// the per-repo result. A top-level error is only returned if the root
// itself is not accessible.
func PullAllRepos(basePath string, opts PullOpts) (PullSummary, error) {
	start := time.Now()

	root := opts.ReposRoot
	if root == "" {
		root = filepath.Join(basePath, "repos")
	}

	timeout := opts.PerRepoTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	if _, err := os.Stat(root); err != nil {
		return PullSummary{}, fmt.Errorf("repos root not accessible: %w", err)
	}

	repos, err := FindGitRepos(root)
	if err != nil {
		return PullSummary{}, fmt.Errorf("scanning repos: %w", err)
	}

	summary := PullSummary{Repos: make([]PullRepoResult, 0, len(repos))}
	for _, repo := range repos {
		res := pullOneRepo(repo, timeout, opts.FetchOnly)
		summary.Repos = append(summary.Repos, res)
		switch res.Action {
		case "pulled":
			summary.Pulled++
		case "fetched":
			summary.Fetched++
		case "error":
			summary.Errors++
		default:
			summary.Skipped++
		}
	}
	summary.Duration = time.Since(start)
	return summary, nil
}

// FindGitRepos walks root recursively and returns directories that
// contain a `.git` entry. Stops descending once a repo is found so
// submodules and nested worktrees don't multiply the list. Hidden dirs
// and common build/vendor dirs are skipped.
func FindGitRepos(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			out = append(out, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func pullOneRepo(repo string, timeout time.Duration, fetchOnly bool) PullRepoResult {
	res := PullRepoResult{Path: repo}

	if err := pullRunGit(repo, timeout, "fetch", "--all", "--prune", "--quiet"); err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("fetch: %w", err)
		return res
	}

	if fetchOnly {
		res.Action = "fetched"
		return res
	}

	def, err := pullRunGitOutput(repo, timeout, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		res.Action = "fetched"
		return res
	}
	def = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(def), "refs/remotes/origin/"))
	res.Default = def

	branch, err := pullRunGitOutput(repo, timeout, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		res.Action = "fetched"
		return res
	}
	branch = strings.TrimSpace(branch)
	res.Branch = branch

	if branch != def {
		res.Action = "skipped-branch"
		return res
	}

	status, err := pullRunGitOutput(repo, timeout, "status", "--porcelain")
	if err != nil {
		res.Action = "fetched"
		return res
	}
	if strings.TrimSpace(status) != "" {
		res.Action = "skipped-dirty"
		return res
	}

	if _, err := pullRunGitOutput(repo, timeout, "rev-parse", "--abbrev-ref", "@{u}"); err != nil {
		res.Action = "skipped-no-upstream"
		return res
	}

	if err := pullRunGit(repo, timeout, "pull", "--ff-only", "--quiet"); err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("pull: %w", err)
		return res
	}
	res.Action = "pulled"
	return res
}

func pullRunGit(dir string, timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pullRunGitOutput(dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Format renders a short summary line for logs/CLI output.
func (s PullSummary) Format() string {
	return fmt.Sprintf(
		"%d repos: pulled=%d fetched=%d skipped=%d errors=%d in %s",
		len(s.Repos), s.Pulled, s.Fetched, s.Skipped, s.Errors, s.Duration.Round(time.Millisecond),
	)
}

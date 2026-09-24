package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PullRepoResult is the outcome of syncing a single repo via PullAllRepos.
type PullRepoResult struct {
	Path    string
	Action  string // "pulled" | "up-to-date" | "fetched" | "skipped-dirty" | "skipped-ahead" | "skipped-no-remote" | "error"
	Branch  string
	Default string
	Err     error
}

// PullSummary aggregates results across all synced repos.
type PullSummary struct {
	Repos    []PullRepoResult
	Fetched  int
	Pulled   int
	UpToDate int
	Skipped  int
	Errors   int
	Duration time.Duration
}

// PullOpts configures PullAllRepos.
type PullOpts struct {
	// ReposRoot is the directory to walk for repos. Defaults to
	// <basePath>/repos when empty.
	ReposRoot string
	// PerRepoTimeout bounds each git operation. Defaults to 5m —
	// concurrent fetches share bandwidth, so a large repo can take
	// far longer than it would alone.
	PerRepoTimeout time.Duration
	// FetchOnly skips the checkout/reset step and only fetches.
	FetchOnly bool
	// Depth is the history depth fetched per repo that is already
	// shallow. Defaults to 1 — the repos/ clones are local mirrors of
	// the latest state, not history archives. Full (non-shallow)
	// clones always fetch normally: a --depth fetch would re-shallow
	// them and destroy deliberately fetched history. Ignored when
	// FullHistory is set.
	Depth int
	// FullHistory unshallows shallow repos instead of keeping them at
	// --depth. Full clones are unaffected (they already fetch fully).
	FullHistory bool
	// Concurrency is how many repos sync in parallel. Defaults to 8.
	Concurrency int
	// OnResult, when set, is invoked (serialized) as each repo
	// finishes so callers can stream progress.
	OnResult func(PullRepoResult)
}

// PullAllRepos walks the repos root (default: <basePath>/repos) looking
// for directories that contain `.git` and brings each to the latest tip
// of its origin default branch: resolve origin/HEAD (running
// `remote set-head --auto` when unset), shallow-fetch that branch, and
// `checkout -B <default> origin/<default>` so the repo ends up checked
// out on the default branch at the latest commit — regardless of what
// branch it was on before.
//
// Repos with tracked modifications are skipped (untracked files don't
// block). Errors inside a single repo never abort the walk — they're
// recorded in the per-repo result. A top-level error is only returned
// if the root itself is not accessible.
func PullAllRepos(basePath string, opts PullOpts) (PullSummary, error) {
	start := time.Now()

	root := opts.ReposRoot
	if root == "" {
		root = filepath.Join(basePath, "repos")
	}

	timeout := opts.PerRepoTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}

	if _, err := os.Stat(root); err != nil {
		return PullSummary{}, fmt.Errorf("repos root not accessible: %w", err)
	}

	repos, err := FindGitRepos(root)
	if err != nil {
		return PullSummary{}, fmt.Errorf("scanning repos: %w", err)
	}

	results := make([]PullRepoResult, len(repos))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := pullOneRepo(repo, timeout, opts)
			results[i] = res
			if opts.OnResult != nil {
				mu.Lock()
				opts.OnResult(res)
				mu.Unlock()
			}
		}(i, repo)
	}
	wg.Wait()

	summary := PullSummary{Repos: results}
	for _, res := range results {
		switch res.Action {
		case "pulled":
			summary.Pulled++
		case "up-to-date":
			summary.UpToDate++
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
			// The WalkDir callback contract inverts the usual meaning of a nil
			// return: it means "keep walking", not "no error occurred". An
			// unreadable entry (permissions, a race with a concurrent delete)
			// is skipped so one bad directory cannot abort the whole scan.
			//nolint:nilerr // WalkDir contract: nil continues the walk; the entry is skipped
			return nil
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

func pullOneRepo(repo string, timeout time.Duration, opts PullOpts) PullRepoResult {
	res := PullRepoResult{Path: repo}

	if _, err := pullRunGitOutput(repo, timeout, "remote", "get-url", "origin"); err != nil {
		res.Action = "skipped-no-remote"
		return res
	}

	def, err := resolveDefaultBranch(repo, timeout)
	if err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("resolve default branch: %w", err)
		return res
	}
	res.Default = def

	// Shallow-aware fetch: only repos that are ALREADY shallow stay at
	// --depth. A full clone got its history on purpose (e.g. a repo
	// being developed in place) — fetching it with --depth would
	// re-shallow it and destroy that history.
	shallow, err := pullRunGitOutput(repo, timeout, "rev-parse", "--is-shallow-repository")
	if err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("check shallow: %w", err)
		return res
	}
	isShallow := strings.TrimSpace(shallow) == "true"

	fetchArgs := []string{"fetch", "--prune", "--quiet"}
	if isShallow {
		if opts.FullHistory {
			fetchArgs = append(fetchArgs, "--unshallow")
		} else {
			depth := opts.Depth
			if depth <= 0 {
				depth = 1
			}
			fetchArgs = append(fetchArgs, fmt.Sprintf("--depth=%d", depth))
		}
	}
	fetchArgs = append(fetchArgs, "origin", def)
	if err := pullRunGit(repo, timeout, fetchArgs...); err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("fetch: %w", err)
		return res
	}

	if opts.FetchOnly {
		res.Action = "fetched"
		return res
	}

	branch, err := pullRunGitOutput(repo, timeout, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		res.Action = "fetched"
		return res
	}
	res.Branch = strings.TrimSpace(branch)

	// Tracked modifications block the checkout; untracked files don't.
	status, err := pullRunGitOutput(repo, timeout, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		res.Action = "fetched"
		return res
	}
	if strings.TrimSpace(status) != "" {
		res.Action = "skipped-dirty"
		return res
	}

	if res.Branch == def && sameCommit(repo, timeout, "HEAD", "refs/remotes/origin/"+def) {
		res.Action = "up-to-date"
		return res
	}

	// Never orphan unpushed work: checkout -B resets the local default
	// branch, so if it holds commits origin doesn't have, skip. Only
	// full clones get this check — in a shallow repo consecutive
	// --depth fetches leave old and new tips disconnected, so ancestry
	// counting always reads as "ahead" and would wedge the repo. A
	// shallow repo is a latest-only mirror by construction; unshallow
	// it (--full-history or `git fetch --unshallow`) to develop in it
	// with this protection. (A rev-list error means no local default
	// branch exists — nothing to orphan, proceed.)
	if !isShallow {
		ahead, err := pullRunGitOutput(repo, timeout,
			"rev-list", "--count", "refs/remotes/origin/"+def+"..refs/heads/"+def)
		if err == nil && strings.TrimSpace(ahead) != "0" {
			res.Action = "skipped-ahead"
			return res
		}
	}

	if err := pullRunGit(repo, timeout, "checkout", "--quiet", "-B", def, "refs/remotes/origin/"+def); err != nil {
		res.Action = "error"
		res.Err = fmt.Errorf("checkout %s: %w", def, err)
		return res
	}
	res.Branch = def
	res.Action = "pulled"
	return res
}

// resolveDefaultBranch returns the origin default branch, asking the
// remote (`remote set-head --auto`) when the local origin/HEAD symref
// was never recorded.
func resolveDefaultBranch(repo string, timeout time.Duration) (string, error) {
	out, err := pullRunGitOutput(repo, timeout, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		if err := pullRunGit(repo, timeout, "remote", "set-head", "origin", "--auto"); err != nil {
			return "", err
		}
		out, err = pullRunGitOutput(repo, timeout, "symbolic-ref", "refs/remotes/origin/HEAD")
		if err != nil {
			return "", err
		}
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/origin/"), nil
}

func sameCommit(repo string, timeout time.Duration, a, b string) bool {
	ra, errA := pullRunGitOutput(repo, timeout, "rev-parse", a)
	rb, errB := pullRunGitOutput(repo, timeout, "rev-parse", b)
	return errA == nil && errB == nil && strings.TrimSpace(ra) == strings.TrimSpace(rb)
}

// pullGitEnv disables interactive credential/SSH prompts so a repo
// that would ask for auth fails fast instead of hanging a bulk sync.
func pullGitEnv() []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	return env
}

func pullRunGit(dir string, timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = pullGitEnv()
	// Without WaitDelay, a grandchild (ssh, remote helper) that inherits
	// the output pipe keeps CombinedOutput blocked past the timeout kill.
	cmd.WaitDelay = 5 * time.Second
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
	cmd.Env = pullGitEnv()
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Format renders a short summary line for logs/CLI output.
func (s PullSummary) Format() string {
	return fmt.Sprintf(
		"%d repos: pulled=%d up-to-date=%d fetched=%d skipped=%d errors=%d in %s",
		len(s.Repos), s.Pulled, s.UpToDate, s.Fetched, s.Skipped, s.Errors, s.Duration.Round(time.Millisecond),
	)
}

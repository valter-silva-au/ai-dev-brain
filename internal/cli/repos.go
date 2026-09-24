package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewReposCmd creates the `adb repos` command group.
func NewReposCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "Manage cloned repositories under <workspace>/repos",
		Long: `Operations across every git repository under <workspace>/repos.

Used in one-shot form directly (` + "`adb repo pull`" + `) or scheduled via
the adb scheduler (see ` + "`adb scheduler`" + `).`,
	}
	cmd.AddCommand(newReposPullCmd(), newReposListCmd())
	return cmd
}

// repoRegistryEntry is one repo in the derived multi-repo registry: a repo and
// the ticket IDs that span it (#213). The registry is a derivation over
// backlog.yaml — the single source of truth — not a stored list.
type repoRegistryEntry struct {
	Repo    string   `json:"repo"`
	Tickets []string `json:"tickets"`
}

// buildRepoRegistry derives the distinct repos referenced by non-archived
// tickets and the ticket IDs spanning each, sorted for stable output (#213).
func buildRepoRegistry(tasks []models.Task) []repoRegistryEntry {
	byRepo := map[string][]string{}
	for _, t := range tasks {
		if t.Repo == "" || t.Status == models.TaskStatusArchived {
			continue
		}
		byRepo[t.Repo] = append(byRepo[t.Repo], t.ID)
	}
	entries := make([]repoRegistryEntry, 0, len(byRepo))
	for repo, ids := range byRepo {
		sort.Strings(ids)
		entries = append(entries, repoRegistryEntry{Repo: repo, Tickets: ids})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Repo < entries[j].Repo })
	return entries
}

// distinctRepos returns the ordered, deduped repos of non-archived tickets
// matching pred — the basis for correlated, ticket-aware cross-repo pulls (#213).
func distinctRepos(tasks []models.Task, pred func(models.Task) bool) []string {
	seen := map[string]bool{}
	var repos []string
	for _, t := range tasks {
		if t.Repo == "" || t.Status == models.TaskStatusArchived || !pred(t) {
			continue
		}
		if !seen[t.Repo] {
			seen[t.Repo] = true
			repos = append(repos, t.Repo)
		}
	}
	sort.Strings(repos)
	return repos
}

func newReposListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repos referenced by tickets (the multi-repo registry)",
		Long: `Derive, from backlog.yaml, the distinct repos across non-archived tickets
and which tickets span each. The registry is a derivation over the single
source of truth — no separate .mrconfig to author or keep in sync (#213).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			backlog, err := App.BacklogManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load backlog: %w", err)
			}
			reg := buildRepoRegistry(backlog.Tasks)
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(reg)
			}
			if len(reg) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No repos referenced by any ticket.")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "REPO\tTICKETS\tIDS")
			for _, e := range reg {
				fmt.Fprintf(tw, "%s\t%d\t%v\n", e.Repo, len(e.Tickets), e.Tickets)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the registry as JSON")
	return cmd
}

func newReposPullCmd() *cobra.Command {
	var (
		fetchOnly   bool
		fullHistory bool
		timeout     time.Duration
		root        string
		initiative  string
		ticket      string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Bring every repo under <workspace>/repos to the latest default branch",
		Long: `Walks <workspace>/repos recursively for git repositories and, for each:

  - resolves the origin default branch (asking the remote when unset),
  - fetches its latest tip — shallow repos stay at --depth 1 (or are
    unshallowed with --full-history); full clones always fetch their
    complete history, so a deliberately unshallowed repo is never
    re-shallowed,
  - checks the repo out on that branch at the fetched tip, regardless of
    what branch it was on before.

The repos/ tree is treated as a local mirror of the latest upstream state,
not a place for in-flight work — task worktrees live under work/. Repos with
tracked modifications are skipped rather than treated as errors.

With --initiative or --ticket the pull is correlated: only the repos that unit
of work spans (derived from backlog.yaml) are fetched/pulled (#213).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			opts := integration.PullOpts{PerRepoTimeout: timeout, FetchOnly: fetchOnly, FullHistory: fullHistory}

			// Ticket-aware (correlated) pull: restrict to the repos the given
			// initiative/ticket spans, reusing the same pull engine per repo.
			if initiative != "" || ticket != "" {
				backlog, err := App.BacklogManager.Load()
				if err != nil {
					return fmt.Errorf("failed to load backlog: %w", err)
				}
				var repos []string
				if initiative != "" {
					repos = distinctRepos(backlog.Tasks, func(t models.Task) bool { return t.Initiative == initiative })
				} else {
					repos = distinctRepos(backlog.Tasks, func(t models.Task) bool { return t.ID == ticket })
				}
				if len(repos) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No repos found for that selector.")
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Correlated pull across %d repo(s):\n", len(repos))
				// Stream per-repo results as they land, same as the main path.
				opts.OnResult = func(r integration.PullRepoResult) {
					printPullResult(cmd.OutOrStdout(), r)
				}
				for _, repo := range repos {
					repoDir, derr := repoCloneDirFor(repo)
					if derr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "  skip %s: %v\n", repo, derr)
						continue
					}
					perRepo := opts
					perRepo.ReposRoot = repoDir
					if _, perr := integration.PullAllRepos(App.BasePath, perRepo); perr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %v\n", repo, perr)
					}
				}
				return nil
			}

			opts.ReposRoot = root
			if opts.ReposRoot == "" {
				opts.ReposRoot = filepath.Join(App.BasePath, "repos")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Scanning %s...\n", opts.ReposRoot)
			opts.OnResult = func(r integration.PullRepoResult) {
				printPullResult(cmd.OutOrStdout(), r)
			}
			summary, err := integration.PullAllRepos(App.BasePath, opts)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), summary.Format())
			return nil
		},
	}
	cmd.Flags().BoolVar(&fetchOnly, "fetch-only", false, "Only fetch — never check out the default branch")
	cmd.Flags().BoolVar(&fullHistory, "full-history", false, "Unshallow shallow repos (fetch complete history) instead of keeping them at --depth=1")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Per-repo timeout for each git command")
	cmd.Flags().StringVar(&root, "root", "", "Override repos root (default: <workspace>/repos)")
	cmd.Flags().StringVar(&initiative, "initiative", "", "Correlated pull: only repos this initiative's tickets span")
	cmd.Flags().StringVar(&ticket, "ticket", "", "Correlated pull: only the repo(s) this ticket spans")
	return cmd
}

// printPullResult renders one streamed per-repo pull result line, shared by
// the main and correlated pull paths.
func printPullResult(w io.Writer, r integration.PullRepoResult) {
	status := r.Action
	if r.Err != nil {
		status = fmt.Sprintf("%s: %v", r.Action, r.Err)
	}
	fmt.Fprintf(w, "  %-20s %s\n", status, r.Path)
}

// repoCloneDirFor resolves a platform-qualified repo to its local clone dir,
// mirroring the worktree manager's layout (<workspace>/repos/<normalized>).
func repoCloneDirFor(repo string) (string, error) {
	norm, err := App.GitWorktreeManager.NormalizeRepoPath(repo)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(norm) {
		return norm, nil
	}
	return filepath.Join(App.BasePath, "repos", norm), nil
}

package cli

import (
	"encoding/json"
	"fmt"
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

Used in one-shot form directly (` + "`adb repos pull`" + `) or scheduled via
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
		fetchOnly  bool
		timeout    time.Duration
		root       string
		initiative string
		ticket     string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch and fast-forward repos under <workspace>/repos (optionally ticket-aware)",
		Long: `Walks <workspace>/repos recursively for git repositories and:

  - runs 'git fetch --all --prune' on each,
  - runs 'git pull --ff-only' only if the working tree is clean, HEAD is
    on the upstream default branch, and an upstream is configured.

Dirty, non-default-branch, or unconfigured-upstream repos are recorded as
skipped rather than treated as errors.

With --initiative or --ticket the pull is correlated: only the repos that unit
of work spans (derived from backlog.yaml) are fetched/pulled (#213).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			opts := integration.PullOpts{PerRepoTimeout: timeout, FetchOnly: fetchOnly}

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
				for _, repo := range repos {
					repoDir, derr := repoCloneDirFor(repo)
					if derr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "  skip %s: %v\n", repo, derr)
						continue
					}
					perRepo := opts
					perRepo.ReposRoot = repoDir
					summary, perr := integration.PullAllRepos(App.BasePath, perRepo)
					if perr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %v\n", repo, perr)
						continue
					}
					for _, r := range summary.Repos {
						status := r.Action
						if r.Err != nil {
							status = fmt.Sprintf("%s: %v", r.Action, r.Err)
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %s\n", status, r.Path)
					}
				}
				return nil
			}

			opts.ReposRoot = root
			if opts.ReposRoot == "" {
				opts.ReposRoot = filepath.Join(App.BasePath, "repos")
			}
			fmt.Printf("Scanning %s...\n", opts.ReposRoot)
			summary, err := integration.PullAllRepos(App.BasePath, opts)
			if err != nil {
				return err
			}
			fmt.Println(summary.Format())
			for _, r := range summary.Repos {
				status := r.Action
				if r.Err != nil {
					status = fmt.Sprintf("%s: %v", r.Action, r.Err)
				}
				fmt.Printf("  %-20s %s\n", status, r.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fetchOnly, "fetch-only", false, "Only fetch — never fast-forward")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Per-repo timeout for each git command")
	cmd.Flags().StringVar(&root, "root", "", "Override repos root (default: <workspace>/repos)")
	cmd.Flags().StringVar(&initiative, "initiative", "", "Correlated pull: only repos this initiative's tickets span")
	cmd.Flags().StringVar(&ticket, "ticket", "", "Correlated pull: only the repo(s) this ticket spans")
	return cmd
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

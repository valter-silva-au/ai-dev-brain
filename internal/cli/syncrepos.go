package cli

import (
	"fmt"
	"strings"

	"github.com/drapaimern/ai-dev-brain/internal/integration"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// RepoSyncMgr is set during App init in app.go.
var RepoSyncMgr *integration.RepoSyncManager

var syncReposCmd = &cobra.Command{
	Use:   "sync-repos",
	Short: "Fetch, prune, and clean all tracked repositories",
	Long: `Synchronise all git repositories under the repos/ directory.

For each discovered repository this command:
  - Fetches all remotes and prunes stale remote-tracking branches
  - Fast-forwards the default branch if behind origin
  - Deletes local branches that have been merged into the default branch

Branches associated with active backlog tasks are never deleted.

Results are printed per repository with a summary at the end.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if RepoSyncMgr == nil {
			return fmt.Errorf("repo sync manager not initialized")
		}

		// Build a map of protected branches from the backlog. Any branch
		// associated with a task that is not done or archived must be
		// preserved.
		protected, err := buildProtectedBranches()
		if err != nil {
			return fmt.Errorf("loading backlog: %w", err)
		}

		results, err := RepoSyncMgr.SyncAll(protected)
		if err != nil {
			return fmt.Errorf("syncing repositories: %w", err)
		}

		if len(results) == 0 {
			fmt.Println("No repositories found under repos/.")
			return nil
		}

		totalDeleted := 0
		totalSkipped := 0
		totalErrors := 0

		for _, r := range results {
			fmt.Printf("\n== %s ==\n", r.RepoPath)

			if r.Error != nil {
				fmt.Printf("  ERROR: %s\n", r.Error)
				totalErrors++
				continue
			}

			if r.Fetched {
				fmt.Println("  Fetched: yes")
			} else {
				fmt.Println("  Fetched: no")
			}

			if r.DefaultBranch != "" {
				fmt.Printf("  Default branch: %s\n", r.DefaultBranch)
			}

			if len(r.BranchesDeleted) > 0 {
				fmt.Printf("  Deleted branches: %s\n", strings.Join(r.BranchesDeleted, ", "))
				totalDeleted += len(r.BranchesDeleted)
			}

			if len(r.BranchesSkipped) > 0 {
				fmt.Printf("  Skipped (active tasks): %s\n", strings.Join(r.BranchesSkipped, ", "))
				totalSkipped += len(r.BranchesSkipped)
			}
		}

		fmt.Printf("\nSynced %d repos, deleted %d branches, skipped %d (active), %d errors\n",
			len(results), totalDeleted, totalSkipped, totalErrors)
		return nil
	},
}

// buildProtectedBranches loads all tasks from the backlog and returns a map
// of normalised repo paths to sets of branch names that must not be deleted.
// Only tasks in done or archived status are excluded from protection.
func buildProtectedBranches() (map[string]map[string]bool, error) {
	protected := make(map[string]map[string]bool)

	if TaskMgr == nil {
		return protected, nil
	}

	tasks, err := TaskMgr.GetAllTasks()
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		if t.Repo == "" || t.Branch == "" {
			continue
		}
		// Only protect branches for tasks that are still active.
		if t.Status == models.StatusDone || t.Status == models.StatusArchived {
			continue
		}
		normalized := integration.NormalizeRepoPath(t.Repo)
		if protected[normalized] == nil {
			protected[normalized] = make(map[string]bool)
		}
		protected[normalized][t.Branch] = true
	}

	return protected, nil
}

func init() {
	rootCmd.AddCommand(syncReposCmd)
}

package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

// completeTaskIDs returns a completion function that lists task IDs,
// optionally filtered to exclude certain statuses.
func completeTaskIDs(excludeStatuses ...models.TaskStatus) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if TaskMgr == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		tasks, err := TaskMgr.GetAllTasks()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		exclude := make(map[models.TaskStatus]bool)
		for _, s := range excludeStatuses {
			exclude[s] = true
		}

		var ids []string
		for _, task := range tasks {
			if exclude[task.Status] {
				continue
			}
			if toComplete == "" || strings.HasPrefix(task.ID, toComplete) {
				// Include branch name as description for better UX.
				ids = append(ids, task.ID+"\t"+string(task.Type)+": "+task.Branch)
			}
		}

		return ids, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeRepoPaths returns a completion function that lists repository paths
// under basePath/repos/ in platform/org/repo format.
func completeRepoPaths(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if BasePath == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	reposDir := filepath.Join(BasePath, "repos")
	pattern := filepath.Join(reposDir, "*", "*", "*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var repos []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() {
			continue
		}
		// Convert absolute path to platform/org/repo format.
		rel, err := filepath.Rel(reposDir, match)
		if err != nil {
			continue
		}
		if toComplete == "" || strings.HasPrefix(rel, toComplete) {
			repos = append(repos, rel)
		}
	}

	return repos, cobra.ShellCompDirectiveNoFileComp
}

// completePriorities returns a completion function for priority values.
func completePriorities(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"P0\tCritical",
		"P1\tHigh",
		"P2\tMedium",
		"P3\tLow",
	}, cobra.ShellCompDirectiveNoFileComp
}

// completeStatuses returns a completion function for task status values.
func completeStatuses(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"backlog\tQueued for future work",
		"in_progress\tActively being worked on",
		"blocked\tWaiting on dependency",
		"review\tIn code review",
		"done\tCompleted",
		"archived\tArchived with handoff",
	}, cobra.ShellCompDirectiveNoFileComp
}

// registerTaskCommandCompletions registers flag completion functions on a task
// creation command (feat, bug, spike, refactor).
func registerTaskCommandCompletions(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("repo", completeRepoPaths)
	_ = cmd.RegisterFlagCompletionFunc("priority", completePriorities)
}

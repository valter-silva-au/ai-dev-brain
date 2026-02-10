package cli

import (
	"fmt"
	"strings"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

var statusFilter string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display tasks grouped by status",
	Long: `Display all tasks organized by their lifecycle status.

Optionally filter to a single status using --filter (e.g. --filter in_progress).
Output is formatted as a table with columns: ID, Priority, Type, Branch.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		if statusFilter != "" {
			// Show tasks for a single status.
			status := models.TaskStatus(statusFilter)
			tasks, err := TaskMgr.GetTasksByStatus(status)
			if err != nil {
				return fmt.Errorf("fetching tasks: %w", err)
			}
			printStatusGroup(string(status), tasks)
			return nil
		}

		// Show all tasks grouped by status.
		tasks, err := TaskMgr.GetAllTasks()
		if err != nil {
			return fmt.Errorf("fetching tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}

		// Group tasks by status in lifecycle order.
		statusOrder := []models.TaskStatus{
			models.StatusInProgress,
			models.StatusBlocked,
			models.StatusReview,
			models.StatusBacklog,
			models.StatusDone,
			models.StatusArchived,
		}

		grouped := make(map[models.TaskStatus][]*models.Task)
		for _, task := range tasks {
			grouped[task.Status] = append(grouped[task.Status], task)
		}

		for _, status := range statusOrder {
			if group, ok := grouped[status]; ok && len(group) > 0 {
				printStatusGroup(string(status), group)
				fmt.Println()
			}
		}

		return nil
	},
}

// printStatusGroup prints a table of tasks under a status heading.
func printStatusGroup(status string, tasks []*models.Task) {
	fmt.Printf("== %s (%d) ==\n", strings.ToUpper(status), len(tasks))
	fmt.Printf("  %-12s %-4s %-10s %s\n", "ID", "PRI", "TYPE", "BRANCH")
	fmt.Printf("  %-12s %-4s %-10s %s\n", "----", "---", "----", "------")
	for _, task := range tasks {
		fmt.Printf("  %-12s %-4s %-10s %s\n", task.ID, task.Priority, task.Type, task.Branch)
	}
}

func init() {
	statusCmd.Flags().StringVar(&statusFilter, "filter", "", "Filter by status (backlog, in_progress, blocked, review, done, archived)")
	rootCmd.AddCommand(statusCmd)
}

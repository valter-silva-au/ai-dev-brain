package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// resumableStatuses defines the task statuses that can be resumed.
var resumableStatuses = map[models.TaskStatus]bool{
	models.StatusInProgress: true,
	models.StatusBlocked:    true,
	models.StatusReview:     true,
	models.StatusBacklog:    true,
}

// statusOrder defines the display order for the interactive picker
// (in_progress first, then blocked, review, backlog).
var statusOrder = []models.TaskStatus{
	models.StatusInProgress,
	models.StatusBlocked,
	models.StatusReview,
	models.StatusBacklog,
}

// pickResumableTask shows an interactive list of resumable tasks and
// returns the selected task ID. Returns an error if no tasks are
// available or the user cancels.
func pickResumableTask() (string, error) {
	if TaskMgr == nil {
		return "", fmt.Errorf("task manager not initialized")
	}

	allTasks, err := TaskMgr.GetAllTasks()
	if err != nil {
		return "", fmt.Errorf("listing tasks: %w", err)
	}

	// Filter to resumable tasks.
	var tasks []*models.Task
	for _, t := range allTasks {
		if resumableStatuses[t.Status] {
			tasks = append(tasks, t)
		}
	}

	if len(tasks) == 0 {
		return "", fmt.Errorf("no resumable tasks found (use 'adb feat <branch>' to create one)")
	}

	// Sort by status order, then by priority within each status.
	sort.Slice(tasks, func(i, j int) bool {
		si := statusIndex(tasks[i].Status)
		sj := statusIndex(tasks[j].Status)
		if si != sj {
			return si < sj
		}
		return tasks[i].Priority < tasks[j].Priority
	})

	// Display the list.
	fmt.Println("\nResumable tasks:")
	fmt.Println()
	fmt.Printf("  %-4s %-20s %-6s %-4s %-12s %s\n", "#", "ID", "TYPE", "PRI", "STATUS", "BRANCH")
	fmt.Printf("  %-4s %-20s %-6s %-4s %-12s %s\n", "---", "---", "----", "---", "------", "------")
	for i, t := range tasks {
		fmt.Printf("  %-4d %-20s %-6s %-4s %-12s %s\n",
			i+1, t.ID, t.Type, t.Priority, t.Status, t.Branch)
	}
	fmt.Println()

	// Read selection.
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Select task [1-%d] (or 'q' to cancel): ", len(tasks))
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "q" || input == "Q" {
			return "", fmt.Errorf("cancelled")
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(tasks) {
			fmt.Printf("  Invalid selection. Enter a number between 1 and %d.\n", len(tasks))
			continue
		}

		selected := tasks[num-1]
		return selected.ID, nil
	}
}

// statusIndex returns a sort key for status ordering in the picker.
func statusIndex(s models.TaskStatus) int {
	for i, status := range statusOrder {
		if s == status {
			return i
		}
	}
	return len(statusOrder)
}

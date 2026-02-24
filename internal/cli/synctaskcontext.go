package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

var syncTaskContextCmd = &cobra.Command{
	Use:   "sync-task-context",
	Short: "Regenerate task context in the current worktree",
	Long: `Regenerate .claude/rules/task-context.md in the current worktree.

In --hook-mode, this is called by a ConfigChange hook to automatically
keep the task context file up to date when configuration changes.

Without --hook-mode, it can be run manually to refresh the task context
file in the current worktree.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		hookMode, _ := cmd.Flags().GetBool("hook-mode")

		taskID := os.Getenv("ADB_TASK_ID")
		taskType := os.Getenv("ADB_TASK_TYPE")
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")
		ticketPath := os.Getenv("ADB_TICKET_PATH")
		branch := os.Getenv("ADB_BRANCH")

		if taskType == "" {
			taskType = "unknown"
		}

		if taskID == "" {
			if hookMode {
				// In hook mode, silently exit if no task context.
				return nil
			}
			return fmt.Errorf("ADB_TASK_ID not set; run this from within a task worktree")
		}

		if worktreePath == "" {
			worktreePath, _ = os.Getwd()
		}

		// Generate the task context content.
		contextContent := fmt.Sprintf(`# Task Context: %s

This worktree is for task %s (%s).

- **Type**: %s
- **Branch**: %s
- **Ticket**: %s

## Key Files
- %s/context.md -- Running context (update as you work)
- %s/notes.md -- Requirements and acceptance criteria
- %s/design.md -- Technical design document
- %s/sessions/ -- Session summaries (save progress between sessions)
- %s/knowledge/ -- Extracted decisions and facts

## Instructions
- Update context.md with progress, decisions, and blockers as you work
- Save session summaries to sessions/ when ending a work session
- Record key decisions in knowledge/decisions.yaml
`,
			taskID, taskID, branch,
			taskType, branch, ticketPath,
			ticketPath, ticketPath, ticketPath, ticketPath, ticketPath,
		)

		// Write the task context file.
		contextDir := filepath.Join(worktreePath, ".claude", "rules")
		if err := os.MkdirAll(contextDir, 0o755); err != nil {
			return fmt.Errorf("creating rules directory: %w", err)
		}

		contextPath := filepath.Join(contextDir, "task-context.md")
		if err := os.WriteFile(contextPath, []byte(contextContent), 0o644); err != nil {
			return fmt.Errorf("writing task context: %w", err)
		}

		// Log config change event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "config.task_context_synced",
				Message: "config.task_context_synced",
				Data: map[string]any{
					"task_id":       taskID,
					"worktree_path": worktreePath,
					"hook_mode":     hookMode,
				},
			})
		}

		if hookMode {
			// Silent in hook mode.
			return nil
		}

		fmt.Printf("Task context regenerated: %s\n", contextPath)
		return nil
	},
}

func init() {
	syncTaskContextCmd.Flags().Bool("hook-mode", false, "Run in hook mode (silent, non-fatal)")
	rootCmd.AddCommand(syncTaskContextCmd)
}

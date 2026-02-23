package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

var worktreeHookCmd = &cobra.Command{
	Use:   "worktree-hook",
	Short: "Handle worktree lifecycle hooks (called by Claude Code)",
	Long: `Handle worktree lifecycle hooks for Claude Code v2.1.50+.

This command is called by hook scripts in response to WorktreeCreate and
WorktreeRemove events. It validates worktree state and logs observability
events.

These commands are designed to be called from hooks and will never fail hard
to avoid blocking Claude Code operations.`,
}

var worktreeHookCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Handle WorktreeCreate hook",
	Long: `Handle the WorktreeCreate hook event from Claude Code.

Reads ADB_TASK_ID and ADB_WORKTREE_PATH from environment variables,
validates that .claude/rules/task-context.md exists in the worktree,
and logs a worktree.created event.

This command is graceful and will not fail even if environment variables
are missing or validation fails.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := os.Getenv("ADB_TASK_ID")
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")

		// Graceful: if env vars are missing, just exit successfully.
		if taskID == "" || worktreePath == "" {
			return nil
		}

		// Validate that task-context.md exists in the worktree.
		taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")
		contextExists := false
		if _, err := os.Stat(taskContextPath); err == nil {
			contextExists = true
		}

		// Log worktree.created event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.created",
				Message: "worktree.created",
				Data: map[string]any{
					"task_id":        taskID,
					"worktree_path":  worktreePath,
					"context_exists": contextExists,
				},
			})
		}

		// Print validation result (non-blocking).
		if contextExists {
			fmt.Printf("Worktree created: %s (task context validated)\n", taskID)
		} else {
			fmt.Printf("Worktree created: %s (task context missing)\n", taskID)
		}

		return nil
	},
}

var worktreeHookRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Handle WorktreeRemove hook",
	Long: `Handle the WorktreeRemove hook event from Claude Code.

Reads ADB_TASK_ID from environment variable and logs a worktree.removed event.

This command is graceful and will not fail even if environment variables
are missing.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := os.Getenv("ADB_TASK_ID")

		// Graceful: if env var is missing, just exit successfully.
		if taskID == "" {
			return nil
		}

		// Log worktree.removed event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.removed",
				Message: "worktree.removed",
				Data: map[string]any{
					"task_id": taskID,
				},
			})
		}

		fmt.Printf("Worktree removed: %s\n", taskID)
		return nil
	},
}

var worktreeHookViolationCmd = &cobra.Command{
	Use:   "violation",
	Short: "Log a worktree boundary violation",
	Long: `Log a worktree isolation violation event to .adb_events.jsonl with HIGH severity.

Called by the PreToolUse boundary validation hook when a tool attempts to
access a path outside the worktree boundary.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		violationPath, _ := cmd.Flags().GetString("path")
		taskID := os.Getenv("ADB_TASK_ID")
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")

		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "WARN",
				Type:    "worktree.isolation_violation",
				Message: "worktree.isolation_violation",
				Data: map[string]any{
					"task_id":        taskID,
					"worktree_path":  worktreePath,
					"violation_path": violationPath,
					"severity":       "HIGH",
				},
			})
		}

		fmt.Fprintf(os.Stderr, "Isolation violation: %s (outside %s)\n", violationPath, worktreePath)
		return nil
	},
}

func init() {
	worktreeHookViolationCmd.Flags().String("path", "", "The path that violated the boundary")
	worktreeHookCmd.AddCommand(worktreeHookCreateCmd)
	worktreeHookCmd.AddCommand(worktreeHookRemoveCmd)
	worktreeHookCmd.AddCommand(worktreeHookViolationCmd)
	rootCmd.AddCommand(worktreeHookCmd)
}

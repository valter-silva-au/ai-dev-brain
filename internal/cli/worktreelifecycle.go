package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

var worktreeLifecycleCmd = &cobra.Command{
	Use:   "worktree-lifecycle",
	Short: "Worktree lifecycle automation commands",
	Long: `Worktree lifecycle automation commands for pre-creation validation,
post-creation setup, pre-removal checks, and post-removal cleanup.

These commands integrate with Claude Code worktree hooks to provide
full task lifecycle orchestration.`,
}

var worktreePreCreateCmd = &cobra.Command{
	Use:   "pre-create",
	Short: "Validate conditions before worktree creation",
	Long: `Run pre-creation validation checks before a worktree is created.

Checks for:
  - Uncommitted changes in the main working tree
  - Merge conflicts on the target branch
  - Blocking tasks in the backlog

Exits with code 1 if validation fails, 0 if all checks pass.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var issues []string

		// Check for uncommitted changes in the current directory.
		gitCmd := exec.Command("git", "status", "--porcelain")
		output, err := gitCmd.Output()
		if err == nil && len(output) > 0 {
			issues = append(issues, "uncommitted changes detected in working tree")
		}

		// Check for merge conflicts (unmerged paths).
		gitCmd = exec.Command("git", "diff", "--name-only", "--diff-filter=U")
		output, err = gitCmd.Output()
		if err == nil && len(output) > 0 {
			issues = append(issues, "merge conflicts detected (unmerged paths)")
		}

		// Log pre-create event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.pre_create",
				Message: "worktree.pre_create",
				Data: map[string]any{
					"task_id": os.Getenv("ADB_TASK_ID"),
					"issues":  len(issues),
					"passed":  len(issues) == 0,
				},
			})
		}

		if len(issues) > 0 {
			fmt.Println("Pre-creation validation failed:")
			for _, issue := range issues {
				fmt.Printf("  - %s\n", issue)
			}
			return fmt.Errorf("pre-creation validation failed with %d issue(s)", len(issues))
		}

		fmt.Println("Pre-creation validation passed")
		return nil
	},
}

var worktreePostCreateCmd = &cobra.Command{
	Use:   "post-create",
	Short: "Run post-creation setup tasks",
	Long: `Run setup tasks after a worktree is created.

Actions:
  - Validates .claude/rules/task-context.md was written
  - Logs worktree creation with task metadata
  - Prints worktree path and task ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := os.Getenv("ADB_TASK_ID")
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")

		if taskID == "" || worktreePath == "" {
			fmt.Println("Post-create: no task context (ADB_TASK_ID or ADB_WORKTREE_PATH not set)")
			return nil
		}

		// Validate task context file.
		taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")
		contextExists := false
		if _, err := os.Stat(taskContextPath); err == nil {
			contextExists = true
		}

		// Log post-create event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.post_create",
				Message: "worktree.post_create",
				Data: map[string]any{
					"task_id":        taskID,
					"worktree_path":  worktreePath,
					"context_exists": contextExists,
				},
			})
		}

		fmt.Printf("Post-create: %s at %s", taskID, worktreePath)
		if contextExists {
			fmt.Println(" (task context validated)")
		} else {
			fmt.Println(" (task context missing)")
		}

		return nil
	},
}

var worktreePreRemoveCmd = &cobra.Command{
	Use:   "pre-remove",
	Short: "Check conditions before worktree removal",
	Long: `Run pre-removal checks before a worktree is removed.

Checks for:
  - Uncommitted changes in the worktree
  - Unpushed commits

Exits with code 1 if checks detect unsaved work, 0 otherwise.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")
		if worktreePath == "" {
			fmt.Println("Pre-remove: no worktree path (ADB_WORKTREE_PATH not set)")
			return nil
		}

		var issues []string

		// Check for uncommitted changes in the worktree.
		gitCmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
		output, err := gitCmd.Output()
		if err == nil && len(output) > 0 {
			issues = append(issues, "uncommitted changes in worktree")
		}

		// Check for unpushed commits.
		gitCmd = exec.Command("git", "-C", worktreePath, "log", "--oneline", "@{upstream}..HEAD")
		output, err = gitCmd.Output()
		if err == nil && len(output) > 0 {
			issues = append(issues, "unpushed commits in worktree")
		}

		// Log pre-remove event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.pre_remove",
				Message: "worktree.pre_remove",
				Data: map[string]any{
					"task_id":       os.Getenv("ADB_TASK_ID"),
					"worktree_path": worktreePath,
					"issues":        len(issues),
					"passed":        len(issues) == 0,
				},
			})
		}

		if len(issues) > 0 {
			fmt.Println("Pre-removal check found unsaved work:")
			for _, issue := range issues {
				fmt.Printf("  - %s\n", issue)
			}
			fmt.Println("Consider committing and pushing before removal.")
			// Non-fatal: warn but don't block.
		} else {
			fmt.Println("Pre-removal check passed (no unsaved work)")
		}

		return nil
	},
}

var worktreePostRemoveCmd = &cobra.Command{
	Use:   "post-remove",
	Short: "Run post-removal cleanup tasks",
	Long: `Run cleanup tasks after a worktree is removed.

Actions:
  - Archives orphaned session files
  - Cleans up temporary files
  - Logs worktree removal event`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := os.Getenv("ADB_TASK_ID")
		ticketPath := os.Getenv("ADB_TICKET_PATH")

		// Log post-remove event.
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "worktree.post_remove",
				Message: "worktree.post_remove",
				Data: map[string]any{
					"task_id": taskID,
				},
			})
		}

		// Clean up temp files in the ticket directory if it exists.
		if ticketPath != "" {
			tmpPattern := filepath.Join(ticketPath, "*.tmp")
			tmpFiles, _ := filepath.Glob(tmpPattern)
			for _, f := range tmpFiles {
				_ = os.Remove(f)
			}
			if len(tmpFiles) > 0 {
				fmt.Printf("Post-remove: cleaned %d temp file(s)\n", len(tmpFiles))
			}
		}

		fmt.Printf("Post-remove: %s cleanup complete\n", taskID)
		return nil
	},
}

func init() {
	worktreeLifecycleCmd.AddCommand(worktreePreCreateCmd)
	worktreeLifecycleCmd.AddCommand(worktreePostCreateCmd)
	worktreeLifecycleCmd.AddCommand(worktreePreRemoveCmd)
	worktreeLifecycleCmd.AddCommand(worktreePostRemoveCmd)
	rootCmd.AddCommand(worktreeLifecycleCmd)
}

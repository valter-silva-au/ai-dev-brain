package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage session summaries for tasks",
	Long:  `Commands for saving and managing session summaries that capture work progress between sessions.`,
}

var sessionSaveCmd = &cobra.Command{
	Use:   "save [task-id]",
	Short: "Save a session summary for the current task",
	Long: `Save a structured session summary to the task's sessions/ directory.

If no task-id is provided, the ADB_TASK_ID environment variable is used.
The session file is timestamped and contains sections for accomplished work,
decisions, blockers, and next steps.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := ""
		if len(args) > 0 {
			taskID = args[0]
		} else {
			taskID = os.Getenv("ADB_TASK_ID")
		}

		if taskID == "" {
			return fmt.Errorf("task ID required: provide as argument or set ADB_TASK_ID")
		}

		if BasePath == "" {
			return fmt.Errorf("base path not initialized")
		}

		ticketPath := filepath.Join(BasePath, "tickets", taskID)
		if _, err := os.Stat(ticketPath); err != nil {
			return fmt.Errorf("task %s not found: %w", taskID, err)
		}

		sessionsDir := filepath.Join(ticketPath, "sessions")
		if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
			return fmt.Errorf("creating sessions directory: %w", err)
		}

		// Read context.md to include current state in the session summary.
		contextPath := filepath.Join(ticketPath, "context.md")
		contextSummary := ""
		if data, err := os.ReadFile(contextPath); err == nil {
			contextSummary = string(data)
		}

		now := time.Now().UTC()
		filename := now.Format("2006-01-02T15-04-05Z") + ".md"
		sessionPath := filepath.Join(sessionsDir, filename)

		content := fmt.Sprintf(`# Session: %s

**Task:** %s
**Date:** %s

## Accomplished

- (describe what was completed this session)

## Decisions

- (record any decisions made)

## Blockers

- (note any blockers encountered)

## Next Steps

- (list what should happen next)
`, now.Format(time.RFC3339), taskID, now.Format("2006-01-02"))

		if contextSummary != "" {
			content += fmt.Sprintf(`
## Context Snapshot

%s
`, contextSummary)
		}

		if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing session file: %w", err)
		}

		fmt.Printf("Session saved: %s\n", sessionPath)
		fmt.Printf("Edit the file to fill in session details.\n")

		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionSaveCmd)
	rootCmd.AddCommand(sessionCmd)
}

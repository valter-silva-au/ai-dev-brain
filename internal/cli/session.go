package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// NewSessionCmd creates the session command group
func NewSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage captured Claude Code sessions",
		Long:  `Capture, save, list, and view Claude Code sessions for analysis and knowledge extraction.`,
	}

	cmd.AddCommand(newSessionSaveCmd())
	cmd.AddCommand(newSessionIngestCmd())
	cmd.AddCommand(newSessionCaptureCmd())
	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionShowCmd())

	return cmd
}

// newSessionSaveCmd saves a session from a YAML file
func newSessionSaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save <file>",
		Short: "Save a session from a YAML file",
		Long:  `Load a session from a YAML file and save it to the session store.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			// Read the file
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			// Parse YAML
			var session models.CapturedSession
			if err := yaml.Unmarshal(data, &session); err != nil {
				return fmt.Errorf("failed to parse session YAML: %w", err)
			}

			// Validate session
			if session.ID == "" {
				return fmt.Errorf("session must have an ID")
			}

			// Get session store
			sessionStore := App.GetSessionStore()

			// Save session
			if err := sessionStore.SaveSession(&session); err != nil {
				return fmt.Errorf("failed to save session: %w", err)
			}

			fmt.Printf("Session %s saved successfully\n", session.ID)
			return nil
		},
	}

	return cmd
}

// newSessionIngestCmd ingests a session from Claude Code output directory
func newSessionIngestCmd() *cobra.Command {
	var taskID string

	cmd := &cobra.Command{
		Use:   "ingest <directory>",
		Short: "Ingest a session from Claude Code output",
		Long:  `Parse Claude Code session output directory and create a captured session.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := args[0]

			// Check if directory exists
			if _, err := os.Stat(directory); os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", directory)
			}

			// Get next session ID
			sessionStore := App.GetSessionStore()
			sessionID, err := sessionStore.GetNextSessionID()
			if err != nil {
				return fmt.Errorf("failed to generate session ID: %w", err)
			}

			// Create new session
			session := models.NewCapturedSession(sessionID)
			session.TaskID = taskID

			// Try to parse transcript if exists
			transcriptPath := filepath.Join(directory, "transcript.json")
			if _, err := os.Stat(transcriptPath); err == nil {
				// TODO: Parse transcript.json and populate turns
				fmt.Printf("Found transcript: %s (parsing not implemented yet)\n", transcriptPath)
			}

			// Try to find summary
			summaryPath := filepath.Join(directory, "summary.md")
			if _, err := os.Stat(summaryPath); err == nil {
				summaryData, err := os.ReadFile(summaryPath)
				if err == nil {
					session.Summary = string(summaryData)
				}
			}

			// Finalize session
			session.Finalize()

			// Save session
			if err := sessionStore.SaveSession(session); err != nil {
				return fmt.Errorf("failed to save session: %w", err)
			}

			fmt.Printf("Session %s ingested from %s\n", session.ID, directory)
			return nil
		},
	}

	cmd.Flags().StringVar(&taskID, "task", "", "Associate session with task ID")

	return cmd
}

// newSessionCaptureCmd captures a new session interactively
func newSessionCaptureCmd() *cobra.Command {
	var taskID string
	var summary string
	var tags []string

	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture a new session interactively",
		Long:  `Create a new captured session with metadata.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get next session ID
			sessionStore := App.GetSessionStore()
			sessionID, err := sessionStore.GetNextSessionID()
			if err != nil {
				return fmt.Errorf("failed to generate session ID: %w", err)
			}

			// Create new session
			session := models.NewCapturedSession(sessionID)
			session.TaskID = taskID
			session.Summary = summary
			session.Tags = tags

			// Finalize session
			session.Finalize()

			// Save session
			if err := sessionStore.SaveSession(session); err != nil {
				return fmt.Errorf("failed to save session: %w", err)
			}

			fmt.Printf("Session %s captured\n", session.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&taskID, "task", "", "Associate session with task ID")
	cmd.Flags().StringVar(&summary, "summary", "", "Session summary")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Session tags (comma-separated)")

	return cmd
}

// newSessionListCmd lists all captured sessions
func newSessionListCmd() *cobra.Command {
	var taskID string
	var tagsStr string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all captured sessions",
		Long:  `Display a list of all captured sessions with their metadata.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionStore := App.GetSessionStore()

			// Build filter
			var filter *models.SessionFilter
			if taskID != "" || tagsStr != "" {
				filter = &models.SessionFilter{}
				if taskID != "" {
					filter.TaskID = taskID
				}
				if tagsStr != "" {
					filter.Tags = strings.Split(tagsStr, ",")
				}
			}

			// List sessions
			var sessions []*models.CapturedSession
			var err error
			if filter != nil {
				sessions, err = sessionStore.FilterSessions(filter)
			} else {
				entries, err := sessionStore.ListSessions()
				if err != nil {
					return fmt.Errorf("failed to list sessions: %w", err)
				}

				// Print table
				fmt.Printf("%-10s %-12s %-20s %-50s\n", "ID", "Task", "Start Time", "Summary")
				fmt.Println(strings.Repeat("-", 100))

				for _, entry := range entries {
					summary := entry.Summary
					if len(summary) > 47 {
						summary = summary[:47] + "..."
					}
					taskID := entry.TaskID
					if taskID == "" {
						taskID = "-"
					}
					fmt.Printf("%-10s %-12s %-20s %-50s\n", entry.ID, taskID, entry.StartTime, summary)
				}

				return nil
			}

			if err != nil {
				return fmt.Errorf("failed to filter sessions: %w", err)
			}

			// Print filtered results
			fmt.Printf("%-10s %-12s %-20s %-50s\n", "ID", "Task", "Start Time", "Summary")
			fmt.Println(strings.Repeat("-", 100))

			for _, session := range sessions {
				summary := session.Summary
				if len(summary) > 47 {
					summary = summary[:47] + "..."
				}
				taskID := session.TaskID
				if taskID == "" {
					taskID = "-"
				}
				startTime := session.StartTime.Format("2006-01-02 15:04:05")
				fmt.Printf("%-10s %-12s %-20s %-50s\n", session.ID, taskID, startTime, summary)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&taskID, "task", "", "Filter by task ID")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Filter by tags (comma-separated)")

	return cmd
}

// newSessionShowCmd shows details of a specific session
func newSessionShowCmd() *cobra.Command {
	var showTurns bool

	cmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show details of a specific session",
		Long:  `Display detailed information about a captured session including turns.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]

			sessionStore := App.GetSessionStore()

			// Get session
			session, err := sessionStore.GetSession(sessionID)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}

			// Print session details
			fmt.Printf("Session ID: %s\n", session.ID)
			if session.TaskID != "" {
				fmt.Printf("Task ID: %s\n", session.TaskID)
			}
			fmt.Printf("Start Time: %s\n", session.StartTime.Format(time.RFC3339))
			if !session.EndTime.IsZero() {
				fmt.Printf("End Time: %s\n", session.EndTime.Format(time.RFC3339))
				fmt.Printf("Duration: %d seconds\n", session.Duration)
			}

			if len(session.Tags) > 0 {
				fmt.Printf("Tags: %s\n", strings.Join(session.Tags, ", "))
			}

			if len(session.ToolsUsed) > 0 {
				fmt.Printf("Tools Used: %s\n", strings.Join(session.ToolsUsed, ", "))
			}

			if len(session.FilesEdited) > 0 {
				fmt.Printf("Files Edited: %d\n", len(session.FilesEdited))
			}

			if session.Summary != "" {
				fmt.Printf("\nSummary:\n%s\n", session.Summary)
			}

			// Show turns if requested
			if showTurns && len(session.Turns) > 0 {
				fmt.Printf("\n--- Turns (%d total) ---\n", len(session.Turns))
				for _, turn := range session.Turns {
					fmt.Printf("\n[%d] %s (%s)\n", turn.Index, turn.Role, turn.Timestamp.Format(time.RFC3339))
					fmt.Printf("%s\n", turn.Content)
					if len(turn.ToolCalls) > 0 {
						fmt.Printf("Tool Calls: %s\n", strings.Join(turn.ToolCalls, ", "))
					}
				}
			} else if len(session.Turns) > 0 {
				fmt.Printf("\nTurns: %d (use --turns to show details)\n", len(session.Turns))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&showTurns, "turns", false, "Show all turns in detail")

	return cmd
}

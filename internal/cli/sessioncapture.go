package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/integration"
	"github.com/drapaimern/ai-dev-brain/internal/observability"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"github.com/spf13/cobra"
)

var sessionCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture a Claude Code session",
	Long: `Capture a Claude Code session from a JSONL transcript.

In --from-hook mode, reads session metadata from stdin (called by the
SessionEnd hook). In manual mode, specify --transcript and --session-id.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fromHook, _ := cmd.Flags().GetBool("from-hook")
		transcriptPath, _ := cmd.Flags().GetString("transcript")
		sessionID, _ := cmd.Flags().GetString("session-id")
		projectDir, _ := cmd.Flags().GetString("project-dir")
		taskID, _ := cmd.Flags().GetString("task-id")

		if fromHook {
			// In hook mode, always exit 0. Catch all errors silently.
			if err := captureFromHook(); err != nil {
				// Swallow the error -- hook must not fail.
				_ = err
			}
			return nil
		}

		if transcriptPath == "" || sessionID == "" {
			return fmt.Errorf("either --from-hook or both --transcript and --session-id are required")
		}

		if taskID == "" {
			taskID = os.Getenv("ADB_TASK_ID")
		}

		return captureFromTranscript(transcriptPath, sessionID, projectDir, taskID)
	},
}

var sessionListCapturedCmd = &cobra.Command{
	Use:   "list",
	Short: "List captured sessions",
	Long:  `List captured sessions with optional filtering by task, time range, or project.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if SessionCapture == nil {
			return fmt.Errorf("session capture not initialized")
		}

		taskID, _ := cmd.Flags().GetString("task")
		since, _ := cmd.Flags().GetString("since")
		project, _ := cmd.Flags().GetString("project")

		filter := models.SessionFilter{
			TaskID:      taskID,
			ProjectPath: project,
		}

		if since != "" {
			t, err := parseSinceDuration(since)
			if err != nil {
				return fmt.Errorf("parsing --since: %w", err)
			}
			filter.Since = &t
		}

		sessions, err := SessionCapture.ListSessions(filter)
		if err != nil {
			return fmt.Errorf("listing sessions: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No captured sessions found.")
			return nil
		}

		fmt.Printf("%-10s  %-12s  %-8s  %5s  %s\n", "ID", "TASK", "DURATION", "TURNS", "PROJECT")
		fmt.Printf("%-10s  %-12s  %-8s  %5s  %s\n", "----------", "------------", "--------", "-----", strings.Repeat("-", 40))
		for _, s := range sessions {
			taskCol := s.TaskID
			if taskCol == "" {
				taskCol = "-"
			}
			fmt.Printf("%-10s  %-12s  %-8s  %5d  %s\n", s.ID, taskCol, s.Duration, s.TurnCount, s.ProjectPath)
		}

		return nil
	},
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <session-id>",
	Short: "Show session details",
	Long:  `Show detailed information about a captured session including turn summary.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if SessionCapture == nil {
			return fmt.Errorf("session capture not initialized")
		}

		sessionID := args[0]
		session, err := SessionCapture.GetSession(sessionID)
		if err != nil {
			return fmt.Errorf("session not found: %w", err)
		}

		fmt.Printf("Session: %s\n", session.ID)
		fmt.Printf("Claude Session ID: %s\n", session.SessionID)
		if session.TaskID != "" {
			fmt.Printf("Task: %s\n", session.TaskID)
		}
		fmt.Printf("Project: %s\n", session.ProjectPath)
		if session.GitBranch != "" {
			fmt.Printf("Branch: %s\n", session.GitBranch)
		}
		fmt.Printf("Started: %s\n", session.StartedAt.Format(time.RFC3339))
		fmt.Printf("Ended: %s\n", session.EndedAt.Format(time.RFC3339))
		fmt.Printf("Duration: %s\n", session.Duration)
		fmt.Printf("Turns: %d\n", session.TurnCount)
		if session.Summary != "" {
			fmt.Printf("\nSummary: %s\n", session.Summary)
		}
		if len(session.Tags) > 0 {
			fmt.Printf("Tags: %s\n", strings.Join(session.Tags, ", "))
		}

		// Show turn digests.
		turns, err := SessionCapture.GetSessionTurns(sessionID)
		if err != nil {
			return nil // Non-fatal: just skip turn display.
		}

		if len(turns) > 0 {
			fmt.Printf("\n--- Turns ---\n")
			for _, t := range turns {
				role := t.Role
				if role == "assistant" {
					role = "AI"
				}
				digest := t.Digest
				if digest == "" {
					digest = "(no content)"
				}
				toolInfo := ""
				if len(t.ToolsUsed) > 0 {
					toolInfo = fmt.Sprintf(" [%s]", strings.Join(t.ToolsUsed, ", "))
				}
				fmt.Printf("[%d] %s%s: %s\n", t.Index, role, toolInfo, digest)
			}
		}

		return nil
	},
}

// hookInput represents the JSON payload from a Claude Code SessionEnd hook.
type hookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
}

func captureFromHook() error {
	// Read stdin JSON.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading hook input: %w", err)
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parsing hook input: %w", err)
	}

	if input.TranscriptPath == "" {
		return fmt.Errorf("no transcript_path in hook input")
	}

	taskID := os.Getenv("ADB_TASK_ID")

	return captureFromTranscript(input.TranscriptPath, input.SessionID, input.CWD, taskID)
}

func captureFromTranscript(transcriptPath, sessionID, cwd, taskID string) error {
	if SessionCapture == nil {
		return fmt.Errorf("session capture not initialized")
	}

	// Validate transcript file exists.
	if _, err := os.Stat(transcriptPath); err != nil {
		return fmt.Errorf("transcript file not found: %w", err)
	}

	// Parse the transcript.
	parser := integration.NewTranscriptParser()
	result, err := parser.ParseTranscript(transcriptPath)
	if err != nil {
		return fmt.Errorf("parsing transcript: %w", err)
	}

	if len(result.Turns) == 0 {
		return nil // Nothing to capture.
	}

	// Check minimum turn count (default 3).
	minTurns := 3
	if len(result.Turns) < minTurns {
		return nil // Too few turns; skip.
	}

	// Compute duration.
	var duration time.Duration
	startedAt := result.StartedAt
	endedAt := result.EndedAt
	if !startedAt.IsZero() && !endedAt.IsZero() {
		duration = endedAt.Sub(startedAt)
	}

	// Generate session ID via the store.
	storeID, err := SessionCapture.GenerateID()
	if err != nil {
		return fmt.Errorf("generating session ID: %w", err)
	}

	// Build summary.
	summary := result.Summary
	if summary == "" {
		summarizer := &integration.StructuralSummarizer{}
		summary = summarizer.Summarize(result.Turns)
	}

	// Detect git branch.
	gitBranch := captureDetectGitBranch(cwd)

	session := models.CapturedSession{
		ID:          storeID,
		SessionID:   sessionID,
		TaskID:      taskID,
		ProjectPath: cwd,
		GitBranch:   gitBranch,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
		Duration:    formatDuration(duration),
		TurnCount:   len(result.Turns),
		Summary:     summary,
	}

	id, err := SessionCapture.CaptureSession(session, result.Turns)
	if err != nil {
		return fmt.Errorf("storing session: %w", err)
	}

	// Save the index.
	if err := SessionCapture.Save(); err != nil {
		// Non-fatal: session files are already written.
		_ = err
	}

	// Link to task if applicable.
	if taskID != "" && BasePath != "" {
		linkSessionToTask(id, taskID)
	}

	// Log observability event.
	if EventLog != nil {
		_ = EventLog.Write(observability.Event{
			Time:    time.Now().UTC(),
			Level:   "INFO",
			Type:    "session.captured",
			Message: "session.captured",
			Data: map[string]any{
				"session_id":     id,
				"claude_session": sessionID,
				"task_id":        taskID,
				"project":        cwd,
				"turn_count":     len(result.Turns),
			},
		})
	}

	fmt.Printf("Session captured: %s (%d turns, %s)\n", id, len(result.Turns), formatDuration(duration))
	return nil
}

// captureDetectGitBranch attempts to get the current git branch from the given directory.
func captureDetectGitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// linkSessionToTask creates a symlink (or copy on Windows) from the task's
// sessions directory to the captured session's summary.md.
func linkSessionToTask(sessionID, taskID string) {
	if BasePath == "" {
		return
	}

	// Resolve ticket path (check both active and archived).
	ticketPath := ""
	candidates := []string{
		filepath.Join(BasePath, "tickets", taskID),
		filepath.Join(BasePath, "tickets", "_archived", taskID),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			ticketPath = p
			break
		}
	}
	if ticketPath == "" {
		return // Task ticket not found; skip linking.
	}

	sessionsDir := filepath.Join(ticketPath, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return
	}

	src := filepath.Join(BasePath, "sessions", sessionID, "summary.md")
	dst := filepath.Join(sessionsDir, sessionID+".md")

	if runtime.GOOS == "windows" {
		// Windows: copy instead of symlink.
		data, err := os.ReadFile(src) //nolint:gosec // G304: path from trusted internal code
		if err != nil {
			return
		}
		_ = os.WriteFile(dst, data, 0o644)
	} else {
		// Unix: create relative symlink.
		rel, err := filepath.Rel(sessionsDir, src)
		if err != nil {
			return
		}
		_ = os.Symlink(rel, dst)
	}
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func init() {
	sessionCaptureCmd.Flags().Bool("from-hook", false, "Read session data from stdin (used by SessionEnd hook)")
	sessionCaptureCmd.Flags().String("transcript", "", "Path to JSONL transcript file")
	sessionCaptureCmd.Flags().String("session-id", "", "Claude session UUID")
	sessionCaptureCmd.Flags().String("project-dir", "", "Project directory path")
	sessionCaptureCmd.Flags().String("task-id", "", "Task ID (also checks ADB_TASK_ID env var)")

	sessionListCapturedCmd.Flags().String("task", "", "Filter by task ID")
	sessionListCapturedCmd.Flags().String("since", "", "Show sessions since (e.g., 7d, 24h)")
	sessionListCapturedCmd.Flags().String("project", "", "Filter by project path")

	sessionCmd.AddCommand(sessionCaptureCmd)
	sessionCmd.AddCommand(sessionListCapturedCmd)
	sessionCmd.AddCommand(sessionShowCmd)
}

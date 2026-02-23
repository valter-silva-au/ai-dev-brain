package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

var teamCmd = &cobra.Command{
	Use:   "team <team-name> <prompt>",
	Short: "Launch a multi-agent team session with task context",
	Long: `Launch a Claude Code multi-agent team session with automatic task context
injection. This sets CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 and passes the
prompt to Claude Code for multi-agent orchestration.

The team-name is used for telemetry and session tracking. Common team names:
  design-review    Architecture and design validation
  security-audit   Security review and vulnerability scanning
  code-review      Multi-perspective code review
  spike            Research and investigation team

Task context (ADB_TASK_ID, ADB_BRANCH, etc.) is injected when available.
Team session telemetry is logged to .adb_events.jsonl.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		teamName := args[0]
		prompt := args[1]

		// Look for claude binary.
		claudePath, err := exec.LookPath("claude")
		if err != nil {
			return fmt.Errorf("claude binary not found in PATH: %w", err)
		}

		// Check version for agent teams support.
		if VersionChecker != nil {
			supported, featErr := VersionChecker.SupportsFeature("agent_teams")
			if featErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not check agent teams support: %v\n", featErr)
			} else if !supported {
				return fmt.Errorf("agent teams require Claude Code >= 2.1.32; upgrade with: npm install -g @anthropic-ai/claude-code")
			}
		}

		// Build environment with agent teams enabled.
		env := os.Environ()
		env = append(env,
			"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1",
		)

		// Inject task context if available.
		taskID := os.Getenv("ADB_TASK_ID")
		branch := os.Getenv("ADB_BRANCH")
		worktreePath := os.Getenv("ADB_WORKTREE_PATH")
		ticketPath := os.Getenv("ADB_TICKET_PATH")

		if taskID != "" {
			env = append(env, "ADB_TASK_ID="+taskID)
		}
		if branch != "" {
			env = append(env, "ADB_BRANCH="+branch)
		}
		if worktreePath != "" {
			env = append(env, "ADB_WORKTREE_PATH="+worktreePath)
		}
		if ticketPath != "" {
			env = append(env, "ADB_TICKET_PATH="+ticketPath)
		}

		// Log team session start event.
		startTime := time.Now().UTC()
		if EventLog != nil {
			_ = EventLog.Write(observability.Event{
				Time:    startTime,
				Level:   "INFO",
				Type:    "team.session_started",
				Message: "team.session_started",
				Data: map[string]any{
					"team_name": teamName,
					"task_id":   taskID,
					"prompt":    truncatePrompt(prompt, 200),
				},
			})
		}

		fmt.Printf("Launching team session: %s\n", teamName)
		if taskID != "" {
			fmt.Printf("  Task: %s\n", taskID)
		}

		// Launch Claude Code with the prompt.
		claudeArgs := []string{"--dangerously-skip-permissions", "-p", prompt}

		claudeCmd := exec.Command(claudePath, claudeArgs...)
		claudeCmd.Env = env
		claudeCmd.Stdin = os.Stdin
		claudeCmd.Stdout = os.Stdout
		claudeCmd.Stderr = os.Stderr

		if worktreePath != "" {
			claudeCmd.Dir = worktreePath
		}

		runErr := claudeCmd.Run()

		// Log team session end event.
		duration := time.Since(startTime)
		if EventLog != nil {
			exitCode := 0
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
				}
			}
			_ = EventLog.Write(observability.Event{
				Time:    time.Now().UTC(),
				Level:   "INFO",
				Type:    "team.session_ended",
				Message: "team.session_ended",
				Data: map[string]any{
					"team_name":    teamName,
					"task_id":      taskID,
					"duration_sec": int(duration.Seconds()),
					"exit_code":    exitCode,
				},
			})
		}

		if runErr != nil {
			fmt.Printf("Team session ended: %v\n", runErr)
		} else {
			fmt.Printf("Team session completed (%.0fs)\n", duration.Seconds())
		}

		return nil
	},
}

// truncatePrompt truncates a prompt string to maxLen characters with an ellipsis.
func truncatePrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	rootCmd.AddCommand(teamCmd)
}

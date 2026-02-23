package cli

import (
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"

	"github.com/spf13/cobra"
)

var hookSessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Handle SessionEnd hook events (non-blocking)",
	Long: `Capture session and update context when a Claude Code session ends.
Reads session metadata from stdin JSON.

Actions (all non-blocking):
- Capture session transcript (delegates to existing session capture)
- Update context.md with session activity summary
- Extract knowledge from transcript (Phase 2)
- Log notable communications (Phase 3)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse stdin for session metadata.
		input, err := hooks.ParseStdin[hooks.SessionEndInput](os.Stdin)
		if err != nil {
			return nil // Non-blocking, swallow errors.
		}

		// Step 1: Capture session using existing infrastructure.
		// This reuses the proven adb session capture logic.
		if input.TranscriptPath != "" && SessionCapture != nil {
			taskID := os.Getenv("ADB_TASK_ID")
			// Swallow errors: session capture is non-blocking.
			_ = captureFromTranscript(input.TranscriptPath, input.SessionID, input.CWD, taskID)
		}

		// Step 2: Run hook engine for context updates.
		if HookEngine != nil {
			_ = HookEngine.HandleSessionEnd(*input)
		}

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookSessionEndCmd)
}

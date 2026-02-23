package cli

import (
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"

	"github.com/spf13/cobra"
)

var hookPostToolUseCmd = &cobra.Command{
	Use:   "post-tool-use",
	Short: "Handle PostToolUse hook events (non-blocking)",
	Long: `React after a tool executes. Reads tool_name and tool_input from stdin JSON.

Actions (all non-blocking):
- Auto-format Go files with gofmt
- Track file modifications in .adb_session_changes
- Detect dependency changes in go.mod (Phase 2)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if HookEngine == nil {
			return nil
		}

		input, err := hooks.ParseStdin[hooks.PostToolUseInput](os.Stdin)
		if err != nil {
			return nil // Non-blocking, swallow errors.
		}

		// Non-blocking: swallow all errors.
		_ = HookEngine.HandlePostToolUse(*input)

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookPostToolUseCmd)
}

package cli

import (
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"

	"github.com/spf13/cobra"
)

var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Handle Stop hook events (non-blocking, advisory)",
	Long: `Run advisory checks when a Claude Code session stops.
Reads stop metadata from stdin JSON.

Advisory checks (all non-blocking):
- Warn about uncommitted changes
- Run go build and go vet
- Update context.md with session activity summary
- Update status.yaml timestamp
- Clean up .adb_session_changes tracker`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if HookEngine == nil {
			return nil
		}

		input, err := hooks.ParseStdin[hooks.StopInput](os.Stdin)
		if err != nil {
			return nil // Non-blocking, swallow errors.
		}

		// Non-blocking: swallow all errors.
		_ = HookEngine.HandleStop(*input)

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookStopCmd)
}

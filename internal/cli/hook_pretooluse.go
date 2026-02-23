package cli

import (
	"fmt"
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"

	"github.com/spf13/cobra"
)

var hookPreToolUseCmd = &cobra.Command{
	Use:   "pre-tool-use",
	Short: "Handle PreToolUse hook events (blocking)",
	Long: `Validate before a tool executes. Reads tool_name and tool_input from stdin JSON.

Blocks the tool execution (exit 2) if the edit targets:
- vendor/ files (use 'go mod vendor' instead)
- go.sum (use 'go mod tidy' instead)
- core/ importing storage/ or integration/ (architecture guard, Phase 2)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if HookEngine == nil {
			return nil
		}

		input, err := hooks.ParseStdin[hooks.PreToolUseInput](os.Stdin)
		if err != nil {
			return nil // Swallow parse errors, don't block.
		}

		if err := HookEngine.HandlePreToolUse(*input); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookPreToolUseCmd)
}

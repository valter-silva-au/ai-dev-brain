package cli

import (
	"fmt"
	"os"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"

	"github.com/spf13/cobra"
)

var hookTaskCompletedCmd = &cobra.Command{
	Use:   "task-completed",
	Short: "Handle TaskCompleted hook events (blocking)",
	Long: `Quality gate for task completion. Two-phase execution:

Phase A (blocking - exit 2 on failure):
- Check for uncommitted Go files
- Run test suite
- Run linter

Phase B (non-blocking - failures logged but don't block):
- Extract knowledge from task (Phase 2)
- Update wiki with learnings (Phase 2)
- Generate ADR drafts (Phase 2)
- Update context.md with completion summary`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if HookEngine == nil {
			return nil
		}

		input, err := hooks.ParseStdin[hooks.TaskCompletedInput](os.Stdin)
		if err != nil {
			return nil // Swallow parse errors, don't block.
		}

		if err := HookEngine.HandleTaskCompleted(*input); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}

		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookTaskCompletedCmd)
}

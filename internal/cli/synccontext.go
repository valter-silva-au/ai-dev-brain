package cli

import (
	"fmt"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/spf13/cobra"
)

// AICtxGen is the AIContextGenerator used by the sync-context command.
// Set during application wiring (Task #43).
var AICtxGen core.AIContextGenerator

var syncContextCmd = &cobra.Command{
	Use:   "sync-context",
	Short: "Regenerate AI context files (CLAUDE.md, kiro.md)",
	Long: `Regenerate the root-level AI context files by assembling current state
from wiki content, ADRs, active tasks, glossary, and contacts.

This ensures AI assistants have up-to-date project context.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if AICtxGen == nil {
			return fmt.Errorf("AI context generator not initialized")
		}

		if err := AICtxGen.SyncContext(); err != nil {
			return fmt.Errorf("syncing AI context: %w", err)
		}

		fmt.Println("AI context files regenerated successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncContextCmd)
}

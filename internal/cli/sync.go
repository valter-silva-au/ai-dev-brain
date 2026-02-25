package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize context, repos, and Claude Code configuration",
	Long: `Unified sync commands for keeping project context, repositories,
and AI assistant configuration up to date.`,
}

var syncContextSubCmd = &cobra.Command{
	Use:   "context",
	Short: "Regenerate AI context files (CLAUDE.md, kiro.md)",
	Long: `Regenerate the root-level AI context files by assembling current state
from wiki content, ADRs, active tasks, glossary, and contacts.

This ensures AI assistants have up-to-date project context.`,
	Args: cobra.NoArgs,
	RunE: syncContextCmd.RunE,
}

var syncTaskContextSubCmd = &cobra.Command{
	Use:   "task-context",
	Short: "Regenerate task context in the current worktree",
	Long: `Regenerate .claude/rules/task-context.md in the current worktree.

Use --hook-mode when calling from a ConfigChange hook (silent, non-fatal).`,
	Args: cobra.NoArgs,
	RunE: syncTaskContextCmd.RunE,
}

var syncReposSubCmd = &cobra.Command{
	Use:   "repos",
	Short: "Fetch, prune, and clean all tracked repositories",
	Long: `Synchronise all git repositories under the repos/ directory.

For each discovered repository this command:
  - Fetches all remotes and prunes stale remote-tracking branches
  - Fast-forwards the default branch if behind origin
  - Deletes local branches that have been merged into the default branch

Branches associated with active backlog tasks are never deleted.`,
	Args: cobra.NoArgs,
	RunE: syncReposCmd.RunE,
}

var syncClaudeUserSubCmd = &cobra.Command{
	Use:   "claude-user",
	Short: "Sync Claude Code agents, skills, and status line to user config",
	Long: `Sync Claude Code configuration from adb's embedded templates to the
user-level ~/.claude/ directory.

By default, syncs all agents, workflow skills, quality gate checklists,
artifact templates, and the adb MCP server.

Use --mcp to also merge third-party MCP server definitions into ~/.claude.json.`,
	Args: cobra.NoArgs,
	RunE: syncClaudeUserCmd.RunE,
}

var syncAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all sync operations (context, repos, claude-user)",
	Long: `Run context, repos, and claude-user sync operations sequentially.

All operations are attempted even if some fail. Errors are collected
and reported at the end.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var errs []error

		fmt.Println("Syncing AI context files...")
		if err := syncContextCmd.RunE(syncContextCmd, nil); err != nil {
			fmt.Printf("  Warning: context sync failed: %v\n", err)
			errs = append(errs, fmt.Errorf("context: %w", err))
		}

		fmt.Println("Syncing repositories...")
		if err := syncReposCmd.RunE(syncReposCmd, nil); err != nil {
			fmt.Printf("  Warning: repos sync failed: %v\n", err)
			errs = append(errs, fmt.Errorf("repos: %w", err))
		}

		fmt.Println("Syncing Claude Code user config...")
		if err := syncClaudeUserCmd.RunE(syncClaudeUserCmd, nil); err != nil {
			fmt.Printf("  Warning: claude-user sync failed: %v\n", err)
			errs = append(errs, fmt.Errorf("claude-user: %w", err))
		}

		if len(errs) > 0 {
			fmt.Printf("\n%d sync operation(s) failed:\n", len(errs))
			for _, e := range errs {
				fmt.Printf("  - %v\n", e)
			}
			return fmt.Errorf("%d sync operation(s) failed", len(errs))
		}

		fmt.Println("\nAll sync operations completed successfully.")
		return nil
	},
}

func init() {
	// sync task-context flags (mirror the original command)
	syncTaskContextSubCmd.Flags().Bool("hook-mode", false, "Run in hook mode (silent, non-fatal)")

	// sync claude-user flags (mirror the original command)
	syncClaudeUserSubCmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
	syncClaudeUserSubCmd.Flags().Bool("mcp", false, "Also sync third-party MCP servers to ~/.claude.json")

	syncCmd.AddCommand(syncContextSubCmd)
	syncCmd.AddCommand(syncTaskContextSubCmd)
	syncCmd.AddCommand(syncReposSubCmd)
	syncCmd.AddCommand(syncClaudeUserSubCmd)
	syncCmd.AddCommand(syncAllCmd)

	rootCmd.AddCommand(syncCmd)
}

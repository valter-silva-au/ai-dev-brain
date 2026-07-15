package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// wikiClassifier maps a task to its org + initiative for wiki namespacing, from
// the task's Initiative field (backlog) and that initiative's OrgID (registry).
// Either may be empty → the page lands ungrouped at the wiki root.
type wikiClassifier struct{}

func (wikiClassifier) Classify(taskID string) (string, string) {
	if App == nil || App.BacklogManager == nil {
		return "", ""
	}
	t, err := App.BacklogManager.GetTask(taskID)
	if err != nil || t == nil || t.Initiative == "" {
		return "", ""
	}
	org := ""
	if App.StageManager != nil {
		if in, err := App.StageManager.GetInitiative(t.Initiative); err == nil {
			org = in.OrgID
		}
	}
	return org, t.Initiative
}

// NewSyncCmd creates the sync command with all subcommands
func NewSyncCmd() *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync and regenerate context files",
		Long:  `Commands for regenerating context files for AI agents`,
	}

	// Add subcommands
	syncCmd.AddCommand(
		newSyncContextCmd(),
		newSyncTaskContextCmd(),
		newSyncReposCmd(),
		newSyncClaudeUserCmd(),
		newSyncWikiCmd(),
		newSyncIssuesCmd(), // WS-E: GitHub/GitLab issue sync (defined in sync_issues.go)
		newSyncAllCmd(),
		newSyncCloudCmd(), // WS-G: S3 archive plane. Sibling: WS-E adds `sync issues`.
	)

	return syncCmd
}

// newSyncWikiCmd creates the 'sync wiki' command — publishes each task's
// extracted knowledge (decisions, learnings, gotchas) as wiki markdown so
// the knowledge accumulated in tickets/<id>/knowledge/decisions.yaml flows
// OUT to a human-readable, cross-linkable knowledge base.
func newSyncWikiCmd() *cobra.Command {
	var outDir string

	cmd := &cobra.Command{
		Use:   "wiki [--out <dir>]",
		Short: "Publish extracted task knowledge as wiki markdown",
		Long: `Render one markdown page per task that has extracted knowledge.

Pages carry YAML frontmatter (title/created/updated/tags/source) and group
decisions, learnings, and gotchas under headings, so they drop straight into
a file-based wiki. With --out you can target an external wiki directory
outside the adb workspace (the in-tool knowledge stays authoritative).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if outDir == "" {
				outDir = App.BasePath + "/docs/wiki/knowledge"
			}

			publisher := core.NewWikiPublisher(App.BasePath)
			// Graph cross-links + org/initiative namespacing (issue #127).
			publisher.SetGraph(App.GraphManager)
			publisher.SetClassifier(wikiClassifier{})
			// Index the published corpus for semantic search when memory is
			// configured for the workspace (opt-in; same store search_knowledge reads).
			if store, configured, err := App.OpenMemoryStore(context.Background()); err == nil && configured {
				defer store.Close()
				publisher.SetIndexer(store)
			}

			fmt.Printf("Publishing task knowledge to %s...\n", outDir)
			res, err := publisher.PublishAll(outDir)
			if err != nil {
				return fmt.Errorf("failed to publish wiki: %w", err)
			}

			fmt.Printf("✓ %d page(s) + %d nav artifact(s) written, %d skipped (no knowledge), %d scanned",
				len(res.PagesWritten), len(res.IndexPages), len(res.Skipped), res.TasksScanned)
			if res.Indexed > 0 {
				fmt.Printf(", %d indexed for search", res.Indexed)
			}
			fmt.Println()
			for _, page := range res.PagesWritten {
				fmt.Printf("  + %s\n", page)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", "", "Output directory (default: <workspace>/docs/wiki/knowledge)")

	return cmd
}

// newSyncContextCmd creates the 'sync context' command
func newSyncContextCmd() *cobra.Command {
	var rich bool

	cmd := &cobra.Command{
		Use:   "context [--rich]",
		Short: "Regenerate CLAUDE.md",
		Long: `Regenerate the main CLAUDE.md context file from backlog and task data.

With --rich, use the multi-section AIContextGenerator: an 11-section CLAUDE.md
(overview, directory, conventions, glossary, decisions, active tasks, critical
decisions, recent/captured sessions, stakeholders, "What's Changed") that
maintains .context_state.yaml section-hashing for change detection. Without the
flag, the trivial generator that dumps backlog.yaml is used (the current
default, kept for now to de-risk the switch).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			if rich {
				if App.AIContextGenerator == nil {
					return fmt.Errorf("AI context generator not initialized")
				}
				fmt.Println("Regenerating CLAUDE.md (rich, multi-section)...")
				if err := App.AIContextGenerator.Generate(); err != nil {
					return fmt.Errorf("failed to regenerate rich context: %w", err)
				}
				fmt.Println("✓ CLAUDE.md regenerated (rich) + .context_state.yaml updated")
				return nil
			}

			// Create context generator
			contextGen := core.NewContextGenerator(
				App.BasePath+"/backlog.yaml",
				App.BasePath+"/tickets",
				App.BasePath,
				App.TemplateManager,
			)

			fmt.Println("Regenerating CLAUDE.md...")
			if err := contextGen.GenerateContext(); err != nil {
				return fmt.Errorf("failed to regenerate context: %w", err)
			}

			fmt.Println("✓ CLAUDE.md regenerated")
			return nil
		},
	}

	cmd.Flags().BoolVar(&rich, "rich", false, "Use the multi-section AIContextGenerator (maintains .context_state.yaml)")

	return cmd
}

// newSyncTaskContextCmd creates the 'sync task-context' command
func newSyncTaskContextCmd() *cobra.Command {
	var hookMode bool

	cmd := &cobra.Command{
		Use:   "task-context <task-id>",
		Short: "Regenerate task-specific context",
		Long:  `Regenerate context.md for a specific task`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			taskID := args[0]

			// Create context generator
			contextGen := core.NewContextGenerator(
				App.BasePath+"/backlog.yaml",
				App.BasePath+"/tickets",
				App.BasePath,
				App.TemplateManager,
			)

			fmt.Printf("Regenerating context for %s...\n", taskID)
			if err := contextGen.GenerateTaskContext(taskID, hookMode); err != nil {
				return fmt.Errorf("failed to regenerate task context: %w", err)
			}

			fmt.Printf("✓ Task context regenerated for %s\n", taskID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&hookMode, "hook-mode", false, "Run in hook mode (append timestamp only)")

	return cmd
}

// newSyncReposCmd creates the 'sync repos' command
func newSyncReposCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "Regenerate repository context",
		Long:  `Regenerate repository structure context`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Create context generator
			contextGen := core.NewContextGenerator(
				App.BasePath+"/backlog.yaml",
				App.BasePath+"/tickets",
				App.BasePath,
				App.TemplateManager,
			)

			fmt.Println("Regenerating repository context...")
			if err := contextGen.GenerateRepoContext(); err != nil {
				return fmt.Errorf("failed to regenerate repo context: %w", err)
			}

			fmt.Println("✓ Repository context regenerated")
			return nil
		},
	}

	return cmd
}

// newSyncClaudeUserCmd creates the 'sync claude-user' command
func newSyncClaudeUserCmd() *cobra.Command {
	var (
		dryRun bool
		mcp    bool
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "claude-user",
		Short: "Regenerate Claude user context and install the Skills+Agents harness",
		Long: `Regenerate Claude-specific user context files, then install the embedded
Skills + Agents harness (the devils-advocate agent and the founder-playbook
skills) into your Claude Code config directory so Claude Code can load them.

Harness files install under <config>/agents and <config>/skills, where <config>
is $CLAUDE_CONFIG_DIR (if set) or ~/.claude. The install is idempotent: an
up-to-date file is left unchanged and a file you have edited is skipped rather
than clobbered (pass --force to overwrite). --dry-run previews without writing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Create context generator
			contextGen := core.NewContextGenerator(
				App.BasePath+"/backlog.yaml",
				App.BasePath+"/tickets",
				App.BasePath,
				App.TemplateManager,
			)

			fmt.Println("Regenerating Claude user context...")
			if err := contextGen.GenerateClaudeUserContext(dryRun, mcp); err != nil {
				return fmt.Errorf("failed to regenerate Claude user context: %w", err)
			}
			if !dryRun {
				fmt.Println("✓ Claude user context regenerated")
			}

			return installClaudeHarness(cmd, dryRun, force)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing")
	cmd.Flags().BoolVar(&mcp, "mcp", false, "Include MCP integration context")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite harness files that were edited (differ from the embedded copy)")

	return cmd
}

// installClaudeHarness installs the embedded Skills+Agents harness into the
// resolved Claude config dir and prints a per-file summary. The logic lives in
// core.InstallHarness; this handler only resolves the destination and renders.
func installClaudeHarness(cmd *cobra.Command, dryRun, force bool) error {
	claudeDir, err := resolveClaudeConfigDir()
	if err != nil {
		return err
	}

	res, err := core.InstallHarness(claude.FS, claudeDir, core.HarnessInstallOptions{DryRun: dryRun, Force: force})
	if err != nil {
		return fmt.Errorf("failed to install Claude harness: %w", err)
	}

	out := cmd.OutOrStdout()
	verb := "Installing"
	if dryRun {
		verb = "Would install"
	}
	fmt.Fprintf(out, "%s Claude harness into %s...\n", verb, claudeDir)
	for _, e := range res.Entries {
		note := ""
		if e.Action == core.HarnessSkipped {
			note = " (edited locally; pass --force to overwrite)"
		}
		// Show the path relative to the config dir for readability.
		rel, relErr := filepath.Rel(claudeDir, e.Dest)
		if relErr != nil {
			rel = e.Dest
		}
		fmt.Fprintf(out, "  %-9s %s%s\n", e.Action, filepath.ToSlash(rel), note)
	}
	fmt.Fprintf(out, "✓ Harness: %d installed, %d unchanged, %d skipped\n",
		res.Count(core.HarnessInstalled), res.Count(core.HarnessUnchanged), res.Count(core.HarnessSkipped))
	return nil
}

// resolveClaudeConfigDir returns the Claude Code config directory: $CLAUDE_CONFIG_DIR
// if set, else ~/.claude. This is the impure edge (env + home lookup); tests point
// CLAUDE_CONFIG_DIR at a temp dir to avoid touching the real config.
func resolveClaudeConfigDir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory for the Claude config dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// newSyncAllCmd creates the 'sync all' command
func newSyncAllCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "all",
		Short: "Regenerate all context files",
		Long:  `Regenerate all context files (CLAUDE.md, repo context, user context)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			// Create context generator
			contextGen := core.NewContextGenerator(
				App.BasePath+"/backlog.yaml",
				App.BasePath+"/tickets",
				App.BasePath,
				App.TemplateManager,
			)

			fmt.Println("Regenerating all context files...")
			if err := contextGen.GenerateAll(); err != nil {
				return fmt.Errorf("failed to regenerate context files: %w", err)
			}

			fmt.Println("✓ All context files regenerated")
			return nil
		},
	}

	return cmd
}

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var syncClaudeUserCmd = &cobra.Command{
	Use:   "sync-claude-user",
	Short: "Sync universal Claude Code skills, agents, MCP servers, and status line to user config",
	Long: `Sync universal (language-agnostic) Claude Code configuration from adb's
canonical templates to the user-level ~/.claude/ directory.

By default, syncs skills, agents, and the status line script. Use --mcp
to also merge MCP server definitions into ~/.claude.json so they are
available in every project.

This ensures git workflow skills (commit, pr, push, review, sync, changelog),
the generic code-reviewer agent, the universal status line, and shared MCP
servers are available on any machine after a single command.

Skills, agents, and the status line are overwritten if they already exist
(templates are the source of truth). MCP servers are merged -- existing
servers are updated, new servers are added, and servers not in the template
are left untouched.

The status line script (~/.claude/statusline.sh) provides tiered context:
  - Tier 1 (always): project name, model, context%, cost, lines, duration
  - Tier 2 (git):    branch, dirty count, ahead/behind
  - Tier 3 (adb):    task ID/type/priority, portfolio counts, alerts

The command also configures ~/.claude/settings.json to use the status line.

Run this after installing adb on a new machine, or after upgrading adb
to pick up template changes.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		syncMCP, _ := cmd.Flags().GetBool("mcp")

		// Resolve template directory
		templateDir := filepath.Join(BasePath, "repos", "github.com", "valter-silva-au", "ai-dev-brain", "templates", "claude")
		if _, err := os.Stat(templateDir); os.IsNotExist(err) {
			execPath, execErr := os.Executable()
			if execErr == nil {
				templateDir = filepath.Join(filepath.Dir(execPath), "templates", "claude")
			}
			if _, err := os.Stat(templateDir); os.IsNotExist(err) {
				return fmt.Errorf("template directory not found: run from an adb workspace or set ADB_HOME")
			}
		}

		// Resolve user claude directory
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving home directory: %w", err)
		}
		userClaudeDir := filepath.Join(home, ".claude")

		var synced int

		// Sync skills
		skills := []string{"commit", "pr", "push", "review", "sync", "changelog"}
		for _, skill := range skills {
			src := filepath.Join(templateDir, "skills", skill, "SKILL.md")
			dst := filepath.Join(userClaudeDir, "skills", skill, "SKILL.md")

			if dryRun {
				fmt.Printf("  [dry-run] skill: %s\n", skill)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating skill directory for %s: %w", skill, err)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping skill %s: %v\n", skill, err)
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("writing skill %s: %w", skill, err)
			}
			fmt.Printf("  synced skill: %s\n", skill)
			synced++
		}

		// Sync agents
		agents := []string{"code-reviewer.md"}
		for _, agent := range agents {
			src := filepath.Join(templateDir, "agents", agent)
			dst := filepath.Join(userClaudeDir, "agents", agent)

			if dryRun {
				fmt.Printf("  [dry-run] agent: %s\n", agent)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating agents directory: %w", err)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping agent %s: %v\n", agent, err)
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("writing agent %s: %w", agent, err)
			}
			fmt.Printf("  synced agent: %s\n", agent)
			synced++
		}

		// Sync statusline.sh
		{
			src := filepath.Join(templateDir, "statusline.sh")
			dst := filepath.Join(userClaudeDir, "statusline.sh")

			if dryRun {
				fmt.Printf("  [dry-run] statusline: statusline.sh\n")
				synced++
			} else {
				data, err := os.ReadFile(src)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  warning: skipping statusline: %v\n", err)
				} else {
					if err := os.WriteFile(dst, data, 0o755); err != nil {
						return fmt.Errorf("writing statusline.sh: %w", err)
					}
					fmt.Printf("  synced statusline: statusline.sh\n")
					synced++
				}
			}
		}

		// Merge statusLine config into ~/.claude/settings.json
		if err := mergeStatusLineConfig(userClaudeDir, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not update settings.json: %v\n", err)
		}

		// Sync MCP servers into ~/.claude.json
		if syncMCP {
			mcpCount, err := syncMCPServers(templateDir, home, dryRun)
			if err != nil {
				return fmt.Errorf("syncing MCP servers: %w", err)
			}
			synced += mcpCount
		}

		if dryRun {
			fmt.Printf("\n[dry-run] Would sync %d items to %s\n", synced, userClaudeDir)
		} else {
			fmt.Printf("\nSynced %d items to %s\n", synced, userClaudeDir)
		}

		// Check required environment variables
		checkEnvVars()

		return nil
	},
}

// syncMCPServers merges MCP server definitions from the template into ~/.claude.json.
// Existing servers are updated, new servers are added, unrelated keys are preserved.
func syncMCPServers(templateDir, home string, dryRun bool) (int, error) {
	// Read template MCP servers
	templatePath := filepath.Join(templateDir, "mcp-servers.json")
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return 0, fmt.Errorf("reading MCP template: %w", err)
	}

	var templateServers map[string]interface{}
	if err := json.Unmarshal(templateData, &templateServers); err != nil {
		return 0, fmt.Errorf("parsing MCP template: %w", err)
	}

	if dryRun {
		for name := range templateServers {
			fmt.Printf("  [dry-run] mcp server: %s\n", name)
		}
		return len(templateServers), nil
	}

	// Read existing ~/.claude.json
	claudeJSONPath := filepath.Join(home, ".claude.json")
	var claudeConfig map[string]interface{}

	existing, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing config -- create a minimal one
			claudeConfig = map[string]interface{}{
				"mcpServers": templateServers,
			}
		} else {
			return 0, fmt.Errorf("reading ~/.claude.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(existing, &claudeConfig); err != nil {
			return 0, fmt.Errorf("parsing ~/.claude.json: %w", err)
		}

		// Merge MCP servers
		existingServers, ok := claudeConfig["mcpServers"].(map[string]interface{})
		if !ok {
			existingServers = make(map[string]interface{})
		}
		for name, config := range templateServers {
			existingServers[name] = config
		}
		claudeConfig["mcpServers"] = existingServers
	}

	// Write back
	output, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling ~/.claude.json: %w", err)
	}
	if err := os.WriteFile(claudeJSONPath, output, 0o644); err != nil {
		return 0, fmt.Errorf("writing ~/.claude.json: %w", err)
	}

	for name := range templateServers {
		fmt.Printf("  synced mcp server: %s\n", name)
	}
	return len(templateServers), nil
}

// checkEnvVars prints warnings for required environment variables that are not set.
func checkEnvVars() {
	requiredVars := map[string]string{
		"CONTEXT7_API_KEY": "Context7 library docs (get one at https://context7.com)",
	}
	var missing []string
	for env, desc := range requiredVars {
		if os.Getenv(env) == "" {
			missing = append(missing, fmt.Sprintf("  %s -- %s", env, desc))
		}
	}
	if len(missing) > 0 {
		fmt.Println("\nEnvironment variables not set (add to your shell profile):")
		for _, m := range missing {
			fmt.Println(m)
		}
	}
}

// mergeStatusLineConfig ensures ~/.claude/settings.json has a statusLine block
// pointing to ~/.claude/statusline.sh. Preserves all existing keys.
func mergeStatusLineConfig(claudeDir string, dryRun bool) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	if dryRun {
		fmt.Printf("  [dry-run] settings.json: add statusLine config\n")
		return nil
	}

	var settings map[string]interface{}

	existing, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return fmt.Errorf("reading settings.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	settings["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": "~/.claude/statusline.sh",
	}

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings.json: %w", err)
	}
	output = append(output, '\n')

	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("creating claude directory: %w", err)
	}
	if err := os.WriteFile(settingsPath, output, 0o644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	fmt.Printf("  updated settings.json: statusLine -> ~/.claude/statusline.sh\n")
	return nil
}

func init() {
	syncClaudeUserCmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
	syncClaudeUserCmd.Flags().Bool("mcp", false, "Also sync MCP server definitions to ~/.claude.json")
	rootCmd.AddCommand(syncClaudeUserCmd)
}

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
	Short: "Sync universal Claude Code skills, agents, and MCP servers to user config",
	Long: `Sync universal (language-agnostic) Claude Code configuration from adb's
canonical templates to the user-level ~/.claude/ directory.

By default, syncs skills and agents. Use --mcp to also merge MCP server
definitions into ~/.claude.json so they are available in every project.

This ensures git workflow skills (commit, pr, push, review, sync, changelog),
the generic code-reviewer agent, and shared MCP servers are available on any
machine after a single command.

Skills and agents are overwritten if they already exist (templates are the
source of truth). MCP servers are merged -- existing servers are updated,
new servers are added, and servers not in the template are left untouched.

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

		// Sync session capture hook
		hookCount, hookErr := syncSessionHook(templateDir, userClaudeDir, dryRun)
		if hookErr != nil {
			fmt.Fprintf(os.Stderr, "  warning: session hook sync failed: %v\n", hookErr)
		} else {
			synced += hookCount
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

// syncSessionHook installs the session capture hook script and merges
// a SessionEnd entry into ~/.claude/settings.json.
func syncSessionHook(templateDir, userClaudeDir string, dryRun bool) (int, error) {
	synced := 0

	// 1. Copy hook script to ~/.claude/hooks/
	hookSrc := filepath.Join(templateDir, "hooks", "adb-session-capture.sh")
	hookDst := filepath.Join(userClaudeDir, "hooks", "adb-session-capture.sh")

	if dryRun {
		fmt.Printf("  [dry-run] hook: adb-session-capture.sh\n")
		fmt.Printf("  [dry-run] settings.json: SessionEnd hook entry\n")
		return 2, nil
	}

	if err := os.MkdirAll(filepath.Dir(hookDst), 0o755); err != nil {
		return 0, fmt.Errorf("creating hooks directory: %w", err)
	}

	hookData, err := os.ReadFile(hookSrc)
	if err != nil {
		return 0, fmt.Errorf("reading hook template: %w", err)
	}
	if err := os.WriteFile(hookDst, hookData, 0o755); err != nil {
		return 0, fmt.Errorf("writing hook script: %w", err)
	}
	fmt.Printf("  synced hook: adb-session-capture.sh\n")
	synced++

	// 2. Merge SessionEnd hook entry into ~/.claude/settings.json
	settingsPath := filepath.Join(userClaudeDir, "settings.json")
	var settings map[string]interface{}

	existing, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return synced, fmt.Errorf("reading settings.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return synced, fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	// Build the SessionEnd hook entry.
	hookCommand := "~/.claude/hooks/adb-session-capture.sh"
	hookEntry := map[string]interface{}{
		"type":    "command",
		"command": hookCommand,
	}

	// Get or create the hooks section.
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	// Get or create the SessionEnd array.
	sessionEnd, ok := hooks["SessionEnd"].([]interface{})
	if !ok {
		sessionEnd = nil
	}

	// Check if our hook entry already exists.
	alreadyPresent := false
	for _, entry := range sessionEnd {
		entryArr, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hooksList, ok := entryArr["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, ok := hMap["command"].(string); ok && cmd == hookCommand {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			break
		}
	}

	if !alreadyPresent {
		newEntry := map[string]interface{}{
			"hooks": []interface{}{hookEntry},
		}
		sessionEnd = append(sessionEnd, newEntry)
		hooks["SessionEnd"] = sessionEnd
		settings["hooks"] = hooks

		output, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return synced, fmt.Errorf("marshaling settings.json: %w", err)
		}
		if err := os.WriteFile(settingsPath, output, 0o644); err != nil {
			return synced, fmt.Errorf("writing settings.json: %w", err)
		}
		fmt.Printf("  synced settings.json: SessionEnd hook entry\n")
	} else {
		fmt.Printf("  settings.json: SessionEnd hook already present\n")
	}
	synced++

	return synced, nil
}

func init() {
	syncClaudeUserCmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
	syncClaudeUserCmd.Flags().Bool("mcp", false, "Also sync MCP server definitions to ~/.claude.json")
	rootCmd.AddCommand(syncClaudeUserCmd)
}

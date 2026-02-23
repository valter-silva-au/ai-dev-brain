package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"

	claudetpl "github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

var syncClaudeUserCmd = &cobra.Command{
	Use:   "sync-claude-user",
	Short: "Sync Claude Code agents, skills, checklists, templates, and status line to user config",
	Long: `Sync Claude Code configuration from adb's embedded templates to the
user-level ~/.claude/ directory.

By default, syncs all BMAD-method agents (analyst, product-owner,
design-reviewer, scrum-master, quick-flow-dev, team-lead, and more),
workflow skills (create-brief, create-prd, create-architecture,
create-stories, quick-spec, quick-dev, adversarial-review, and more),
quality gate checklists, artifact templates, and the adb MCP server.

Use --mcp to also merge third-party MCP server definitions (aws-docs,
aws-knowledge, context7) into ~/.claude.json. These are opt-in because
they require external dependencies (uvx, API keys, network access).

Skills, agents, checklists, artifact templates, and the status line are
overwritten if they already exist (embedded templates are the source of
truth). MCP servers are merged -- existing servers are updated, new
servers are added, and servers not in the template are left untouched.

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

		// Resolve user claude directory
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving home directory: %w", err)
		}
		userClaudeDir := filepath.Join(home, ".claude")

		var synced int

		// Sync skills (git workflow + BMAD workflow + project tools)
		skills := []string{
			// Git workflow skills
			"commit", "pr", "push", "review", "sync", "changelog",
			// BMAD workflow skills
			"create-brief", "create-prd", "create-architecture", "create-stories",
			"check-readiness", "correct-course", "run-checklist",
			"quick-spec", "quick-dev", "adversarial-review",
			// Project tool skills
			"build", "test", "lint", "security", "docker", "release",
			"add-command", "add-interface",
			"coverage-report", "status-check", "health-dashboard",
			"context-refresh", "dependency-check", "knowledge-extract",
			"onboard", "standup", "retrospective",
			"browser-automate", "playwright", "ui-review",
		}
		for _, skill := range skills {
			embeddedPath := path.Join("skills", skill, "SKILL.md")
			dst := filepath.Join(userClaudeDir, "skills", skill, "SKILL.md")

			if dryRun {
				fmt.Printf("  [dry-run] skill: %s\n", skill)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating skill directory for %s: %w", skill, err)
			}
			data, err := claudetpl.FS.ReadFile(embeddedPath)
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

		// Sync agents (BMAD personas + supporting agents)
		agents := []string{
			// BMAD workflow agents
			"analyst.md", "product-owner.md", "design-reviewer.md",
			"scrum-master.md", "quick-flow-dev.md", "team-lead.md",
			// Supporting agents
			"code-reviewer.md", "architecture-guide.md", "researcher.md",
			"debugger.md", "doc-writer.md", "knowledge-curator.md",
			"go-tester.md", "security-auditor.md", "release-manager.md",
			"observability-reporter.md", "browser-qa.md", "playwright-browser.md",
		}
		for _, agent := range agents {
			embeddedPath := path.Join("agents", agent)
			dst := filepath.Join(userClaudeDir, "agents", agent)

			if dryRun {
				fmt.Printf("  [dry-run] agent: %s\n", agent)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating agents directory: %w", err)
			}
			data, err := claudetpl.FS.ReadFile(embeddedPath)
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

		// Sync quality gate checklists
		checklists := []string{
			"prd-checklist.md", "architecture-checklist.md",
			"story-checklist.md", "readiness-checklist.md",
			"code-review-checklist.md",
		}
		for _, checklist := range checklists {
			embeddedPath := path.Join("checklists", checklist)
			dst := filepath.Join(userClaudeDir, "checklists", checklist)

			if dryRun {
				fmt.Printf("  [dry-run] checklist: %s\n", checklist)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating checklists directory: %w", err)
			}
			data, err := claudetpl.FS.ReadFile(embeddedPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping checklist %s: %v\n", checklist, err)
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("writing checklist %s: %w", checklist, err)
			}
			fmt.Printf("  synced checklist: %s\n", checklist)
			synced++
		}

		// Sync artifact templates
		artifacts := []string{
			"product-brief.md", "prd.md", "architecture-doc.md",
			"epics.md", "tech-spec.md",
		}
		for _, artifact := range artifacts {
			embeddedPath := path.Join("artifacts", artifact)
			dst := filepath.Join(userClaudeDir, "artifacts", artifact)

			if dryRun {
				fmt.Printf("  [dry-run] artifact template: %s\n", artifact)
				synced++
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("creating artifacts directory: %w", err)
			}
			data, err := claudetpl.FS.ReadFile(embeddedPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping artifact %s: %v\n", artifact, err)
				continue
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("writing artifact %s: %w", artifact, err)
			}
			fmt.Printf("  synced artifact template: %s\n", artifact)
			synced++
		}

		// Sync statusline script (~/.claude/statusline.sh)
		statuslineDst := filepath.Join(userClaudeDir, "statusline.sh")
		if dryRun {
			fmt.Printf("  [dry-run] statusline: statusline.sh\n")
			synced++
		} else {
			statuslineData, err := claudetpl.FS.ReadFile("statusline.sh")
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping statusline.sh: %v\n", err)
			} else {
				if err := os.WriteFile(statuslineDst, statuslineData, 0o755); err != nil {
					return fmt.Errorf("writing statusline.sh: %w", err)
				}
				fmt.Printf("  synced statusline: statusline.sh\n")
				synced++
			}
		}

		// Configure statusLine in ~/.claude/settings.json
		if err := ensureStatusLineConfig(userClaudeDir, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: statusline settings sync failed: %v\n", err)
		}

		// Sync session capture hook
		hookCount, hookErr := syncSessionHook(userClaudeDir, dryRun)
		if hookErr != nil {
			fmt.Fprintf(os.Stderr, "  warning: session hook sync failed: %v\n", hookErr)
		} else {
			synced += hookCount
		}

		// Always sync the adb MCP server into ~/.claude.json (zero external deps)
		adbCount, err := syncAdbMCPServer(home, dryRun)
		if err != nil {
			return fmt.Errorf("syncing adb MCP server: %w", err)
		}
		synced += adbCount

		// Sync third-party MCP servers into ~/.claude.json (opt-in via --mcp)
		if syncMCP {
			mcpCount, err := syncThirdPartyMCPServers(home, dryRun)
			if err != nil {
				return fmt.Errorf("syncing third-party MCP servers: %w", err)
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

// adbMCPServerName is the key used for the adb MCP server in ~/.claude.json.
const adbMCPServerName = "adb"

// adbMCPServerConfig returns the MCP server definition for adb's own server.
func adbMCPServerConfig() map[string]interface{} {
	return map[string]interface{}{
		"type":    "stdio",
		"command": "adb",
		"args":    []interface{}{"mcp", "serve"},
	}
}

// syncAdbMCPServer registers the adb MCP server in ~/.claude.json.
// This is always called (not gated behind --mcp) because the adb server
// has zero external dependencies -- it only needs the adb binary on PATH.
func syncAdbMCPServer(home string, dryRun bool) (int, error) {
	servers := map[string]interface{}{
		adbMCPServerName: adbMCPServerConfig(),
	}
	return mergeMCPServers(servers, home, dryRun)
}

// syncThirdPartyMCPServers merges third-party MCP server definitions from the
// embedded template into ~/.claude.json, excluding the adb server (already synced).
func syncThirdPartyMCPServers(home string, dryRun bool) (int, error) {
	// Read template MCP servers from embedded FS
	templateData, err := claudetpl.FS.ReadFile("mcp-servers.json")
	if err != nil {
		return 0, fmt.Errorf("reading embedded MCP template: %w", err)
	}

	var templateServers map[string]interface{}
	if err := json.Unmarshal(templateData, &templateServers); err != nil {
		return 0, fmt.Errorf("parsing MCP template: %w", err)
	}

	// Exclude the adb server -- it is already synced unconditionally
	delete(templateServers, adbMCPServerName)

	if len(templateServers) == 0 {
		return 0, nil
	}

	return mergeMCPServers(templateServers, home, dryRun)
}

// mergeMCPServers merges the given server definitions into ~/.claude.json.
// Existing servers are updated, new servers are added, unrelated keys are preserved.
func mergeMCPServers(servers map[string]interface{}, home string, dryRun bool) (int, error) {
	if dryRun {
		for name := range servers {
			fmt.Printf("  [dry-run] mcp server: %s\n", name)
		}
		return len(servers), nil
	}

	claudeJSONPath := filepath.Join(home, ".claude.json")
	var claudeConfig map[string]interface{}

	existing, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			claudeConfig = map[string]interface{}{
				"mcpServers": servers,
			}
		} else {
			return 0, fmt.Errorf("reading ~/.claude.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(existing, &claudeConfig); err != nil {
			return 0, fmt.Errorf("parsing ~/.claude.json: %w", err)
		}

		existingServers, ok := claudeConfig["mcpServers"].(map[string]interface{})
		if !ok {
			existingServers = make(map[string]interface{})
		}
		for name, config := range servers {
			existingServers[name] = config
		}
		claudeConfig["mcpServers"] = existingServers
	}

	output, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling ~/.claude.json: %w", err)
	}
	if err := os.WriteFile(claudeJSONPath, output, 0o644); err != nil {
		return 0, fmt.Errorf("writing ~/.claude.json: %w", err)
	}

	for name := range servers {
		fmt.Printf("  synced mcp server: %s\n", name)
	}
	return len(servers), nil
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
func syncSessionHook(userClaudeDir string, dryRun bool) (int, error) {
	synced := 0

	// 1. Copy hook script to ~/.claude/hooks/
	hookDst := filepath.Join(userClaudeDir, "hooks", "adb-session-capture.sh")

	if dryRun {
		fmt.Printf("  [dry-run] hook: adb-session-capture.sh\n")
		fmt.Printf("  [dry-run] settings.json: SessionEnd hook entry\n")
		return 2, nil
	}

	if err := os.MkdirAll(filepath.Dir(hookDst), 0o755); err != nil {
		return 0, fmt.Errorf("creating hooks directory: %w", err)
	}

	hookData, err := claudetpl.FS.ReadFile(path.Join("hooks", "adb-session-capture.sh"))
	if err != nil {
		return 0, fmt.Errorf("reading embedded hook template: %w", err)
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

// ensureStatusLineConfig ensures ~/.claude/settings.json has a statusLine
// entry pointing to ~/.claude/statusline.sh. If a statusLine entry already
// exists it is left untouched; otherwise one is added.
func ensureStatusLineConfig(userClaudeDir string, dryRun bool) error {
	settingsPath := filepath.Join(userClaudeDir, "settings.json")
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

	// Only add statusLine if not already configured
	if _, ok := settings["statusLine"]; ok {
		return nil
	}

	if dryRun {
		fmt.Printf("  [dry-run] settings.json: statusLine entry\n")
		return nil
	}

	settings["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": "~/.claude/statusline.sh",
	}

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings.json: %w", err)
	}
	if err := os.WriteFile(settingsPath, output, 0o644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}
	fmt.Printf("  synced settings.json: statusLine entry\n")
	return nil
}

func init() {
	syncClaudeUserCmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
	syncClaudeUserCmd.Flags().Bool("mcp", false, "Also sync third-party MCP servers (aws-docs, aws-knowledge, context7) to ~/.claude.json")
	rootCmd.AddCommand(syncClaudeUserCmd)
}

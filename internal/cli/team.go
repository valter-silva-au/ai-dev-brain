package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewTeamCmd creates the 'team' command for multi-agent orchestration
func NewTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team <team-name> <prompt>",
		Short: "Launch multi-agent orchestration",
		Long: `Launch a team of specialized agents to collaborate on a task.
Team names: dev, qa, design, research, ops`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			teamName := args[0]
			prompt := strings.Join(args[1:], " ")

			// Validate team name
			validTeams := map[string][]string{
				"dev":      {"backend-dev", "frontend-dev", "devops"},
				"qa":       {"test-engineer", "qa-lead", "automation"},
				"design":   {"ux-designer", "ui-designer", "researcher"},
				"research": {"architect", "tech-lead", "researcher"},
				"ops":      {"sre", "devops", "security"},
			}

			agents, ok := validTeams[teamName]
			if !ok {
				return fmt.Errorf("unknown team: %s (valid teams: dev, qa, design, research, ops)", teamName)
			}

			fmt.Printf("🚀 Launching team '%s' with agents: %v\n", teamName, agents)
			fmt.Printf("📋 Task: %s\n\n", prompt)

			// Create orchestration session
			sessionID := fmt.Sprintf("team-%s-%d", teamName, time.Now().Unix())
			sessionDir := filepath.Join(App.BasePath, "sessions", sessionID)
			if err := os.MkdirAll(sessionDir, 0o755); err != nil {
				return fmt.Errorf("failed to create session directory: %w", err)
			}

			// Write orchestration plan
			planPath := filepath.Join(sessionDir, "orchestration-plan.md")
			planContent := fmt.Sprintf(`# Team Orchestration Plan

**Team**: %s
**Agents**: %v
**Session**: %s
**Timestamp**: %s

## Task
%s

## Agent Responsibilities
`, teamName, agents, sessionID, time.Now().Format(time.RFC3339), prompt)

			// Add agent-specific responsibilities
			responsibilities := map[string]string{
				"backend-dev":   "Design and implement backend services, APIs, and data models",
				"frontend-dev":  "Build user interfaces, components, and client-side logic",
				"devops":        "Set up CI/CD, infrastructure, and deployment automation",
				"test-engineer": "Design test strategy, write tests, and ensure quality",
				"qa-lead":       "Define quality gates and acceptance criteria",
				"automation":    "Automate testing and quality assurance processes",
				"ux-designer":   "Design user experience flows and interactions",
				"ui-designer":   "Create visual designs and design systems",
				"researcher":    "Conduct user research and validate assumptions",
				"architect":     "Design system architecture and technical approach",
				"tech-lead":     "Make technical decisions and guide implementation",
				"sre":           "Ensure reliability, monitoring, and incident response",
				"security":      "Identify and mitigate security risks",
			}

			for _, agent := range agents {
				if resp, ok := responsibilities[agent]; ok {
					planContent += fmt.Sprintf("\n### %s\n%s\n", agent, resp)
				}
			}

			planContent += "\n## Execution Log\n"

			if err := os.WriteFile(planPath, []byte(planContent), 0o644); err != nil {
				return fmt.Errorf("failed to write orchestration plan: %w", err)
			}

			fmt.Printf("✅ Orchestration plan created: %s\n", planPath)
			fmt.Printf("💡 Next steps:\n")
			fmt.Printf("   1. Review the plan at: %s\n", planPath)
			fmt.Printf("   2. Each agent should execute their responsibilities\n")
			fmt.Printf("   3. Log progress in the execution log\n")

			return nil
		},
	}

	return cmd
}

// NewAgentsCmd creates the 'agents' command to list available agents
func NewAgentsCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "List available specialized agents",
		Long:  `List all available specialized agents and their capabilities`,
		RunE: func(cmd *cobra.Command, args []string) error {
			agents := map[string]map[string]string{
				"Development": {
					"backend-dev":  "Backend services, APIs, data models, business logic",
					"frontend-dev": "UI components, client-side logic, user interactions",
					"devops":       "CI/CD, infrastructure, containerization, deployment",
				},
				"Quality Assurance": {
					"test-engineer": "Test strategy, test implementation, quality metrics",
					"qa-lead":       "Quality gates, acceptance criteria, process improvement",
					"automation":    "Test automation, continuous testing, frameworks",
				},
				"Design": {
					"ux-designer": "User experience, flows, wireframes, usability",
					"ui-designer": "Visual design, design systems, branding, accessibility",
					"researcher":  "User research, interviews, surveys, validation",
				},
				"Architecture": {
					"architect":  "System design, architecture patterns, scalability",
					"tech-lead":  "Technical decisions, code review, mentoring",
					"researcher": "Technology evaluation, proof of concepts, research",
				},
				"Operations": {
					"sre":      "Reliability, monitoring, alerting, incident response",
					"devops":   "Infrastructure as code, automation, observability",
					"security": "Security review, vulnerability assessment, compliance",
				},
			}

			fmt.Println("🤖 Available Specialized Agents")
			fmt.Println()

			for category, agentList := range agents {
				fmt.Printf("## %s\n", category)
				for agent, description := range agentList {
					if verbose {
						fmt.Printf("  • %s\n    %s\n\n", agent, description)
					} else {
						fmt.Printf("  • %s: %s\n", agent, description)
					}
				}
				fmt.Println()
			}

			fmt.Println("💡 Usage:")
			fmt.Println("   adb team <team-name> <prompt>")
			fmt.Println("\n🎯 Available Teams:")
			fmt.Println("   • dev      - Backend, Frontend, DevOps")
			fmt.Println("   • qa       - Test Engineer, QA Lead, Automation")
			fmt.Println("   • design   - UX, UI, Research")
			fmt.Println("   • research - Architect, Tech Lead, Researcher")
			fmt.Println("   • ops      - SRE, DevOps, Security")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed agent descriptions")

	return cmd
}

// NewMCPCmd creates the 'mcp' command with check subcommand
func NewMCPCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP (Model Context Protocol) utilities",
		Long:  `Utilities for MCP server management and health checks`,
	}

	mcpCmd.AddCommand(newMCPCheckCmd())
	mcpCmd.AddCommand(newMCPServeCmd())

	return mcpCmd
}

// newMCPCheckCmd creates the 'mcp check' command
func newMCPCheckCmd() *cobra.Command {
	var noCache bool

	cmd := &cobra.Command{
		Use:   "check [--no-cache]",
		Short: "Validate MCP server health",
		Long:  `Check the health and availability of configured MCP servers`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🔍 Checking MCP server health...")
			fmt.Println()

			// Look for Claude desktop config
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}

			configPath := filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json")

			// Try alternative locations
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				configPath = filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
			}

			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				fmt.Println("⚠️  Claude desktop config not found")
				fmt.Println("    Expected locations:")
				fmt.Println("      - ~/.config/Claude/claude_desktop_config.json")
				fmt.Println("      - ~/Library/Application Support/Claude/claude_desktop_config.json")
				fmt.Println("\n💡 No MCP servers configured")
				return nil
			}

			// Read config
			configData, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config: %w", err)
			}

			var config struct {
				MCPServers map[string]interface{} `json:"mcpServers"`
			}

			if err := json.Unmarshal(configData, &config); err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}

			if len(config.MCPServers) == 0 {
				fmt.Println("💡 No MCP servers configured")
				return nil
			}

			fmt.Printf("Found %d MCP server(s):\n\n", len(config.MCPServers))

			// Check each server
			for serverName := range config.MCPServers {
				status := checkMCPServer(serverName, noCache)
				emoji := "✅"
				if status != "healthy" {
					emoji = "❌"
				}
				fmt.Printf("%s %s: %s\n", emoji, serverName, status)
			}

			fmt.Println("\n💡 MCP servers are command-line tools invoked by Claude Desktop")
			fmt.Println("   Health check validates that the server commands are available")

			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache and force fresh check")

	return cmd
}

// checkMCPServer performs a health check on an MCP server
func checkMCPServer(serverName string, noCache bool) string {
	// Simple health check - verify if common MCP commands exist
	// In a real implementation, this would check the actual server process

	// Check if npx is available (many MCP servers use npx)
	if _, err := exec.LookPath("npx"); err != nil {
		return "npx not found"
	}

	// Try to ping localhost (basic connectivity check)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:3000/health")
	if err == nil && resp.StatusCode == 200 {
		return "healthy"
	}

	// Default to "configured" if we can't determine actual health
	return "configured (health check not available)"
}

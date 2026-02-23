package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List available Claude Code agents",
	Long: `List available Claude Code agents from both project-level (.claude/agents/)
and user-level (~/.claude/agents/) configurations.

This wraps the 'claude agents' CLI command when available and falls back
to scanning agent definition files directly.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try using `claude agents` directly first.
		claudePath, err := exec.LookPath("claude")
		if err == nil {
			claudeCmd := exec.Command(claudePath, "agents")
			claudeCmd.Stdout = os.Stdout
			claudeCmd.Stderr = os.Stderr
			if err := claudeCmd.Run(); err == nil {
				return nil
			}
			// Fall through to manual scan if claude agents fails.
		}

		// Manual scan: check project and user agent directories.
		fmt.Println("Available agents:")

		// Project-level agents.
		projectAgentsDir := filepath.Join(".claude", "agents")
		if agents, err := listAgentFiles(projectAgentsDir); err == nil && len(agents) > 0 {
			fmt.Println("  Project agents (.claude/agents/):")
			for _, a := range agents {
				fmt.Printf("    - %s\n", a)
			}
			fmt.Println()
		}

		// User-level agents.
		home, _ := os.UserHomeDir()
		if home != "" {
			userAgentsDir := filepath.Join(home, ".claude", "agents")
			if agents, err := listAgentFiles(userAgentsDir); err == nil && len(agents) > 0 {
				fmt.Println("  User agents (~/.claude/agents/):")
				for _, a := range agents {
					fmt.Printf("    - %s\n", a)
				}
				fmt.Println()
			}
		}

		return nil
	},
}

// listAgentFiles returns agent names from .md files in the given directory.
func listAgentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var agents []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		agents = append(agents, name)
	}
	return agents, nil
}

func init() {
	rootCmd.AddCommand(agentsCmd)
}

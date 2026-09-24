package cli

import (
	"github.com/spf13/cobra"
)

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

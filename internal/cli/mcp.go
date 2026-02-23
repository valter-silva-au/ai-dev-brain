package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	adbmcp "github.com/valter-silva-au/ai-dev-brain/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server commands",
	Long:  "Commands for running the adb MCP (Model Context Protocol) server.",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the adb MCP server on stdio",
	Long: `Start the adb MCP server on stdio transport.

The server exposes adb functionality as MCP tools that AI coding assistants
can call: get_task, list_tasks, update_task_status, get_metrics, get_alerts.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if TaskMgr == nil {
			return fmt.Errorf("task manager not initialized")
		}

		srv := adbmcp.NewServer(TaskMgr, MetricsCalc, AlertEngine, appVersion)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		if err := srv.Run(ctx); err != nil {
			return fmt.Errorf("running MCP server: %w", err)
		}

		return nil
	},
}

var mcpCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configured MCP servers",
	Long: `Check the health of all configured MCP servers defined in .mcp.json.

For HTTP servers, sends a request and checks the response status.
For stdio servers, verifies the command binary exists in PATH.

Results are cached in .adb_mcp_cache.json with a configurable TTL
to speed up repeated checks. Use --no-cache to force a fresh check.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		noCache, _ := cmd.Flags().GetBool("no-cache")

		// Find .mcp.json
		mcpConfigPath := filepath.Join(BasePath, ".mcp.json")
		if _, err := os.Stat(mcpConfigPath); os.IsNotExist(err) {
			// Try current directory.
			cwd, _ := os.Getwd()
			mcpConfigPath = filepath.Join(cwd, ".mcp.json")
			if _, err := os.Stat(mcpConfigPath); os.IsNotExist(err) {
				return fmt.Errorf("no .mcp.json found in workspace or current directory")
			}
		}

		client := integration.NewMCPClient()

		// Check cache first.
		if !noCache {
			cached := client.LoadCache(BasePath)
			if cached != nil {
				fmt.Printf("MCP server status (cached, checked at %s):\n\n", cached.CheckedAt.Format(time.RFC3339))
				printMCPResults(cached)
				return nil
			}
		}

		fmt.Println("Checking MCP servers...")
		result, err := client.CheckServers(mcpConfigPath)
		if err != nil {
			return fmt.Errorf("checking MCP servers: %w", err)
		}

		// Cache results (5 minute TTL).
		_ = client.SaveCache(BasePath, result, 5*time.Minute)

		printMCPResults(result)
		return nil
	},
}

func printMCPResults(result *integration.MCPCheckResult) {
	healthy := 0
	for _, s := range result.Servers {
		status := "FAIL"
		if s.Healthy {
			status = "OK"
			healthy++
		}
		fmt.Printf("  %-20s [%s] %-6s %s", s.Name, s.Type, status, s.ResponseTime.Round(time.Millisecond))
		if s.Error != "" {
			fmt.Printf(" (%s)", s.Error)
		}
		fmt.Println()
	}
	fmt.Printf("\n%d/%d servers healthy\n", healthy, len(result.Servers))
}

func init() {
	mcpCheckCmd.Flags().Bool("no-cache", false, "Skip cache and force fresh check")
	mcpCmd.AddCommand(mcpServeCmd)
	mcpCmd.AddCommand(mcpCheckCmd)
	rootCmd.AddCommand(mcpCmd)
}

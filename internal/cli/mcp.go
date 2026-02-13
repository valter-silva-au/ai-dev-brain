package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	adbmcp "github.com/drapaimern/ai-dev-brain/internal/mcp"
	"github.com/spf13/cobra"
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

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}

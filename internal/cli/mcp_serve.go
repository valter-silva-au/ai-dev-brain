package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/mcpserver"
)

// newMCPServeCmd creates the 'mcp serve' command, which runs ADB as a
// Model Context Protocol server over stdio. This is the command MCP clients
// (Claude Code, Claude Desktop) launch via their server registration:
//
//	{ "command": "adb", "args": ["mcp", "serve"], "type": "stdio" }
//
// The server shares the same App (storage, TaskManager) as every other adb
// command, so the workspace it operates on is resolved the usual way:
// ADB_HOME, then a walked-up .taskconfig/.taskrc, then the cwd.
func newMCPServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run adb as an MCP server over stdio",
		Long: `Run AI Dev Brain as a Model Context Protocol (MCP) server over stdio.

Exposes the task lifecycle (list, create, start, close, update, and bulk
start-all/close-all) as MCP tools so clients like Claude Code can manage the
workspace's tickets natively.

The workspace is resolved like every adb command: the ADB_HOME env var wins,
otherwise adb walks up from the working directory looking for .taskconfig or
.taskrc, falling back to the current directory. When launched by an MCP client
the working directory is often unpredictable, so set ADB_HOME in the server
registration to pin the workspace.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			// stdio is the MCP transport here, so nothing may write to
			// stdout except the protocol itself. ServeStdio blocks until
			// the client disconnects.
			return mcpserver.Serve(App, Version)
		},
	}

	return cmd
}

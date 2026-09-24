package cli

import (
	"os"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

func TestNewMCPCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewMCPCmd()

	if cmd == nil {
		t.Error("NewMCPCmd returned nil")
	}

	// Verify subcommands exist
	checkCmd := cmd.Commands()
	if len(checkCmd) == 0 {
		t.Error("Expected MCP command to have subcommands")
	}
}

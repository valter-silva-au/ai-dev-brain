package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

func TestNewTeamCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "team-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	// Create sessions directory
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("Failed to create sessions dir: %v", err)
	}

	cmd := NewTeamCmd()

	// Test valid team
	cmd.SetArgs([]string{"dev", "implement user authentication"})
	err = cmd.Execute()
	if err != nil {
		t.Errorf("Valid team command failed: %v", err)
	}

	// Verify orchestration plan was created
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatalf("Failed to read sessions dir: %v", err)
	}

	if len(entries) == 0 {
		t.Error("Expected orchestration session to be created")
	}

	// Check for orchestration plan file
	for _, entry := range entries {
		if entry.IsDir() {
			planPath := filepath.Join(sessionsDir, entry.Name(), "orchestration-plan.md")
			if _, err := os.Stat(planPath); err == nil {
				// Plan file exists, success
				return
			}
		}
	}

	t.Error("Orchestration plan file not found")
}

func TestNewTeamCmd_InvalidTeam(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "team-invalid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewTeamCmd()

	// Test invalid team
	cmd.SetArgs([]string{"invalid-team", "some task"})
	err = cmd.Execute()
	if err == nil {
		t.Error("Expected error for invalid team, got nil")
	}
}

func TestNewAgentsCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "agents-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewAgentsCmd()

	// Test agents list
	err = cmd.Execute()
	if err != nil {
		t.Errorf("Agents command failed: %v", err)
	}
}

func TestNewAgentsCmd_Verbose(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "agents-verbose-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewAgentsCmd()

	// Test with verbose flag
	cmd.SetArgs([]string{"--verbose"})
	err = cmd.Execute()
	if err != nil {
		t.Errorf("Agents command with verbose flag failed: %v", err)
	}
}

func TestNewMCPCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
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

func TestMCPCheckCmd(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mcp-check-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewMCPCmd()

	// Test check command (should not error even if no config found)
	cmd.SetArgs([]string{"check"})
	err = cmd.Execute()
	if err != nil {
		t.Errorf("MCP check command failed: %v", err)
	}
}

func TestMCPCheckCmd_NoCache(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mcp-check-nocache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	App = app

	cmd := NewMCPCmd()

	// Test check command with --no-cache flag
	cmd.SetArgs([]string{"check", "--no-cache"})
	err = cmd.Execute()
	if err != nil {
		t.Errorf("MCP check command with --no-cache failed: %v", err)
	}
}

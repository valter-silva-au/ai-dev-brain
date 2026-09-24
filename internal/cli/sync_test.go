package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

func TestSyncCommands(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Load-bearing: with a plain NewApp the `sync wiki` / `sync all` cases can
	// WRITE OUTSIDE their sandbox. newSyncWikiCmd calls App.OpenMemoryStore, which
	// honours an ABSOLUTE hooks.memory.db_path verbatim (internal/memory_store.go)
	// — and that value can come from the GLOBAL tier, i.e. the developer's real
	// $HOME/.taskconfig. A pre-existing file at such a path gets schema DDL run
	// against it while this test still reports PASS (proven: a 0-byte canary grew
	// to 20480 bytes). NewAppIsolated points the global tier at <tmpDir>/.taskconfig
	// (absent), so db_path stays unset and the store resolves to
	// <tmpDir>/.adb/memory.sqlite — inside the sandbox.
	app, err := internal.NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	// Set global app for CLI
	oldApp := App
	App = app
	defer func() { App = oldApp }()

	// `sync claude-user` now also installs the embedded harness into the Claude
	// config dir; keep that hermetic so the test never touches the real ~/.claude.
	// This env var stays: it is a WRITE TARGET resolved by resolveClaudeConfigDir
	// (sync.go), not a config tier, so NewAppIsolated has no say over it.
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmpDir, ".claude"))

	// Create initial backlog
	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	if err := os.WriteFile(backlogPath, []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("failed to create backlog: %v", err)
	}

	tests := []struct {
		name    string
		cmd     func() *cobra.Command
		args    []string
		wantErr bool
	}{
		{
			name:    "sync context",
			cmd:     newSyncContextCmd,
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "sync repos",
			cmd:     newSyncReposCmd,
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "sync claude-user",
			cmd:     newSyncClaudeUserCmd,
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "sync claude-user dry-run",
			cmd:     newSyncClaudeUserCmd,
			args:    []string{"--dry-run"},
			wantErr: false,
		},
		{
			name:    "sync wiki",
			cmd:     newSyncWikiCmd,
			args:    []string{"--out", filepath.Join(tmpDir, "wiki-out")},
			wantErr: false,
		},
		{
			name:    "sync all",
			cmd:     newSyncAllCmd,
			args:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSyncContextRich verifies `adb sync context --rich` routes to the rich
// AIContextGenerator: it writes a multi-section CLAUDE.md and .context_state.yaml
// (whereas the default trivial path only dumps backlog.yaml). This is the T3
// CLI wiring per the ticket-bootstrap-context ADR, part B.
func TestSyncContextRich(t *testing.T) {
	tmpDir := t.TempDir()

	app, err := internal.NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	oldApp := App
	App = app
	defer func() { App = oldApp }()

	cmd := newSyncContextCmd()
	cmd.SetArgs([]string{"--rich"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Rich CLAUDE.md: assert a section the trivial generator never emits.
	agentsPath := filepath.Join(tmpDir, core.CanonicalInstructionFile)
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(data), "## What's Changed") {
		t.Error("rich CLAUDE.md missing '## What's Changed' section")
	}
	if !strings.Contains(string(data), "## Active Tasks") {
		t.Error("rich CLAUDE.md missing '## Active Tasks' section")
	}

	// context_state.yaml must have been maintained (under .adb/ as of #186/#189).
	statePath := filepath.Join(tmpDir, ".adb", "context_state.yaml")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error(".adb/context_state.yaml was not written by `sync context --rich`")
	}
}

func TestSyncTaskContext(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Initialize app
	app, err := internal.NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	// Set global app for CLI
	oldApp := App
	App = app
	defer func() { App = oldApp }()

	// Create task directory
	taskID := "TASK-00001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("failed to create task directory: %v", err)
	}

	cmd := newSyncTaskContextCmd()
	cmd.SetArgs([]string{taskID})
	if err := cmd.Execute(); err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	// Verify context was created
	contextPath := filepath.Join(taskDir, "context.md")
	if _, err := os.Stat(contextPath); os.IsNotExist(err) {
		t.Errorf("context.md was not created")
	}
}

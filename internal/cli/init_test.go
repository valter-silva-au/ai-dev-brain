package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

func TestInitWorkspace(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "test-workspace")

	// Initialize app (needed for CLI)
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	oldApp := App
	App = app
	defer func() { App = oldApp }()

	// Create workspace init command
	cmd := newInitWorkspaceCmd()
	cmd.SetArgs([]string{workspaceDir, "--name", "test-workspace"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify directories were created
	dirs := []string{"tickets", "work", "sessions", ".adb"}
	for _, dir := range dirs {
		dirPath := filepath.Join(workspaceDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("directory %s was not created", dir)
		}
	}

	// Verify files were created
	files := []string{"backlog.yaml", ".taskrc", "README.md"}
	for _, file := range files {
		filePath := filepath.Join(workspaceDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("file %s was not created", file)
		}
	}

	// #186: adb's state now lives under .adb/, so a freshly initialised
	// workspace root must be free of the legacy .adb_* / .task_counter /
	// .context_state.yaml dotfiles — including the two event logs relocated in
	// #190 (.events.jsonl / .governance.jsonl).
	for _, legacy := range []string{
		".task_counter",
		".session_counter",
		".context_state.yaml",
		".adb_memory.sqlite",
		".adb_scheduler.pid",
		".adb_scheduler.log",
		".adb_scheduler_state.yaml",
		".adb_automation_cursor",
		".adb_session_changes",
		".adb_evidence_reads",
		".events.jsonl",
		".governance.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(workspaceDir, legacy)); !os.IsNotExist(err) {
			t.Errorf("legacy state file %q should not exist at the workspace root (err=%v)", legacy, err)
		}
	}
}

// TestInitWorkspace_TaskrcUsesFlatSchema is a regression test for the bug where
// `adb init workspace` emitted a .taskrc with `name:` + nested `build:`/`git:`
// blocks that RepoConfig has no fields for — so Viper silently dropped them and
// the workspace's repo_name / build_command never took effect. The template
// must emit the flat keys the binary actually parses, and they must round-trip
// through the real config loader.
func TestInitWorkspace_TaskrcUsesFlatSchema(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "flat-schema-ws")

	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	oldApp := App
	App = app
	defer func() { App = oldApp }()

	cmd := newInitWorkspaceCmd()
	cmd.SetArgs([]string{
		workspaceDir,
		"--name", "flat-schema-ws",
		"--build-command", "go build ./...",
		"--test-command", "go test ./...",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	taskrcPath := filepath.Join(workspaceDir, ".taskrc")
	raw, err := os.ReadFile(taskrcPath)
	if err != nil {
		t.Fatalf("failed to read .taskrc: %v", err)
	}
	content := string(raw)

	// The flat keys the binary reads must be present...
	for _, key := range []string{"repo_name:", "task_id_prefix:", "build_command:", "test_command:"} {
		if !strings.Contains(content, key) {
			t.Errorf(".taskrc missing flat key %q\n---\n%s", key, content)
		}
	}
	// ...and the dropped-on-parse nested keys must NOT be reintroduced.
	for _, bad := range []string{"\nname:", "\nbuild:", "\ngit:", "worktree_dir:"} {
		if strings.Contains(content, bad) {
			t.Errorf(".taskrc reintroduced inert key %q — RepoConfig has no field for it\n---\n%s", strings.TrimSpace(bad), content)
		}
	}

	// The real loader must parse the values back out (proves they're honored).
	cm := core.NewViperConfigManager("", taskrcPath)
	repo, err := cm.GetRepoConfig()
	if err != nil {
		t.Fatalf("GetRepoConfig() error = %v", err)
	}
	if repo.RepoName != "flat-schema-ws" {
		t.Errorf("repo_name did not round-trip: want flat-schema-ws, got %q", repo.RepoName)
	}
	if repo.BuildCommand != "go build ./..." {
		t.Errorf("build_command did not round-trip: want %q, got %q", "go build ./...", repo.BuildCommand)
	}
	if repo.TestCommand != "go test ./..." {
		t.Errorf("test_command did not round-trip: want %q, got %q", "go test ./...", repo.TestCommand)
	}
}

func TestInitClaude(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Initialize app
	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	oldApp := App
	App = app
	defer func() { App = oldApp }()

	// Create claude init command
	cmd := newInitClaudeCmd()
	cmd.SetArgs([]string{tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify files were created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Errorf("CLAUDE.md was not created")
	}

	userContextPath := filepath.Join(tmpDir, ".adb", "claude-user.md")
	if _, err := os.Stat(userContextPath); os.IsNotExist(err) {
		t.Errorf("claude-user.md was not created")
	}
}

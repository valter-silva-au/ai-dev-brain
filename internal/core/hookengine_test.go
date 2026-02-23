package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func defaultTestHookConfig() models.HookConfig {
	return models.DefaultHookConfig()
}

func disabledHookConfig() models.HookConfig {
	return models.HookConfig{Enabled: false}
}

func TestHookEngine_PreToolUse_BlockVendor(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, defaultTestHookConfig(), nil, nil)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"vendor/ blocked", "vendor/foo.go", true},
		{"nested vendor/ blocked", "some/vendor/bar.go", true},
		{"go.sum blocked", "go.sum", true},
		{"nested go.sum blocked", "subdir/go.sum", true},
		{"normal file allowed", "internal/core/foo.go", false},
		{"empty path allowed", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := hooks.PreToolUseInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": tt.path,
				},
			}
			err := engine.HandlePreToolUse(input)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHookEngine_PreToolUse_Disabled(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PreToolUse.Enabled = false
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "vendor/foo.go",
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("disabled hook should not block: %v", err)
	}
}

func TestHookEngine_PreToolUse_GloballyDisabled(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, disabledHookConfig(), nil, nil)

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "vendor/foo.go",
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("globally disabled hook should not block: %v", err)
	}
}

func TestHookEngine_PostToolUse_ChangeTracking(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	// Disable go format to avoid needing gofmt binary.
	cfg.PostToolUse.GoFormat = false
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PostToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "internal/core/foo.go",
		},
	}

	if err := engine.HandlePostToolUse(input); err != nil {
		t.Fatalf("HandlePostToolUse failed: %v", err)
	}

	// Verify change was tracked.
	tracker := hooks.NewChangeTracker(dir)
	entries, err := tracker.Read()
	if err != nil {
		t.Fatalf("reading changes: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 change entry, got %d", len(entries))
	}
	if entries[0].FilePath != "internal/core/foo.go" {
		t.Errorf("change entry file = %q, want %q", entries[0].FilePath, "internal/core/foo.go")
	}
	if entries[0].Tool != "Edit" {
		t.Errorf("change entry tool = %q, want %q", entries[0].Tool, "Edit")
	}
}

func TestHookEngine_PostToolUse_Disabled(t *testing.T) {
	dir := t.TempDir()
	cfg := disabledHookConfig()
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PostToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "foo.go",
		},
	}
	if err := engine.HandlePostToolUse(input); err != nil {
		t.Errorf("disabled hook should not error: %v", err)
	}

	// Verify no changes tracked.
	tracker := hooks.NewChangeTracker(dir)
	entries, _ := tracker.Read()
	if len(entries) != 0 {
		t.Errorf("disabled hook should not track changes, got %d entries", len(entries))
	}
}

func TestHookEngine_PostToolUse_EmptyPath(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, defaultTestHookConfig(), nil, nil)

	input := hooks.PostToolUseInput{
		ToolName: "Read",
		// No file_path in tool_input.
	}
	if err := engine.HandlePostToolUse(input); err != nil {
		t.Errorf("empty path should not error: %v", err)
	}
}

func TestHookEngine_Stop_ContextUpdate(t *testing.T) {
	dir := t.TempDir()

	// Set up a task ticket with context.md.
	taskID := "TASK-00001"
	ticketPath := filepath.Join(dir, "tickets", taskID)
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("creating ticket dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "context.md"), []byte("# Context\n"), 0o644); err != nil {
		t.Fatalf("writing context.md: %v", err)
	}
	statusYAML := "id: TASK-00001\nstatus: in_progress\nupdated: \"2025-01-01T00:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(ticketPath, "status.yaml"), []byte(statusYAML), 0o644); err != nil {
		t.Fatalf("writing status.yaml: %v", err)
	}

	// Pre-populate change tracker.
	tracker := hooks.NewChangeTracker(dir)
	_ = tracker.Append(models.SessionChangeEntry{Timestamp: 1000, Tool: "Edit", FilePath: "internal/core/foo.go"})
	_ = tracker.Append(models.SessionChangeEntry{Timestamp: 2000, Tool: "Write", FilePath: "internal/cli/bar.go"})

	// Set ADB_TASK_ID for ticket path resolution.
	t.Setenv("ADB_TASK_ID", taskID)

	cfg := defaultTestHookConfig()
	// Disable build and vet checks (they'd fail in the test environment).
	cfg.Stop.BuildCheck = false
	cfg.Stop.VetCheck = false
	cfg.Stop.UncommittedCheck = false

	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.StopInput{}
	if err := engine.HandleStop(input); err != nil {
		t.Fatalf("HandleStop failed: %v", err)
	}

	// Verify context.md was updated.
	data, err := os.ReadFile(filepath.Join(ticketPath, "context.md"))
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "### Session") || !strings.Contains(content, "internal/core/") || !strings.Contains(content, "internal/cli/") {
		t.Error("context.md should contain session summary with modified directories")
	}

	// Verify change tracker was cleaned up.
	entries, _ := tracker.Read()
	if len(entries) != 0 {
		t.Errorf("change tracker should be cleaned up, got %d entries", len(entries))
	}
}

func TestHookEngine_Stop_Disabled(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, disabledHookConfig(), nil, nil)

	input := hooks.StopInput{}
	if err := engine.HandleStop(input); err != nil {
		t.Errorf("disabled hook should not error: %v", err)
	}
}

func TestHookEngine_TaskCompleted_Disabled(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, disabledHookConfig(), nil, nil)

	input := hooks.TaskCompletedInput{}
	if err := engine.HandleTaskCompleted(input); err != nil {
		t.Errorf("disabled hook should not error: %v", err)
	}
}

func TestHookEngine_SessionEnd_Disabled(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, disabledHookConfig(), nil, nil)

	input := hooks.SessionEndInput{}
	if err := engine.HandleSessionEnd(input); err != nil {
		t.Errorf("disabled hook should not error: %v", err)
	}
}

func TestHookEngine_SessionEnd_ContextUpdate(t *testing.T) {
	dir := t.TempDir()

	// Set up a task ticket.
	taskID := "TASK-00002"
	ticketPath := filepath.Join(dir, "tickets", taskID)
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("creating ticket dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "context.md"), []byte("# Context\n"), 0o644); err != nil {
		t.Fatalf("writing context.md: %v", err)
	}

	// Pre-populate change tracker.
	tracker := hooks.NewChangeTracker(dir)
	_ = tracker.Append(models.SessionChangeEntry{Timestamp: 1000, Tool: "Edit", FilePath: "foo.go"})

	t.Setenv("ADB_TASK_ID", taskID)

	cfg := defaultTestHookConfig()
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.SessionEndInput{SessionID: "abc123"}
	if err := engine.HandleSessionEnd(input); err != nil {
		t.Fatalf("HandleSessionEnd failed: %v", err)
	}

	// Verify context was updated.
	data, err := os.ReadFile(filepath.Join(ticketPath, "context.md"))
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	if len(data) <= len("# Context\n") {
		t.Error("context.md should have been appended to")
	}
}

func TestHookEngine_ArchitectureGuard(t *testing.T) {
	dir := t.TempDir()

	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ArchitectureGuard = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	// Create a core file that imports storage.
	coreDir := filepath.Join(dir, "internal", "core")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatalf("creating core dir: %v", err)
	}
	badFile := filepath.Join(coreDir, "bad.go")
	content := `package core

import "github.com/valter-silva-au/ai-dev-brain/internal/storage"

var _ = storage.NewBacklogManager
`
	if err := os.WriteFile(badFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing bad file: %v", err)
	}

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": badFile,
		},
	}
	if err := engine.HandlePreToolUse(input); err == nil {
		t.Error("architecture guard should block core/ importing storage/")
	}
}

func TestHookEngine_ResolveTicketPath_NoTaskID(t *testing.T) {
	dir := t.TempDir()
	engine := &hookEngine{basePath: dir}

	t.Setenv("ADB_TASK_ID", "")
	if got := engine.resolveTicketPath(); got != "" {
		t.Errorf("no task ID should resolve to empty, got %q", got)
	}
}

func TestHookEngine_ResolveTicketPath_Active(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00005"
	ticketPath := filepath.Join(dir, "tickets", taskID)
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("creating ticket dir: %v", err)
	}

	engine := &hookEngine{basePath: dir}
	t.Setenv("ADB_TASK_ID", taskID)

	if got := engine.resolveTicketPath(); got != ticketPath {
		t.Errorf("resolveTicketPath() = %q, want %q", got, ticketPath)
	}
}

func TestHookEngine_ResolveTicketPath_Archived(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00006"
	archivedPath := filepath.Join(dir, "tickets", "_archived", taskID)
	if err := os.MkdirAll(archivedPath, 0o755); err != nil {
		t.Fatalf("creating archived dir: %v", err)
	}

	engine := &hookEngine{basePath: dir}
	t.Setenv("ADB_TASK_ID", taskID)

	if got := engine.resolveTicketPath(); got != archivedPath {
		t.Errorf("resolveTicketPath() = %q, want %q", got, archivedPath)
	}
}

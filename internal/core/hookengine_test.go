package core

import (
	"fmt"
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

// --- Test mocks ---

type mockKnowledgeExtractor struct {
	extractCalls  int
	extractResult *models.ExtractedKnowledge
	extractErr    error
	wikiCalls     int
	wikiErr       error
	adrCalls      int
	adrErr        error
}

func (m *mockKnowledgeExtractor) ExtractFromTask(taskID string) (*models.ExtractedKnowledge, error) {
	m.extractCalls++
	return m.extractResult, m.extractErr
}

func (m *mockKnowledgeExtractor) GenerateHandoff(taskID string) (*models.HandoffDocument, error) {
	return nil, nil
}

func (m *mockKnowledgeExtractor) UpdateWiki(knowledge *models.ExtractedKnowledge) error {
	m.wikiCalls++
	return m.wikiErr
}

func (m *mockKnowledgeExtractor) CreateADR(decision models.Decision, taskID string) (string, error) {
	m.adrCalls++
	return "", m.adrErr
}

type mockConflictDetector struct {
	conflicts []Conflict
	err       error
}

func (m *mockConflictDetector) CheckForConflicts(ctx ConflictContext) ([]Conflict, error) {
	return m.conflicts, m.err
}

// cmdResult maps a command name to its output and error.
type cmdResult struct {
	output string
	err    error
}

// fakeCmdRunner returns a command runner that returns preconfigured results
// based on the command name. It also records which commands were invoked.
func fakeCmdRunner(results map[string]cmdResult, invoked *[]string) func(string, ...string) (string, error) {
	return func(name string, args ...string) (string, error) {
		key := name
		if invoked != nil {
			*invoked = append(*invoked, name+" "+strings.Join(args, " "))
		}
		if r, ok := results[key]; ok {
			return r.output, r.err
		}
		return "", nil
	}
}

// setupTicketDir creates a ticket directory with context.md and status.yaml.
func setupTicketDir(t *testing.T, basePath, taskID string) string {
	t.Helper()
	ticketPath := filepath.Join(basePath, "tickets", taskID)
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("creating ticket dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "context.md"), []byte("# Context\n"), 0o644); err != nil {
		t.Fatalf("writing context.md: %v", err)
	}
	statusYAML := fmt.Sprintf("id: %s\nstatus: in_progress\nupdated: \"2025-01-01T00:00:00Z\"\n", taskID)
	if err := os.WriteFile(filepath.Join(ticketPath, "status.yaml"), []byte(statusYAML), 0o644); err != nil {
		t.Fatalf("writing status.yaml: %v", err)
	}
	return ticketPath
}

// --- PreToolUse tests ---

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

func TestHookEngine_PreToolUse_ADRConflictWithNilDetector(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ADRConflictCheck = true
	// conflictDt is nil -- should not panic.
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "internal/core/foo.go",
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("nil conflict detector should not error: %v", err)
	}
}

func TestHookEngine_PreToolUse_ADRConflictWarning(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ADRConflictCheck = true

	mockCD := &mockConflictDetector{
		conflicts: []Conflict{
			{Severity: "medium", Description: "conflicts with ADR-001", Source: "ADR-001"},
		},
	}
	engine := NewHookEngine(dir, cfg, nil, mockCD)

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "internal/core/foo.go",
		},
	}
	// ADR conflicts warn but do not block.
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("ADR conflict should warn, not block: %v", err)
	}
}

func TestHookEngine_PreToolUse_ArchitectureGuardNonCoreFile(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ArchitectureGuard = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	// Non-core file should not be checked.
	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": filepath.Join(dir, "internal", "cli", "foo.go"),
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("non-core file should not trigger architecture guard: %v", err)
	}
}

func TestHookEngine_PreToolUse_ArchitectureGuardNonGoFile(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ArchitectureGuard = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	// .md file in core/ should not be checked.
	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": filepath.Join(dir, "internal", "core", "README.md"),
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("non-Go file in core/ should not trigger architecture guard: %v", err)
	}
}

// --- PostToolUse tests ---

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

func TestHookEngine_PostToolUse_EmptyToolName(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.PostToolUse.GoFormat = false
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PostToolUseInput{
		ToolName: "",
		ToolInput: map[string]interface{}{
			"file_path": "foo.go",
		},
	}
	if err := engine.HandlePostToolUse(input); err != nil {
		t.Fatalf("HandlePostToolUse failed: %v", err)
	}

	// Verify "unknown" was recorded as tool name.
	tracker := hooks.NewChangeTracker(dir)
	entries, _ := tracker.Read()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Tool != "unknown" {
		t.Errorf("empty tool name should be recorded as 'unknown', got %q", entries[0].Tool)
	}
}

func TestHookEngine_PostToolUse_DependencyDetection(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00010"
	setupTicketDir(t, dir, taskID)
	t.Setenv("ADB_TASK_ID", taskID)

	cfg := defaultTestHookConfig()
	cfg.PostToolUse.GoFormat = false
	cfg.PostToolUse.DependencyDetection = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	input := hooks.PostToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "go.mod",
		},
	}
	if err := engine.HandlePostToolUse(input); err != nil {
		t.Fatalf("HandlePostToolUse failed: %v", err)
	}

	// Verify context.md was updated with dependency notice.
	data, err := os.ReadFile(filepath.Join(dir, "tickets", taskID, "context.md"))
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	if !strings.Contains(string(data), "Dependency Change") {
		t.Error("context.md should contain dependency change notice")
	}
}

// --- Stop tests ---

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

func TestHookEngine_Stop_AdvisoryWithRunner(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.Stop.UncommittedCheck = true
	cfg.Stop.BuildCheck = true
	cfg.Stop.VetCheck = true
	cfg.Stop.ContextUpdate = false
	cfg.Stop.StatusTimestamp = false

	var invoked []string
	runner := fakeCmdRunner(map[string]cmdResult{
		"git": {output: " M main.go\n", err: nil},
		"go":  {output: "", err: nil},
	}, &invoked)

	engine := newHookEngineWithRunner(dir, cfg, nil, nil, runner)
	if err := engine.HandleStop(hooks.StopInput{}); err != nil {
		t.Fatalf("HandleStop should not error: %v", err)
	}

	// All three advisory checks should have been invoked.
	if len(invoked) < 3 {
		t.Errorf("expected at least 3 commands invoked, got %d: %v", len(invoked), invoked)
	}
}

// --- TaskCompleted Phase A tests ---

func TestHookEngine_TaskCompleted_Disabled(t *testing.T) {
	dir := t.TempDir()
	engine := NewHookEngine(dir, disabledHookConfig(), nil, nil)

	input := hooks.TaskCompletedInput{}
	if err := engine.HandleTaskCompleted(input); err != nil {
		t.Errorf("disabled hook should not error: %v", err)
	}
}

func TestHookEngine_TaskCompleted_PhaseA(t *testing.T) {
	tests := []struct {
		name        string
		config      func() models.HookConfig
		cmdResults  map[string]cmdResult
		wantErr     bool
		errContains string
	}{
		{
			name: "uncommitted Go files blocks",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = true
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"git": {output: "main.go\nhelper.go\n", err: nil},
			},
			wantErr:     true,
			errContains: "BLOCKED: uncommitted Go changes",
		},
		{
			name: "no uncommitted files passes",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = true
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"git": {output: "", err: nil},
			},
			wantErr: false,
		},
		{
			name: "tests pass",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = true
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"go": {output: "ok\n", err: nil},
			},
			wantErr: false,
		},
		{
			name: "tests fail blocks",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = true
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"go": {output: "FAIL foo_test.go:42", err: fmt.Errorf("exit status 1")},
			},
			wantErr:     true,
			errContains: "BLOCKED: tests failed",
		},
		{
			name: "lint passes",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = true
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"golangci-lint": {output: "", err: nil},
			},
			wantErr: false,
		},
		{
			name: "lint fails blocks",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = true
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"golangci-lint": {output: "foo.go:1: error", err: fmt.Errorf("exit status 1")},
			},
			wantErr:     true,
			errContains: "BLOCKED: lint failed",
		},
		{
			name: "custom test command",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = true
				cfg.TaskCompleted.TestCommand = "make test"
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"make": {output: "ok", err: nil},
			},
			wantErr: false,
		},
		{
			name: "empty test command guard",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = true
				cfg.TaskCompleted.TestCommand = "   " // whitespace-only
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults:  map[string]cmdResult{},
			wantErr:     true,
			errContains: "test command is empty",
		},
		{
			name: "empty lint command guard",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = true
				cfg.TaskCompleted.LintCommand = "   "
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults:  map[string]cmdResult{},
			wantErr:     true,
			errContains: "lint command is empty",
		},
		{
			name: "all Phase A disabled passes",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = false
				cfg.TaskCompleted.RunTests = false
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{},
			wantErr:    false,
		},
		{
			name: "uncommitted blocks before tests run",
			config: func() models.HookConfig {
				cfg := defaultTestHookConfig()
				cfg.TaskCompleted.CheckUncommitted = true
				cfg.TaskCompleted.RunTests = true
				cfg.TaskCompleted.RunLint = false
				cfg.TaskCompleted.UpdateContext = false
				return cfg
			},
			cmdResults: map[string]cmdResult{
				"git": {output: "dirty.go\n", err: nil},
				"go":  {output: "ok", err: nil},
			},
			wantErr:     true,
			errContains: "BLOCKED: uncommitted Go changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var invoked []string
			runner := fakeCmdRunner(tt.cmdResults, &invoked)
			engine := newHookEngineWithRunner(dir, tt.config(), nil, nil, runner)

			err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestHookEngine_TaskCompleted_ExecutionOrder(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = true
	cfg.TaskCompleted.RunTests = true
	cfg.TaskCompleted.RunLint = true
	cfg.TaskCompleted.UpdateContext = false

	var invoked []string
	runner := fakeCmdRunner(map[string]cmdResult{
		"git":           {output: "", err: nil},
		"go":            {output: "ok", err: nil},
		"golangci-lint": {output: "", err: nil},
	}, &invoked)

	engine := newHookEngineWithRunner(dir, cfg, nil, nil, runner)
	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify order: git (uncommitted) -> go (test) -> golangci-lint (lint).
	if len(invoked) < 4 { // 2x git diff + go test + golangci-lint
		t.Fatalf("expected at least 4 invocations, got %d: %v", len(invoked), invoked)
	}
	// First two should be git diff calls.
	if !strings.HasPrefix(invoked[0], "git diff") {
		t.Errorf("first invocation should be git diff, got %q", invoked[0])
	}
	// Then go test.
	foundGo := false
	foundLint := false
	goIdx := -1
	lintIdx := -1
	for i, cmd := range invoked {
		if strings.HasPrefix(cmd, "go test") {
			foundGo = true
			goIdx = i
		}
		if strings.HasPrefix(cmd, "golangci-lint") {
			foundLint = true
			lintIdx = i
		}
	}
	if !foundGo {
		t.Error("go test should have been invoked")
	}
	if !foundLint {
		t.Error("golangci-lint should have been invoked")
	}
	if goIdx >= 0 && lintIdx >= 0 && goIdx > lintIdx {
		t.Error("go test should run before golangci-lint")
	}
}

// --- TaskCompleted Phase B tests ---

func TestHookEngine_TaskCompleted_PhaseB_SingleExtraction(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00020"
	setupTicketDir(t, dir, taskID)
	t.Setenv("ADB_TASK_ID", taskID)

	mockKE := &mockKnowledgeExtractor{
		extractResult: &models.ExtractedKnowledge{
			TaskID:    taskID,
			Learnings: []string{"learned something"},
			Decisions: []models.Decision{{Decision: "use gRPC"}},
		},
	}

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.ExtractKnowledge = true
	cfg.TaskCompleted.UpdateWiki = true
	cfg.TaskCompleted.GenerateADRs = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, mockKE, nil, runner)

	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ExtractFromTask was called exactly once (not three times).
	if mockKE.extractCalls != 1 {
		t.Errorf("ExtractFromTask called %d times, want 1", mockKE.extractCalls)
	}
	if mockKE.wikiCalls != 1 {
		t.Errorf("UpdateWiki called %d times, want 1", mockKE.wikiCalls)
	}
	if mockKE.adrCalls != 1 {
		t.Errorf("CreateADR called %d times, want 1 (one decision)", mockKE.adrCalls)
	}
}

func TestHookEngine_TaskCompleted_PhaseB_ExtractionErrorNonBlocking(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00021"
	t.Setenv("ADB_TASK_ID", taskID)

	mockKE := &mockKnowledgeExtractor{
		extractErr: fmt.Errorf("extraction failed"),
	}

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.ExtractKnowledge = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, mockKE, nil, runner)

	// Phase B errors are non-blocking -- should return nil.
	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Errorf("Phase B extraction error should not block: %v", err)
	}
}

func TestHookEngine_TaskCompleted_PhaseB_NilKnowledgeExtractor(t *testing.T) {
	dir := t.TempDir()

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.ExtractKnowledge = true
	cfg.TaskCompleted.UpdateWiki = true
	cfg.TaskCompleted.GenerateADRs = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, nil, nil, runner)

	// nil knowledgeX -- should not panic.
	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Errorf("nil knowledgeX should not error: %v", err)
	}
}

func TestHookEngine_TaskCompleted_PhaseB_NoTaskID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ADB_TASK_ID", "")

	mockKE := &mockKnowledgeExtractor{
		extractResult: &models.ExtractedKnowledge{},
	}

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.ExtractKnowledge = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, mockKE, nil, runner)

	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Errorf("no ADB_TASK_ID should skip Phase B silently: %v", err)
	}
	if mockKE.extractCalls != 0 {
		t.Errorf("ExtractFromTask should not be called without ADB_TASK_ID, called %d times", mockKE.extractCalls)
	}
}

func TestHookEngine_TaskCompleted_PhaseB_ADRPerDecision(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00022"
	setupTicketDir(t, dir, taskID)
	t.Setenv("ADB_TASK_ID", taskID)

	mockKE := &mockKnowledgeExtractor{
		extractResult: &models.ExtractedKnowledge{
			TaskID: taskID,
			Decisions: []models.Decision{
				{Decision: "use gRPC"},
				{Decision: "use Postgres"},
				{Decision: "use Redis"},
			},
		},
	}

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.ExtractKnowledge = false
	cfg.TaskCompleted.UpdateWiki = false
	cfg.TaskCompleted.GenerateADRs = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, mockKE, nil, runner)

	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockKE.adrCalls != 3 {
		t.Errorf("CreateADR called %d times, want 3 (one per decision)", mockKE.adrCalls)
	}
}

func TestHookEngine_TaskCompleted_CompletionSummary(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00023"
	ticketPath := setupTicketDir(t, dir, taskID)
	t.Setenv("ADB_TASK_ID", taskID)

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.UpdateContext = true

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, nil, nil, runner)

	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(ticketPath, "context.md"))
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Task Completed") || !strings.Contains(content, "Quality gates passed") {
		t.Error("context.md should contain completion summary")
	}
}

func TestHookEngine_TaskCompleted_PhaseB_WikiError(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00024"
	t.Setenv("ADB_TASK_ID", taskID)
	setupTicketDir(t, dir, taskID)

	mockKE := &mockKnowledgeExtractor{
		extractResult: &models.ExtractedKnowledge{TaskID: taskID},
		wikiErr:       fmt.Errorf("wiki write failed"),
	}

	cfg := defaultTestHookConfig()
	cfg.TaskCompleted.CheckUncommitted = false
	cfg.TaskCompleted.RunTests = false
	cfg.TaskCompleted.RunLint = false
	cfg.TaskCompleted.UpdateWiki = true
	cfg.TaskCompleted.UpdateContext = false

	runner := fakeCmdRunner(map[string]cmdResult{}, nil)
	engine := newHookEngineWithRunner(dir, cfg, mockKE, nil, runner)

	// Wiki error is non-blocking.
	if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
		t.Errorf("wiki error should be non-blocking: %v", err)
	}
}

// --- SessionEnd tests ---

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

func TestHookEngine_SessionEnd_NoContextUpdateWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00030"
	ticketPath := setupTicketDir(t, dir, taskID)
	t.Setenv("ADB_TASK_ID", taskID)

	// Pre-populate tracker.
	tracker := hooks.NewChangeTracker(dir)
	_ = tracker.Append(models.SessionChangeEntry{Timestamp: 1000, Tool: "Edit", FilePath: "foo.go"})

	cfg := defaultTestHookConfig()
	cfg.SessionEnd.UpdateContext = false
	engine := NewHookEngine(dir, cfg, nil, nil)

	if err := engine.HandleSessionEnd(hooks.SessionEndInput{}); err != nil {
		t.Fatalf("HandleSessionEnd failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(ticketPath, "context.md"))
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	if string(data) != "# Context\n" {
		t.Error("context.md should not be modified when UpdateContext is false")
	}
}

// --- Architecture guard tests ---

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

func TestHookEngine_ArchitectureGuard_IntegrationImport(t *testing.T) {
	dir := t.TempDir()

	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ArchitectureGuard = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	coreDir := filepath.Join(dir, "internal", "core")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatalf("creating core dir: %v", err)
	}
	badFile := filepath.Join(coreDir, "bad2.go")
	content := `package core

import "github.com/valter-silva-au/ai-dev-brain/internal/integration"

var _ = integration.NewGitWorktreeManager
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
	err := engine.HandlePreToolUse(input)
	if err == nil {
		t.Error("architecture guard should block core/ importing integration/")
	}
	if err != nil && !strings.Contains(err.Error(), "integration") {
		t.Errorf("error should mention integration, got: %v", err)
	}
}

func TestHookEngine_ArchitectureGuard_CleanFile(t *testing.T) {
	dir := t.TempDir()

	cfg := defaultTestHookConfig()
	cfg.PreToolUse.ArchitectureGuard = true
	engine := NewHookEngine(dir, cfg, nil, nil)

	coreDir := filepath.Join(dir, "internal", "core")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatalf("creating core dir: %v", err)
	}
	goodFile := filepath.Join(coreDir, "clean.go")
	content := `package core

import "fmt"

func hello() { fmt.Println("hi") }
`
	if err := os.WriteFile(goodFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing good file: %v", err)
	}

	input := hooks.PreToolUseInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path": goodFile,
		},
	}
	if err := engine.HandlePreToolUse(input); err != nil {
		t.Errorf("clean core file should not trigger architecture guard: %v", err)
	}
}

// --- ResolveTicketPath tests ---

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

// --- Package-level helper tests ---

func TestFilterGoFiles(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   int
		expect []string
	}{
		{"empty string", "", 0, nil},
		{"only newlines", "\n\n\n", 0, nil},
		{"single go file", "main.go", 1, []string{"main.go"}},
		{"multiple go files", "main.go\nhelper.go\nfoo_test.go", 3, []string{"main.go", "helper.go", "foo_test.go"}},
		{"mixed files", "main.go\nREADME.md\nfoo.go", 2, []string{"main.go", "foo.go"}},
		{"whitespace lines", "  main.go  \n  \n  foo.go  ", 2, []string{"main.go", "foo.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterGoFiles(tt.input)
			if len(got) != tt.want {
				t.Errorf("filterGoFiles() returned %d files, want %d: %v", len(got), tt.want, got)
			}
			for i, expected := range tt.expect {
				if i < len(got) && got[i] != expected {
					t.Errorf("filterGoFiles()[%d] = %q, want %q", i, got[i], expected)
				}
			}
		})
	}
}

func TestIsVendorPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"vendor/foo.go", true},
		{"vendor/sub/bar.go", true},
		{"some/vendor/baz.go", true},
		{"internal/core/foo.go", false},
		{"vendorish/foo.go", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isVendorPath(tt.path); got != tt.want {
				t.Errorf("isVendorPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsGoSumPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"go.sum", true},
		{"subdir/go.sum", true},
		{"go.mod", false},
		{"go.summary", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isGoSumPath(tt.path); got != tt.want {
				t.Errorf("isGoSumPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

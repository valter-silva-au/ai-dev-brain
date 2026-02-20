package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// mockWorktreeCreator is a test double for WorktreeCreator.
type mockWorktreeCreator struct {
	createdPath string
	err         error
	lastConfig  WorktreeCreateConfig
}

func (m *mockWorktreeCreator) CreateWorktree(config WorktreeCreateConfig) (string, error) {
	m.lastConfig = config
	if m.err != nil {
		return "", m.err
	}
	return m.createdPath, nil
}

func TestBootstrap_CreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	result, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test feature",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TaskID != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", result.TaskID)
	}

	// Verify ticket directory exists.
	expectedTicket := filepath.Join(dir, "tickets", "TASK-00001")
	if result.TicketPath != expectedTicket {
		t.Errorf("expected ticket path %s, got %s", expectedTicket, result.TicketPath)
	}

	// Verify communications/ directory exists.
	commDir := filepath.Join(expectedTicket, "communications")
	if info, err := os.Stat(commDir); err != nil || !info.IsDir() {
		t.Errorf("communications/ directory should exist at %s", commDir)
	}

	// Verify notes.md exists and has content.
	notesPath := filepath.Join(expectedTicket, "notes.md")
	notes, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("notes.md should exist: %v", err)
	}
	if !strings.Contains(string(notes), "Feature Notes") {
		t.Error("notes.md should contain feat template content")
	}

	// Verify design.md exists and has content.
	designPath := filepath.Join(expectedTicket, "design.md")
	design, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("design.md should exist: %v", err)
	}
	if !strings.Contains(string(design), "Technical Design") {
		t.Error("design.md should contain design template content")
	}

	// Verify context.md exists and has scaffold.
	contextPath := filepath.Join(expectedTicket, "context.md")
	context, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("context.md should exist: %v", err)
	}
	if !strings.Contains(string(context), "Task Context: TASK-00001") {
		t.Error("context.md should reference the task ID")
	}
	if result.ContextPath != contextPath {
		t.Errorf("expected context path %s, got %s", contextPath, result.ContextPath)
	}

	// Verify status.yaml exists and has correct fields.
	statusPath := filepath.Join(expectedTicket, "status.yaml")
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("status.yaml should exist: %v", err)
	}

	var task models.Task
	if err := yaml.Unmarshal(statusData, &task); err != nil {
		t.Fatalf("failed to parse status.yaml: %v", err)
	}
	if task.ID != "TASK-00001" {
		t.Errorf("status.yaml ID should be TASK-00001, got %s", task.ID)
	}
	if task.Title != "Test feature" {
		t.Errorf("status.yaml title should be 'Test feature', got %s", task.Title)
	}
	if task.Type != models.TaskTypeFeat {
		t.Errorf("status.yaml type should be feat, got %s", task.Type)
	}
	if task.Status != models.StatusBacklog {
		t.Errorf("status.yaml status should be backlog, got %s", task.Status)
	}
}

func TestBootstrap_BugTemplate(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "BUG", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	result, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeBug,
		Title: "Fix crash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(result.TicketPath, "notes.md"))
	if err != nil {
		t.Fatalf("notes.md should exist: %v", err)
	}
	if !strings.Contains(string(notes), "Steps to Reproduce") {
		t.Error("bug notes.md should contain Steps to Reproduce")
	}
}

func TestBootstrap_WithWorktree(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	mockWt := &mockWorktreeCreator{createdPath: "/tmp/worktree/TASK-00001"}
	bs := NewBootstrapSystem(dir, idGen, mockWt, tmplMgr)

	result, err := bs.Bootstrap(BootstrapConfig{
		Type:       models.TaskTypeFeat,
		Title:      "With worktree",
		RepoPath:   "github.com/org/repo",
		BranchName: "feat/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WorktreePath != "/tmp/worktree/TASK-00001" {
		t.Errorf("expected worktree path, got %s", result.WorktreePath)
	}
	if mockWt.lastConfig.TaskID != "TASK-00001" {
		t.Errorf("worktree should have received task ID TASK-00001, got %s", mockWt.lastConfig.TaskID)
	}
	if mockWt.lastConfig.BranchName != "feat/test" {
		t.Errorf("worktree should have received branch feat/test, got %s", mockWt.lastConfig.BranchName)
	}

	// Verify worktree path is stored in status.yaml.
	statusData, err := os.ReadFile(filepath.Join(result.TicketPath, "status.yaml"))
	if err != nil {
		t.Fatalf("status.yaml should exist: %v", err)
	}
	var task models.Task
	if err := yaml.Unmarshal(statusData, &task); err != nil {
		t.Fatalf("failed to parse status.yaml: %v", err)
	}
	if task.WorktreePath != "/tmp/worktree/TASK-00001" {
		t.Errorf("status.yaml worktree should be set, got %s", task.WorktreePath)
	}
}

func TestBootstrap_NilWorktreeManager(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	// Even with repo/branch, nil worktree manager should skip worktree creation.
	result, err := bs.Bootstrap(BootstrapConfig{
		Type:       models.TaskTypeFeat,
		Title:      "No worktree",
		RepoPath:   "github.com/org/repo",
		BranchName: "feat/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WorktreePath != "" {
		t.Errorf("expected empty worktree path, got %s", result.WorktreePath)
	}
}

func TestBootstrap_SequentialIDs(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	r1, err := bs.Bootstrap(BootstrapConfig{Type: models.TaskTypeFeat, Title: "First"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := bs.Bootstrap(BootstrapConfig{Type: models.TaskTypeBug, Title: "Second"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r1.TaskID != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", r1.TaskID)
	}
	if r2.TaskID != "TASK-00002" {
		t.Errorf("expected TASK-00002, got %s", r2.TaskID)
	}
}

func TestApplyTemplate_ViaBootstrap(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	// Create the ticket folder first.
	ticketPath := filepath.Join(dir, "tickets", "TASK-99999")
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("failed to create ticket dir: %v", err)
	}

	if err := bs.ApplyTemplate("TASK-99999", models.TaskTypeSpike); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("notes.md should exist: %v", err)
	}
	if !strings.Contains(string(notes), "Spike Notes") {
		t.Error("spike notes.md should contain Spike Notes")
	}
}

func TestGenerateTaskID_ViaBootstrap(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	id, err := bs.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", id)
	}
}

// mockFailingIDGenerator always returns an error from GenerateTaskID.
type mockFailingIDGenerator struct{}

func (m *mockFailingIDGenerator) GenerateTaskID() (string, error) {
	return "", fmt.Errorf("ID generation failed")
}

func TestBootstrap_IDGenerationError(t *testing.T) {
	dir := t.TempDir()
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, &mockFailingIDGenerator{}, nil, tmplMgr)

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error when ID generation fails")
	}
	if !strings.Contains(err.Error(), "generating task ID") {
		t.Errorf("expected generating task ID error, got: %v", err)
	}
}

func TestBootstrap_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	// Create a file where tickets/ should be a directory.
	if err := os.WriteFile(filepath.Join(dir, "tickets"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error when tickets/ is a file")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("expected creating directory error, got: %v", err)
	}
}

// mockFailingTemplateManager always returns an error from ApplyTemplate.
type mockFailingTemplateManager struct{}

func (m *mockFailingTemplateManager) ApplyTemplate(_ string, _ models.TaskType) error {
	return fmt.Errorf("template apply failed")
}
func (m *mockFailingTemplateManager) GetTemplate(_ models.TaskType) (string, error) {
	return "", nil
}
func (m *mockFailingTemplateManager) RegisterTemplate(_ models.TaskType, _ string) error {
	return nil
}

func TestBootstrap_TemplateApplyError(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	bs := NewBootstrapSystem(dir, idGen, nil, &mockFailingTemplateManager{})

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error when template apply fails")
	}
	if !strings.Contains(err.Error(), "applying template") {
		t.Errorf("expected applying template error, got: %v", err)
	}
}

func TestBootstrap_ContextWriteError(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	// To trigger the context.md write error, we need to make the directory
	// write succeed but make the context.md file write fail.
	// We can do this by making context.md a directory.
	// First, generate a task ID to predict the next one.
	nextID := "TASK-00001"
	ticketPath := filepath.Join(dir, "tickets", nextID)
	if err := os.MkdirAll(filepath.Join(ticketPath, "communications"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create context.md as a directory so WriteFile will fail.
	if err := os.MkdirAll(filepath.Join(ticketPath, "context.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error when context.md cannot be written")
	}
	if !strings.Contains(err.Error(), "writing context.md") {
		t.Errorf("expected writing context.md error, got: %v", err)
	}
}

func TestBootstrap_WorktreeCreationError(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	mockWt := &mockWorktreeCreator{err: fmt.Errorf("worktree creation failed")}
	bs := NewBootstrapSystem(dir, idGen, mockWt, tmplMgr)

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:       models.TaskTypeFeat,
		Title:      "Test",
		RepoPath:   "github.com/org/repo",
		BranchName: "feat/test",
	})
	if err == nil {
		t.Fatal("expected error when worktree creation fails")
	}
	if !strings.Contains(err.Error(), "creating worktree") {
		t.Errorf("expected creating worktree error, got: %v", err)
	}
}

func TestBootstrap_StatusYAMLWriteError(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	// Pre-create the ticket directory structure so bootstrap proceeds normally
	// until status.yaml write. Make status.yaml a directory so it fails.
	nextID := "TASK-00001"
	ticketPath := filepath.Join(dir, "tickets", nextID)
	if err := os.MkdirAll(filepath.Join(ticketPath, "communications"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create status.yaml as a directory.
	if err := os.MkdirAll(filepath.Join(ticketPath, "status.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := bs.Bootstrap(BootstrapConfig{
		Type:  models.TaskTypeFeat,
		Title: "Test",
	})
	if err == nil {
		t.Fatal("expected error when status.yaml cannot be written")
	}
	if !strings.Contains(err.Error(), "writing status.yaml") {
		t.Errorf("expected writing status.yaml error, got: %v", err)
	}
}

func TestBootstrap_StoresTagsAndSource(t *testing.T) {
	dir := t.TempDir()
	idGen := NewTaskIDGenerator(dir, "TASK", 5)
	tmplMgr := NewTemplateManager(dir)
	bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

	result, err := bs.Bootstrap(BootstrapConfig{
		Type:   models.TaskTypeFeat,
		Title:  "Tagged task",
		Tags:   []string{"backend", "api"},
		Source: "slack",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read status.yaml and verify tags and source.
	statusData, err := os.ReadFile(filepath.Join(result.TicketPath, "status.yaml"))
	if err != nil {
		t.Fatalf("status.yaml should exist: %v", err)
	}
	var task models.Task
	if err := yaml.Unmarshal(statusData, &task); err != nil {
		t.Fatalf("failed to parse status.yaml: %v", err)
	}
	if len(task.Tags) != 2 || task.Tags[0] != "backend" || task.Tags[1] != "api" {
		t.Errorf("expected tags [backend api], got %v", task.Tags)
	}
	if task.Source != "slack" {
		t.Errorf("expected source 'slack', got %q", task.Source)
	}
}

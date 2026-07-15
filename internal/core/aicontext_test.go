package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

func TestAIContextGenerator_Generate(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Setup test environment
	setupTestEnvironment(t, tmpDir)

	// Create backlog manager
	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	// Add some test tasks
	task1 := models.NewTask("TASK-001", "Test Task 1", models.TaskTypeFeat)
	task1.Status = models.TaskStatusInProgress
	task1.Priority = models.PriorityP0
	task1.Owner = "test-owner"

	task2 := models.NewTask("TASK-002", "Test Task 2", models.TaskTypeBug)
	task2.Status = models.TaskStatusReview
	task2.Priority = models.PriorityP1

	task3 := models.NewTask("TASK-003", "Test Task 3", models.TaskTypeRefactor)
	task3.Status = models.TaskStatusDone

	if err := backlogMgr.AddTask(*task1); err != nil {
		t.Fatalf("Failed to add task1: %v", err)
	}
	if err := backlogMgr.AddTask(*task2); err != nil {
		t.Fatalf("Failed to add task2: %v", err)
	}
	if err := backlogMgr.AddTask(*task3); err != nil {
		t.Fatalf("Failed to add task3: %v", err)
	}

	// Create generator
	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	// Generate context
	if err := generator.Generate(); err != nil {
		t.Fatalf("Failed to generate context: %v", err)
	}

	// Verify CLAUDE.md was created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)

	// Verify expected sections exist
	expectedSections := []string{
		"# AI Dev Brain - Claude Context",
		"## What's Changed",
		"## Directory Structure",
		"## Conventions",
		"## Glossary",
		"## Architectural Decisions",
		"## Active Tasks",
		"## Critical Decisions",
		"## Recent Sessions",
		"## Captured Sessions",
		"## Stakeholders & Contacts",
	}

	for _, section := range expectedSections {
		if !strings.Contains(contentStr, section) {
			t.Errorf("Expected section %q not found in CLAUDE.md", section)
		}
	}

	// Verify active tasks are included
	if !strings.Contains(contentStr, "TASK-001") {
		t.Error("Active task TASK-001 not found in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "TASK-002") {
		t.Error("Active task TASK-002 not found in CLAUDE.md")
	}
	// Done task should not be in active tasks
	if strings.Contains(contentStr, "TASK-003") {
		t.Error("Done task TASK-003 should not be in active tasks section")
	}

	// Verify context_state.yaml was created (under .adb/ as of #186/#189).
	statePath := filepath.Join(tmpDir, ".adb", "context_state.yaml")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal(".adb/context_state.yaml was not created")
	}

	// Read and verify state
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read .context_state.yaml: %v", err)
	}

	var state ContextState
	if err := yaml.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("Failed to unmarshal context state: %v", err)
	}

	if state.LastGenerated.IsZero() {
		t.Error("LastGenerated timestamp not set")
	}

	if len(state.SectionHashes) == 0 {
		t.Error("SectionHashes is empty")
	}

	// Verify expected sections have hashes
	expectedHashSections := []string{
		"overview", "directory", "conventions", "glossary",
		"decisions", "active_tasks", "critical_decisions",
		"recent_sessions", "captured_sessions", "stakeholders",
	}

	for _, section := range expectedHashSections {
		if _, exists := state.SectionHashes[section]; !exists {
			t.Errorf("Expected hash for section %q not found", section)
		}
	}
}

func TestAIContextGenerator_GenerateWithChanges(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnvironment(t, tmpDir)

	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	task1 := models.NewTask("TASK-001", "Test Task 1", models.TaskTypeFeat)
	task1.Status = models.TaskStatusInProgress
	if err := backlogMgr.AddTask(*task1); err != nil {
		t.Fatalf("Failed to add task1: %v", err)
	}

	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	// First generation
	if err := generator.Generate(); err != nil {
		t.Fatalf("First generation failed: %v", err)
	}

	// Wait a bit to ensure timestamp difference
	time.Sleep(100 * time.Millisecond)

	// Add another task
	task2 := models.NewTask("TASK-002", "Test Task 2", models.TaskTypeBug)
	task2.Status = models.TaskStatusReview
	if err := backlogMgr.AddTask(*task2); err != nil {
		t.Fatalf("Failed to add task2: %v", err)
	}

	// Second generation
	if err := generator.Generate(); err != nil {
		t.Fatalf("Second generation failed: %v", err)
	}

	// Read CLAUDE.md and verify "What's Changed" section shows changes
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Active Tasks**: Updated") {
		t.Error("What's Changed section should show Active Tasks as updated")
	}
}

func TestAIContextGenerator_HandlesMinimalFilesGracefully(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal setup - only backlog
	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	// Should not fail even with missing files
	if err := generator.Generate(); err != nil {
		t.Fatalf("Generate should handle missing files gracefully: %v", err)
	}

	// Verify CLAUDE.md was still created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md should be created even with minimal files")
	}

	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)

	// Verify graceful messages for missing sections
	if !strings.Contains(contentStr, "_No convention documents found._") {
		t.Error("Should indicate no conventions found")
	}
	if !strings.Contains(contentStr, "_No glossary found._") {
		t.Error("Should indicate no glossary found")
	}
	if !strings.Contains(contentStr, "_No active tasks._") {
		t.Error("Should indicate no active tasks")
	}
}

func TestAIContextGenerator_WithDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnvironment(t, tmpDir)

	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	// Add active task
	task1 := models.NewTask("TASK-001", "Test Task with Decisions", models.TaskTypeFeat)
	task1.Status = models.TaskStatusInProgress
	if err := backlogMgr.AddTask(*task1); err != nil {
		t.Fatalf("Failed to add task1: %v", err)
	}

	// Create decisions for the task
	decisionsDir := filepath.Join(tmpDir, "tickets", "TASK-001", "knowledge")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatalf("Failed to create decisions directory: %v", err)
	}

	decisions := struct {
		Decisions []models.Decision `yaml:"decisions"`
	}{
		Decisions: []models.Decision{
			{
				ID:          "DEC-001",
				Title:       "Use PostgreSQL",
				Description: "We decided to use PostgreSQL for data storage",
				Status:      "accepted",
				Rationale:   "Better support for complex queries",
				DecidedAt:   time.Now().UTC(),
			},
		},
	}

	decisionsData, err := yaml.Marshal(decisions)
	if err != nil {
		t.Fatalf("Failed to marshal decisions: %v", err)
	}

	decisionsPath := filepath.Join(decisionsDir, "decisions.yaml")
	if err := os.WriteFile(decisionsPath, decisionsData, 0o644); err != nil {
		t.Fatalf("Failed to write decisions file: %v", err)
	}

	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	if err := generator.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify decision appears in CLAUDE.md
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Use PostgreSQL") {
		t.Error("Decision title not found in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "PostgreSQL for data storage") {
		t.Error("Decision description not found in CLAUDE.md")
	}
}

func TestAIContextGenerator_WithSessions(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnvironment(t, tmpDir)

	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	// Create session files
	sessionsDir := filepath.Join(tmpDir, "tickets", "TASK-001", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("Failed to create sessions directory: %v", err)
	}

	sessionContent := `# Session 2024-03-13

## Summary
This is a test session.

## Actions Taken
1. Created test files
2. Ran tests
3. Fixed bugs

## Outcomes
All tests passing.
`

	sessionPath := filepath.Join(sessionsDir, "session-001.md")
	if err := os.WriteFile(sessionPath, []byte(sessionContent), 0o644); err != nil {
		t.Fatalf("Failed to write session file: %v", err)
	}

	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	if err := generator.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify session appears in CLAUDE.md
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "tickets/TASK-001/sessions/session-001.md") {
		t.Error("Session file path not found in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "This is a test session") {
		t.Error("Session content not found in CLAUDE.md")
	}
}

func TestAIContextGenerator_WithArchitecturalDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnvironment(t, tmpDir)

	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	// Create ADR document
	decisionsDir := filepath.Join(tmpDir, "docs", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatalf("Failed to create decisions directory: %v", err)
	}

	adrContent := `# ADR-001: Use Microservices Architecture

Status: accepted

## Context
We need to scale our application to handle more traffic.

## Decision
We will adopt a microservices architecture.

## Consequences
- Better scalability
- More complex deployment
`

	adrPath := filepath.Join(decisionsDir, "001-microservices.md")
	if err := os.WriteFile(adrPath, []byte(adrContent), 0o644); err != nil {
		t.Fatalf("Failed to write ADR file: %v", err)
	}

	generator := NewAIContextGenerator(tmpDir, backlogMgr)

	if err := generator.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify ADR appears in CLAUDE.md
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("Failed to read CLAUDE.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "001-microservices.md") {
		t.Error("ADR filename not found in CLAUDE.md")
	}
}

func TestAIContextGenerator_SectionHashing(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnvironment(t, tmpDir)

	backlogPath := filepath.Join(tmpDir, "backlog.yaml")
	backlogMgr := storage.NewFileBacklogManager(backlogPath)

	gen := &DefaultAIContextGenerator{
		repoRoot:       tmpDir,
		backlogManager: backlogMgr,
	}

	// Test hash generation
	content1 := "test content"
	hash1 := gen.hashContent(content1)

	if hash1 == "" {
		t.Error("Hash should not be empty")
	}

	// Same content should produce same hash
	hash1b := gen.hashContent(content1)
	if hash1 != hash1b {
		t.Error("Same content should produce same hash")
	}

	// Different content should produce different hash
	content2 := "different content"
	hash2 := gen.hashContent(content2)
	if hash1 == hash2 {
		t.Error("Different content should produce different hash")
	}
}

func TestAIContextGenerator_StateManagement(t *testing.T) {
	tmpDir := t.TempDir()

	gen := &DefaultAIContextGenerator{
		repoRoot: tmpDir,
	}

	// Test saving state
	state := &ContextState{
		LastGenerated: time.Now().UTC(),
		SectionHashes: map[string]string{
			"overview":  "hash1",
			"directory": "hash2",
		},
	}

	if err := gen.saveContextState(state); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Test loading state
	loadedState, err := gen.loadContextState()
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if loadedState.SectionHashes["overview"] != "hash1" {
		t.Error("Loaded state does not match saved state")
	}
	if loadedState.SectionHashes["directory"] != "hash2" {
		t.Error("Loaded state does not match saved state")
	}
}

// setupTestEnvironment creates a basic test environment
func setupTestEnvironment(t *testing.T, tmpDir string) {
	// Create basic directory structure
	dirs := []string{
		"docs/wiki",
		"docs/decisions",
		"tickets",
		"sessions",
		"templates",
	}

	for _, dir := range dirs {
		path := filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create basic docs
	glossaryContent := `# Glossary

- **ADR**: Architectural Decision Record
- **Task**: A unit of work in the backlog
`
	glossaryPath := filepath.Join(tmpDir, "docs", "glossary.md")
	if err := os.WriteFile(glossaryPath, []byte(glossaryContent), 0o644); err != nil {
		t.Fatalf("Failed to write glossary: %v", err)
	}

	stakeholdersContent := `# Stakeholders

- **Product Owner**: Alice Smith
- **Tech Lead**: Bob Johnson
`
	stakeholdersPath := filepath.Join(tmpDir, "docs", "stakeholders.md")
	if err := os.WriteFile(stakeholdersPath, []byte(stakeholdersContent), 0o644); err != nil {
		t.Fatalf("Failed to write stakeholders: %v", err)
	}
}

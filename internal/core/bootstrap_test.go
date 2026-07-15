package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

func TestBootstrapSystem(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create template manager
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	// Create bootstrap config
	config := BootstrapConfig{
		TaskID:      "TASK-00001",
		Title:       "Test Task",
		Description: "This is a test task for bootstrapping",
		AcceptanceCriteria: []string{
			"Criterion 1",
			"Criterion 2",
			"Criterion 3",
		},
		Dependencies: []string{
			"TASK-00000",
		},
		RelatedTasks: "Related to TASK-00002",
		Status:       "pending",
		TicketsDir:   filepath.Join(tempDir, "tickets"),
		WorktreeDir:  tempDir,
	}

	// Run bootstrap
	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() failed: %v", err)
	}

	// Verify task directory was created
	if result.TaskDir == "" {
		t.Error("TaskDir should not be empty")
	}
	if _, err := os.Stat(result.TaskDir); os.IsNotExist(err) {
		t.Errorf("Task directory was not created: %s", result.TaskDir)
	}

	// Verify sessions directory was created
	if result.SessionsDir == "" {
		t.Error("SessionsDir should not be empty")
	}
	if _, err := os.Stat(result.SessionsDir); os.IsNotExist(err) {
		t.Errorf("Sessions directory was not created: %s", result.SessionsDir)
	}

	// Verify knowledge directory was created
	if result.KnowledgeDir == "" {
		t.Error("KnowledgeDir should not be empty")
	}
	if _, err := os.Stat(result.KnowledgeDir); os.IsNotExist(err) {
		t.Errorf("Knowledge directory was not created: %s", result.KnowledgeDir)
	}

	// Verify status.yaml was created
	if result.StatusFile == "" {
		t.Error("StatusFile should not be empty")
	}
	if _, err := os.Stat(result.StatusFile); os.IsNotExist(err) {
		t.Errorf("Status file was not created: %s", result.StatusFile)
	}

	// Verify context.md was created
	if result.ContextFile == "" {
		t.Error("ContextFile should not be empty")
	}
	if _, err := os.Stat(result.ContextFile); os.IsNotExist(err) {
		t.Errorf("Context file was not created: %s", result.ContextFile)
	}

	// Verify notes.md was created
	if result.NotesFile == "" {
		t.Error("NotesFile should not be empty")
	}
	if _, err := os.Stat(result.NotesFile); os.IsNotExist(err) {
		t.Errorf("Notes file was not created: %s", result.NotesFile)
	}

	// Verify design.md was created
	if result.DesignFile == "" {
		t.Error("DesignFile should not be empty")
	}
	if _, err := os.Stat(result.DesignFile); os.IsNotExist(err) {
		t.Errorf("Design file was not created: %s", result.DesignFile)
	}

	// Verify decisions.yaml was created
	if result.DecisionsFile == "" {
		t.Error("DecisionsFile should not be empty")
	}
	if _, err := os.Stat(result.DecisionsFile); os.IsNotExist(err) {
		t.Errorf("Decisions file was not created: %s", result.DecisionsFile)
	}

	// Verify task-context.md was created in .claude/rules/
	if result.TaskContextFile == "" {
		t.Error("TaskContextFile should not be empty")
	}
	if _, err := os.Stat(result.TaskContextFile); os.IsNotExist(err) {
		t.Errorf("Task context file was not created: %s", result.TaskContextFile)
	}

	// Verify expected path structure
	expectedTaskDir := filepath.Join(tempDir, "tickets", "TASK-00001")
	if result.TaskDir != expectedTaskDir {
		t.Errorf("Expected TaskDir to be %s, got %s", expectedTaskDir, result.TaskDir)
	}

	expectedTaskContextFile := filepath.Join(tempDir, ".claude", "rules", "task-context.md")
	if result.TaskContextFile != expectedTaskContextFile {
		t.Errorf("Expected TaskContextFile to be %s, got %s", expectedTaskContextFile, result.TaskContextFile)
	}
}

func TestBootstrapSystemFileContents(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create template manager
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	// Create bootstrap config
	config := BootstrapConfig{
		TaskID:      "TASK-99999",
		Title:       "Content Test Task",
		Description: "Testing file contents",
		AcceptanceCriteria: []string{
			"Must have content A",
			"Must have content B",
		},
		Dependencies: []string{
			"TASK-99998",
		},
		RelatedTasks: "Related to TASK-99997",
		Status:       "in_progress",
		TicketsDir:   filepath.Join(tempDir, "tickets"),
		WorktreeDir:  tempDir,
		TicketPath:   "tickets/github.com/acme/widget/TASK-99999-content-test-task",
	}

	// Run bootstrap
	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() failed: %v", err)
	}

	// Test status.yaml content
	statusContent, err := os.ReadFile(result.StatusFile)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}
	statusStr := string(statusContent)
	expectedInStatus := []string{
		"task_id: TASK-99999",
		"title: Content Test Task",
		"status: in_progress",
	}
	for _, expected := range expectedInStatus {
		if !strings.Contains(statusStr, expected) {
			t.Errorf("Expected status.yaml to contain %q, but it didn't.\nContent:\n%s", expected, statusStr)
		}
	}

	// Test context.md content
	contextContent, err := os.ReadFile(result.ContextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}
	contextStr := string(contextContent)
	expectedInContext := []string{
		"# Context: Content Test Task",
		"**Task ID:** TASK-99999",
		"Testing file contents",
		"- [ ] Must have content A",
		"- [ ] Must have content B",
		"- TASK-99998",
		"Related to TASK-99997",
	}
	for _, expected := range expectedInContext {
		if !strings.Contains(contextStr, expected) {
			t.Errorf("Expected context.md to contain %q, but it didn't.\nContent:\n%s", expected, contextStr)
		}
	}

	// Test notes.md content
	notesContent, err := os.ReadFile(result.NotesFile)
	if err != nil {
		t.Fatalf("Failed to read notes file: %v", err)
	}
	notesStr := string(notesContent)
	expectedInNotes := []string{
		"# Notes: Content Test Task",
		"**Task ID:** TASK-99999",
		"Testing file contents",
		"- [ ] Must have content A",
		"- [ ] Must have content B",
	}
	for _, expected := range expectedInNotes {
		if !strings.Contains(notesStr, expected) {
			t.Errorf("Expected notes.md to contain %q, but it didn't.\nContent:\n%s", expected, notesStr)
		}
	}

	// Test design.md content
	designContent, err := os.ReadFile(result.DesignFile)
	if err != nil {
		t.Fatalf("Failed to read design file: %v", err)
	}
	designStr := string(designContent)
	expectedInDesign := []string{
		"# Design Document: Content Test Task",
		"**Task ID:** TASK-99999",
	}
	for _, expected := range expectedInDesign {
		if !strings.Contains(designStr, expected) {
			t.Errorf("Expected design.md to contain %q, but it didn't.\nContent:\n%s", expected, designStr)
		}
	}

	// Test decisions.yaml content
	decisionsContent, err := os.ReadFile(result.DecisionsFile)
	if err != nil {
		t.Fatalf("Failed to read decisions file: %v", err)
	}
	decisionsStr := string(decisionsContent)
	expectedInDecisions := []string{
		"# Task Decisions",
		"decisions: []",
	}
	for _, expected := range expectedInDecisions {
		if !strings.Contains(decisionsStr, expected) {
			t.Errorf("Expected decisions.yaml to contain %q, but it didn't.\nContent:\n%s", expected, decisionsStr)
		}
	}

	// Test task-context.md content
	taskContextContent, err := os.ReadFile(result.TaskContextFile)
	if err != nil {
		t.Fatalf("Failed to read task-context file: %v", err)
	}
	taskContextStr := string(taskContextContent)
	expectedInTaskContext := []string{
		"# Task Context (Tier 0)",
		"You are working on **TASK-99999: Content Test Task**",
		"- [ ] Must have content A",
		"- [ ] Must have content B",
		"**Status:** in_progress",
		"tickets/github.com/acme/widget/TASK-99999-content-test-task",
	}
	for _, expected := range expectedInTaskContext {
		if !strings.Contains(taskContextStr, expected) {
			t.Errorf("Expected task-context.md to contain %q, but it didn't.\nContent:\n%s", expected, taskContextStr)
		}
	}
	// The stale flat path must not reappear.
	if strings.Contains(taskContextStr, "tickets/TASK-99999/") {
		t.Errorf("task-context.md still contains the stale flat path.\nContent:\n%s", taskContextStr)
	}
}

func TestBootstrapSystemMissingTaskID(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "", // Missing TaskID
		Title:       "Test Task",
		Description: "Test",
		TicketsDir:  filepath.Join(tempDir, "tickets"),
		WorktreeDir: tempDir,
	}

	_, err = BootstrapSystem(config, tm)
	if err == nil {
		t.Error("Expected error when TaskID is missing, got nil")
	}
	if !strings.Contains(err.Error(), "TaskID is required") {
		t.Errorf("Expected error to mention TaskID, got: %v", err)
	}
}

func TestBootstrapSystemMissingTitle(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "TASK-00001",
		Title:       "", // Missing Title
		Description: "Test",
		TicketsDir:  filepath.Join(tempDir, "tickets"),
		WorktreeDir: tempDir,
	}

	_, err = BootstrapSystem(config, tm)
	if err == nil {
		t.Error("Expected error when Title is missing, got nil")
	}
	if !strings.Contains(err.Error(), "Title is required") {
		t.Errorf("Expected error to mention Title, got: %v", err)
	}
}

func TestBootstrapSystemDefaultValues(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	// Config with minimal values (should use defaults)
	config := BootstrapConfig{
		TaskID:      "TASK-00002",
		Title:       "Minimal Config Task",
		Description: "Test defaults",
		// TicketsDir and WorktreeDir not set - should default
		// Status not set - should default to "pending"
	}

	// Change to temp directory for this test
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() with defaults failed: %v", err)
	}

	// Verify status defaults to "pending"
	statusContent, err := os.ReadFile(result.StatusFile)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}
	statusStr := string(statusContent)
	if !strings.Contains(statusStr, "status: pending") {
		t.Errorf("Expected default status to be 'pending', content:\n%s", statusStr)
	}

	// Verify directories were created relative to current directory
	if _, err := os.Stat(result.TaskDir); os.IsNotExist(err) {
		t.Errorf("Task directory was not created with default TicketsDir")
	}
}

func TestBootstrapSystemEmptyAcceptanceCriteria(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:             "TASK-00003",
		Title:              "Empty Criteria Task",
		Description:        "Test empty criteria",
		AcceptanceCriteria: []string{}, // Empty
		TicketsDir:         filepath.Join(tempDir, "tickets"),
		WorktreeDir:        tempDir,
	}

	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() failed: %v", err)
	}

	// Verify notes.md shows default criteria
	notesContent, err := os.ReadFile(result.NotesFile)
	if err != nil {
		t.Fatalf("Failed to read notes file: %v", err)
	}
	notesStr := string(notesContent)
	if !strings.Contains(notesStr, "- [ ] Define acceptance criteria") {
		t.Errorf("Expected default acceptance criteria when list is empty.\nContent:\n%s", notesStr)
	}

	// Verify context.md shows default criteria
	contextContent, err := os.ReadFile(result.ContextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}
	contextStr := string(contextContent)
	if !strings.Contains(contextStr, "- [ ] Define acceptance criteria") {
		t.Errorf("Expected default acceptance criteria in context.md when list is empty.\nContent:\n%s", contextStr)
	}
}

func TestBootstrapSystemEmptyDependencies(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:       "TASK-00004",
		Title:        "Empty Dependencies Task",
		Description:  "Test empty dependencies",
		Dependencies: []string{}, // Empty
		TicketsDir:   filepath.Join(tempDir, "tickets"),
		WorktreeDir:  tempDir,
	}

	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() failed: %v", err)
	}

	// Verify context.md shows default dependencies
	contextContent, err := os.ReadFile(result.ContextFile)
	if err != nil {
		t.Fatalf("Failed to read context file: %v", err)
	}
	contextStr := string(contextContent)
	if !strings.Contains(contextStr, "- No dependencies") {
		t.Errorf("Expected default dependencies message when list is empty.\nContent:\n%s", contextStr)
	}
}

func TestBootstrapSystemDirectoryStructure(t *testing.T) {
	tempDir := t.TempDir()

	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "TASK-12345",
		Title:       "Directory Structure Test",
		Description: "Testing directory structure",
		TicketsDir:  filepath.Join(tempDir, "tickets"),
		WorktreeDir: tempDir,
	}

	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() failed: %v", err)
	}

	// Verify exact directory structure
	expectedDirs := []string{
		filepath.Join(tempDir, "tickets", "TASK-12345"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "sessions"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "knowledge"),
		filepath.Join(tempDir, ".claude"),
		filepath.Join(tempDir, ".claude", "rules"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("Expected directory does not exist: %s", dir)
			continue
		}
		if err != nil {
			t.Errorf("Error checking directory %s: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}

	// Verify exact file structure
	expectedFiles := []string{
		filepath.Join(tempDir, "tickets", "TASK-12345", "status.yaml"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "context.md"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "notes.md"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "design.md"),
		filepath.Join(tempDir, "tickets", "TASK-12345", "knowledge", "decisions.yaml"),
		filepath.Join(tempDir, ".claude", "rules", "task-context.md"),
	}

	for _, file := range expectedFiles {
		info, err := os.Stat(file)
		if os.IsNotExist(err) {
			t.Errorf("Expected file does not exist: %s", file)
			continue
		}
		if err != nil {
			t.Errorf("Error checking file %s: %v", file, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("Expected %s to be a file, not a directory", file)
		}
	}

	// Verify result paths match expected
	if result.TaskDir != filepath.Join(tempDir, "tickets", "TASK-12345") {
		t.Errorf("Unexpected TaskDir: %s", result.TaskDir)
	}
	if result.SessionsDir != filepath.Join(tempDir, "tickets", "TASK-12345", "sessions") {
		t.Errorf("Unexpected SessionsDir: %s", result.SessionsDir)
	}
	if result.KnowledgeDir != filepath.Join(tempDir, "tickets", "TASK-12345", "knowledge") {
		t.Errorf("Unexpected KnowledgeDir: %s", result.KnowledgeDir)
	}
}

func TestBootstrapSystem_EmptyWorktreeDir(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	// Bootstrap with empty WorktreeDir — should skip task-context.md
	config := BootstrapConfig{
		TaskID:      "TASK-SKIP",
		Title:       "Skip Context Test",
		Description: "Test that empty WorktreeDir skips task-context.md",
		TicketsDir:  filepath.Join(tempDir, "tickets"),
		WorktreeDir: "", // Empty — no task-context.md
	}

	result, err := BootstrapSystem(config, tm)
	if err != nil {
		t.Fatalf("BootstrapSystem() error = %v", err)
	}

	// TaskContextFile should be empty
	if result.TaskContextFile != "" {
		t.Errorf("TaskContextFile should be empty when WorktreeDir is empty, got %q", result.TaskContextFile)
	}

	// But ticket files should still exist
	if _, err := os.Stat(result.StatusFile); os.IsNotExist(err) {
		t.Error("status.yaml should exist")
	}
	if _, err := os.Stat(result.ContextFile); os.IsNotExist(err) {
		t.Error("context.md should exist")
	}
}

func TestGenerateTaskContext(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "TASK-00099",
		Title:       "Context Gen Test",
		Description: "Testing generateTaskContext",
		Status:      "in_progress",
	}

	// Generate task-context.md in temp dir (simulating a worktree)
	err = generateTaskContext(tempDir, tm, config)
	if err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}

	// Verify file exists
	taskContextPath := filepath.Join(tempDir, ".claude", "rules", "task-context.md")
	content, err := os.ReadFile(taskContextPath)
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}

	// Verify content contains task ID
	if !strings.Contains(string(content), "TASK-00099") {
		t.Error("task-context.md should contain task ID")
	}
}

// TestGenerateTaskContext_WorktreeRendersRichFields guards against the bug
// where generateTaskContext (the real per-worktree code path invoked from
// TaskManager.Create) built a THIN template-data map missing CreatedAt,
// AcceptanceCriteria, and the correlation-layout ticket path — so the rendered
// .claude/rules/task-context.md showed the literal "<no value>", fell through
// to the {{else}} acceptance-criteria placeholder, and hardcoded the stale
// flat "tickets/TASK-.../" path.
func TestGenerateTaskContext_WorktreeRendersRichFields(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "TASK-00085",
		Title:       "Worktree Rich Fields",
		Description: "Testing that the worktree render is complete",
		AcceptanceCriteria: []string{
			"First acceptance criterion",
			"Second acceptance criterion",
		},
		Status:       "in_progress",
		TicketPath:   "tickets/github.com/valter-silva-au/ai-dev-brain/TASK-00085-worktree-rich-fields",
		Branch:       "fix/worktree-rich-fields",
		WorktreePath: "work/github.com/valter-silva-au/ai-dev-brain/TASK-00085-worktree-rich-fields",
	}

	if err := generateTaskContext(tempDir, tm, config); err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}

	taskContextPath := filepath.Join(tempDir, ".claude", "rules", "task-context.md")
	content, err := os.ReadFile(taskContextPath)
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}
	rendered := string(content)

	// (a) no literal Go "<no value>" from a missing template key
	if strings.Contains(rendered, "<no value>") {
		t.Errorf("task-context.md contains literal \"<no value>\" (missing template data).\nContent:\n%s", rendered)
	}
	// (b) no stale flat "tickets/TASK-" path — the correlation layout uses TicketPath
	if strings.Contains(rendered, "tickets/TASK-") {
		t.Errorf("task-context.md contains the stale flat path \"tickets/TASK-\".\nContent:\n%s", rendered)
	}
	// (c) the real acceptance-criteria lines appear (not the {{else}} fallback)
	for _, want := range []string{"First acceptance criterion", "Second acceptance criterion"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("task-context.md missing acceptance criterion %q.\nContent:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Define acceptance criteria") {
		t.Errorf("task-context.md fell through to the {{else}} acceptance-criteria placeholder.\nContent:\n%s", rendered)
	}
	// (d) a real Created value (not empty / not "<no value>")
	if !strings.Contains(rendered, "**Created:**") {
		t.Errorf("task-context.md missing Created line.\nContent:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if idx := strings.Index(line, "**Created:**"); idx >= 0 {
			// The Created value may be followed by " · **Updated:** ..." on the
			// same line; take just the token immediately after the marker.
			rest := strings.TrimSpace(line[idx+len("**Created:**"):])
			val := rest
			if sep := strings.Index(rest, " ·"); sep >= 0 {
				val = strings.TrimSpace(rest[:sep])
			}
			if val == "" || val == "<no value>" {
				t.Errorf("task-context.md Created value is empty/invalid: %q", line)
			}
		}
	}
	// The correlation-layout ticket path should be present.
	if !strings.Contains(rendered, config.TicketPath) {
		t.Errorf("task-context.md missing TicketPath %q.\nContent:\n%s", config.TicketPath, rendered)
	}
}

func TestGenerateTaskContext_InvalidPath(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID: "TASK-BAD",
		Title:  "Bad Path Test",
	}

	// Use a path that's guaranteed-unusable on every supported OS.
	// `\x00` is illegal in filenames on POSIX and Windows both, so
	// os.MkdirAll / os.WriteFile uniformly fail — avoids the previous
	// fixture's reliance on `/nonexistent/...` which Windows
	// interprets relative to the current drive and might succeed.
	invalidPath := filepath.Join(t.TempDir(), "invalid\x00path")
	err = generateTaskContext(invalidPath, tm, config)
	if err == nil {
		t.Error("generateTaskContext() with invalid path should fail")
	}
}

// tier0Fixture is a realistic, fully-populated task used by the Tier-0 tests.
func tier0Fixture() BootstrapConfig {
	return BootstrapConfig{
		TaskID:      "TASK-00086",
		Title:       "adb task-context Tier-0 template",
		Description: "Rebuild task-context.md as the Tier-0 core per ADR part C.",
		AcceptanceCriteria: []string{
			"Template contains all four Tier-0 blocks in order",
			"Standing DOs/DONTs are inlined from the embedded rules file",
			"Rendered Tier-0 file is <= ~2000 tokens for a real task",
		},
		Status:          "in_progress",
		Phase:           "implementation",
		ProgressPct:     60,
		SteerDirectives: "Ship T2 before T3; keep it under the token budget.",
		TicketPath:      "tickets/github.com/valter-silva-au/ai-dev-brain/TASK-00086-adb-task-context-tier0-template",
		Branch:          "refactor/adb-task-context-tier0-template",
		WorktreePath:    "work/github.com/valter-silva-au/ai-dev-brain/TASK-00086-adb-task-context-tier0-template",
		RepoSubPath:     "github.com/valter-silva-au/ai-dev-brain",
		Siblings: []string{
			"TASK-00085 fix/adb-worktree-task-context-empty-fields (merged)",
			"TASK-00087 feat/adb-wire-ai-context-generator (queued)",
		},
	}
}

// TestTaskContext_Tier0AllBlocks asserts the new Tier-0 template renders all
// four non-negotiable blocks (a-d), inlines the standing DOs/DONTs, points to
// deeper tiers rather than inlining them, and preserves the T1 guarantees
// (no "<no value>", no stale flat "tickets/TASK-" path, correlation TicketPath
// present, real acceptance criteria not the {{else}} placeholder).
func TestTaskContext_Tier0AllBlocks(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := tier0Fixture()
	if err := generateTaskContext(tempDir, tm, config); err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, ".claude", "rules", "task-context.md"))
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}
	rendered := string(content)

	// T1 guarantees still hold.
	if strings.Contains(rendered, "<no value>") {
		t.Errorf("task-context.md contains literal \"<no value>\".\nContent:\n%s", rendered)
	}
	if strings.Contains(rendered, "tickets/TASK-") {
		t.Errorf("task-context.md contains the stale flat path \"tickets/TASK-\".\nContent:\n%s", rendered)
	}
	if !strings.Contains(rendered, config.TicketPath) {
		t.Errorf("task-context.md missing correlation TicketPath %q.", config.TicketPath)
	}
	if strings.Contains(rendered, "Not yet defined") {
		t.Errorf("task-context.md fell through to the {{else}} acceptance-criteria placeholder.\nContent:\n%s", rendered)
	}

	// (a) Identity & State — every field present.
	for _, want := range []string{
		"## (a) Identity & State",
		"TASK-00086",
		config.TicketPath,
		config.WorktreePath,
		config.Branch,
		"**Status:** in_progress",
		"**Phase:** implementation",
		"**Progress:** 60%",
		"Acceptance criteria:",
		"Template contains all four Tier-0 blocks in order",
		config.SteerDirectives,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("block (a) missing %q.\nContent:\n%s", want, rendered)
		}
	}

	// (b) Loop + DoD + inlined standing DOs/DONTs.
	for _, want := range []string{
		"## (b) The Loop",
		"research -> develop -> verify -> repeat",
		"Definition of Done",
		"Reply as the project owner", // from standing-dos-donts.md
		"Branch discipline",          // from standing-dos-donts.md
		"Production safety",          // from standing-dos-donts.md
		"Verify the model",           // from standing-dos-donts.md
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("block (b) missing %q.\nContent:\n%s", want, rendered)
		}
	}

	// (c) Related tickets — pointers only, plus the wiki index + repo dir.
	for _, want := range []string{
		"## (c) Related tickets",
		"TASK-00085 fix/adb-worktree-task-context-empty-fields (merged)",
		"tickets/github.com/valter-silva-au/ai-dev-brain/",
		"wiki/decisions/",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("block (c) missing %q.\nContent:\n%s", want, rendered)
		}
	}

	// (d) Other live sessions — section renders even when empty.
	if !strings.Contains(rendered, "## (d) Other live sessions") {
		t.Errorf("block (d) section missing.\nContent:\n%s", rendered)
	}
	if !strings.Contains(rendered, "No other active sessions reported") {
		t.Errorf("block (d) empty-state placeholder missing.\nContent:\n%s", rendered)
	}

	// Go-deeper pointers (Tier 1/2) — pointers, not inlined content.
	for _, want := range []string{
		"Go deeper (Tier 1/2",
		config.TicketPath + "/context.md",
		"$ADB_HOME/CLAUDE.md",
		"_active-digest",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("Go-deeper pointer missing %q.\nContent:\n%s", want, rendered)
		}
	}

	// Footer comment with state hash + generated timestamp.
	if !strings.Contains(rendered, "<!-- adb:tier0 state=") {
		t.Errorf("Tier-0 footer comment missing.\nContent:\n%s", rendered)
	}
}

// TestTaskContext_Tier0EmptyOptionalFields verifies the template degrades
// gracefully when the optional fields (Phase/Progress/steer/Siblings/
// LiveSessions) are all zero-valued — no awkward empty lines, both placeholders
// present, and still no "<no value>".
func TestTaskContext_Tier0EmptyOptionalFields(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := BootstrapConfig{
		TaskID:      "TASK-00090",
		Title:       "Minimal Tier-0",
		Description: "No optional fields set.",
		AcceptanceCriteria: []string{
			"Renders without optional fields",
		},
		Status:     "pending",
		TicketPath: "tickets/_local/TASK-00090-minimal-tier0",
	}
	if err := generateTaskContext(tempDir, tm, config); err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".claude", "rules", "task-context.md"))
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}
	rendered := string(content)

	if strings.Contains(rendered, "<no value>") {
		t.Errorf("minimal render contains \"<no value>\".\nContent:\n%s", rendered)
	}
	// Optional-field markers must NOT appear when unset (Stage guards the
	// initiative line, so no ticket without an initiative shows a Stage).
	for _, absent := range []string{"**Phase:**", "**Progress:**", "**Stage:**"} {
		if strings.Contains(rendered, absent) {
			t.Errorf("minimal render should not contain %q (field unset).\nContent:\n%s", absent, rendered)
		}
	}
	// Graceful placeholders present.
	for _, want := range []string{
		"re-read `tickets/_local/TASK-00090-minimal-tier0/steer.md`",
		"No siblings resolved yet",
		"No other active sessions reported",
		"<platform>/<org>/<repo>", // RepoSubPath unset -> generic pointer
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("minimal render missing placeholder %q.\nContent:\n%s", want, rendered)
		}
	}
	// StateHash empty when no siblings/digest -> footer shows "state= ".
	if !strings.Contains(rendered, "<!-- adb:tier0 state= generated=") {
		t.Errorf("expected empty state hash in footer for minimal render.\nContent:\n%s", rendered)
	}
}

// TestTaskContext_StageSurfaced verifies issue #88: when a ticket is associated
// with an initiative, the worktree Tier-0 context surfaces the initiative's
// founder-playbook Stage (and name) in the Identity block, so a Claude session
// always knows which stage it is operating in.
func TestTaskContext_StageSurfaced(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}

	config := tier0Fixture()
	config.Initiative = "Widget Launcher"
	config.Stage = "MVP"
	if err := generateTaskContext(tempDir, tm, config); err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".claude", "rules", "task-context.md"))
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}
	rendered := string(content)

	for _, want := range []string{"**Stage:** MVP", "Widget Launcher", "Idea→MVP→Launch→Scale"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("stage-surfaced render missing %q.\nContent:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "<no value>") {
		t.Errorf("stage-surfaced render contains \"<no value>\".\nContent:\n%s", rendered)
	}
}

// TestTaskContext_Tier0TokenBudget enforces the ADR's Tier-0 token budget:
// the rendered file for a realistic, fully-populated task must stay within
// ~2000 tokens (~8000 characters). We measure characters as a conservative
// proxy (English prose averages < 4 chars/token, so char/4 over-estimates
// tokens safely).
func TestTaskContext_Tier0TokenBudget(t *testing.T) {
	tempDir := t.TempDir()
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("Failed to create template manager: %v", err)
	}
	if err := generateTaskContext(tempDir, tm, tier0Fixture()); err != nil {
		t.Fatalf("generateTaskContext() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(tempDir, ".claude", "rules", "task-context.md"))
	if err != nil {
		t.Fatalf("Failed to read task-context.md: %v", err)
	}

	const maxChars = 8000 // ~2000 tokens
	if len(content) > maxChars {
		t.Errorf("Tier-0 render is %d chars (> %d budget ~= 2000 tokens); tighten prose / lean on pointers.\nContent:\n%s",
			len(content), maxChars, string(content))
	}
	approxTokens := (len(content) + 3) / 4
	t.Logf("Tier-0 render: %d chars, ~%d tokens (budget ~2000)", len(content), approxTokens)
}

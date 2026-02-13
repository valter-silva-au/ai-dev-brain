package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func setupAIContextTest(t *testing.T) (AIContextGenerator, string) {
	t.Helper()
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)
	return gen, dir
}

func TestGenerateContextFile_Claude(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	path, err := gen.GenerateContextFile(AITypeClaude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filepath.Base(path) != "CLAUDE.md" {
		t.Fatalf("expected CLAUDE.md, got %q", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	content := string(data)
	requiredSections := []string{
		"## Project Overview",
		"## Directory Structure",
		"## Key Conventions",
		"## Glossary",
		"## Active Decisions Summary",
		"## Active Tasks",
		"## Key Contacts",
	}
	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Fatalf("CLAUDE.md missing section: %s", section)
		}
	}

	_ = dir
}

func TestGenerateContextFile_Kiro(t *testing.T) {
	gen, _ := setupAIContextTest(t)

	path, err := gen.GenerateContextFile(AITypeKiro)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filepath.Base(path) != "kiro.md" {
		t.Fatalf("expected kiro.md, got %q", filepath.Base(path))
	}
}

func TestSyncContext(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	if err := gen.SyncContext(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both files should exist.
	for _, name := range []string{"CLAUDE.md", "kiro.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s not created: %v", name, err)
		}
	}
}

func TestAssembleProjectOverview(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	overview, err := gen.AssembleProjectOverview()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overview, "AI Dev Brain") {
		t.Fatal("overview should mention AI Dev Brain")
	}
}

func TestAssembleDirectoryStructure(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	structure, err := gen.AssembleDirectoryStructure()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(structure, "docs/") {
		t.Fatal("structure should mention docs/")
	}
	if !strings.Contains(structure, "tickets/") {
		t.Fatal("structure should mention tickets/")
	}
}

func TestAssembleConventions_Default(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	conventions, err := gen.AssembleConventions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(conventions, "Branch naming") {
		t.Fatal("conventions should include branch naming")
	}
}

func TestAssembleGlossary_Default(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	glossary, err := gen.AssembleGlossary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(glossary, "Task") {
		t.Fatal("glossary should include Task definition")
	}
}

func TestAssembleGlossary_FromFile(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0o755)
	_ = os.WriteFile(filepath.Join(docsDir, "glossary.md"), []byte("# Glossary\n\n- **Widget**: A UI component"), 0o644)

	glossary, err := gen.AssembleGlossary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(glossary, "Widget") {
		t.Fatal("glossary should include Widget from file")
	}
}

func TestAssembleActiveTaskSummaries(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	// Add tasks to backlog.
	backlogMgr := storage.NewBacklogManager(dir)
	_ = backlogMgr.AddTask(storage.BacklogEntry{
		ID:       "TASK-00001",
		Title:    "Implement OAuth",
		Status:   models.StatusInProgress,
		Priority: models.P1,
		Branch:   "feat/oauth",
	})
	_ = backlogMgr.AddTask(storage.BacklogEntry{
		ID:       "TASK-00002",
		Title:    "Archived task",
		Status:   models.StatusArchived,
		Priority: models.P2,
	})
	_ = backlogMgr.Save()

	summary, err := gen.AssembleActiveTaskSummaries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "TASK-00001") {
		t.Fatal("summary should include active task TASK-00001")
	}
	if strings.Contains(summary, "TASK-00002") {
		t.Fatal("summary should not include archived task TASK-00002")
	}
}

func TestAssembleActiveTaskSummaries_Empty(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	summary, err := gen.AssembleActiveTaskSummaries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "No active tasks") {
		t.Fatal("should indicate no active tasks")
	}
}

func TestAssembleDecisionsSummary(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	decisionsDir := filepath.Join(dir, "docs", "decisions")
	_ = os.MkdirAll(decisionsDir, 0o755)
	adr := `# ADR-0001: Use Auth0

**Status:** Accepted
**Date:** 2026-02-05
**Source:** TASK-00001

## Context
Need OAuth
`
	_ = os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-use-auth0.md"), []byte(adr), 0o644)

	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "ADR-0001") {
		t.Fatal("summary should include ADR-0001")
	}
	if !strings.Contains(summary, "TASK-00001") {
		t.Fatal("summary should include source task")
	}
}

func TestAssembleDecisionsSummary_NoDecisions(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "No decisions") {
		t.Fatal("should indicate no decisions")
	}
}

func TestRegenerateSection(t *testing.T) {
	gen, _ := setupAIContextTest(t)

	sections := []ContextSection{
		SectionOverview, SectionStructure, SectionConventions,
		SectionGlossary, SectionDecisions, SectionActiveTasks,
		SectionContacts,
	}
	for _, section := range sections {
		if err := gen.RegenerateSection(section); err != nil {
			t.Fatalf("regenerating section %s: %v", section, err)
		}
	}
}

func TestRegenerateSection_Unknown(t *testing.T) {
	gen, _ := setupAIContextTest(t)
	err := gen.RegenerateSection("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown section")
	}
}

func TestSyncContext_ReflectsChanges(t *testing.T) {
	gen, dir := setupAIContextTest(t)

	// Initial sync.
	_ = gen.SyncContext()

	// Add a task and re-sync.
	backlogMgr := storage.NewBacklogManager(dir)
	_ = backlogMgr.AddTask(storage.BacklogEntry{
		ID:     "TASK-00099",
		Title:  "New task added",
		Status: models.StatusInProgress,
		Branch: "feat/new",
	})
	_ = backlogMgr.Save()

	_ = gen.SyncContext()

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)
	if !strings.Contains(content, "TASK-00099") {
		t.Fatal("sync should reflect newly added task")
	}
}

// --- Additional tests for full coverage ---

func TestFilenameForAI_Default(t *testing.T) {
	gen := NewAIContextGenerator(t.TempDir(), storage.NewBacklogManager(t.TempDir()), nil).(*aiContextGenerator)

	// Test the default case (unknown AI type).
	result := gen.filenameForAI(AIType("unknown"))
	if result != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md for unknown AI type, got %q", result)
	}

	// Verify known types.
	if gen.filenameForAI(AITypeClaude) != "CLAUDE.md" {
		t.Error("expected CLAUDE.md for claude type")
	}
	if gen.filenameForAI(AITypeKiro) != "kiro.md" {
		t.Error("expected kiro.md for kiro type")
	}
}

func TestGenerateContextFile_WriteError(t *testing.T) {
	// Create a directory where CLAUDE.md should be to cause WriteFile to fail.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "CLAUDE.md"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.GenerateContextFile(AITypeClaude)
	if err == nil {
		t.Fatal("expected error when write fails")
	}
	if !strings.Contains(err.Error(), "generating context file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncContext_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory where CLAUDE.md should be.
	_ = os.MkdirAll(filepath.Join(dir, "CLAUDE.md"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	err := gen.SyncContext()
	if err == nil {
		t.Fatal("expected error when write fails")
	}
	if !strings.Contains(err.Error(), "syncing context") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAssembleConventions_FromWikiFiles(t *testing.T) {
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "docs", "wiki")
	_ = os.MkdirAll(wikiDir, 0o755)
	_ = os.WriteFile(filepath.Join(wikiDir, "coding-conventions.md"), []byte("Custom conventions"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	conventions, err := gen.AssembleConventions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(conventions, "Custom conventions") {
		t.Error("should include custom conventions from wiki file")
	}
	// Should NOT contain default conventions when custom ones exist.
	if strings.Contains(conventions, "Branch naming") {
		t.Error("should not include defaults when custom conventions exist")
	}
}

func TestAssembleConventions_NonMatchingWikiFiles(t *testing.T) {
	dir := t.TempDir()
	wikiDir := filepath.Join(dir, "docs", "wiki")
	_ = os.MkdirAll(wikiDir, 0o755)
	// File without "convention" in name should be ignored.
	_ = os.WriteFile(filepath.Join(wikiDir, "other-topic.md"), []byte("Other content"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	conventions, err := gen.AssembleConventions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to defaults.
	if !strings.Contains(conventions, "Branch naming") {
		t.Error("should include default conventions when no convention files found")
	}
}

func TestAssembleGlossary_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Create glossary.md as a directory to cause a non-IsNotExist read error.
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(filepath.Join(docsDir, "glossary.md"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.AssembleGlossary()
	if err == nil {
		t.Fatal("expected error for non-IsNotExist read error")
	}
	if !strings.Contains(err.Error(), "reading glossary") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAssembleDecisionsSummary_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	// Create docs/decisions as a file instead of directory.
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0o755)
	_ = os.WriteFile(filepath.Join(docsDir, "decisions"), []byte("not a dir"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.AssembleDecisionsSummary()
	if err == nil {
		t.Fatal("expected error when decisions is not a directory")
	}
	if !strings.Contains(err.Error(), "assembling decisions") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAssembleDecisionsSummary_NonAcceptedADR(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	_ = os.MkdirAll(decisionsDir, 0o755)

	// ADR without "**Status:** Accepted" should be skipped.
	draftADR := "# ADR-0001: Draft Decision\n\n**Status:** Draft\n\n## Decision\nSomething"
	_ = os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-draft.md"), []byte(draftADR), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "No decisions") {
		t.Error("should report no decisions when none are accepted")
	}
}

func TestAssembleDecisionsSummary_AcceptedWithoutSource(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	_ = os.MkdirAll(decisionsDir, 0o755)

	// ADR that is accepted but has no Source field.
	adr := "# ADR-0001: No Source\n\n**Status:** Accepted\n\n## Decision\nSomething"
	_ = os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-no-source.md"), []byte(adr), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "ADR-0001") {
		t.Error("should include ADR without source")
	}
	// Should not contain parentheses since there's no source.
	if strings.Contains(summary, "(") {
		t.Error("should not have source parentheses when source is empty")
	}
}

func TestAssembleDecisionsSummary_WithSubdirsAndNonMd(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	_ = os.MkdirAll(decisionsDir, 0o755)

	// Create a subdirectory (should be skipped).
	_ = os.MkdirAll(filepath.Join(decisionsDir, "subdir"), 0o755)
	// Create a non-md file (should be skipped).
	_ = os.WriteFile(filepath.Join(decisionsDir, "README.txt"), []byte("text"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "No decisions") {
		t.Error("should report no decisions")
	}
}

func TestAssembleStakeholders_WithFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "docs", "stakeholders.md"), []byte("# Stakeholders"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil).(*aiContextGenerator)

	result := gen.assembleStakeholders()
	if !strings.Contains(result, "stakeholders.md") {
		t.Error("should reference stakeholders.md")
	}
}

func TestAssembleStakeholders_NoFile(t *testing.T) {
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil).(*aiContextGenerator)

	result := gen.assembleStakeholders()
	if !strings.Contains(result, "No stakeholders file found") {
		t.Error("should indicate no stakeholders file")
	}
}

func TestAssembleContacts_WithFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "docs", "contacts.md"), []byte("# Contacts"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil).(*aiContextGenerator)

	result := gen.assembleContacts()
	if !strings.Contains(result, "contacts.md") {
		t.Error("should reference contacts.md")
	}
}

func TestAssembleContacts_NoFile(t *testing.T) {
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil).(*aiContextGenerator)

	result := gen.assembleContacts()
	if !strings.Contains(result, "No contacts file found") {
		t.Error("should indicate no contacts file")
	}
}

func TestAssembleActiveTaskSummaries_LoadError(t *testing.T) {
	// Make backlog.yaml a directory so Load fails with a read error.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "backlog.yaml"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.AssembleActiveTaskSummaries()
	if err == nil {
		t.Fatal("expected error from corrupted backlog")
	}
	if !strings.Contains(err.Error(), "assembling active tasks") {
		t.Errorf("unexpected error: %v", err)
	}
}

// failingBacklogManager is a backlog manager that returns errors on FilterTasks.
type failingBacklogManager struct {
	filterErr error
}

func (f *failingBacklogManager) Load() error                              { return nil }
func (f *failingBacklogManager) Save() error                              { return nil }
func (f *failingBacklogManager) AddTask(entry storage.BacklogEntry) error { return nil }
func (f *failingBacklogManager) UpdateTask(taskID string, updates storage.BacklogEntry) error {
	return nil
}
func (f *failingBacklogManager) GetTask(taskID string) (*storage.BacklogEntry, error) {
	return nil, nil
}
func (f *failingBacklogManager) GetAllTasks() ([]storage.BacklogEntry, error) { return nil, nil }
func (f *failingBacklogManager) DeleteTask(taskID string) error               { return nil }
func (f *failingBacklogManager) RemoveTask(taskID string) error               { return nil }
func (f *failingBacklogManager) FilterTasks(filter storage.BacklogFilter) ([]storage.BacklogEntry, error) {
	if f.filterErr != nil {
		return nil, f.filterErr
	}
	return nil, nil
}

func TestAssembleActiveTaskSummaries_FilterTasksError(t *testing.T) {
	dir := t.TempDir()
	fbm := &failingBacklogManager{
		filterErr: fmt.Errorf("filter failure"),
	}
	gen := NewAIContextGenerator(dir, fbm, nil)

	_, err := gen.AssembleActiveTaskSummaries()
	if err == nil {
		t.Fatal("expected error from FilterTasks failure")
	}
	if !strings.Contains(err.Error(), "filter failure") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAssembleConventions_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	// Create docs/wiki as a file instead of directory to cause ReadDir to fail.
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0o755)
	_ = os.WriteFile(filepath.Join(docsDir, "wiki"), []byte("not a dir"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	conventions, err := gen.AssembleConventions()
	if err != nil {
		t.Fatalf("unexpected error (should fall back to defaults): %v", err)
	}
	// Should use defaults when wiki ReadDir fails.
	if !strings.Contains(conventions, "Branch naming") {
		t.Error("should fall back to default conventions")
	}
}

func TestGenerateContextFile_AssembleAllError(t *testing.T) {
	// Trigger assembleAll error via broken backlog (ActiveTaskSummaries load fails).
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "backlog.yaml"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.GenerateContextFile(AITypeClaude)
	if err == nil {
		t.Fatal("expected error from assembleAll failure")
	}
	if !strings.Contains(err.Error(), "generating context file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncContext_AssembleAllError(t *testing.T) {
	// Trigger assembleAll error via broken backlog.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "backlog.yaml"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	err := gen.SyncContext()
	if err == nil {
		t.Fatal("expected error from assembleAll failure")
	}
	if !strings.Contains(err.Error(), "syncing context") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAssembleAll_GlossaryError(t *testing.T) {
	// Trigger assembleAll error via broken glossary (non-IsNotExist error).
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(docsDir, "glossary.md"), 0o755)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.GenerateContextFile(AITypeClaude)
	if err == nil {
		t.Fatal("expected error from glossary error")
	}
}

func TestAssembleAll_DecisionsError(t *testing.T) {
	// Trigger assembleAll error via broken decisions (not a directory).
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	_ = os.MkdirAll(docsDir, 0o755)
	_ = os.WriteFile(filepath.Join(docsDir, "decisions"), []byte("not a dir"), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	_, err := gen.GenerateContextFile(AITypeClaude)
	if err == nil {
		t.Fatal("expected error from decisions error")
	}
}

func TestAssembleDecisionsSummary_UnreadableADR(t *testing.T) {
	dir := t.TempDir()
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	_ = os.MkdirAll(decisionsDir, 0o755)

	// Create a .md entry that is a directory (ReadFile will fail, should be skipped).
	_ = os.MkdirAll(filepath.Join(decisionsDir, "broken.md"), 0o755)
	// Create a valid accepted ADR.
	adr := "# ADR-0001: Test\n\n**Status:** Accepted\n**Source:** TASK-00001\n"
	_ = os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-test.md"), []byte(adr), 0o644)

	backlogMgr := storage.NewBacklogManager(dir)
	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	summary, err := gen.AssembleDecisionsSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still find the valid ADR.
	if !strings.Contains(summary, "ADR-0001") {
		t.Error("should include valid ADR despite broken .md")
	}
}

func TestRenderContextFile_IncludesKnowledgeSummary(t *testing.T) {
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)

	// Create a knowledge manager with test data.
	store := newInMemoryKnowledgeStore()
	store.topics["auth"] = models.Topic{
		Name: "auth", Description: "Authentication decisions", EntryCount: 2, Tasks: []string{"TASK-00001"},
	}
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-15", KnowledgeID: "K-00001", Event: "decision: Use JWT", Task: "TASK-00001"},
	}
	km := NewKnowledgeManager(store)

	gen := NewAIContextGenerator(dir, backlogMgr, km)

	path, err := gen.GenerateContextFile(AITypeClaude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "## Accumulated Knowledge") {
		t.Error("expected '## Accumulated Knowledge' section in generated context file")
	}
	if !strings.Contains(content, "auth") {
		t.Error("expected 'auth' topic in knowledge summary")
	}
	if !strings.Contains(content, "Use JWT") {
		t.Error("expected 'Use JWT' timeline entry in knowledge summary")
	}
}

func TestRenderContextFile_NilKnowledgeManager(t *testing.T) {
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)

	gen := NewAIContextGenerator(dir, backlogMgr, nil)

	path, err := gen.GenerateContextFile(AITypeClaude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)

	// Should NOT include the knowledge section when manager is nil.
	if strings.Contains(content, "## Accumulated Knowledge") {
		t.Error("should not include knowledge section when knowledge manager is nil")
	}
}

func TestRenderContextFile_EmptyKnowledgeStore(t *testing.T) {
	dir := t.TempDir()
	backlogMgr := storage.NewBacklogManager(dir)

	// Empty knowledge store.
	store := newInMemoryKnowledgeStore()
	km := NewKnowledgeManager(store)

	gen := NewAIContextGenerator(dir, backlogMgr, km)

	path, err := gen.GenerateContextFile(AITypeClaude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)

	// Should NOT include the knowledge section when store is empty.
	if strings.Contains(content, "## Accumulated Knowledge") {
		t.Error("should not include knowledge section when store is empty")
	}
}

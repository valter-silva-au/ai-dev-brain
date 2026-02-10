package core

import (
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
	gen := NewAIContextGenerator(dir, backlogMgr)
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
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "glossary.md"), []byte("# Glossary\n\n- **Widget**: A UI component"), 0o644)

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
	backlogMgr.AddTask(storage.BacklogEntry{
		ID:       "TASK-00001",
		Title:    "Implement OAuth",
		Status:   models.StatusInProgress,
		Priority: models.P1,
		Branch:   "feat/oauth",
	})
	backlogMgr.AddTask(storage.BacklogEntry{
		ID:       "TASK-00002",
		Title:    "Archived task",
		Status:   models.StatusArchived,
		Priority: models.P2,
	})
	backlogMgr.Save()

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
	os.MkdirAll(decisionsDir, 0o755)
	adr := `# ADR-0001: Use Auth0

**Status:** Accepted
**Date:** 2026-02-05
**Source:** TASK-00001

## Context
Need OAuth
`
	os.WriteFile(filepath.Join(decisionsDir, "ADR-0001-use-auth0.md"), []byte(adr), 0o644)

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
	gen.SyncContext()

	// Add a task and re-sync.
	backlogMgr := storage.NewBacklogManager(dir)
	backlogMgr.AddTask(storage.BacklogEntry{
		ID:     "TASK-00099",
		Title:  "New task added",
		Status: models.StatusInProgress,
		Branch: "feat/new",
	})
	backlogMgr.Save()

	gen.SyncContext()

	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)
	if !strings.Contains(content, "TASK-00099") {
		t.Fatal("sync should reflect newly added task")
	}
}

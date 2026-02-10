package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestContextManager(t *testing.T) (*fileContextManager, string) {
	t.Helper()
	dir := t.TempDir()
	mgr := NewContextManager(dir).(*fileContextManager)
	return mgr, dir
}

func TestInitializeContext(t *testing.T) {
	mgr, dir := newTestContextManager(t)

	ctx, err := mgr.InitializeContext("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.TaskID != "TASK-00001" {
		t.Fatalf("expected task ID TASK-00001, got %q", ctx.TaskID)
	}

	// Verify directory structure.
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if _, err := os.Stat(ticketDir); err != nil {
		t.Fatalf("ticket dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ticketDir, "context.md")); err != nil {
		t.Fatalf("context.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ticketDir, "notes.md")); err != nil {
		t.Fatalf("notes.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ticketDir, "communications")); err != nil {
		t.Fatalf("communications/ not created: %v", err)
	}

	// Verify content contains the task ID.
	if !strings.Contains(ctx.Context, "TASK-00001") {
		t.Fatal("context.md does not contain task ID")
	}
	if !strings.Contains(ctx.Notes, "TASK-00001") {
		t.Fatal("notes.md does not contain task ID")
	}
}

func TestLoadContext(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	// Clear in-memory cache to force file read.
	mgr.contexts = make(map[string]*TaskContext)

	ctx, err := mgr.LoadContext("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.TaskID != "TASK-00001" {
		t.Fatalf("expected task ID TASK-00001, got %q", ctx.TaskID)
	}
	if !strings.Contains(ctx.Context, "## Summary") {
		t.Fatal("loaded context missing Summary section")
	}
}

func TestLoadContext_NotFound(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, err := mgr.LoadContext("TASK-99999")
	if err == nil {
		t.Fatal("expected error for missing task context")
	}
}

func TestUpdateContext(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	err := mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"notes":   "Updated notes content",
		"context": "Updated context content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := mgr.contexts["TASK-00001"]
	if ctx.Notes != "Updated notes content" {
		t.Fatalf("notes not updated: %q", ctx.Notes)
	}
	if ctx.Context != "Updated context content" {
		t.Fatalf("context not updated: %q", ctx.Context)
	}
}

func TestUpdateContext_LoadsFromDisk(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	// Clear cache.
	mgr.contexts = make(map[string]*TaskContext)

	err := mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"notes": "New notes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.contexts["TASK-00001"].Notes != "New notes" {
		t.Fatal("notes not updated after loading from disk")
	}
}

func TestPersistContext(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"notes":   "Persisted notes",
		"context": "Persisted context",
	})

	if err := mgr.PersistContext("TASK-00001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Clear cache and reload.
	mgr.contexts = make(map[string]*TaskContext)
	ctx, err := mgr.LoadContext("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Notes != "Persisted notes" {
		t.Fatalf("notes not persisted: %q", ctx.Notes)
	}
	if ctx.Context != "Persisted context" {
		t.Fatalf("context not persisted: %q", ctx.Context)
	}
}

func TestPersistContext_NotLoaded(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	err := mgr.PersistContext("TASK-99999")
	if err == nil {
		t.Fatal("expected error for unloaded context")
	}
}

func TestGetContextForAI(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	customContext := `# Task Context: TASK-00001

## Summary
Working on OAuth implementation

## Current Focus
Token validation

## Recent Progress
- Implemented login flow
- Added token generation

## Open Questions
- [ ] Should we support refresh tokens?
- [ ] What is the token TTL?

## Decisions Made
- Using JWT for tokens
- Auth0 as provider

## Blockers
- Waiting for API key from security team

## Next Steps
- [ ] Implement token refresh
`
	mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"context": customContext,
	})

	ai, err := mgr.GetContextForAI("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ai.Summary != "Working on OAuth implementation" {
		t.Fatalf("unexpected summary: %q", ai.Summary)
	}
	if len(ai.RecentActivity) != 2 {
		t.Fatalf("expected 2 recent activities, got %d", len(ai.RecentActivity))
	}
	if len(ai.OpenQuestions) != 2 {
		t.Fatalf("expected 2 open questions, got %d", len(ai.OpenQuestions))
	}
	if len(ai.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(ai.Decisions))
	}
	if len(ai.Blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(ai.Blockers))
	}
}

func TestGetContextForAI_EmptyContext(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	mgr.InitializeContext("TASK-00001")

	ai, err := mgr.GetContextForAI("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Template has placeholder text in list items, so they should be parsed.
	if ai.Summary == "" {
		t.Fatal("expected non-empty summary from template")
	}
}

func TestParseCommunicationFile(t *testing.T) {
	content := `# 2026-02-05-slack-john-new-requirement.md

**Date:** 2026-02-05
**Source:** Slack
**Contact:** John Smith (@john)
**Topic:** New OAuth requirement

## Content

OAuth must support PKCE flow

## Tags
- requirement
- action_item
`

	comm := parseCommunicationFile(content)
	if comm.Source != "Slack" {
		t.Fatalf("expected source Slack, got %q", comm.Source)
	}
	if comm.Contact != "John Smith (@john)" {
		t.Fatalf("expected contact 'John Smith (@john)', got %q", comm.Contact)
	}
	if comm.Topic != "New OAuth requirement" {
		t.Fatalf("expected topic 'New OAuth requirement', got %q", comm.Topic)
	}
	if !strings.Contains(comm.Content, "PKCE flow") {
		t.Fatalf("content should contain PKCE flow: %q", comm.Content)
	}
	if len(comm.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(comm.Tags))
	}
}

func TestExtractSection(t *testing.T) {
	content := `## Summary
This is the summary

## Next Section
Something else`

	result := extractSection(content, "## Summary")
	if result != "This is the summary" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExtractSection_NotFound(t *testing.T) {
	result := extractSection("no sections here", "## Missing")
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestExtractListSection(t *testing.T) {
	content := `## Items
- First item
- Second item
- [ ] Third item

## Next`

	items := extractListSection(content, "## Items")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(items), items)
	}
	if items[0] != "First item" {
		t.Fatalf("unexpected first item: %q", items[0])
	}
	if items[2] != "Third item" {
		t.Fatalf("unexpected third item (checkbox should be stripped): %q", items[2])
	}
}

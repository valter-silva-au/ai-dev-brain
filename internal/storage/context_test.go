package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
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
	_, _ = mgr.InitializeContext("TASK-00001")

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
	_, _ = mgr.InitializeContext("TASK-00001")

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
	_, _ = mgr.InitializeContext("TASK-00001")

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
	_, _ = mgr.InitializeContext("TASK-00001")

	_ = mgr.UpdateContext("TASK-00001", map[string]interface{}{
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
	_, _ = mgr.InitializeContext("TASK-00001")

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
	_ = mgr.UpdateContext("TASK-00001", map[string]interface{}{
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
	_, _ = mgr.InitializeContext("TASK-00001")

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

func TestInitializeContext_TicketDirCreateError(t *testing.T) {
	dir := t.TempDir()
	// Place a file where the ticket dir would be created.
	ticketsDir := filepath.Join(dir, "tickets")
	if err := os.WriteFile(ticketsDir, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewContextManager(dir).(*fileContextManager)
	_, err := mgr.InitializeContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when ticket dir creation fails")
	}
	if !strings.Contains(err.Error(), "creating ticket dir") {
		t.Fatalf("expected 'creating ticket dir' in error, got %q", err.Error())
	}
}

func TestInitializeContext_CommsDirCreateError(t *testing.T) {
	dir := t.TempDir()
	mgr := NewContextManager(dir).(*fileContextManager)
	// Create ticket dir but put a file where communications/ should be.
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "communications"), []byte("blocker"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err := mgr.InitializeContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when communications dir creation fails")
	}
	if !strings.Contains(err.Error(), "creating communications dir") {
		t.Fatalf("expected 'creating communications dir' in error, got %q", err.Error())
	}
}

func TestInitializeContext_ContextWriteError(t *testing.T) {
	dir := t.TempDir()
	mgr := NewContextManager(dir).(*fileContextManager)
	// Create ticket dir and comms dir, then make context.md a directory to block write.
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(filepath.Join(ticketDir, "communications"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ticketDir, "context.md"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err := mgr.InitializeContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when context.md write fails")
	}
	if !strings.Contains(err.Error(), "writing context.md") {
		t.Fatalf("expected 'writing context.md' in error, got %q", err.Error())
	}
}

func TestInitializeContext_NotesWriteError(t *testing.T) {
	dir := t.TempDir()
	mgr := NewContextManager(dir).(*fileContextManager)
	// Create ticket dir and comms dir, then make notes.md a directory to block write.
	ticketDir := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(filepath.Join(ticketDir, "communications"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ticketDir, "notes.md"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err := mgr.InitializeContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when notes.md write fails")
	}
	if !strings.Contains(err.Error(), "writing notes.md") {
		t.Fatalf("expected 'writing notes.md' in error, got %q", err.Error())
	}
}

func TestLoadContext_MissingNotes(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")
	mgr.contexts = make(map[string]*TaskContext)

	// Remove notes.md to trigger the error.
	notesPath := mgr.notesPath("TASK-00001")
	if err := os.Remove(notesPath); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err := mgr.LoadContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error for missing notes.md")
	}
	if !strings.Contains(err.Error(), "reading notes.md") {
		t.Fatalf("expected 'reading notes.md' in error, got %q", err.Error())
	}
}

func TestLoadContext_CommunicationsLoadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ReadDir behavior differs on Windows")
	}
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")
	mgr.contexts = make(map[string]*TaskContext)

	// Make the communications directory unreadable by replacing it with a file.
	commsDir := mgr.commsDir("TASK-00001")
	if err := os.RemoveAll(commsDir); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	// Create a file that is not a directory but not absent either,
	// so ReadDir returns an error that is not IsNotExist.
	if err := os.WriteFile(commsDir, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err := mgr.LoadContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when communications dir is not readable")
	}
	if !strings.Contains(err.Error(), "loading communications") {
		t.Fatalf("expected 'loading communications' in error, got %q", err.Error())
	}
}

func TestLoadCommunications_EmptyDir(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	comms, err := mgr.loadCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comms) != 0 {
		t.Fatalf("expected 0 communications, got %d", len(comms))
	}
}

func TestLoadCommunications_NoDir(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	// Don't initialize - no directory exists.
	comms, err := mgr.loadCommunications("TASK-99999")
	if err != nil {
		t.Fatalf("unexpected error for non-existent dir: %v", err)
	}
	if comms != nil {
		t.Fatalf("expected nil for non-existent dir, got %v", comms)
	}
}

func TestLoadCommunications_ReadDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ReadDir behavior differs on Windows")
	}
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	// Replace communications dir with a file.
	commsDir := mgr.commsDir("TASK-00001")
	if err := os.RemoveAll(commsDir); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.WriteFile(commsDir, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err := mgr.loadCommunications("TASK-00001")
	if err == nil {
		t.Fatal("expected error when comms dir is a file")
	}
}

func TestLoadCommunications_WithFiles(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	commsDir := mgr.commsDir("TASK-00001")

	// Write a valid communication .md file.
	content := "**Date:** 2026-02-05\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Design\n\n## Content\n\nSome content\n\n## Tags\n- decision\n"
	if err := os.WriteFile(filepath.Join(commsDir, "2026-02-05-slack-alice-design.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Write a subdirectory (should be skipped).
	if err := os.MkdirAll(filepath.Join(commsDir, "subdir"), 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Write a non-.md file (should be skipped).
	if err := os.WriteFile(filepath.Join(commsDir, "readme.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	comms, err := mgr.loadCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comms) != 1 {
		t.Fatalf("expected 1 communication, got %d", len(comms))
	}
	if comms[0].Source != "Slack" {
		t.Fatalf("expected source Slack, got %q", comms[0].Source)
	}
}

func TestParseCommunicationFile_ContentWithoutTags(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Design\n\n## Content\n\nSome content without a tags section\n"

	comm := parseCommunicationFile(content)
	if comm.Content != "Some content without a tags section" {
		t.Fatalf("unexpected content: %q", comm.Content)
	}
	if len(comm.Tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(comm.Tags))
	}
}

func TestParseCommunicationFile_TagsFollowedBySection(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Design\n\n## Content\n\nSome content\n\n## Tags\n- decision\n- requirement\n\n## Another Section\nMore stuff\n"

	comm := parseCommunicationFile(content)
	if len(comm.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(comm.Tags), comm.Tags)
	}
	if comm.Tags[0] != models.CommunicationTag("decision") {
		t.Fatalf("expected first tag 'decision', got %q", comm.Tags[0])
	}
}

func TestParseCommunicationFile_InvalidDate(t *testing.T) {
	content := "**Date:** not-a-date\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Design\n"

	comm := parseCommunicationFile(content)
	if !comm.Date.IsZero() {
		t.Fatalf("expected zero date for invalid format, got %v", comm.Date)
	}
	if comm.Source != "Slack" {
		t.Fatalf("expected source Slack, got %q", comm.Source)
	}
}

func TestUpdateContext_ErrorLoadingFromDisk(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	// Don't initialize - no files on disk. Clear cache.
	mgr.contexts = make(map[string]*TaskContext)

	err := mgr.UpdateContext("TASK-99999", map[string]interface{}{"notes": "test"})
	if err == nil {
		t.Fatal("expected error when loading from disk fails")
	}
	if !strings.Contains(err.Error(), "updating context") {
		t.Fatalf("expected 'updating context' in error, got %q", err.Error())
	}
}

func TestUpdateContext_NonStringValues(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	originalNotes := mgr.contexts["TASK-00001"].Notes
	// Passing non-string values should not update.
	err := mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"notes":   123,
		"context": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.contexts["TASK-00001"].Notes != originalNotes {
		t.Fatal("notes should not change for non-string value")
	}
}

func TestPersistContext_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a file where tickets/TASK-00001 should be a directory.
	if err := os.WriteFile(filepath.Join(dir, "tickets"), []byte("blocker"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	mgr := NewContextManager(dir).(*fileContextManager)
	mgr.contexts["TASK-00001"] = &TaskContext{
		TaskID:  "TASK-00001",
		Notes:   "notes",
		Context: "context",
	}

	err := mgr.PersistContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when directory creation fails")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Fatalf("expected 'creating directory' in error, got %q", err.Error())
	}
}

func TestPersistContext_ContextWriteError(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	// Make context.md a directory to cause write error.
	contextPath := mgr.contextPath("TASK-00001")
	if err := os.Remove(contextPath); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.MkdirAll(contextPath, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	err := mgr.PersistContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when context.md write fails")
	}
	if !strings.Contains(err.Error(), "writing context.md") {
		t.Fatalf("expected 'writing context.md' in error, got %q", err.Error())
	}
}

func TestPersistContext_NotesWriteError(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	// Make notes.md a directory to cause write error.
	notesPath := mgr.notesPath("TASK-00001")
	if err := os.Remove(notesPath); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.MkdirAll(notesPath, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	err := mgr.PersistContext("TASK-00001")
	if err == nil {
		t.Fatal("expected error when notes.md write fails")
	}
	if !strings.Contains(err.Error(), "writing notes.md") {
		t.Fatalf("expected 'writing notes.md' in error, got %q", err.Error())
	}
}

func TestGetContextForAI_LoadsFromDisk(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	customContext := `# Task Context: TASK-00001

## Summary
Loaded from disk

## Recent Progress
- Did something

## Open Questions
- [ ] A question

## Decisions Made
- A decision

## Blockers
- A blocker
`
	_ = mgr.UpdateContext("TASK-00001", map[string]interface{}{
		"context": customContext,
	})
	_ = mgr.PersistContext("TASK-00001")

	// Clear cache to force loading from disk.
	mgr.contexts = make(map[string]*TaskContext)

	ai, err := mgr.GetContextForAI("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ai.Summary != "Loaded from disk" {
		t.Fatalf("expected 'Loaded from disk', got %q", ai.Summary)
	}
}

func TestGetContextForAI_LoadError(t *testing.T) {
	mgr, _ := newTestContextManager(t)
	// No context files on disk, cache empty.
	mgr.contexts = make(map[string]*TaskContext)

	_, err := mgr.GetContextForAI("TASK-99999")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
	if !strings.Contains(err.Error(), "getting AI context") {
		t.Fatalf("expected 'getting AI context' in error, got %q", err.Error())
	}
}

func TestExtractSection_LastSection(t *testing.T) {
	content := `## First
First content

## Last
This is the last section with no following header`

	result := extractSection(content, "## Last")
	if result != "This is the last section with no following header" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExtractListSection_EmptySection(t *testing.T) {
	content := `## Items

## Next`

	items := extractListSection(content, "## Items")
	if items != nil {
		t.Fatalf("expected nil for empty section, got %v", items)
	}
}

func TestExtractListSection_NotFound(t *testing.T) {
	items := extractListSection("no sections", "## Missing")
	if items != nil {
		t.Fatalf("expected nil for missing section, got %v", items)
	}
}

func TestExtractListSection_CheckedItems(t *testing.T) {
	content := `## Items
- [x] Completed item
- [ ] Pending item
- Regular item

## Next`

	items := extractListSection(content, "## Items")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(items), items)
	}
	if items[0] != "Completed item" {
		t.Fatalf("expected 'Completed item', got %q", items[0])
	}
}

func TestExtractListSection_EmptyListItems(t *testing.T) {
	content := `## Items
-
- Valid item
-

## Next`

	items := extractListSection(content, "## Items")
	if len(items) != 1 {
		t.Fatalf("expected 1 valid item (empty items skipped), got %d: %v", len(items), items)
	}
	if items[0] != "Valid item" {
		t.Fatalf("expected 'Valid item', got %q", items[0])
	}
}

func TestLoadCommunications_FileReadError(t *testing.T) {
	// This test verifies that unreadable .md files are skipped (continue on error).
	mgr, _ := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	commsDir := mgr.commsDir("TASK-00001")

	// Create a valid .md file.
	content := "**Date:** 2026-02-05\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Test\n\n## Content\n\nHello\n"
	if err := os.WriteFile(filepath.Join(commsDir, "valid.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Create a broken symlink ending in .md: entry.IsDir() returns false,
	// suffix check passes, but ReadFile fails because the target does not exist.
	brokenLink := filepath.Join(commsDir, "broken.md")
	_ = os.Symlink("/nonexistent/path/file.md", brokenLink)

	comms, err := mgr.loadCommunications("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// broken.md symlink should be skipped on ReadFile error.
	// Only the valid.md should be returned.
	if len(comms) != 1 {
		t.Fatalf("expected 1 communication (broken skipped), got %d", len(comms))
	}
}

func TestParseCommunicationFile_NoContent(t *testing.T) {
	content := "**Date:** 2026-02-05\n**Source:** Email\n**Contact:** Bob\n**Topic:** Meeting\n"

	comm := parseCommunicationFile(content)
	if comm.Source != "Email" {
		t.Fatalf("expected source Email, got %q", comm.Source)
	}
	if comm.Content != "" {
		t.Fatalf("expected empty content, got %q", comm.Content)
	}
	if len(comm.Tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(comm.Tags))
	}
}

// Test helper that creates a fully-populated context on disk with communications.
func setupContextWithComms(t *testing.T) (*fileContextManager, string) {
	t.Helper()
	mgr, dir := newTestContextManager(t)
	_, _ = mgr.InitializeContext("TASK-00001")

	commsDir := mgr.commsDir("TASK-00001")
	content := "**Date:** 2026-02-05\n**Source:** Slack\n**Contact:** Alice\n**Topic:** Design\n\n## Content\n\nDiscussion about design\n\n## Tags\n- decision\n"
	_ = os.WriteFile(filepath.Join(commsDir, "2026-02-05-slack-alice-design.md"), []byte(content), 0o644)

	return mgr, dir
}

func TestLoadContext_WithCommunications(t *testing.T) {
	mgr, _ := setupContextWithComms(t)
	mgr.contexts = make(map[string]*TaskContext)

	ctx, err := mgr.LoadContext("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Communications) != 1 {
		t.Fatalf("expected 1 communication, got %d", len(ctx.Communications))
	}
	if ctx.Communications[0].Source != "Slack" {
		t.Fatalf("expected source Slack, got %q", ctx.Communications[0].Source)
	}
	if ctx.Communications[0].Date != (time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected date 2026-02-05, got %v", ctx.Communications[0].Date)
	}
	if len(ctx.Communications[0].Tags) != 1 || ctx.Communications[0].Tags[0] != models.CommunicationTag("decision") {
		t.Fatalf("expected tag 'decision', got %v", ctx.Communications[0].Tags)
	}
}

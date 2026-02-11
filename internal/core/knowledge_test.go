package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func setupKnowledgeTest(t *testing.T) (KnowledgeExtractor, string) {
	t.Helper()
	dir := t.TempDir()
	ctxMgr := storage.NewContextManager(dir)
	commMgr := storage.NewCommunicationManager(dir)
	ke := NewKnowledgeExtractor(dir, ctxMgr, commMgr)
	return ke, dir
}

func initTaskWithContext(t *testing.T, dir, taskID, contextContent, notesContent string) {
	t.Helper()
	ctxMgr := storage.NewContextManager(dir)
	_, _ = ctxMgr.InitializeContext(taskID)
	_ = ctxMgr.UpdateContext(taskID, map[string]interface{}{
		"context": contextContent,
		"notes":   notesContent,
	})
	_ = ctxMgr.PersistContext(taskID)
}

func TestExtractFromTask(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	contextContent := `# Task Context: TASK-00001

## Summary
Working on OAuth

## Decisions Made
- Use JWT tokens
- Use Auth0 as provider

## Blockers
- Waiting for API key
`
	notesContent := `# Notes: TASK-00001

## Learnings
- JWT validation is tricky
- Token refresh needs careful handling

## Gotchas
- Auth0 rate limits are low in dev
`
	initTaskWithContext(t, dir, "TASK-00001", contextContent, notesContent)

	knowledge, err := ke.ExtractFromTask("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if knowledge.TaskID != "TASK-00001" {
		t.Fatalf("expected task ID TASK-00001, got %q", knowledge.TaskID)
	}
	if len(knowledge.Learnings) != 2 {
		t.Fatalf("expected 2 learnings, got %d: %v", len(knowledge.Learnings), knowledge.Learnings)
	}
	if len(knowledge.Gotchas) != 1 {
		t.Fatalf("expected 1 gotcha, got %d: %v", len(knowledge.Gotchas), knowledge.Gotchas)
	}
	if len(knowledge.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(knowledge.Decisions))
	}
}

func TestExtractFromTask_WithCommunications(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	contextContent := `# Task Context: TASK-00001

## Summary
Working on OAuth
`
	notesContent := `# Notes: TASK-00001
`
	initTaskWithContext(t, dir, "TASK-00001", contextContent, notesContent)

	// Add a communication with decision tag.
	commMgr := storage.NewCommunicationManager(dir)
	comm := models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "John",
		Topic:   "Auth0 provider decision",
		Content: "We decided to use Auth0",
		Tags:    []models.CommunicationTag{models.TagDecision},
	}
	commMgr.AddCommunication("TASK-00001", comm)

	knowledge, err := ke.ExtractFromTask("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(knowledge.Decisions) != 1 {
		t.Fatalf("expected 1 decision from communication, got %d", len(knowledge.Decisions))
	}
	if knowledge.Decisions[0].Title != "Auth0 provider decision" {
		t.Fatalf("expected decision title from communication, got %q", knowledge.Decisions[0].Title)
	}
}

func TestGenerateHandoff(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	contextContent := `# Task Context: TASK-00001

## Summary
Implemented OAuth flow for the auth service

## Recent Progress
- Set up Auth0 integration
- Implemented token validation

## Next Steps
- [ ] Add refresh token support

## Decisions Made
- Use JWT tokens
`
	notesContent := `# Notes: TASK-00001

## Learnings
- JWT validation requires careful error handling
`
	initTaskWithContext(t, dir, "TASK-00001", contextContent, notesContent)

	handoff, err := ke.GenerateHandoff("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if handoff.TaskID != "TASK-00001" {
		t.Fatalf("expected task ID TASK-00001, got %q", handoff.TaskID)
	}
	if !strings.Contains(handoff.Summary, "OAuth flow") {
		t.Fatalf("expected summary to contain OAuth flow, got %q", handoff.Summary)
	}
	if len(handoff.CompletedWork) != 2 {
		t.Fatalf("expected 2 completed items, got %d", len(handoff.CompletedWork))
	}
	if len(handoff.OpenItems) != 1 {
		t.Fatalf("expected 1 open item, got %d", len(handoff.OpenItems))
	}
	if len(handoff.Learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(handoff.Learnings))
	}

	// Verify handoff.md was written.
	handoffPath := filepath.Join(dir, "tickets", "TASK-00001", "handoff.md")
	data, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("handoff.md not created: %v", err)
	}
	if !strings.Contains(string(data), "TASK-00001") {
		t.Fatal("handoff.md does not contain task ID")
	}
	if !strings.Contains(string(data), "## Provenance") {
		t.Fatal("handoff.md does not contain Provenance section")
	}
}

func TestUpdateWiki(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	knowledge := &models.ExtractedKnowledge{
		TaskID: "TASK-00001",
		WikiUpdates: []models.WikiUpdate{
			{
				Topic:   "OAuth Implementation",
				Content: "Use PKCE flow for public clients",
				TaskID:  "TASK-00001",
			},
		},
	}

	if err := ke.UpdateWiki(knowledge); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wikiDir := filepath.Join(dir, "docs", "wiki")
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		t.Fatalf("wiki directory not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 wiki file, got %d", len(entries))
	}

	data, _ := os.ReadFile(filepath.Join(wikiDir, entries[0].Name()))
	content := string(data)
	if !strings.Contains(content, "TASK-00001") {
		t.Fatal("wiki file does not contain provenance")
	}
	if !strings.Contains(content, "Learned from TASK-00001") {
		t.Fatal("wiki file does not contain 'Learned from' attribution")
	}
}

func TestUpdateWiki_AppendToExisting(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	// Create initial wiki content.
	wikiDir := filepath.Join(dir, "docs", "wiki")
	os.MkdirAll(wikiDir, 0o755)
	os.WriteFile(filepath.Join(wikiDir, "oauth-implementation.md"), []byte("# Existing content\n"), 0o644)

	knowledge := &models.ExtractedKnowledge{
		TaskID: "TASK-00002",
		WikiUpdates: []models.WikiUpdate{
			{
				WikiPath: "docs/wiki/oauth-implementation.md",
				Topic:    "OAuth Update",
				Content:  "New learning about token refresh",
				TaskID:   "TASK-00002",
			},
		},
	}

	if err := ke.UpdateWiki(knowledge); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(wikiDir, "oauth-implementation.md"))
	content := string(data)
	if !strings.Contains(content, "Existing content") {
		t.Fatal("existing wiki content was overwritten")
	}
	if !strings.Contains(content, "TASK-00002") {
		t.Fatal("new content not appended")
	}
}

func TestCreateADR(t *testing.T) {
	ke, _ := setupKnowledgeTest(t)

	decision := models.Decision{
		Title:        "Use Auth0 as OAuth Provider",
		Context:      "Need an identity provider for OAuth",
		Decision:     "We will use Auth0",
		Consequences: []string{"Vendor lock-in", "Quick setup"},
		Alternatives: []string{"Keycloak", "Custom implementation"},
	}

	path, err := ke.CreateADR(decision, "TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("expected .md path, got %q", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ADR file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "ADR-0001") {
		t.Fatal("ADR does not contain ID")
	}
	if !strings.Contains(content, "**Status:** Accepted") {
		t.Fatal("ADR missing Status")
	}
	if !strings.Contains(content, "**Source:** TASK-00001") {
		t.Fatal("ADR missing Source/provenance")
	}
	if !strings.Contains(content, "## Context") {
		t.Fatal("ADR missing Context section")
	}
	if !strings.Contains(content, "## Decision") {
		t.Fatal("ADR missing Decision section")
	}
	if !strings.Contains(content, "## Consequences") {
		t.Fatal("ADR missing Consequences section")
	}
	if !strings.Contains(content, "## Alternatives Considered") {
		t.Fatal("ADR missing Alternatives section")
	}
	if !strings.Contains(content, "Keycloak") {
		t.Fatal("ADR missing alternative: Keycloak")
	}
}

func TestCreateADR_Increment(t *testing.T) {
	ke, dir := setupKnowledgeTest(t)

	decision1 := models.Decision{Title: "First decision", Decision: "Do A"}
	decision2 := models.Decision{Title: "Second decision", Decision: "Do B"}

	path1, _ := ke.CreateADR(decision1, "TASK-00001")
	path2, _ := ke.CreateADR(decision2, "TASK-00001")

	if !strings.Contains(filepath.Base(path1), "ADR-0001") {
		t.Fatalf("first ADR should be ADR-0001, got %q", filepath.Base(path1))
	}
	if !strings.Contains(filepath.Base(path2), "ADR-0002") {
		t.Fatalf("second ADR should be ADR-0002, got %q", filepath.Base(path2))
	}

	// Verify both files exist.
	decisionsDir := filepath.Join(dir, "docs", "decisions")
	entries, _ := os.ReadDir(decisionsDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 ADR files, got %d", len(entries))
	}
}

func TestExtractSectionText(t *testing.T) {
	content := `## Summary
This is the summary text

## Next Section
Other content`

	result := extractSectionText(content, "## Summary")
	if result != "This is the summary text" {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExtractListItems(t *testing.T) {
	content := `## Items
- First
- Second
- [ ] Third

## Next`

	items := extractListItems(content, "## Items")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(items), items)
	}
	if items[2] != "Third" {
		t.Fatalf("checkbox prefix should be stripped: %q", items[2])
	}
}

func TestSanitizeForPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple Title", "simple-title"},
		{"OAuth Implementation!", "oauth-implementation"},
		{"  spaces  and  stuff  ", "spaces-and-stuff"},
		{"already-clean", "already-clean"},
	}

	for _, tt := range tests {
		result := sanitizeForPath(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeForPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

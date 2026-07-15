package core

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

func TestNewEmbedTemplateManager(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}
	if tm == nil {
		t.Fatal("NewEmbedTemplateManager() returned nil")
	}

	// Verify all templates were loaded
	expectedTemplates := []TemplateType{
		TemplateTypeNotes,
		TemplateTypeDesign,
		TemplateTypeHandoff,
		TemplateTypeStatus,
	}

	for _, tt := range expectedTemplates {
		if _, ok := tm.templates[tt]; !ok {
			t.Errorf("Template %s was not loaded", tt)
		}
	}
}

func TestRenderNotes(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"Title":              "Test Task",
		"TaskID":             "TASK-00001",
		"CreatedAt":          "2026-03-13",
		"Context":            "This is a test task",
		"AcceptanceCriteria": []string{"Criterion 1", "Criterion 2", "Criterion 3"},
		"Notes":              "Some notes here",
		"References":         "https://example.com",
	}

	result, err := tm.Render(TemplateTypeNotes, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"# Notes: Test Task",
		"**Task ID:** TASK-00001",
		"**Created:** 2026-03-13",
		"This is a test task",
		"- [ ] Criterion 1",
		"- [ ] Criterion 2",
		"- [ ] Criterion 3",
		"Some notes here",
		"https://example.com",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q, but it didn't.\nResult:\n%s", expected, result)
		}
	}
}

func TestRenderNotesWithEmptyCriteria(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"Title":              "Test Task",
		"TaskID":             "TASK-00001",
		"CreatedAt":          "2026-03-13",
		"Context":            "This is a test task",
		"AcceptanceCriteria": []string{},
		"Notes":              "Some notes here",
		"References":         "",
	}

	result, err := tm.Render(TemplateTypeNotes, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify default criterion is shown when list is empty
	if !strings.Contains(result, "- [ ] Define acceptance criteria") {
		t.Errorf("Expected default criterion when list is empty.\nResult:\n%s", result)
	}
}

func TestRenderDesign(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"Title":              "Test Design",
		"TaskID":             "TASK-00002",
		"CreatedAt":          "2026-03-13",
		"Overview":           "Design overview",
		"Components":         "Component A, Component B",
		"DataFlow":           "Data flows from A to B",
		"Dependencies":       []string{"Dependency 1", "Dependency 2"},
		"ImplementationPlan": "Step 1, Step 2",
		"TechnicalDecisions": "Decision 1",
		"OpenQuestions":      "Question 1?",
	}

	result, err := tm.Render(TemplateTypeDesign, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"# Design Document: Test Design",
		"**Task ID:** TASK-00002",
		"Design overview",
		"Component A, Component B",
		"Data flows from A to B",
		"- Dependency 1",
		"- Dependency 2",
		"Step 1, Step 2",
		"Decision 1",
		"Question 1?",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q, but it didn't.\nResult:\n%s", expected, result)
		}
	}
}

func TestRenderDesignWithNoDependencies(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"Title":              "Test Design",
		"TaskID":             "TASK-00002",
		"CreatedAt":          "2026-03-13",
		"Overview":           "Design overview",
		"Components":         "Component A",
		"DataFlow":           "Simple flow",
		"Dependencies":       []string{},
		"ImplementationPlan": "Step 1",
		"TechnicalDecisions": "Decision 1",
		"OpenQuestions":      "",
	}

	result, err := tm.Render(TemplateTypeDesign, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify default dependency message is shown
	if !strings.Contains(result, "- No external dependencies") {
		t.Errorf("Expected default dependency message when list is empty.\nResult:\n%s", result)
	}
}

func TestRenderHandoff(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	type Decision struct {
		Title       string
		Description string
		Rationale   string
	}

	data := map[string]interface{}{
		"Title":          "Test Handoff",
		"TaskID":         "TASK-00003",
		"CompletedAt":    "2026-03-13",
		"Summary":        "Task completed successfully",
		"CompletedItems": []string{"Item 1", "Item 2"},
		"Decisions": []Decision{
			{
				Title:       "Decision A",
				Description: "We decided to use approach A",
				Rationale:   "It's more efficient",
			},
			{
				Title:       "Decision B",
				Description: "We decided to use library B",
				Rationale:   "Better community support",
			},
		},
		"OpenItems":  []string{"Follow-up task 1", "Follow-up task 2"},
		"NextSteps":  "Continue with phase 2",
		"References": "https://example.com/doc",
	}

	result, err := tm.Render(TemplateTypeHandoff, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"# Handoff: Test Handoff",
		"**Task ID:** TASK-00003",
		"**Completed:** 2026-03-13",
		"Task completed successfully",
		"- Item 1",
		"- Item 2",
		"### Decision A",
		"We decided to use approach A",
		"**Rationale:** It's more efficient",
		"### Decision B",
		"We decided to use library B",
		"**Rationale:** Better community support",
		"- [ ] Follow-up task 1",
		"- [ ] Follow-up task 2",
		"Continue with phase 2",
		"https://example.com/doc",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q, but it didn't.\nResult:\n%s", expected, result)
		}
	}
}

func TestRenderHandoffWithEmptyLists(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	type Decision struct {
		Title       string
		Description string
		Rationale   string
	}

	data := map[string]interface{}{
		"Title":          "Test Handoff",
		"TaskID":         "TASK-00003",
		"CompletedAt":    "2026-03-13",
		"Summary":        "Task completed",
		"CompletedItems": []string{},
		"Decisions":      []Decision{},
		"OpenItems":      []string{},
		"NextSteps":      "",
		"References":     "",
	}

	result, err := tm.Render(TemplateTypeHandoff, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify default messages are shown
	expectedDefaults := []string{
		"- No items completed",
		"- No open items",
	}

	for _, expected := range expectedDefaults {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q when list is empty.\nResult:\n%s", expected, result)
		}
	}
}

func TestRenderStatus(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"TaskID":      "TASK-00004",
		"Title":       "Test Status",
		"Status":      "in_progress",
		"CreatedAt":   "2026-03-13T10:00:00Z",
		"UpdatedAt":   "2026-03-13T11:00:00Z",
		"CompletedAt": "",
		"Assignee":    "john.doe",
		"Priority":    "high",
		"Tags":        []string{"backend", "api", "urgent"},
	}

	result, err := tm.Render(TemplateTypeStatus, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify key content is present
	expectedStrings := []string{
		"task_id: TASK-00004",
		"title: Test Status",
		"status: in_progress",
		"created_at: 2026-03-13T10:00:00Z",
		"updated_at: 2026-03-13T11:00:00Z",
		"assignee: john.doe",
		"priority: high",
		"tags:",
		"  - backend",
		"  - api",
		"  - urgent",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain %q, but it didn't.\nResult:\n%s", expected, result)
		}
	}

	// Verify CompletedAt is not present when empty
	if strings.Contains(result, "completed_at:") {
		t.Errorf("Expected result to NOT contain 'completed_at:' when empty.\nResult:\n%s", result)
	}
}

func TestRenderStatusWithCompletedAt(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"TaskID":      "TASK-00005",
		"Title":       "Completed Task",
		"Status":      "done",
		"CreatedAt":   "2026-03-13T10:00:00Z",
		"UpdatedAt":   "2026-03-13T12:00:00Z",
		"CompletedAt": "2026-03-13T12:00:00Z",
		"Assignee":    "",
		"Priority":    "",
		"Tags":        []string{},
	}

	result, err := tm.Render(TemplateTypeStatus, data)
	if err != nil {
		t.Fatalf("Render() failed: %v", err)
	}

	// Verify CompletedAt is present
	if !strings.Contains(result, "completed_at: 2026-03-13T12:00:00Z") {
		t.Errorf("Expected result to contain 'completed_at:' when set.\nResult:\n%s", result)
	}

	// Verify optional fields are not present when empty
	optionalFields := []string{"assignee:", "priority:", "tags:"}
	for _, field := range optionalFields {
		if strings.Contains(result, field) {
			t.Errorf("Expected result to NOT contain '%s' when empty.\nResult:\n%s", field, result)
		}
	}
}

func TestRenderBytes(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"TaskID":    "TASK-00006",
		"Title":     "Test Bytes",
		"Status":    "pending",
		"CreatedAt": "2026-03-13",
		"UpdatedAt": "2026-03-13",
	}

	resultBytes, err := tm.RenderBytes(TemplateTypeStatus, data)
	if err != nil {
		t.Fatalf("RenderBytes() failed: %v", err)
	}

	// Verify we got bytes
	if len(resultBytes) == 0 {
		t.Error("RenderBytes() returned empty byte slice")
	}

	// Verify content
	result := string(resultBytes)
	if !strings.Contains(result, "task_id: TASK-00006") {
		t.Errorf("Expected result to contain task_id.\nResult:\n%s", result)
	}
}

func TestRenderInvalidTemplateType(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	data := map[string]interface{}{
		"Title": "Test",
	}

	_, err = tm.Render(TemplateType("invalid.md"), data)
	if err == nil {
		t.Error("Expected error when rendering invalid template type, got nil")
	}

	expectedError := "template invalid.md not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain %q, got: %v", expectedError, err)
	}
}

func TestRenderWithMissingData(t *testing.T) {
	tm, err := NewEmbedTemplateManager(claude.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager() failed: %v", err)
	}

	// Render with minimal data (some fields will be empty)
	data := map[string]interface{}{
		"TaskID": "TASK-00007",
		"Title":  "Minimal Data",
	}

	// Should not fail, just render empty values for missing fields
	result, err := tm.Render(TemplateTypeStatus, data)
	if err != nil {
		t.Fatalf("Render() should not fail with missing data: %v", err)
	}

	// Verify task_id is present
	if !strings.Contains(result, "task_id: TASK-00007") {
		t.Errorf("Expected result to contain task_id.\nResult:\n%s", result)
	}
}

// TestRenderDropInTemplateType proves the task-template path is data-driven too:
// a template present in the backing filesystem but never listed in the pre-parsed
// set still renders on demand (no code change needed to register it).
func TestRenderDropInTemplateType(t *testing.T) {
	fsys := fstest.MapFS{
		// A template type that is NOT in NewEmbedTemplateManager's pre-parse list.
		"changelog.md": {Data: []byte("# Changelog for {{.Title}}\n")},
		// The pre-parsed set must still exist or construction fails.
		"notes.md":        {Data: []byte("notes")},
		"design.md":       {Data: []byte("design")},
		"handoff.md":      {Data: []byte("handoff")},
		"status.yaml":     {Data: []byte("status")},
		"context.md":      {Data: []byte("context")},
		"task-context.md": {Data: []byte("task-context")},
	}

	tm, err := NewEmbedTemplateManager(fsys)
	if err != nil {
		t.Fatalf("constructing manager: %v", err)
	}

	out, err := tm.Render(TemplateType("changelog.md"), map[string]interface{}{"Title": "v1"})
	if err != nil {
		t.Fatalf("expected drop-in template to render, got error: %v", err)
	}
	if !strings.Contains(out, "# Changelog for v1") {
		t.Errorf("drop-in template not rendered; got %q", out)
	}
}

func TestTemplateTypes(t *testing.T) {
	// Test that all template type constants are defined correctly
	expectedTypes := map[TemplateType]string{
		TemplateTypeNotes:   "notes.md",
		TemplateTypeDesign:  "design.md",
		TemplateTypeHandoff: "handoff.md",
		TemplateTypeStatus:  "status.yaml",
	}

	for tt, expected := range expectedTypes {
		if string(tt) != expected {
			t.Errorf("Expected template type %v to equal %q, got %q", tt, expected, string(tt))
		}
	}
}

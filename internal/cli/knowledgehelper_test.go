package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveKnowledgeSection_NoSection(t *testing.T) {
	input := "# Task Context\n\nSome content here.\n"
	got := removeKnowledgeSection(input)
	want := "# Task Context\n\nSome content here.\n"
	if got != want {
		t.Errorf("removeKnowledgeSection() = %q, want %q", got, want)
	}
}

func TestRemoveKnowledgeSection_WithSection(t *testing.T) {
	input := "# Task Context\n\nSome content.\n\n## Accumulated Project Knowledge\n\n### Knowledge Topics\n\n| Topic |\n"
	got := removeKnowledgeSection(input)
	want := "# Task Context\n\nSome content.\n"
	if got != want {
		t.Errorf("removeKnowledgeSection() = %q, want %q", got, want)
	}
}

func TestAppendKnowledgeToTaskContext_NilKnowledgeMgr(t *testing.T) {
	// With KnowledgeMgr set to nil, the function should return without error.
	original := KnowledgeMgr
	KnowledgeMgr = nil
	defer func() { KnowledgeMgr = original }()

	tmpDir := t.TempDir()
	appendKnowledgeToTaskContext(tmpDir)
	// No panic or error means success.
}

func TestAppendKnowledgeToTaskContext_AppendsSection(t *testing.T) {
	original := KnowledgeMgr
	defer func() { KnowledgeMgr = original }()

	KnowledgeMgr = &mockKnowledgeMgrForHelper{
		summary: "### Recent Knowledge (last 30 days)\n\n- **2025-01-15**: learning: something useful\n\n",
	}

	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialContent := "# Task Context: TASK-00099\n\nSome initial content.\n"
	taskContextPath := filepath.Join(rulesDir, "task-context.md")
	if err := os.WriteFile(taskContextPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	appendKnowledgeToTaskContext(tmpDir)

	data, err := os.ReadFile(taskContextPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)

	// Should contain the original content.
	if !strings.Contains(content, "Some initial content.") {
		t.Error("expected original content to be preserved")
	}

	// Should contain the knowledge section header.
	if !strings.Contains(content, knowledgeSectionHeader) {
		t.Errorf("expected %q in content", knowledgeSectionHeader)
	}

	// Should contain the summary text.
	if !strings.Contains(content, "something useful") {
		t.Error("expected knowledge summary to be appended")
	}
}

func TestAppendKnowledgeToTaskContext_RefreshesSection(t *testing.T) {
	original := KnowledgeMgr
	defer func() { KnowledgeMgr = original }()

	KnowledgeMgr = &mockKnowledgeMgrForHelper{
		summary: "### Recent Knowledge (last 30 days)\n\n- **2025-01-16**: learning: updated knowledge\n\n",
	}

	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Simulate a file that already has a knowledge section (from a prior run).
	existingContent := "# Task Context: TASK-00099\n\nSome content.\n\n## Accumulated Project Knowledge\n\n### Old Knowledge\n\n- old stuff\n"
	taskContextPath := filepath.Join(rulesDir, "task-context.md")
	if err := os.WriteFile(taskContextPath, []byte(existingContent), 0o644); err != nil {
		t.Fatal(err)
	}

	appendKnowledgeToTaskContext(tmpDir)

	data, err := os.ReadFile(taskContextPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)

	// Old knowledge should be replaced.
	if strings.Contains(content, "old stuff") {
		t.Error("expected old knowledge section to be removed")
	}

	// New knowledge should be present.
	if !strings.Contains(content, "updated knowledge") {
		t.Error("expected refreshed knowledge summary")
	}

	// Original content before the section should be preserved.
	if !strings.Contains(content, "Some content.") {
		t.Error("expected original content to be preserved")
	}
}

func TestAppendKnowledgeToTaskContext_EmptySummary(t *testing.T) {
	original := KnowledgeMgr
	defer func() { KnowledgeMgr = original }()

	KnowledgeMgr = &mockKnowledgeMgrForHelper{summary: ""}

	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialContent := "# Task Context: TASK-00099\n"
	taskContextPath := filepath.Join(rulesDir, "task-context.md")
	if err := os.WriteFile(taskContextPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	appendKnowledgeToTaskContext(tmpDir)

	data, err := os.ReadFile(taskContextPath)
	if err != nil {
		t.Fatal(err)
	}

	// Content should be unchanged since summary is empty.
	if string(data) != initialContent {
		t.Errorf("expected file unchanged, got %q", string(data))
	}
}

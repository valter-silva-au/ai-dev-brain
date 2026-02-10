package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocTemplates_StakeholdersTemplate(t *testing.T) {
	dt := NewDocTemplates()
	content, err := dt.StakeholdersTemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "# Stakeholders") {
		t.Error("stakeholders template missing heading")
	}
	if !strings.Contains(content, "Outcome Owners") {
		t.Error("stakeholders template missing Outcome Owners section")
	}
}

func TestDocTemplates_ContactsTemplate(t *testing.T) {
	dt := NewDocTemplates()
	content, err := dt.ContactsTemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "# Contacts") {
		t.Error("contacts template missing heading")
	}
	if !strings.Contains(content, "Subject Matter Experts") {
		t.Error("contacts template missing Subject Matter Experts section")
	}
}

func TestDocTemplates_GlossaryTemplate(t *testing.T) {
	dt := NewDocTemplates()
	content, err := dt.GlossaryTemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "# Glossary") {
		t.Error("glossary template missing heading")
	}
	if !strings.Contains(content, "ADR") {
		t.Error("glossary template missing ADR term")
	}
}

func TestDocTemplates_ADRTemplate(t *testing.T) {
	dt := NewDocTemplates()
	content, err := dt.ADRTemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "ADR-XXXX") {
		t.Error("ADR template missing ADR-XXXX placeholder")
	}
	if !strings.Contains(content, "## Context") {
		t.Error("ADR template missing Context section")
	}
	if !strings.Contains(content, "## Decision") {
		t.Error("ADR template missing Decision section")
	}
	if !strings.Contains(content, "## Consequences") {
		t.Error("ADR template missing Consequences section")
	}
	if !strings.Contains(content, "## Alternatives Considered") {
		t.Error("ADR template missing Alternatives Considered section")
	}
}

func TestScaffoldDocs_CreatesDirectoryStructure(t *testing.T) {
	base := t.TempDir()
	dt := NewDocTemplates()

	if err := dt.ScaffoldDocs(base); err != nil {
		t.Fatalf("ScaffoldDocs failed: %v", err)
	}

	// Check directories exist.
	for _, dir := range []string{"docs", "docs/wiki", "docs/decisions", "docs/runbooks"} {
		info, err := os.Stat(filepath.Join(base, dir))
		if err != nil {
			t.Errorf("directory %s not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}

	// Check files exist with content.
	files := map[string]string{
		"docs/stakeholders.md":            "# Stakeholders",
		"docs/contacts.md":                "# Contacts",
		"docs/glossary.md":                "# Glossary",
		"docs/decisions/ADR-TEMPLATE.md":  "ADR-XXXX",
	}
	for path, expectedContent := range files {
		data, err := os.ReadFile(filepath.Join(base, path))
		if err != nil {
			t.Errorf("file %s not created: %v", path, err)
			continue
		}
		if !strings.Contains(string(data), expectedContent) {
			t.Errorf("file %s missing expected content %q", path, expectedContent)
		}
	}
}

func TestScaffoldDocs_SkipsExistingFiles(t *testing.T) {
	base := t.TempDir()
	dt := NewDocTemplates()

	// First scaffold.
	if err := dt.ScaffoldDocs(base); err != nil {
		t.Fatalf("first ScaffoldDocs failed: %v", err)
	}

	// Modify a file.
	stakeholdersPath := filepath.Join(base, "docs", "stakeholders.md")
	customContent := "# My Custom Stakeholders\nDo not overwrite this."
	if err := os.WriteFile(stakeholdersPath, []byte(customContent), 0644); err != nil {
		t.Fatalf("failed to write custom stakeholders: %v", err)
	}

	// Second scaffold should skip existing files.
	if err := dt.ScaffoldDocs(base); err != nil {
		t.Fatalf("second ScaffoldDocs failed: %v", err)
	}

	// Custom content should be preserved.
	data, err := os.ReadFile(stakeholdersPath)
	if err != nil {
		t.Fatalf("failed to read stakeholders after second scaffold: %v", err)
	}
	if string(data) != customContent {
		t.Errorf("stakeholders.md was overwritten: got %q, want %q", string(data), customContent)
	}
}

func TestScaffoldDocs_Idempotent(t *testing.T) {
	base := t.TempDir()
	dt := NewDocTemplates()

	// Run scaffold twice.
	if err := dt.ScaffoldDocs(base); err != nil {
		t.Fatalf("first ScaffoldDocs failed: %v", err)
	}
	if err := dt.ScaffoldDocs(base); err != nil {
		t.Fatalf("second ScaffoldDocs failed: %v", err)
	}

	// Structure should still be intact.
	info, err := os.Stat(filepath.Join(base, "docs", "stakeholders.md"))
	if err != nil {
		t.Fatalf("stakeholders.md missing after second scaffold: %v", err)
	}
	if info.IsDir() {
		t.Error("stakeholders.md is a directory, not a file")
	}
}

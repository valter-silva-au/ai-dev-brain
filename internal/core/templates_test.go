package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

func TestApplyTemplate_Feat(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	if err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("failed to read notes.md: %v", err)
	}
	if !strings.Contains(string(notes), "Requirements") {
		t.Error("feat notes.md should contain Requirements section")
	}
	if !strings.Contains(string(notes), "Acceptance Criteria") {
		t.Error("feat notes.md should contain Acceptance Criteria section")
	}

	design, err := os.ReadFile(filepath.Join(ticketPath, "design.md"))
	if err != nil {
		t.Fatalf("failed to read design.md: %v", err)
	}
	if !strings.Contains(string(design), "Architecture") {
		t.Error("feat design.md should contain Architecture section")
	}
}

func TestApplyTemplate_Bug(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "BUG-00001")
	if err := tm.ApplyTemplate(ticketPath, models.TaskTypeBug); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("failed to read notes.md: %v", err)
	}
	if !strings.Contains(string(notes), "Steps to Reproduce") {
		t.Error("bug notes.md should contain Steps to Reproduce section")
	}
	if !strings.Contains(string(notes), "Root Cause Analysis") {
		t.Error("bug notes.md should contain Root Cause Analysis section")
	}

	design, err := os.ReadFile(filepath.Join(ticketPath, "design.md"))
	if err != nil {
		t.Fatalf("failed to read design.md: %v", err)
	}
	if !strings.Contains(string(design), "Root Cause") {
		t.Error("bug design.md should contain Root Cause section")
	}
}

func TestApplyTemplate_Spike(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "SPIKE-00001")
	if err := tm.ApplyTemplate(ticketPath, models.TaskTypeSpike); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("failed to read notes.md: %v", err)
	}
	if !strings.Contains(string(notes), "Research Questions") {
		t.Error("spike notes.md should contain Research Questions section")
	}
	if !strings.Contains(string(notes), "Time-Box") {
		t.Error("spike notes.md should contain Time-Box section")
	}
}

func TestApplyTemplate_Refactor(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "REFACTOR-00001")
	if err := tm.ApplyTemplate(ticketPath, models.TaskTypeRefactor); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("failed to read notes.md: %v", err)
	}
	if !strings.Contains(string(notes), "Current State") {
		t.Error("refactor notes.md should contain Current State section")
	}
	if !strings.Contains(string(notes), "Rollback Plan") {
		t.Error("refactor notes.md should contain Rollback Plan section")
	}

	design, err := os.ReadFile(filepath.Join(ticketPath, "design.md"))
	if err != nil {
		t.Fatalf("failed to read design.md: %v", err)
	}
	if !strings.Contains(string(design), "Migration Plan") {
		t.Error("refactor design.md should contain Migration Plan section")
	}
}

func TestGetTemplate_BuiltIn(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	tmpl, err := tm.GetTemplate(models.TaskTypeFeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(tmpl, "Feature Notes") {
		t.Error("built-in feat template should contain 'Feature Notes'")
	}
}

func TestGetTemplate_UnknownType(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	_, err := tm.GetTemplate(models.TaskType("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown task type")
	}
}

func TestRegisterTemplate_CustomOverride(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Create a custom template file.
	customContent := "# Custom Feature Template\n\n## Custom Section\n"
	customPath := filepath.Join(dir, "custom_feat.md")
	if err := os.WriteFile(customPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("failed to write custom template: %v", err)
	}

	if err := tm.RegisterTemplate(models.TaskTypeFeat, customPath); err != nil {
		t.Fatalf("unexpected error registering template: %v", err)
	}

	// GetTemplate should now return the custom content.
	tmpl, err := tm.GetTemplate(models.TaskTypeFeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl != customContent {
		t.Errorf("expected custom content, got %q", tmpl)
	}

	// ApplyTemplate should use the custom template for notes.md.
	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	if err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat); err != nil {
		t.Fatalf("unexpected error applying template: %v", err)
	}

	notes, err := os.ReadFile(filepath.Join(ticketPath, "notes.md"))
	if err != nil {
		t.Fatalf("failed to read notes.md: %v", err)
	}
	if string(notes) != customContent {
		t.Errorf("notes.md should contain custom template, got %q", string(notes))
	}
}

func TestRegisterTemplate_MissingFile(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	err := tm.RegisterTemplate(models.TaskTypeFeat, filepath.Join(dir, "nonexistent.md"))
	if err == nil {
		t.Fatal("expected error for missing template file")
	}
}

func TestRegisterTemplate_RelativePath(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Create a custom template relative to basePath.
	customContent := "# Relative Custom\n"
	if err := os.WriteFile(filepath.Join(dir, "rel.md"), []byte(customContent), 0o644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	if err := tm.RegisterTemplate(models.TaskTypeBug, "rel.md"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpl, err := tm.GetTemplate(models.TaskTypeBug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl != customContent {
		t.Errorf("expected custom content, got %q", tmpl)
	}
}

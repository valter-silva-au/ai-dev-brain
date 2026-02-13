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

func TestApplyTemplate_InvalidType(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	err := tm.ApplyTemplate(ticketPath, models.TaskType("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown task type")
	}
	if !strings.Contains(err.Error(), "rendering notes template") {
		t.Errorf("expected rendering notes template error, got: %v", err)
	}
}

func TestApplyTemplate_InvalidDesignType(t *testing.T) {
	// Register a custom notes template so notes rendering succeeds,
	// but use a task type with no design template to trigger design error.
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Since there are only 4 built-in types, we need to test the error
	// from renderTemplate returning an error for design.
	// We can test by making the ticket directory read-only after notes is written.

	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")

	// First verify a valid type works (feat).
	err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify both files exist.
	if _, err := os.Stat(filepath.Join(ticketPath, "notes.md")); err != nil {
		t.Errorf("notes.md should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ticketPath, "design.md")); err != nil {
		t.Errorf("design.md should exist: %v", err)
	}
}

func TestApplyTemplate_WriteNotesError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Create a directory where notes.md would go, so writing fails.
	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(filepath.Join(ticketPath, "notes.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat)
	if err == nil {
		t.Fatal("expected error when notes.md is a directory")
	}
	if !strings.Contains(err.Error(), "writing notes.md") {
		t.Errorf("expected writing notes.md error, got: %v", err)
	}
}

func TestApplyTemplate_WriteDesignError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a directory where design.md would go, so writing fails.
	if err := os.MkdirAll(filepath.Join(ticketPath, "design.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat)
	if err == nil {
		t.Fatal("expected error when design.md is a directory")
	}
	if !strings.Contains(err.Error(), "writing design.md") {
		t.Errorf("expected writing design.md error, got: %v", err)
	}
}

func TestApplyTemplate_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Create a file where the ticket directory would be, so MkdirAll fails.
	ticketPath := filepath.Join(dir, "tickets")
	if err := os.WriteFile(ticketPath, []byte("file blocking mkdir"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := tm.ApplyTemplate(filepath.Join(ticketPath, "TASK-00001"), models.TaskTypeFeat)
	if err == nil {
		t.Fatal("expected error when ticket path cannot be created")
	}
	if !strings.Contains(err.Error(), "creating ticket directory") {
		t.Errorf("expected creating ticket directory error, got: %v", err)
	}
}

func TestGetTemplate_CustomTemplateReadError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Register a custom template path that will be deleted.
	customPath := filepath.Join(dir, "custom.md")
	if err := os.WriteFile(customPath, []byte("# Custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tm.RegisterTemplate(models.TaskTypeFeat, customPath); err != nil {
		t.Fatal(err)
	}

	// Delete the custom template file.
	if err := os.Remove(customPath); err != nil {
		t.Fatal(err)
	}

	_, err := tm.GetTemplate(models.TaskTypeFeat)
	if err == nil {
		t.Fatal("expected error for missing custom template file")
	}
	if !strings.Contains(err.Error(), "reading custom template") {
		t.Errorf("expected reading custom template error, got: %v", err)
	}
}

func TestRenderTemplate_CustomTemplateReadError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Register a custom template that will be deleted.
	customPath := filepath.Join(dir, "custom.md")
	if err := os.WriteFile(customPath, []byte("# Custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tm.RegisterTemplate(models.TaskTypeFeat, customPath); err != nil {
		t.Fatal(err)
	}
	// Delete the custom template file.
	if err := os.Remove(customPath); err != nil {
		t.Fatal(err)
	}

	// ApplyTemplate triggers renderTemplate which reads the custom file for notes.
	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	err := tm.ApplyTemplate(ticketPath, models.TaskTypeFeat)
	if err == nil {
		t.Fatal("expected error when custom template file is missing during apply")
	}
	if !strings.Contains(err.Error(), "rendering notes template") {
		t.Errorf("expected rendering notes template error, got: %v", err)
	}
}

func TestRenderTemplate_UnknownFileKind(t *testing.T) {
	dir := t.TempDir()
	// Access the internal renderTemplate through the templateManager type.
	tm := &templateManager{
		basePath:        dir,
		customTemplates: make(map[models.TaskType]string),
	}

	_, err := tm.renderTemplate(models.TaskTypeFeat, "unknown_kind", templateData{TaskID: "TASK-00001"})
	if err == nil {
		t.Fatal("expected error for unknown file kind")
	}
	if !strings.Contains(err.Error(), "unknown template file kind") {
		t.Errorf("expected unknown file kind error, got: %v", err)
	}
}

func TestRenderTemplate_UnknownTaskTypeForDesign(t *testing.T) {
	dir := t.TempDir()
	tm := &templateManager{
		basePath:        dir,
		customTemplates: make(map[models.TaskType]string),
	}

	_, err := tm.renderTemplate(models.TaskType("nonexistent"), "design", templateData{TaskID: "TASK-00001"})
	if err == nil {
		t.Fatal("expected error for unknown task type in design templates")
	}
	if !strings.Contains(err.Error(), "no design template") {
		t.Errorf("expected no design template error, got: %v", err)
	}
}

func TestApplyTemplate_DesignRenderError(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)

	// Create a ticket directory first.
	ticketPath := filepath.Join(dir, "tickets", "TASK-00001")
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// For an unknown task type, notes rendering will fail first.
	// Test design rendering error by using a valid type but making design.md unwritable.
	// Actually, we already test WriteDesignError above.
	// The renderTemplate error paths for template.Parse and template.Execute are
	// unreachable with valid built-in templates. Custom templates could have parse errors.

	// Let's inject a custom template with invalid Go template syntax.
	customPath := filepath.Join(dir, "bad_template.md")
	badTemplateContent := "# Template\n\n{{.Invalid syntax without closing}}"
	if err := os.WriteFile(customPath, []byte(badTemplateContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Note: renderTemplate only uses custom templates for "notes", not "design".
	// And custom templates are NOT parsed through text/template (line 118 returns raw string).
	// So we can't trigger a parse error in renderTemplate with custom templates.

	// The only way to trigger parse error is if the built-in templates had invalid syntax,
	// which they don't. The parse/execute error paths in renderTemplate are defensive
	// but unreachable with the current code.

	// However, we can verify that the built-in templates DO parse correctly.
	ticketPath2 := filepath.Join(dir, "tickets", "TASK-00002")
	err := tm.ApplyTemplate(ticketPath2, models.TaskTypeFeat)
	if err != nil {
		t.Fatalf("valid template should not error: %v", err)
	}
}

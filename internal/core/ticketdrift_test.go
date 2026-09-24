package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// driftFixture builds a ticket dir with the given notes/context bodies.
func driftFixture(t *testing.T, notesBody, contextBody string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(notesBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte(contextBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const driftDesc = "The full problem description that the old seed duplicated."

const seededNotes = `# Notes: t

## Context

` + driftDesc + `

## Notes

some progress
`

const cleanContext = "# Context: t\n\n## Description\n\n" + driftDesc + "\n"

// TestInspectTicket_DuplicateSeed flags a seeded notes.md.
func TestInspectTicket_DuplicateSeed(t *testing.T) {
	dir := driftFixture(t, seededNotes, cleanContext)
	lm, err := NewLayeredTemplateManager("", templates.FS)
	if err != nil {
		t.Fatalf("layered manager: %v", err)
	}
	findings := InspectTicket(dir, driftDesc, lm)
	if len(findings) != 1 || findings[0].Kind != "duplicate_seed" {
		t.Fatalf("want 1 duplicate_seed finding, got %+v", findings)
	}
}

// TestInspectTicket_CleanAndLegacy covers the no-finding paths: a de-seeded
// ticket, and one whose description only lives in context.md.
func TestInspectTicket_Clean(t *testing.T) {
	lm, err := NewLayeredTemplateManager("", templates.FS)
	if err != nil {
		t.Fatalf("layered manager: %v", err)
	}
	notes := "# Notes\n\n## Session Log\n\n- entry\n"
	dir := driftFixture(t, notes, cleanContext)
	if findings := InspectTicket(dir, driftDesc, lm); len(findings) != 0 {
		t.Fatalf("clean ticket flagged: %+v", findings)
	}
}

// TestInspectTicket_StaleTemplate flags a frontmatter version that no longer
// matches the resolved template.
func TestInspectTicket_StaleTemplate(t *testing.T) {
	lm, err := NewLayeredTemplateManager("", templates.FS)
	if err != nil {
		t.Fatalf("layered manager: %v", err)
	}
	want, _ := lm.Version(TemplateTypeNotes)
	notes := "---\ntemplate: ticket/notes\ntemplate_version: deadbeef\n---\n\n# Notes\n"
	dir := driftFixture(t, notes, cleanContext)
	findings := InspectTicket(dir, driftDesc, lm)
	if len(findings) != 1 || findings[0].Kind != "stale_template" {
		t.Fatalf("want 1 stale_template finding, got %+v", findings)
	}
	if !strings.Contains(findings[0].Detail, "deadbeef") {
		t.Errorf("detail should cite the stamped version, got %q", findings[0].Detail)
	}
	_ = want
}

// TestInspectTicket_CurrentTemplate is quiet when the stamp matches.
func TestInspectTicket_CurrentTemplate(t *testing.T) {
	wsDir := t.TempDir()
	ticketDir := t.TempDir()
	lm, err := NewLayeredTemplateManager(wsDir, templates.FS)
	if err != nil {
		t.Fatalf("layered: %v", err)
	}
	// Provision the workspace template, then stamp a ticket with its version.
	res, err := ProvisionTemplates(templates.FS, wsDir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if res.Count(HarnessInstalled) != len(TicketTemplates) {
		t.Fatalf("provision failed")
	}
	b, err := os.ReadFile(filepath.Join(wsDir, string(TemplateTypeNotes)))
	if err != nil {
		t.Fatal(err)
	}
	ver := TemplateVersion(b)
	notes := "---\ntemplate: ticket/notes\ntemplate_version: " + ver + "\n---\n\n# Notes\n"
	if err := os.WriteFile(filepath.Join(ticketDir, "notes.md"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	if findings := InspectTicket(ticketDir, driftDesc, lm); len(findings) != 0 {
		t.Fatalf("current-stamp ticket flagged: %+v", findings)
	}
}

// TestParseTicketFrontmatter covers tolerant parsing.
func TestParseTicketFrontmatter(t *testing.T) {
	fm := parseTicketFrontmatter("---\ntemplate: ticket/notes\ntemplate_version: abc12345\n---\nbody")
	if fm.Template != "ticket/notes" || fm.TemplateVersion != "abc12345" {
		t.Fatalf("parsed %+v", fm)
	}
	if fm := parseTicketFrontmatter("no frontmatter here"); fm != (ticketFrontmatter{}) {
		t.Fatalf("expected empty frontmatter, got %+v", fm)
	}
}

// TestExtractSection covers the description extractor the CLI feeds on.
func TestExtractSection(t *testing.T) {
	ctx := "# Context: t\n\n## Description\n\nline one\nline two\n\n## Acceptance\n\n- a\n"
	got := ExtractSection(ctx, "Description")
	if got != "line one\nline two" {
		t.Errorf("ExtractSection = %q", got)
	}
	if ExtractSection(ctx, "Missing") != "" {
		t.Errorf("missing section should be empty")
	}
}

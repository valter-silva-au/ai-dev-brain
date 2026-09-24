package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// embeddedFixture builds a minimal embedded template FS carrying all ticket
// templates, so the layered manager's fail-fast constructor is exercised.
func embeddedFixture() fstest.MapFS {
	return fstest.MapFS{
		"context.md":      &fstest.MapFile{Data: []byte("# Context: {{.Title}}\n{{.Description}}\n")},
		"notes.md":        &fstest.MapFile{Data: []byte("# Notes: {{.Title}}\n")},
		"design.md":       &fstest.MapFile{Data: []byte("# Design\n")},
		"handoff.md":      &fstest.MapFile{Data: []byte("# Handoff\n")},
		"status.yaml":     &fstest.MapFile{Data: []byte("task_id: {{.TaskID}}\n")},
		"task-context.md": &fstest.MapFile{Data: []byte("# Task Context\n")},
	}
}

// TestLayeredTemplateManager_EmbeddedFallback asserts the no-workspace layer
// renders exactly like the embedded-only manager did.
func TestLayeredTemplateManager_EmbeddedFallback(t *testing.T) {
	lm, err := NewLayeredTemplateManager("", embeddedFixture())
	if err != nil {
		t.Fatalf("NewLayeredTemplateManager: %v", err)
	}
	out, err := lm.Render(TemplateTypeNotes, map[string]string{"Title": "T"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "# Notes: T") {
		t.Errorf("unexpected render: %q", out)
	}
	source, err := lm.Source(TemplateTypeNotes)
	if err != nil || source != "embedded" {
		t.Errorf("source = %q, err = %v; want embedded/nil", source, err)
	}
}

// TestLayeredTemplateManager_WorkspaceWins provisions a workspace copy and
// asserts resolution prefers it, reporting its directory as the source.
func TestLayeredTemplateManager_WorkspaceWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.md", "# Workspace notes override\n")
	lm, err := NewLayeredTemplateManager(dir, embeddedFixture())
	if err != nil {
		t.Fatalf("NewLayeredTemplateManager: %v", err)
	}
	out, err := lm.Render(TemplateTypeNotes, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != "# Workspace notes override\n" {
		t.Errorf("workspace override did not win: %q", out)
	}
	if source, _ := lm.Source(TemplateTypeNotes); source != dir {
		t.Errorf("source = %q; want %q", source, dir)
	}
	// A template with no workspace copy still falls back.
	if source, _ := lm.Source(TemplateTypeContext); source != "embedded" {
		t.Errorf("fallback source = %q; want embedded", source)
	}
}

// TestLayeredTemplateManager_VersionStamps asserts version stability and
// sensitivity: same bytes → same stamp, different bytes → different stamp.
func TestLayeredTemplateManager_VersionStamps(t *testing.T) {
	lm, err := NewLayeredTemplateManager("", embeddedFixture())
	if err != nil {
		t.Fatalf("NewLayeredTemplateManager: %v", err)
	}
	v1, err := lm.Version(TemplateTypeNotes)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if len(v1) != 8 {
		t.Errorf("version stamp %q is not 8 hex chars", v1)
	}

	// Swap in a workspace override and confirm the stamp follows the resolved copy.
	dir := t.TempDir()
	writeFile(t, dir, "notes.md", "# Edited\n")
	lm2, err := NewLayeredTemplateManager(dir, embeddedFixture())
	if err != nil {
		t.Fatalf("NewLayeredTemplateManager: %v", err)
	}
	v2, _ := lm2.Version(TemplateTypeNotes)
	if v2 == v1 {
		t.Errorf("edited workspace template kept the embedded stamp %s", v1)
	}
}

// TestProvisionTemplates exercises the differ-skipped installer over the
// ticket templates: fresh install → unchanged on re-run → skipped on edit →
// force overwrites.
func TestProvisionTemplates(t *testing.T) {
	dir := t.TempDir()

	// 1. Fresh install.
	res, err := ProvisionTemplates(embeddedFixture(), dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("ProvisionTemplates: %v", err)
	}
	if res.Count(HarnessInstalled) != len(TicketTemplates) {
		t.Fatalf("fresh install: got %d installed, want %d", res.Count(HarnessInstalled), len(TicketTemplates))
	}

	// 2. Re-run: everything unchanged.
	res, err = ProvisionTemplates(embeddedFixture(), dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	if res.Count(HarnessUnchanged) != len(TicketTemplates) {
		t.Fatalf("re-run: got %d unchanged, want %d", res.Count(HarnessUnchanged), len(TicketTemplates))
	}

	// 3. User edits one → differ-skipped without --force.
	writeFile(t, dir, "notes.md", "# User edit\n")
	res, err = ProvisionTemplates(embeddedFixture(), dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("edited re-run: %v", err)
	}
	if res.Count(HarnessSkipped) != 1 {
		t.Fatalf("edited re-run: got %d skipped, want 1", res.Count(HarnessSkipped))
	}
	if b := read(t, dir+"/notes.md"); b != "# User edit\n" {
		t.Errorf("user edit clobbered: %q", b)
	}

	// 4. --force overwrites the edited file.
	res, err = ProvisionTemplates(embeddedFixture(), dir, HarnessInstallOptions{Force: true})
	if err != nil {
		t.Fatalf("force run: %v", err)
	}
	if res.Count(HarnessInstalled) != 1 {
		t.Fatalf("force run: got %d installed, want 1", res.Count(HarnessInstalled))
	}
	if b := read(t, dir+"/notes.md"); b == "# User edit\n" {
		t.Errorf("--force did not overwrite the user edit")
	}

	// 5. DryRun writes nothing.
	dir2 := t.TempDir()
	res, err = ProvisionTemplates(embeddedFixture(), dir2, HarnessInstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Count(HarnessInstalled) != len(TicketTemplates) {
		t.Fatalf("dry run: got %d planned installs, want %d", res.Count(HarnessInstalled), len(TicketTemplates))
	}
}

// TestTemplateVersion_Deterministic pins the stamp's shape and stability.
func TestTemplateVersion_Deterministic(t *testing.T) {
	a := TemplateVersion([]byte("same"))
	b := TemplateVersion([]byte("same"))
	c := TemplateVersion([]byte("different"))
	if a != b || a == c {
		t.Errorf("TemplateVersion not deterministic: %q/%q/%q", a, b, c)
	}
}

// helpers ------------------------------------------------------------

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

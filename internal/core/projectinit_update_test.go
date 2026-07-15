package core

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// v1FS is the template set at version 1: three base artifacts.
func v1FS() fstest.MapFS {
	return fstest.MapFS{
		"projectinit/VERSION":           {Data: []byte("1\n")},
		"projectinit/base/backlog.yaml": {Data: []byte("tasks: []\n")},
		"projectinit/base/.taskrc":      {Data: []byte("name: {{.Name}}\n")},
		"projectinit/base/README.md":    {Data: []byte("# {{.Name}} v1\n")},
	}
}

// v2FS bumps the version, changes README, leaves .taskrc/backlog untouched, and
// adds a new file.
func v2FS() fstest.MapFS {
	return fstest.MapFS{
		"projectinit/VERSION":           {Data: []byte("2\n")},
		"projectinit/base/backlog.yaml": {Data: []byte("tasks: []\n")},
		"projectinit/base/.taskrc":      {Data: []byte("name: {{.Name}}\n")},
		"projectinit/base/README.md":    {Data: []byte("# {{.Name}} v2\n")},
		"projectinit/base/AGENTS.md":    {Data: []byte("agents for {{.Name}}\n")},
	}
}

func TestInitializeProject_WritesManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	pi := NewFileProjectInitializer(v1FS())
	if err := pi.InitializeProject(dir, InitOptions{Name: "acme", TaskIDPrefix: "AC"}); err != nil {
		t.Fatalf("InitializeProject: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".adb", "template-manifest.yaml"))
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	var m models.TemplateManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if m.TemplateVersion != "1" {
		t.Errorf("manifest version = %q, want 1", m.TemplateVersion)
	}
	if m.Options.Name != "acme" || m.Options.TaskIDPrefix != "AC" {
		t.Errorf("manifest answers = %+v", m.Options)
	}
	for _, want := range []string{"backlog.yaml", ".taskrc", "README.md"} {
		if _, ok := m.Files[want]; !ok {
			t.Errorf("manifest missing file hash for %q (files=%v)", want, m.Files)
		}
	}
}

func TestPlanUpdate_ClassifiesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	if err := NewFileProjectInitializer(v1FS()).InitializeProject(dir, InitOptions{Name: "acme"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	plan, err := NewFileProjectInitializer(v2FS()).PlanUpdate(dir)
	if err != nil {
		t.Fatalf("PlanUpdate: %v", err)
	}
	if plan.FromVersion != "1" || plan.ToVersion != "2" {
		t.Errorf("version %s->%s, want 1->2", plan.FromVersion, plan.ToVersion)
	}
	if len(plan.Added) != 1 || plan.Added[0] != "AGENTS.md" {
		t.Errorf("Added = %v, want [AGENTS.md]", plan.Added)
	}
	if len(plan.Updated) != 1 || plan.Updated[0] != "README.md" {
		t.Errorf("Updated = %v, want [README.md]", plan.Updated)
	}
	// .taskrc + backlog.yaml unchanged between versions.
	if len(plan.Unchanged) != 2 {
		t.Errorf("Unchanged = %v, want 2 entries", plan.Unchanged)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("Conflicts = %v, want none", plan.Conflicts)
	}
	if !plan.HasChanges() {
		t.Error("HasChanges should be true")
	}
}

func TestApplyUpdate_WritesAndIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	if err := NewFileProjectInitializer(v1FS()).InitializeProject(dir, InitOptions{Name: "acme"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	pi2 := NewFileProjectInitializer(v2FS())
	if _, err := pi2.ApplyUpdate(dir, false); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}

	// README updated to v2, new file added.
	if got := readFileString(t, filepath.Join(dir, "README.md")); got != "# acme v2\n" {
		t.Errorf("README = %q, want v2", got)
	}
	if got := readFileString(t, filepath.Join(dir, "AGENTS.md")); got != "agents for acme\n" {
		t.Errorf("AGENTS.md = %q", got)
	}

	// Manifest advanced to version 2.
	m, err := readManifest(dir)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if m.TemplateVersion != "2" {
		t.Errorf("manifest version = %q, want 2", m.TemplateVersion)
	}

	// A second plan against the same (v2) template set is a no-op.
	plan, err := pi2.PlanUpdate(dir)
	if err != nil {
		t.Fatalf("PlanUpdate (2nd): %v", err)
	}
	if plan.HasChanges() || len(plan.Conflicts) != 0 {
		t.Errorf("second plan not clean: %+v", plan)
	}
}

func TestUpdate_ConflictPreservesUserEditUnlessForced(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	if err := NewFileProjectInitializer(v1FS()).InitializeProject(dir, InitOptions{Name: "acme"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	// User edits README after scaffold.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# acme EDITED\n"), 0o644); err != nil {
		t.Fatalf("edit README: %v", err)
	}

	pi2 := NewFileProjectInitializer(v2FS())
	plan, err := pi2.PlanUpdate(dir)
	if err != nil {
		t.Fatalf("PlanUpdate: %v", err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0] != "README.md" {
		t.Fatalf("Conflicts = %v, want [README.md]", plan.Conflicts)
	}

	// Non-forced apply must not clobber the user's edit.
	if _, err := pi2.ApplyUpdate(dir, false); err != nil {
		t.Fatalf("ApplyUpdate(false): %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, "README.md")); got != "# acme EDITED\n" {
		t.Errorf("README overwritten despite conflict: %q", got)
	}

	// Forced apply overwrites the conflict.
	if _, err := pi2.ApplyUpdate(dir, true); err != nil {
		t.Fatalf("ApplyUpdate(true): %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, "README.md")); got != "# acme v2\n" {
		t.Errorf("forced README = %q, want v2", got)
	}
}

func TestPlanUpdate_NoManifestIsError(t *testing.T) {
	dir := t.TempDir() // no scaffold, no manifest
	if _, err := NewFileProjectInitializer(v1FS()).PlanUpdate(dir); err == nil {
		t.Error("expected error when no manifest is present")
	}
}

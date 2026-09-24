package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withProgramApp wires a real, ISOLATED App over a fresh temp workspace for the
// duration of a test, mirroring the house pattern in stage_cli_test.go. It returns
// the workspace root, which is the root `adb program status` resolves outputs
// against.
//
// NewAppIsolated (not NewApp) is load-bearing for THIS file in particular.
// programSearchPaths() deliberately resolves all three config tiers, and an
// un-isolated App's GLOBAL tier is the developer's real ~/.taskconfig — so a
// machine carrying
//
//	custom_settings: {programs_search_paths: /tmp/ext}
//
// makes every "the shipped packs are exactly these five, all embedded" assertion
// below fail: a red suite caused by the machine, not the code. Isolation repoints
// the global tier at <tmp>/.taskconfig (absent unless a test writes it) and drops
// $ADB_ORG, so the fixture is the only input.
//
// This replaced a t.Setenv("HOME", …) helper that had to be remembered at every
// call site — and, being t.Setenv, forbade t.Parallel. (Parallelism is still off
// the table here, but for a different reason: `App` below is a package-level
// singleton. See the barrier note in hook_options_test.go.)
func withProgramApp(t *testing.T) string {
	t.Helper()
	return withAppAt(t, t.TempDir()).BasePath
}

// TestProgramCLI_List asserts `adb program list` surfaces every shipped pack, in
// both the human and the --json shape.
func TestProgramCLI_List(t *testing.T) {
	withProgramApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "list"); err != nil {
			t.Fatalf("program list: %v", err)
		}
	})
	for _, id := range []string{
		"product-discovery", "technical-design", "delivery-readiness",
		"operations", "change-adoption",
	} {
		if !strings.Contains(out, id) {
			t.Errorf("program list omits shipped pack %q:\n%s", id, out)
		}
	}

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "program", "list", "--json"); err != nil {
			t.Fatalf("program list --json: %v", err)
		}
	})
	var rows []programListJSON
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil {
		t.Fatalf("program list --json is not a JSON array: %v\n%s", err, jsonOut)
	}
	if len(rows) != 5 {
		t.Errorf("expected 5 shipped packs, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Error != "" {
			t.Errorf("pack %s reported an error: %s", r.ID, r.Error)
		}
		if r.Name == "" || r.Templates == 0 || r.Phases == 0 {
			t.Errorf("pack %s has an incomplete row: %+v", r.ID, r)
		}
		if r.Source != "embedded" {
			t.Errorf("pack %s should be embedded, got source %q", r.ID, r.Source)
		}
	}
}

// TestProgramCLI_Show asserts `adb program show` prints phases + templates and
// that --json round-trips the manifest.
func TestProgramCLI_Show(t *testing.T) {
	withProgramApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "show", "technical-design"); err != nil {
			t.Fatalf("program show: %v", err)
		}
	})
	for _, want := range []string{"Technical Design", "requirements", "prd", "Lineage:", "review:"} {
		if !strings.Contains(out, want) {
			t.Errorf("program show output missing %q:\n%s", want, out)
		}
	}

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "program", "show", "technical-design", "--json"); err != nil {
			t.Fatalf("program show --json: %v", err)
		}
	})
	var shown struct {
		Program struct {
			ID      string   `json:"id"`
			Lineage []string `json:"lineage"`
		} `json:"program"`
		Templates []struct {
			ID string `json:"id"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &shown); err != nil {
		t.Fatalf("program show --json is not valid JSON: %v\n%s", err, jsonOut)
	}
	if shown.Program.ID != "technical-design" || len(shown.Program.Lineage) == 0 {
		t.Errorf("unexpected --json identity: %+v", shown.Program)
	}
	if len(shown.Templates) != 8 {
		t.Errorf("technical-design should carry 8 templates, got %d", len(shown.Templates))
	}

	if err := runADB(t, "program", "show", "no-such-program"); err == nil {
		t.Error("show on an unknown program should error")
	}
}

// TestProgramCLI_Scaffold drives `adb program scaffold`: --dry-run writes nothing,
// a real run installs the tree (preserving subpaths), a re-run is clobber-safe, and
// --force overwrites.
func TestProgramCLI_Scaffold(t *testing.T) {
	tmp := withProgramApp(t)

	dest := filepath.Join(tmp, "scaffolded")
	if err := runADB(t, "program", "scaffold", "change-adoption", dest, "--dry-run"); err != nil {
		t.Fatalf("scaffold --dry-run: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("--dry-run must not write (err=%v)", err)
	}

	if err := runADB(t, "program", "scaffold", "change-adoption", dest); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	manifest := filepath.Join(dest, "program.yaml")
	if info, err := os.Stat(manifest); err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty scaffolded manifest (err=%v)", err)
	}
	// The pack tree is nested, so at least one template must land in a subdirectory.
	var nested bool
	if err := filepath.Walk(filepath.Join(dest, "templates"), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			nested = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if !nested {
		t.Error("scaffold did not preserve the pack's templates/ subtree")
	}

	// A local edit survives a plain re-run, and --force overwrites it.
	if err := os.WriteFile(manifest, []byte("EDITED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runADB(t, "program", "scaffold", "change-adoption", dest); err != nil {
		t.Fatalf("scaffold re-run: %v", err)
	}
	if b, _ := os.ReadFile(manifest); string(b) != "EDITED\n" {
		t.Error("re-run clobbered a local edit")
	}
	if err := runADB(t, "program", "scaffold", "change-adoption", dest, "--force"); err != nil {
		t.Fatalf("scaffold --force: %v", err)
	}
	if b, _ := os.ReadFile(manifest); string(b) == "EDITED\n" {
		t.Error("--force should overwrite a locally-edited file")
	}

	if err := runADB(t, "program", "scaffold", "no-such-program"); err == nil {
		t.Error("scaffold of an unknown program should error")
	}
}

// TestProgramCLI_StatusAndNext is the dependency-gate guard through the CLI: in a
// fresh workspace nothing is generated, phase-triggered roots are ready and their
// downstream templates are blocked ON A NAMED dependency; creating one upstream
// output flips the downstream template to ready.
func TestProgramCLI_StatusAndNext(t *testing.T) {
	tmp := withProgramApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "status", "technical-design"); err != nil {
			t.Fatalf("program status: %v", err)
		}
	})
	if !strings.Contains(out, "technical-design — Technical Design") {
		t.Errorf("status should head with the program identity:\n%s", out)
	}
	for _, want := range []string{
		"requirements", // a phase heading
		"blocked",      // design-doc has an unsatisfied requirement
		"pending",      // the manual/event-triggered templates
		"0 of 8 generated",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
	// The specific missing dependency is named, not just "blocked".
	if !strings.Contains(out, "needs: business-requirements → docs/technical-design/requirements/business-requirements.md") {
		t.Errorf("status must name the specific missing dependency:\n%s", out)
	}

	states := programStatusJSON(t, "technical-design")
	for _, st := range states {
		if st.State == "generated" {
			t.Errorf("nothing should be generated in a fresh workspace: %s", st.ID)
		}
	}
	if got := stateOf(t, states, "business-requirements"); got != "ready" {
		t.Errorf("a requirement-free phase template should be ready, got %q", got)
	}
	if got := stateOf(t, states, "prd"); got != "blocked" {
		t.Errorf("prd should be blocked before business-requirements exists, got %q", got)
	}
	if got := stateOf(t, states, "design-doc-mini"); got != "pending" {
		t.Errorf("a manual-trigger template should be pending, got %q", got)
	}

	// `next` offers exactly the ready set.
	next := programNextJSON(t)
	for _, st := range next {
		if st.State != "ready" {
			t.Errorf("next returned a non-ready template %s (%s)", st.ID, st.State)
		}
	}

	// Create the upstream artifact → the downstream template flips to ready.
	writeArtifact(t, tmp, "docs/technical-design/requirements/business-requirements.md",
		"---\nsources: []\n---\n# BRD\n")
	states = programStatusJSON(t, "technical-design")
	if got := stateOf(t, states, "business-requirements"); got != "generated" {
		t.Errorf("business-requirements should be generated once its output exists, got %q", got)
	}
	if got := stateOf(t, states, "prd"); got != "ready" {
		t.Errorf("prd should flip to ready once its requirement exists, got %q", got)
	}

	// A generated artifact whose template requires review is surfaced as such.
	out = captureStdout(t, func() {
		if err := runADB(t, "program", "status", "technical-design"); err != nil {
			t.Fatalf("program status: %v", err)
		}
	})
	if !strings.Contains(out, "still need human review") {
		t.Errorf("status must surface the outstanding human-review requirement:\n%s", out)
	}
}

// TestProgramCLI_Trace asserts `adb program trace` errors before the artifact
// exists, then reports the artifact's cited sources plus the declared requires it
// fails to cite.
func TestProgramCLI_Trace(t *testing.T) {
	tmp := withProgramApp(t)

	if err := runADB(t, "program", "trace", "technical-design", "prd"); err == nil {
		t.Error("trace should error while the artifact has not been generated")
	}
	if err := runADB(t, "program", "trace", "technical-design", "no-such-template"); err == nil {
		t.Error("trace should error on an unknown template id")
	}

	writeArtifact(t, tmp, "docs/technical-design/requirements/business-requirements.md", "# BRD\n")
	writeArtifact(t, tmp, "docs/technical-design/requirements/prd.md",
		"---\nsources:\n"+
			"  - docs/technical-design/requirements/business-requirements.md\n"+
			"  - path: project-doc/vision.md\n    note: framing\n"+
			"---\n# PRD\n")

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "trace", "technical-design", "prd"); err != nil {
			t.Fatalf("program trace: %v", err)
		}
	})
	if !strings.Contains(out, "[business-requirements]") {
		t.Errorf("trace should name the upstream template for a cited program artifact:\n%s", out)
	}
	if !strings.Contains(out, "framing") {
		t.Errorf("trace should print a source note:\n%s", out)
	}
	// project-doc/vision.md does not exist, so it must be marked absent.
	if !strings.Contains(out, "⨯ project-doc/vision.md") {
		t.Errorf("trace should flag a cited-but-missing source:\n%s", out)
	}

	// An artifact that cites none of its declared requires reports the drift.
	writeArtifact(t, tmp, "docs/technical-design/design/design-doc.md", "---\nsources: []\n---\n# HLD\n")
	out = captureStdout(t, func() {
		if err := runADB(t, "program", "trace", "technical-design", "design-doc"); err != nil {
			t.Fatalf("program trace design-doc: %v", err)
		}
	})
	if !strings.Contains(out, "declared requires not cited") {
		t.Errorf("trace should flag uncited declared requires:\n%s", out)
	}
}

// TestProgramCLI_SearchPathsResolvesExternalPack proves `programs_search_paths` is
// really wired to the layered config: a pack in a configured directory shows up in
// `list` and loads for `show`/`status`.
func TestProgramCLI_SearchPathsResolvesExternalPack(t *testing.T) {
	tmp := t.TempDir()
	packsDir := filepath.Join(tmp, "external-packs")
	writePack(t, filepath.Join(packsDir, "house-style"))

	// The repo tier carries the setting, exactly as `adb config get` would read it.
	taskrc := "repo_name: demo\ntask_id_prefix: TASK\ncustom_settings:\n  programs_search_paths: " + packsDir + "\n"
	if err := os.WriteFile(filepath.Join(tmp, ".taskrc"), []byte(taskrc), 0o644); err != nil {
		t.Fatal(err)
	}

	app := withAppAt(t, tmp)

	if _, _, ok := app.MergedConfig.SettingSource("programs_search_paths"); !ok {
		t.Fatal("test setup: programs_search_paths did not resolve through the layered config")
	}

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "list"); err != nil {
			t.Fatalf("program list: %v", err)
		}
	})
	if !strings.Contains(out, "house-style") {
		t.Fatalf("an external pack on a configured search path should list:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := runADB(t, "program", "status", "house-style"); err != nil {
			t.Fatalf("program status house-style: %v", err)
		}
	})
	if !strings.Contains(out, "House Style") {
		t.Errorf("status should load the external manifest:\n%s", out)
	}
}

// --- helpers ---------------------------------------------------------------

func programStatusJSON(t *testing.T, program string) []programTemplateJSON {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runADB(t, "program", "status", program, "--json"); err != nil {
			t.Fatalf("program status --json: %v", err)
		}
	})
	var rows []programTemplateJSON
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("program status --json is not a JSON array: %v\n%s", err, out)
	}
	return rows
}

func programNextJSON(t *testing.T) []programTemplateJSON {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runADB(t, "program", "next", "technical-design", "--json"); err != nil {
			t.Fatalf("program next --json: %v", err)
		}
	})
	var rows []programTemplateJSON
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("program next --json is not a JSON array: %v\n%s", err, out)
	}
	return rows
}

func stateOf(t *testing.T, rows []programTemplateJSON, id string) string {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r.State
		}
	}
	t.Fatalf("no template %q in the status rows", id)
	return ""
}

// writeArtifact creates a program artifact at a workspace-relative path — the
// presence-based gate's only input.
func writeArtifact(t *testing.T, root, rel, body string) {
	t.Helper()
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writePack writes a minimal valid external program pack at dir, declaring the id
// `house-style` (which every caller also uses as the directory's base name).
func writePack(t *testing.T, dir string) {
	t.Helper()
	writePackAs(t, dir, "house-style", "House Style")
}

// writePackAs writes a minimal valid external pack at dir declaring the given
// manifest id and name.
//
// id is a parameter, and separable from filepath.Base(dir), because pack
// DISCOVERY and RESOLUTION key on the DIRECTORY name while the manifest declares
// its own id — so the two can legitimately disagree for an external pack, and
// output that quotes the wrong one names something no command accepts. Keep them
// equal unless a test is specifically about that divergence.
func writePackAs(t *testing.T, dir, id, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "templates", "draft"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `program:
  id: ` + id + `
  name: ` + name + `
  lineage: ["internal house standard"]
phases:
  - {id: draft, name: Draft}
templates:
  - id: charter
    name: Charter
    phase: draft
    path: templates/draft/charter.md
    output: docs/` + id + `/draft/charter.md
    human_review:
      required: true
      note: "A human owns the charter."
      risk: "An unowned charter is not a charter."
`
	if err := os.WriteFile(filepath.Join(dir, "program.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "draft", "charter.md"), []byte("# Charter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

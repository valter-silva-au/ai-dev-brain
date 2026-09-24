package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// validManifest is a minimal well-formed program manifest used across the tests:
// two phases, a prd in the first, a design doc in the second that requires it.
const validManifest = `program:
  id: technical-design
  name: Technical Design
  version: 1
  lineage:
    - "Design Docs at Google — Malte Ubl (industrialempathy.com)"
phases:
  - { id: requirements, name: Requirements }
  - { id: design, name: Design }
templates:
  - id: prd
    name: Product Requirements
    phase: requirements
    path: templates/requirements/prd.md
    output: docs/technical-design/requirements/prd.md
    reads: ["project-doc/**"]
    human_review:
      required: true
      note: "Confirm the problem statement with the requester."
  - id: design-doc
    name: High-Level Design
    phase: design
    path: templates/design/design-doc.md
    output: docs/technical-design/design/design-doc.md
    requires: [prd]
    human_review:
      required: true
      note: "Review non-goals and rejected alternatives with the team."
      risk: "Architecture decisions are expensive to reverse after implementation starts."
`

// programFS returns a synthetic pack tree rooted at programs/technical-design so
// no test depends on the real embedded template tree.
func programFS(manifest string) fstest.MapFS {
	return fstest.MapFS{
		"programs/technical-design/program.yaml":                  {Data: []byte(manifest)},
		"programs/technical-design/README.md":                     {Data: []byte("# Technical Design\n")},
		"programs/technical-design/templates/requirements/prd.md": {Data: []byte("# PRD\n")},
		"programs/technical-design/templates/design/design-doc.md": {
			Data: []byte("# Design Doc\n"),
		},
	}
}

// mustProgram loads the synthetic program or fails the test.
func mustProgram(t *testing.T) *models.Program {
	t.Helper()
	p, err := LoadProgram(programFS(validManifest), "programs/technical-design")
	if err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	return p
}

// writeArtifact creates workspaceRoot-relative file rel with the given content.
func writeArtifact(t *testing.T, workspaceRoot, rel, content string) {
	t.Helper()
	dest := filepath.Join(workspaceRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestLoadProgram(t *testing.T) {
	p := mustProgram(t)

	if p.Program.ID != "technical-design" || p.Program.Name != "Technical Design" {
		t.Errorf("meta = %+v", p.Program)
	}
	if len(p.Program.Lineage) != 1 {
		t.Errorf("lineage = %v, want 1 entry", p.Program.Lineage)
	}
	if p.Root != "programs/technical-design" {
		t.Errorf("Root = %q", p.Root)
	}
	if got := len(p.Phases); got != 2 {
		t.Fatalf("phases = %d, want 2", got)
	}
	if p.PhaseIndex("requirements") != 0 || p.PhaseIndex("design") != 1 {
		t.Error("phase order is not manifest order")
	}
	if p.PhaseIndex("nope") != -1 {
		t.Error("unknown phase should index -1")
	}

	prd, ok := p.Template("prd")
	if !ok {
		t.Fatal("template prd not found")
	}
	// Defaults are applied at load time so callers never branch on "".
	if prd.EffectiveTrigger() != models.TriggerPhase {
		t.Errorf("trigger default = %q", prd.EffectiveTrigger())
	}
	if prd.EffectiveRequiresMode() != models.RequiresAll {
		t.Errorf("requires_mode default = %q", prd.EffectiveRequiresMode())
	}
	if len(prd.Reads) != 1 || prd.Reads[0] != "project-doc/**" {
		t.Errorf("reads = %v", prd.Reads)
	}
}

func TestLoadProgram_Errors(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		root string
		want string
	}{
		{
			name: "missing manifest",
			fsys: fstest.MapFS{"programs/x/README.md": {Data: []byte("hi")}},
			root: "programs/x",
			want: "read program manifest",
		},
		{
			name: "malformed yaml",
			fsys: fstest.MapFS{"programs/x/program.yaml": {Data: []byte("program: [oops\n")}},
			root: "programs/x",
			want: "parse program manifest",
		},
		{
			name: "invalid manifest",
			fsys: fstest.MapFS{"programs/x/program.yaml": {Data: []byte("program:\n  id: x\n  name: X\n")}},
			root: "programs/x",
			want: "lineage",
		},
		{
			name: "empty root",
			fsys: fstest.MapFS{},
			root: "",
			want: "program root not resolved",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadProgram(tc.fsys, tc.root)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateProgram(t *testing.T) {
	// base returns a fresh valid program every call so mutations don't leak.
	base := func() *models.Program {
		return &models.Program{
			Program: models.ProgramMeta{
				ID:      "p",
				Name:    "P",
				Lineage: []string{"A public source"},
			},
			Phases: []models.ProgramPhase{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
			Templates: []models.ProgramTemplate{
				{
					ID: "a", Name: "A", Phase: "one", Path: "t/a.md", Output: "docs/a.md",
					HumanReview: &models.HumanReview{Required: true, Note: "look at a"},
				},
				{
					ID: "b", Name: "B", Phase: "two", Path: "t/b.md", Output: "docs/b.md",
					Requires:    []string{"a"},
					HumanReview: &models.HumanReview{Required: true, Note: "look at b"},
				},
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(p *models.Program)
		wantErr string // substring; "" means the program must validate
	}{
		{name: "valid", mutate: func(*models.Program) {}},
		{
			name:    "empty lineage",
			mutate:  func(p *models.Program) { p.Program.Lineage = nil },
			wantErr: "lineage must list at least one public source",
		},
		{
			name:    "blank lineage entry",
			mutate:  func(p *models.Program) { p.Program.Lineage = []string{"  "} },
			wantErr: "lineage",
		},
		{
			name:    "missing program id",
			mutate:  func(p *models.Program) { p.Program.ID = "" },
			wantErr: "program id is required",
		},
		{
			name:    "no phases",
			mutate:  func(p *models.Program) { p.Phases = nil },
			wantErr: "at least one phase",
		},
		{
			name:    "duplicate phase id",
			mutate:  func(p *models.Program) { p.Phases[1].ID = "one" },
			wantErr: "duplicate phase id",
		},
		{
			name:    "unknown phase on template",
			mutate:  func(p *models.Program) { p.Templates[1].Phase = "three" },
			wantErr: `unknown phase "three"`,
		},
		{
			name:    "unknown requires id",
			mutate:  func(p *models.Program) { p.Templates[1].Requires = []string{"nope"} },
			wantErr: `unknown template id "nope"`,
		},
		{
			name:    "requires a path instead of an id",
			mutate:  func(p *models.Program) { p.Templates[1].Requires = []string{"docs/a.md"} },
			wantErr: "unknown template id",
		},
		{
			name:    "duplicate template ids",
			mutate:  func(p *models.Program) { p.Templates[1].ID = "a" },
			wantErr: "duplicate template id",
		},
		{
			name:    "duplicate outputs",
			mutate:  func(p *models.Program) { p.Templates[1].Output = "docs/a.md" },
			wantErr: "duplicate output",
		},
		{
			name:    "missing human review",
			mutate:  func(p *models.Program) { p.Templates[0].HumanReview = nil },
			wantErr: "human_review is required",
		},
		{
			name:    "self dependency",
			mutate:  func(p *models.Program) { p.Templates[0].Requires = []string{"a"} },
			wantErr: "dependency cycle",
		},
		{
			name:    "two node cycle",
			mutate:  func(p *models.Program) { p.Templates[0].Requires = []string{"b"} },
			wantErr: "dependency cycle: a -> b -> a",
		},
		{
			name:    "bad trigger",
			mutate:  func(p *models.Program) { p.Templates[0].Trigger = "someday" },
			wantErr: `invalid trigger "someday"`,
		},
		{
			name:    "bad requires mode",
			mutate:  func(p *models.Program) { p.Templates[1].RequiresMode = "most" },
			wantErr: `invalid requires_mode "most"`,
		},
		{
			name:    "missing output",
			mutate:  func(p *models.Program) { p.Templates[0].Output = "" },
			wantErr: "output is required",
		},
		{
			name:    "missing path",
			mutate:  func(p *models.Program) { p.Templates[0].Path = "" },
			wantErr: "path is required",
		},
		{
			name:    "no templates",
			mutate:  func(p *models.Program) { p.Templates = nil },
			wantErr: "at least one template",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(p)
			err := ValidateProgram(p)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected an error mentioning %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}

	t.Run("nil program", func(t *testing.T) {
		if err := ValidateProgram(nil); err == nil {
			t.Fatal("expected an error for a nil program")
		}
	})
}

func TestListPrograms(t *testing.T) {
	embedded := fstest.MapFS{
		"programs/technical-design/program.yaml": {Data: []byte(validManifest)},
		"programs/incident-review/program.yaml":  {Data: []byte(validManifest)},
		// A stray directory without a manifest is not a program.
		"programs/not-a-program/README.md": {Data: []byte("nope")},
	}

	external := t.TempDir()
	for _, id := range []string{"custom-program", "technical-design"} {
		if err := os.MkdirAll(filepath.Join(external, id), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(external, id, "program.yaml"), []byte(validManifest), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	// Also a bare dir with no manifest, which must be ignored.
	if err := os.MkdirAll(filepath.Join(external, "junk"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Run("embedded only", func(t *testing.T) {
		got, err := ListPrograms(embedded, "programs", nil)
		if err != nil {
			t.Fatalf("ListPrograms: %v", err)
		}
		want := []string{"incident-review", "technical-design"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("embedded plus external, sorted and deduped", func(t *testing.T) {
		got, err := ListPrograms(embedded, "programs", []string{external, filepath.Join(external, "missing")})
		if err != nil {
			t.Fatalf("ListPrograms: %v", err)
		}
		want := []string{"custom-program", "incident-review", "technical-design"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v (technical-design must appear once)", got, want)
		}
	})

	t.Run("missing embedded root is not fatal", func(t *testing.T) {
		got, err := ListPrograms(fstest.MapFS{}, "programs", []string{external})
		if err != nil {
			t.Fatalf("ListPrograms: %v", err)
		}
		want := []string{"custom-program", "technical-design"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestScaffoldProgram(t *testing.T) {
	fsys := programFS(validManifest)

	t.Run("preserves nested subpaths", func(t *testing.T) {
		dir := t.TempDir()
		entries, err := ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{})
		if err != nil {
			t.Fatalf("ScaffoldProgram: %v", err)
		}
		if len(entries) != 4 {
			t.Errorf("scaffolded %d entries, want 4: %+v", len(entries), entries)
		}
		for _, rel := range []string{
			"program.yaml",
			"README.md",
			filepath.Join("templates", "requirements", "prd.md"),
			filepath.Join("templates", "design", "design-doc.md"),
		} {
			data, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil || len(data) == 0 {
				t.Errorf("%s not written: %v (len %d)", rel, err, len(data))
			}
		}
		for _, e := range entries {
			if e.Action != HarnessInstalled {
				t.Errorf("%s action = %q, want installed", e.Name, e.Action)
			}
		}
	})

	t.Run("idempotent then clobber-safe then force", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{}); err != nil {
			t.Fatalf("first scaffold: %v", err)
		}
		again, err := ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{})
		if err != nil {
			t.Fatalf("second scaffold: %v", err)
		}
		for _, e := range again {
			if e.Action != HarnessUnchanged {
				t.Errorf("re-scaffold %s = %q, want unchanged", e.Name, e.Action)
			}
		}

		edited := filepath.Join(dir, "templates", "requirements", "prd.md")
		if err := os.WriteFile(edited, []byte("# my edit\n"), 0o644); err != nil {
			t.Fatalf("edit: %v", err)
		}
		res, err := ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{})
		if err != nil {
			t.Fatalf("third scaffold: %v", err)
		}
		if act := packActionFor(res, "templates/requirements/prd.md"); act != HarnessSkipped {
			t.Errorf("edited file = %q, want skipped", act)
		}
		if data, _ := os.ReadFile(edited); string(data) != "# my edit\n" {
			t.Error("edited file was clobbered without --force")
		}

		res, err = ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{Force: true})
		if err != nil {
			t.Fatalf("force scaffold: %v", err)
		}
		if act := packActionFor(res, "templates/requirements/prd.md"); act != HarnessInstalled {
			t.Errorf("forced file = %q, want installed", act)
		}
		if data, _ := os.ReadFile(edited); string(data) != "# PRD\n" {
			t.Errorf("force did not restore the embedded content, got %q", data)
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		entries, err := ScaffoldProgram(fsys, "programs", "technical-design", dir, HarnessInstallOptions{DryRun: true})
		if err != nil {
			t.Fatalf("dry run: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("dry run planned nothing")
		}
		if _, err := os.Stat(filepath.Join(dir, "program.yaml")); err == nil {
			t.Error("dry run wrote a file")
		}
	})

	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProgram(fsys, "programs", "nope", dir, HarnessInstallOptions{}); err == nil {
			t.Error("expected an error for an unknown program")
		}
		if _, err := ScaffoldProgram(fsys, "programs", "technical-design", "", HarnessInstallOptions{}); err == nil {
			t.Error("expected an error for an empty destination")
		}
		// An invalid manifest must be refused BEFORE anything is written.
		bad := fstest.MapFS{
			"programs/broken/program.yaml": {Data: []byte("program:\n  id: broken\n  name: Broken\n")},
		}
		if _, err := ScaffoldProgram(bad, "programs", "broken", dir, HarnessInstallOptions{}); err == nil {
			t.Error("expected an error for an invalid manifest")
		}
	})
}

// packActionFor finds the entry whose destination ends with the given slash-relative
// suffix and returns its action.
func packActionFor(entries []PackScaffoldEntry, relSuffix string) HarnessInstallAction {
	want := filepath.FromSlash(relSuffix)
	for _, e := range entries {
		if strings.HasSuffix(e.Dest, want) {
			return e.Action
		}
	}
	return ""
}

func TestProgramStatus(t *testing.T) {
	t.Run("blocked then ready then generated", func(t *testing.T) {
		p := mustProgram(t)
		ws := t.TempDir()

		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if len(states) != 2 {
			t.Fatalf("got %d states, want 2", len(states))
		}
		// Phase order: requirements (prd) before design (design-doc).
		if states[0].Template.ID != "prd" || states[1].Template.ID != "design-doc" {
			t.Fatalf("ordering = %s, %s", states[0].Template.ID, states[1].Template.ID)
		}
		if states[0].Status != models.TemplateReady {
			t.Errorf("prd = %q, want ready", states[0].Status)
		}
		if states[1].Status != models.TemplateBlocked {
			t.Errorf("design-doc = %q, want blocked", states[1].Status)
		}
		if len(states[1].BlockedBy) != 1 {
			t.Fatalf("BlockedBy = %v, want one entry", states[1].BlockedBy)
		}
		blocked := states[1].BlockedBy[0]
		if !strings.Contains(blocked, "prd") ||
			!strings.Contains(blocked, "docs/technical-design/requirements/prd.md") {
			t.Errorf("BlockedBy %q must name the missing template id AND its output path", blocked)
		}

		// Generating the prd unblocks the design doc (presence-based gate).
		writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "# PRD\n")
		states, err = ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if states[0].Status != models.TemplateGenerated {
			t.Errorf("prd = %q, want generated", states[0].Status)
		}
		if states[1].Status != models.TemplateReady {
			t.Errorf("design-doc = %q, want ready", states[1].Status)
		}
		if len(states[1].BlockedBy) != 0 {
			t.Errorf("BlockedBy = %v, want empty once satisfied", states[1].BlockedBy)
		}
	})

	t.Run("manual and event triggers are pending, never ready", func(t *testing.T) {
		p := mustProgram(t)
		p.Templates[0].Trigger = models.TriggerManual
		p.Templates[1].Trigger = models.TriggerEvent
		ws := t.TempDir()

		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		for _, st := range states {
			if st.Status != models.TemplatePending {
				t.Errorf("%s = %q, want pending", st.Template.ID, st.Status)
			}
		}
		next, err := NextTemplates(p, ws)
		if err != nil {
			t.Fatalf("NextTemplates: %v", err)
		}
		if len(next) != 0 {
			t.Errorf("NextTemplates = %v, want none", next)
		}
	})

	t.Run("generated wins over trigger", func(t *testing.T) {
		p := mustProgram(t)
		p.Templates[0].Trigger = models.TriggerManual
		ws := t.TempDir()
		writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "# PRD\n")

		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if states[0].Status != models.TemplateGenerated {
			t.Errorf("prd = %q, want generated", states[0].Status)
		}
	})

	t.Run("requires_mode any", func(t *testing.T) {
		p := mustProgram(t)
		p.Templates = append(p.Templates, models.ProgramTemplate{
			ID: "notes", Name: "Notes", Phase: "requirements",
			Path: "templates/requirements/notes.md", Output: "docs/technical-design/requirements/notes.md",
			HumanReview: &models.HumanReview{Required: true},
		})
		p.Templates[1].Requires = []string{"prd", "notes"}
		p.Templates[1].RequiresMode = models.RequiresAny
		if err := ValidateProgram(p); err != nil {
			t.Fatalf("mutated program must stay valid: %v", err)
		}
		ws := t.TempDir()

		// Nothing generated → blocked, and BOTH missing requirements are named.
		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		design := stateFor(t, states, "design-doc")
		if design.Status != models.TemplateBlocked {
			t.Errorf("design-doc = %q, want blocked", design.Status)
		}
		if len(design.BlockedBy) != 2 {
			t.Errorf("BlockedBy = %v, want both requirements", design.BlockedBy)
		}

		// One of two is enough under `any`.
		writeArtifact(t, ws, "docs/technical-design/requirements/notes.md", "# Notes\n")
		states, err = ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if got := stateFor(t, states, "design-doc").Status; got != models.TemplateReady {
			t.Errorf("design-doc = %q, want ready under requires_mode any", got)
		}
	})

	t.Run("phase order then manifest order", func(t *testing.T) {
		p := mustProgram(t)
		// Prepend a design-phase template so manifest order and phase order differ.
		p.Templates = append([]models.ProgramTemplate{{
			ID: "adr", Name: "ADR", Phase: "design",
			Path: "templates/design/adr.md", Output: "docs/technical-design/design/adr.md",
			HumanReview: &models.HumanReview{Required: true},
		}}, p.Templates...)
		if err := ValidateProgram(p); err != nil {
			t.Fatalf("mutated program must stay valid: %v", err)
		}
		states, err := ProgramStatus(p, t.TempDir())
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		var ids []string
		for _, st := range states {
			ids = append(ids, st.Template.ID)
		}
		want := "prd,adr,design-doc"
		if strings.Join(ids, ",") != want {
			t.Errorf("order = %v, want %s", ids, want)
		}
	})

	t.Run("errors", func(t *testing.T) {
		if _, err := ProgramStatus(nil, t.TempDir()); err == nil {
			t.Error("expected an error for a nil program")
		}
		if _, err := ProgramStatus(mustProgram(t), ""); err == nil {
			t.Error("expected an error for an empty workspace root")
		}
	})
}

func TestNextTemplates(t *testing.T) {
	p := mustProgram(t)
	ws := t.TempDir()

	next, err := NextTemplates(p, ws)
	if err != nil {
		t.Fatalf("NextTemplates: %v", err)
	}
	if len(next) != 1 || next[0].Template.ID != "prd" {
		t.Fatalf("next = %+v, want only prd", next)
	}

	writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "# PRD\n")
	next, err = NextTemplates(p, ws)
	if err != nil {
		t.Fatalf("NextTemplates: %v", err)
	}
	if len(next) != 1 || next[0].Template.ID != "design-doc" {
		t.Fatalf("next = %+v, want only design-doc", next)
	}
}

// stateFor finds a template state by id.
func stateFor(t *testing.T, states []TemplateState, id string) TemplateState {
	t.Helper()
	for _, st := range states {
		if st.Template.ID == id {
			return st
		}
	}
	t.Fatalf("no state for template %q", id)
	return TemplateState{}
}

func TestTraceArtifact(t *testing.T) {
	t.Run("reads the sources frontmatter", func(t *testing.T) {
		p := mustProgram(t)
		ws := t.TempDir()
		writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "# PRD\n")
		writeArtifact(t, ws, "project-doc/vision.md", "# Vision\n")
		writeArtifact(t, ws, "docs/technical-design/design/design-doc.md", `---
sources:
  - docs/technical-design/requirements/prd.md
  - path: project-doc/vision.md
    note: framing
  - project-doc/gone.md
---

# Design Doc
`)

		tr, err := TraceArtifact(p, ws, "design-doc")
		if err != nil {
			t.Fatalf("TraceArtifact: %v", err)
		}
		if tr.TemplateID != "design-doc" {
			t.Errorf("TemplateID = %q", tr.TemplateID)
		}
		if len(tr.Sources) != 3 {
			t.Fatalf("sources = %+v, want 3", tr.Sources)
		}
		if tr.Sources[0].TemplateID != "prd" {
			t.Errorf("first source should resolve to the prd template, got %q", tr.Sources[0].TemplateID)
		}
		if !tr.Sources[0].Exists || !tr.Sources[1].Exists {
			t.Error("present sources should be marked as existing")
		}
		if tr.Sources[1].Note != "framing" {
			t.Errorf("note = %q", tr.Sources[1].Note)
		}
		if tr.Sources[2].Exists {
			t.Error("a source file that is gone must not be marked as existing")
		}
		if len(tr.MissingRequires) != 0 {
			t.Errorf("MissingRequires = %v, want none (prd is cited)", tr.MissingRequires)
		}
	})

	t.Run("declared requirement not cited", func(t *testing.T) {
		p := mustProgram(t)
		ws := t.TempDir()
		writeArtifact(t, ws, "docs/technical-design/design/design-doc.md", "# Design Doc\n")

		tr, err := TraceArtifact(p, ws, "design-doc")
		if err != nil {
			t.Fatalf("TraceArtifact: %v", err)
		}
		if len(tr.Sources) != 0 {
			t.Errorf("sources = %+v, want none for an artifact without frontmatter", tr.Sources)
		}
		if len(tr.MissingRequires) != 1 || tr.MissingRequires[0] != "prd" {
			t.Errorf("MissingRequires = %v, want [prd]", tr.MissingRequires)
		}
	})

	t.Run("errors", func(t *testing.T) {
		p := mustProgram(t)
		ws := t.TempDir()

		_, err := TraceArtifact(p, ws, "design-doc")
		if err == nil {
			t.Fatal("expected an error for an artifact that does not exist yet")
		}
		if !strings.Contains(err.Error(), "not been generated") {
			t.Errorf("error %q should say the artifact has not been generated", err)
		}
		if _, err := TraceArtifact(p, ws, "nope"); err == nil {
			t.Error("expected an error for an unknown template id")
		}
		if _, err := TraceArtifact(nil, ws, "prd"); err == nil {
			t.Error("expected an error for a nil program")
		}
		if _, err := TraceArtifact(p, "", "prd"); err == nil {
			t.Error("expected an error for an empty workspace root")
		}

		// Malformed frontmatter is an error, not silently-empty sources.
		writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "---\nsources: [oops\n---\n")
		if _, err := TraceArtifact(p, ws, "prd"); err == nil {
			t.Error("expected an error for malformed frontmatter")
		}
	})
}

func TestScaffoldPackTree_FlatPackMatchesScaffoldPack(t *testing.T) {
	// The recursive walker must behave exactly like the flat scaffolder for a
	// flat pack — same entries, same actions — so the sibling function is a pure
	// superset and existing callers could migrate without a behaviour change.
	fsys := fstest.MapFS{
		"gtm/moat/moat-narrative.md": {Data: []byte("# Moat\n")},
		"gtm/moat/positioning.md":    {Data: []byte("# Positioning\n")},
	}
	flatDir, treeDir := t.TempDir(), t.TempDir()

	flat, err := scaffoldPack(fsys, "gtm", "moat", flatDir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffoldPack: %v", err)
	}
	tree, err := scaffoldPackTree(fsys, "gtm", "moat", treeDir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffoldPackTree: %v", err)
	}
	if len(flat) != len(tree) {
		t.Fatalf("flat %d entries, tree %d entries", len(flat), len(tree))
	}
	for i := range flat {
		if flat[i].Name != tree[i].Name || flat[i].Action != tree[i].Action {
			t.Errorf("entry %d: flat %+v vs tree %+v", i, flat[i], tree[i])
		}
	}
}

func TestScaffoldPackTree_UsesSlashRelativeNames(t *testing.T) {
	fsys := fstest.MapFS{
		"programs/x/a/b/c.md": {Data: []byte("c\n")},
	}
	dir := t.TempDir()
	entries, err := scaffoldPackTree(fsys, "programs", "x", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("scaffoldPackTree: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want 1", entries)
	}
	if entries[0].Name != "a/b/c.md" {
		t.Errorf("Name = %q, want the slash-relative subpath a/b/c.md", entries[0].Name)
	}
	if entries[0].Dest != filepath.Join(dir, "a", "b", "c.md") {
		t.Errorf("Dest = %q", entries[0].Dest)
	}
}

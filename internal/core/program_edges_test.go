package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Edge cases the main tables do not reach: the remaining validation messages, the
// discovery rules for a messy search path, and the failure modes of reading an
// artifact's frontmatter. These are the branches a malformed pack or a hostile
// filesystem hits, and each one is a message a human has to act on.

// TestValidateProgram_RemainingMessages covers the identity/name invariants the
// main validation table leaves out, so every rejection reason has a pinned
// message rather than just a non-nil error.
func TestValidateProgram_RemainingMessages(t *testing.T) {
	base := func() *models.Program {
		return &models.Program{
			Program: models.ProgramMeta{ID: "p", Name: "P", Lineage: []string{"src"}},
			Phases:  []models.ProgramPhase{{ID: "one", Name: "One"}},
			Templates: []models.ProgramTemplate{{
				ID: "a", Name: "A", Phase: "one", Path: "t/a.md", Output: "docs/a.md",
				HumanReview: &models.HumanReview{Required: true, Note: "n", Risk: "r"},
			}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(p *models.Program)
		wantErr string
	}{
		{
			name:    "missing program name",
			mutate:  func(p *models.Program) { p.Program.Name = "" },
			wantErr: `program "p": name is required`,
		},
		{
			name:    "whitespace program name",
			mutate:  func(p *models.Program) { p.Program.Name = "   " },
			wantErr: "name is required",
		},
		{
			name:    "phase with no id",
			mutate:  func(p *models.Program) { p.Phases[0].ID = "" },
			wantErr: "every phase needs an id",
		},
		{
			name:    "template with no id",
			mutate:  func(p *models.Program) { p.Templates[0].ID = "" },
			wantErr: "every template needs an id",
		},
		{
			name:    "template with no name",
			mutate:  func(p *models.Program) { p.Templates[0].Name = "" },
			wantErr: `template "a": name is required`,
		},
		{
			name:    "whitespace template id is still missing",
			mutate:  func(p *models.Program) { p.Templates[0].ID = "  " },
			wantErr: "every template needs an id",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(p)
			err := ValidateProgram(p)
			if err == nil {
				t.Fatalf("expected an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestListPrograms_DiscoveryRules covers the messy-search-path rules: an empty
// entry, a plain file where a pack directory would be, a nested directory without
// a manifest, and precedence between two search paths.
func TestListPrograms_DiscoveryRules(t *testing.T) {
	embedded := fstest.MapFS{
		"programs/shipped/program.yaml": {Data: []byte(validManifest)},
	}

	first := t.TempDir()
	second := t.TempDir()
	// A real external pack in each, plus a colliding id in both.
	for _, spec := range []struct{ dir, id string }{
		{first, "only-in-first"},
		{first, "in-both"},
		{second, "in-both"},
		{second, "only-in-second"},
	} {
		dir := filepath.Join(spec.dir, spec.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "program.yaml"), []byte(validManifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A plain FILE in a search path is not a pack directory.
	if err := os.WriteFile(filepath.Join(first, "loose-file.yaml"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory with no manifest is skipped, not an error.
	if err := os.MkdirAll(filepath.Join(first, "no-manifest", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ListPrograms(embedded, "programs", []string{
		"", // an empty entry is ignored rather than treated as "."
		first,
		second,
		filepath.Join(first, "does-not-exist"), // a missing search path is not an error
	})
	if err != nil {
		t.Fatalf("ListPrograms: %v", err)
	}
	want := "in-both,only-in-first,only-in-second,shipped"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// TestListPrograms_UnreadableSearchPathIsAnError asserts a search path that
// exists but cannot be read is reported rather than silently yielding no packs —
// a misconfigured directory should not look like an empty one.
func TestListPrograms_UnreadableSearchPathIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission bits this test relies on")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // so TempDir cleanup succeeds

	_, err := ListPrograms(fstest.MapFS{}, "programs", []string{dir})
	if err == nil {
		t.Fatal("an unreadable search path should be an error, not an empty result")
	}
	if !strings.Contains(err.Error(), "read program search path") {
		t.Errorf("error %q should name the unreadable search path", err)
	}
}

// TestNextTemplates_PropagatesStatusErrors asserts `next` does not swallow the
// errors `status` raises — they are the same preconditions.
func TestNextTemplates_PropagatesStatusErrors(t *testing.T) {
	tests := []struct {
		name string
		p    *models.Program
		ws   string
	}{
		{name: "nil program", p: nil, ws: t.TempDir()},
		{name: "empty workspace root", p: mustProgram(t), ws: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NextTemplates(tc.p, tc.ws); err == nil {
				t.Error("expected NextTemplates to propagate the error")
			}
		})
	}
}

// TestParseArtifactFrontmatter covers the frontmatter reader's cases as a table:
// what counts as absent, what counts as malformed, and which stray bytes are
// tolerated. `trace` is only as trustworthy as this parser.
func TestParseArtifactFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		body string
		// wantErr means an error is required; errSubstr, when set, must appear in it.
		wantErr     bool
		errSubstr   string
		wantSources []string
	}{
		{
			name: "no frontmatter is not an error",
			body: "# Just a document\n",
		},
		{
			name: "empty file is not an error",
			body: "",
		},
		{
			name:        "closed with three dashes",
			body:        "---\nsources:\n  - a.md\n---\n# Doc\n",
			wantSources: []string{"a.md"},
		},
		{
			name:        "closed with an ellipsis terminator",
			body:        "---\nsources:\n  - a.md\n...\n# Doc\n",
			wantSources: []string{"a.md"},
		},
		{
			name:        "a UTF-8 BOM is tolerated",
			body:        "\uFEFF---\nsources:\n  - a.md\n---\n",
			wantSources: []string{"a.md"},
		},
		{
			name:        "CRLF line endings are tolerated",
			body:        "---\r\nsources:\r\n  - a.md\r\n---\r\n",
			wantSources: []string{"a.md"},
		},
		{
			name:        "the mapping form carries a note",
			body:        "---\nsources:\n  - path: a.md\n    note: why\n---\n",
			wantSources: []string{"a.md"},
		},
		{
			name:        "unknown keys are ignored",
			body:        "---\ntitle: T\nowner: o\nsources: [a.md]\nlineage: \"x\"\n---\n",
			wantSources: []string{"a.md"},
		},
		{
			name: "an explicitly empty sources list records nothing",
			body: "---\nsources: []\n---\n",
		},
		{
			// A block that opens and never closes is MALFORMED, not absent — saying
			// "no sources" for a truncated file would silently under-report provenance.
			name:      "an unterminated block is an error",
			body:      "---\nsources:\n  - a.md\n",
			wantErr:   true,
			errSubstr: "never closed",
		},
		{
			name:    "malformed yaml inside the block is an error",
			body:    "---\nsources: [oops\n---\n",
			wantErr: true,
		},
		{
			name:    "a source entry that is neither scalar nor mapping is an error",
			body:    "---\nsources:\n  - [1, 2]\n---\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, err := parseArtifactFrontmatter([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.name)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("error %q does not mention %q", err, tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fm.Sources) != len(tc.wantSources) {
				t.Fatalf("got %d sources, want %d (%v)", len(fm.Sources), len(tc.wantSources), fm.Sources)
			}
			for i, want := range tc.wantSources {
				if fm.Sources[i].Path != want {
					t.Errorf("source %d = %q, want %q", i, fm.Sources[i].Path, want)
				}
			}
		})
	}
}

// TestTraceArtifact_UnreadableArtifact asserts a read failure that is NOT
// "missing" is reported as a read error, distinct from the not-generated-yet
// message — the two mean different things to whoever has to fix it.
func TestTraceArtifact_UnreadableArtifact(t *testing.T) {
	p := mustProgram(t)
	ws := t.TempDir()

	// A DIRECTORY at the output path: the gate does not count it as generated, and
	// reading it fails with something other than not-exist.
	if err := os.MkdirAll(filepath.Join(ws, filepath.FromSlash("docs/technical-design/requirements/prd.md")), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := TraceArtifact(p, ws, "prd")
	if err == nil {
		t.Fatal("expected an error tracing a directory")
	}
	if strings.Contains(err.Error(), "not been generated") {
		t.Errorf("a directory at the output path should not report as not-generated: %v", err)
	}
	if !strings.Contains(err.Error(), "read artifact") {
		t.Errorf("error %q should say it failed to read the artifact", err)
	}
}

// TestTraceArtifact_MissingRequiresUnderRequiresAny asserts the drift signal is
// computed from the DECLARED requires list regardless of requires_mode: `any`
// changes when a template is ready, not what it is supposed to cite.
func TestTraceArtifact_MissingRequiresUnderRequiresAny(t *testing.T) {
	p := mustProgram(t)
	p.Templates = append(p.Templates, models.ProgramTemplate{
		ID: "notes", Name: "Notes", Phase: "requirements",
		Path: "templates/requirements/notes.md", Output: "docs/technical-design/requirements/notes.md",
		HumanReview: &models.HumanReview{Required: true, Note: "n", Risk: "r"},
	})
	p.Templates[1].Requires = []string{"prd", "notes"}
	p.Templates[1].RequiresMode = models.RequiresAny
	if err := ValidateProgram(p); err != nil {
		t.Fatalf("mutated program must stay valid: %v", err)
	}

	ws := t.TempDir()
	writeArtifact(t, ws, "docs/technical-design/requirements/prd.md", "# PRD\n")
	// The design doc cites only ONE of its two `any` requirements.
	writeArtifact(t, ws, "docs/technical-design/design/design-doc.md",
		"---\nsources:\n  - docs/technical-design/requirements/prd.md\n---\n# HLD\n")

	tr, err := TraceArtifact(p, ws, "design-doc")
	if err != nil {
		t.Fatalf("TraceArtifact: %v", err)
	}
	// `notes` is uncited, so it is reported even though `any` was satisfied by prd.
	if len(tr.MissingRequires) != 1 || tr.MissingRequires[0] != "notes" {
		t.Errorf("MissingRequires = %v, want [notes] — the drift signal is per declared "+
			"requirement, not per requires_mode", tr.MissingRequires)
	}
}

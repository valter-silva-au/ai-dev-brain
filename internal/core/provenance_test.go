package core_test

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// This file is the mechanical enforcement of the document-programs provenance
// boundary (TASK-00023). Every template under templates/programs/ is
// authored by adb from PUBLICLY published sources; none is derived from internal
// or confidentiality-marked material.
//
// A register in templates/programs/SOURCES.md documents the lineage per
// pack. A register alone rots, so these tests make the boundary a build failure:
// methodology text is exactly the kind of content that gets pasted in from
// wherever the author last saw it, and a reviewer cannot eyeball 30+ documents
// for provenance on every PR.

// programsRoot is the embedded pack root inside templates.FS.
const programsRoot = "programs"

// sourcesRegister is exempt from the marker scan by design: it documents the
// excluded (non-public) source categories. Exempting one known file is safer
// than weakening the pattern for the other files.
const sourcesRegister = "programs/SOURCES.md"

// internalMarkers are strings that betray a non-public source. Each is matched
// case-insensitively. Short or acronym-like markers are anchored on word
// boundaries so ordinary prose cannot trip them. The markers are generic
// (confidentiality stamps, internal hosts, non-public tooling) so the guard
// itself carries no author history.
var internalMarkers = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"confidentiality stamp (Not Public)", regexp.MustCompile(`(?i)not public`)},
	{"confidentiality stamp (internal use only)", regexp.MustCompile(`(?i)internal[- ]use[- ]only`)},
	{"confidentiality stamp (company confidential)", regexp.MustCompile(`(?i)company[- ]confidential`)},
	{"confidentiality stamp (internal visibility)", regexp.MustCompile(`(?i)visibility:\s*internal`)},
	{"internal wiki host", regexp.MustCompile(`(?i)wiki\.internal`)},
	{"internal doc host", regexp.MustCompile(`(?i)docs\.internal`)},
	{"internal tool workspace", regexp.MustCompile(`(?i)\.kiro\b`)},
}

// walkPackFiles calls fn for every regular file under the embedded programs root.
func walkPackFiles(t *testing.T, fn func(p string, content []byte)) {
	t.Helper()
	err := fs.WalkDir(templates.FS, programsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, rerr := fs.ReadFile(templates.FS, p)
		if rerr != nil {
			return rerr
		}
		fn(p, content)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", programsRoot, err)
	}
}

// TestProvenance_NoInternalMarkers is the load-bearing guard: no shipped template
// may carry a string that indicates an internal or confidentiality-marked source.
func TestProvenance_NoInternalMarkers(t *testing.T) {
	scanned := 0
	walkPackFiles(t, func(p string, content []byte) {
		if p == sourcesRegister {
			return
		}
		scanned++
		for _, m := range internalMarkers {
			if loc := m.pattern.FindIndex(content); loc != nil {
				t.Errorf(
					"%s contains %s (matched %q at byte %d)\n"+
						"Templates must be authored from public sources only. "+
						"See templates/programs/SOURCES.md.",
					p, m.name, content[loc[0]:loc[1]], loc[0],
				)
			}
		}
	})
	// A guard that silently scans nothing is worse than no guard: it reports
	// success when the embed directive is missing or the tree was renamed.
	if scanned == 0 {
		t.Fatal("scanned 0 pack files — is `//go:embed all:programs` present in templates/embed.go?")
	}
	t.Logf("scanned %d pack files for internal markers", scanned)
}

// TestProvenance_SourcesRegisterExists guards the exemption above: the register
// must actually be present, or the scan is exempting a file that does not exist
// while the packs go undocumented.
func TestProvenance_SourcesRegisterExists(t *testing.T) {
	content, err := fs.ReadFile(templates.FS, sourcesRegister)
	if err != nil {
		t.Fatalf("reading the provenance register: %v", err)
	}
	for _, want := range []string{"Excluded sources", "Sources by pack"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("%s is missing its %q section", sourcesRegister, want)
		}
	}
}

// TestProvenance_EveryPackIsRegistered pairs the packs on disk with the register,
// so adding a pack without documenting its lineage fails the build.
func TestProvenance_EveryPackIsRegistered(t *testing.T) {
	register, err := fs.ReadFile(templates.FS, sourcesRegister)
	if err != nil {
		t.Fatalf("reading the provenance register: %v", err)
	}
	packs, err := core.ListPrograms(templates.FS, programsRoot, nil)
	if err != nil {
		t.Fatalf("listing programs: %v", err)
	}
	if len(packs) == 0 {
		t.Fatal("no program packs found in the embedded filesystem")
	}
	for _, pack := range packs {
		if !strings.Contains(string(register), pack) {
			t.Errorf("pack %q is not named in %s — add its public sources there", pack, sourcesRegister)
		}
	}
}

// TestProvenance_EveryPackDeclaresLineageAndValidates asserts each embedded pack
// parses, passes ValidateProgram (which enforces non-empty lineage, resolvable
// phases and requires, no cycles, and a human_review on every template), and that
// each declared template path resolves to a real file.
func TestProvenance_EveryPackDeclaresLineageAndValidates(t *testing.T) {
	packs, err := core.ListPrograms(templates.FS, programsRoot, nil)
	if err != nil {
		t.Fatalf("listing programs: %v", err)
	}

	for _, pack := range packs {
		t.Run(pack, func(t *testing.T) {
			p, err := core.LoadProgram(templates.FS, path.Join(programsRoot, pack))
			if err != nil {
				t.Fatalf("loading program %q: %v", pack, err)
			}
			if err := core.ValidateProgram(p); err != nil {
				t.Fatalf("validating program %q: %v", pack, err)
			}
			if len(p.Program.Lineage) == 0 {
				t.Errorf("program %q declares no lineage", pack)
			}
			if len(p.Templates) == 0 {
				t.Fatalf("program %q declares no templates", pack)
			}
			for _, tpl := range p.Templates {
				if tpl.Path == "" {
					t.Errorf("template %q has no path", tpl.ID)
					continue
				}
				full := path.Join(programsRoot, pack, tpl.Path)
				if _, err := fs.Stat(templates.FS, full); err != nil {
					t.Errorf("template %q declares path %q which does not exist: %v", tpl.ID, tpl.Path, err)
				}
				if tpl.HumanReview == nil {
					t.Errorf("template %q has no human_review block", tpl.ID)
					continue
				}
				// A required review with no stated risk is a checkbox, not a
				// safeguard — the point is that the cost of skipping is explicit.
				if tpl.HumanReview.Required && strings.TrimSpace(tpl.HumanReview.Risk) == "" {
					t.Errorf("template %q requires human review but states no risk", tpl.ID)
				}
			}
		})
	}
}

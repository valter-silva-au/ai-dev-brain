package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file covers two properties that are easy to break invisibly:
//
//  1. PATH DISCIPLINE. The engine deliberately splits `path` (the embedded FS,
//     always forward-slash) from `filepath` (on disk, OS-specific). A manifest is
//     authored with forward slashes on every platform, so any value the engine
//     resolves to disk must go through filepath — and any value it hands back as a
//     manifest reference must stay forward-slash. On Linux/macOS the two coincide,
//     which is exactly why a regression here would be silent until Windows.
//
//  2. CONCURRENCY / IDEMPOTENCY. Status is a pure classification over one presence
//     pass, so concurrent readers must be safe; scaffolding is a file install, so
//     repeating it must converge rather than accumulate.

// nestedManifest declares outputs and template paths several segments deep, so a
// separator bug has somewhere to show up.
const nestedManifest = `program:
  id: nested
  name: Nested
  lineage: ["a public source"]
phases:
  - {id: one, name: One}
  - {id: two, name: Two}
templates:
  - id: deep
    name: Deep
    phase: one
    path: templates/one/sub/deep.md
    output: docs/nested/one/sub/deep.md
    human_review: {required: true, note: n, risk: r}
  - id: deeper
    name: Deeper
    phase: two
    path: templates/two/a/b/deeper.md
    output: docs/nested/two/a/b/deeper.md
    requires: [deep]
    human_review: {required: true, note: n, risk: r}
`

// nestedFS is the pack tree for nestedManifest.
func nestedFS() fstest.MapFS {
	return fstest.MapFS{
		"programs/nested/program.yaml":                {Data: []byte(nestedManifest)},
		"programs/nested/templates/one/sub/deep.md":   {Data: []byte("# Deep\n")},
		"programs/nested/templates/two/a/b/deeper.md": {Data: []byte("# Deeper\n")},
	}
}

// TestProgramPaths_ManifestIsForwardSlashDiskIsFilepath pins the split. Every
// manifest-facing value stays forward-slash; every on-disk value is filepath-joined
// from the workspace root.
func TestProgramPaths_ManifestIsForwardSlashDiskIsFilepath(t *testing.T) {
	p, err := LoadProgram(nestedFS(), "programs/nested")
	if err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	ws := t.TempDir()

	states, err := ProgramStatus(p, ws)
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}

	for _, st := range states {
		// The manifest values are handed back verbatim, forward-slash.
		if strings.Contains(st.Template.Output, `\`) {
			t.Errorf("%s: manifest output %q must stay forward-slash", st.Template.ID, st.Template.Output)
		}
		if strings.Contains(st.Template.Path, `\`) {
			t.Errorf("%s: manifest path %q must stay forward-slash", st.Template.ID, st.Template.Path)
		}
		// The resolved on-disk path is filepath-joined — separator-correct on any
		// platform, and equal to what a caller would compute the same way.
		want := filepath.Join(ws, filepath.FromSlash(st.Template.Output))
		if st.OutputPath != want {
			t.Errorf("%s: OutputPath = %q, want %q", st.Template.ID, st.OutputPath, want)
		}
		if filepath.Separator != '/' && strings.Contains(st.OutputPath, "/") {
			t.Errorf("%s: OutputPath %q kept a forward slash on a %c-separator platform",
				st.Template.ID, st.OutputPath, filepath.Separator)
		}
	}

	// The gate must find a deeply-nested artifact created through OS-joined paths.
	writeArtifact(t, ws, "docs/nested/one/sub/deep.md", "# Deep\n")
	states, err = ProgramStatus(p, ws)
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}
	if got := stateFor(t, states, "deep").Status; got != models.TemplateGenerated {
		t.Errorf("a nested artifact = %q, want generated — the output path did not resolve", got)
	}
	if got := stateFor(t, states, "deeper").Status; got != models.TemplateReady {
		t.Errorf("the dependent of a nested artifact = %q, want ready", got)
	}

	// Trace's artifact path is on-disk, so it is filepath-shaped too.
	tr, err := TraceArtifact(p, ws, "deep")
	if err != nil {
		t.Fatalf("TraceArtifact: %v", err)
	}
	if want := filepath.Join(ws, filepath.FromSlash("docs/nested/one/sub/deep.md")); tr.Artifact != want {
		t.Errorf("Trace.Artifact = %q, want %q", tr.Artifact, want)
	}
}

// TestProgramPaths_ScaffoldPreservesForwardSlashNames asserts the scaffolder
// reports pack-relative names with forward slashes (they name entries in the
// embedded FS) while writing destinations with OS separators.
func TestProgramPaths_ScaffoldPreservesForwardSlashNames(t *testing.T) {
	dir := t.TempDir()
	entries, err := ScaffoldProgram(nestedFS(), "programs", "nested", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("ScaffoldProgram: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("scaffolded %d entries, want 3", len(entries))
	}
	for _, e := range entries {
		if strings.Contains(e.Name, `\`) {
			t.Errorf("entry name %q must be forward-slash (it names an embedded-FS path)", e.Name)
		}
		if !strings.HasPrefix(e.Dest, dir) {
			t.Errorf("entry dest %q is not under the destination dir %q", e.Dest, dir)
		}
		if want := filepath.Join(dir, filepath.FromSlash(e.Name)); e.Dest != want {
			t.Errorf("entry %q dest = %q, want %q", e.Name, e.Dest, want)
		}
		if _, err := os.Stat(e.Dest); err != nil {
			t.Errorf("entry %q was reported but not written: %v", e.Name, err)
		}
	}
}

// TestProgramPaths_BackslashInOutputIsNotASeparator documents the boundary of the
// forward-slash convention: a manifest is NOT allowed to use backslashes as
// separators, and the engine does not silently reinterpret one. On a POSIX host a
// backslash is a legal filename character, so the artifact it names is a
// single oddly-named file — not a nested path.
func TestProgramPaths_BackslashInOutputIsNotASeparator(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("the distinction only exists on a forward-slash platform")
	}
	p := buildGateProgram(t, []string{"one"}, gateTemplate{id: "a"})
	p.Templates[0].Output = `docs\weird\name.md`
	ws := t.TempDir()

	states, err := ProgramStatus(p, ws)
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}
	// The whole backslash string is one path element, so the resolved path is the
	// workspace root joined with a single literal filename.
	if want := filepath.Join(ws, `docs\weird\name.md`); states[0].OutputPath != want {
		t.Errorf("OutputPath = %q, want %q (a backslash is a filename char here)", states[0].OutputPath, want)
	}
	// Creating the "nested" path a Windows author would have meant does NOT
	// satisfy the gate — which is why manifests must use forward slashes.
	writeArtifact(t, ws, "docs/weird/name.md", "# nope\n")
	states, err = ProgramStatus(p, ws)
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}
	if states[0].Status == models.TemplateGenerated {
		t.Error("a forward-slash artifact must not satisfy a backslash-declared output")
	}
}

// TestProgramStatus_ConcurrentReadersAreSafe runs the classification concurrently
// while artifacts appear underneath it. Under -race this is the guard that the
// presence pass and the pure classification hold no shared mutable state.
func TestProgramStatus_ConcurrentReadersAreSafe(t *testing.T) {
	p := buildGateProgram(t, []string{"one", "two"},
		gateTemplate{id: "a", phase: "one"},
		gateTemplate{id: "b", phase: "one"},
		gateTemplate{id: "c", phase: "two", requires: []string{"a", "b"}},
	)
	ws := t.TempDir()

	valid := map[models.TemplateStatus]bool{
		models.TemplateGenerated: true, models.TemplateReady: true,
		models.TemplateBlocked: true, models.TemplatePending: true,
	}

	var wg sync.WaitGroup
	// One writer, making artifacts appear mid-flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, id := range []string{"a", "b"} {
			writeArtifactConcurrent(t, ws, gateOutput(id), "# "+id+"\n")
		}
	}()

	// Many readers, each asserting only invariants that hold at ANY moment: the
	// template set is complete, every status is valid, and `next` is a subset of
	// the ready set from the same call.
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states, err := ProgramStatus(p, ws)
			if err != nil {
				t.Errorf("ProgramStatus: %v", err)
				return
			}
			if len(states) != 3 {
				t.Errorf("got %d states, want 3 regardless of timing", len(states))
			}
			ready := map[string]bool{}
			for _, st := range states {
				if !valid[st.Status] {
					t.Errorf("template %s has invalid status %q", st.Template.ID, st.Status)
				}
				if st.Status == models.TemplateReady {
					ready[st.Template.ID] = true
				}
				// A blocked template must never name a requirement it does not have.
				for _, entry := range st.BlockedBy {
					id, _, _ := strings.Cut(entry, " (")
					var declared bool
					for _, req := range st.Template.Requires {
						if req == id {
							declared = true
							break
						}
					}
					if !declared {
						t.Errorf("template %s is blocked by %q which it does not require",
							st.Template.ID, id)
					}
				}
			}
			next, err := NextTemplates(p, ws)
			if err != nil {
				t.Errorf("NextTemplates: %v", err)
				return
			}
			for _, st := range next {
				if st.Status != models.TemplateReady {
					t.Errorf("NextTemplates returned %s in state %q", st.Template.ID, st.Status)
				}
			}
		}()
	}
	wg.Wait()

	// Once the writer is done the end state is deterministic.
	states, err := ProgramStatus(p, ws)
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}
	if got := stateFor(t, states, "c").Status; got != models.TemplateReady {
		t.Errorf("c = %q, want ready once both requirements exist", got)
	}
}

// writeArtifactConcurrent is writeArtifact without t.Helper's fatal semantics —
// t.Fatalf must not be called from a non-test goroutine.
func writeArtifactConcurrent(t *testing.T, workspaceRoot, rel, content string) {
	dest := filepath.Join(workspaceRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Errorf("mkdir: %v", err)
		return
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		t.Errorf("write %s: %v", rel, err)
	}
}

// TestScaffoldProgram_ConcurrentDistinctDestinations installs several packs at
// once into separate directories — provisioning more than one pack is a real
// flow, and the installer must hold no cross-call state.
//
// NOTE: concurrent scaffolds into the SAME destination are deliberately not
// asserted here. The installer takes no lock, so two writers racing on one file
// is genuinely unsynchronised; a test that pinned an outcome would be flaky
// rather than informative. It is called out as an untested case in the report.
func TestScaffoldProgram_ConcurrentDistinctDestinations(t *testing.T) {
	fsys := nestedFS()
	const n = 8
	dirs := make([]string, n)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			if _, err := ScaffoldProgram(fsys, "programs", "nested", dir, HarnessInstallOptions{}); err != nil {
				t.Errorf("ScaffoldProgram into %s: %v", dir, err)
			}
		}(dirs[i])
	}
	wg.Wait()

	// Every destination must be byte-identical: the install is a pure function of
	// the pack, so a differing tree would mean shared state.
	want, err := os.ReadFile(filepath.Join(dirs[0], "templates", "one", "sub", "deep.md"))
	if err != nil {
		t.Fatalf("read reference: %v", err)
	}
	for _, dir := range dirs {
		got, err := os.ReadFile(filepath.Join(dir, "templates", "one", "sub", "deep.md"))
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s: content differs across concurrent installs", dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "program.yaml")); err != nil {
			t.Errorf("%s: manifest missing: %v", dir, err)
		}
	}
}

// TestScaffoldProgram_RepeatedInstallsConverge asserts repeating an install
// converges instead of accumulating: the third run reports exactly what the
// second did, and the tree is unchanged.
func TestScaffoldProgram_RepeatedInstallsConverge(t *testing.T) {
	fsys := nestedFS()
	dir := t.TempDir()

	first, err := ScaffoldProgram(fsys, "programs", "nested", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	for _, e := range first {
		if e.Action != HarnessInstalled {
			t.Errorf("first install: %s = %q, want installed", e.Name, e.Action)
		}
	}

	second, err := ScaffoldProgram(fsys, "programs", "nested", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	third, err := ScaffoldProgram(fsys, "programs", "nested", dir, HarnessInstallOptions{})
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if len(second) != len(third) {
		t.Fatalf("run 2 reported %d entries, run 3 reported %d", len(second), len(third))
	}
	for i := range second {
		if second[i].Name != third[i].Name || second[i].Action != third[i].Action {
			t.Errorf("entry %d drifted between identical runs: %+v vs %+v", i, second[i], third[i])
		}
		if second[i].Action != HarnessUnchanged {
			t.Errorf("%s = %q on a repeat run, want unchanged", second[i].Name, second[i].Action)
		}
	}

	// A dry run against a fully-installed tree must also be a pure read.
	before := treeSnapshot(t, dir)
	if _, err := ScaffoldProgram(fsys, "programs", "nested", dir, HarnessInstallOptions{DryRun: true}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if after := treeSnapshot(t, dir); after != before {
		t.Error("a dry run modified the installed tree")
	}
}

// treeSnapshot renders a directory tree as a stable string of relative paths and
// CONTENT hashes, so two states can be compared for "nothing changed at all". It
// hashes rather than sizing because an edit that preserves length is exactly the
// kind a size-only snapshot would miss.
func treeSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if info.IsDir() {
			b.WriteString("d " + rel + "\n")
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		b.WriteString("f " + rel + " " + hex.EncodeToString(sha256Sum(data)) + "\n")
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return b.String()
}

// sha256Sum returns the content digest of b as a slice.
func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

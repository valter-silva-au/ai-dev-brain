package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// The regression these tests guard (TASK-00027): `adb sync context` used to default
// to the legacy generator in internal/core/contextgen.go, which emits an overview
// plus a raw dump of the whole backlog. Run over a workspace whose CLAUDE.md came
// from the multi-section generator, that DROPPED the Conventions and Stakeholders
// sections — and still printed "✓ CLAUDE.md regenerated". The rich generator was
// reachable, but only behind an opt-in --rich flag, so the safe path was the one you
// had to know to ask for. The default is now rich; the legacy path is --thin.

// richMarkers are headings only the multi-section generator produces.
var richMarkers = []string{
	"## Conventions",
	"## Stakeholders & Contacts",
	"## Active Tasks",
	"## What's Changed",
}

// thinMarker is the heading only the legacy backlog-dump generator produces.
const thinMarker = "## Current Backlog"

// newContextWorkspace builds an isolated workspace seeded with the inputs the rich
// generator reads, and returns it alongside the CLAUDE.md path.
//
// NewAppIsolated rather than NewApp is load-bearing here for the same reason it is in
// TestSyncCommands: NewApp merges the developer's real $HOME/.taskconfig and writes
// terminal state into their real home directory, so a test rooted at t.TempDir() is
// only hermetic if the App itself is constructed isolated (see L400 §8).
func newContextWorkspace(t *testing.T) (dir, agentsPath string) {
	t.Helper()
	dir = t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("write backlog: %v", err)
	}

	// Seed real content for the two sections the regression dropped, so the
	// assertions prove the content was INLINED rather than that a placeholder
	// heading happened to be emitted.
	docsWiki := filepath.Join(dir, "docs", "wiki")
	if err := os.MkdirAll(docsWiki, 0o755); err != nil {
		t.Fatalf("mkdir docs/wiki: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsWiki, "team-conventions.md"),
		[]byte("# Team conventions\n\nUse the glab CLI, never plain curl.\n"), 0o644); err != nil {
		t.Fatalf("write conventions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "stakeholders.md"),
		[]byte("# Stakeholders\n\nRecords live in the career repo.\n"), 0o644); err != nil {
		t.Fatalf("write stakeholders: %v", err)
	}

	app, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	oldApp := App
	App = app
	t.Cleanup(func() { App = oldApp })

	return dir, filepath.Join(dir, core.CanonicalInstructionFile)
}

func runSyncContext(t *testing.T, args ...string) {
	t.Helper()
	cmd := newSyncContextCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync context %v: unexpected error: %v", args, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestSyncContext_DefaultsToRich is the core regression assertion: no flags must
// produce the multi-section file, not the backlog dump.
func TestSyncContext_DefaultsToRich(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	runSyncContext(t)

	got := readFile(t, claudePath)
	for _, marker := range richMarkers {
		if !strings.Contains(got, marker) {
			t.Errorf("default CLAUDE.md is missing %q — the lossy generator ran by default", marker)
		}
	}
	if strings.Contains(got, thinMarker) {
		t.Errorf("default CLAUDE.md contains %q — the legacy backlog dump ran by default", thinMarker)
	}

	// The seeded content must be inlined, not merely represented by a heading.
	if !strings.Contains(got, "Use the glab CLI, never plain curl.") {
		t.Error("Conventions section did not inline docs/wiki/*convention*.md")
	}
	if !strings.Contains(got, "Records live in the career repo.") {
		t.Error("Stakeholders section did not inline docs/stakeholders.md")
	}
}

// TestSyncContext_RichFlagMatchesDefault pins --rich as a no-op alias, so scripts and
// docs that already pass it keep working and cannot drift from the default.
func TestSyncContext_RichFlagMatchesDefault(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	runSyncContext(t)
	viaDefault := readFile(t, claudePath)

	// Remove the state file so --rich starts from the same blank slate; otherwise
	// "What's Changed" would legitimately differ between the two runs.
	if err := os.RemoveAll(filepath.Join(filepath.Dir(claudePath), ".adb")); err != nil {
		t.Fatalf("reset state: %v", err)
	}
	if err := os.Remove(claudePath); err != nil {
		t.Fatalf("reset CLAUDE.md: %v", err)
	}

	runSyncContext(t, "--rich")
	viaFlag := readFile(t, claudePath)

	if viaDefault != viaFlag {
		t.Error("--rich produced different output from the default; it must be a no-op alias")
	}
}

// TestSyncContext_ThinFlagStillWorks keeps the legacy shape reachable, so the switch
// is revertible by flag rather than by a rebuild.
func TestSyncContext_ThinFlagStillWorks(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	runSyncContext(t, "--thin")

	got := readFile(t, claudePath)
	if !strings.Contains(got, thinMarker) {
		t.Errorf("--thin CLAUDE.md is missing %q", thinMarker)
	}
	if strings.Contains(got, "## Stakeholders & Contacts") {
		t.Error("--thin produced a rich section; the flag did not select the legacy generator")
	}
}

// TestSyncContext_ThinAndRichAreMutuallyExclusive — the two flags select opposite
// generators, so accepting both would silently honour one and ignore the other.
func TestSyncContext_ThinAndRichAreMutuallyExclusive(t *testing.T) {
	newContextWorkspace(t)

	cmd := newSyncContextCmd()
	cmd.SetArgs([]string{"--thin", "--rich"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error when --thin and --rich are combined")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should explain the conflict, got: %v", err)
	}
}

// TestSyncContext_IsIdempotentAfterFirstRun pins the property that made the
// regression expensive: a regeneration that changes nothing must produce no diff, so
// a real change to CLAUDE.md is visible in git rather than buried in churn.
//
// Runs 1 and 2 differ BY DESIGN and that is not churn: "What's Changed" compares
// against the section hashes in .adb/context_state.yaml, so the first run (no prior
// state) reports every section as newly added and the second reports no changes.
// From the second run onward the output must be byte-identical — which also guards
// the removal of the `time.Now()` footer, since a timestamp in the body would fail
// this test.
func TestSyncContext_IsIdempotentAfterFirstRun(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	runSyncContext(t)
	first := readFile(t, claudePath)

	runSyncContext(t)
	second := readFile(t, claudePath)

	runSyncContext(t)
	third := readFile(t, claudePath)

	if second != third {
		t.Error("CLAUDE.md is not idempotent: run 3 differs from run 2")
	}
	if strings.Contains(third, "Generated by AI Dev Brain on ") {
		t.Error("CLAUDE.md still embeds a generation timestamp; that guarantees churn on every regen")
	}
	// Guard the assumption behind the test rather than leaving it implicit.
	if first == second {
		t.Log("note: run 1 == run 2; the What's Changed hash comparison may no longer be active")
	}
}

// TestSyncContext_WhatsChangedIsDeterministicallyOrdered pins the fix for a separate
// non-determinism found while writing these tests: generateWhatsChanged ranged over a
// Go MAP, whose iteration order is randomized, so the change list came out in a
// different order on every run. CLAUDE.md is git-tracked, so two regenerations from
// identical inputs produced different bytes whenever more than one section changed —
// invisible to the idempotency test above, because that compares two runs that each
// report ZERO changes and therefore an empty list.
//
// The first run has no prior state, so every section reports "New section added" —
// which makes it the case that exercises the full ordering.
func TestSyncContext_WhatsChangedIsDeterministicallyOrdered(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	runSyncContext(t)

	// Must match assembleCLAUDEmd's section order.
	want := []string{
		"- **Project Overview**: New section added",
		"- **Directory Structure**: New section added",
		"- **Conventions**: New section added",
		"- **Glossary**: New section added",
		"- **Architectural Decisions**: New section added",
		"- **Active Tasks**: New section added",
		"- **Critical Decisions**: New section added",
		"- **Recent Sessions**: New section added",
		"- **Captured Sessions**: New section added",
		"- **Stakeholders & Contacts**: New section added",
	}

	got := readFile(t, claudePath)
	prev := -1
	for _, line := range want {
		at := strings.Index(got, line)
		if at == -1 {
			t.Errorf("What's Changed is missing %q", line)
			continue
		}
		if at < prev {
			t.Errorf("What's Changed is out of order at %q — the list must be deterministic", line)
		}
		prev = at
	}
}

// TestSyncAll_DoesNotFlattenCLAUDEmd — `sync all` used to call GenerateAll(), whose
// GenerateContext step is the legacy dump, so it was a second entrance to the same
// data loss.
func TestSyncAll_DoesNotFlattenCLAUDEmd(t *testing.T) {
	_, claudePath := newContextWorkspace(t)

	cmd := newSyncAllCmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync all: unexpected error: %v", err)
	}

	got := readFile(t, claudePath)
	for _, marker := range richMarkers {
		if !strings.Contains(got, marker) {
			t.Errorf("sync all produced a CLAUDE.md missing %q", marker)
		}
	}
	if strings.Contains(got, thinMarker) {
		t.Errorf("sync all produced the legacy backlog dump (%q)", thinMarker)
	}
}

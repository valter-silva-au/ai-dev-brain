// Package hygiene holds build-enforced invariants about the REPOSITORY's own
// tooling configuration, as distinct from anything adb does at runtime. Nothing
// here imports an adb package, and nothing here should: a test in this directory
// asserts a fact about a checked-in config file, not about product behaviour.
//
// It exists for one recorded event. Before TASK-00039 Batch 4,
// `.pre-commit-config.yaml` had no Go section at all — the hygiene, secrets,
// Python, shell and JS tiers ran, and nothing whatsoever ran on a .go file at
// commit time. That is how this repo accumulated **985 golangci-lint findings
// unnoticed**. The Go block added in 5eded84 closed it, and this test is what
// keeps it closed: without it, deleting three lines from a YAML file silently
// removes the only thing that lints Go before a commit, and the suite stays
// green while it happens.
//
// So this guards a gap that actually occurred, not a hypothetical one. The
// failure it is built for is mundane — a careless edit, a `pre-commit
// autoupdate` that rewrites more than it should, a future session tidying a
// config it does not have the history for.
package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoPreCommitConfig is the checked-in config, relative to this test's directory.
const repoPreCommitConfig = "../../.pre-commit-config.yaml"

// goGateFindings reports what a pre-commit config is missing for Go to be gated
// at commit time. An empty slice means the gate is intact.
//
// It reads the raw text rather than unmarshalling YAML deliberately: all three
// properties are textual promises about ONE hook, the failure mode is the
// block's disappearance rather than a subtle restructuring of it, and it keeps
// this leaf the only test in the repo needing no non-stdlib import.
func goGateFindings(config string) []string {
	var missing []string

	// The hook must exist at all. It is `repo: local` so that it uses the same
	// binary the Makefile requires — the upstream hook builds its own, which is
	// how a hook starts passing what CI fails.
	if !strings.Contains(config, "golangci-lint run ./...") {
		missing = append(missing, "no `golangci-lint run ./...` entry — nothing lints Go at commit time")
	}

	// Without this the hook runs per-file, and a per-file run CANNOT see
	// cross-package findings: `unused` is answered by whether any OTHER package
	// references a symbol. File-scoped runs would have missed 11 of the 12 dead
	// functions Batch 4 deleted, so this line is load-bearing rather than a
	// performance choice.
	if !strings.Contains(config, "pass_filenames: false") {
		missing = append(missing, "no `pass_filenames: false` — a per-file run cannot see cross-package findings such as `unused`")
	}

	// `types: [go]` keeps the hook off commits that touch no Go. Its own known
	// gap is worth remembering rather than fixing here: it matches files PRESENT
	// in a commit, so a pure-deletion commit is skipped entirely — and a deletion
	// is exactly what can create an `unused` finding elsewhere.
	if !strings.Contains(config, "types: [go]") {
		missing = append(missing, "no `types: [go]` — the hook's scoping predicate is gone")
	}

	return missing
}

// readRepoConfig returns the checked-in config, failing loudly if it is absent —
// a missing config is the most complete version of the failure being guarded
// against, not a reason to skip.
func readRepoConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(repoPreCommitConfig))
	if err != nil {
		t.Fatalf("cannot read %s: %v", repoPreCommitConfig, err)
	}
	return string(raw)
}

// TestPreCommitConfigStillGatesGo is the guard itself.
func TestPreCommitConfigStillGatesGo(t *testing.T) {
	t.Parallel()

	if missing := goGateFindings(readRepoConfig(t)); len(missing) > 0 {
		t.Errorf(`%s no longer gates Go at commit time:
  - %s

Nothing else lints Go before a commit. This is the gap that let 985
golangci-lint findings accumulate unnoticed before it was closed in 5eded84 —
re-add the block rather than adjusting this test.`,
			repoPreCommitConfig, strings.Join(missing, "\n  - "))
	}
}

// TestGoGateFindings_FiresWhenTheBlockIsRemoved keeps the guard above from
// passing vacuously.
//
// The guard alone cannot prove it works: it reads a file that currently
// satisfies it, so a checker returning nil unconditionally would look identical.
// This removes the Go block from the REAL config in memory — the actual bytes
// the guarded-against edit would leave behind — and asserts every finding fires.
// Using the real file rather than a synthetic one means this test also fails if
// the block is ever restructured such that `goGateFindings` stops locating it.
func TestGoGateFindings_FiresWhenTheBlockIsRemoved(t *testing.T) {
	t.Parallel()

	config := readRepoConfig(t)

	// The block is fenced by this comment in the config; everything from it to
	// EOF is the Go section.
	const fence = "# ── Go: format + lint gate"
	withoutGoBlock, _, found := strings.Cut(config, fence)
	if !found {
		t.Fatalf("cannot locate the Go block's fence comment %q in %s — if the block was "+
			"restructured, update this test's fence and TestPreCommitConfigStillGatesGo together",
			fence, repoPreCommitConfig)
	}

	missing := goGateFindings(withoutGoBlock)
	if len(missing) != 3 {
		t.Fatalf("with the Go block removed, all three properties should be reported missing; got %d: %v",
			len(missing), missing)
	}

	// Each finding must name the property it is about, so the message the real
	// guard prints is diagnosable rather than a bare count.
	for _, want := range []string{"golangci-lint run ./...", "pass_filenames: false", "types: [go]"} {
		if !strings.Contains(strings.Join(missing, "\n"), want) {
			t.Errorf("no finding mentions %q; findings were: %v", want, missing)
		}
	}

	// And the unmodified config must report nothing, or the two tests are not
	// distinguishing anything.
	if got := goGateFindings(config); len(got) != 0 {
		t.Errorf("the checked-in config reports findings %v, so this test cannot tell it from an ungated one", got)
	}
}

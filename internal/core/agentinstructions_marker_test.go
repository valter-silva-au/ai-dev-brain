package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIsADBGeneratedRecognizesEveryMarkerEverWritten is the migration guard for
// renaming `adb sync context` → `adb context build`.
//
// The generated marker is not decoration: `IsADBGenerated` keys on it to decide
// whether a file is adb's to overwrite. It also *names the command that
// regenerates it*, so a command rename changes the string — and a changed string
// means every AGENTS.md/CLAUDE.md adb has ever written stops being recognized,
// gets reported as hand-written, and is never updated again without --force.
//
// That failure is silent and hits exactly the workspaces that have been using
// adb longest. So recognition must be the union of every marker ever written,
// while writing uses only the current one.
func TestIsADBGeneratedRecognizesEveryMarkerEverWritten(t *testing.T) {
	body := "\n\n# Context\n\nsome generated body\n"

	for name, content := range map[string]string{
		"current marker": generatedMarker + body,
		// Every entry in legacyGeneratedMarkers, spelled out here on purpose: if
		// someone edits generatedMarker without appending the old value to the
		// legacy list, this test is what fails.
		"pre-rename marker (adb sync context)": "<!-- adb:generated -- regenerate with `adb sync context`; edits here are overwritten -->" + body,
		"legacy claude first line":             "# AI Dev Brain - Claude Context" + body,
		"legacy generic first line":            "# AI Dev Brain Context" + body,
	} {
		t.Run(name, func(t *testing.T) {
			if !IsADBGenerated([]byte(content)) {
				t.Fatalf(
					"a file adb wrote is no longer recognized as generated;\n"+
						"it would be reported hand-written and never updated:\n%s",
					content,
				)
			}
		})
	}
}

// TestIsADBGeneratedRejectsHandWrittenFiles is the other half: recognition must
// stay narrow, or --force stops being the gate protecting a user's own file.
func TestIsADBGeneratedRejectsHandWrittenFiles(t *testing.T) {
	for name, content := range map[string]string{
		"empty":                    "",
		"ordinary markdown":        "# My Project\n\nNotes I wrote myself.\n",
		"mentions adb in passing":  "# Notes\n\nWe use adb for tasks.\n",
		"mentions the command":     "# Notes\n\nRun `adb context build` sometimes.\n",
		"similar but not a marker": "<!-- generated -->\n\n# Context\n",
	} {
		t.Run(name, func(t *testing.T) {
			if IsADBGenerated([]byte(content)) {
				t.Fatalf("a hand-written file was claimed as adb-generated:\n%s", content)
			}
		})
	}
}

// TestCurrentMarkerNamesTheLiveCommand keeps the marker's instruction honest. It
// tells the reader how to regenerate the file, so it must not name a command
// that was renamed away — that is the whole reason this string changes at all.
func TestCurrentMarkerNamesTheLiveCommand(t *testing.T) {
	if !strings.Contains(generatedMarker, "adb context build") {
		t.Fatalf("marker does not name the live regenerate command: %q", generatedMarker)
	}
	if strings.Contains(generatedMarker, "adb sync context") {
		t.Fatalf("marker still names the retired command: %q", generatedMarker)
	}
}

// TestPreRenameFileIsUpgradedNotSkipped is the end-to-end consequence: a
// workspace whose AGENTS.md carries the OLD marker must be rewritten in place
// (picking up the new marker), not reported skipped.
func TestPreRenameFileIsUpgradedNotSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CanonicalInstructionFile)
	preRename := "<!-- adb:generated -- regenerate with `adb sync context`; edits here are overwritten -->" +
		"\n\n# AI Dev Brain Context\n\nstale body\n"
	if err := os.WriteFile(path, []byte(preRename), 0o644); err != nil {
		t.Fatalf("seed pre-rename file: %v", err)
	}

	results, err := WriteInstructionFiles(dir, "# AI Dev Brain Context\n\nfresh body\n", nil, false)
	if err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}

	var canonical *InstructionFileResult
	for i := range results {
		if results[i].Name == CanonicalInstructionFile {
			canonical = &results[i]
		}
	}
	if canonical == nil {
		t.Fatalf("no result for %s: %#v", CanonicalInstructionFile, results)
	}
	if canonical.Action == InstructionSkipped {
		t.Fatalf(
			"a pre-rename generated file was skipped as hand-written (%s) — "+
				"the marker rename orphaned it",
			canonical.Reason,
		)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(updated), "fresh body") {
		t.Fatalf("file was not regenerated:\n%s", updated)
	}
	if !strings.Contains(string(updated), generatedMarker) {
		t.Fatalf("file did not pick up the current marker:\n%s", updated)
	}
}

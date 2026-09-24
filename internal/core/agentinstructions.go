package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalInstructionFile is the vendor-neutral instruction filename adb owns.
//
// AGENTS.md is an open format (https://agents.md) stewarded by the Agentic AI
// Foundation and read by Codex, Cursor, Aider, Gemini CLI, Zed, Devin, Copilot's
// coding agent and others. adb writes the generated context there, and keeps a
// thin per-harness pointer beside it for agents that look for their own filename
// instead (Claude Code reads CLAUDE.md). One body, many doorways — so switching
// coding agent never means regenerating context into a different shape.
const CanonicalInstructionFile = "AGENTS.md"

// InstructionPointersSetting is the layered-config custom setting naming the
// per-harness pointer files to maintain, comma-separated. "none" disables them.
const InstructionPointersSetting = "instruction_pointers"

// generatedMarker is written into every instruction file adb owns. Its presence
// is what makes overwriting safe: a file without it is assumed to be the user's
// and is never clobbered without --force.
//
// It also names the command that regenerates the file — which makes this string
// change whenever that command is renamed. Writing uses THIS value;
// recognition must use it *and* every value below. See legacyGeneratedMarkers.
const generatedMarker = "<!-- adb:generated -- regenerate with `adb context build`; edits here are overwritten -->"

// legacyGeneratedMarkers are markers adb wrote in earlier versions. They must
// stay recognized permanently, and the reason is not politeness about old files:
// recognition is what authorises an overwrite. Drop one of these and every
// instruction file adb wrote under that spelling starts reading as hand-written,
// gets reported skipped, and is never regenerated again without --force — a
// silent failure that hits the longest-running workspaces hardest.
//
// So: when the regenerate command is renamed, append the previous marker here in
// the same commit. `TestIsADBGeneratedRecognizesEveryMarkerEverWritten` spells
// each one out literally so an edit to generatedMarker alone fails the build.
var legacyGeneratedMarkers = []string{
	// TASK-00039 retired the `adb sync` namespace: `sync context` → `context build`.
	"<!-- adb:generated -- regenerate with `adb sync context`; edits here are overwritten -->",
}

// legacyGeneratedFirstLines are the opening lines of instruction files adb wrote
// before generatedMarker existed. Without them, every pre-existing workspace's
// CLAUDE.md would read as hand-written, and the migration to AGENTS.md would
// silently never happen for exactly the workspaces that have one.
var legacyGeneratedFirstLines = []string{
	"# AI Dev Brain - Claude Context",
	"# AI Dev Brain Context",
}

// DefaultInstructionPointers is the pointer set used when nothing is configured.
// Claude Code is the default launch agent, so its filename is the default
// doorway; anything else is opt-in via InstructionPointersSetting.
func DefaultInstructionPointers() []string {
	return []string{"CLAUDE.md"}
}

// InstructionFileAction records what happened to one instruction file.
type InstructionFileAction string

const (
	// InstructionWritten means adb created or updated the file.
	InstructionWritten InstructionFileAction = "written"
	// InstructionUnchanged means the file already had the exact desired content.
	InstructionUnchanged InstructionFileAction = "unchanged"
	// InstructionSkipped means the file exists, is not adb-generated, and was
	// left alone.
	InstructionSkipped InstructionFileAction = "skipped"
)

// InstructionFileResult is the per-file outcome of WriteInstructionFiles.
type InstructionFileResult struct {
	Name   string
	Action InstructionFileAction
	Reason string
}

// ParseInstructionPointers turns a configured value into a pointer filename
// list. Empty/whitespace yields DefaultInstructionPointers; the literal "none"
// (any case) yields none.
//
// Entries carrying a path separator are dropped: these names are joined onto the
// workspace root, so accepting "../../etc/passwd" would let a config value write
// outside the workspace. A pointer is a filename, never a path.
func ParseInstructionPointers(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DefaultInstructionPointers()
	}
	if strings.EqualFold(trimmed, "none") {
		return nil
	}

	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(trimmed, ",") {
		name := strings.TrimSpace(part)
		if name == "" || seen[name] {
			continue
		}
		if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// InstructionPointerBody is the content of a per-harness pointer file.
//
// The @import line is load-bearing rather than decorative: Claude Code only
// actually reads a referenced file through its @ syntax, so a pointer carrying
// only a markdown link would look right and supply no context at all. The link
// follows for harnesses (and humans) that do not implement imports.
func InstructionPointerBody() string {
	return fmt.Sprintf(`# Agent instructions

%s

Canonical instructions for this workspace live in [%s](./%s) — a single
agent-agnostic file, so every coding agent reads the same context.

@%s
`, generatedMarker, CanonicalInstructionFile, CanonicalInstructionFile, CanonicalInstructionFile)
}

// IsADBGenerated reports whether data looks like a file adb wrote — it carries
// the current marker, carries a marker from an earlier version, or opens with one
// of the pre-marker generated headers.
//
// Recognition is deliberately the union of everything adb has ever written: it is
// what authorises an overwrite, so narrowing it silently strands files adb owns.
func IsADBGenerated(data []byte) bool {
	if bytes.Contains(data, []byte(generatedMarker)) {
		return true
	}
	for _, legacy := range legacyGeneratedMarkers {
		if bytes.Contains(data, []byte(legacy)) {
			return true
		}
	}
	firstLine := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	for _, legacy := range legacyGeneratedFirstLines {
		if firstLine == legacy {
			return true
		}
	}
	return false
}

// WriteInstructionFiles publishes the generated context as the canonical
// AGENTS.md plus a pointer file per configured harness.
//
// Nothing the user wrote is destroyed: a target that exists and is not
// recognizable as adb-generated is reported InstructionSkipped and left byte
// for byte alone unless force is set. That is a deliberate change from the
// pre-AGENTS.md behaviour, which overwrote the workspace CLAUDE.md
// unconditionally — a hand-written one was simply lost on the next sync.
func WriteInstructionFiles(
	root string,
	content string,
	pointers []string,
	force bool,
) ([]InstructionFileResult, error) {
	if root == "" {
		return nil, errors.New("instruction root is required")
	}

	results := make([]InstructionFileResult, 0, len(pointers)+1)

	canonical, err := publishInstructionFile(
		root, CanonicalInstructionFile, withMarker(content), force,
	)
	if err != nil {
		return results, err
	}
	results = append(results, canonical)

	for _, name := range pointers {
		// A config listing the canonical name as a pointer must not turn
		// AGENTS.md into a pointer to itself.
		if name == CanonicalInstructionFile {
			continue
		}
		pointer, err := publishInstructionFile(root, name, InstructionPointerBody(), force)
		if err != nil {
			return results, err
		}
		results = append(results, pointer)
	}

	return results, nil
}

// withMarker ensures generated content carries the marker, inserted after the
// first heading so the file still opens with its title.
func withMarker(content string) string {
	if strings.Contains(content, generatedMarker) {
		return content
	}
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "#") {
		return lines[0] + "\n\n" + generatedMarker + "\n" + lines[1]
	}
	return generatedMarker + "\n\n" + content
}

func publishInstructionFile(
	root string,
	name string,
	desired string,
	force bool,
) (InstructionFileResult, error) {
	path := filepath.Join(root, name)

	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if string(existing) == desired {
			return InstructionFileResult{Name: name, Action: InstructionUnchanged}, nil
		}
		if !force && !IsADBGenerated(existing) {
			return InstructionFileResult{
				Name:   name,
				Action: InstructionSkipped,
				Reason: "exists and was not generated by adb — pass --force to overwrite",
			}, nil
		}
	case errors.Is(err, os.ErrNotExist):
		// Fresh write.
	default:
		return InstructionFileResult{}, fmt.Errorf("read %s: %w", name, err)
	}

	// gosec G703 flags `path` as tainted because `name` reaches here from the
	// `instruction_pointers` config setting. The traversal it is looking for cannot
	// happen: ParseInstructionPointers DROPS any entry where
	// `name != filepath.Base(name)` or which contains a `/` or `\`, precisely
	// because these names get joined onto the workspace root — so `name` is always a
	// single path component. The canonical file name is a const, not config.
	//nolint:gosec // name is constrained to one path component by ParseInstructionPointers
	if err := os.WriteFile(path, []byte(desired), 0o644); err != nil {
		return InstructionFileResult{}, fmt.Errorf("write %s: %w", name, err)
	}
	return InstructionFileResult{Name: name, Action: InstructionWritten}, nil
}

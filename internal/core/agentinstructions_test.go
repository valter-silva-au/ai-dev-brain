package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteInstructionFiles_WritesCanonicalAndPointer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	results, err := WriteInstructionFiles(root, "# Body\n", []string{"CLAUDE.md"}, false)
	if err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}

	canonical, err := os.ReadFile(filepath.Join(root, CanonicalInstructionFile))
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !strings.Contains(string(canonical), "# Body") {
		t.Errorf("canonical file lost the generated body:\n%s", canonical)
	}
	if !IsADBGenerated(canonical) {
		t.Error("canonical file is not recognizable as adb-generated")
	}

	pointer, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read pointer: %v", err)
	}
	// Claude Code only actually loads a referenced file through its @import
	// syntax; a bare markdown link would render as prose and the pointer would
	// silently carry no context at all.
	if !strings.Contains(string(pointer), "@"+CanonicalInstructionFile) {
		t.Errorf("pointer does not @import the canonical file:\n%s", pointer)
	}
	if strings.Contains(string(pointer), "# Body") {
		t.Error("pointer duplicated the body instead of pointing at it")
	}

	if got := instructionActionFor(results, CanonicalInstructionFile); got != InstructionWritten {
		t.Errorf("canonical action = %q, want %q", got, InstructionWritten)
	}
	if got := instructionActionFor(results, "CLAUDE.md"); got != InstructionWritten {
		t.Errorf("pointer action = %q, want %q", got, InstructionWritten)
	}
}

func TestWriteInstructionFiles_RefusesToClobberHandWrittenFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	handWritten := "# My own notes\n\nAlways show the full path of the files.\n"
	for _, name := range []string{CanonicalInstructionFile, "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(handWritten), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	results, err := WriteInstructionFiles(root, "# Generated\n", []string{"CLAUDE.md"}, false)
	if err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}

	for _, name := range []string{CanonicalInstructionFile, "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != handWritten {
			t.Errorf("%s was overwritten; hand-written content must survive:\n%s", name, data)
		}
		if got := instructionActionFor(results, name); got != InstructionSkipped {
			t.Errorf("%s action = %q, want %q", name, got, InstructionSkipped)
		}
	}
}

func TestWriteInstructionFiles_ForceOverwritesHandWrittenFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, CanonicalInstructionFile)
	if err := os.WriteFile(path, []byte("# Mine\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := WriteInstructionFiles(root, "# Generated\n", nil, true); err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "# Generated") {
		t.Errorf("--force did not overwrite:\n%s", data)
	}
}

// A workspace generated before the marker existed must still be adoptable —
// otherwise every existing user's CLAUDE.md reads as hand-written and the
// migration to AGENTS.md silently never happens for them.
func TestWriteInstructionFiles_AdoptsPreMarkerGeneratedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacy := "# AI Dev Brain - Claude Context\n\nold generated body\n"
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := WriteInstructionFiles(root, "# Generated\n", []string{"CLAUDE.md"}, false)
	if err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}
	if got := instructionActionFor(results, "CLAUDE.md"); got != InstructionWritten {
		t.Errorf("legacy generated file action = %q, want %q", got, InstructionWritten)
	}

	data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "@"+CanonicalInstructionFile) {
		t.Errorf("legacy file was not converted to a pointer:\n%s", data)
	}
}

func TestWriteInstructionFiles_IsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := WriteInstructionFiles(root, "# Body\n", []string{"CLAUDE.md"}, false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	results, err := WriteInstructionFiles(root, "# Body\n", []string{"CLAUDE.md"}, false)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	for _, name := range []string{CanonicalInstructionFile, "CLAUDE.md"} {
		if got := instructionActionFor(results, name); got != InstructionUnchanged {
			t.Errorf("%s action = %q, want %q on a no-op re-run", name, got, InstructionUnchanged)
		}
	}
}

func TestWriteInstructionFiles_PointerNamedLikeCanonicalIsIgnored(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// A config that lists AGENTS.md as a pointer must not turn the canonical
	// file into a pointer to itself.
	results, err := WriteInstructionFiles(
		root, "# Body\n", []string{CanonicalInstructionFile, "CLAUDE.md"}, false,
	)
	if err != nil {
		t.Fatalf("WriteInstructionFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, CanonicalInstructionFile))
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if !strings.Contains(string(data), "# Body") {
		t.Errorf("canonical file became a self-pointer:\n%s", data)
	}
	if count := instructionCountFor(results, CanonicalInstructionFile); count != 1 {
		t.Errorf("canonical reported %d times, want 1", count)
	}
}

func TestParseInstructionPointers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty falls back to default", "", DefaultInstructionPointers()},
		{"whitespace falls back to default", "   ", DefaultInstructionPointers()},
		{"single", "CLAUDE.md", []string{"CLAUDE.md"}},
		{"comma separated, trimmed", " CLAUDE.md , CODEX.md ", []string{"CLAUDE.md", "CODEX.md"}},
		{"blank entries dropped", "CLAUDE.md,,CODEX.md", []string{"CLAUDE.md", "CODEX.md"}},
		{"duplicates dropped", "CLAUDE.md,CLAUDE.md", []string{"CLAUDE.md"}},
		{"none disables pointers", "none", nil},
		{"NONE is case-insensitive", "NONE", nil},
		// A path separator would let a config write outside the workspace root.
		{"path traversal dropped", "../../etc/passwd,CLAUDE.md", []string{"CLAUDE.md"}},
		{"nested path dropped", "sub/CLAUDE.md,CODEX.md", []string{"CODEX.md"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ParseInstructionPointers(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseInstructionPointers(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ParseInstructionPointers(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func instructionActionFor(results []InstructionFileResult, name string) InstructionFileAction {
	for _, r := range results {
		if r.Name == name {
			return r.Action
		}
	}
	return ""
}

func instructionCountFor(results []InstructionFileResult, name string) int {
	n := 0
	for _, r := range results {
		if r.Name == name {
			n++
		}
	}
	return n
}

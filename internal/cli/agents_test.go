package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAgentFiles(t *testing.T) {
	t.Run("returns agent names from md files", func(t *testing.T) {
		dir := t.TempDir()
		// Create some .md files.
		for _, name := range []string{"code-reviewer.md", "researcher.md", "debugger.md"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("# Agent"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// Create a non-md file (should be ignored).
		if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("readme"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Create a directory (should be ignored).
		if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
			t.Fatal(err)
		}

		agents, err := listAgentFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(agents) != 3 {
			t.Fatalf("expected 3 agents, got %d: %v", len(agents), agents)
		}
	})

	t.Run("returns empty for nonexistent directory", func(t *testing.T) {
		_, err := listAgentFiles("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for nonexistent directory")
		}
	})

	t.Run("returns empty for empty directory", func(t *testing.T) {
		dir := t.TempDir()
		agents, err := listAgentFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(agents) != 0 {
			t.Fatalf("expected 0 agents, got %d", len(agents))
		}
	})
}

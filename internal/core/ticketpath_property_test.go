package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestProperty31_ResolveTicketDirAlwaysReturnsValidPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		basePath := t.TempDir()
		taskID := "TASK-" + rapid.StringMatching(`[0-9]{5}`).Draw(rt, "taskID")

		result := resolveTicketDir(basePath, taskID)

		// Result must always contain the task ID.
		if !strings.Contains(result, taskID) {
			rt.Fatalf("resolveTicketDir result %q should contain task ID %q", result, taskID)
		}

		// Result must always be under basePath.
		if !strings.HasPrefix(result, basePath) {
			rt.Fatalf("resolveTicketDir result %q should be under basePath %q", result, basePath)
		}
	})
}

func TestProperty32_ResolveTicketDirPrefersActive(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		basePath := t.TempDir()
		taskID := "TASK-" + rapid.StringMatching(`[0-9]{5}`).Draw(rt, "taskID")

		activeDir := filepath.Join(basePath, "tickets", taskID)
		archivedDir := filepath.Join(basePath, "tickets", ArchivedDir, taskID)

		// Create both directories.
		if err := os.MkdirAll(activeDir, 0o755); err != nil {
			rt.Fatal(err)
		}
		if err := os.MkdirAll(archivedDir, 0o755); err != nil {
			rt.Fatal(err)
		}

		result := resolveTicketDir(basePath, taskID)

		// Active path must be preferred when both exist.
		if result != activeDir {
			rt.Fatalf("resolveTicketDir should prefer active path %q, got %q", activeDir, result)
		}
	})
}

func TestProperty33_ResolveTicketDirFindsArchived(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		basePath := t.TempDir()
		taskID := "TASK-" + rapid.StringMatching(`[0-9]{5}`).Draw(rt, "taskID")

		archivedDir := filepath.Join(basePath, "tickets", ArchivedDir, taskID)
		if err := os.MkdirAll(archivedDir, 0o755); err != nil {
			rt.Fatal(err)
		}

		result := resolveTicketDir(basePath, taskID)

		// Should find the archived directory when active doesn't exist.
		if result != archivedDir {
			rt.Fatalf("resolveTicketDir should find archived path %q, got %q", archivedDir, result)
		}
	})
}

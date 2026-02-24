package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestChangeTracker_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	tracker := NewChangeTracker(dir)

	// Append entries.
	entries := []models.SessionChangeEntry{
		{Timestamp: 1000, Tool: "Edit", FilePath: "internal/core/foo.go"},
		{Timestamp: 2000, Tool: "Write", FilePath: "internal/cli/bar.go"},
		{Timestamp: 3000, Tool: "Edit", FilePath: "pkg/models/baz.go"},
	}
	for _, e := range entries {
		if err := tracker.Append(e); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read back.
	got, err := tracker.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Read returned %d entries, want 3", len(got))
	}

	for i, e := range got {
		if e.Timestamp != entries[i].Timestamp {
			t.Errorf("entry[%d].Timestamp = %d, want %d", i, e.Timestamp, entries[i].Timestamp)
		}
		if e.Tool != entries[i].Tool {
			t.Errorf("entry[%d].Tool = %q, want %q", i, e.Tool, entries[i].Tool)
		}
		if e.FilePath != entries[i].FilePath {
			t.Errorf("entry[%d].FilePath = %q, want %q", i, e.FilePath, entries[i].FilePath)
		}
	}
}

func TestChangeTracker_ReadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	tracker := NewChangeTracker(dir)

	got, err := tracker.Read()
	if err != nil {
		t.Fatalf("Read on non-existent file failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Read returned %d entries, want 0", len(got))
	}
}

func TestChangeTracker_Cleanup(t *testing.T) {
	dir := t.TempDir()
	tracker := NewChangeTracker(dir)

	// Create the file.
	if err := tracker.Append(models.SessionChangeEntry{
		Timestamp: 1000, Tool: "Edit", FilePath: "foo.go",
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify file exists.
	filePath := filepath.Join(dir, sessionChangesFile)
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("change file should exist: %v", err)
	}

	// Cleanup.
	if err := tracker.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify file removed.
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("change file should be removed after cleanup")
	}
}

func TestChangeTracker_CleanupNonExistent(t *testing.T) {
	dir := t.TempDir()
	tracker := NewChangeTracker(dir)

	// Cleanup on non-existent file should not error.
	if err := tracker.Cleanup(); err != nil {
		t.Fatalf("Cleanup on non-existent file should not error: %v", err)
	}
}

func TestChangeTracker_AppendAutoTimestamp(t *testing.T) {
	dir := t.TempDir()
	tracker := NewChangeTracker(dir)

	// Append with zero timestamp should auto-fill.
	if err := tracker.Append(models.SessionChangeEntry{
		Tool: "Edit", FilePath: "foo.go",
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	got, err := tracker.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Read returned %d entries, want 1", len(got))
	}
	if got[0].Timestamp == 0 {
		t.Error("auto-generated timestamp should not be 0")
	}
}

func TestChangeTracker_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, sessionChangesFile)

	// Write a file with mixed valid and malformed lines.
	content := "1000|Edit|foo.go\nbadline\n2000|Write|bar.go\n||\n3000|Edit|baz.go\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	tracker := NewChangeTracker(dir)
	got, err := tracker.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Read returned %d entries, want 3 (skipping malformed)", len(got))
	}
}

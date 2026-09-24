package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChangeTracker(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "tracker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewChangeTracker(tmpDir)

	t.Run("TrackChange", func(t *testing.T) {
		err := tracker.TrackChange("file1.go", "modified")
		if err != nil {
			t.Errorf("TrackChange() error = %v", err)
		}

		err = tracker.TrackChange("file2.go", "created")
		if err != nil {
			t.Errorf("TrackChange() error = %v", err)
		}
	})

	t.Run("GetChanges", func(t *testing.T) {
		changes, err := tracker.GetChanges()
		if err != nil {
			t.Fatalf("GetChanges() error = %v", err)
		}

		if len(changes) != 2 {
			t.Errorf("GetChanges() returned %d changes, want 2", len(changes))
		}

		if changes[0].FilePath != "file1.go" {
			t.Errorf("First change FilePath = %v, want %v", changes[0].FilePath, "file1.go")
		}

		if changes[0].Operation != "modified" {
			t.Errorf("First change Operation = %v, want %v", changes[0].Operation, "modified")
		}

		if changes[1].FilePath != "file2.go" {
			t.Errorf("Second change FilePath = %v, want %v", changes[1].FilePath, "file2.go")
		}

		if changes[1].Operation != "created" {
			t.Errorf("Second change Operation = %v, want %v", changes[1].Operation, "created")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		err := tracker.Clear()
		if err != nil {
			t.Errorf("Clear() error = %v", err)
		}

		changes, err := tracker.GetChanges()
		if err != nil {
			t.Fatalf("GetChanges() error = %v", err)
		}

		if len(changes) != 0 {
			t.Errorf("GetChanges() after Clear() returned %d changes, want 0", len(changes))
		}
	})

	t.Run("GetChanges on non-existent file", func(t *testing.T) {
		tracker2 := NewChangeTracker(filepath.Join(tmpDir, "nonexistent"))
		changes, err := tracker2.GetChanges()
		if err != nil {
			t.Errorf("GetChanges() on non-existent file should not error, got: %v", err)
		}

		if len(changes) != 0 {
			t.Errorf("GetChanges() on non-existent file should return empty slice, got %d changes", len(changes))
		}
	})
}

func TestChangeTrackerMultipleOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracker-multi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := NewChangeTracker(tmpDir)

	// Track multiple changes
	operations := []struct {
		file string
		op   string
	}{
		{"main.go", "created"},
		{"main.go", "modified"},
		{"test.go", "created"},
		{"util.go", "modified"},
		{"util.go", "deleted"},
	}

	for _, op := range operations {
		if err := tracker.TrackChange(op.file, op.op); err != nil {
			t.Fatalf("TrackChange(%s, %s) error = %v", op.file, op.op, err)
		}
	}

	changes, err := tracker.GetChanges()
	if err != nil {
		t.Fatalf("GetChanges() error = %v", err)
	}

	if len(changes) != len(operations) {
		t.Errorf("GetChanges() returned %d changes, want %d", len(changes), len(operations))
	}

	// Verify each change
	for i, change := range changes {
		if change.FilePath != operations[i].file {
			t.Errorf("Change %d FilePath = %v, want %v", i, change.FilePath, operations[i].file)
		}
		if change.Operation != operations[i].op {
			t.Errorf("Change %d Operation = %v, want %v", i, change.Operation, operations[i].op)
		}
		if change.Timestamp == "" {
			t.Errorf("Change %d has empty timestamp", i)
		}
	}
}

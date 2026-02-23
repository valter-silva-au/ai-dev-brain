package hooks

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// TestProperty34_ChangeTrackerOrderPreservation verifies that change entries
// are always read back in the same order they were appended.
func TestProperty34_ChangeTrackerOrderPreservation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		tracker := NewChangeTracker(dir)

		n := rapid.IntRange(1, 20).Draw(rt, "num_entries")
		var written []models.SessionChangeEntry

		for i := 0; i < n; i++ {
			entry := models.SessionChangeEntry{
				Timestamp: int64(1000 + i),
				Tool:      rapid.SampledFrom([]string{"Edit", "Write"}).Draw(rt, "tool"),
				FilePath:  rapid.StringMatching(`[a-z/]{1,30}\.go`).Draw(rt, "path"),
			}
			if err := tracker.Append(entry); err != nil {
				rt.Fatalf("Append failed: %v", err)
			}
			written = append(written, entry)
		}

		read, err := tracker.Read()
		if err != nil {
			rt.Fatalf("Read failed: %v", err)
		}

		if len(read) != len(written) {
			rt.Fatalf("Read returned %d entries, wrote %d", len(read), len(written))
		}

		for i := range written {
			if read[i].Timestamp != written[i].Timestamp {
				rt.Fatalf("entry[%d].Timestamp = %d, want %d", i, read[i].Timestamp, written[i].Timestamp)
			}
			if read[i].Tool != written[i].Tool {
				rt.Fatalf("entry[%d].Tool = %q, want %q", i, read[i].Tool, written[i].Tool)
			}
			if read[i].FilePath != written[i].FilePath {
				rt.Fatalf("entry[%d].FilePath = %q, want %q", i, read[i].FilePath, written[i].FilePath)
			}
		}
	})
}

// TestProperty35_ChangeTrackerCleanupIdempotent verifies that calling Cleanup
// multiple times never fails.
func TestProperty35_ChangeTrackerCleanupIdempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		tracker := NewChangeTracker(dir)

		// Optionally write some entries.
		if rapid.Bool().Draw(rt, "write_first") {
			_ = tracker.Append(models.SessionChangeEntry{
				Timestamp: 1000,
				Tool:      "Edit",
				FilePath:  "foo.go",
			})
		}

		// Cleanup multiple times.
		n := rapid.IntRange(1, 5).Draw(rt, "cleanup_count")
		for i := 0; i < n; i++ {
			if err := tracker.Cleanup(); err != nil {
				rt.Fatalf("Cleanup #%d failed: %v", i+1, err)
			}
		}

		// After cleanup, read should return empty.
		entries, err := tracker.Read()
		if err != nil {
			rt.Fatalf("Read after cleanup failed: %v", err)
		}
		if len(entries) != 0 {
			rt.Fatalf("expected 0 entries after cleanup, got %d", len(entries))
		}
	})
}

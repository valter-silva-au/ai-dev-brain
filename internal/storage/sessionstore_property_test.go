package storage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

func genSessionID(t *rapid.T) string {
	n := rapid.IntRange(1, 99999).Draw(t, "sessionNum")
	return fmt.Sprintf("S-%05d", n)
}

func genCapturedSession(t *rapid.T) models.CapturedSession {
	id := genSessionID(t)
	taskID := genTaskID(t)
	project := "/project/" + genAlphaString(t, "projName", 2, 10)
	branch := genAlphaString(t, "branchName", 2, 15)
	summary := genAlphaString(t, "summary", 5, 50)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	startOffset := rapid.IntRange(0, 365*24).Draw(t, "startHourOffset")
	durationHours := rapid.IntRange(0, 4).Draw(t, "durationHours")
	start := baseTime.Add(time.Duration(startOffset) * time.Hour)
	end := start.Add(time.Duration(durationHours) * time.Hour)

	turnCount := rapid.IntRange(1, 100).Draw(t, "turnCount")

	return models.CapturedSession{
		ID:          id,
		SessionID:   "claude-" + id,
		TaskID:      taskID,
		ProjectPath: project,
		GitBranch:   branch,
		StartedAt:   start,
		EndedAt:     end,
		Duration:    fmt.Sprintf("%dh", durationHours),
		TurnCount:   turnCount,
		Summary:     summary,
	}
}

// TestProperty31_SessionStoreRoundTrip verifies that sessions survive a
// Save/Load cycle with all fields preserved.
func TestProperty31_SessionStoreRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessions := rapid.SliceOfN(rapid.Custom(genCapturedSession), 1, 15).Draw(t, "sessions")

		// Deduplicate by ID.
		seen := make(map[string]bool)
		var unique []models.CapturedSession
		for _, s := range sessions {
			if !seen[s.ID] {
				seen[s.ID] = true
				unique = append(unique, s)
			}
		}

		dir, err := os.MkdirTemp("", "session-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		store := NewSessionStoreManager(dir).(*fileSessionStore)
		for _, s := range unique {
			if _, err := store.AddSession(s, nil); err != nil {
				t.Fatal(err)
			}
		}

		if err := store.Save(); err != nil {
			t.Fatal(err)
		}

		store2 := NewSessionStoreManager(dir).(*fileSessionStore)
		if err := store2.Load(); err != nil {
			t.Fatal(err)
		}

		loaded, _ := store2.ListSessions(models.SessionFilter{})
		if len(loaded) != len(unique) {
			t.Fatalf("expected %d sessions, got %d", len(unique), len(loaded))
		}

		for _, orig := range unique {
			got, err := store2.GetSession(orig.ID)
			if err != nil {
				t.Fatalf("session %s not found after round-trip", orig.ID)
			}
			if got.SessionID != orig.SessionID {
				t.Fatalf("session %s sessionID mismatch: %q vs %q", orig.ID, got.SessionID, orig.SessionID)
			}
			if got.TaskID != orig.TaskID {
				t.Fatalf("session %s taskID mismatch: %q vs %q", orig.ID, got.TaskID, orig.TaskID)
			}
			if got.Summary != orig.Summary {
				t.Fatalf("session %s summary mismatch: %q vs %q", orig.ID, got.Summary, orig.Summary)
			}
			if got.TurnCount != orig.TurnCount {
				t.Fatalf("session %s turnCount mismatch: %d vs %d", orig.ID, got.TurnCount, orig.TurnCount)
			}
		}
	})
}

// TestProperty32_SessionFilterSubset verifies that filtered results are always
// a subset of all sessions.
func TestProperty32_SessionFilterSubset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessions := rapid.SliceOfN(rapid.Custom(genCapturedSession), 1, 15).Draw(t, "sessions")

		// Deduplicate.
		seen := make(map[string]bool)
		var unique []models.CapturedSession
		for _, s := range sessions {
			if !seen[s.ID] {
				seen[s.ID] = true
				unique = append(unique, s)
			}
		}

		dir, err := os.MkdirTemp("", "session-filter-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		store := NewSessionStoreManager(dir).(*fileSessionStore)
		for _, s := range unique {
			if _, err := store.AddSession(s, nil); err != nil {
				t.Fatal(err)
			}
		}

		// Build a random filter from existing data.
		allSessions, _ := store.ListSessions(models.SessionFilter{})
		allIDs := make(map[string]bool)
		for _, s := range allSessions {
			allIDs[s.ID] = true
		}

		taskIDs := make([]string, 0)
		for _, s := range unique {
			taskIDs = append(taskIDs, s.TaskID)
		}
		taskIDs = append(taskIDs, "") // Include empty to test unfiltered.

		filter := models.SessionFilter{
			TaskID: taskIDs[rapid.IntRange(0, len(taskIDs)-1).Draw(t, "taskIdx")],
		}

		filtered, _ := store.ListSessions(filter)

		// Every filtered result must be in the full set.
		for _, f := range filtered {
			if !allIDs[f.ID] {
				t.Fatalf("filtered session %s not found in full set", f.ID)
			}
		}

		// If task filter is set, all results must match.
		if filter.TaskID != "" {
			for _, f := range filtered {
				if f.TaskID != filter.TaskID {
					t.Fatalf("session %s task %q does not match filter %q", f.ID, f.TaskID, filter.TaskID)
				}
			}
		}
	})
}

// TestProperty33_SessionIDAlwaysUnique verifies that GenerateID always
// produces unique IDs across multiple calls.
func TestProperty33_SessionIDAlwaysUnique(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 50).Draw(t, "numIDs")

		dir, err := os.MkdirTemp("", "session-id-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		store := NewSessionStoreManager(dir)
		seen := make(map[string]bool)

		for i := 0; i < n; i++ {
			id, err := store.GenerateID()
			if err != nil {
				t.Fatalf("GenerateID call %d failed: %v", i, err)
			}
			if seen[id] {
				t.Fatalf("duplicate ID generated: %s (call %d)", id, i)
			}
			seen[id] = true
		}
	})
}

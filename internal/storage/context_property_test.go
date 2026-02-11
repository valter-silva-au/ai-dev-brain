package storage

import (
	"fmt"
	"os"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ai-dev-brain, Property 14: Context Persistence Round-Trip
func TestContextPersistenceRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := genTaskID(t)

		dir, err := os.MkdirTemp("", "context-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		mgr := NewContextManager(dir).(*fileContextManager)

		_, err = mgr.InitializeContext(taskID)
		if err != nil {
			t.Fatal(err)
		}

		// Generate random context and notes content.
		contextContent := fmt.Sprintf(`# Task Context: %s

## Summary
%s

## Current Focus
%s

## Recent Progress
- %s
- %s

## Open Questions
- [ ] %s

## Decisions Made
- %s

## Blockers
- %s

## Next Steps
- [ ] %s
`,
			taskID,
			genAlphaString(t, "summary", 5, 50),
			genAlphaString(t, "focus", 5, 50),
			genAlphaString(t, "progress1", 5, 40),
			genAlphaString(t, "progress2", 5, 40),
			genAlphaString(t, "question", 5, 40),
			genAlphaString(t, "decision", 5, 40),
			genAlphaString(t, "blocker", 5, 40),
			genAlphaString(t, "nextstep", 5, 40),
		)

		notesContent := fmt.Sprintf("# Notes: %s\n\n%s\n",
			taskID,
			genAlphaString(t, "notes", 10, 100),
		)

		err = mgr.UpdateContext(taskID, map[string]interface{}{
			"context": contextContent,
			"notes":   notesContent,
		})
		if err != nil {
			t.Fatal(err)
		}

		if err := mgr.PersistContext(taskID); err != nil {
			t.Fatal(err)
		}

		// Create a new manager to simulate a new session.
		mgr2 := NewContextManager(dir).(*fileContextManager)
		loaded, err := mgr2.LoadContext(taskID)
		if err != nil {
			t.Fatal(err)
		}

		if loaded.Context != contextContent {
			t.Fatalf("context mismatch after round-trip:\ngot:  %q\nwant: %q", loaded.Context, contextContent)
		}
		if loaded.Notes != notesContent {
			t.Fatalf("notes mismatch after round-trip:\ngot:  %q\nwant: %q", loaded.Notes, notesContent)
		}
		if loaded.TaskID != taskID {
			t.Fatalf("task ID mismatch: got %q, want %q", loaded.TaskID, taskID)
		}
	})
}

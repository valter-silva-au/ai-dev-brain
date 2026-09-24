package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTaskManager_RemoveOrphanWorktree_EmitsWorktreeRemoved pins the follow-up to
// #206: the sweepers (`adb task worktree prune`, `reconcile --prune`) used to call
// the git remover DIRECTLY from the CLI handler, so an orphan sweep removed a
// worktree and logged nothing — created/removed never balanced across a prune.
//
// The attribution question this method answers: an orphan has, by definition, no
// task to hang the event on. It still emits `worktree.removed` (the removal is the
// same kind of fact) with an EMPTY task_id and a `reason` naming why the id is
// empty, rather than a new event type.
func TestTaskManager_RemoveOrphanWorktree_EmitsWorktreeRemoved(t *testing.T) {
	tm, _, eventLogger, _, worktreeRemover, tempDir := createTestTaskManager(t)

	orphan := filepath.Join(tempDir, "work", "github.com", "acme", "thing", "TASK-00099-gone")

	if err := tm.RemoveOrphanWorktree(orphan, false); err != nil {
		t.Fatalf("RemoveOrphanWorktree: %v", err)
	}

	if len(worktreeRemover.removed) != 1 || worktreeRemover.removed[0] != orphan {
		t.Fatalf("expected the orphan path to reach the remover, got %v", worktreeRemover.removed)
	}
	if len(worktreeRemover.removedForce) != 1 || worktreeRemover.removedForce[0] {
		t.Errorf("expected force=false propagated (the #207 dirty guard must hold), got %v", worktreeRemover.removedForce)
	}

	if len(eventLogger.events) != 1 {
		t.Fatalf("expected exactly 1 event (worktree.removed), got %v", eventLogger.events)
	}
	assertEventWithPath(t, eventLogger.events, "worktree.removed", orphan)

	e := eventLogger.events[0]
	// An empty string, not an absent key: every worktree.removed event then has
	// one shape, and every reader in the tree already treats "" as
	// "unattributable" (see observability/sessiondigest.go).
	if got := eventData(e, "task_id"); got != "" {
		t.Errorf("orphan sweep task_id = %#v, want %q", got, "")
	}
	if got := eventData(e, "reason"); got != "orphaned" {
		t.Errorf("orphan sweep reason = %#v, want %q", got, "orphaned")
	}
}

// TestTaskManager_RemoveOrphanWorktree_ForceAndFailure covers the two remaining
// paths: --force reaches the remover, and a refused removal (the #207 dirty
// guard) returns the error WITHOUT logging a removal that did not happen.
func TestTaskManager_RemoveOrphanWorktree_ForceAndFailure(t *testing.T) {
	t.Run("force propagates", func(t *testing.T) {
		tm, _, _, _, worktreeRemover, tempDir := createTestTaskManager(t)
		if err := tm.RemoveOrphanWorktree(filepath.Join(tempDir, "work", "orphan"), true); err != nil {
			t.Fatalf("RemoveOrphanWorktree: %v", err)
		}
		if len(worktreeRemover.removedForce) != 1 || !worktreeRemover.removedForce[0] {
			t.Errorf("expected force=true propagated, got %v", worktreeRemover.removedForce)
		}
	})

	t.Run("a refused removal logs nothing", func(t *testing.T) {
		tm, _, eventLogger, _, worktreeRemover, tempDir := createTestTaskManager(t)
		worktreeRemover.removeErr = os.ErrPermission
		err := tm.RemoveOrphanWorktree(filepath.Join(tempDir, "work", "orphan"), false)
		if err == nil {
			t.Fatal("expected an error when the remover refuses")
		}
		if len(eventLogger.events) != 0 {
			t.Errorf("expected no event for a removal that did not happen, got %v", eventLogger.events)
		}
	})

	t.Run("an empty path is refused", func(t *testing.T) {
		tm, _, eventLogger, _, _, _ := createTestTaskManager(t)
		if err := tm.RemoveOrphanWorktree("", false); err == nil {
			t.Fatal("expected an error for an empty worktree path")
		}
		if len(eventLogger.events) != 0 {
			t.Errorf("expected no event, got %v", eventLogger.events)
		}
	})
}

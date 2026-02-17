package core

import (
	"os"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// archivableStatuses are the statuses from which a task can be archived.
var archivableStatuses = []models.TaskStatus{
	models.StatusBacklog,
	models.StatusInProgress,
	models.StatusBlocked,
	models.StatusReview,
	models.StatusDone,
}

func statusGenerator() *rapid.Generator[models.TaskStatus] {
	return rapid.Custom(func(t *rapid.T) models.TaskStatus {
		idx := rapid.IntRange(0, len(archivableStatuses)-1).Draw(t, "statusIdx")
		return archivableStatuses[idx]
	})
}

// Feature: ai-dev-brain, Property 4: Archive/Unarchive Round-Trip
// For any task that is archived and then unarchived, the task SHALL be
// restored to a resumable state with its previous status preserved.
func TestProperty_ArchiveUnarchiveRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskType := taskTypeGenerator().Draw(rt, "taskType")
		preArchiveStatus := statusGenerator().Draw(rt, "preArchiveStatus")
		branchName := rapid.StringMatching(`[a-z]{3,10}/[a-z]{3,10}`).Draw(rt, "branchName")

		dir, err := os.MkdirTemp("", "taskmanager-prop4-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		idGen := NewTaskIDGenerator(dir, "TASK", 5)
		tmplMgr := NewTemplateManager(dir)
		bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)
		backlog := newInMemoryBacklog()
		ctxStore := newMockContextStore()
		mgr := NewTaskManager(dir, bs, backlog, ctxStore, nil, nil)

		// Create a task.
		task, err := mgr.CreateTask(taskType, branchName, "", CreateTaskOpts{})
		if err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		// Set to the chosen pre-archive status.
		if err := mgr.UpdateTaskStatus(task.ID, preArchiveStatus); err != nil {
			t.Fatalf("UpdateTaskStatus to %s failed: %v", preArchiveStatus, err)
		}

		// Verify the status was set.
		beforeArchive, err := mgr.GetTask(task.ID)
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if beforeArchive.Status != preArchiveStatus {
			t.Fatalf("expected status %s before archive, got %s", preArchiveStatus, beforeArchive.Status)
		}

		// Archive the task.
		handoff, err := mgr.ArchiveTask(task.ID)
		if err != nil {
			t.Fatalf("ArchiveTask failed: %v", err)
		}

		// Verify the handoff document was created.
		if handoff == nil {
			t.Fatal("handoff document must not be nil")
		}
		if handoff.TaskID != task.ID {
			t.Fatalf("handoff TaskID should be %s, got %s", task.ID, handoff.TaskID)
		}

		// Verify task is archived.
		archived, err := mgr.GetTask(task.ID)
		if err != nil {
			t.Fatalf("GetTask after archive failed: %v", err)
		}
		if archived.Status != models.StatusArchived {
			t.Fatalf("task should be archived, got %s", archived.Status)
		}

		// Unarchive the task.
		restored, err := mgr.UnarchiveTask(task.ID)
		if err != nil {
			t.Fatalf("UnarchiveTask failed: %v", err)
		}

		// Property: restored status must match pre-archive status.
		if restored.Status != preArchiveStatus {
			t.Fatalf("restored status should be %s, got %s", preArchiveStatus, restored.Status)
		}

		// Verify through GetTask as well.
		final, err := mgr.GetTask(task.ID)
		if err != nil {
			t.Fatalf("GetTask after unarchive failed: %v", err)
		}
		if final.Status != preArchiveStatus {
			t.Fatalf("final status should be %s, got %s", preArchiveStatus, final.Status)
		}
	})
}

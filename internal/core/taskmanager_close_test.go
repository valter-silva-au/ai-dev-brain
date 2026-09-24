package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestTaskManager_Close pins the one method both `adb task close` and the MCP
// server's adb_task_close route through. Before it existed each caller wrote
// UpdateStatus(id, done) itself, which is how "closing a task" acquires two
// slightly different meanings.
func TestTaskManager_Close(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{Title: "Close me", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	eventLogger.events = []map[string]interface{}{}

	if err := tm.Close(task.ID); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stored, err := backlogStore.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if stored.Status != models.TaskStatusDone {
		t.Errorf("status = %s, want done", stored.Status)
	}

	// Close must be indistinguishable from UpdateStatus(done) on the wire: no
	// new event type, so every existing consumer (metrics, alerts, the digest)
	// keeps working without being taught about a "close".
	if len(eventLogger.events) != 1 {
		t.Fatalf("events = %d, want 1", len(eventLogger.events))
	}
	if got := eventLogger.events[0]["type"]; got != "task.status_changed" {
		t.Errorf("event type = %v, want task.status_changed", got)
	}
}

// TestTaskManager_Close_UnknownTask keeps the error path honest — an unknown id
// must fail rather than silently succeed, because `task close` on a typo would
// otherwise report a ticket closed that was never touched.
func TestTaskManager_Close_UnknownTask(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)

	if err := tm.Close("TASK-99999"); err == nil {
		t.Fatal("Close on an unknown task returned nil, want an error")
	}
}

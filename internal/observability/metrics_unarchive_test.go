package observability

import (
	"path/filepath"
	"testing"
)

// TestComputeMetrics_UnarchiveRestoresStatusBucket pins the sibling half of the
// #154 archived-task fix. core.TaskManager.Unarchive sets the task's status back
// to backlog but emits ONLY "task.unarchived" — no task.status_changed — so the
// fold has to re-enter the task itself. While task.unarchived was an unhandled
// case, `task.archived` decremented the bucket and nothing ever put the task
// back: `adb metrics` under-reported the backlog by one per unarchived task,
// permanently.
func TestComputeMetrics_UnarchiveRestoresStatusBucket(t *testing.T) {
	el := NewEventLog(filepath.Join(t.TempDir(), "events.jsonl"))
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001", "type": "feat", "status": "backlog",
	})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-001"})
	el.Log(EventTaskUnarchived, map[string]interface{}{"task_id": "TASK-001"})

	m, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if got := m.TasksByStatus["backlog"]; got != 1 {
		t.Errorf("TasksByStatus[backlog] = %d, want 1 (unarchive returns the task to backlog); full map: %v", got, m.TasksByStatus)
	}
}

// TestComputeMetrics_UnarchivePrefersPayloadStatus keeps the fallback honest: if
// a future emit site starts carrying a "status" key, the fold must use it rather
// than assuming backlog.
func TestComputeMetrics_UnarchivePrefersPayloadStatus(t *testing.T) {
	el := NewEventLog(filepath.Join(t.TempDir(), "events.jsonl"))
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-002", "type": "feat", "status": "backlog",
	})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-002"})
	el.Log(EventTaskUnarchived, map[string]interface{}{"task_id": "TASK-002", "status": "review"})

	m, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if got := m.TasksByStatus["review"]; got != 1 {
		t.Errorf("TasksByStatus[review] = %d, want 1; full map: %v", got, m.TasksByStatus)
	}
	if got := m.TasksByStatus["backlog"]; got != 0 {
		t.Errorf("TasksByStatus[backlog] = %d, want 0 (the payload named review); full map: %v", got, m.TasksByStatus)
	}
}

// TestComputeMetrics_UnarchiveIsNotDoubleCounted guards the other direction: an
// unarchive for a task that is already live must not inflate the tally. (Nothing
// emits that today, but the fold reads a log it does not control.)
func TestComputeMetrics_UnarchiveIsNotDoubleCounted(t *testing.T) {
	el := NewEventLog(filepath.Join(t.TempDir(), "events.jsonl"))
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-003", "type": "feat", "status": "backlog",
	})
	el.Log(EventTaskUnarchived, map[string]interface{}{"task_id": "TASK-003"})

	m, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if got := m.TasksByStatus["backlog"]; got != 1 {
		t.Errorf("TasksByStatus[backlog] = %d, want 1 (no double count); full map: %v", got, m.TasksByStatus)
	}
}

// TestGetTasksInStatus_UnarchiveRestoresTask is the alerting-facing half of the
// same defect. AlertEvaluator consumes GetTasksInStatus, and task.archived
// deletes the task from the map so retired tasks stop firing alerts (#154). With
// task.unarchived unhandled, an unarchived task stayed invisible forever — so
// backlog_too_large silently under-counted and a re-opened ticket could never
// raise a stale/blocked alert again.
func TestGetTasksInStatus_UnarchiveRestoresTask(t *testing.T) {
	el := NewEventLog(filepath.Join(t.TempDir(), "events.jsonl"))
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-004", "type": "feat", "status": "backlog",
	})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-004"})
	el.Log(EventTaskUnarchived, map[string]interface{}{"task_id": "TASK-004"})

	ids, err := mc.GetTasksInStatus("backlog")
	if err != nil {
		t.Fatalf("GetTasksInStatus: %v", err)
	}
	if len(ids) != 1 || ids[0] != "TASK-004" {
		t.Errorf("GetTasksInStatus(backlog) = %v, want [TASK-004]", ids)
	}
}

// TestGetTasksInStatus_ArchivedStaysGone is the negative control for the fix
// above: the #154 behaviour it builds on must survive.
func TestGetTasksInStatus_ArchivedStaysGone(t *testing.T) {
	el := NewEventLog(filepath.Join(t.TempDir(), "events.jsonl"))
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-005", "type": "feat", "status": "backlog",
	})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-005"})

	ids, err := mc.GetTasksInStatus("backlog")
	if err != nil {
		t.Fatalf("GetTasksInStatus: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("GetTasksInStatus(backlog) = %v, want empty (archived tasks leave the live set)", ids)
	}
}

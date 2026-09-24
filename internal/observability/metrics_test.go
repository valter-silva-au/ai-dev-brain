package observability

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMetricsCalculator_ComputeMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Log various events
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"type":    "feat",
		"status":  "backlog",
	})

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-002",
		"type":    "bug",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	el.Log(EventTaskCompleted, map[string]interface{}{
		"task_id": "TASK-001",
	})

	el.Log(EventAgentSessionStarted, map[string]interface{}{
		"session_id": "session-1",
	})

	el.Log(EventWorktreeCreated, map[string]interface{}{
		"path": "/tmp/worktree1",
	})

	// Compute metrics
	metrics, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("Failed to compute metrics: %v", err)
	}

	// Verify metrics
	if metrics.TasksCreated != 2 {
		t.Errorf("Expected 2 tasks created, got %d", metrics.TasksCreated)
	}

	if metrics.TasksCompleted != 1 {
		t.Errorf("Expected 1 task completed, got %d", metrics.TasksCompleted)
	}

	if metrics.TasksByType["feat"] != 1 {
		t.Errorf("Expected 1 feat task, got %d", metrics.TasksByType["feat"])
	}

	if metrics.TasksByType["bug"] != 1 {
		t.Errorf("Expected 1 bug task, got %d", metrics.TasksByType["bug"])
	}

	if metrics.AgentSessions != 1 {
		t.Errorf("Expected 1 agent session, got %d", metrics.AgentSessions)
	}

	if metrics.WorktreesCreated != 1 {
		t.Errorf("Expected 1 worktree created, got %d", metrics.WorktreesCreated)
	}

	// Verify status counts (backlog should have 1, in_progress should be 0 after completion)
	if metrics.TasksByStatus["backlog"] != 1 {
		t.Errorf("Expected 1 task in backlog status, got %d", metrics.TasksByStatus["backlog"])
	}

	if metrics.TasksByStatus["in_progress"] != 1 {
		t.Errorf("Expected 1 task in in_progress status, got %d", metrics.TasksByStatus["in_progress"])
	}
}

func TestMetricsCalculator_StatusHistory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create task and change status multiple times
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "in_progress",
		"new_status": "blocked",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "blocked",
		"new_status": "in_progress",
	})

	// Compute metrics
	metrics, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("Failed to compute metrics: %v", err)
	}

	// Verify status history
	history := metrics.TaskStatusHistory["TASK-001"]
	if len(history) != 3 {
		t.Fatalf("Expected 3 status changes, got %d", len(history))
	}

	expectedChanges := []struct {
		old string
		new string
	}{
		{"backlog", "in_progress"},
		{"in_progress", "blocked"},
		{"blocked", "in_progress"},
	}

	for i, expected := range expectedChanges {
		if history[i].OldStatus != expected.old {
			t.Errorf("Change %d: expected old status %s, got %s", i, expected.old, history[i].OldStatus)
		}
		if history[i].NewStatus != expected.new {
			t.Errorf("Change %d: expected new status %s, got %s", i, expected.new, history[i].NewStatus)
		}
	}
}

func TestMetricsCalculator_GetTaskDuration(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create task with initial status
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Change status
	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	// Wait a bit more
	time.Sleep(10 * time.Millisecond)

	// Get duration in current status
	duration, err := mc.GetTaskDuration("TASK-001", "in_progress")
	if err != nil {
		t.Fatalf("Failed to get task duration: %v", err)
	}

	if duration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", duration)
	}

	// Duration for old status should be 0
	duration, err = mc.GetTaskDuration("TASK-001", "backlog")
	if err != nil {
		t.Fatalf("Failed to get task duration: %v", err)
	}

	if duration != 0 {
		t.Errorf("Expected 0 duration for old status, got %v", duration)
	}
}

func TestMetricsCalculator_GetTasksInStatus(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create multiple tasks with different statuses
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-002",
		"status":  "backlog",
	})

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-003",
		"status":  "backlog",
	})

	// Move one task to in_progress
	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	// Move another to blocked
	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-002",
		"old_status": "backlog",
		"new_status": "blocked",
	})

	// Get tasks in backlog
	backlogTasks, err := mc.GetTasksInStatus("backlog")
	if err != nil {
		t.Fatalf("Failed to get tasks in status: %v", err)
	}

	if len(backlogTasks) != 1 {
		t.Errorf("Expected 1 task in backlog, got %d", len(backlogTasks))
	}

	// Get tasks in progress
	inProgressTasks, err := mc.GetTasksInStatus("in_progress")
	if err != nil {
		t.Fatalf("Failed to get tasks in status: %v", err)
	}

	if len(inProgressTasks) != 1 {
		t.Errorf("Expected 1 task in progress, got %d", len(inProgressTasks))
	}

	// Get blocked tasks
	blockedTasks, err := mc.GetTasksInStatus("blocked")
	if err != nil {
		t.Fatalf("Failed to get tasks in status: %v", err)
	}

	if len(blockedTasks) != 1 {
		t.Errorf("Expected 1 blocked task, got %d", len(blockedTasks))
	}
}

func TestMetricsCalculator_EmptyLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Compute metrics from empty log
	metrics, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("Failed to compute metrics: %v", err)
	}

	if metrics.TasksCreated != 0 {
		t.Errorf("Expected 0 tasks created, got %d", metrics.TasksCreated)
	}

	if metrics.TasksCompleted != 0 {
		t.Errorf("Expected 0 tasks completed, got %d", metrics.TasksCompleted)
	}

	if metrics.AgentSessions != 0 {
		t.Errorf("Expected 0 agent sessions, got %d", metrics.AgentSessions)
	}
}

func TestMetricsCalculator_AllEventTypes(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Log all event types
	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-001"})
	el.Log(EventTaskCompleted, map[string]interface{}{"task_id": "TASK-001"})
	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "done",
	})
	el.Log(EventAgentSessionStarted, map[string]interface{}{"session": "1"})
	el.Log(EventKnowledgeExtracted, map[string]interface{}{"item": "1"})
	el.Log(EventWorktreeCreated, map[string]interface{}{"path": "/tmp/wt1"})
	el.Log(EventWorktreeRemoved, map[string]interface{}{"path": "/tmp/wt1"})

	// Compute metrics
	metrics, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("Failed to compute metrics: %v", err)
	}

	// Verify all counters
	if metrics.TasksCreated != 1 {
		t.Errorf("Expected 1 task created, got %d", metrics.TasksCreated)
	}

	if metrics.TasksCompleted != 1 {
		t.Errorf("Expected 1 task completed, got %d", metrics.TasksCompleted)
	}

	if metrics.AgentSessions != 1 {
		t.Errorf("Expected 1 agent session, got %d", metrics.AgentSessions)
	}

	if metrics.KnowledgeExtracts != 1 {
		t.Errorf("Expected 1 knowledge extract, got %d", metrics.KnowledgeExtracts)
	}

	if metrics.WorktreesCreated != 1 {
		t.Errorf("Expected 1 worktree created, got %d", metrics.WorktreesCreated)
	}

	if metrics.WorktreesRemoved != 1 {
		t.Errorf("Expected 1 worktree removed, got %d", metrics.WorktreesRemoved)
	}
}

// TestComputeMetricsSince_WindowsEvents verifies that ComputeMetricsSince tallies
// ONLY events within [cutoff, now], not the whole log. The old --since path
// (metrics.go) replayed every event and merely blanked the result when the single
// most-recent event predated the cutoff — so `adb metrics --since 1h` returned
// all-time counts whenever any event was recent (#153).
func TestComputeMetricsSince_WindowsEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")
	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	old := time.Now().UTC().Add(-100 * 24 * time.Hour) // ~100 days ago
	recent := time.Now().UTC().Add(-30 * time.Minute)

	// Five old task.created, one recent.
	for i := 0; i < 5; i++ {
		if err := el.appendEvent(Event{Timestamp: old, Type: EventTaskCreated, Data: map[string]interface{}{
			"task_id": "OLD", "type": "feat", "status": "backlog",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := el.appendEvent(Event{Timestamp: recent, Type: EventTaskCreated, Data: map[string]interface{}{
		"task_id": "NEW", "type": "fix", "status": "backlog",
	}}); err != nil {
		t.Fatal(err)
	}

	// Window = last hour: must see ONLY the one recent fix task.
	cutoff := time.Now().UTC().Add(-1 * time.Hour)
	m, err := mc.ComputeMetricsSince(cutoff)
	if err != nil {
		t.Fatalf("ComputeMetricsSince: %v", err)
	}
	if m.TasksCreated != 1 {
		t.Errorf("windowed TasksCreated = %d, want 1 (only the recent event)", m.TasksCreated)
	}
	if m.TasksByType["fix"] != 1 || m.TasksByType["feat"] != 0 {
		t.Errorf("windowed TasksByType = %v, want {fix:1}", m.TasksByType)
	}

	// A zero cutoff means "no window" — ComputeMetrics counts all six.
	all, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if all.TasksCreated != 6 {
		t.Errorf("unwindowed TasksCreated = %d, want 6", all.TasksCreated)
	}
}

// TestComputeMetrics_TerminalEventsClearStatus verifies that task.archived and
// task.deleted remove a task from the live TasksByStatus tally. Before the fix,
// ComputeMetrics only handled created + status_changed, so an archived/deleted
// task stayed counted in its last live status forever (#154).
func TestComputeMetrics_TerminalEventsClearStatus(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")
	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Task A: created → review → archived (should NOT remain in review).
	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-A", "type": "feat", "status": "backlog"})
	el.Log(EventTaskStatusChanged, map[string]interface{}{"task_id": "TASK-A", "old_status": "backlog", "new_status": "review"})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-A"})

	// Task B: created → deleted (should NOT remain in backlog).
	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-B", "type": "fix", "status": "backlog"})
	el.Log(EventTaskDeleted, map[string]interface{}{"task_id": "TASK-B"})

	m, err := mc.ComputeMetrics()
	if err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	if got := m.TasksByStatus["review"]; got != 0 {
		t.Errorf("archived task still counted in review: TasksByStatus[review] = %d, want 0", got)
	}
	if got := m.TasksByStatus["backlog"]; got != 0 {
		t.Errorf("deleted task still counted in backlog: TasksByStatus[backlog] = %d, want 0", got)
	}
}

// TestGetTasksInStatus_ExcludesTerminal verifies archived/deleted tasks are no
// longer returned by GetTasksInStatus, which AlertEvaluator uses — otherwise a
// retired task keeps firing "blocked for Nh" alerts forever (#154).
func TestGetTasksInStatus_ExcludesTerminal(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")
	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-A", "status": "backlog"})
	el.Log(EventTaskStatusChanged, map[string]interface{}{"task_id": "TASK-A", "old_status": "backlog", "new_status": "blocked"})
	el.Log(EventTaskArchived, map[string]interface{}{"task_id": "TASK-A"})

	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-B", "status": "backlog"})
	el.Log(EventTaskStatusChanged, map[string]interface{}{"task_id": "TASK-B", "old_status": "backlog", "new_status": "review"})
	el.Log(EventTaskDeleted, map[string]interface{}{"task_id": "TASK-B"})

	blocked, err := mc.GetTasksInStatus("blocked")
	if err != nil {
		t.Fatalf("GetTasksInStatus: %v", err)
	}
	if len(blocked) != 0 {
		t.Errorf("archived task still reported blocked: %v", blocked)
	}
	review, err := mc.GetTasksInStatus("review")
	if err != nil {
		t.Fatalf("GetTasksInStatus: %v", err)
	}
	if len(review) != 0 {
		t.Errorf("deleted task still reported in review: %v", review)
	}
}

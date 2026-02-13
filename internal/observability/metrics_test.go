package observability

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMetricsCalculator_Calculate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	base := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			Time:    base,
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": "TASK-00001", "type": "feat"},
		},
		{
			Time:    base.Add(time.Hour),
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": "TASK-00002", "type": "bug"},
		},
		{
			Time:    base.Add(2 * time.Hour),
			Level:   "INFO",
			Type:    "task.status_changed",
			Message: "task status changed",
			Data:    map[string]any{"task_id": "TASK-00001", "new_status": "in_progress"},
		},
		{
			Time:    base.Add(3 * time.Hour),
			Level:   "INFO",
			Type:    "task.completed",
			Message: "task completed",
			Data:    map[string]any{"task_id": "TASK-00001"},
		},
		{
			Time:    base.Add(4 * time.Hour),
			Level:   "INFO",
			Type:    "agent.session_started",
			Message: "agent session started",
		},
		{
			Time:    base.Add(5 * time.Hour),
			Level:   "INFO",
			Type:    "knowledge.extracted",
			Message: "knowledge extracted",
			Data:    map[string]any{"task_id": "TASK-00001"},
		},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	calc := NewMetricsCalculator(log)
	m, err := calc.Calculate(base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("calculating metrics: %v", err)
	}

	if m.TasksCreated != 2 {
		t.Errorf("expected 2 tasks created, got %d", m.TasksCreated)
	}
	if m.TasksCompleted != 1 {
		t.Errorf("expected 1 task completed, got %d", m.TasksCompleted)
	}
	if m.AgentSessions != 1 {
		t.Errorf("expected 1 agent session, got %d", m.AgentSessions)
	}
	if m.KnowledgeExtracted != 1 {
		t.Errorf("expected 1 knowledge extracted, got %d", m.KnowledgeExtracted)
	}
	if m.EventCount != 6 {
		t.Errorf("expected 6 events, got %d", m.EventCount)
	}
	if m.TasksByType["feat"] != 1 {
		t.Errorf("expected 1 feat task, got %d", m.TasksByType["feat"])
	}
	if m.TasksByType["bug"] != 1 {
		t.Errorf("expected 1 bug task, got %d", m.TasksByType["bug"])
	}
	if m.TasksByStatus["in_progress"] != 1 {
		t.Errorf("expected 1 in_progress status change, got %d", m.TasksByStatus["in_progress"])
	}
	if m.OldestEvent == nil || !m.OldestEvent.Equal(base) {
		t.Errorf("expected oldest event at %v, got %v", base, m.OldestEvent)
	}
	expectedNewest := base.Add(5 * time.Hour)
	if m.NewestEvent == nil || !m.NewestEvent.Equal(expectedNewest) {
		t.Errorf("expected newest event at %v, got %v", expectedNewest, m.NewestEvent)
	}
}

func TestMetricsCalculator_EmptyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	calc := NewMetricsCalculator(log)
	m, err := calc.Calculate(time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("calculating metrics: %v", err)
	}

	if m.TasksCreated != 0 {
		t.Errorf("expected 0 tasks created, got %d", m.TasksCreated)
	}
	if m.EventCount != 0 {
		t.Errorf("expected 0 events, got %d", m.EventCount)
	}
	if m.OldestEvent != nil {
		t.Errorf("expected nil oldest event, got %v", m.OldestEvent)
	}
}

func TestMetricsCalculator_FiltersBySince(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	base := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{Time: base, Level: "INFO", Type: "task.created", Message: "old task", Data: map[string]any{"task_id": "TASK-00001", "type": "feat"}},
		{Time: base.Add(48 * time.Hour), Level: "INFO", Type: "task.created", Message: "new task", Data: map[string]any{"task_id": "TASK-00002", "type": "bug"}},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	calc := NewMetricsCalculator(log)
	m, err := calc.Calculate(base.Add(24 * time.Hour))
	if err != nil {
		t.Fatalf("calculating metrics: %v", err)
	}

	if m.TasksCreated != 1 {
		t.Errorf("expected 1 task created after since filter, got %d", m.TasksCreated)
	}
	if m.EventCount != 1 {
		t.Errorf("expected 1 event after since filter, got %d", m.EventCount)
	}
}

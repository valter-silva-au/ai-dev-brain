package observability

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAlertEngine_BlockedTaskAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	// A task blocked 48 hours ago should trigger an alert with 24h threshold.
	blockedTime := time.Now().UTC().Add(-48 * time.Hour)
	event := Event{
		Time:    blockedTime,
		Level:   "WARN",
		Type:    "task.status_changed",
		Message: "task blocked",
		Data:    map[string]any{"task_id": "TASK-00001", "new_status": "blocked"},
	}
	if err := log.Write(event); err != nil {
		t.Fatalf("writing event: %v", err)
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Condition == "task_blocked_too_long" && a.ID == "blocked-TASK-00001" {
			found = true
			if a.Severity != SeverityHigh {
				t.Errorf("expected high severity, got %s", a.Severity)
			}
		}
	}

	if !found {
		t.Error("expected blocked task alert but none found")
	}
}

func TestAlertEngine_NoBlockedAlertWithinThreshold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	// A task blocked 1 hour ago should not trigger with 24h threshold.
	blockedTime := time.Now().UTC().Add(-time.Hour)
	event := Event{
		Time:    blockedTime,
		Level:   "WARN",
		Type:    "task.status_changed",
		Message: "task blocked",
		Data:    map[string]any{"task_id": "TASK-00001", "new_status": "blocked"},
	}
	if err := log.Write(event); err != nil {
		t.Fatalf("writing event: %v", err)
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	for _, a := range alerts {
		if a.Condition == "task_blocked_too_long" {
			t.Error("did not expect blocked alert within threshold")
		}
	}
}

func TestAlertEngine_StaleTaskAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	// A task that went in_progress 5 days ago with no further activity.
	staleTime := time.Now().UTC().Add(-5 * 24 * time.Hour)
	event := Event{
		Time:    staleTime,
		Level:   "INFO",
		Type:    "task.status_changed",
		Message: "task in progress",
		Data:    map[string]any{"task_id": "TASK-00002", "new_status": "in_progress"},
	}
	if err := log.Write(event); err != nil {
		t.Fatalf("writing event: %v", err)
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Condition == "task_stale" && a.ID == "stale-TASK-00002" {
			found = true
			if a.Severity != SeverityMedium {
				t.Errorf("expected medium severity, got %s", a.Severity)
			}
		}
	}

	if !found {
		t.Error("expected stale task alert but none found")
	}
}

func TestAlertEngine_ReviewTooLongAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	// A task in review for 7 days should trigger with 5-day threshold.
	reviewTime := time.Now().UTC().Add(-7 * 24 * time.Hour)
	event := Event{
		Time:    reviewTime,
		Level:   "INFO",
		Type:    "task.status_changed",
		Message: "task in review",
		Data:    map[string]any{"task_id": "TASK-00003", "new_status": "review"},
	}
	if err := log.Write(event); err != nil {
		t.Fatalf("writing event: %v", err)
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Condition == "review_too_long" && a.ID == "review-TASK-00003" {
			found = true
		}
	}

	if !found {
		t.Error("expected review too long alert but none found")
	}
}

func TestAlertEngine_BacklogSizeAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	now := time.Now().UTC()
	// Create 12 tasks (exceeds MaxBacklogSize of 10).
	for i := 0; i < 12; i++ {
		event := Event{
			Time:    now.Add(time.Duration(i) * time.Second),
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": "TASK-" + string(rune('A'+i))},
		}
		if err := log.Write(event); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Condition == "backlog_too_large" {
			found = true
			if a.Severity != SeverityLow {
				t.Errorf("expected low severity, got %s", a.Severity)
			}
		}
	}

	if !found {
		t.Error("expected backlog size alert but none found")
	}
}

func TestAlertEngine_NoAlertsOnCleanState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts on clean state, got %d", len(alerts))
	}
}

func TestAlertEngine_TaskUnblockedDoesNotAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	// Task was blocked 48 hours ago but then moved to in_progress.
	blockedTime := time.Now().UTC().Add(-48 * time.Hour)
	unblockedTime := time.Now().UTC().Add(-1 * time.Hour)

	events := []Event{
		{
			Time:    blockedTime,
			Level:   "WARN",
			Type:    "task.status_changed",
			Message: "task blocked",
			Data:    map[string]any{"task_id": "TASK-00001", "new_status": "blocked"},
		},
		{
			Time:    unblockedTime,
			Level:   "INFO",
			Type:    "task.status_changed",
			Message: "task unblocked",
			Data:    map[string]any{"task_id": "TASK-00001", "new_status": "in_progress"},
		},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	engine := NewAlertEngine(log, DefaultAlertThresholds())
	alerts, err := engine.Evaluate()
	if err != nil {
		t.Fatalf("evaluating alerts: %v", err)
	}

	for _, a := range alerts {
		if a.Condition == "task_blocked_too_long" && a.ID == "blocked-TASK-00001" {
			t.Error("task was unblocked, should not trigger blocked alert")
		}
	}
}

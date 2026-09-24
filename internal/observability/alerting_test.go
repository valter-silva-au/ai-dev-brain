package observability

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultAlertConfig(t *testing.T) {
	config := DefaultAlertConfig()

	if len(config.Thresholds) != 4 {
		t.Errorf("Expected 4 default thresholds, got %d", len(config.Thresholds))
	}

	// Verify task_blocked_too_long
	threshold := config.GetThreshold(AlertTaskBlockedTooLong)
	if threshold == nil {
		t.Fatal("Expected task_blocked_too_long threshold to exist")
	}
	if threshold.Severity != AlertSeverityHigh {
		t.Errorf("Expected High severity, got %s", threshold.Severity)
	}
	if threshold.Duration != 24*time.Hour {
		t.Errorf("Expected 24h duration, got %v", threshold.Duration)
	}

	// Verify task_stale
	threshold = config.GetThreshold(AlertTaskStale)
	if threshold == nil {
		t.Fatal("Expected task_stale threshold to exist")
	}
	if threshold.Severity != AlertSeverityMedium {
		t.Errorf("Expected Medium severity, got %s", threshold.Severity)
	}
	if threshold.Duration != 3*24*time.Hour {
		t.Errorf("Expected 3d duration, got %v", threshold.Duration)
	}

	// Verify review_too_long
	threshold = config.GetThreshold(AlertReviewTooLong)
	if threshold == nil {
		t.Fatal("Expected review_too_long threshold to exist")
	}
	if threshold.Severity != AlertSeverityMedium {
		t.Errorf("Expected Medium severity, got %s", threshold.Severity)
	}
	if threshold.Duration != 5*24*time.Hour {
		t.Errorf("Expected 5d duration, got %v", threshold.Duration)
	}

	// Verify backlog_too_large
	threshold = config.GetThreshold(AlertBacklogTooLarge)
	if threshold == nil {
		t.Fatal("Expected backlog_too_large threshold to exist")
	}
	if threshold.Severity != AlertSeverityLow {
		t.Errorf("Expected Low severity, got %s", threshold.Severity)
	}
	if threshold.Count != 10 {
		t.Errorf("Expected count 10, got %d", threshold.Count)
	}
}

func TestAlertConfig_SetThreshold(t *testing.T) {
	config := DefaultAlertConfig()

	// Modify existing threshold
	config.SetThreshold(AlertThreshold{
		Type:     AlertTaskBlockedTooLong,
		Severity: AlertSeverityMedium,
		Duration: 48 * time.Hour,
	})

	threshold := config.GetThreshold(AlertTaskBlockedTooLong)
	if threshold.Duration != 48*time.Hour {
		t.Errorf("Expected 48h duration, got %v", threshold.Duration)
	}
	if threshold.Severity != AlertSeverityMedium {
		t.Errorf("Expected Medium severity, got %s", threshold.Severity)
	}

	// Add new threshold
	config.SetThreshold(AlertThreshold{
		Type:     "custom_alert",
		Severity: AlertSeverityHigh,
		Duration: 1 * time.Hour,
	})

	if len(config.Thresholds) != 5 {
		t.Errorf("Expected 5 thresholds after adding custom, got %d", len(config.Thresholds))
	}
}

func TestAlertEvaluator_BlockedTooLong(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create custom config with short threshold for testing
	config := &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertTaskBlockedTooLong,
				Severity: AlertSeverityHigh,
				Duration: 50 * time.Millisecond,
			},
		},
	}

	ae := NewAlertEvaluator(config, mc)

	// Create task and block it
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "blocked",
	})

	// Wait for threshold
	time.Sleep(100 * time.Millisecond)

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have one blocked alert
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Type != AlertTaskBlockedTooLong {
		t.Errorf("Expected alert type %s, got %s", AlertTaskBlockedTooLong, alerts[0].Type)
	}

	if alerts[0].Severity != AlertSeverityHigh {
		t.Errorf("Expected High severity, got %s", alerts[0].Severity)
	}

	if alerts[0].TaskID != "TASK-001" {
		t.Errorf("Expected task ID TASK-001, got %s", alerts[0].TaskID)
	}
}

func TestAlertEvaluator_TaskStale(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create custom config with short threshold
	config := &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertTaskStale,
				Severity: AlertSeverityMedium,
				Duration: 50 * time.Millisecond,
			},
		},
	}

	ae := NewAlertEvaluator(config, mc)

	// Create task in progress
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	// Wait for threshold
	time.Sleep(100 * time.Millisecond)

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have one stale alert
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Type != AlertTaskStale {
		t.Errorf("Expected alert type %s, got %s", AlertTaskStale, alerts[0].Type)
	}
}

func TestAlertEvaluator_ReviewTooLong(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create custom config with short threshold
	config := &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertReviewTooLong,
				Severity: AlertSeverityMedium,
				Duration: 50 * time.Millisecond,
			},
		},
	}

	ae := NewAlertEvaluator(config, mc)

	// Create task in review
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "review",
	})

	// Wait for threshold
	time.Sleep(100 * time.Millisecond)

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have one review alert
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Type != AlertReviewTooLong {
		t.Errorf("Expected alert type %s, got %s", AlertReviewTooLong, alerts[0].Type)
	}
}

func TestAlertEvaluator_BacklogTooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create custom config with small threshold
	config := &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertBacklogTooLarge,
				Severity: AlertSeverityLow,
				Count:    2,
			},
		},
	}

	ae := NewAlertEvaluator(config, mc)

	// Create tasks in backlog
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

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have one backlog alert
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Type != AlertBacklogTooLarge {
		t.Errorf("Expected alert type %s, got %s", AlertBacklogTooLarge, alerts[0].Type)
	}

	if alerts[0].Severity != AlertSeverityLow {
		t.Errorf("Expected Low severity, got %s", alerts[0].Severity)
	}
}

func TestAlertEvaluator_MultipleAlerts(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create custom config with short thresholds
	config := &AlertConfig{
		Thresholds: []AlertThreshold{
			{
				Type:     AlertTaskBlockedTooLong,
				Severity: AlertSeverityHigh,
				Duration: 50 * time.Millisecond,
			},
			{
				Type:     AlertTaskStale,
				Severity: AlertSeverityMedium,
				Duration: 50 * time.Millisecond,
			},
			{
				Type:     AlertBacklogTooLarge,
				Severity: AlertSeverityLow,
				Count:    1,
			},
		},
	}

	ae := NewAlertEvaluator(config, mc)

	// Create multiple tasks with different issues
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-001",
		"old_status": "backlog",
		"new_status": "blocked",
	})

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-002",
		"status":  "backlog",
	})

	el.Log(EventTaskStatusChanged, map[string]interface{}{
		"task_id":    "TASK-002",
		"old_status": "backlog",
		"new_status": "in_progress",
	})

	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-003",
		"status":  "backlog",
	})

	// Wait for thresholds
	time.Sleep(100 * time.Millisecond)

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have multiple alerts
	if len(alerts) < 2 {
		t.Errorf("Expected at least 2 alerts, got %d", len(alerts))
	}

	// Verify we have different alert types
	alertTypes := make(map[AlertType]bool)
	for _, alert := range alerts {
		alertTypes[alert.Type] = true
	}

	if !alertTypes[AlertTaskBlockedTooLong] {
		t.Error("Expected blocked task alert")
	}

	if !alertTypes[AlertTaskStale] {
		t.Error("Expected stale task alert")
	}
}

func TestAlertEvaluator_NoAlerts(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	ae := NewAlertEvaluator(DefaultAlertConfig(), mc)

	// Create tasks that don't trigger alerts
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"status":  "backlog",
	})

	el.Log(EventTaskCompleted, map[string]interface{}{
		"task_id": "TASK-001",
	})

	// Evaluate alerts
	alerts, err := ae.EvaluateAll()
	if err != nil {
		t.Fatalf("Failed to evaluate alerts: %v", err)
	}

	// Should have no alerts
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts, got %d", len(alerts))
	}
}

func TestAlertEvaluator_NilConfig(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	mc := NewMetricsCalculator(el)

	// Create evaluator with nil config (should use defaults)
	ae := NewAlertEvaluator(nil, mc)

	if ae.config == nil {
		t.Fatal("Expected default config when nil provided")
	}

	if len(ae.config.Thresholds) != 4 {
		t.Errorf("Expected 4 default thresholds, got %d", len(ae.config.Thresholds))
	}
}

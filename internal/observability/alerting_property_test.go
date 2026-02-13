package observability

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Generators
// =============================================================================

// genTaskID generates a random task ID in TASK-XXXXX format.
func genTaskID(t *rapid.T, label string) string {
	num := rapid.IntRange(1, 99999).Draw(t, label)
	return fmt.Sprintf("TASK-%05d", num)
}

// genBlockedEvents generates a set of events where some tasks transition to "blocked".
// Returns the events and the distinct task IDs that ended up in "blocked" status.
func genBlockedEvents(t *rapid.T) []Event {
	numTasks := rapid.IntRange(1, 10).Draw(t, "numTasks")
	baseTime := time.Now().UTC().Add(-200 * time.Hour)

	var events []Event
	for i := 0; i < numTasks; i++ {
		taskID := genTaskID(t, fmt.Sprintf("taskID_%d", i))
		hoursAgo := rapid.IntRange(1, 168).Draw(t, fmt.Sprintf("hoursAgo_%d", i))
		eventTime := baseTime.Add(time.Duration(hoursAgo) * time.Hour)

		// First create the task.
		events = append(events, Event{
			Time:    eventTime.Add(-time.Hour),
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": taskID, "type": "feat"},
		})

		// Then transition to blocked.
		events = append(events, Event{
			Time:    eventTime,
			Level:   "WARN",
			Type:    "task.status_changed",
			Message: "task blocked",
			Data:    map[string]any{"task_id": taskID, "new_status": "blocked"},
		})
	}
	return events
}

// genInProgressEvents generates events where tasks transition to "in_progress"
// at various times in the past.
func genInProgressEvents(t *rapid.T) []Event {
	numTasks := rapid.IntRange(1, 10).Draw(t, "numTasks")
	baseTime := time.Now().UTC().Add(-200 * time.Hour)

	var events []Event
	for i := 0; i < numTasks; i++ {
		taskID := genTaskID(t, fmt.Sprintf("taskID_%d", i))
		hoursAgo := rapid.IntRange(1, 168).Draw(t, fmt.Sprintf("hoursAgo_%d", i))
		eventTime := baseTime.Add(time.Duration(hoursAgo) * time.Hour)

		// Create and move to in_progress.
		events = append(events, Event{
			Time:    eventTime.Add(-time.Hour),
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": taskID, "type": "feat"},
		})
		events = append(events, Event{
			Time:    eventTime,
			Level:   "INFO",
			Type:    "task.status_changed",
			Message: "task in progress",
			Data:    map[string]any{"task_id": taskID, "new_status": "in_progress"},
		})
	}
	return events
}

// genBacklogEvents generates task.created events so tasks start in backlog.
func genBacklogEvents(t *rapid.T) []Event {
	numTasks := rapid.IntRange(1, 20).Draw(t, "numTasks")
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	var events []Event
	for i := 0; i < numTasks; i++ {
		taskID := genTaskID(t, fmt.Sprintf("taskID_%d", i))
		events = append(events, Event{
			Time:    baseTime.Add(time.Duration(i) * time.Minute),
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": taskID, "type": "feat"},
		})
	}
	return events
}

// =============================================================================
// Property 31: Blocked Alert Threshold Monotonicity
// =============================================================================

// Feature: observability, Property 31: Blocked Alert Threshold Monotonicity
// *For any* set of events containing blocked tasks, increasing the BlockedHours
// threshold SHALL produce fewer or equal blocked alerts.
//
// **Validates: Alert threshold consistency**
func TestProperty31_BlockedAlertThresholdMonotonicity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		events := genBlockedEvents(rt)
		for _, e := range events {
			if err := el.Write(e); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		// Generate two thresholds where low < high.
		lowThreshold := rapid.IntRange(1, 50).Draw(rt, "lowThreshold")
		highThreshold := rapid.IntRange(lowThreshold+1, 200).Draw(rt, "highThreshold")

		engineLow := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   lowThreshold,
			StaleDays:      365, // effectively disable other alerts
			ReviewDays:     365,
			MaxBacklogSize: 10000,
		})

		engineHigh := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   highThreshold,
			StaleDays:      365,
			ReviewDays:     365,
			MaxBacklogSize: 10000,
		})

		alertsLow, err := engineLow.Evaluate()
		if err != nil {
			t.Fatalf("evaluating low threshold alerts: %v", err)
		}

		alertsHigh, err := engineHigh.Evaluate()
		if err != nil {
			t.Fatalf("evaluating high threshold alerts: %v", err)
		}

		// Count only blocked alerts.
		blockedLow := countAlertsByCondition(alertsLow, "task_blocked_too_long")
		blockedHigh := countAlertsByCondition(alertsHigh, "task_blocked_too_long")

		if blockedHigh > blockedLow {
			rt.Errorf("higher threshold (%dh) produced more blocked alerts (%d) than lower threshold (%dh, %d)",
				highThreshold, blockedHigh, lowThreshold, blockedLow)
		}
	})
}

// =============================================================================
// Property 32: Stale Alert Threshold Monotonicity
// =============================================================================

// Feature: observability, Property 32: Stale Alert Threshold Monotonicity
// *For any* set of events with in_progress tasks, increasing the StaleDays
// threshold SHALL produce fewer or equal stale alerts.
//
// **Validates: Alert threshold consistency**
func TestProperty32_StaleAlertThresholdMonotonicity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		events := genInProgressEvents(rt)
		for _, e := range events {
			if err := el.Write(e); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		lowThreshold := rapid.IntRange(1, 5).Draw(rt, "lowThreshold")
		highThreshold := rapid.IntRange(lowThreshold+1, 30).Draw(rt, "highThreshold")

		engineLow := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   99999, // effectively disable other alerts
			StaleDays:      lowThreshold,
			ReviewDays:     99999,
			MaxBacklogSize: 10000,
		})

		engineHigh := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   99999,
			StaleDays:      highThreshold,
			ReviewDays:     99999,
			MaxBacklogSize: 10000,
		})

		alertsLow, err := engineLow.Evaluate()
		if err != nil {
			t.Fatalf("evaluating low threshold alerts: %v", err)
		}

		alertsHigh, err := engineHigh.Evaluate()
		if err != nil {
			t.Fatalf("evaluating high threshold alerts: %v", err)
		}

		staleLow := countAlertsByCondition(alertsLow, "task_stale")
		staleHigh := countAlertsByCondition(alertsHigh, "task_stale")

		if staleHigh > staleLow {
			rt.Errorf("higher threshold (%dd) produced more stale alerts (%d) than lower threshold (%dd, %d)",
				highThreshold, staleHigh, lowThreshold, staleLow)
		}
	})
}

// =============================================================================
// Property 33: Backlog Alert Threshold Monotonicity
// =============================================================================

// Feature: observability, Property 33: Backlog Alert Threshold Monotonicity
// *For any* set of task.created events, increasing MaxBacklogSize SHALL produce
// fewer or equal backlog alerts.
//
// **Validates: Alert threshold consistency**
func TestProperty33_BacklogAlertThresholdMonotonicity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		events := genBacklogEvents(rt)
		for _, e := range events {
			if err := el.Write(e); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		lowThreshold := rapid.IntRange(1, 10).Draw(rt, "lowThreshold")
		highThreshold := rapid.IntRange(lowThreshold+1, 50).Draw(rt, "highThreshold")

		engineLow := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   99999,
			StaleDays:      99999,
			ReviewDays:     99999,
			MaxBacklogSize: lowThreshold,
		})

		engineHigh := NewAlertEngine(el, AlertThresholds{
			BlockedHours:   99999,
			StaleDays:      99999,
			ReviewDays:     99999,
			MaxBacklogSize: highThreshold,
		})

		alertsLow, err := engineLow.Evaluate()
		if err != nil {
			t.Fatalf("evaluating low threshold alerts: %v", err)
		}

		alertsHigh, err := engineHigh.Evaluate()
		if err != nil {
			t.Fatalf("evaluating high threshold alerts: %v", err)
		}

		backlogLow := countAlertsByCondition(alertsLow, "backlog_too_large")
		backlogHigh := countAlertsByCondition(alertsHigh, "backlog_too_large")

		if backlogHigh > backlogLow {
			rt.Errorf("higher threshold (%d) produced more backlog alerts (%d) than lower threshold (%d, %d)",
				highThreshold, backlogHigh, lowThreshold, backlogLow)
		}
	})
}

// =============================================================================
// Property 34: Event Filter Time Range
// =============================================================================

// Feature: observability, Property 34: Event Filter Time Range
// *For any* set of events with random timestamps, applying an EventFilter with
// Since and Until SHALL return only events with timestamps within [Since, Until].
//
// **Validates: EventFilter correctness**
func TestProperty34_EventFilterTimeRange(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		numEvents := rapid.IntRange(1, 20).Draw(rt, "numEvents")

		for i := 0; i < numEvents; i++ {
			hoursOffset := rapid.IntRange(0, 168).Draw(rt, fmt.Sprintf("hoursOffset_%d", i))
			eventTime := baseTime.Add(time.Duration(hoursOffset) * time.Hour)

			event := Event{
				Time:    eventTime,
				Level:   "INFO",
				Type:    "task.created",
				Message: fmt.Sprintf("event %d", i),
				Data:    map[string]any{"task_id": genTaskID(rt, fmt.Sprintf("filterTaskID_%d", i))},
			}
			if err := el.Write(event); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		// Generate Since and Until where since <= until.
		sinceOffset := rapid.IntRange(0, 100).Draw(rt, "sinceOffset")
		untilOffset := rapid.IntRange(sinceOffset, 168).Draw(rt, "untilOffset")

		since := baseTime.Add(time.Duration(sinceOffset) * time.Hour)
		until := baseTime.Add(time.Duration(untilOffset) * time.Hour)

		filtered, err := el.Read(EventFilter{Since: &since, Until: &until})
		if err != nil {
			t.Fatalf("reading filtered events: %v", err)
		}

		for _, event := range filtered {
			if event.Time.Before(since) {
				rt.Errorf("event at %v is before Since %v", event.Time, since)
			}
			if event.Time.After(until) {
				rt.Errorf("event at %v is after Until %v", event.Time, until)
			}
		}
	})
}

// =============================================================================
// Helpers
// =============================================================================

// countAlertsByCondition counts alerts matching a specific condition string.
func countAlertsByCondition(alerts []Alert, condition string) int {
	count := 0
	for _, a := range alerts {
		if a.Condition == condition {
			count++
		}
	}
	return count
}

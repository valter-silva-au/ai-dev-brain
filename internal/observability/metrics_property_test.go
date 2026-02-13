package observability

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Property 35: Metrics Task Created Matches Events
// =============================================================================

// Feature: observability, Property 35: Metrics Task Created Matches Events
// *For any* N random task.created events written to an event log, the
// MetricsCalculator SHALL report TasksCreated == N.
//
// **Validates: MetricsCalculator accuracy for task creation counting**
func TestProperty35_MetricsTaskCreatedMatchesEvents(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		numEvents := rapid.IntRange(1, 20).Draw(rt, "numEvents")
		baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		taskTypes := []string{"feat", "bug", "spike", "refactor"}

		for i := 0; i < numEvents; i++ {
			taskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(1, 99999).Draw(rt, fmt.Sprintf("taskNum_%d", i)))
			taskType := rapid.SampledFrom(taskTypes).Draw(rt, fmt.Sprintf("taskType_%d", i))
			hoursOffset := rapid.IntRange(0, 168).Draw(rt, fmt.Sprintf("hoursOffset_%d", i))

			event := Event{
				Time:    baseTime.Add(time.Duration(hoursOffset) * time.Hour),
				Level:   "INFO",
				Type:    "task.created",
				Message: "task created",
				Data:    map[string]any{"task_id": taskID, "type": taskType},
			}
			if err := el.Write(event); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		calc := NewMetricsCalculator(el)
		since := baseTime.Add(-time.Hour)
		metrics, err := calc.Calculate(since)
		if err != nil {
			t.Fatalf("calculating metrics: %v", err)
		}

		if metrics.TasksCreated != numEvents {
			rt.Errorf("TasksCreated = %d, want %d", metrics.TasksCreated, numEvents)
		}
	})
}

// =============================================================================
// Property 36: Metrics Event Count Is Total
// =============================================================================

// Feature: observability, Property 36: Metrics Event Count Is Total
// *For any* mix of random event types written to an event log, the
// MetricsCalculator SHALL report EventCount equal to the total number of events.
//
// **Validates: MetricsCalculator total event counting accuracy**
func TestProperty36_MetricsEventCountIsTotal(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "events.jsonl")
		el, err := NewJSONLEventLog(logPath)
		if err != nil {
			t.Fatalf("creating event log: %v", err)
		}
		defer el.Close()

		numEvents := rapid.IntRange(1, 20).Draw(rt, "numEvents")
		baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		eventTypes := []string{
			"task.created",
			"task.completed",
			"task.status_changed",
			"agent.session_started",
			"knowledge.extracted",
		}

		for i := 0; i < numEvents; i++ {
			eventType := rapid.SampledFrom(eventTypes).Draw(rt, fmt.Sprintf("eventType_%d", i))
			hoursOffset := rapid.IntRange(0, 168).Draw(rt, fmt.Sprintf("hoursOffset_%d", i))
			taskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(1, 99999).Draw(rt, fmt.Sprintf("taskNum_%d", i)))

			data := map[string]any{"task_id": taskID}
			switch eventType {
			case "task.created":
				taskTypes := []string{"feat", "bug", "spike", "refactor"}
				data["type"] = rapid.SampledFrom(taskTypes).Draw(rt, fmt.Sprintf("taskType_%d", i))
			case "task.status_changed":
				statuses := []string{"in_progress", "blocked", "review", "done"}
				data["new_status"] = rapid.SampledFrom(statuses).Draw(rt, fmt.Sprintf("newStatus_%d", i))
			}

			event := Event{
				Time:    baseTime.Add(time.Duration(hoursOffset) * time.Hour),
				Level:   "INFO",
				Type:    eventType,
				Message: eventType,
				Data:    data,
			}
			if err := el.Write(event); err != nil {
				t.Fatalf("writing event: %v", err)
			}
		}

		calc := NewMetricsCalculator(el)
		since := baseTime.Add(-time.Hour)
		metrics, err := calc.Calculate(since)
		if err != nil {
			t.Fatalf("calculating metrics: %v", err)
		}

		if metrics.EventCount != numEvents {
			rt.Errorf("EventCount = %d, want %d", metrics.EventCount, numEvents)
		}
	})
}

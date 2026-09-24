package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEventLog_Log(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Log an event
	el.Log(EventTaskCreated, map[string]interface{}{
		"task_id": "TASK-001",
		"type":    "feat",
		"status":  "backlog",
	})

	// Read events
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventTaskCreated {
		t.Errorf("Expected event type %s, got %s", EventTaskCreated, events[0].Type)
	}

	if events[0].Data["task_id"] != "TASK-001" {
		t.Errorf("Expected task_id TASK-001, got %v", events[0].Data["task_id"])
	}
}

func TestEventLog_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Log multiple events
	eventTypes := []EventType{
		EventTaskCreated,
		EventAgentSessionStarted,
		EventTaskStatusChanged,
		EventWorktreeCreated,
		EventKnowledgeExtracted,
	}

	for i, eventType := range eventTypes {
		el.Log(eventType, map[string]interface{}{
			"index": i,
		})
	}

	// Read all events
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	if len(events) != len(eventTypes) {
		t.Fatalf("Expected %d events, got %d", len(eventTypes), len(events))
	}

	for i, event := range events {
		if event.Type != eventTypes[i] {
			t.Errorf("Event %d: expected type %s, got %s", i, eventTypes[i], event.Type)
		}
	}
}

func TestEventLog_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Concurrent writes
	numGoroutines := 10
	eventsPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				el.Log(EventTaskCreated, map[string]interface{}{
					"goroutine": id,
					"event":     j,
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify all events were written
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	expectedCount := numGoroutines * eventsPerGoroutine
	if len(events) != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, len(events))
	}
}

func TestEventLog_GracefullySkipMalformedLines(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	// Create a log file with malformed lines
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	// Write valid event
	validEvent := Event{
		Timestamp: time.Now().UTC(),
		Type:      EventTaskCreated,
		Data:      map[string]interface{}{"task_id": "TASK-001"},
	}
	validJSON, _ := json.Marshal(validEvent)
	f.Write(append(validJSON, '\n'))

	// Write malformed line
	f.WriteString("this is not valid JSON\n")

	// Write another valid event
	validEvent2 := Event{
		Timestamp: time.Now().UTC(),
		Type:      EventTaskCompleted,
		Data:      map[string]interface{}{"task_id": "TASK-002"},
	}
	validJSON2, _ := json.Marshal(validEvent2)
	f.Write(append(validJSON2, '\n'))

	f.Close()

	// Read events (should skip malformed line)
	el := NewEventLog(logPath)
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	// Should have 2 valid events
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	if events[0].Type != EventTaskCreated {
		t.Errorf("Expected first event type %s, got %s", EventTaskCreated, events[0].Type)
	}

	if events[1].Type != EventTaskCompleted {
		t.Errorf("Expected second event type %s, got %s", EventTaskCompleted, events[1].Type)
	}
}

func TestEventLog_NonFatalFileCreationFailure(t *testing.T) {
	// Try to create event log in a non-existent directory without parent creation
	logPath := "/nonexistent/directory/that/does/not/exist/.adb_events.jsonl"

	el := NewEventLog(logPath)

	// Should be disabled but not panic
	if el.enabled {
		t.Error("Expected event log to be disabled for invalid path")
	}

	// Log should not panic
	el.Log(EventTaskCreated, map[string]interface{}{"test": "data"})

	// Read should return empty events with no error (non-fatal behavior)
	events, err := el.ReadAll()
	if err != nil {
		t.Errorf("Expected no error when reading from non-existent log, got: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

func TestEventLog_ReadByType(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Log different event types
	el.Log(EventTaskCreated, map[string]interface{}{"id": "1"})
	el.Log(EventTaskCompleted, map[string]interface{}{"id": "2"})
	el.Log(EventTaskCreated, map[string]interface{}{"id": "3"})
	el.Log(EventAgentSessionStarted, map[string]interface{}{"id": "4"})
	el.Log(EventTaskCreated, map[string]interface{}{"id": "5"})

	// Read only TaskCreated events
	events, err := el.ReadByType(EventTaskCreated)
	if err != nil {
		t.Fatalf("Failed to read events by type: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 TaskCreated events, got %d", len(events))
	}

	for _, event := range events {
		if event.Type != EventTaskCreated {
			t.Errorf("Expected all events to be TaskCreated, got %s", event.Type)
		}
	}
}

func TestEventLog_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Read from empty log
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events from empty log, got %d", len(events))
	}
}

// TestEventLog_ReadSince returns only the events with Timestamp >= cutoff.
// The `adb events tail` command uses this to seed the initial view when the
// webview reconnects with a "last seen" watermark.
func TestEventLog_ReadSince(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})
	// Sleep long enough that the microsecond-resolution timestamp of the next
	// event is strictly greater than `cut`. `time.Now()` on macOS often
	// returns the same nanosecond twice back-to-back.
	time.Sleep(5 * time.Millisecond)
	cut := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	el.Log(EventTaskDeleted, map[string]interface{}{"task_id": "TASK-1"})

	got, err := el.ReadSince(cut)
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ReadSince returned %d events, want 1: %+v", len(got), got)
	}
	if got[0].Type != EventTaskDeleted {
		t.Errorf("ReadSince returned type %s, want %s", got[0].Type, EventTaskDeleted)
	}
}

// TestEventLog_ReadSince_Boundary: an event whose timestamp equals the cutoff
// is included (>= cutoff, not > cutoff). This matches the "last seen" idiom.
func TestEventLog_ReadSince_Boundary(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	// Craft an event with a known timestamp so the boundary is deterministic.
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e := Event{Timestamp: ts, Type: EventTaskCreated, Data: map[string]interface{}{"task_id": "TASK-1"}}
	b, _ := json.Marshal(e)
	if err := os.WriteFile(logPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	el := NewEventLog(logPath)
	got, err := el.ReadSince(ts)
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected boundary event to be included, got %d events", len(got))
	}
}

// TestEventLog_ReadSince_ZeroCutoff: a zero cutoff returns every event
// (equivalent to ReadAll). This lets `adb events tail` skip a special case
// when the caller has no watermark yet.
func TestEventLog_ReadSince_ZeroCutoff(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)
	el.Log(EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})
	el.Log(EventTaskDeleted, map[string]interface{}{"task_id": "TASK-1"})

	got, err := el.ReadSince(time.Time{})
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("zero cutoff should return all 2 events, got %d", len(got))
	}
}

func TestEventLog_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, ".adb_events.jsonl")

	el := NewEventLog(logPath)

	// Log some events
	el.Log(EventTaskCreated, map[string]interface{}{"id": "1"})
	el.Log(EventTaskCompleted, map[string]interface{}{"id": "2"})

	// Clear the log
	if err := el.Clear(); err != nil {
		t.Fatalf("Failed to clear log: %v", err)
	}

	// Verify log is empty
	events, err := el.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read events: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events after clear, got %d", len(events))
	}
}

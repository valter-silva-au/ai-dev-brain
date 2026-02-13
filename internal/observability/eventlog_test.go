package observability

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEventLog_WriteAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	events := []Event{
		{
			Time:    now,
			Level:   "INFO",
			Type:    "task.created",
			Message: "task created",
			Data:    map[string]any{"task_id": "TASK-00001"},
		},
		{
			Time:    now.Add(time.Second),
			Level:   "WARN",
			Type:    "task.status_changed",
			Message: "task blocked",
			Data:    map[string]any{"task_id": "TASK-00001", "new_status": "blocked"},
		},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	result, err := log.Read(EventFilter{})
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}

	if result[0].Type != "task.created" {
		t.Errorf("expected type task.created, got %s", result[0].Type)
	}
	if result[0].Message != "task created" {
		t.Errorf("expected message 'task created', got %s", result[0].Message)
	}
	if result[1].Level != "WARN" {
		t.Errorf("expected level WARN, got %s", result[1].Level)
	}
}

func TestEventLog_FilterByType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	now := time.Now().UTC()
	events := []Event{
		{Time: now, Level: "INFO", Type: "task.created", Message: "created"},
		{Time: now.Add(time.Second), Level: "INFO", Type: "task.status_changed", Message: "status changed"},
		{Time: now.Add(2 * time.Second), Level: "INFO", Type: "task.created", Message: "another created"},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	result, err := log.Read(EventFilter{Type: "task.created"})
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 events of type task.created, got %d", len(result))
	}

	for _, e := range result {
		if e.Type != "task.created" {
			t.Errorf("expected type task.created, got %s", e.Type)
		}
	}
}

func TestEventLog_FilterByTimeRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	base := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{Time: base, Level: "INFO", Type: "task.created", Message: "first"},
		{Time: base.Add(time.Hour), Level: "INFO", Type: "task.created", Message: "second"},
		{Time: base.Add(2 * time.Hour), Level: "INFO", Type: "task.created", Message: "third"},
		{Time: base.Add(3 * time.Hour), Level: "INFO", Type: "task.created", Message: "fourth"},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	since := base.Add(30 * time.Minute)
	until := base.Add(2*time.Hour + 30*time.Minute)
	result, err := log.Read(EventFilter{Since: &since, Until: &until})
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 events in time range, got %d", len(result))
	}

	if result[0].Message != "second" {
		t.Errorf("expected 'second', got %s", result[0].Message)
	}
	if result[1].Message != "third" {
		t.Errorf("expected 'third', got %s", result[1].Message)
	}
}

func TestEventLog_FilterByLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	now := time.Now().UTC()
	events := []Event{
		{Time: now, Level: "INFO", Type: "task.created", Message: "info event"},
		{Time: now.Add(time.Second), Level: "WARN", Type: "task.status_changed", Message: "warn event"},
		{Time: now.Add(2 * time.Second), Level: "ERROR", Type: "task.failed", Message: "error event"},
		{Time: now.Add(3 * time.Second), Level: "WARN", Type: "task.blocked", Message: "another warn"},
	}

	for _, e := range events {
		if err := log.Write(e); err != nil {
			t.Fatalf("writing event: %v", err)
		}
	}

	result, err := log.Read(EventFilter{Level: "WARN"})
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 WARN events, got %d", len(result))
	}

	for _, e := range result {
		if e.Level != "WARN" {
			t.Errorf("expected level WARN, got %s", e.Level)
		}
	}
}

func TestEventLog_EmptyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	result, err := log.Read(EventFilter{})
	if err != nil {
		t.Fatalf("reading empty log: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 events from empty log, got %d", len(result))
	}
}

func TestEventLog_ConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := NewJSONLEventLog(path)
	if err != nil {
		t.Fatalf("creating event log: %v", err)
	}
	defer log.Close()

	const goroutines = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := Event{
					Time:    time.Now().UTC(),
					Level:   "INFO",
					Type:    "task.created",
					Message: "concurrent event",
					Data:    map[string]any{"goroutine": id, "index": i},
				}
				if err := log.Write(event); err != nil {
					t.Errorf("concurrent write error: %v", err)
				}
			}
		}(g)
	}

	wg.Wait()

	result, err := log.Read(EventFilter{})
	if err != nil {
		t.Fatalf("reading events after concurrent writes: %v", err)
	}

	expected := goroutines * eventsPerGoroutine
	if len(result) != expected {
		t.Errorf("expected %d events, got %d", expected, len(result))
	}
}

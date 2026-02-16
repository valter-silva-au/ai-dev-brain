package observability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Event represents a single observable event in the system.
type Event struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"` // INFO, WARN, ERROR
	Type    string         `json:"type"`  // e.g. "task.created", "task.status_changed"
	Message string         `json:"msg"`
	Data    map[string]any `json:"data,omitempty"`
}

// EventFilter specifies criteria for reading events.
type EventFilter struct {
	Since *time.Time
	Until *time.Time
	Type  string
	Level string
}

// EventLog defines the interface for writing and reading events.
type EventLog interface {
	Write(event Event) error
	Read(filter EventFilter) ([]Event, error)
	Close() error
}

// jsonlEventLog implements EventLog using append-only JSONL files.
type jsonlEventLog struct {
	path string
	file *os.File
	mu   sync.Mutex
}

// NewJSONLEventLog creates a new EventLog backed by a JSONL file at the given path.
func NewJSONLEventLog(path string) (EventLog, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening event log: %w", err)
	}
	return &jsonlEventLog{
		path: path,
		file: f,
	}, nil
}

// Write appends a JSON-encoded event followed by a newline to the log file.
func (l *jsonlEventLog) Write(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshalling event: %w", err)
	}
	data = append(data, '\n')

	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}
	return nil
}

// Read opens the log file for reading, scans line by line, decodes each event,
// and returns those matching the given filter.
func (l *jsonlEventLog) Read(filter EventFilter) ([]Event, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening event log for reading: %w", err)
	}
	defer func() { _ = f.Close() }()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip malformed lines
		}

		if matchesEventFilter(event, filter) {
			events = append(events, event)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning event log: %w", err)
	}

	return events, nil
}

// Close closes the underlying log file.
func (l *jsonlEventLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("closing event log: %w", err)
	}
	return nil
}

// matchesEventFilter checks whether an event satisfies all filter criteria.
func matchesEventFilter(event Event, filter EventFilter) bool {
	if filter.Since != nil && event.Time.Before(*filter.Since) {
		return false
	}
	if filter.Until != nil && event.Time.After(*filter.Until) {
		return false
	}
	if filter.Type != "" && event.Type != filter.Type {
		return false
	}
	if filter.Level != "" && event.Level != filter.Level {
		return false
	}
	return true
}

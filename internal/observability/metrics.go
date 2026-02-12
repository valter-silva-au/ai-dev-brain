package observability

import (
	"fmt"
	"time"
)

// Metrics holds calculated metrics derived from the event log.
type Metrics struct {
	TasksCreated       int            `json:"tasks_created"`
	TasksCompleted     int            `json:"tasks_completed"`
	TasksByStatus      map[string]int `json:"tasks_by_status"`
	TasksByType        map[string]int `json:"tasks_by_type"`
	AgentSessions      int            `json:"agent_sessions"`
	KnowledgeExtracted int            `json:"knowledge_extracted"`
	EventCount         int            `json:"event_count"`
	OldestEvent        *time.Time     `json:"oldest_event,omitempty"`
	NewestEvent        *time.Time     `json:"newest_event,omitempty"`
}

// MetricsCalculator derives metrics from the event log.
type MetricsCalculator interface {
	Calculate(since time.Time) (*Metrics, error)
}

// metricsCalculator implements MetricsCalculator by reading from an EventLog.
type metricsCalculator struct {
	eventLog EventLog
}

// NewMetricsCalculator creates a new MetricsCalculator that reads from the given EventLog.
func NewMetricsCalculator(eventLog EventLog) MetricsCalculator {
	return &metricsCalculator{eventLog: eventLog}
}

// Calculate reads all events since the given time and aggregates them into metrics.
func (mc *metricsCalculator) Calculate(since time.Time) (*Metrics, error) {
	events, err := mc.eventLog.Read(EventFilter{Since: &since})
	if err != nil {
		return nil, fmt.Errorf("reading events for metrics: %w", err)
	}

	m := &Metrics{
		TasksByStatus: make(map[string]int),
		TasksByType:   make(map[string]int),
	}

	m.EventCount = len(events)

	for i, event := range events {
		if i == 0 {
			t := event.Time
			m.OldestEvent = &t
		}
		t := event.Time
		m.NewestEvent = &t

		switch event.Type {
		case "task.created":
			m.TasksCreated++
			if taskType, ok := event.Data["type"].(string); ok {
				m.TasksByType[taskType]++
			}
		case "task.completed":
			m.TasksCompleted++
		case "task.status_changed":
			if status, ok := event.Data["new_status"].(string); ok {
				m.TasksByStatus[status]++
			}
		case "agent.session_started":
			m.AgentSessions++
		case "knowledge.extracted":
			m.KnowledgeExtracted++
		}
	}

	return m, nil
}

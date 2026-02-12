package core

// EventLogger is the subset of the observability event log that core
// services need. Defining it here avoids importing the observability package.
type EventLogger interface {
	LogEvent(eventType string, data map[string]any) error
}

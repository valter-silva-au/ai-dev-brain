package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// ParseStdin reads JSON from stdin and unmarshals it into the provided type
func ParseStdin[T any](input io.Reader) (*T, error) {
	if input == nil {
		input = os.Stdin
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("failed to read stdin: %w", err)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &result, nil
}

// HookEvent represents a generic Claude Code hook event
type HookEvent struct {
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// PreToolUseEvent represents the PreToolUse hook payload
type PreToolUseEvent struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Timestamp  string                 `json:"timestamp"`
}

// PostToolUseEvent represents the PostToolUse hook payload
type PostToolUseEvent struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Result     interface{}            `json:"result,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// TaskCompletedEvent represents the TaskCompleted hook payload
type TaskCompletedEvent struct {
	TaskID    string                 `json:"task_id"`
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SessionEndEvent represents the SessionEnd hook payload
type SessionEndEvent struct {
	SessionID string                 `json:"session_id"`
	Timestamp string                 `json:"timestamp"`
	Duration  float64                `json:"duration,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

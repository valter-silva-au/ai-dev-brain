package hooks

import (
	"strings"
	"testing"
)

func TestParseStdin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid PreToolUseEvent",
			input:   `{"tool_name":"Edit","parameters":{"file_path":"test.go"},"timestamp":"2024-01-01T00:00:00Z"}`,
			wantErr: false,
		},
		{
			name:    "valid PostToolUseEvent",
			input:   `{"tool_name":"Write","parameters":{"file_path":"test.go"},"result":"success","timestamp":"2024-01-01T00:00:00Z"}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result, err := ParseStdin[PreToolUseEvent](reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStdin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Errorf("ParseStdin() returned nil result")
			}
		})
	}
}

func TestParseStdinPreToolUseEvent(t *testing.T) {
	input := `{"tool_name":"Edit","parameters":{"file_path":"test.go"},"timestamp":"2024-01-01T00:00:00Z"}`
	reader := strings.NewReader(input)

	result, err := ParseStdin[PreToolUseEvent](reader)
	if err != nil {
		t.Fatalf("ParseStdin() error = %v", err)
	}

	if result.ToolName != "Edit" {
		t.Errorf("ToolName = %v, want %v", result.ToolName, "Edit")
	}

	if result.Parameters["file_path"] != "test.go" {
		t.Errorf("Parameters[file_path] = %v, want %v", result.Parameters["file_path"], "test.go")
	}
}

func TestParseStdinPostToolUseEvent(t *testing.T) {
	input := `{"tool_name":"Write","parameters":{"file_path":"test.go","content":"package main"},"result":"success","timestamp":"2024-01-01T00:00:00Z"}`
	reader := strings.NewReader(input)

	result, err := ParseStdin[PostToolUseEvent](reader)
	if err != nil {
		t.Fatalf("ParseStdin() error = %v", err)
	}

	if result.ToolName != "Write" {
		t.Errorf("ToolName = %v, want %v", result.ToolName, "Write")
	}

	if result.Result != "success" {
		t.Errorf("Result = %v, want %v", result.Result, "success")
	}
}

func TestParseStdinTaskCompletedEvent(t *testing.T) {
	input := `{"task_id":"TASK-001","status":"done","timestamp":"2024-01-01T00:00:00Z"}`
	reader := strings.NewReader(input)

	result, err := ParseStdin[TaskCompletedEvent](reader)
	if err != nil {
		t.Fatalf("ParseStdin() error = %v", err)
	}

	if result.TaskID != "TASK-001" {
		t.Errorf("TaskID = %v, want %v", result.TaskID, "TASK-001")
	}

	if result.Status != "done" {
		t.Errorf("Status = %v, want %v", result.Status, "done")
	}
}

func TestParseStdinSessionEndEvent(t *testing.T) {
	input := `{"session_id":"sess-123","timestamp":"2024-01-01T00:00:00Z","duration":120.5}`
	reader := strings.NewReader(input)

	result, err := ParseStdin[SessionEndEvent](reader)
	if err != nil {
		t.Fatalf("ParseStdin() error = %v", err)
	}

	if result.SessionID != "sess-123" {
		t.Errorf("SessionID = %v, want %v", result.SessionID, "sess-123")
	}

	if result.Duration != 120.5 {
		t.Errorf("Duration = %v, want %v", result.Duration, 120.5)
	}
}

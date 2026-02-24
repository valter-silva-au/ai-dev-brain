package hooks

import (
	"strings"
	"testing"
)

func TestParseStdin_PreToolUseInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTool string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "valid edit input",
			input:    `{"tool_name":"Edit","tool_input":{"file_path":"/path/to/file.go","old_string":"a","new_string":"b"}}`,
			wantTool: "Edit",
			wantPath: "/path/to/file.go",
		},
		{
			name:     "valid write input",
			input:    `{"tool_name":"Write","tool_input":{"file_path":"/path/to/new.go","content":"hello"}}`,
			wantTool: "Write",
			wantPath: "/path/to/new.go",
		},
		{
			name:     "empty input returns zero value",
			input:    "",
			wantTool: "",
			wantPath: "",
		},
		{
			name:    "invalid json",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:     "missing tool_input",
			input:    `{"tool_name":"Read"}`,
			wantTool: "Read",
			wantPath: "",
		},
		{
			name:     "missing file_path in tool_input",
			input:    `{"tool_name":"Edit","tool_input":{"old_string":"a"}}`,
			wantTool: "Edit",
			wantPath: "",
		},
		{
			name:     "non-string file_path",
			input:    `{"tool_name":"Edit","tool_input":{"file_path":123}}`,
			wantTool: "Edit",
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			result, err := ParseStdin[PreToolUseInput](r)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ToolName != tt.wantTool {
				t.Errorf("ToolName = %q, want %q", result.ToolName, tt.wantTool)
			}
			if result.FilePath() != tt.wantPath {
				t.Errorf("FilePath() = %q, want %q", result.FilePath(), tt.wantPath)
			}
		})
	}
}

func TestParseStdin_PostToolUseInput(t *testing.T) {
	r := strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"main.go"}}`)
	result, err := ParseStdin[PostToolUseInput](r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q", result.ToolName, "Edit")
	}
	if result.FilePath() != "main.go" {
		t.Errorf("FilePath() = %q, want %q", result.FilePath(), "main.go")
	}
}

func TestParseStdin_StopInput(t *testing.T) {
	r := strings.NewReader(`{"stop_hook_active":true,"transcript_path":"/tmp/transcript.jsonl"}`)
	result, err := ParseStdin[StopInput](r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.StopHookActive {
		t.Error("StopHookActive should be true")
	}
	if result.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", result.TranscriptPath, "/tmp/transcript.jsonl")
	}
}

func TestParseStdin_TaskCompletedInput(t *testing.T) {
	r := strings.NewReader(`{"task_id":"TASK-00042","task_name":"Add auth"}`)
	result, err := ParseStdin[TaskCompletedInput](r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TaskID != "TASK-00042" {
		t.Errorf("TaskID = %q, want %q", result.TaskID, "TASK-00042")
	}
	if result.TaskName != "Add auth" {
		t.Errorf("TaskName = %q, want %q", result.TaskName, "Add auth")
	}
}

func TestParseStdin_SessionEndInput(t *testing.T) {
	r := strings.NewReader(`{"session_id":"abc123","transcript_path":"/tmp/t.jsonl","cwd":"/home/user","duration_ms":60000}`)
	result, err := ParseStdin[SessionEndInput](r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "abc123")
	}
	if result.DurationMS != 60000 {
		t.Errorf("DurationMS = %d, want %d", result.DurationMS, 60000)
	}
}

func TestPreToolUseInput_FilePath_NilToolInput(t *testing.T) {
	input := PreToolUseInput{ToolName: "Edit"}
	if got := input.FilePath(); got != "" {
		t.Errorf("FilePath() with nil tool_input = %q, want empty", got)
	}
}

func TestPostToolUseInput_FilePath_NilToolInput(t *testing.T) {
	input := PostToolUseInput{ToolName: "Edit"}
	if got := input.FilePath(); got != "" {
		t.Errorf("FilePath() with nil tool_input = %q, want empty", got)
	}
}

func TestToolInputFilePath(t *testing.T) {
	tests := []struct {
		name      string
		toolInput map[string]interface{}
		want      string
	}{
		{"nil map", nil, ""},
		{"empty map", map[string]interface{}{}, ""},
		{"no file_path key", map[string]interface{}{"old_string": "a"}, ""},
		{"file_path as int", map[string]interface{}{"file_path": 123}, ""},
		{"file_path as bool", map[string]interface{}{"file_path": true}, ""},
		{"file_path as nil", map[string]interface{}{"file_path": nil}, ""},
		{"valid file_path", map[string]interface{}{"file_path": "/path/to/file.go"}, "/path/to/file.go"},
		{"empty string file_path", map[string]interface{}{"file_path": ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolInputFilePath(tt.toolInput)
			if got != tt.want {
				t.Errorf("toolInputFilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

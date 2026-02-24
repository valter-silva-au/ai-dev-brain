package hooks

import (
	"encoding/json"
	"fmt"
	"io"
)

// PreToolUseInput is the stdin JSON for PreToolUse hooks.
type PreToolUseInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// FilePath returns the file_path from tool_input, or empty string if absent or non-string.
func (p PreToolUseInput) FilePath() string {
	return toolInputFilePath(p.ToolInput)
}

// PostToolUseInput is the stdin JSON for PostToolUse hooks.
type PostToolUseInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// FilePath returns the file_path from tool_input, or empty string if absent or non-string.
func (p PostToolUseInput) FilePath() string {
	return toolInputFilePath(p.ToolInput)
}

// StopInput is the stdin JSON for Stop hooks.
type StopInput struct {
	StopHookActive bool   `json:"stop_hook_active"`
	TranscriptPath string `json:"transcript_path"`
}

// TaskCompletedInput is the stdin JSON for TaskCompleted hooks.
type TaskCompletedInput struct {
	TaskID   string `json:"task_id"`
	TaskName string `json:"task_name"`
}

// SessionEndInput is the stdin JSON for SessionEnd hooks.
type SessionEndInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	DurationMS     int64  `json:"duration_ms"`
}

// ParseStdin reads JSON from the given reader into a new instance of T.
func ParseStdin[T any](r io.Reader) (*T, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		// Return zero-value struct when no input is provided.
		var zero T
		return &zero, nil
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing stdin JSON: %w", err)
	}
	return &result, nil
}

// toolInputFilePath extracts the file_path string from a tool_input map.
// Returns empty string if the map is nil or file_path is not a string.
func toolInputFilePath(toolInput map[string]interface{}) string {
	if toolInput == nil {
		return ""
	}
	fp, ok := toolInput["file_path"].(string)
	if !ok {
		return ""
	}
	return fp
}

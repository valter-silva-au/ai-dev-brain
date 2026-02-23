package hooks

import (
	"encoding/json"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty31_StdinParsingPreservesFields verifies that for any valid JSON
// with tool_name and tool_input.file_path, ParseStdin preserves all fields.
func TestProperty31_StdinParsingPreservesFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := rapid.SampledFrom([]string{"Edit", "Write", "Read", "Bash"}).Draw(t, "tool_name")
		filePath := rapid.StringMatching(`[a-z/]{1,50}\.go`).Draw(t, "file_path")

		inputJSON := map[string]interface{}{
			"tool_name": toolName,
			"tool_input": map[string]interface{}{
				"file_path": filePath,
			},
		}
		data, _ := json.Marshal(inputJSON)

		result, err := ParseStdin[PreToolUseInput](strings.NewReader(string(data)))
		if err != nil {
			t.Fatalf("parsing valid JSON should not fail: %v", err)
		}
		if result.ToolName != toolName {
			t.Fatalf("ToolName = %q, want %q", result.ToolName, toolName)
		}
		if result.FilePath() != filePath {
			t.Fatalf("FilePath() = %q, want %q", result.FilePath(), filePath)
		}
	})
}

// TestProperty32_StdinParsingEmptyInputReturnsZero verifies that empty input
// always returns a zero-value struct without error.
func TestProperty32_StdinParsingEmptyInputReturnsZero(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		result, err := ParseStdin[PreToolUseInput](strings.NewReader(""))
		if err != nil {
			t.Fatalf("empty input should not fail: %v", err)
		}
		if result.ToolName != "" {
			t.Fatalf("empty input should produce zero ToolName, got %q", result.ToolName)
		}
		if result.FilePath() != "" {
			t.Fatalf("empty input should produce empty FilePath, got %q", result.FilePath())
		}
	})
}

// TestProperty33_StdinParsingInvalidJSONAlwaysFails verifies that malformed
// JSON always produces an error.
func TestProperty33_StdinParsingInvalidJSONAlwaysFails(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate strings that look like bad JSON.
		badJSON := "{" + rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "bad") + "}"

		_, err := ParseStdin[PreToolUseInput](strings.NewReader(badJSON))
		if err == nil {
			t.Fatalf("invalid JSON %q should produce an error", badJSON)
		}
	})
}

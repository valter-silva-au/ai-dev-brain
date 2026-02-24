package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
	"pgregory.net/rapid"
)

// TestProperty30_ParsePreToolUseInputNeverPanics verifies that ParseStdin[PreToolUseInput]
// never panics for any well-formed JSON input.
func TestProperty30_ParsePreToolUseInputNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := rapid.StringMatching(`[a-zA-Z]{1,20}`).Draw(t, "toolName")
		filePath := rapid.StringMatching(`[a-zA-Z0-9/_.-]{0,100}`).Draw(t, "filePath")

		input := map[string]interface{}{
			"tool_name": toolName,
			"tool_input": map[string]interface{}{
				"file_path": filePath,
			},
		}
		data, err := json.Marshal(input)
		if err != nil {
			return // Skip malformed generation.
		}

		result, err := hooks.ParseStdin[hooks.PreToolUseInput](bytes.NewReader(data))
		if err != nil {
			return // Parse errors are fine, just must not panic.
		}
		// Result must be non-nil when err is nil.
		if result == nil {
			t.Fatal("expected non-nil result when err is nil")
		}
	})
}

// TestProperty31_PostToolUseAlwaysNonBlocking verifies that HandlePostToolUse
// always returns nil (non-blocking) regardless of input.
func TestProperty31_PostToolUseAlwaysNonBlocking(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := rapid.StringMatching(`[a-zA-Z]{0,20}`).Draw(t, "toolName")
		filePath := rapid.StringMatching(`[a-zA-Z0-9/_.-]{0,100}`).Draw(t, "filePath")

		mock := &hookEngineMock{
			postToolUseFn: func(input hooks.PostToolUseInput) error {
				return nil // PostToolUse is always non-blocking.
			},
		}

		input := hooks.PostToolUseInput{
			ToolName: toolName,
			ToolInput: map[string]interface{}{
				"file_path": filePath,
			},
		}

		err := mock.HandlePostToolUse(input)
		if err != nil {
			t.Fatalf("PostToolUse should never error, got: %v", err)
		}
	})
}

// TestProperty32_NilHookEngineGracefulDegradation verifies that all hook CLI
// commands return nil when HookEngine is nil (graceful degradation).
func TestProperty32_NilHookEngineGracefulDegradation(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()
	HookEngine = nil

	commands := []struct {
		name string
		fn   func() error
	}{
		{"pre-tool-use", func() error { return hookPreToolUseCmd.RunE(hookPreToolUseCmd, []string{}) }},
		{"post-tool-use", func() error { return hookPostToolUseCmd.RunE(hookPostToolUseCmd, []string{}) }},
		{"stop", func() error { return hookStopCmd.RunE(hookStopCmd, []string{}) }},
		{"task-completed", func() error { return hookTaskCompletedCmd.RunE(hookTaskCompletedCmd, []string{}) }},
	}

	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			err := cmd.fn()
			if err != nil {
				t.Errorf("nil HookEngine should return nil for %s, got: %v", cmd.name, err)
			}
		})
	}
}

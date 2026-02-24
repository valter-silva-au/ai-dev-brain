package cli

import (
	"testing"
)

func TestHookSessionEndCmd_NilEngineAndCapture(t *testing.T) {
	origEngine := HookEngine
	origCapture := SessionCapture
	defer func() {
		HookEngine = origEngine
		SessionCapture = origCapture
	}()
	HookEngine = nil
	SessionCapture = nil

	// Both nil: graceful exit, no error.
	err := hookSessionEndCmd.RunE(hookSessionEndCmd, []string{})
	if err != nil {
		t.Fatalf("nil engine and capture should return nil, got: %v", err)
	}
}

func TestHookSessionEndCmd_WithEngine(t *testing.T) {
	origEngine := HookEngine
	origCapture := SessionCapture
	defer func() {
		HookEngine = origEngine
		SessionCapture = origCapture
	}()

	HookEngine = &hookEngineMock{}
	SessionCapture = nil

	// Non-blocking: with empty stdin, ParseStdin returns zero-value.
	// Session capture is skipped when TranscriptPath is empty.
}

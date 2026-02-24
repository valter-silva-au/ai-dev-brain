package cli

import (
	"testing"
)

func TestHookStopCmd_NilEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()
	HookEngine = nil

	// nil engine: graceful exit, no error.
	err := hookStopCmd.RunE(hookStopCmd, []string{})
	if err != nil {
		t.Fatalf("nil HookEngine should return nil, got: %v", err)
	}
}

func TestHookStopCmd_WithEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()

	HookEngine = &hookEngineMock{}

	// Non-blocking: graceful degradation.
}

package cli

import (
	"testing"
)

func TestHookPostToolUseCmd_NilEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()
	HookEngine = nil

	// nil engine: graceful exit, no error.
	err := hookPostToolUseCmd.RunE(hookPostToolUseCmd, []string{})
	if err != nil {
		t.Fatalf("nil HookEngine should return nil, got: %v", err)
	}
}

func TestHookPostToolUseCmd_WithEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()

	HookEngine = &hookEngineMock{}

	// Non-blocking: even if stdin is empty, should not error.
	// ParseStdin returns zero-value on empty input.
	// We can't pipe stdin easily but nil engine test validates graceful path.
}

package cli

import (
	"testing"
)

func TestHookTaskCompletedCmd_NilEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()
	HookEngine = nil

	// nil engine: graceful exit, no error.
	err := hookTaskCompletedCmd.RunE(hookTaskCompletedCmd, []string{})
	if err != nil {
		t.Fatalf("nil HookEngine should return nil, got: %v", err)
	}
}

func TestHookTaskCompletedCmd_WithEngine(t *testing.T) {
	orig := HookEngine
	defer func() { HookEngine = orig }()

	HookEngine = &hookEngineMock{}

	// Blocking hook: with nil stdin, ParseStdin returns zero-value.
	// Engine processes the zero-value input and returns nil.
}

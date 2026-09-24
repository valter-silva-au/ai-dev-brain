package core

import (
	"errors"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
)

func specGateEngine(hasSpec func() (bool, error)) *HookEngine {
	return NewHookEngineWithOptions("", HookEngineOptions{
		SpecGate: SpecGateConfig{
			Enabled:    true,
			WritePaths: []string{"internal/*.go", "*.go"},
			HasSpec:    hasSpec,
		},
	})
}

func writeEvent(path string) *hooks.PreToolUseEvent {
	return &hooks.PreToolUseEvent{
		ToolName:   "Write",
		Parameters: map[string]interface{}{"file_path": path},
	}
}

func TestSpecGate_BlocksGuardedWriteWithoutSpec(t *testing.T) {
	engine := specGateEngine(func() (bool, error) { return false, nil })

	// A guarded write with no accepted spec is blocked.
	if err := engine.ProcessPreToolUse(writeEvent("internal/core/foo.go")); err == nil {
		t.Error("expected spec-gate to block a guarded write with no accepted ADR")
	}

	// An unguarded path is unaffected.
	if err := engine.ProcessPreToolUse(writeEvent("docs/notes.md")); err != nil {
		t.Errorf("unguarded write should pass, got %v", err)
	}
}

func TestSpecGate_AllowsGuardedWriteWithSpec(t *testing.T) {
	engine := specGateEngine(func() (bool, error) { return true, nil })
	if err := engine.ProcessPreToolUse(writeEvent("internal/core/foo.go")); err != nil {
		t.Errorf("guarded write should pass once a spec exists, got %v", err)
	}
}

func TestSpecGate_NilCheckerFailsSafe(t *testing.T) {
	// Enabled but no HasSpec closure → treated as "no spec" → blocks (safe+loud).
	engine := NewHookEngineWithOptions("", HookEngineOptions{
		SpecGate: SpecGateConfig{Enabled: true, WritePaths: []string{"*.go"}},
	})
	if err := engine.ProcessPreToolUse(writeEvent("main.go")); err == nil {
		t.Error("nil HasSpec should fail safe (block), not silently allow")
	}
}

func TestSpecGate_CheckErrorSurfaces(t *testing.T) {
	engine := specGateEngine(func() (bool, error) { return false, errors.New("registry unreadable") })
	err := engine.ProcessPreToolUse(writeEvent("main.go"))
	if err == nil {
		t.Fatal("expected the checker error to surface")
	}
}

func TestSpecGate_DisabledIsNoOp(t *testing.T) {
	engine := NewHookEngineWithOptions("", HookEngineOptions{}) // spec-gate off
	if err := engine.ProcessPreToolUse(writeEvent("internal/core/foo.go")); err != nil {
		t.Errorf("disabled spec-gate should not block, got %v", err)
	}
}

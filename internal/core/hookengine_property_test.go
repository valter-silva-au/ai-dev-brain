package core

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestProperty34_PreToolUseVendorGuardInvariant verifies that for any file
// path containing /vendor/ or starting with vendor/, HandlePreToolUse with
// default config MUST return an error.
func TestProperty34_PreToolUseVendorGuardInvariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		prefix := rapid.SampledFrom([]string{
			"vendor/",
			"some/vendor/",
			"deep/nested/vendor/",
		}).Draw(rt, "prefix")
		suffix := rapid.StringMatching(`[a-z]{1,20}\.go`).Draw(rt, "suffix")
		fp := prefix + suffix

		dir := t.TempDir()
		engine := NewHookEngine(dir, defaultTestHookConfig(), nil, nil)
		input := hooks.PreToolUseInput{
			ToolName:  "Edit",
			ToolInput: map[string]interface{}{"file_path": fp},
		}
		err := engine.HandlePreToolUse(input)
		if err == nil {
			rt.Fatalf("vendor path %q must be blocked", fp)
		}
	})
}

// TestProperty35_PreToolUseGoSumGuardInvariant verifies that for any path
// where filepath.Base(fp) == "go.sum", HandlePreToolUse MUST return an error.
func TestProperty35_PreToolUseGoSumGuardInvariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dirs := rapid.SliceOfN(
			rapid.StringMatching(`[a-z]{1,10}`),
			0, 3,
		).Draw(rt, "dirs")

		fp := "go.sum"
		for _, d := range dirs {
			fp = d + "/" + fp
		}

		dir := t.TempDir()
		engine := NewHookEngine(dir, defaultTestHookConfig(), nil, nil)
		input := hooks.PreToolUseInput{
			ToolName:  "Edit",
			ToolInput: map[string]interface{}{"file_path": fp},
		}
		err := engine.HandlePreToolUse(input)
		if err == nil {
			rt.Fatalf("go.sum path %q must be blocked", fp)
		}
	})
}

// TestProperty36_DisabledHookPassthrough verifies that for any hook config
// with Enabled=false, all Handle* methods return nil regardless of input.
func TestProperty36_DisabledHookPassthrough(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.SampledFrom([]string{"Edit", "Write", "Read", "Bash"}).Draw(rt, "tool")
		fp := rapid.StringMatching(`[a-z/]{0,30}`).Draw(rt, "path")

		dir := t.TempDir()
		cfg := models.HookConfig{Enabled: false}
		engine := NewHookEngine(dir, cfg, nil, nil)

		preInput := hooks.PreToolUseInput{
			ToolName:  toolName,
			ToolInput: map[string]interface{}{"file_path": fp},
		}
		if err := engine.HandlePreToolUse(preInput); err != nil {
			rt.Fatalf("disabled PreToolUse returned error: %v", err)
		}

		postInput := hooks.PostToolUseInput{
			ToolName:  toolName,
			ToolInput: map[string]interface{}{"file_path": fp},
		}
		if err := engine.HandlePostToolUse(postInput); err != nil {
			rt.Fatalf("disabled PostToolUse returned error: %v", err)
		}

		if err := engine.HandleStop(hooks.StopInput{}); err != nil {
			rt.Fatalf("disabled Stop returned error: %v", err)
		}

		if err := engine.HandleTaskCompleted(hooks.TaskCompletedInput{}); err != nil {
			rt.Fatalf("disabled TaskCompleted returned error: %v", err)
		}

		if err := engine.HandleSessionEnd(hooks.SessionEndInput{}); err != nil {
			rt.Fatalf("disabled SessionEnd returned error: %v", err)
		}
	})
}

// TestProperty37_PostToolUseNonBlocking verifies that HandlePostToolUse
// always returns nil for any input (non-blocking invariant).
func TestProperty37_PostToolUseNonBlocking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.SampledFrom([]string{"Edit", "Write", "", "Bash"}).Draw(rt, "tool")
		fp := rapid.StringMatching(`[a-z._/]{0,40}`).Draw(rt, "path")

		dir := t.TempDir()
		cfg := defaultTestHookConfig()
		cfg.PostToolUse.GoFormat = false // Avoid needing gofmt.
		engine := NewHookEngine(dir, cfg, nil, nil)

		input := hooks.PostToolUseInput{
			ToolName:  toolName,
			ToolInput: map[string]interface{}{"file_path": fp},
		}
		if err := engine.HandlePostToolUse(input); err != nil {
			rt.Fatalf("PostToolUse must never error, got: %v", err)
		}
	})
}

// TestProperty38_SessionEndNonBlocking verifies that HandleSessionEnd
// always returns nil for any input (non-blocking invariant).
func TestProperty38_SessionEndNonBlocking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		sessionID := rapid.StringMatching(`[a-z0-9]{0,20}`).Draw(rt, "session_id")

		dir := t.TempDir()
		cfg := defaultTestHookConfig()
		engine := NewHookEngine(dir, cfg, nil, nil)

		input := hooks.SessionEndInput{
			SessionID: sessionID,
		}
		if err := engine.HandleSessionEnd(input); err != nil {
			rt.Fatalf("SessionEnd must never error, got: %v", err)
		}
	})
}

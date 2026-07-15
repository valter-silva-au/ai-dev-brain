package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestHookInstall_PrintsCurrentSettingsSchema is a regression test for the bug
// where `adb hook install` printed an activation snippet in an OUTDATED schema:
// a flat name->path map under `.claude/config.json`. Current Claude Code reads
// hooks from `.claude/settings.json` in an event-keyed shape (PreToolUse /
// PostToolUse / Stop / SessionEnd, each an array of {matcher?, hooks:[{type,
// command}]}). Wiring the old snippet verbatim silently does nothing. The
// printed snippet must be the current schema and must be valid JSON.
func TestHookInstall_PrintsCurrentSettingsSchema(t *testing.T) {
	tmpDir := t.TempDir()

	app, err := internal.NewApp(tmpDir)
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
	defer app.Cleanup()

	oldApp := App
	App = app
	defer func() { App = oldApp }()

	// The snippet is emitted via fmt.Println (os.Stdout), so capture os.Stdout.
	out := captureStdout(t, func() {
		cmd := newHookInstallCmd()
		cmd.SetArgs([]string{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	// Must point at settings.json, not the stale config.json.
	if strings.Contains(out, ".claude/config.json") {
		t.Errorf("snippet still references outdated .claude/config.json\n---\n%s", out)
	}
	if !strings.Contains(out, ".claude/settings.json") {
		t.Errorf("snippet does not mention .claude/settings.json\n---\n%s", out)
	}

	// Must use the event-keyed schema, not the flat lower_snake map.
	for _, want := range []string{"PreToolUse", "PostToolUse", "Stop", "SessionEnd", `"matcher"`, `"type": "command"`} {
		if !strings.Contains(out, want) {
			t.Errorf("snippet missing current-schema token %q\n---\n%s", want, out)
		}
	}
	for _, bad := range []string{`"pre_tool_use"`, `"post_tool_use"`, `"task_completed"`, `"session_end"`} {
		if strings.Contains(out, bad) {
			t.Errorf("snippet reintroduced outdated flat key %q\n---\n%s", bad, out)
		}
	}

	// The JSON object in the snippet must actually parse.
	jsonBlock := extractFirstJSONObject(out)
	if jsonBlock == "" {
		t.Fatalf("no JSON object found in snippet\n---\n%s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonBlock), &parsed); err != nil {
		t.Fatalf("snippet JSON does not parse: %v\n---\n%s", err, jsonBlock)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Errorf("snippet JSON has no top-level \"hooks\" key\n---\n%s", jsonBlock)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written. The hook-install snippet uses fmt.Println (os.Stdout) rather than
// cmd.OutOrStdout(), so cobra's SetOut can't capture it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(data)
}

// extractFirstJSONObject returns the substring from the first '{' to its
// matching '}', tolerating strings/escapes so a brace inside a string literal
// doesn't throw off the depth count.
func extractFirstJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inStr:
			escaped = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// skip
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

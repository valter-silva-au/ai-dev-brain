package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// The agent-agnostic contract (TASK-00039): the generated body lands in the
// canonical AGENTS.md, and each configured harness gets a thin pointer beside it
// rather than its own copy of the context.
func TestSyncContext_WritesCanonicalPlusClaudePointer(t *testing.T) {
	dir, agentsPath := newContextWorkspace(t)
	runSyncContext(t)

	body := readFile(t, agentsPath)
	if !strings.Contains(body, "Use the glab CLI, never plain curl.") {
		t.Errorf("AGENTS.md is missing the generated body:\n%s", body)
	}

	pointer := readFile(t, filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(pointer, "@"+core.CanonicalInstructionFile) {
		t.Errorf("CLAUDE.md does not @import AGENTS.md:\n%s", pointer)
	}
	// The whole point of a pointer is that context lives in ONE file; a pointer
	// carrying the body would put the two immediately out of sync.
	if strings.Contains(pointer, "Use the glab CLI, never plain curl.") {
		t.Errorf("CLAUDE.md duplicated the body instead of pointing at it:\n%s", pointer)
	}
}

// A hand-written instruction file is the user's, not adb's. Before this change
// `sync context` overwrote the workspace CLAUDE.md unconditionally, so a
// hand-written one was silently destroyed on the next sync.
func TestSyncContext_LeavesHandWrittenPointerAloneAndWarns(t *testing.T) {
	dir, _ := newContextWorkspace(t)

	handWritten := "# My own house rules\n\nAlways show the full path of the files.\n"
	pointerPath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(pointerPath, []byte(handWritten), 0o644); err != nil {
		t.Fatalf("seed hand-written pointer: %v", err)
	}

	var stderr strings.Builder
	cmd := newSyncContextCmd()
	cmd.SetArgs(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync context: %v", err)
	}

	if got := readFile(t, pointerPath); got != handWritten {
		t.Errorf("hand-written CLAUDE.md was overwritten:\n%s", got)
	}
	if !strings.Contains(stderr.String(), "CLAUDE.md skipped") {
		t.Errorf("skipping a hand-written file must be reported; stderr was:\n%s", stderr.String())
	}
}

func TestSyncContext_ForceOverwritesHandWrittenPointer(t *testing.T) {
	dir, _ := newContextWorkspace(t)

	pointerPath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(pointerPath, []byte("# Mine\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	runSyncContext(t, "--force")

	if got := readFile(t, pointerPath); !strings.Contains(got, "@"+core.CanonicalInstructionFile) {
		t.Errorf("--force did not convert the pointer:\n%s", got)
	}
}

// --thin must land the same layout; otherwise the legacy generator quietly
// reintroduces a Claude-only workspace.
func TestSyncContext_ThinAlsoWritesCanonical(t *testing.T) {
	dir, agentsPath := newContextWorkspace(t)
	runSyncContext(t, "--thin")

	if body := readFile(t, agentsPath); !strings.Contains(body, "Current Backlog") {
		t.Errorf("thin generator did not write AGENTS.md:\n%s", body)
	}
	if pointer := readFile(t, filepath.Join(dir, "CLAUDE.md")); !strings.Contains(
		pointer, "@"+core.CanonicalInstructionFile,
	) {
		t.Errorf("thin generator did not write the pointer:\n%s", pointer)
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// setupSyncTest wires a temp workspace + global App the way sync_test.go's
// TestSyncCommands harness does. Returns the temp dir + a cleanup function
// that restores the global App.
func setupSyncTest(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	oldApp := App
	App = app
	return tmp, func() {
		App = oldApp
		app.Cleanup()
	}
}

// TestSyncIssues_DryRun_LocalTicketsSkipped: a backlog with only a _local
// (repo-less) task must produce no output beyond the summary and a "0 synced"
// count. No shell-out ever happens — the ProviderFor filter drops it.
func TestSyncIssues_DryRun_LocalTicketsSkipped(t *testing.T) {
	tmp, cleanup := setupSyncTest(t)
	defer cleanup()

	backlog := "tasks:\n" +
		"  - id: TASK-00001\n" +
		"    title: local only\n" +
		"    type: feat\n" +
		"    status: backlog\n" +
		"    priority: P2\n"
	if err := os.WriteFile(filepath.Join(tmp, "backlog.yaml"), []byte(backlog), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newSyncIssuesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("0 synced")) {
		t.Errorf("expected '0 synced' in summary, got:\n%s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("dry-run")) {
		t.Errorf("expected 'dry-run' marker in summary, got:\n%s", out.String())
	}
}

// TestSyncIssues_RejectsBadDirection asserts the flag validator rejects
// anything outside {both, push, pull} before any backlog work.
func TestSyncIssues_RejectsBadDirection(t *testing.T) {
	_, cleanup := setupSyncTest(t)
	defer cleanup()

	cmd := newSyncIssuesCmd()
	cmd.SetArgs([]string{"--direction", "sideways"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for bad --direction")
	}
}

// TestSyncIssues_AcceptsAllValidDirections keeps the accepted set explicit —
// so a future flag reshuffle doesn't silently drop 'push' or 'pull'.
func TestSyncIssues_AcceptsAllValidDirections(t *testing.T) {
	tmp, cleanup := setupSyncTest(t)
	defer cleanup()

	if err := os.WriteFile(filepath.Join(tmp, "backlog.yaml"), []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"both", "push", "pull"} {
		t.Run(dir, func(t *testing.T) {
			cmd := newSyncIssuesCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--dry-run", "--direction", dir})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute --direction %s: %v", dir, err)
			}
		})
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// helperNewSyncCloud isolates the "make an app, wire App, tear down"
// dance that every sync-cloud CLI test does. Returns the workspace root.
func helperNewSyncCloud(t *testing.T) (root string) {
	t.Helper()
	tmp := t.TempDir()
	// Populate a minimal workspace so App.NewApp is happy.
	if err := os.WriteFile(filepath.Join(tmp, "backlog.yaml"), []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "raw", "a.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })

	oldApp := App
	App = app
	t.Cleanup(func() { App = oldApp })
	return tmp
}

// TestSyncCloudCmd_Structure asserts the command tree: `sync cloud`
// has push/pull/status/destroy subcommands.
func TestSyncCloudCmd_Structure(t *testing.T) {
	cmd := newSyncCloudCmd()
	if cmd.Name() != "cloud" {
		t.Fatalf("cmd.Name() = %q, want cloud", cmd.Name())
	}
	got := map[string]bool{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = true
	}
	for _, want := range []string{"push", "pull", "status", "destroy"} {
		if !got[want] {
			t.Errorf("missing subcommand %q; have %v", want, got)
		}
	}
}

// TestSyncCloudPush_DryRunNoStore asserts `sync cloud push --dry-run`
// runs OFFLINE (never touches AWS, never needs a real bucket) and
// exits 0.
func TestSyncCloudPush_DryRunNoStore(t *testing.T) {
	_ = helperNewSyncCloud(t)

	cmd := newSyncCloudCmd()
	cmd.SetArgs([]string{"push", "--dry-run", "--bucket", "unused", "--region", "ap-southeast-2"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("push --dry-run: %v\nstderr: %s", err, stderr.String())
	}
	// Dry-run reports the plan.
	if !strings.Contains(stdout.String()+stderr.String(), "dry-run") {
		t.Errorf("dry-run output should mention 'dry-run': stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}

// TestSyncCloudDestroy_RequiresConfirm asserts `destroy` without
// --confirm exits with an error and never contacts AWS.
func TestSyncCloudDestroy_RequiresConfirm(t *testing.T) {
	_ = helperNewSyncCloud(t)

	cmd := newSyncCloudCmd()
	cmd.SetArgs([]string{"destroy", "--bucket", "unused", "--region", "ap-southeast-2"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err == nil {
		t.Fatalf("destroy without --confirm must return an error")
	}
}

// TestSyncCloudCmd_RegistersOnSyncCmd asserts that once wired into
// NewSyncCmd, the parent has a `cloud` subcommand. This is the shared
// one-line AddCommand collision-point with WS-E — pin it.
func TestSyncCloudCmd_RegistersOnSyncCmd(t *testing.T) {
	syncCmd := NewSyncCmd()
	found := false
	for _, sub := range syncCmd.Commands() {
		if sub.Name() == "cloud" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("NewSyncCmd() must include the 'cloud' subcommand")
	}
}

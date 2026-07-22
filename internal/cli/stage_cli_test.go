package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// runADB drives a command through the real root command, exactly as the binary would.
func runADB(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs(args)
	// Discard cobra's own output during tests. Use io.Discard (a pure io.Writer,
	// no file descriptor) — NOT os.NewFile(0, os.DevNull). The latter is a trap:
	// it wraps fd 0 (stdin, despite the os.DevNull label) in an *os.File whose GC
	// finalizer CLOSES fd 0 when the wrapper is collected. Every runADB call leaks
	// two such throwaway wrappers, so their finalizers fire at unpredictable points
	// later in the run.
	//
	// The hazard class is a stale finalizer closing a REUSED descriptor. Once fd 0
	// is closed, the kernel is free to hand the number 0 to the next descriptor the
	// process opens — e.g. the READ end of the os.Pipe the gate test uses to capture
	// os.Stdout (captureGateStdout in initiative_gate_test.go). When a leftover
	// wrapper's finalizer then runs, its close(2) lands on fd 0 — now that reused
	// pipe descriptor — out from under the runtime, which had registered it with the
	// netpoller. The capture goroutine parked in io.Copy on the read end never wakes
	// or sees EOF (even though the write end was closed), so the gate test hung to
	// the package timeout on Linux. PR #2's per-registry-write temp-file churn added
	// *os.File allocations that shifted GC/finalizer and fd-allocation timing enough
	// to surface it. The exact descriptor a given stale finalizer lands on is
	// timing-dependent — what is certain is the hazard class (stale finalizer closes
	// a reused fd), and that dropping the fd-0 wrappers removes it. io.Discard has no
	// fd and no finalizer, and is the pattern every other CLI test helper uses.
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func TestStageCLI_OrgInitiativeStage_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	// Create an org, an initiative under it, then advance the stage — all via the CLI.
	if err := runADB(t, "org", "create", "Acme Robotics", "--git-host", "github.com"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget Launcher", "--org", "acme-robotics"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	if err := runADB(t, "initiative", "set-stage", "widget-launcher", "MVP"); err != nil {
		t.Fatalf("set-stage: %v", err)
	}

	// Verify persistence through a FRESH App over the same workspace.
	reopened, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("reopen NewApp: %v", err)
	}
	defer reopened.Cleanup()

	org, err := reopened.StageManager.GetOrganization("acme-robotics")
	if err != nil {
		t.Fatalf("GetOrganization after reopen: %v", err)
	}
	if org.GitHost != "github.com" {
		t.Errorf("git host = %q, want github.com", org.GitHost)
	}

	init, err := reopened.StageManager.GetInitiative("widget-launcher")
	if err != nil {
		t.Fatalf("GetInitiative after reopen: %v", err)
	}
	if init.OrgID != "acme-robotics" {
		t.Errorf("initiative org = %q, want acme-robotics", init.OrgID)
	}
	if init.Stage != models.StageMVP {
		t.Errorf("stage = %q, want MVP", init.Stage)
	}

	// Registries live under the workspace root as metadata (not in the ticket path layout).
	if _, err := os.Stat(filepath.Join(tmp, "orgs", "index.yaml")); err != nil {
		t.Errorf("orgs/index.yaml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "initiatives", "index.yaml")); err != nil {
		t.Errorf("initiatives/index.yaml not written: %v", err)
	}
}

func TestStageCLI_ErrorPaths(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	// Initiative under a non-existent org is rejected.
	if err := runADB(t, "initiative", "create", "Orphan", "--org", "ghost"); err == nil {
		t.Error("initiative create with unknown org should fail")
	}
	// --org is required.
	if err := runADB(t, "initiative", "create", "NoOrg"); err == nil {
		t.Error("initiative create without --org should fail")
	}

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	// Invalid stage is rejected.
	if err := runADB(t, "initiative", "set-stage", "widget", "Growth"); err == nil {
		t.Error("set-stage with invalid stage should fail")
	}
	// set-stage on a missing initiative is rejected.
	if err := runADB(t, "initiative", "set-stage", "nope", "MVP"); err == nil {
		t.Error("set-stage on missing initiative should fail")
	}
}

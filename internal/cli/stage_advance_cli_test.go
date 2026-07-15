package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// runADBOut drives the root command capturing stdout+stderr, so tests can assert
// on rendered output (e.g. `adb events query`).
func runADBOut(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// TestStageCLI_Advance_BlockThenPass drives `adb stage advance` end-to-end
// through the real file-backed store (issue #89): the advance is refused while
// deterministic evidence is missing, and succeeds once the evidence artifacts
// exist — with the new stage + gate state persisting across a fresh App.
func TestStageCLI_Advance_BlockThenPass(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme", "--git-host", "github.com"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget Launcher", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}

	// Blocked: no evidence yet → non-zero (error) from the command, and it must
	// fail for the RIGHT reason (unmet deterministic evidence), not incidentally.
	if err := runADB(t, "stage", "advance", "widget-launcher"); err == nil {
		t.Fatal("stage advance should be refused while evidence is missing")
	} else if !strings.Contains(err.Error(), "required item(s) unmet") {
		t.Errorf("blocked advance failed for the wrong reason: %v", err)
	}
	// The blocked advance must not have moved the stage.
	if init, _ := app.StageManager.GetInitiative("widget-launcher"); init.Stage != models.StageIdea {
		t.Errorf("stage = %q after blocked advance, want Idea", init.Stage)
	}

	// Drop the required deterministic evidence artifacts.
	evDir := filepath.Join(tmp, "initiatives", "widget-launcher", "evidence")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	for _, f := range []string{"problem-statement.md", "target-customer.md"} {
		if err := os.WriteFile(filepath.Join(evDir, f), []byte("evidence content"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	// Clean pass: advance succeeds.
	if err := runADB(t, "stage", "advance", "widget-launcher"); err != nil {
		t.Fatalf("stage advance (clean pass): %v", err)
	}

	// Stage + gate state persist across a FRESH App over the same workspace.
	reopened, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("reopen NewApp: %v", err)
	}
	defer reopened.Cleanup()
	init, err := reopened.StageManager.GetInitiative("widget-launcher")
	if err != nil {
		t.Fatalf("GetInitiative after reopen: %v", err)
	}
	if init.Stage != models.StageMVP {
		t.Errorf("stage = %q after reopen, want MVP", init.Stage)
	}
	if init.Gate == nil || !init.Gate.Passed || init.Gate.Transition != "Idea->MVP" {
		t.Errorf("expected persisted passed Idea->MVP gate, got %+v", init.Gate)
	}
}

// TestStageCLI_Advance_MVPToLaunch drives the second gate (issue #98) end-to-end
// through the real file-backed store: an initiative walks Idea→MVP→Launch, the
// MVP→Launch advance is refused until its deterministic evidence exists, and the
// Launch stage + MVP->Launch gate persist across a fresh App.
func TestStageCLI_Advance_MVPToLaunch(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	writeFile := func(name string) {
		if err := os.WriteFile(filepath.Join(evDir, name), []byte("evidence"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Clear the Idea→MVP gate first.
	writeFile("problem-statement.md")
	writeFile("target-customer.md")
	if err := runADB(t, "stage", "advance", "widget"); err != nil {
		t.Fatalf("Idea->MVP advance: %v", err)
	}

	// MVP→Launch is now the active gate and is BLOCKED until its own evidence exists.
	if err := runADB(t, "stage", "advance", "widget"); err == nil {
		t.Fatal("MVP->Launch advance should be refused while its evidence is missing")
	} else if !strings.Contains(err.Error(), "required item(s) unmet") {
		t.Errorf("blocked MVP->Launch advance failed for the wrong reason: %v", err)
	}
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageMVP {
		t.Errorf("stage = %q after blocked MVP->Launch advance, want MVP", init.Stage)
	}

	// Record the MVP→Launch metric nodes that meet the numeric threshold, then
	// advance (decision D11: Sean-Ellis ≥40% + effort/retention bar, closing #103).
	if err := runADB(t, "pmf", "record", "--initiative", "widget", "--metric", "sean-ellis", "--value", "48"); err != nil {
		t.Fatalf("pmf record sean-ellis: %v", err)
	}
	if err := runADB(t, "pmf", "record", "--initiative", "widget", "--metric", "retention", "--value", "45"); err != nil {
		t.Fatalf("pmf record retention: %v", err)
	}
	if err := runADB(t, "stage", "advance", "widget"); err != nil {
		t.Fatalf("MVP->Launch advance (clean pass): %v", err)
	}

	// Launch stage + MVP->Launch gate persist across a FRESH App.
	reopened, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("reopen NewApp: %v", err)
	}
	defer reopened.Cleanup()
	init, err := reopened.StageManager.GetInitiative("widget")
	if err != nil {
		t.Fatalf("GetInitiative after reopen: %v", err)
	}
	if init.Stage != models.StageLaunch {
		t.Errorf("stage = %q after reopen, want Launch", init.Stage)
	}
	if init.Gate == nil || !init.Gate.Passed || init.Gate.Transition != "MVP->Launch" {
		t.Errorf("expected persisted passed MVP->Launch gate, got %+v", init.Gate)
	}
}

// TestStageCLI_Advance_HybridVerdict drives the hybrid gate (#102) end-to-end
// through the real App (which wires the RecordedVerdictSource): with all
// deterministic evidence present, a recorded `VERDICT: fail` blocks the advance,
// and flipping the recorded verdict to `pass` lets it through — closing the loop
// with the devils-advocate agent, whose output is what gets recorded.
func TestStageCLI_Advance_HybridVerdict(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(evDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// Deterministic evidence is fully satisfied for Idea->MVP.
	write("problem-statement.md", "real pain")
	write("target-customer.md", "indie founders")

	// A recorded FAILING adversarial verdict blocks the advance.
	write("problem-validation.verdict.md", "VERDICT: fail\nREASONS:\n- hypothetical enthusiasm only\n")
	if err := runADB(t, "stage", "advance", "widget"); err == nil {
		t.Fatal("advance should be blocked by a failing adversarial verdict")
	} else if !strings.Contains(err.Error(), "failed adversarial verdict") {
		t.Errorf("blocked advance failed for the wrong reason: %v", err)
	}
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageIdea {
		t.Errorf("stage = %q after a failed verdict, want Idea", init.Stage)
	}

	// Flip the recorded verdict to pass -> the advance goes through.
	write("problem-validation.verdict.md", "VERDICT: pass\nCONFIDENCE: high\n")
	if err := runADB(t, "stage", "advance", "widget"); err != nil {
		t.Fatalf("advance with a passing verdict: %v", err)
	}
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageMVP {
		t.Errorf("stage = %q after a passing verdict, want MVP", init.Stage)
	}
}

// TestStageCLI_Advance_Override drives issue #90 through the real command:
// `--override` requires `--reason`, a reason-logged override advances a blocked
// gate, and both stage.advanced and stage.override are readable via `adb events`.
func TestStageCLI_Advance_Override(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}

	// A non-clean advance without --override still fails (override is the only bypass).
	if err := runADB(t, "stage", "advance", "widget"); err == nil {
		t.Fatal("blocked advance without --override should fail")
	}
	// --override without --reason is refused (human override must be logged).
	if err := runADB(t, "stage", "advance", "widget", "--override"); err == nil {
		t.Fatal("--override without --reason should fail")
	}
	// Reason-logged override advances the blocked gate.
	if err := runADB(t, "stage", "advance", "widget", "--override", "--reason", "founder call: offline validation"); err != nil {
		t.Fatalf("override advance: %v", err)
	}
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageMVP {
		t.Errorf("stage = %q after override, want MVP", init.Stage)
	}

	// Both governance events are readable via `adb events query`.
	advancedOut, err := runADBOut(t, "events", "query", "--type", "stage.advanced")
	if err != nil {
		t.Fatalf("events query stage.advanced: %v", err)
	}
	if !strings.Contains(advancedOut, "stage.advanced") || !strings.Contains(advancedOut, "widget") {
		t.Errorf("stage.advanced not readable via events query, got:\n%s", advancedOut)
	}
	overrideOut, err := runADBOut(t, "events", "query", "--type", "stage.override")
	if err != nil {
		t.Fatalf("events query stage.override: %v", err)
	}
	if !strings.Contains(overrideOut, "stage.override") || !strings.Contains(overrideOut, "founder call") {
		t.Errorf("stage.override (with reason) not readable via events query, got:\n%s", overrideOut)
	}
}

// TestStageCLI_Advance_AutomationCannotOverride reproduces #157: when the CLI
// runs under an automation (the D7 rule engine stamps ADB_AUTOMATION_ACTIVE=1 on
// the child it shells), `adb stage advance --override` must be refused — the
// human-only override guard (D5) only fires when the CLI reports Automated=true,
// which it previously never did.
func TestStageCLI_Advance_AutomationCannotOverride(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}

	// Simulate the rule-engine child environment.
	t.Setenv("ADB_AUTOMATION_ACTIVE", "1")

	err = runADB(t, "stage", "advance", "widget", "--override", "--reason", "unattended bot decision")
	if err == nil {
		t.Fatal("an automation running --override must be refused (D5 human-only)")
	}
	if !strings.Contains(err.Error(), "human-only") {
		t.Errorf("override refusal failed for the wrong reason: %v", err)
	}
	// The refused override must not have moved the stage.
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageIdea {
		t.Errorf("stage = %q after refused automation override, want Idea (unchanged)", init.Stage)
	}
}

// TestStageCLI_Advance_AutomationCannotAdvanceHumanOnlyGate reproduces the other
// half of #157: even a clean-pass advance of the human-only Launch→Scale gate
// must be refused when run under an automation.
func TestStageCLI_Advance_AutomationCannotAdvanceHumanOnlyGate(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	// Park the initiative at Launch and record scale metrics that CLEANLY pass
	// the Launch→Scale thresholds (nrr ≥ 100, growth ≥ 15).
	if err := runADB(t, "initiative", "set-stage", "widget", "Launch"); err != nil {
		t.Fatalf("set-stage Launch: %v", err)
	}
	if err := runADB(t, "pmf", "record", "--initiative", "widget", "--metric", "nrr", "--value", "120", "--source", "board"); err != nil {
		t.Fatalf("pmf record nrr: %v", err)
	}
	if err := runADB(t, "pmf", "record", "--initiative", "widget", "--metric", "growth", "--value", "20", "--source", "board"); err != nil {
		t.Fatalf("pmf record growth: %v", err)
	}

	t.Setenv("ADB_AUTOMATION_ACTIVE", "1")

	err = runADB(t, "stage", "advance", "widget")
	if err == nil {
		t.Fatal("an automation must not advance the human-only Launch→Scale gate, even on a clean pass")
	}
	if !strings.Contains(err.Error(), "human-only") {
		t.Errorf("human-only refusal failed for the wrong reason: %v", err)
	}
	if init, _ := app.StageManager.GetInitiative("widget"); init.Stage != models.StageLaunch {
		t.Errorf("stage = %q after refused automation advance, want Launch (unchanged)", init.Stage)
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// These are the CLI contract tests for `adb initiative gate <id> [--json]` — the
// read seam a cockpit prototype consumes. They drive the real root command and
// assert the JSON projection (gateView) shape for the blocked, overridden/stale,
// and terminal cases, plus the human output and the not-found error. The
// side-effect-free guarantee is proven at the core layer (stagemanager_read_test.go)
// and re-checked once here through the CLI.

// captureGateStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. printJSON and the human renderer write to os.Stdout (not
// cmd.OutOrStdout), so this is how we capture command output.
func captureGateStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	outCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outCh <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-outCh
}

// newGateTestApp builds a real App over a temp workspace with one initiative
// "widget" at stage Idea, sets the package-level App, and returns the app + its
// workspace root (for reopen/side-effect checks).
func newGateTestApp(t *testing.T) (*internal.App, string) {
	t.Helper()
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	App = app
	if err := runADB(t, "org", "create", "Acme", "--git-host", "github.com"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}
	return app, tmp
}

func TestInitiativeGateCLI_JSON_Blocked_IsSideEffectFree(t *testing.T) {
	app, tmp := newGateTestApp(t)
	defer app.Cleanup()

	out := captureGateStdout(t, func() {
		if err := runADB(t, "initiative", "gate", "widget", "--json"); err != nil {
			t.Fatalf("initiative gate --json: %v", err)
		}
	})

	var v gateView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("unmarshal gate json: %v\noutput:\n%s", err, out)
	}
	if !v.HasGate || v.Stage != "Idea" {
		t.Errorf("has_gate=%v stage=%q, want true/Idea", v.HasGate, v.Stage)
	}
	if v.CurrentEvaluation == nil || v.CurrentEvaluation.Transition != "Idea->MVP" {
		t.Fatalf("current_evaluation wrong: %+v", v.CurrentEvaluation)
	}
	if v.CurrentEvaluation.Passed {
		t.Error("blocked gate should not be passed")
	}
	if v.EvaluatedAt == nil || v.EvaluatedAt.IsZero() {
		t.Error("evaluated_at should be present and non-zero")
	}
	if v.LastTransitionDecision != nil {
		t.Errorf("no advance yet; last_transition_decision should be null, got %+v", v.LastTransitionDecision)
	}

	// Side-effect-free: reopening the workspace shows no advance, no persisted gate.
	reopened, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("reopen NewApp: %v", err)
	}
	defer reopened.Cleanup()
	got, err := reopened.StageManager.GetInitiative("widget")
	if err != nil {
		t.Fatalf("GetInitiative after reopen: %v", err)
	}
	if got.Stage != models.StageIdea || got.Gate != nil {
		t.Errorf("read mutated state: stage=%q gate=%+v", got.Stage, got.Gate)
	}
}

func TestInitiativeGateCLI_JSON_Overridden_CurrentDiffersFromLast(t *testing.T) {
	app, _ := newGateTestApp(t)
	defer app.Cleanup()

	// Human override advances Idea->MVP and persists the overridden Idea->MVP gate.
	if err := runADB(t, "stage", "advance", "widget", "--override", "--reason", "validated offline over 20 interviews"); err != nil {
		t.Fatalf("stage advance --override: %v", err)
	}

	out := captureGateStdout(t, func() {
		if err := runADB(t, "initiative", "gate", "widget", "--json"); err != nil {
			t.Fatalf("initiative gate --json: %v", err)
		}
	})
	var v gateView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("unmarshal gate json: %v\noutput:\n%s", err, out)
	}
	if v.Stage != "MVP" {
		t.Errorf("stage = %q, want MVP", v.Stage)
	}
	if v.CurrentEvaluation == nil || v.CurrentEvaluation.Transition != "MVP->Launch" {
		t.Fatalf("current_evaluation should be MVP->Launch, got %+v", v.CurrentEvaluation)
	}
	if v.LastTransitionDecision == nil || v.LastTransitionDecision.Transition != "Idea->MVP" {
		t.Fatalf("last_transition_decision should be the Idea->MVP override, got %+v", v.LastTransitionDecision)
	}
	if !v.LastTransitionDecision.Overridden {
		t.Error("last_transition_decision should be marked overridden")
	}
	if v.CurrentEvaluation.Transition == v.LastTransitionDecision.Transition {
		t.Error("current_evaluation must differ from last_transition_decision (the whole point)")
	}
}

func TestInitiativeGateCLI_JSON_Terminal_Scale_NullGate(t *testing.T) {
	app, _ := newGateTestApp(t)
	defer app.Cleanup()

	if err := runADB(t, "initiative", "set-stage", "widget", "Scale"); err != nil {
		t.Fatalf("set-stage Scale: %v", err)
	}

	out := captureGateStdout(t, func() {
		if err := runADB(t, "initiative", "gate", "widget", "--json"); err != nil {
			t.Fatalf("initiative gate --json: %v", err)
		}
	})
	var v gateView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("unmarshal gate json: %v\noutput:\n%s", err, out)
	}
	if v.Stage != "Scale" {
		t.Errorf("stage = %q, want Scale", v.Stage)
	}
	if v.HasGate {
		t.Error("Scale is terminal; has_gate must be false")
	}
	if v.CurrentEvaluation != nil {
		t.Errorf("current_evaluation should be null at a terminal stage, got %+v", v.CurrentEvaluation)
	}
	if v.EvaluatedAt != nil {
		t.Errorf("evaluated_at should be null at a terminal stage, got %v", v.EvaluatedAt)
	}
}

func TestInitiativeGateCLI_Human_Blocked(t *testing.T) {
	app, _ := newGateTestApp(t)
	defer app.Cleanup()

	out := captureGateStdout(t, func() {
		if err := runADB(t, "initiative", "gate", "widget"); err != nil {
			t.Fatalf("initiative gate: %v", err)
		}
	})
	if !strings.Contains(out, "Idea->MVP") {
		t.Errorf("human output should name the current transition; got:\n%s", out)
	}
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("human output should mark the gate BLOCKED; got:\n%s", out)
	}
}

func TestInitiativeGateCLI_NotFound(t *testing.T) {
	app, _ := newGateTestApp(t)
	defer app.Cleanup()

	if err := runADB(t, "initiative", "gate", "ghost", "--json"); err == nil {
		t.Error("gate on a missing initiative should error")
	}
}

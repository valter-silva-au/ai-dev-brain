package core

import (
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// EvaluateCurrentGate is the side-effect-free read companion to AdvanceStage. The
// tests below are the CONTRACT for the five cases a caller (e.g. a cockpit UI)
// must handle — clean pass, blocked, overridden/stale, terminal, and
// unavailable-verdict — plus the not-found / unconfigured-root errors and, most
// importantly, proof that the read NEVER mutates the store.

// newIdeaInitiative builds a StageManager (with the given options) holding one
// initiative "widget" at stage Idea, returning the manager and its evidence dir.
func newIdeaInitiative(t *testing.T, evRoot string, opts ...StageManagerOption) StageManager {
	t.Helper()
	base := append([]StageManagerOption{WithEvidenceRoot(evRoot)}, opts...)
	sm := NewStageManager(&fakeStageStore{}, base...)
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	return sm
}

// Case: BLOCKED — the current-stage gate is unmet, and the read must not mutate.
func TestEvaluateCurrentGate_Blocked_IsSideEffectFree(t *testing.T) {
	tmp := t.TempDir()
	sm := newIdeaInitiative(t, tmp) // Idea, no evidence written

	res, err := sm.EvaluateCurrentGate("widget")
	if err != nil {
		t.Fatalf("EvaluateCurrentGate: %v", err)
	}
	if !res.HasGate {
		t.Fatal("Idea stage has a gate; HasGate should be true")
	}
	if res.Stage != models.StageIdea {
		t.Errorf("stage = %q, want Idea", res.Stage)
	}
	if res.CurrentEvaluation.Transition != "Idea->MVP" {
		t.Errorf("transition = %q, want Idea->MVP", res.CurrentEvaluation.Transition)
	}
	if res.CurrentEvaluation.Passed {
		t.Error("gate should be blocked with no evidence")
	}
	if statusOf(res.CurrentEvaluation, "problem-statement") != models.GateItemMissing {
		t.Errorf("problem-statement = %q, want missing", statusOf(res.CurrentEvaluation, "problem-statement"))
	}
	if res.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt must be stamped on the current evaluation")
	}
	if res.CurrentEvaluation.Evaluated != res.EvaluatedAt {
		t.Error("CurrentEvaluation.Evaluated must equal EvaluatedAt")
	}
	if res.LastTransitionDecision != nil {
		t.Errorf("no advance has happened; LastTransitionDecision should be nil, got %+v", res.LastTransitionDecision)
	}

	// The read must NOT have persisted anything.
	got, err := sm.GetInitiative("widget")
	if err != nil {
		t.Fatalf("GetInitiative: %v", err)
	}
	if got.Stage != models.StageIdea {
		t.Errorf("stage mutated to %q; EvaluateCurrentGate must not advance", got.Stage)
	}
	if got.Gate != nil {
		t.Errorf("gate persisted by a read; want nil, got %+v", got.Gate)
	}
}

// Case: CLEAN PASS — deterministic evidence satisfied and a passing verdict.
func TestEvaluateCurrentGate_CleanPass(t *testing.T) {
	tmp := t.TempDir()
	sm := newIdeaInitiative(t, tmp, WithVerdictSource(&fakeVerdictSource{available: true, pass: true}))
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "real pain")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	res, err := sm.EvaluateCurrentGate("widget")
	if err != nil {
		t.Fatalf("EvaluateCurrentGate: %v", err)
	}
	if !res.CurrentEvaluation.Passed {
		t.Fatalf("gate should pass with evidence + a passing verdict; items=%+v", res.CurrentEvaluation.Items)
	}
	if statusOf(res.CurrentEvaluation, "problem-validation") != models.GateItemMet {
		t.Errorf("judgment = %q, want met", statusOf(res.CurrentEvaluation, "problem-validation"))
	}
	// Still side-effect-free even on a clean pass — a read never persists.
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageIdea || got.Gate != nil {
		t.Errorf("a passing evaluation mutated the store: stage=%q gate=%+v", got.Stage, got.Gate)
	}
}

// Case: UNAVAILABLE VERDICT — no verdict source wired; the judgment item degrades
// to pending and never blocks. This is distinct from the clean-pass case (which
// had a passing verdict → met): with a pending judgment the gate still passes on
// satisfied deterministic evidence.
func TestEvaluateCurrentGate_UnavailableVerdict_DegradesPending(t *testing.T) {
	tmp := t.TempDir()
	sm := newIdeaInitiative(t, tmp) // no verdict source
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "real pain")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	res, err := sm.EvaluateCurrentGate("widget")
	if err != nil {
		t.Fatalf("EvaluateCurrentGate: %v", err)
	}
	if statusOf(res.CurrentEvaluation, "problem-validation") != models.GateItemPending {
		t.Errorf("judgment = %q, want pending (no verdict source)", statusOf(res.CurrentEvaluation, "problem-validation"))
	}
	if !res.CurrentEvaluation.Passed {
		t.Error("a pending judgment must never block; gate should pass on satisfied deterministic evidence")
	}
}

// Case: OVERRIDDEN / STALE — the CENTRAL bug this API exists to fix. After a human
// override advances Idea→MVP, the initiative sits at MVP but its PERSISTED gate is
// the overridden Idea->MVP one. EvaluateCurrentGate must return the CURRENT stage's
// gate (MVP->Launch), clearly distinct from the stored LastTransitionDecision — and
// must not overwrite that stored decision.
func TestEvaluateCurrentGate_Overridden_CurrentDiffersFromLastDecision(t *testing.T) {
	tmp := t.TempDir()
	sm := newIdeaInitiative(t, tmp) // no metric source → MVP->Launch metrics will be missing

	// Human override past the blocked Idea->MVP gate: persists an overridden
	// Idea->MVP gate and moves the initiative to MVP.
	if adv, err := sm.AdvanceStage("widget", AdvanceOptions{Override: true, Reason: "validated offline over 20 interviews"}); err != nil || !adv.Advanced || !adv.Overridden {
		t.Fatalf("override advance: err=%v adv=%+v", err, adv)
	}

	res, err := sm.EvaluateCurrentGate("widget")
	if err != nil {
		t.Fatalf("EvaluateCurrentGate: %v", err)
	}
	if res.Stage != models.StageMVP {
		t.Errorf("stage = %q, want MVP", res.Stage)
	}
	// CURRENT evaluation is the MVP->Launch gate...
	if !res.HasGate || res.CurrentEvaluation.Transition != "MVP->Launch" {
		t.Fatalf("current evaluation should be MVP->Launch, got HasGate=%v transition=%q", res.HasGate, res.CurrentEvaluation.Transition)
	}
	if res.CurrentEvaluation.Passed {
		t.Error("MVP->Launch should be blocked (no metrics recorded)")
	}
	// ...and it is NOT the stale stored decision.
	if res.LastTransitionDecision == nil {
		t.Fatal("LastTransitionDecision should carry the persisted Idea->MVP override")
	}
	if res.LastTransitionDecision.Transition != "Idea->MVP" {
		t.Errorf("LastTransitionDecision.Transition = %q, want Idea->MVP", res.LastTransitionDecision.Transition)
	}
	if !res.LastTransitionDecision.Overridden {
		t.Error("LastTransitionDecision should be marked Overridden")
	}
	if res.CurrentEvaluation.Transition == res.LastTransitionDecision.Transition {
		t.Error("the whole point: current evaluation must differ from the last transition decision")
	}

	// Side-effect-free: the stored gate is STILL the overridden Idea->MVP one — the
	// MVP->Launch read did not overwrite it, and the stage did not move.
	got, _ := sm.GetInitiative("widget")
	if got.Stage != models.StageMVP {
		t.Errorf("stage moved to %q after a read", got.Stage)
	}
	if got.Gate == nil || got.Gate.Transition != "Idea->MVP" || !got.Gate.Overridden {
		t.Errorf("stored gate was overwritten by a read; want the Idea->MVP override, got %+v", got.Gate)
	}
}

// Case: TERMINAL — at Scale there is no gate; HasGate is false and the caller must
// not render a checklist.
func TestEvaluateCurrentGate_Terminal_Scale_HasNoGate(t *testing.T) {
	tmp := t.TempDir()
	sm := newIdeaInitiative(t, tmp)
	if _, err := sm.SetStage("widget", models.StageScale); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	res, err := sm.EvaluateCurrentGate("widget")
	if err != nil {
		t.Fatalf("EvaluateCurrentGate: %v", err)
	}
	if res.Stage != models.StageScale {
		t.Errorf("stage = %q, want Scale", res.Stage)
	}
	if res.HasGate {
		t.Error("Scale is terminal; HasGate must be false")
	}
	if !res.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt should be zero when there is no gate to evaluate")
	}
	if len(res.CurrentEvaluation.Items) != 0 || res.CurrentEvaluation.Transition != "" {
		t.Errorf("CurrentEvaluation should be the zero value at a terminal stage, got %+v", res.CurrentEvaluation)
	}
}

// Error: unknown initiative.
func TestEvaluateCurrentGate_NotFound(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(t.TempDir()))
	if _, err := sm.EvaluateCurrentGate("ghost"); err == nil {
		t.Error("evaluating a missing initiative should error")
	}
}

// Error: evidence root not configured (mirrors AdvanceStage's precondition).
func TestEvaluateCurrentGate_EvidenceRootNotConfigured(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{}) // no WithEvidenceRoot
	if _, err := sm.EvaluateCurrentGate("widget"); err == nil {
		t.Error("evaluating without a configured evidence root should error")
	}
}

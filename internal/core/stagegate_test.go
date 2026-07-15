package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func writeEvidence(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write evidence %s: %v", name, err)
	}
}

// statusOf returns the evaluated status of item id in a GateState.
func statusOf(gs models.GateState, id string) models.GateItemStatus {
	for _, it := range gs.Items {
		if it.ID == id {
			return it.Status
		}
	}
	return ""
}

// TestEvaluateGate_IdeaToMVP covers the shipped bundle: deterministic items are
// missing until their non-empty artifacts exist; the judgment item is always
// pending and never blocks.
func TestEvaluateGate_IdeaToMVP(t *testing.T) {
	dir := t.TempDir()

	// No evidence yet: both deterministic items missing → not passed.
	gs := evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir})
	if gs.Passed {
		t.Fatal("gate should be blocked with no evidence")
	}
	if gs.Transition != "Idea->MVP" {
		t.Errorf("transition = %q, want Idea->MVP", gs.Transition)
	}
	if statusOf(gs, "problem-statement") != models.GateItemMissing {
		t.Errorf("problem-statement = %q, want missing", statusOf(gs, "problem-statement"))
	}
	if statusOf(gs, "problem-validation") != models.GateItemPending {
		t.Errorf("judgment item = %q, want pending", statusOf(gs, "problem-validation"))
	}

	// Drop both deterministic artifacts (non-empty) → passes; judgment stays pending.
	writeEvidence(t, dir, "problem-statement.md", "solo founders lose hours to X")
	writeEvidence(t, dir, "target-customer.md", "indie SaaS founders")
	gs = evaluateGate(ideaToMVPBundle, gateEval{evidenceDir: dir})
	if !gs.Passed {
		t.Fatalf("gate should pass with both artifacts; items=%+v", gs.Items)
	}
	if statusOf(gs, "problem-validation") != models.GateItemPending {
		t.Errorf("judgment item should remain pending after a pass, got %q", statusOf(gs, "problem-validation"))
	}
}

// fakeMetricSource is an in-memory MetricSource for the gate tests, keyed by
// "<initiative>|<name>".
type fakeMetricSource struct {
	values map[string]float64
	err    error
}

func (f *fakeMetricSource) Metric(initiative, name string) (float64, bool, error) {
	if f.err != nil {
		return 0, false, f.err
	}
	v, ok := f.values[initiative+"|"+name]
	return v, ok, nil
}

// TestEvaluateGate_MVPToLaunch covers the second shipped bundle, now enforced as
// NUMERIC metric thresholds (decision D11, closing #103): the Sean-Ellis and
// effort/retention items block until their metric nodes meet ≥40%; the judgment
// item is always pending and never blocks.
func TestEvaluateGate_MVPToLaunch(t *testing.T) {
	ms := &fakeMetricSource{values: map[string]float64{}}
	ev := gateEval{initiativeID: "onboarding", metrics: ms}

	// No metrics recorded: both metric items missing → not passed.
	gs := evaluateGate(mvpToLaunchBundle, ev)
	if gs.Passed {
		t.Fatal("gate should be blocked with no metrics recorded")
	}
	if gs.Transition != "MVP->Launch" {
		t.Errorf("transition = %q, want MVP->Launch", gs.Transition)
	}
	if statusOf(gs, "sean-ellis") != models.GateItemMissing {
		t.Errorf("sean-ellis = %q, want missing", statusOf(gs, "sean-ellis"))
	}
	if statusOf(gs, "launch-readiness") != models.GateItemPending {
		t.Errorf("judgment item = %q, want pending", statusOf(gs, "launch-readiness"))
	}

	// Sean-Ellis below the 40% bar → still blocked, even with retention met.
	ms.values["onboarding|sean-ellis"] = 30
	ms.values["onboarding|retention"] = 45
	gs = evaluateGate(mvpToLaunchBundle, ev)
	if gs.Passed {
		t.Fatalf("a below-threshold sean-ellis must block; items=%+v", gs.Items)
	}
	if statusOf(gs, "sean-ellis") != models.GateItemMissing {
		t.Errorf("sean-ellis at 30%% = %q, want missing (blocks)", statusOf(gs, "sean-ellis"))
	}

	// Both metrics ≥ 40 → passes; judgment stays pending.
	ms.values["onboarding|sean-ellis"] = 48
	gs = evaluateGate(mvpToLaunchBundle, ev)
	if !gs.Passed {
		t.Fatalf("gate should pass with both metrics ≥40; items=%+v", gs.Items)
	}
	if statusOf(gs, "launch-readiness") != models.GateItemPending {
		t.Errorf("judgment item should remain pending after a pass, got %q", statusOf(gs, "launch-readiness"))
	}
}

// TestArtifactSatisfied_EmptyFileIsMissing verifies an existing but empty file
// does NOT satisfy a deterministic check (the "non-empty" requirement).
func TestArtifactSatisfied_EmptyFileIsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := artifactSatisfied(dir, "empty.md"); ok {
		t.Error("empty file should not satisfy a deterministic check")
	}
	writeEvidence(t, dir, "full.md", "x")
	if ok, _ := artifactSatisfied(dir, "full.md"); !ok {
		t.Error("non-empty file should satisfy")
	}
	// No evidence dir configured → never satisfied.
	if ok, _ := artifactSatisfied("", "full.md"); ok {
		t.Error("empty evidence dir should never satisfy")
	}
}

// TestEvaluateGate_GenericOverBundle proves the engine is generic over the
// declarative bundle — a custom bundle (not in the registry) evaluates without
// any engine change, which is the "adjustable without changing the gate engine"
// acceptance criterion.
func TestEvaluateGate_GenericOverBundle(t *testing.T) {
	dir := t.TempDir()
	writeEvidence(t, dir, "custom.md", "hi")
	custom := gateBundle{
		From: models.StageMVP,
		To:   models.StageLaunch,
		Items: []gateItem{
			{ID: "custom", Desc: "a custom check", Kind: gateDeterministic, Artifact: "custom.md"},
			{ID: "verdict", Desc: "a custom verdict", Kind: gateJudgment},
		},
	}
	gs := evaluateGate(custom, gateEval{evidenceDir: dir})
	if !gs.Passed {
		t.Errorf("custom bundle should pass; items=%+v", gs.Items)
	}
	if gs.Transition != "MVP->Launch" {
		t.Errorf("transition = %q, want MVP->Launch", gs.Transition)
	}
}

// TestStageGates_KeysMatchBundleFromStage locks the registry invariant: each
// bundle must be registered under its own From stage, so the AdvanceStage
// lookup (stageGates[init.Stage]) can never return a bundle for a different
// starting stage.
func TestStageGates_KeysMatchBundleFromStage(t *testing.T) {
	for key, bundle := range stageGates {
		if key != bundle.From {
			t.Errorf("stageGates[%q] has bundle.From=%q; key must match From", key, bundle.From)
		}
	}
}

// TestArtifactSatisfied_RejectsTraversal verifies the containment guard: an
// artifact name that would escape the evidence directory is refused even if a
// matching file exists outside it.
func TestArtifactSatisfied_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if ok, detail := artifactSatisfied(dir, "../escape.md"); ok {
		t.Errorf("traversal artifact should be refused, got ok=true detail=%q", detail)
	}
}

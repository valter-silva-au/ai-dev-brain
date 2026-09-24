package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// captureLogger records Log calls for governance/telemetry assertions.
type captureLogger struct {
	types []string
}

func (c *captureLogger) Log(eventType string, _ map[string]interface{}) {
	c.types = append(c.types, eventType)
}
func (c *captureLogger) count(t string) int {
	n := 0
	for _, e := range c.types {
		if e == t {
			n++
		}
	}
	return n
}

// launchInitiative builds a StageManager with a Launch-stage initiative and the
// given scale metrics, returning the manager and initiative id.
func launchInitiative(t *testing.T, nrr, growth float64, extra ...StageManagerOption) (StageManager, string) {
	t.Helper()
	ms := &fakeMetricSource{values: map[string]float64{
		"widget|nrr":    nrr,
		"widget|growth": growth,
	}}
	opts := append([]StageManagerOption{WithEvidenceRoot(t.TempDir()), WithMetricSource(ms)}, extra...)
	sm := NewStageManager(&fakeStageStore{}, opts...)
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	if _, err := sm.SetStage("widget", models.StageLaunch); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	return sm, "widget"
}

func TestLaunchToScaleGate_BlocksThenPasses(t *testing.T) {
	// Below the scale thresholds → blocked, stage unchanged.
	sm, id := launchInitiative(t, 90, 5)
	res, err := sm.AdvanceStage(id, AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if res.Advanced {
		t.Fatal("gate should block below scale thresholds")
	}
	if res.Gate.Transition != "Launch->Scale" {
		t.Errorf("transition = %q, want Launch->Scale", res.Gate.Transition)
	}

	// Meet nrr ≥ 100 and growth ≥ 15 → passes to Scale.
	sm2, id2 := launchInitiative(t, 100, 20)
	res2, err := sm2.AdvanceStage(id2, AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if !res2.Advanced || res2.To != models.StageScale {
		t.Fatalf("expected advance to Scale, got advanced=%v to=%s", res2.Advanced, res2.To)
	}
}

func TestLaunchToScaleGate_IsHumanOnly(t *testing.T) {
	// Metrics met, but an AUTOMATION cannot advance a human-only gate (D5).
	sm, id := launchInitiative(t, 100, 20)
	if _, err := sm.AdvanceStage(id, AdvanceOptions{Automated: true}); err == nil {
		t.Error("an automation must not be able to advance the human-only Launch→Scale gate")
	}
	// A human advances it fine.
	res, err := sm.AdvanceStage(id, AdvanceOptions{})
	if err != nil {
		t.Fatalf("human AdvanceStage: %v", err)
	}
	if !res.Advanced {
		t.Error("a human should advance Launch→Scale on a clean pass")
	}
}

func TestAdvanceStage_AutomatedOverrideRefused(t *testing.T) {
	// Below threshold; an automation may not override (overrides are human-only, D5).
	sm, id := launchInitiative(t, 90, 5)
	if _, err := sm.AdvanceStage(id, AdvanceOptions{Automated: true, Override: true, Reason: "ship it"}); err == nil {
		t.Error("an automated override must be refused")
	}
}

func TestAdvanceStage_GovernanceStreamReceivesEvents(t *testing.T) {
	dev := &captureLogger{}
	gov := &captureLogger{}
	// Launch→Scale, metrics met, with both a dev and a governance logger wired.
	sm, id := launchInitiative(t, 100, 20, WithEventLogger(dev), WithGovernanceLogger(gov))
	if _, err := sm.AdvanceStage(id, AdvanceOptions{}); err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if gov.count("stage.advanced") != 1 {
		t.Errorf("governance stream stage.advanced = %d, want 1 (types=%v)", gov.count("stage.advanced"), gov.types)
	}
	// The dev telemetry stream still gets it too (KnownEventTypes contract).
	if dev.count("stage.advanced") != 1 {
		t.Errorf("dev stream stage.advanced = %d, want 1", dev.count("stage.advanced"))
	}
}

func TestAdvanceStage_GovernanceStreamCapturesOverride(t *testing.T) {
	gov := &captureLogger{}
	// Below threshold → a human override; governance stream must record both events.
	sm, id := launchInitiative(t, 90, 5, WithGovernanceLogger(gov))
	res, err := sm.AdvanceStage(id, AdvanceOptions{Override: true, Reason: "board approved a growth bet"})
	if err != nil {
		t.Fatalf("override AdvanceStage: %v", err)
	}
	if !res.Overridden {
		t.Fatal("expected an overridden advance")
	}
	if gov.count("stage.advanced") != 1 || gov.count("stage.override") != 1 {
		t.Errorf("governance override events = advanced:%d override:%d, want 1/1 (types=%v)",
			gov.count("stage.advanced"), gov.count("stage.override"), gov.types)
	}
}

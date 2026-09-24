package core

import (
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeStageStore is an in-memory StageStore for unit-testing StageManager rules.
type fakeStageStore struct {
	orgs        []models.Organization
	initiatives []models.Initiative
}

func (f *fakeStageStore) CreateOrganization(org models.Organization) error {
	for _, o := range f.orgs {
		if o.ID == org.ID {
			return errDuplicate(org.ID)
		}
	}
	f.orgs = append(f.orgs, org)
	return nil
}

func (f *fakeStageStore) GetOrganization(id string) (models.Organization, bool, error) {
	for _, o := range f.orgs {
		if o.ID == id {
			return o, true, nil
		}
	}
	return models.Organization{}, false, nil
}

func (f *fakeStageStore) ListOrganizations() ([]models.Organization, error) { return f.orgs, nil }

func (f *fakeStageStore) CreateInitiative(init models.Initiative) error {
	for _, i := range f.initiatives {
		if i.ID == init.ID {
			return errDuplicate(init.ID)
		}
	}
	f.initiatives = append(f.initiatives, init)
	return nil
}

func (f *fakeStageStore) GetInitiative(id string) (models.Initiative, bool, error) {
	for _, i := range f.initiatives {
		if i.ID == id {
			return i, true, nil
		}
	}
	return models.Initiative{}, false, nil
}

func (f *fakeStageStore) ListInitiatives() ([]models.Initiative, error) { return f.initiatives, nil }

func (f *fakeStageStore) UpdateInitiative(init models.Initiative) error {
	for idx, i := range f.initiatives {
		if i.ID == init.ID {
			f.initiatives[idx] = init
			return nil
		}
	}
	return errNotFound(init.ID)
}

func errDuplicate(id string) error { return &storeErr{"duplicate " + id} }
func errNotFound(id string) error  { return &storeErr{"not found " + id} }

type storeErr struct{ msg string }

func (e *storeErr) Error() string { return e.msg }

func TestStageManager_CreateOrganization_SlugID(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{})

	org, err := sm.CreateOrganization("Acme Robotics", "github.com")
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.ID != "acme-robotics" {
		t.Errorf("id = %q, want acme-robotics", org.ID)
	}
	if org.GitHost != "github.com" {
		t.Errorf("git host = %q, want github.com", org.GitHost)
	}
	if org.Created.IsZero() {
		t.Error("Created not stamped")
	}

	// An all-symbol name has no usable slug.
	if _, err := sm.CreateOrganization("!!!", ""); err == nil {
		t.Error("expected error for empty-slug name")
	}
}

func TestStageManager_CreateInitiative_DefaultsToIdea_AndRequiresOrg(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{})

	// Org must exist first.
	if _, err := sm.CreateInitiative("Widget", "acme"); err == nil {
		t.Fatal("CreateInitiative with unknown org should error")
	}

	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	init, err := sm.CreateInitiative("Widget", "acme")
	if err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	if init.ID != "widget" {
		t.Errorf("id = %q, want widget", init.ID)
	}
	if init.OrgID != "acme" {
		t.Errorf("org id = %q, want acme", init.OrgID)
	}
	if init.Stage != models.StageIdea {
		t.Errorf("stage = %q, want Idea (default)", init.Stage)
	}
}

func TestStageManager_SetStage(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{})
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}

	updated, err := sm.SetStage("widget", models.StageMVP)
	if err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if updated.Stage != models.StageMVP {
		t.Errorf("stage = %q, want MVP", updated.Stage)
	}

	// Invalid stage is rejected.
	if _, err := sm.SetStage("widget", models.Stage("Growth")); err == nil {
		t.Error("SetStage with invalid stage should error")
	}
	// Missing initiative is rejected.
	if _, err := sm.SetStage("ghost", models.StageMVP); err == nil {
		t.Error("SetStage on missing initiative should error")
	}
}

func TestStageManager_Get_NotFound(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{})
	if _, err := sm.GetOrganization("nope"); err == nil {
		t.Error("GetOrganization on missing org should error")
	}
	if _, err := sm.GetInitiative("nope"); err == nil {
		t.Error("GetInitiative on missing initiative should error")
	}
}

// TestStageManager_AdvanceStage_BlockThenPass exercises the whole Idea→MVP gate
// (issue #89): blocked while deterministic evidence is unmet (no mutation), then
// a clean pass advances the stage and records the gate durably.
func TestStageManager_AdvanceStage_BlockThenPass(t *testing.T) {
	tmp := t.TempDir()
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}

	// Blocked: no evidence exists.
	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage (blocked): %v", err)
	}
	if res.Advanced {
		t.Fatal("advance should be blocked with no evidence")
	}
	if res.Gate.Passed {
		t.Error("gate should not be passed with no evidence")
	}
	// A judgment item must be represented as pending (never blocking).
	if statusOf(res.Gate, "problem-validation") != models.GateItemPending {
		t.Errorf("judgment item = %q, want pending", statusOf(res.Gate, "problem-validation"))
	}
	// A blocked advance must NOT mutate the initiative.
	got, err := sm.GetInitiative("widget")
	if err != nil {
		t.Fatalf("GetInitiative: %v", err)
	}
	if got.Stage != models.StageIdea {
		t.Errorf("stage = %q after blocked advance, want Idea (unchanged)", got.Stage)
	}
	if got.Gate != nil {
		t.Error("blocked advance must not persist a gate state")
	}

	// Satisfy the deterministic evidence, then advance.
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "founders lose hours to manual X")
	writeEvidence(t, evDir, "target-customer.md", "indie SaaS founders")

	res, err = sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage (pass): %v", err)
	}
	if !res.Advanced {
		t.Fatalf("advance should succeed with evidence; items=%+v", res.Gate.Items)
	}
	if res.From != models.StageIdea || res.To != models.StageMVP {
		t.Errorf("transition = %s->%s, want Idea->MVP", res.From, res.To)
	}

	// Stage + gate state must persist on the initiative.
	got, err = sm.GetInitiative("widget")
	if err != nil {
		t.Fatalf("GetInitiative: %v", err)
	}
	if got.Stage != models.StageMVP {
		t.Errorf("stage = %q after pass, want MVP", got.Stage)
	}
	if got.Gate == nil || !got.Gate.Passed {
		t.Errorf("expected a persisted, passed gate state, got %+v", got.Gate)
	}
	if got.Gate != nil && got.Gate.Evaluated.IsZero() {
		t.Error("gate Evaluated timestamp not stamped")
	}
}

// countEvents returns how many recorded events have the given type.
func countEvents(logger *MockEventLogger, eventType string) int {
	n := 0
	for _, e := range logger.events {
		if e["type"] == eventType {
			n++
		}
	}
	return n
}

// TestStageManager_AdvanceStage_Override covers issue #90: a human override
// advances past a blocked gate (with a logged reason) and emits both
// stage.advanced and stage.override; an override without a reason is refused.
func TestStageManager_AdvanceStage_Override(t *testing.T) {
	tmp := t.TempDir()
	logger := NewMockEventLogger()
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithEventLogger(logger))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}

	// Override with no reason (or a whitespace-only reason) is refused —
	// a human-only override must be logged with a meaningful reason.
	if _, err := sm.AdvanceStage("widget", AdvanceOptions{Override: true}); err == nil {
		t.Error("override without a reason should error")
	}
	if _, err := sm.AdvanceStage("widget", AdvanceOptions{Override: true, Reason: "   "}); err == nil {
		t.Error("override with a whitespace-only reason should error")
	}
	// The refusal must not emit any events or advance the stage.
	if len(logger.events) != 0 {
		t.Errorf("refused override should emit no events, got %d", len(logger.events))
	}

	// Override with a reason advances the (blocked) gate and records the reason.
	res, err := sm.AdvanceStage("widget", AdvanceOptions{Override: true, Reason: "founder call: validated over 20 interviews offline"})
	if err != nil {
		t.Fatalf("override advance: %v", err)
	}
	if !res.Advanced || !res.Overridden {
		t.Fatalf("expected an overridden advance, got Advanced=%v Overridden=%v", res.Advanced, res.Overridden)
	}
	got, _ := sm.GetInitiative("widget")
	if got.Stage != models.StageMVP {
		t.Errorf("stage = %q after override, want MVP", got.Stage)
	}
	if got.Gate == nil || !got.Gate.Overridden || got.Gate.Reason == "" {
		t.Errorf("expected a persisted overridden gate with a reason, got %+v", got.Gate)
	}
	if got.Gate != nil && got.Gate.Passed {
		t.Error("an overridden gate should not be marked Passed")
	}

	// Events: exactly one stage.advanced (overridden) + one stage.override.
	if n := countEvents(logger, "stage.advanced"); n != 1 {
		t.Errorf("stage.advanced count = %d, want 1", n)
	}
	if n := countEvents(logger, "stage.override"); n != 1 {
		t.Errorf("stage.override count = %d, want 1", n)
	}
	for _, e := range logger.events {
		if e["type"] == "stage.advanced" && eventData(e, "overridden") != true {
			t.Errorf("stage.advanced overridden flag = %v, want true", eventData(e, "overridden"))
		}
		if e["type"] == "stage.override" && eventData(e, "reason") == "" {
			t.Error("stage.override event missing reason")
		}
	}
}

// TestStageManager_AdvanceStage_CleanPassEmitsAdvancedOnly verifies a clean
// pass emits stage.advanced (overridden=false) and NOT stage.override.
func TestStageManager_AdvanceStage_CleanPassEmitsAdvancedOnly(t *testing.T) {
	tmp := t.TempDir()
	logger := NewMockEventLogger()
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithEventLogger(logger))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "real pain")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("clean advance: %v", err)
	}
	if !res.Advanced || res.Overridden {
		t.Fatalf("expected a clean (non-override) advance, got Advanced=%v Overridden=%v", res.Advanced, res.Overridden)
	}
	if n := countEvents(logger, "stage.advanced"); n != 1 {
		t.Errorf("stage.advanced count = %d, want 1", n)
	}
	if n := countEvents(logger, "stage.override"); n != 0 {
		t.Errorf("stage.override count = %d, want 0 on a clean pass", n)
	}
}

// TestStageManager_AdvanceStage_BlockedEmitsNothing verifies a blocked advance
// (no override) emits no events and does not advance.
func TestStageManager_AdvanceStage_BlockedEmitsNothing(t *testing.T) {
	tmp := t.TempDir()
	logger := NewMockEventLogger()
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithEventLogger(logger))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if res.Advanced {
		t.Error("blocked advance should not advance")
	}
	if len(logger.events) != 0 {
		t.Errorf("blocked advance should emit no events, got %d", len(logger.events))
	}
}

// TestStageManager_AdvanceStage_MVPToLaunch exercises the second gate (issue
// #98) through the manager: blocked while deterministic evidence is unmet (no
// mutation), then a clean pass advances MVP→Launch and records the gate durably.
func TestStageManager_AdvanceStage_MVPToLaunch(t *testing.T) {
	tmp := t.TempDir()
	ms := &fakeMetricSource{values: map[string]float64{}}
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithMetricSource(ms))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	// Start at MVP (the transition under test is MVP→Launch).
	if _, err := sm.SetStage("widget", models.StageMVP); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	// Blocked: no MVP→Launch evidence exists.
	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage (blocked): %v", err)
	}
	if res.Advanced {
		t.Fatal("advance should be blocked with no evidence")
	}
	if res.Gate.Transition != "MVP->Launch" {
		t.Errorf("transition = %q, want MVP->Launch", res.Gate.Transition)
	}
	if statusOf(res.Gate, "launch-readiness") != models.GateItemPending {
		t.Errorf("judgment item = %q, want pending", statusOf(res.Gate, "launch-readiness"))
	}
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageMVP || got.Gate != nil {
		t.Errorf("blocked advance mutated the initiative: stage=%q gate=%+v", got.Stage, got.Gate)
	}

	// Record the metric nodes that meet the numeric threshold, then advance
	// (decision D11: MVP→Launch is a Sean-Ellis ≥40% + effort/retention bar).
	ms.values["widget|sean-ellis"] = 48
	ms.values["widget|retention"] = 45

	res, err = sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage (pass): %v", err)
	}
	if !res.Advanced || res.From != models.StageMVP || res.To != models.StageLaunch {
		t.Fatalf("expected MVP->Launch advance, got Advanced=%v %s->%s", res.Advanced, res.From, res.To)
	}
	got, _ := sm.GetInitiative("widget")
	if got.Stage != models.StageLaunch {
		t.Errorf("stage = %q after pass, want Launch", got.Stage)
	}
	if got.Gate == nil || !got.Gate.Passed || got.Gate.Transition != "MVP->Launch" {
		t.Errorf("expected a persisted passed MVP->Launch gate, got %+v", got.Gate)
	}
}

// TestStageManager_AdvanceStage_FullChain_IdeaToLaunch walks a single initiative
// through BOTH gates in sequence — Idea→MVP then MVP→Launch — proving the two
// declarative bundles compose without any engine change.
func TestStageManager_AdvanceStage_FullChain_IdeaToLaunch(t *testing.T) {
	tmp := t.TempDir()
	ms := &fakeMetricSource{values: map[string]float64{
		"widget|sean-ellis": 48,
		"widget|retention":  45,
	}}
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithMetricSource(ms))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	// Idea→MVP still uses file evidence; MVP→Launch uses the metric nodes above.
	writeEvidence(t, evDir, "problem-statement.md", "founders lose hours")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	// Idea → MVP.
	if res, err := sm.AdvanceStage("widget", AdvanceOptions{}); err != nil || !res.Advanced || res.To != models.StageMVP {
		t.Fatalf("Idea->MVP: err=%v res=%+v", err, res)
	}
	// MVP → Launch.
	if res, err := sm.AdvanceStage("widget", AdvanceOptions{}); err != nil || !res.Advanced || res.To != models.StageLaunch {
		t.Fatalf("MVP->Launch: err=%v res=%+v", err, res)
	}
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageLaunch {
		t.Errorf("final stage = %q, want Launch", got.Stage)
	}
}

// TestStageManager_AdvanceStage_NoGateDefined rejects advancing from a stage
// with no defined gate. Idea, MVP, and Launch have gates; Scale is terminal and
// has none, so advancing from Scale must error.
func TestStageManager_AdvanceStage_NoGateDefined(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(t.TempDir()))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	if _, err := sm.SetStage("widget", models.StageScale); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if _, err := sm.AdvanceStage("widget", AdvanceOptions{}); err == nil {
		t.Error("advancing from Scale (terminal, no gate defined) should error")
	}
}

// TestStageManager_AdvanceStage_NotFound rejects a missing initiative.
func TestStageManager_AdvanceStage_NotFound(t *testing.T) {
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(t.TempDir()))
	if _, err := sm.AdvanceStage("ghost", AdvanceOptions{}); err == nil {
		t.Error("advancing a missing initiative should error")
	}
}

// TestStageManager_AdvanceStage_HybridVerdictBlocks proves the hybrid gate (#102):
// with ALL deterministic evidence present, a FAILING adversarial verdict still
// blocks the advance (no mutation), and a human --override bypasses it.
func TestStageManager_AdvanceStage_HybridVerdictBlocks(t *testing.T) {
	tmp := t.TempDir()
	fake := &fakeVerdictSource{available: true, pass: false}
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithVerdictSource(fake))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	// Deterministic evidence is fully satisfied — only the verdict decides.
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "real pain")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if res.Advanced {
		t.Fatal("a failing adversarial verdict must block even with all deterministic evidence")
	}
	if statusOf(res.Gate, "problem-validation") != models.GateItemFailed {
		t.Errorf("judgment = %q, want failed", statusOf(res.Gate, "problem-validation"))
	}
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageIdea || got.Gate != nil {
		t.Errorf("blocked verdict mutated the initiative: stage=%q gate=%+v", got.Stage, got.Gate)
	}
	// The verdict source was consulted for the right transition + judgment item.
	if fake.gotTransition != "Idea->MVP" || fake.gotItemID != "problem-validation" {
		t.Errorf("verdict source queried with (%q,%q)", fake.gotTransition, fake.gotItemID)
	}

	// A human override advances past the failed verdict (D5).
	res, err = sm.AdvanceStage("widget", AdvanceOptions{Override: true, Reason: "founder validated offline"})
	if err != nil || !res.Advanced || !res.Overridden {
		t.Fatalf("override should advance past a failed verdict, got Advanced=%v Overridden=%v err=%v", res.Advanced, res.Overridden, err)
	}
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageMVP {
		t.Errorf("stage = %q after override, want MVP", got.Stage)
	}
}

// TestStageManager_AdvanceStage_HybridVerdictPasses proves a PASSING verdict plus
// satisfied deterministic evidence advances as a clean (non-override) pass.
func TestStageManager_AdvanceStage_HybridVerdictPasses(t *testing.T) {
	tmp := t.TempDir()
	sm := NewStageManager(&fakeStageStore{}, WithEvidenceRoot(tmp), WithVerdictSource(&fakeVerdictSource{available: true, pass: true}))
	if _, err := sm.CreateOrganization("Acme", ""); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := sm.CreateInitiative("Widget", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}
	evDir := filepath.Join(tmp, "initiatives", "widget", "evidence")
	writeEvidence(t, evDir, "problem-statement.md", "real pain")
	writeEvidence(t, evDir, "target-customer.md", "indie founders")

	res, err := sm.AdvanceStage("widget", AdvanceOptions{})
	if err != nil {
		t.Fatalf("AdvanceStage: %v", err)
	}
	if !res.Advanced || res.Overridden {
		t.Fatalf("expected a clean pass, got Advanced=%v Overridden=%v", res.Advanced, res.Overridden)
	}
	if statusOf(res.Gate, "problem-validation") != models.GateItemMet {
		t.Errorf("judgment = %q, want met", statusOf(res.Gate, "problem-validation"))
	}
	if got, _ := sm.GetInitiative("widget"); got.Stage != models.StageMVP {
		t.Errorf("stage = %q, want MVP", got.Stage)
	}
}

// Stage is orthogonal to TaskStatus: the two type sets are disjoint by construction.
func TestStage_DisjointFromTaskStatus(t *testing.T) {
	statuses := map[string]bool{
		string(models.TaskStatusBacklog):    true,
		string(models.TaskStatusInProgress): true,
		string(models.TaskStatusBlocked):    true,
		string(models.TaskStatusReview):     true,
		string(models.TaskStatusDone):       true,
		string(models.TaskStatusArchived):   true,
	}
	for _, s := range models.ValidStages {
		if statuses[string(s)] {
			t.Errorf("stage %q collides with a TaskStatus value", s)
		}
	}
}

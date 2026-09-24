package storage

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileRuleStore_MissingFileIsEmpty(t *testing.T) {
	s := NewFileRuleStore(t.TempDir())
	set, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(set.Rules) != 0 {
		t.Fatalf("expected empty rule set, got %+v", set.Rules)
	}
}

func TestFileRuleStore_SaveLoadRoundTrip(t *testing.T) {
	s := NewFileRuleStore(t.TempDir())
	want := models.RuleSet{Rules: []models.Rule{
		{Name: "nightly", On: models.RuleTrigger{Schedule: "15m"}, Run: models.RuleAction{Skill: "repos-pull"}},
		{
			Name:  "flag-blocked",
			On:    models.RuleTrigger{Event: "task.status_changed"},
			If:    &models.RuleCondition{Entity: "{{.task_id}}", HasEdge: models.EdgeDependsOn},
			Run:   models.RuleAction{Exec: []string{"echo", "blocked"}},
			Write: []models.RuleOutput{{Edge: &models.Link{Type: models.EdgeRelatesTo, Target: "INIT-1"}, EdgeFrom: "{{.task_id}}"}},
		},
	}}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(got.Rules))
	}
	if got.Rules[0].Name != "nightly" || got.Rules[1].If == nil || got.Rules[1].If.HasEdge != models.EdgeDependsOn {
		t.Fatalf("round-trip mismatch: %+v", got.Rules)
	}
}

func TestFileRuleStore_SaveRejectsInvalid(t *testing.T) {
	s := NewFileRuleStore(t.TempDir())
	// Two triggers on one rule is structurally invalid.
	bad := models.RuleSet{Rules: []models.Rule{
		{Name: "bad", On: models.RuleTrigger{Schedule: "1m", Event: "task.created"}, Run: models.RuleAction{Skill: "s"}},
	}}
	if err := s.Save(bad); err == nil {
		t.Fatal("expected Save to reject an invalid rule set, got nil")
	}
	// And nothing should have been written.
	set, _ := s.Load()
	if len(set.Rules) != 0 {
		t.Fatalf("invalid save leaked to disk: %+v", set.Rules)
	}
}

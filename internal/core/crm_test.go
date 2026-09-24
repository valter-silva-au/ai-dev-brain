package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type memCRMStore struct {
	deals []models.Deal
	seq   int
}

func (s *memCRMStore) NextID() (string, error) {
	s.seq++
	return "DEAL-" + pad4(s.seq), nil
}
func (s *memCRMStore) Add(d models.Deal) error {
	s.deals = append(s.deals, d)
	return nil
}
func (s *memCRMStore) List() ([]models.Deal, error) { return s.deals, nil }
func (s *memCRMStore) Get(id string) (models.Deal, bool, error) {
	for _, d := range s.deals {
		if d.ID == id {
			return d, true, nil
		}
	}
	return models.Deal{}, false, nil
}
func (s *memCRMStore) Update(d models.Deal) error {
	for i := range s.deals {
		if s.deals[i].ID == d.ID {
			s.deals[i] = d
			return nil
		}
	}
	return nil
}

func TestCRMManager_AddScoreAndDefaults(t *testing.T) {
	m := NewCRMManager(&memCRMStore{})

	// Default stage is awareness; MEDDPICC score counts filled fields.
	d, err := m.Add("Acme Corp", "", models.MEDDPICC{Metrics: "20% faster", Champion: "VP Eng"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if d.Stage != models.BowtieAwareness {
		t.Errorf("default stage = %s, want awareness", d.Stage)
	}
	if d.Score() != 2 {
		t.Errorf("score = %d, want 2 (metrics + champion)", d.Score())
	}

	// Empty name + invalid stage rejected.
	if _, err := m.Add("  ", "", models.MEDDPICC{}); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := m.Add("Bad", models.BowtieStage("bogus"), models.MEDDPICC{}); err == nil {
		t.Error("expected error for invalid stage")
	}
}

func TestCRMManager_ListOrderedByFunnel(t *testing.T) {
	m := NewCRMManager(&memCRMStore{})
	// Add out of funnel order.
	if _, err := m.Add("Expansion deal", models.BowtieExpansion, models.MEDDPICC{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add("New lead", models.BowtieAwareness, models.MEDDPICC{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add("Evaluating", models.BowtieSelection, models.MEDDPICC{}); err != nil {
		t.Fatal(err)
	}
	deals, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Funnel order: awareness < selection < expansion.
	if deals[0].Stage != models.BowtieAwareness || deals[1].Stage != models.BowtieSelection || deals[2].Stage != models.BowtieExpansion {
		t.Errorf("not ordered by funnel: %s, %s, %s", deals[0].Stage, deals[1].Stage, deals[2].Stage)
	}
}

func TestCRMManager_SetStage(t *testing.T) {
	m := NewCRMManager(&memCRMStore{})
	d, _ := m.Add("Acme", "", models.MEDDPICC{})

	got, err := m.SetStage(d.ID, models.BowtieSelection)
	if err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if got.Stage != models.BowtieSelection {
		t.Errorf("stage = %s, want selection", got.Stage)
	}
	if _, err := m.SetStage(d.ID, models.BowtieStage("bogus")); err == nil {
		t.Error("expected error for invalid stage")
	}
	if _, err := m.SetStage("DEAL-9999", models.BowtieImpact); err == nil {
		t.Error("expected error for unknown deal")
	}
}

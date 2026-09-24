package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

type memSLOStore struct{ slos []models.SLO }

func (s *memSLOStore) Set(slo models.SLO) error {
	for i := range s.slos {
		if s.slos[i].Name == slo.Name {
			s.slos[i] = slo
			return nil
		}
	}
	s.slos = append(s.slos, slo)
	return nil
}
func (s *memSLOStore) List() ([]models.SLO, error) { return s.slos, nil }

func TestSLOManager_SetValidatesAndUpserts(t *testing.T) {
	m := NewSLOManager(&memSLOStore{})

	if _, err := m.Set("api-availability", 99.9, "30d", "API uptime"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Out-of-range objective rejected.
	if _, err := m.Set("bad", 0, "", ""); err == nil {
		t.Error("expected error for objective 0")
	}
	if _, err := m.Set("bad", 150, "", ""); err == nil {
		t.Error("expected error for objective > 100")
	}
	// Missing name rejected.
	if _, err := m.Set("  ", 99, "", ""); err == nil {
		t.Error("expected error for empty name")
	}

	// Upsert: re-setting the same name updates in place.
	if _, err := m.Set("api-availability", 99.95, "7d", ""); err != nil {
		t.Fatalf("Set (update): %v", err)
	}
	slos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slos) != 1 {
		t.Fatalf("want 1 SLO after upsert, got %d", len(slos))
	}
	if slos[0].Objective != 99.95 || slos[0].Window != "7d" {
		t.Errorf("upsert did not update: %+v", slos[0])
	}
}

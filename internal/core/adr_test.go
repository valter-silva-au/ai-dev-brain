package core

import (
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// memADRStore is an in-memory ADRStore for the manager tests.
type memADRStore struct {
	adrs   []models.ADR
	bodies map[int]string
}

func newMemADRStore() *memADRStore { return &memADRStore{bodies: map[int]string{}} }

func (s *memADRStore) NextNumber() (int, error) {
	max := 0
	for _, a := range s.adrs {
		if a.Number > max {
			max = a.Number
		}
	}
	return max + 1, nil
}
func (s *memADRStore) Create(adr models.ADR, body string) error {
	s.adrs = append(s.adrs, adr)
	s.bodies[adr.Number] = body
	return nil
}
func (s *memADRStore) List() ([]models.ADR, error) { return s.adrs, nil }
func (s *memADRStore) Get(number int) (models.ADR, bool, error) {
	for _, a := range s.adrs {
		if a.Number == number {
			return a, true, nil
		}
	}
	return models.ADR{}, false, nil
}
func (s *memADRStore) Update(adr models.ADR) error {
	for i := range s.adrs {
		if s.adrs[i].Number == adr.Number {
			s.adrs[i] = adr
			return nil
		}
	}
	return nil
}
func (s *memADRStore) Body(adr models.ADR) (string, error) { return s.bodies[adr.Number], nil }

func fixedClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC) }
}

func TestADRManager_NewAssignsNumberAndScaffolds(t *testing.T) {
	m := NewADRManager(newMemADRStore(), WithADRClock(fixedClock()))

	a1, err := m.New("Use a single manifest for templates", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a1.Number != 1 || a1.Status != models.ADRProposed {
		t.Errorf("first ADR = %+v, want number 1 proposed", a1)
	}
	if a1.GraphID() != "adr:0001" {
		t.Errorf("GraphID = %q, want adr:0001", a1.GraphID())
	}
	if a1.Slug != "use-a-single-manifest-for-templates" {
		t.Errorf("slug = %q", a1.Slug)
	}

	a2, err := m.New("Second decision", []models.Link{{Type: models.EdgeRelatesTo, Target: "onboarding"}})
	if err != nil {
		t.Fatalf("New 2: %v", err)
	}
	if a2.Number != 2 {
		t.Errorf("second ADR number = %d, want 2", a2.Number)
	}
	if len(a2.Links) != 1 || a2.Links[0].Target != "onboarding" {
		t.Errorf("links not preserved: %+v", a2.Links)
	}

	// Show returns the scaffolded MADR body.
	_, body, err := m.Show(1)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(body, "# 1. Use a single manifest for templates") ||
		!strings.Contains(body, "## Decision Outcome") {
		t.Errorf("MADR body not scaffolded:\n%s", body)
	}

	// Empty title rejected.
	if _, err := m.New("   ", nil); err == nil {
		t.Error("expected error for empty title")
	}
}

func TestADRManager_SetStatus(t *testing.T) {
	m := NewADRManager(newMemADRStore(), WithADRClock(fixedClock()))
	if _, err := m.New("A decision", nil); err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := m.SetStatus(1, models.ADRAccepted)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got.Status != models.ADRAccepted {
		t.Errorf("status = %q, want accepted", got.Status)
	}

	// Invalid status rejected.
	if _, err := m.SetStatus(1, models.ADRStatus("bogus")); err == nil {
		t.Error("expected error for invalid status")
	}
	// Unknown number rejected.
	if _, err := m.SetStatus(99, models.ADRAccepted); err == nil {
		t.Error("expected error for unknown ADR")
	}
}

func TestADRManager_ListSortedByNumber(t *testing.T) {
	store := newMemADRStore()
	// Insert out of order.
	store.adrs = []models.ADR{{Number: 3, Title: "c"}, {Number: 1, Title: "a"}, {Number: 2, Title: "b"}}
	m := NewADRManager(store)
	adrs, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(adrs) != 3 || adrs[0].Number != 1 || adrs[1].Number != 2 || adrs[2].Number != 3 {
		t.Errorf("not sorted by number: %+v", adrs)
	}
}

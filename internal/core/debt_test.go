package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// memDebtStore is an in-memory DebtStore for the manager tests.
type memDebtStore struct {
	items []models.DebtItem
	seq   int
}

func (s *memDebtStore) NextID() (string, error) {
	s.seq++
	return "DEBT-" + pad4(s.seq), nil
}
func (s *memDebtStore) Add(item models.DebtItem) error {
	s.items = append(s.items, item)
	return nil
}
func (s *memDebtStore) List() ([]models.DebtItem, error) { return s.items, nil }
func (s *memDebtStore) Update(item models.DebtItem) error {
	for i := range s.items {
		if s.items[i].ID == item.ID {
			s.items[i] = item
			return nil
		}
	}
	return nil
}

func pad4(n int) string {
	s := "0000" + itoa(n)
	return s[len(s)-4:]
}
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestDebtManager_AddListResolve(t *testing.T) {
	m := NewDebtManager(&memDebtStore{}, WithDebtClock(fixedClock()))

	// Add three, mixed priority.
	if _, err := m.Add("slow query", "storage", "", models.PriorityP1); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := m.Add("no default", "", "priority defaults to P2", ""); err != nil {
		t.Fatalf("Add: %v", err)
	}
	crit, err := m.Add("panics on nil", "core", "", models.PriorityP0)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	items, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	// Triage order: P0 first.
	if items[0].Priority != models.PriorityP0 {
		t.Errorf("first item priority = %s, want P0", items[0].Priority)
	}
	// Default priority applied.
	var def models.DebtItem
	for _, it := range items {
		if it.Title == "no default" {
			def = it
		}
	}
	if def.Priority != models.PriorityP2 {
		t.Errorf("default priority = %s, want P2", def.Priority)
	}

	// Resolve the P0 item; it should sort after open items next time.
	if _, err := m.Resolve(crit.ID); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	items, _ = m.List()
	// Open items (P1, P2) come before the resolved P0.
	if items[len(items)-1].ID != crit.ID {
		t.Errorf("resolved item should sort last, got order %v", ids(items))
	}
	// Resolve is idempotent.
	if _, err := m.Resolve(crit.ID); err != nil {
		t.Errorf("second Resolve should be a no-op, got %v", err)
	}
	// Unknown id errors.
	if _, err := m.Resolve("DEBT-9999"); err == nil {
		t.Error("expected error resolving unknown id")
	}
}

func ids(items []models.DebtItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

// TestDebtManager_Add_RejectsInvalidPriority guards #158: an out-of-set priority
// (e.g. a lowercase "p0" typo) must be rejected, not stored verbatim. List()
// sorts lexically by priority, and ASCII 'p' (112) > 'P' (80), so a stored "p0"
// would sort AFTER every P0–P3 item — mis-triaging a critical item to the bottom
// of the list the docs promise is ordered P0→P3.
func TestDebtManager_Add_RejectsInvalidPriority(t *testing.T) {
	m := NewDebtManager(&memDebtStore{}, WithDebtClock(fixedClock()))

	for _, bad := range []models.Priority{"p0", "P4", "NONSENSE", "high"} {
		if _, err := m.Add("critical thing", "core", "", bad); err == nil {
			t.Errorf("Add with invalid priority %q should error", bad)
		}
	}

	// Valid priorities (and the empty default) still succeed.
	for _, ok := range []models.Priority{models.PriorityP0, models.PriorityP1, models.PriorityP2, models.PriorityP3, ""} {
		if _, err := m.Add("fine", "core", "", ok); err != nil {
			t.Errorf("Add with valid priority %q should succeed, got %v", ok, err)
		}
	}
}

// TestPriority_IsValid is the unit guard for the shared validator #158 uses.
func TestPriority_IsValid(t *testing.T) {
	for _, p := range []models.Priority{models.PriorityP0, models.PriorityP1, models.PriorityP2, models.PriorityP3} {
		if !p.IsValid() {
			t.Errorf("%q should be valid", p)
		}
	}
	for _, p := range []models.Priority{"", "p0", "P4", "P", "critical"} {
		if p.IsValid() {
			t.Errorf("%q should be invalid", p)
		}
	}
}

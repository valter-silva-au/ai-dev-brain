package storage

import (
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileDebtStore_AddListUpdate(t *testing.T) {
	s := NewFileDebtStore(t.TempDir())

	id1, err := s.NextID()
	if err != nil || id1 != "DEBT-0001" {
		t.Fatalf("NextID on empty = (%q, %v), want DEBT-0001", id1, err)
	}
	if err := s.Add(models.DebtItem{ID: id1, Title: "a", Priority: models.PriorityP1, Status: models.DebtOpen, Created: time.Now().UTC()}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// ID advances off the max existing.
	if id2, _ := s.NextID(); id2 != "DEBT-0002" {
		t.Errorf("NextID after one = %q, want DEBT-0002", id2)
	}
	// Duplicate rejected.
	if err := s.Add(models.DebtItem{ID: id1, Title: "dup"}); err == nil {
		t.Error("expected error adding duplicate id")
	}

	items, err := s.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("List = (%d items, %v)", len(items), err)
	}

	// Update round-trip.
	it := items[0]
	it.Status = models.DebtResolved
	if err := s.Update(it); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := s.List()
	if after[0].Status != models.DebtResolved {
		t.Errorf("status after update = %q, want resolved", after[0].Status)
	}
	if err := s.Update(models.DebtItem{ID: "DEBT-9999"}); err == nil {
		t.Error("expected error updating unknown id")
	}
}

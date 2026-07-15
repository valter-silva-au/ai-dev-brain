package storage

import (
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileCRMStore_AddListUpdate(t *testing.T) {
	s := NewFileCRMStore(t.TempDir())

	id1, err := s.NextID()
	if err != nil || id1 != "DEAL-0001" {
		t.Fatalf("NextID on empty = (%q, %v), want DEAL-0001", id1, err)
	}
	if err := s.Add(models.Deal{ID: id1, Name: "Acme", Stage: models.BowtieAwareness, Created: time.Now().UTC()}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id2, _ := s.NextID(); id2 != "DEAL-0002" {
		t.Errorf("NextID after one = %q, want DEAL-0002", id2)
	}
	if err := s.Add(models.Deal{ID: id1, Name: "dup"}); err == nil {
		t.Error("expected error adding duplicate id")
	}

	got, found, err := s.Get(id1)
	if err != nil || !found || got.Name != "Acme" {
		t.Fatalf("Get = (%+v, %v, %v)", got, found, err)
	}

	got.Stage = models.BowtieSelection
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _, _ := s.Get(id1)
	if after.Stage != models.BowtieSelection {
		t.Errorf("stage after update = %s, want selection", after.Stage)
	}
	if err := s.Update(models.Deal{ID: "DEAL-9999"}); err == nil {
		t.Error("expected error updating unknown deal")
	}
}

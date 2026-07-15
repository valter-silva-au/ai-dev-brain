package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileADRStore_CreateNumberingAndBody(t *testing.T) {
	dir := t.TempDir()
	s := NewFileADRStore(dir)

	n, err := s.NextNumber()
	if err != nil || n != 1 {
		t.Fatalf("NextNumber on empty = (%d, %v), want (1, nil)", n, err)
	}

	adr := models.ADR{Number: 1, Title: "First", Status: models.ADRProposed, Slug: "first", Created: time.Now().UTC()}
	if err := s.Create(adr, "# 1. First\n\nbody\n"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The MADR markdown landed under docs/adr/.
	mdPath := filepath.Join(dir, "docs", "adr", "0001-first.md")
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("expected markdown at %s: %v", mdPath, err)
	}
	body, err := s.Body(adr)
	if err != nil || body != "# 1. First\n\nbody\n" {
		t.Errorf("Body = (%q, %v)", body, err)
	}

	// NextNumber advances; duplicate create rejected.
	if n, _ := s.NextNumber(); n != 2 {
		t.Errorf("NextNumber after one = %d, want 2", n)
	}
	if err := s.Create(adr, "dup"); err == nil {
		t.Error("expected error creating duplicate number")
	}

	// Get + Update round-trip.
	got, found, err := s.Get(1)
	if err != nil || !found || got.Title != "First" {
		t.Fatalf("Get(1) = (%+v, %v, %v)", got, found, err)
	}
	got.Status = models.ADRAccepted
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _, _ := s.Get(1)
	if after.Status != models.ADRAccepted {
		t.Errorf("status after update = %q, want accepted", after.Status)
	}
	if err := s.Update(models.ADR{Number: 99}); err == nil {
		t.Error("expected error updating unknown ADR")
	}
}

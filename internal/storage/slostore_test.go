package storage

import (
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileSLOStore_SetListUpsert(t *testing.T) {
	s := NewFileSLOStore(t.TempDir())

	if slos, err := s.List(); err != nil || len(slos) != 0 {
		t.Fatalf("empty List = (%d, %v)", len(slos), err)
	}
	if err := s.Set(models.SLO{Name: "api", Objective: 99.9, Window: "30d", Updated: time.Now().UTC()}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(models.SLO{Name: "web", Objective: 99.5, Updated: time.Now().UTC()}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Upsert api in place.
	if err := s.Set(models.SLO{Name: "api", Objective: 99.99, Window: "7d", Updated: time.Now().UTC()}); err != nil {
		t.Fatalf("Set (update): %v", err)
	}
	slos, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slos) != 2 {
		t.Fatalf("want 2 SLOs, got %d", len(slos))
	}
	var api models.SLO
	for _, sl := range slos {
		if sl.Name == "api" {
			api = sl
		}
	}
	if api.Objective != 99.99 || api.Window != "7d" {
		t.Errorf("api not upserted: %+v", api)
	}
}

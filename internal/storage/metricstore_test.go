package storage

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFileMetricStore_RecordUpsertAndGet(t *testing.T) {
	s := NewFileMetricStore(t.TempDir())

	// Record → defaults Source=manual, stamps Recorded.
	got, err := s.Record(models.Metric{Initiative: "onboarding", Name: "sean-ellis", Value: 38, Unit: "%"})
	if err != nil {
		t.Fatalf("Record error = %v", err)
	}
	if got.Source != "manual" || got.Recorded.IsZero() {
		t.Fatalf("defaults not applied: %+v", got)
	}

	// Re-record same (initiative, name) → updates in place, not duplicated.
	if _, err := s.Record(models.Metric{Initiative: "onboarding", Name: "sean-ellis", Value: 45, Unit: "%"}); err != nil {
		t.Fatal(err)
	}
	all, _ := s.List()
	if len(all) != 1 {
		t.Fatalf("expected 1 metric after upsert, got %d", len(all))
	}

	m, found, err := s.Get("onboarding", "sean-ellis")
	if err != nil || !found {
		t.Fatalf("Get = found %v err %v", found, err)
	}
	if m.Value != 45 {
		t.Fatalf("value = %v, want 45 (updated)", m.Value)
	}

	// A different metric name coexists.
	if _, err := s.Record(models.Metric{Initiative: "onboarding", Name: "retention", Value: 42}); err != nil {
		t.Fatal(err)
	}
	all, _ = s.List()
	if len(all) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(all))
	}

	// Unknown metric → not found, no error.
	if _, found, err := s.Get("onboarding", "nps"); err != nil || found {
		t.Fatalf("Get(unknown) = found %v err %v, want false/nil", found, err)
	}
}

func TestFileMetricStore_RejectsInvalid(t *testing.T) {
	s := NewFileMetricStore(t.TempDir())
	if _, err := s.Record(models.Metric{Name: "sean-ellis"}); err == nil {
		t.Fatal("expected Record to reject a metric with no initiative")
	}
}

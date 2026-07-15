package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestFileGraphStore_SaveLoadRoundTrip persists a derived index and reads it
// back through a fresh store, proving graph/index.yaml round-trips.
func TestFileGraphStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewFileGraphStore(dir)

	idx := models.GraphIndex{Edges: []models.GraphEdge{
		{From: "TASK-00001", Type: models.EdgeDependsOn, To: "TASK-00002"},
		{From: "TASK-00002", Type: models.EdgePartOf, To: "some-initiative"},
	}}
	if err := s.SaveGraphIndex(idx); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "graph", "index.yaml")); err != nil {
		t.Fatalf("index file not written: %v", err)
	}

	got, ok, err := NewFileGraphStore(dir).LoadGraphIndex()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatal("expected found=true after save")
	}
	if len(got.Edges) != 2 || got.Edges[0].To != "TASK-00002" || got.Edges[1].Type != models.EdgePartOf {
		t.Errorf("round-trip mismatch: %+v", got.Edges)
	}
}

// TestFileGraphStore_LoadMissing returns found=false (not an error) before any
// index is materialised — the seeded-empty convention shared with the other
// registries.
func TestFileGraphStore_LoadMissing(t *testing.T) {
	_, ok, err := NewFileGraphStore(t.TempDir()).LoadGraphIndex()
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if ok {
		t.Error("expected found=false for a workspace with no graph index")
	}
}

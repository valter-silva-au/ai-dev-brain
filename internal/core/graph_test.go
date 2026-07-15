package core

import (
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeGraphSource is an in-memory GraphSource for the graph tests.
type fakeGraphSource struct {
	nodes []GraphNode
	err   error
}

func (f *fakeGraphSource) GraphNodes() ([]GraphNode, error) { return f.nodes, f.err }

// fakeGraphIndexStore is an in-memory GraphIndexStore that records the last
// saved index and can report whether one was ever persisted.
type fakeGraphIndexStore struct {
	saved *models.GraphIndex
}

func (f *fakeGraphIndexStore) SaveGraphIndex(idx models.GraphIndex) error {
	cp := idx
	f.saved = &cp
	return nil
}

func (f *fakeGraphIndexStore) LoadGraphIndex() (models.GraphIndex, bool, error) {
	if f.saved == nil {
		return models.GraphIndex{}, false, nil
	}
	return *f.saved, true, nil
}

func sampleNodes() []GraphNode {
	return []GraphNode{
		{ID: "TASK-00001", Links: []models.Link{
			{Type: models.EdgeDependsOn, Target: "TASK-00002"},
			{Type: models.EdgeRelatesTo, Target: "TASK-00003"},
		}},
		{ID: "TASK-00002", Links: []models.Link{
			{Type: models.EdgePartOf, Target: "some-initiative"},
		}},
		// A node with an unknown-but-tolerated edge type and an incomplete link
		// (empty target) that must be dropped.
		{ID: "TASK-00003", Links: []models.Link{
			{Type: models.EdgeType("mentions"), Target: "TASK-00001"},
			{Type: models.EdgeRelatesTo, Target: ""},
		}},
		{ID: "some-initiative"},
	}
}

// TestGraph_Rebuild_Deterministic proves the derived index is a pure function
// of the declared frontmatter: rebuilding from scratch yields byte-equal edges,
// so deleting the persisted index and rebuilding reconstructs the same graph
// (issue #109 acceptance).
func TestGraph_Rebuild_Deterministic(t *testing.T) {
	src := &fakeGraphSource{nodes: sampleNodes()}
	store := &fakeGraphIndexStore{}
	m := NewGraphManager(src, store)

	if _, err := m.Rebuild(); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if store.saved == nil {
		t.Fatal("Rebuild did not persist the derived index")
	}
	first := *store.saved

	// Simulate "delete the index and rebuild" — a fresh manager over the same
	// authoritative source must reconstruct an identical index.
	store2 := &fakeGraphIndexStore{}
	m2 := NewGraphManager(src, store2)
	if _, err := m2.Rebuild(); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	second := *store2.saved

	if !reflect.DeepEqual(first, second) {
		t.Errorf("rebuild not deterministic:\n first=%+v\nsecond=%+v", first, second)
	}
	// The incomplete (empty-target) link must have been dropped: 4 real edges.
	if len(first.Edges) != 4 {
		t.Errorf("expected 4 edges (empty-target dropped), got %d: %+v", len(first.Edges), first.Edges)
	}
}

// TestGraph_Neighbors_BothDirections verifies incident-edge traversal: a node
// sees edges it declares (outgoing) AND edges declared toward it (incoming).
func TestGraph_Neighbors_BothDirections(t *testing.T) {
	m := NewGraphManager(&fakeGraphSource{nodes: sampleNodes()}, &fakeGraphIndexStore{})

	nb, err := m.Neighbors("TASK-00001")
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	// TASK-00001 declares depends_on TASK-00002 and relates_to TASK-00003
	// (outgoing), and TASK-00003 declares mentions TASK-00001 (incoming) → 3.
	if len(nb) != 3 {
		t.Fatalf("TASK-00001 neighbors=%d want 3: %+v", len(nb), nb)
	}

	// Initiative sees the incoming part_of from TASK-00002.
	initNb, err := m.Neighbors("some-initiative")
	if err != nil {
		t.Fatalf("neighbors: %v", err)
	}
	if len(initNb) != 1 || initNb[0].From != "TASK-00002" || initNb[0].Type != models.EdgePartOf {
		t.Errorf("some-initiative neighbors=%+v want one part_of from TASK-00002", initNb)
	}

	// An unknown node has no neighbours (never panics).
	none, err := m.Neighbors("TASK-99999")
	if err != nil {
		t.Fatalf("neighbors unknown: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("unknown node neighbours=%+v want none", none)
	}
}

// TestGraph_NeighborsByType filters incident edges by edge type.
func TestGraph_NeighborsByType(t *testing.T) {
	m := NewGraphManager(&fakeGraphSource{nodes: sampleNodes()}, &fakeGraphIndexStore{})

	dep, err := m.NeighborsByType("TASK-00001", models.EdgeDependsOn)
	if err != nil {
		t.Fatalf("neighbors-by-type: %v", err)
	}
	if len(dep) != 1 || dep[0].To != "TASK-00002" {
		t.Errorf("depends_on neighbours=%+v want one to TASK-00002", dep)
	}

	// Unknown-but-tolerated type is still queryable.
	mentions, err := m.NeighborsByType("TASK-00001", models.EdgeType("mentions"))
	if err != nil {
		t.Fatalf("neighbors-by-type unknown: %v", err)
	}
	if len(mentions) != 1 || mentions[0].From != "TASK-00003" {
		t.Errorf("mentions neighbours=%+v want one from TASK-00003", mentions)
	}
}

// TestGraph_UnknownEdgeType_Tolerated confirms building over an unknown edge
// type never panics and preserves the edge verbatim.
func TestGraph_UnknownEdgeType_Tolerated(t *testing.T) {
	src := &fakeGraphSource{nodes: []GraphNode{
		{ID: "A", Links: []models.Link{{Type: models.EdgeType("weird_type"), Target: "B"}}},
	}}
	m := NewGraphManager(src, &fakeGraphIndexStore{})
	g, err := m.Graph()
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	edges := g.Neighbors("A")
	if len(edges) != 1 || edges[0].Type != models.EdgeType("weird_type") {
		t.Errorf("unknown edge not preserved: %+v", edges)
	}
}

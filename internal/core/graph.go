package core

import (
	"fmt"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// GraphNode is one entity's contribution to the graph: its id plus the typed
// links it declares in its persisted frontmatter (the source of truth). A node
// with no links still appears so a target-only entity (an initiative nothing
// links out of yet) is a known node.
type GraphNode struct {
	ID    string
	Links []models.Link
}

// GraphSource yields every graph node — each entity and the links it declares.
// It is the seam the GraphManager depends on so core stays ignorant of
// internal/storage; an adapter in internal/app.go bridges it to the backlog
// and the initiative registry (decision D6: the frontmatter is authoritative).
type GraphSource interface {
	GraphNodes() ([]GraphNode, error)
}

// GraphIndexStore persists the DERIVED graph index — a rebuildable cache, never
// a source of truth. core defines it here (where it is consumed); a file-backed
// adapter writes graph/index.yaml. LoadGraphIndex returns found=false (not a
// sentinel error) when no index has been materialised yet, matching the
// StageStore.Get convention.
type GraphIndexStore interface {
	SaveGraphIndex(index models.GraphIndex) error
	LoadGraphIndex() (models.GraphIndex, bool, error)
}

// Graph is the derived, in-memory adjacency structure built from declared
// frontmatter links. It is a cache over the authoritative per-entity Links;
// traversal helpers read incident edges in both directions so a ticket sees
// both the edges it declares (outgoing) and those declared toward it (incoming).
type Graph struct {
	edges    []models.GraphEdge            // canonical, deterministic order
	incident map[string][]models.GraphEdge // id -> edges where id is From or To
}

// buildGraph materialises a Graph from nodes. Links with an empty Type or
// Target are dropped (an edge needs both ends); unknown-but-tolerated edge
// types are preserved verbatim (never rejected, never panics). Edges are sorted
// (From, Type, To) so the derived index is a deterministic function of the
// authoritative frontmatter.
func buildGraph(nodes []GraphNode) *Graph {
	g := &Graph{incident: map[string][]models.GraphEdge{}}
	for _, n := range nodes {
		for _, l := range n.Links {
			if n.ID == "" || l.Type == "" || l.Target == "" {
				continue
			}
			g.edges = append(g.edges, models.GraphEdge{From: n.ID, Type: l.Type, To: l.Target})
		}
	}
	sort.Slice(g.edges, func(i, j int) bool {
		a, b := g.edges[i], g.edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.To < b.To
	})
	for _, e := range g.edges {
		g.incident[e.From] = append(g.incident[e.From], e)
		if e.To != e.From {
			g.incident[e.To] = append(g.incident[e.To], e)
		}
	}
	return g
}

// Index returns the derived index (the flat, sorted edge list) for persistence.
func (g *Graph) Index() models.GraphIndex {
	edges := make([]models.GraphEdge, len(g.edges))
	copy(edges, g.edges)
	return models.GraphIndex{Edges: edges}
}

// Neighbors returns every edge incident to id — edges id declares (id is From)
// and edges declared toward id (id is To). The returned slice is a copy in the
// canonical deterministic order; an unknown id yields an empty slice, never a
// panic.
func (g *Graph) Neighbors(id string) []models.GraphEdge {
	in := g.incident[id]
	out := make([]models.GraphEdge, len(in))
	copy(out, in)
	return out
}

// NeighborsByType is Neighbors filtered to edge type t (which may be an
// unknown-but-tolerated type — it is matched literally).
func (g *Graph) NeighborsByType(id string, t models.EdgeType) []models.GraphEdge {
	var out []models.GraphEdge
	for _, e := range g.incident[id] {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// GraphManager builds and queries the typed edge graph. Per the house
// convention the constructor returns the interface, so callers depend on
// behaviour, not the concrete type. Traversal helpers always derive from the
// authoritative source (never the cache) so a stale persisted index can never
// return wrong neighbours; the persisted index is materialised only by Rebuild.
type GraphManager interface {
	// Graph derives a fresh graph from the authoritative frontmatter links.
	Graph() (*Graph, error)
	// Rebuild derives a fresh graph AND persists the derived index cache
	// (graph/index.yaml). Reconstructs from scratch, ignoring any existing
	// index, so deleting the index and rebuilding yields the same graph.
	Rebuild() (*Graph, error)
	// Neighbors returns edges incident to id (both directions), fresh from the
	// authoritative source.
	Neighbors(id string) ([]models.GraphEdge, error)
	// NeighborsByType is Neighbors filtered to edge type t.
	NeighborsByType(id string, t models.EdgeType) ([]models.GraphEdge, error)
}

type graphManager struct {
	source GraphSource
	store  GraphIndexStore
}

// NewGraphManager returns a GraphManager backed by source (authoritative
// frontmatter) and store (the derived-index cache). store may be nil — Rebuild
// then computes the graph without persisting it.
func NewGraphManager(source GraphSource, store GraphIndexStore) GraphManager {
	return &graphManager{source: source, store: store}
}

func (m *graphManager) Graph() (*Graph, error) {
	nodes, err := m.source.GraphNodes()
	if err != nil {
		return nil, fmt.Errorf("load graph nodes: %w", err)
	}
	return buildGraph(nodes), nil
}

func (m *graphManager) Rebuild() (*Graph, error) {
	g, err := m.Graph()
	if err != nil {
		return nil, err
	}
	if m.store != nil {
		if err := m.store.SaveGraphIndex(g.Index()); err != nil {
			return nil, fmt.Errorf("persist graph index: %w", err)
		}
	}
	return g, nil
}

func (m *graphManager) Neighbors(id string) ([]models.GraphEdge, error) {
	g, err := m.Graph()
	if err != nil {
		return nil, err
	}
	return g.Neighbors(id), nil
}

func (m *graphManager) NeighborsByType(id string, t models.EdgeType) ([]models.GraphEdge, error) {
	g, err := m.Graph()
	if err != nil {
		return nil, err
	}
	return g.NeighborsByType(id, t), nil
}

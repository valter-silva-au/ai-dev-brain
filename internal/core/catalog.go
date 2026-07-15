package core

import (
	"fmt"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// CatalogSource yields the workspace's entity collections for the catalog. It is
// the seam the CatalogBuilder depends on so core stays ignorant of
// internal/storage; an adapter in internal/app.go bridges it to the org/
// initiative registries, the backlog, the ingested-node registry, and the metric
// registry. Each method returns the entities in their registry order — the
// builder sorts deterministically.
type CatalogSource interface {
	Organizations() ([]models.Organization, error)
	Initiatives() ([]models.Initiative, error)
	Tickets() ([]models.Task, error)
	IngestedNodes() ([]models.IngestedNode, error)
	Metrics() ([]models.Metric, error)
	ADRs() ([]models.ADR, error)
}

// CatalogService builds a Backstage-style entity catalog: one generated,
// queryable inventory of every entity in the workspace, derived from the
// registries and annotated with graph degree from the #109 graph.
type CatalogService interface {
	// Build assembles the full catalog. Entries are sorted by id and the graph
	// degree of each entity is computed from a fresh graph (never a stale cache).
	Build() (*models.Catalog, error)
}

type catalogBuilder struct {
	source CatalogSource
	graph  GraphManager
}

// NewCatalogBuilder returns a CatalogService backed by source (the registries)
// and graph (the derived graph, for degree annotation). graph may be nil — every
// entity then reports zero edges.
func NewCatalogBuilder(source CatalogSource, graph GraphManager) CatalogService {
	return &catalogBuilder{source: source, graph: graph}
}

func (b *catalogBuilder) Build() (*models.Catalog, error) {
	orgs, err := b.source.Organizations()
	if err != nil {
		return nil, fmt.Errorf("catalog: load organizations: %w", err)
	}
	inits, err := b.source.Initiatives()
	if err != nil {
		return nil, fmt.Errorf("catalog: load initiatives: %w", err)
	}
	tickets, err := b.source.Tickets()
	if err != nil {
		return nil, fmt.Errorf("catalog: load tickets: %w", err)
	}
	nodes, err := b.source.IngestedNodes()
	if err != nil {
		return nil, fmt.Errorf("catalog: load ingested nodes: %w", err)
	}
	metrics, err := b.source.Metrics()
	if err != nil {
		return nil, fmt.Errorf("catalog: load metrics: %w", err)
	}
	adrs, err := b.source.ADRs()
	if err != nil {
		return nil, fmt.Errorf("catalog: load ADRs: %w", err)
	}

	// Derive a fresh graph so degree annotations reflect the authoritative
	// frontmatter, not a possibly-stale index cache. A nil graph (or an error)
	// degrades to zero degrees rather than failing the whole catalog.
	var g *Graph
	if b.graph != nil {
		if g, err = b.graph.Graph(); err != nil {
			return nil, fmt.Errorf("catalog: derive graph: %w", err)
		}
	}
	degree := func(id string) int {
		if g == nil {
			return 0
		}
		return len(g.Neighbors(id))
	}

	// Org rollup: how many initiatives name each org.
	initsByOrg := map[string]int{}
	for _, in := range inits {
		initsByOrg[in.OrgID]++
	}
	// Initiative rollup: how many tickets name each initiative.
	ticketsByInit := map[string]int{}
	for _, t := range tickets {
		if t.Initiative != "" {
			ticketsByInit[t.Initiative]++
		}
	}

	cat := &models.Catalog{
		Orgs:          make([]models.CatalogOrg, 0, len(orgs)),
		Initiatives:   make([]models.CatalogInitiative, 0, len(inits)),
		Tickets:       make([]models.CatalogTicket, 0, len(tickets)),
		IngestedNodes: make([]models.CatalogNode, 0, len(nodes)),
		Metrics:       make([]models.CatalogMetric, 0, len(metrics)),
		ADRs:          make([]models.CatalogADR, 0, len(adrs)),
	}

	for _, o := range orgs {
		cat.Orgs = append(cat.Orgs, models.CatalogOrg{
			ID:          o.ID,
			Name:        o.Name,
			GitHost:     o.GitHost,
			Initiatives: initsByOrg[o.ID],
			Edges:       degree(o.ID),
		})
	}
	for _, in := range inits {
		cat.Initiatives = append(cat.Initiatives, models.CatalogInitiative{
			ID:   in.ID,
			Name: in.Name,
			Org:  in.OrgID,
			// gateSummary lives in taskmanager.go (same package) — reused here so
			// the catalog renders a gate exactly as the AI context does.
			Stage:   string(in.Stage),
			Gate:    gateSummary(in.Gate),
			Tickets: ticketsByInit[in.ID],
			Edges:   degree(in.ID),
		})
	}
	for _, t := range tickets {
		cat.Tickets = append(cat.Tickets, models.CatalogTicket{
			ID:         t.ID,
			Title:      t.Title,
			Type:       string(t.Type),
			Status:     string(t.Status),
			Initiative: t.Initiative,
			Edges:      degree(t.ID),
		})
	}
	for _, n := range nodes {
		cat.IngestedNodes = append(cat.IngestedNodes, models.CatalogNode{
			ID:     n.ID,
			Type:   n.Type,
			Title:  n.Title,
			Source: n.Source,
			Edges:  degree(n.ID),
		})
	}
	for _, m := range metrics {
		cat.Metrics = append(cat.Metrics, models.CatalogMetric{
			ID:         m.GraphID(),
			Initiative: m.Initiative,
			Name:       m.Name,
			Value:      m.Value,
			Unit:       m.Unit,
			Source:     m.Source,
			Edges:      degree(m.GraphID()),
		})
	}
	for _, a := range adrs {
		cat.ADRs = append(cat.ADRs, models.CatalogADR{
			ID:     a.GraphID(),
			Number: a.Number,
			Title:  a.Title,
			Status: string(a.Status),
			Edges:  degree(a.GraphID()),
		})
	}

	// Deterministic order (registry order is not guaranteed stable across runs).
	sort.Slice(cat.Orgs, func(i, j int) bool { return cat.Orgs[i].ID < cat.Orgs[j].ID })
	sort.Slice(cat.Initiatives, func(i, j int) bool { return cat.Initiatives[i].ID < cat.Initiatives[j].ID })
	sort.Slice(cat.Tickets, func(i, j int) bool { return cat.Tickets[i].ID < cat.Tickets[j].ID })
	sort.Slice(cat.IngestedNodes, func(i, j int) bool { return cat.IngestedNodes[i].ID < cat.IngestedNodes[j].ID })
	sort.Slice(cat.Metrics, func(i, j int) bool { return cat.Metrics[i].ID < cat.Metrics[j].ID })
	sort.Slice(cat.ADRs, func(i, j int) bool { return cat.ADRs[i].Number < cat.ADRs[j].Number })

	totalEdges := 0
	if g != nil {
		totalEdges = len(g.Index().Edges)
	}
	cat.Summary = models.CatalogSummary{
		Orgs:          len(cat.Orgs),
		Initiatives:   len(cat.Initiatives),
		Tickets:       len(cat.Tickets),
		IngestedNodes: len(cat.IngestedNodes),
		Metrics:       len(cat.Metrics),
		ADRs:          len(cat.ADRs),
		Edges:         totalEdges,
	}
	return cat, nil
}

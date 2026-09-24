package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeCatalogSource is an in-memory CatalogSource for the builder tests.
type fakeCatalogSource struct {
	orgs    []models.Organization
	inits   []models.Initiative
	tasks   []models.Task
	nodes   []models.IngestedNode
	metrics []models.Metric
	adrs    []models.ADR
	debt    []models.DebtItem
}

func (f *fakeCatalogSource) Organizations() ([]models.Organization, error) { return f.orgs, nil }
func (f *fakeCatalogSource) Initiatives() ([]models.Initiative, error)     { return f.inits, nil }
func (f *fakeCatalogSource) Tickets() ([]models.Task, error)               { return f.tasks, nil }
func (f *fakeCatalogSource) IngestedNodes() ([]models.IngestedNode, error) { return f.nodes, nil }
func (f *fakeCatalogSource) Metrics() ([]models.Metric, error)             { return f.metrics, nil }
func (f *fakeCatalogSource) ADRs() ([]models.ADR, error)                   { return f.adrs, nil }
func (f *fakeCatalogSource) Debt() ([]models.DebtItem, error)              { return f.debt, nil }

// fakeGraphSource yields fixed graph nodes for the derived-graph seam.
type fakeCatalogGraphSource struct{ nodes []GraphNode }

func (f *fakeCatalogGraphSource) GraphNodes() ([]GraphNode, error) { return f.nodes, nil }

func TestCatalogBuilder_Build(t *testing.T) {
	src := &fakeCatalogSource{
		orgs: []models.Organization{{ID: "acme", Name: "Acme", GitHost: "github.com"}},
		inits: []models.Initiative{
			{ID: "onboarding", Name: "Onboarding", OrgID: "acme", Stage: models.StageMVP,
				Links: []models.Link{{Type: models.EdgePartOf, Target: "acme"}}},
			{ID: "growth", Name: "Growth", OrgID: "acme", Stage: models.StageIdea,
				Links: []models.Link{{Type: models.EdgePartOf, Target: "acme"}}},
		},
		tasks: []models.Task{
			{ID: "TASK-1", Title: "wire onboarding", Type: models.TaskTypeFeat, Status: models.TaskStatusInProgress,
				Initiative: "onboarding", Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "onboarding"}}},
			{ID: "TASK-2", Title: "loose ticket", Type: models.TaskTypeChore, Status: models.TaskStatusBacklog},
		},
		nodes: []models.IngestedNode{
			{ID: "stakeholder:jane", Type: "Stakeholder", Title: "Jane"},
		},
		metrics: []models.Metric{
			{Initiative: "onboarding", Name: "sean-ellis", Value: 42, Unit: "%", Source: "manual"},
		},
	}

	// The graph must know the same nodes+links so the catalog can annotate degree.
	graphSrc := &fakeCatalogGraphSource{nodes: []GraphNode{
		{ID: "onboarding", Links: []models.Link{{Type: models.EdgePartOf, Target: "acme"}}},
		{ID: "growth", Links: []models.Link{{Type: models.EdgePartOf, Target: "acme"}}},
		{ID: "TASK-1", Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "onboarding"}}},
		{ID: "metric:onboarding:sean-ellis", Links: []models.Link{{Type: models.EdgePartOf, Target: "onboarding"}}},
	}}
	gm := NewGraphManager(graphSrc, nil)

	builder := NewCatalogBuilder(src, gm)
	cat, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Summary counts.
	if cat.Summary.Orgs != 1 || cat.Summary.Initiatives != 2 || cat.Summary.Tickets != 2 ||
		cat.Summary.IngestedNodes != 1 || cat.Summary.Metrics != 1 {
		t.Errorf("summary = %+v", cat.Summary)
	}
	// Total edges: onboarding->acme, growth->acme, TASK-1->onboarding, metric->onboarding = 4.
	if cat.Summary.Edges != 4 {
		t.Errorf("Summary.Edges = %d, want 4", cat.Summary.Edges)
	}

	// Org rollup: acme has 2 initiatives and 2 incident edges (both part_of it).
	if len(cat.Orgs) != 1 {
		t.Fatalf("want 1 org, got %d", len(cat.Orgs))
	}
	if cat.Orgs[0].Initiatives != 2 {
		t.Errorf("org initiatives = %d, want 2", cat.Orgs[0].Initiatives)
	}
	if cat.Orgs[0].Edges != 2 {
		t.Errorf("org edges = %d, want 2", cat.Orgs[0].Edges)
	}

	// Initiatives sorted by ID: growth, onboarding.
	if cat.Initiatives[0].ID != "growth" || cat.Initiatives[1].ID != "onboarding" {
		t.Errorf("initiatives not sorted by id: %q, %q", cat.Initiatives[0].ID, cat.Initiatives[1].ID)
	}
	// Onboarding rolls up 1 ticket and carries its stage + 2 edges (part_of acme, relates_to from TASK-1).
	var onboarding *models.CatalogInitiative
	for i := range cat.Initiatives {
		if cat.Initiatives[i].ID == "onboarding" {
			onboarding = &cat.Initiatives[i]
		}
	}
	if onboarding == nil {
		t.Fatal("onboarding initiative missing from catalog")
	}
	if onboarding.Tickets != 1 {
		t.Errorf("onboarding tickets = %d, want 1", onboarding.Tickets)
	}
	if onboarding.Stage != string(models.StageMVP) {
		t.Errorf("onboarding stage = %q, want MVP", onboarding.Stage)
	}
	// Incident edges: part_of->acme, relates_to from TASK-1, part_of from the metric = 3.
	if onboarding.Edges != 3 {
		t.Errorf("onboarding edges = %d, want 3", onboarding.Edges)
	}

	// Metric entry uses the deterministic graph id.
	if len(cat.Metrics) != 1 || cat.Metrics[0].ID != "metric:onboarding:sean-ellis" {
		t.Errorf("metric entry = %+v", cat.Metrics)
	}
	if cat.Metrics[0].Value != 42 {
		t.Errorf("metric value = %v, want 42", cat.Metrics[0].Value)
	}
}

func TestCatalogBuilder_SurfacesADRs(t *testing.T) {
	src := &fakeCatalogSource{
		adrs: []models.ADR{
			{Number: 2, Title: "Second", Status: models.ADRProposed},
			{Number: 1, Title: "First", Status: models.ADRAccepted,
				Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "TASK-1"}}},
		},
	}
	// The graph knows adr:0001's relates_to edge, so its degree is 1.
	graphSrc := &fakeCatalogGraphSource{nodes: []GraphNode{
		{ID: "adr:0001", Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "TASK-1"}}},
	}}
	cat, err := NewCatalogBuilder(src, NewGraphManager(graphSrc, nil)).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cat.Summary.ADRs != 2 {
		t.Errorf("Summary.ADRs = %d, want 2", cat.Summary.ADRs)
	}
	// Sorted by number: adr 1 then adr 2.
	if len(cat.ADRs) != 2 || cat.ADRs[0].Number != 1 || cat.ADRs[1].Number != 2 {
		t.Fatalf("ADRs not sorted by number: %+v", cat.ADRs)
	}
	if cat.ADRs[0].ID != "adr:0001" || cat.ADRs[0].Status != string(models.ADRAccepted) {
		t.Errorf("adr entry = %+v", cat.ADRs[0])
	}
	if cat.ADRs[0].Edges != 1 {
		t.Errorf("adr:0001 degree = %d, want 1", cat.ADRs[0].Edges)
	}
}

// TestCatalogBuilder_SurfacesDebt is the debt half of the ADR test above: a
// tech-debt item is a first-class graph node (debt:DEBT-NNNN) and a catalog
// entry, so the registry `adb debt` writes is readable by everything that reads
// the catalog. Before this, debt was the one governance registry nothing read.
func TestCatalogBuilder_SurfacesDebt(t *testing.T) {
	src := &fakeCatalogSource{
		debt: []models.DebtItem{
			// Registry order is deliberately NOT id order, so the builder's sort
			// is what produces the deterministic output below.
			{ID: "DEBT-0002", Title: "flat knowledge scan", Priority: models.PriorityP2, Status: models.DebtOpen},
			{ID: "DEBT-0001", Title: "panics on nil graph", Priority: models.PriorityP0, Status: models.DebtOpen,
				Area: "core", Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "TASK-1"}}},
			{ID: "DEBT-0003", Title: "done already", Priority: models.PriorityP1, Status: models.DebtResolved},
		},
	}
	// The graph knows debt:DEBT-0001's relates_to edge, so its degree is 1;
	// DEBT-0002 declares no links and is therefore a degree-0 node.
	graphSrc := &fakeCatalogGraphSource{nodes: []GraphNode{
		{ID: "debt:DEBT-0001", Links: []models.Link{{Type: models.EdgeRelatesTo, Target: "TASK-1"}}},
		{ID: "debt:DEBT-0002"},
	}}
	cat, err := NewCatalogBuilder(src, NewGraphManager(graphSrc, nil)).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cat.Summary.Debt != 3 {
		t.Errorf("Summary.Debt = %d, want 3", cat.Summary.Debt)
	}
	if len(cat.Debt) != 3 {
		t.Fatalf("want 3 debt entries, got %+v", cat.Debt)
	}
	// Sorted by id (deterministic), not registry order.
	wantIDs := []string{"debt:DEBT-0001", "debt:DEBT-0002", "debt:DEBT-0003"}
	for i, want := range wantIDs {
		if cat.Debt[i].ID != want {
			t.Errorf("Debt[%d].ID = %q, want %q", i, cat.Debt[i].ID, want)
		}
	}
	first := cat.Debt[0]
	if first.Title != "panics on nil graph" || first.Priority != string(models.PriorityP0) ||
		first.Status != string(models.DebtOpen) || first.Area != "core" {
		t.Errorf("debt entry = %+v", first)
	}
	if first.Edges != 1 {
		t.Errorf("debt:DEBT-0001 degree = %d, want 1", first.Edges)
	}
	// A linkless debt item is still a catalog entry — degree 0, not absent.
	if cat.Debt[1].Edges != 0 {
		t.Errorf("debt:DEBT-0002 degree = %d, want 0", cat.Debt[1].Edges)
	}
}

func TestCatalogBuilder_Empty(t *testing.T) {
	builder := NewCatalogBuilder(&fakeCatalogSource{}, NewGraphManager(&fakeCatalogGraphSource{}, nil))
	cat, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cat.Summary.Orgs != 0 || cat.Summary.Edges != 0 {
		t.Errorf("empty catalog summary = %+v", cat.Summary)
	}
	// Slices are non-nil so JSON renders [] not null.
	if cat.Orgs == nil || cat.Tickets == nil || cat.Metrics == nil || cat.Debt == nil {
		t.Error("expected non-nil empty slices for stable JSON")
	}
}

package models

// Catalog is a Backstage-style inventory of every entity in the workspace,
// derived from the registries + the #109 typed graph. It is a GENERATED,
// read-only snapshot — orgs, initiatives, tickets, ingested nodes, and metric
// nodes — each annotated with its graph degree (the count of incident edges) so
// the catalog doubles as a graph overview. It carries no timestamp so the same
// workspace always yields byte-identical output (deterministic for diffs/tests);
// callers that want a "generated at" line stamp it at render time.
type Catalog struct {
	Orgs          []CatalogOrg        `yaml:"orgs" json:"orgs"`
	Initiatives   []CatalogInitiative `yaml:"initiatives" json:"initiatives"`
	Tickets       []CatalogTicket     `yaml:"tickets" json:"tickets"`
	IngestedNodes []CatalogNode       `yaml:"ingested_nodes" json:"ingested_nodes"`
	Metrics       []CatalogMetric     `yaml:"metrics" json:"metrics"`
	ADRs          []CatalogADR        `yaml:"adrs" json:"adrs"`
	Summary       CatalogSummary      `yaml:"summary" json:"summary"`
}

// CatalogSummary is the count rollup across the whole catalog.
type CatalogSummary struct {
	Orgs          int `yaml:"orgs" json:"orgs"`
	Initiatives   int `yaml:"initiatives" json:"initiatives"`
	Tickets       int `yaml:"tickets" json:"tickets"`
	IngestedNodes int `yaml:"ingested_nodes" json:"ingested_nodes"`
	Metrics       int `yaml:"metrics" json:"metrics"`
	ADRs          int `yaml:"adrs" json:"adrs"`
	Edges         int `yaml:"edges" json:"edges"`
}

// CatalogADR is one architecture decision record's catalog entry.
type CatalogADR struct {
	ID     string `yaml:"id" json:"id"` // the ADR graph id (adr:NNNN)
	Number int    `yaml:"number" json:"number"`
	Title  string `yaml:"title" json:"title"`
	Status string `yaml:"status" json:"status"`
	Edges  int    `yaml:"edges" json:"edges"`
}

// CatalogOrg is one organization's catalog entry with its rollups.
type CatalogOrg struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	GitHost     string `yaml:"git_host,omitempty" json:"git_host,omitempty"`
	Initiatives int    `yaml:"initiatives" json:"initiatives"` // initiatives whose org_id is this org
	Edges       int    `yaml:"edges" json:"edges"`             // incident graph edges
}

// CatalogInitiative is one initiative's catalog entry with its stage + rollups.
type CatalogInitiative struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Org     string `yaml:"org,omitempty" json:"org,omitempty"`
	Stage   string `yaml:"stage,omitempty" json:"stage,omitempty"`
	Gate    string `yaml:"gate,omitempty" json:"gate,omitempty"` // e.g. "Idea->MVP passed" / "blocked"
	Tickets int    `yaml:"tickets" json:"tickets"`               // tickets whose initiative is this one
	Edges   int    `yaml:"edges" json:"edges"`
}

// CatalogTicket is one ticket's catalog entry.
type CatalogTicket struct {
	ID         string `yaml:"id" json:"id"`
	Title      string `yaml:"title,omitempty" json:"title,omitempty"`
	Type       string `yaml:"type,omitempty" json:"type,omitempty"`
	Status     string `yaml:"status,omitempty" json:"status,omitempty"`
	Initiative string `yaml:"initiative,omitempty" json:"initiative,omitempty"`
	Edges      int    `yaml:"edges" json:"edges"`
}

// CatalogNode is one ingested (typed) node's catalog entry.
type CatalogNode struct {
	ID     string `yaml:"id" json:"id"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Title  string `yaml:"title,omitempty" json:"title,omitempty"`
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	Edges  int    `yaml:"edges" json:"edges"`
}

// CatalogMetric is one metric node's catalog entry, keyed by its graph id.
type CatalogMetric struct {
	ID         string  `yaml:"id" json:"id"` // the metric's graph id (metric:<initiative>:<name>)
	Initiative string  `yaml:"initiative" json:"initiative"`
	Name       string  `yaml:"name" json:"name"`
	Value      float64 `yaml:"value" json:"value"`
	Unit       string  `yaml:"unit,omitempty" json:"unit,omitempty"`
	Source     string  `yaml:"source,omitempty" json:"source,omitempty"`
	Edges      int     `yaml:"edges" json:"edges"`
}

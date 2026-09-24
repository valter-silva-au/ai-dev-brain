package models

import "time"

// DebtStatus is the triage state of a tech-debt item.
type DebtStatus string

const (
	// DebtOpen: an outstanding tech-debt / architecture-audit item.
	DebtOpen DebtStatus = "open"
	// DebtResolved: the item has been addressed.
	DebtResolved DebtStatus = "resolved"
)

// DebtGraphPrefix is the node-id prefix a tech-debt item carries in the typed
// graph (#109), so `debt:` is spelled once. Every producer and consumer of a
// debt node id — GraphID, the graph source, the catalog, `adb graph neighbors` —
// goes through this const rather than a literal, which is what keeps the graph
// and the catalog talking about the same node.
const DebtGraphPrefix = "debt:"

// DebtItem is one architecture-audit / tech-debt entry (#128 step 16). It is a
// lightweight, priority-triageable record kept in a workspace registry
// (debt/index.yaml) — distinct from a full ticket, so an audit can enumerate
// debt without minting worktrees. Priority reuses the task Priority scale.
//
// A debt item participates in the #109 graph as a `debt:DEBT-NNNN` node, exactly
// as an ADR participates as `adr:NNNN`, so the debt it records can link the
// ticket or initiative it is owed against and shows up in `adb catalog`.
type DebtItem struct {
	ID       string     `yaml:"id" json:"id"` // DEBT-NNNN
	Title    string     `yaml:"title" json:"title"`
	Priority Priority   `yaml:"priority" json:"priority"`
	Status   DebtStatus `yaml:"status" json:"status"`
	Area     string     `yaml:"area,omitempty" json:"area,omitempty"` // optional subsystem/package
	Note     string     `yaml:"note,omitempty" json:"note,omitempty"`
	Created  time.Time  `yaml:"created" json:"created"`
	Resolved *time.Time `yaml:"resolved,omitempty" json:"resolved,omitempty"`

	// Links are the typed graph edges the item declares (e.g. relates_to the
	// ticket the debt was incurred in). Source of truth for the graph (D6);
	// omitempty so a registry written before debt joined the graph is unchanged.
	Links []Link `yaml:"links,omitempty" json:"links,omitempty"`
}

// GraphID is the item's node id in the typed graph: debt:DEBT-NNNN. The id is
// already zero-padded by the store, so lexical order over graph ids is numeric
// order.
func (d DebtItem) GraphID() string { return DebtGraphPrefix + d.ID }

// DebtIndex is the tech-debt registry document (debt/index.yaml).
type DebtIndex struct {
	Items []DebtItem `yaml:"items" json:"items"`
}

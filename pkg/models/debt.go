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

// DebtItem is one architecture-audit / tech-debt entry (#128 step 16). It is a
// lightweight, priority-triageable record kept in a workspace registry
// (debt/index.yaml) — distinct from a full ticket, so an audit can enumerate
// debt without minting worktrees. Priority reuses the task Priority scale.
type DebtItem struct {
	ID       string     `yaml:"id" json:"id"` // DEBT-NNNN
	Title    string     `yaml:"title" json:"title"`
	Priority Priority   `yaml:"priority" json:"priority"`
	Status   DebtStatus `yaml:"status" json:"status"`
	Area     string     `yaml:"area,omitempty" json:"area,omitempty"` // optional subsystem/package
	Note     string     `yaml:"note,omitempty" json:"note,omitempty"`
	Created  time.Time  `yaml:"created" json:"created"`
	Resolved *time.Time `yaml:"resolved,omitempty" json:"resolved,omitempty"`
}

// DebtIndex is the tech-debt registry document (debt/index.yaml).
type DebtIndex struct {
	Items []DebtItem `yaml:"items" json:"items"`
}

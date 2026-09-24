package models

import (
	"fmt"
	"time"
)

// ADRStatus is the lifecycle state of an architecture decision record (MADR).
type ADRStatus string

const (
	// ADRProposed: the decision is drafted but not yet accepted.
	ADRProposed ADRStatus = "proposed"
	// ADRAccepted: the decision is in force.
	ADRAccepted ADRStatus = "accepted"
	// ADRRejected: the decision was considered and declined.
	ADRRejected ADRStatus = "rejected"
	// ADRSuperseded: a later ADR replaces this one.
	ADRSuperseded ADRStatus = "superseded"
	// ADRDeprecated: no longer relevant (e.g. the subsystem was removed).
	ADRDeprecated ADRStatus = "deprecated"
)

// ValidADRStatuses is the canonical set of ADR statuses (display order).
var ValidADRStatuses = []ADRStatus{ADRProposed, ADRAccepted, ADRRejected, ADRSuperseded, ADRDeprecated}

// ADRGraphPrefix is the node-id prefix an ADR carries in the typed graph (#109),
// so `adr:` is spelled once — the same convention DebtGraphPrefix establishes for
// debt items. It matters more here than for debt because two packages sit on
// opposite sides of it: GraphID FORMATS `adr:NNNN` and the CLI's parseADRNumber
// PARSES it back, so a divergence would leave an id the tool prints but cannot
// resolve. Note the prefix is the only part that is a const — the `%04d`
// zero-padding stays in GraphID, because it is the shape the catalog and the
// derived graph index already store and changing it would orphan every existing
// ADR node.
const ADRGraphPrefix = "adr:"

// IsValid reports whether s is one of the canonical statuses.
func (s ADRStatus) IsValid() bool {
	for _, v := range ValidADRStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// ADR is a Markdown Any Decision Record (MADR). The registry (adr/index.yaml)
// is the authoritative index — number, title, status, and typed links — while
// the human-authored decision body lives in docs/adr/NNNN-<slug>.md. Status is
// held on the registry (not sniffed from the markdown) so `adb adr set-status`
// is a single, unambiguous write. An ADR participates in the #109 graph as an
// `adr:NNNN` node so it can link the initiative/ticket it decides for.
type ADR struct {
	Number  int       `yaml:"number" json:"number"`
	Title   string    `yaml:"title" json:"title"`
	Status  ADRStatus `yaml:"status" json:"status"`
	Slug    string    `yaml:"slug" json:"slug"`
	Created time.Time `yaml:"created" json:"created"`
	Updated time.Time `yaml:"updated" json:"updated"`

	// Links are the typed graph edges the ADR declares (e.g. relates_to a
	// ticket, part_of an initiative). Source of truth for the graph (D6).
	Links []Link `yaml:"links,omitempty" json:"links,omitempty"`
}

// GraphID is the ADR's node id in the typed graph: adr:NNNN (zero-padded to a
// minimum of 4 digits, so lexical order over graph ids is numeric order for the
// first 9999 records). The prefix comes from ADRGraphPrefix so the id the CLI
// parses back is composed from the same const.
func (a ADR) GraphID() string { return ADRGraphPrefix + fmt.Sprintf("%04d", a.Number) }

// Filename is the ADR's markdown filename under docs/adr/ (NNNN-<slug>.md).
func (a ADR) Filename() string { return fmt.Sprintf("%04d-%s.md", a.Number, a.Slug) }

// ADRIndex is the ADR registry document (adr/index.yaml).
type ADRIndex struct {
	ADRs []ADR `yaml:"adrs" json:"adrs"`
}

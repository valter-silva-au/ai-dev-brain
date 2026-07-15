package models

// Drift finding kinds. Each classifies one way an entity has drifted from its
// template or catalog expectations (#128 conformance-drift check).
const (
	// DriftStaleTemplate: the workspace was scaffolded from an older template
	// version than the one now shipping (a candidate for `adb init update`).
	DriftStaleTemplate = "stale-template"
	// DriftMissingFile: a file the template manifest recorded as scaffolded is no
	// longer present on disk.
	DriftMissingFile = "missing-file"
	// DriftDanglingOrg: an initiative names an org that is not in the registry.
	DriftDanglingOrg = "dangling-org"
	// DriftDanglingInitiative: a ticket or metric names an initiative that is not
	// in the registry.
	DriftDanglingInitiative = "dangling-initiative"
)

// DriftFinding is one conformance issue: an entity drifting from an expectation.
type DriftFinding struct {
	Entity string `yaml:"entity" json:"entity"` // the drifting entity id or workspace-relative path
	Kind   string `yaml:"kind" json:"kind"`     // one of the Drift* constants
	Detail string `yaml:"detail" json:"detail"` // human-readable explanation
}

// DriftReport is the result of a conformance-drift check: the set of findings
// (empty when the workspace conforms).
type DriftReport struct {
	Findings []DriftFinding `yaml:"findings" json:"findings"`
}

// HasDrift reports whether any finding was raised.
func (r DriftReport) HasDrift() bool { return len(r.Findings) > 0 }

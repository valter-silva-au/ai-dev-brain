package models

import "time"

// Stage is the founder-playbook lifecycle stage carried by an Initiative. It is
// ORTHOGONAL to TaskStatus: a Stage describes where a business initiative sits on
// the Idea -> MVP -> Launch -> Scale journey, whereas TaskStatus tracks a single
// ticket's backlog -> done lifecycle. The two never substitute for one another.
type Stage string

const (
	// StageIdea is the earliest stage: the initiative is an unvalidated idea.
	StageIdea Stage = "Idea"
	// StageMVP is building the minimum viable product.
	StageMVP Stage = "MVP"
	// StageLaunch is taking the product to market.
	StageLaunch Stage = "Launch"
	// StageScale is growing a launched product.
	StageScale Stage = "Scale"
)

// ValidStages is the ordered, canonical set of stages an Initiative may occupy.
var ValidStages = []Stage{StageIdea, StageMVP, StageLaunch, StageScale}

// IsValid reports whether s is one of the four canonical stages.
func (s Stage) IsValid() bool {
	for _, valid := range ValidStages {
		if s == valid {
			return true
		}
	}
	return false
}

// Organization is a business tracked in the workspace. It defaults to the git-host
// org and may later span git orgs. It is METADATA held in a workspace-level registry
// (orgs/index.yaml) — it is NOT part of the physical ticket/worktree path layout, so
// introducing organizations requires no migration of the existing
// tickets/<platform>/<org>/<repo> correlation layout.
type Organization struct {
	ID      string    `yaml:"id" json:"id"`
	Name    string    `yaml:"name" json:"name"`
	GitHost string    `yaml:"git_host,omitempty" json:"git_host,omitempty"`
	Created time.Time `yaml:"created" json:"created"`
}

// Initiative belongs to exactly one Organization and carries a Stage. Like
// Organization it is registry METADATA (initiatives/index.yaml), not a path segment.
type Initiative struct {
	ID      string    `yaml:"id" json:"id"`
	Name    string    `yaml:"name" json:"name"`
	OrgID   string    `yaml:"org_id" json:"org_id"`
	Stage   Stage     `yaml:"stage" json:"stage"`
	Created time.Time `yaml:"created" json:"created"`
	Updated time.Time `yaml:"updated" json:"updated"`

	// Gate is the most recent stage-gate evaluation recorded on this initiative.
	// It persists durably across sessions (initiatives/index.yaml). Nil until the
	// first `adb stage advance`; omitempty keeps pre-gate initiatives
	// byte-identical on marshal.
	Gate *GateState `yaml:"gate,omitempty" json:"gate,omitempty"`

	// Links are the typed graph edges this initiative declares toward other
	// entities (decision D6) — e.g. a `part_of` toward an org, or `relates_to`
	// toward a peer initiative. They are the source of truth for the graph; the
	// derived index is a rebuildable cache. omitempty keeps pre-graph
	// initiatives byte-identical on marshal.
	Links []Link `yaml:"links,omitempty" json:"links,omitempty"`
}

// GateItemStatus is the evaluated status of a single evidence-bundle item.
type GateItemStatus string

const (
	// GateItemMet means a deterministic check is satisfied.
	GateItemMet GateItemStatus = "met"
	// GateItemMissing means a deterministic check is unmet — it blocks the advance.
	GateItemMissing GateItemStatus = "missing"
	// GateItemPending means a judgment (adversarial) item for which no verdict is
	// available yet. Pending items are informational and NEVER block, so the gate
	// degrades gracefully when the adversarial verdict source is absent or silent.
	GateItemPending GateItemStatus = "pending"
	// GateItemFailed means a judgment (adversarial) verdict came back NEGATIVE —
	// the evidence did not survive scrutiny. Like a missing deterministic item, a
	// failed judgment BLOCKS the advance; it is distinct from "missing" so reports
	// can tell a negative verdict apart from absent evidence.
	GateItemFailed GateItemStatus = "failed"
)

// GateItemState is the recorded evaluation of one evidence-bundle item.
type GateItemState struct {
	ID     string         `yaml:"id" json:"id"`
	Desc   string         `yaml:"desc" json:"desc"`
	Kind   string         `yaml:"kind" json:"kind"` // "deterministic" | "judgment"
	Status GateItemStatus `yaml:"status" json:"status"`
	Detail string         `yaml:"detail,omitempty" json:"detail,omitempty"`
}

// GateState is the durable record of a stage gate's most recent evaluation on an
// Initiative. Passed is true iff every deterministic item is met (judgment /
// pending items never block). It is stored on the Initiative so a gate decision
// survives across sessions — distinct from the session-ephemeral code-edit
// evidence tracker, which it deliberately mirrors in shape (record → check →
// block).
type GateState struct {
	Transition string          `yaml:"transition" json:"transition"` // e.g. "Idea->MVP"
	Passed     bool            `yaml:"passed" json:"passed"`
	Evaluated  time.Time       `yaml:"evaluated" json:"evaluated"`
	Items      []GateItemState `yaml:"items,omitempty" json:"items,omitempty"`

	// Overridden records that a human advanced past a BLOCKED gate with an
	// explicit, logged Reason (issue #90 / decision D5). Passed stays false in
	// that case — Overridden captures that the advance happened anyway. Both
	// omitempty so a clean-pass gate marshals unchanged.
	Overridden bool   `yaml:"overridden,omitempty" json:"overridden,omitempty"`
	Reason     string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

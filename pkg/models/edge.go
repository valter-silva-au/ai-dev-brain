package models

// EdgeType is the vocabulary of typed relationships an entity may declare
// toward another entity (decision D6: one generic typed node+edge graph). The
// vocabulary is a CLOSED, canonical set: IsCanonical is the validation primitive
// a write surface that accepts a user-chosen edge type should call to reject an
// unknown one. Today edges are authored in entity frontmatter or minted by
// migration (which only writes canonical types), so there is no such surface
// yet — IsCanonical is ready for the first one that lands. READ is deliberately
// tolerant: an unknown type loaded from frontmatter is preserved verbatim and
// still traversable, so a hand-edited or forward-versioned backlog never panics
// or silently drops an edge. Extend the vocabulary by declaring a new const
// below and adding it to CanonicalEdgeTypes (the documented extension path).
type EdgeType string

const (
	// EdgeRelatesTo is a generic, undirected-in-spirit association: "these two
	// entities are related" with no stronger semantics.
	EdgeRelatesTo EdgeType = "relates_to"
	// EdgePartOf expresses composition/containment: the source is a part of the
	// target (e.g. a ticket is part of an initiative).
	EdgePartOf EdgeType = "part_of"
	// EdgeBlocks means the source blocks the target: the target cannot proceed
	// until the source is resolved.
	EdgeBlocks EdgeType = "blocks"
	// EdgeDependsOn is the inverse of EdgeBlocks: the source depends on the
	// target and is blocked until the target is resolved. `Task.BlockedBy`
	// migrates onto this type (issue #110).
	EdgeDependsOn EdgeType = "depends_on"
	// EdgeDuplicates marks the source as a duplicate of the target.
	EdgeDuplicates EdgeType = "duplicates"
)

// CanonicalEdgeTypes is the ordered, closed vocabulary of edge types adb
// recognises. Order is display / validation-hint order. To add a type, declare
// its const above and append it here — that is the whole documented extension
// path. Callers that need the "must be one of …" hint render this set.
var CanonicalEdgeTypes = []EdgeType{
	EdgeRelatesTo, EdgePartOf, EdgeBlocks, EdgeDependsOn, EdgeDuplicates,
}

// IsCanonical reports whether t is one of the canonical CanonicalEdgeTypes. An
// unknown type (a typo, a forward-versioned type, the empty string) returns
// false; callers use this to validate the WRITE path while still tolerating
// unknown types on READ (the graph never rejects an edge it merely doesn't
// recognise).
func (t EdgeType) IsCanonical() bool {
	for _, v := range CanonicalEdgeTypes {
		if t == v {
			return true
		}
	}
	return false
}

// Link is a single typed edge declared in an entity's persisted frontmatter —
// the SOURCE OF TRUTH for the graph. Target is an entity ref (a task ID like
// TASK-00001, an initiative id, an org id). The derived graph index is a
// rebuildable cache computed from these declared links; the links themselves
// are authoritative. Both fields are required; a Link with an empty Type or
// Target is dropped when the graph index is built.
type Link struct {
	Type   EdgeType `yaml:"type" json:"type"`
	Target string   `yaml:"target" json:"target"`
}

// GraphEdge is one resolved, directed edge in the DERIVED graph index: the
// source entity From declares an edge of Type toward To. It is the
// materialised form of a Link (which carries only Type + Target — From is the
// entity the Link was declared on).
type GraphEdge struct {
	From string   `yaml:"from" json:"from"`
	Type EdgeType `yaml:"type" json:"type"`
	To   string   `yaml:"to" json:"to"`
}

// GraphIndex is the DERIVED, rebuildable cache of every graph edge, computed
// from the authoritative per-entity Links and persisted at graph/index.yaml.
// It is never a source of truth: deleting it and rebuilding from the entities'
// frontmatter reconstructs an identical index (decision D6).
type GraphIndex struct {
	Edges []GraphEdge `yaml:"edges" json:"edges"`
}

package models

import (
	"fmt"
	"strings"
	"time"
)

// This file models the staged ingestion pipeline (decision D8):
//
//	connector → immutable raw/ landing (provenance + hash/cursor dedup)
//	          → extraction skill proposes typed nodes/edges
//	          → confidence-gated landing (auto for certain, review queue for fuzzy)
//
// Provenance travels the whole way: a RawArtifact records where content came
// from, and every EntityProposal / IngestedNode traces back to a raw artifact.

// RawArtifact is one immutable landed source artifact. Connectors land raw
// content under raw/ with provenance — the Source it came from, a content Hash,
// and an optional per-source Cursor (a message id, timestamp, or offset) — so a
// re-pull of the same source is deduplicated rather than re-landed.
type RawArtifact struct {
	ID          string    `yaml:"id" json:"id"`
	Source      string    `yaml:"source" json:"source"`
	Cursor      string    `yaml:"cursor,omitempty" json:"cursor,omitempty"`
	Hash        string    `yaml:"hash" json:"hash"`
	ContentPath string    `yaml:"content_path" json:"content_path"`
	Landed      time.Time `yaml:"landed" json:"landed"`
}

// RawManifest is the raw/ landing ledger (raw/manifest.yaml): every landed
// artifact, used for hash/cursor dedup.
type RawManifest struct {
	Artifacts []RawArtifact `yaml:"artifacts" json:"artifacts"`
}

// Contains reports whether an artifact with this content hash, or this
// source+cursor position, has already been landed — the two dedup keys. An
// empty cursor only matches on hash (a cursorless source dedups on content).
func (m RawManifest) Contains(hash, source, cursor string) bool {
	for _, a := range m.Artifacts {
		if hash != "" && a.Hash == hash {
			return true
		}
		if cursor != "" && a.Source == source && a.Cursor == cursor {
			return true
		}
	}
	return false
}

// ProposalKind distinguishes a proposed edge from a proposed node.
type ProposalKind string

const (
	// ProposalEdge proposes a typed graph edge between two entity refs.
	ProposalEdge ProposalKind = "edge"
	// ProposalNode proposes a new typed graph node (stakeholder, system, …).
	ProposalNode ProposalKind = "node"
)

// ProposalStatus is a proposal's review state.
type ProposalStatus string

const (
	// ProposalPending is awaiting an accept/reject decision in the review queue.
	ProposalPending ProposalStatus = "pending"
	// ProposalAccepted has been applied to the graph.
	ProposalAccepted ProposalStatus = "accepted"
	// ProposalRejected was declined and never applied.
	ProposalRejected ProposalStatus = "rejected"
)

// EntityProposal is a proposed graph mutation (an edge or a node) that an
// extraction skill derives from a raw artifact. RawID is the provenance link
// back to the source artifact; Confidence drives the landing gate (high →
// auto-land, low → review queue).
type EntityProposal struct {
	ID         string         `yaml:"id" json:"id"`
	RawID      string         `yaml:"raw_id" json:"raw_id"`
	Kind       ProposalKind   `yaml:"kind" json:"kind"`
	Confidence float64        `yaml:"confidence" json:"confidence"`
	Status     ProposalStatus `yaml:"status,omitempty" json:"status,omitempty"`
	Created    time.Time      `yaml:"created,omitempty" json:"created,omitempty"`
	Decided    time.Time      `yaml:"decided,omitempty" json:"decided,omitempty"`
	Reason     string         `yaml:"reason,omitempty" json:"reason,omitempty"`

	// Edge fields (Kind == edge): declare Edge as a link on entity From.
	From string `yaml:"from,omitempty" json:"from,omitempty"`
	Edge *Link  `yaml:"edge,omitempty" json:"edge,omitempty"`

	// Node fields (Kind == node): the typed node to land.
	Node *IngestedNode `yaml:"node,omitempty" json:"node,omitempty"`
}

// IngestedNode is a typed graph node landed from ingestion (e.g. a Stakeholder,
// System, or Dataset). It carries provenance (Source → a raw artifact id) and
// may declare its own typed links[], so it participates in the graph exactly
// like a task or initiative once the graph source includes the node registry.
type IngestedNode struct {
	ID      string    `yaml:"id" json:"id"`
	Type    string    `yaml:"type" json:"type"`
	Title   string    `yaml:"title,omitempty" json:"title,omitempty"`
	Source  string    `yaml:"source,omitempty" json:"source,omitempty"`
	Links   []Link    `yaml:"links,omitempty" json:"links,omitempty"`
	Created time.Time `yaml:"created,omitempty" json:"created,omitempty"`
}

// NodeIndex is the ingested-node registry (ingested/nodes.yaml).
type NodeIndex struct {
	Nodes []IngestedNode `yaml:"nodes" json:"nodes"`
}

// ProposalQueue is the review queue (ingested/queue.yaml) of fuzzy proposals
// awaiting a human decision.
type ProposalQueue struct {
	Proposals []EntityProposal `yaml:"proposals" json:"proposals"`
}

// Validate checks a proposal is structurally well-formed for its kind, with a
// confidence in [0,1] and a provenance link to a raw artifact.
func (p EntityProposal) Validate() error {
	if strings.TrimSpace(p.RawID) == "" {
		return fmt.Errorf("proposal %q: raw_id (provenance) is required", p.ID)
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return fmt.Errorf("proposal %q: confidence %.3f out of range [0,1]", p.ID, p.Confidence)
	}
	switch p.Kind {
	case ProposalEdge:
		if strings.TrimSpace(p.From) == "" {
			return fmt.Errorf("proposal %q: edge needs a from entity", p.ID)
		}
		if p.Edge == nil || strings.TrimSpace(string(p.Edge.Type)) == "" || strings.TrimSpace(p.Edge.Target) == "" {
			return fmt.Errorf("proposal %q: edge needs both a type and a target", p.ID)
		}
	case ProposalNode:
		if p.Node == nil || strings.TrimSpace(p.Node.ID) == "" || strings.TrimSpace(p.Node.Type) == "" {
			return fmt.Errorf("proposal %q: node needs both an id and a type", p.ID)
		}
	default:
		return fmt.Errorf("proposal %q: kind must be %q or %q", p.ID, ProposalEdge, ProposalNode)
	}
	return nil
}

package models

import "testing"

func TestRawManifest_Contains(t *testing.T) {
	m := RawManifest{Artifacts: []RawArtifact{
		{ID: "a", Source: "slack:C1", Cursor: "100", Hash: "h1"},
	}}
	cases := []struct {
		name                 string
		hash, source, cursor string
		want                 bool
	}{
		{"same hash", "h1", "other", "999", true},
		{"same source+cursor", "different", "slack:C1", "100", true},
		{"new content new cursor", "h2", "slack:C1", "101", false},
		{"cursorless different hash", "h2", "slack:C1", "", false},
		{"empty hash empty cursor", "", "slack:C1", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.Contains(tc.hash, tc.source, tc.cursor); got != tc.want {
				t.Fatalf("Contains(%q,%q,%q) = %v, want %v", tc.hash, tc.source, tc.cursor, got, tc.want)
			}
		})
	}
}

func TestEntityProposal_Validate(t *testing.T) {
	validEdge := EntityProposal{ID: "p1", RawID: "raw-1", Kind: ProposalEdge, Confidence: 0.9, From: "TASK-1", Edge: &Link{Type: EdgeRelatesTo, Target: "TASK-2"}}
	if err := validEdge.Validate(); err != nil {
		t.Fatalf("valid edge proposal rejected: %v", err)
	}
	validNode := EntityProposal{ID: "p2", RawID: "raw-1", Kind: ProposalNode, Confidence: 0.5, Node: &IngestedNode{ID: "STK-1", Type: "stakeholder"}}
	if err := validNode.Validate(); err != nil {
		t.Fatalf("valid node proposal rejected: %v", err)
	}

	bad := []struct {
		name string
		p    EntityProposal
	}{
		{"no provenance", EntityProposal{ID: "x", Kind: ProposalEdge, Confidence: 0.5, From: "T", Edge: &Link{Type: EdgeRelatesTo, Target: "U"}}},
		{"confidence too high", EntityProposal{ID: "x", RawID: "r", Kind: ProposalEdge, Confidence: 1.5, From: "T", Edge: &Link{Type: EdgeRelatesTo, Target: "U"}}},
		{"confidence negative", EntityProposal{ID: "x", RawID: "r", Kind: ProposalEdge, Confidence: -0.1, From: "T", Edge: &Link{Type: EdgeRelatesTo, Target: "U"}}},
		{"edge missing target", EntityProposal{ID: "x", RawID: "r", Kind: ProposalEdge, Confidence: 0.5, From: "T", Edge: &Link{Type: EdgeRelatesTo}}},
		{"edge missing from", EntityProposal{ID: "x", RawID: "r", Kind: ProposalEdge, Confidence: 0.5, Edge: &Link{Type: EdgeRelatesTo, Target: "U"}}},
		{"node missing type", EntityProposal{ID: "x", RawID: "r", Kind: ProposalNode, Confidence: 0.5, Node: &IngestedNode{ID: "N"}}},
		{"unknown kind", EntityProposal{ID: "x", RawID: "r", Kind: "bogus", Confidence: 0.5}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.p.Validate(); err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.name)
			}
		})
	}
}

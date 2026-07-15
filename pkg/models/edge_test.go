package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEdgeType_IsCanonical pins the closed edge-type vocabulary (decision D6):
// the five canonical types report true, and anything else — a typo, an
// unknown-but-tolerated type read off disk, the empty string — reports false.
func TestEdgeType_IsCanonical(t *testing.T) {
	canonical := []EdgeType{
		EdgeRelatesTo, EdgePartOf, EdgeBlocks, EdgeDependsOn, EdgeDuplicates,
	}
	for _, et := range canonical {
		if !et.IsCanonical() {
			t.Errorf("EdgeType %q should be canonical", et)
		}
	}
	for _, et := range []EdgeType{"relatesto", "child_of", "", "RELATES_TO"} {
		if et.IsCanonical() {
			t.Errorf("EdgeType %q should NOT be canonical", et)
		}
	}
}

// TestCanonicalEdgeTypes_Membership guards the exported vocabulary set so a
// future edit that drops or reorders a type is caught, and every listed type
// round-trips through IsCanonical.
func TestCanonicalEdgeTypes_Membership(t *testing.T) {
	want := []EdgeType{
		EdgeRelatesTo, EdgePartOf, EdgeBlocks, EdgeDependsOn, EdgeDuplicates,
	}
	if len(CanonicalEdgeTypes) != len(want) {
		t.Fatalf("CanonicalEdgeTypes has %d entries, want %d", len(CanonicalEdgeTypes), len(want))
	}
	for i, et := range want {
		if CanonicalEdgeTypes[i] != et {
			t.Errorf("CanonicalEdgeTypes[%d]=%q want %q", i, CanonicalEdgeTypes[i], et)
		}
	}
}

// TestLink_YAMLRoundTrip proves a typed Link survives a backlog.yaml round-trip
// through the yaml.v3 codec that FileBacklogManager uses.
func TestLink_YAMLRoundTrip(t *testing.T) {
	in := Task{
		ID:       "TASK-00099",
		Title:    "linked",
		Type:     TaskTypeFeat,
		Status:   TaskStatusBacklog,
		Priority: PriorityP2,
		Links: []Link{
			{Type: EdgeDependsOn, Target: "TASK-00042"},
			{Type: EdgeRelatesTo, Target: "some-initiative"},
			// an unknown-but-tolerated edge type must survive a round-trip.
			{Type: EdgeType("mentions"), Target: "TASK-00007"},
		},
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Task
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Links) != 3 {
		t.Fatalf("Links len=%d want 3\n%s", len(out.Links), data)
	}
	if out.Links[0].Type != EdgeDependsOn || out.Links[0].Target != "TASK-00042" {
		t.Errorf("Links[0]=%+v want {depends_on TASK-00042}", out.Links[0])
	}
	if out.Links[2].Type != EdgeType("mentions") {
		t.Errorf("unknown edge type not preserved: %+v", out.Links[2])
	}
}

// TestLink_OmitemptyOnZero locks in that a task with no links does NOT emit the
// `links` key — pre-graph backlog entries marshal byte-identically (issue #109
// acceptance: omitempty round-trip leaves pre-graph entities byte-identical).
func TestLink_OmitemptyOnZero(t *testing.T) {
	data, err := yaml.Marshal(Task{
		ID:       "TASK-1",
		Type:     TaskTypeFeat,
		Status:   TaskStatusBacklog,
		Priority: PriorityP2,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "links") {
		t.Errorf("zero-value task should omit `links`, got:\n%s", data)
	}
}

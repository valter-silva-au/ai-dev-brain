package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDebtItem_GraphID pins the node id a tech-debt item carries in the typed
// graph. It is built from the named prefix rather than a literal so the one
// spelling the graph, the catalog, and `adb graph neighbors` all use cannot
// drift between them.
func TestDebtItem_GraphID(t *testing.T) {
	if got := (DebtItem{ID: "DEBT-0001"}).GraphID(); got != "debt:DEBT-0001" {
		t.Fatalf("GraphID() = %q, want debt:DEBT-0001", got)
	}
	if !strings.HasPrefix((DebtItem{ID: "DEBT-0042"}).GraphID(), DebtGraphPrefix) {
		t.Fatalf("GraphID() should be built from DebtGraphPrefix (%q)", DebtGraphPrefix)
	}
	// An id-less item yields the bare prefix rather than panicking; buildGraph
	// drops such a node's links because a node needs both ends of an edge.
	if got := (DebtItem{}).GraphID(); got != DebtGraphPrefix {
		t.Fatalf("empty-id GraphID() = %q, want %q", got, DebtGraphPrefix)
	}
}

// TestDebtItem_LinksAreOmittedWhenEmpty guards the registry's on-disk shape: the
// links field is additive, so a debt/index.yaml written before debt joined the
// graph round-trips byte-identically.
func TestDebtItem_LinksAreOmittedWhenEmpty(t *testing.T) {
	data, err := yaml.Marshal(DebtItem{ID: "DEBT-0001", Title: "t", Priority: PriorityP2, Status: DebtOpen})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "links") {
		t.Errorf("empty Links should be omitted from YAML, got:\n%s", data)
	}

	// A declared link survives the round trip with its typed vocabulary intact.
	var back DebtItem
	in := DebtItem{ID: "DEBT-0002", Links: []Link{{Type: EdgeRelatesTo, Target: "TASK-1"}}}
	data, err = yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Links) != 1 || back.Links[0].Type != EdgeRelatesTo || back.Links[0].Target != "TASK-1" {
		t.Errorf("links round trip = %+v", back.Links)
	}
}

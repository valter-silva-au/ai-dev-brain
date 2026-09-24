package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCommunication_EmptyDirectionOmitted verifies the omitempty byte-identity
// claim: a Communication with no Direction marshals without a `direction:` key,
// so pre-direction records are unchanged.
func TestCommunication_EmptyDirectionOmitted(t *testing.T) {
	data, err := yaml.Marshal(Communication{ID: "c1", TaskID: "TASK-1", Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "direction") {
		t.Fatalf("empty direction should be omitted, got:\n%s", data)
	}
	// A set direction round-trips.
	set, _ := yaml.Marshal(Communication{ID: "c2", Direction: DirectionInbound})
	if !strings.Contains(string(set), "direction: inbound") {
		t.Fatalf("set direction should marshal, got:\n%s", set)
	}
}

func TestCommunicationDirection_IsValid(t *testing.T) {
	cases := []struct {
		d    CommunicationDirection
		want bool
	}{
		{DirectionInbound, true},
		{DirectionOutbound, true},
		{"", false},
		{"sideways", false},
	}
	for _, tc := range cases {
		if got := tc.d.IsValid(); got != tc.want {
			t.Fatalf("(%q).IsValid() = %v, want %v", tc.d, got, tc.want)
		}
	}
}

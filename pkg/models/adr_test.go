package models

import (
	"strings"
	"testing"
)

// TestADR_GraphID pins the node id an ADR carries in the typed graph. The shape
// is load-bearing, not cosmetic: `adr:NNNN` is what the catalog and the derived
// graph index already store, so changing the padding would orphan every existing
// ADR node. The prefix is asserted against the ADRGraphPrefix const so the
// formatter and `parseADRNumber` (the CLI parser that consumes what this
// produces) cannot drift onto two spellings of one prefix.
func TestADR_GraphID(t *testing.T) {
	if ADRGraphPrefix != "adr:" {
		t.Fatalf("ADRGraphPrefix = %q, want \"adr:\"", ADRGraphPrefix)
	}
	cases := []struct {
		number int
		want   string
	}{
		{1, "adr:0001"},
		{2, "adr:0002"},
		{42, "adr:0042"},
		{9999, "adr:9999"},
		{12345, "adr:12345"}, // padding is a MINIMUM: a 5-digit number is not truncated
	}
	for _, c := range cases {
		if got := (ADR{Number: c.number}).GraphID(); got != c.want {
			t.Errorf("ADR{Number: %d}.GraphID() = %q, want %q", c.number, got, c.want)
		}
		if !strings.HasPrefix(c.want, ADRGraphPrefix) {
			t.Errorf("%q does not carry ADRGraphPrefix %q", c.want, ADRGraphPrefix)
		}
	}
}

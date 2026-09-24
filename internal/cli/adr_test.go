package cli

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestParseADRNumber_RoundTripsGraphID is the guard that keeps the two halves of
// the `adr:` prefix honest: models.ADR.GraphID FORMATS it and parseADRNumber
// PARSES it, in different packages. Feeding the formatter's own output back
// through the parser is what makes a divergence a test failure rather than a
// silently unresolvable id.
func TestParseADRNumber_RoundTripsGraphID(t *testing.T) {
	for _, n := range []int{1, 2, 42, 9999, 12345} {
		id := models.ADR{Number: n}.GraphID()
		got, err := parseADRNumber(id)
		if err != nil {
			t.Fatalf("parseADRNumber(%q): %v", id, err)
		}
		if got != n {
			t.Errorf("parseADRNumber(%q) = %d, want %d", id, got, n)
		}
	}
}

// TestParseADRNumber_AcceptedForms pins every spelling a human may reasonably
// type, including the mixed-case ones the lowercasing exists for.
func TestParseADRNumber_AcceptedForms(t *testing.T) {
	for _, in := range []string{"12", "0012", "adr:0012", "ADR:0012", "adr-12", "ADR-12", "  12  "} {
		got, err := parseADRNumber(in)
		if err != nil {
			t.Fatalf("parseADRNumber(%q): %v", in, err)
		}
		if got != 12 {
			t.Errorf("parseADRNumber(%q) = %d, want 12", in, got)
		}
	}
}

// TestParseADRNumber_Rejects keeps the failure cases explicit — an unparseable
// or non-positive number is an error, never a silent 0.
func TestParseADRNumber_Rejects(t *testing.T) {
	for _, in := range []string{"", "abc", "adr:", "0", "-3", "adr:zero"} {
		if _, err := parseADRNumber(in); err == nil {
			t.Errorf("parseADRNumber(%q) = nil error, want a rejection", in)
		}
	}
}

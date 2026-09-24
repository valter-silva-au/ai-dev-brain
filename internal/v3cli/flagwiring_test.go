package v3cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRequireFlag_ReportsAnUnwiredFlag pins that a wiring typo is reported
// rather than discarded. A v3 mutation addresses one entity by a selector that
// is unique only within an organization, so a `--org`/`--workspace`/`--path`
// that silently stopped being required is a correctness problem, not cosmetics.
func TestRequireFlag_ReportsAnUnwiredFlag(t *testing.T) {
	command := &cobra.Command{Use: "demo"}
	var errOut bytes.Buffer
	command.SetErr(&errOut)

	requireFlag(command, "orgg") // deliberate typo: no such flag

	if got := errOut.String(); !strings.Contains(got, "orgg") {
		t.Fatalf("requireFlag must name the unwired flag on stderr, got %q", got)
	}
}

// TestRequireFlag_MarksAndStaysSilent is the other half: a correctly wired flag
// really is marked required, and prints nothing.
func TestRequireFlag_MarksAndStaysSilent(t *testing.T) {
	command := &cobra.Command{Use: "demo"}
	var organization string
	command.Flags().StringVar(&organization, "org", "", "Organization selector")
	var errOut bytes.Buffer
	command.SetErr(&errOut)

	requireFlag(command, "org")

	if errOut.Len() != 0 {
		t.Errorf("requireFlag must be silent for a registered flag, got %q", errOut.String())
	}
	annotations := command.Flags().Lookup("org").Annotations[cobra.BashCompOneRequiredFlag]
	if len(annotations) == 0 || annotations[0] != "true" {
		t.Errorf("requireFlag must actually mark the flag required, annotations = %v", annotations)
	}
}

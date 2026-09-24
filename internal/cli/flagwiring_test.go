package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRequireFlag_ReportsAnUnwiredFlag pins that a wiring typo is REPORTED
// rather than discarded. cobra's MarkFlagRequired fails only when the named
// flag was never registered — a typo in the constructor it sits in, whose
// consequence is a flag that silently stopped being required. Nothing at
// runtime can recover from that, so the least it can do is say so.
func TestRequireFlag_ReportsAnUnwiredFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	requireFlag(cmd, "objectve") // deliberate typo: no such flag

	got := errOut.String()
	if !strings.Contains(got, "objectve") {
		t.Fatalf("requireFlag must name the unwired flag on stderr, got %q", got)
	}
}

// TestRequireFlag_MarksAndStaysSilent is the other half: a correctly wired flag
// really is marked required, and prints nothing. Without this the helper could
// satisfy the test above by only ever printing.
func TestRequireFlag_MarksAndStaysSilent(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	var objective float64
	cmd.Flags().Float64Var(&objective, "objective", 0, "target percentage")
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	requireFlag(cmd, "objective")

	if errOut.Len() != 0 {
		t.Errorf("requireFlag must be silent for a registered flag, got %q", errOut.String())
	}
	annotations := cmd.Flags().Lookup("objective").Annotations[cobra.BashCompOneRequiredFlag]
	if len(annotations) == 0 || annotations[0] != "true" {
		t.Errorf("requireFlag must actually mark the flag required, annotations = %v", annotations)
	}
}

// TestHideFlag_ReportsAnUnwiredFlag is the same contract for MarkHidden, which
// `adb task worktree prune --dry-run` (the retired-alias no-op) depends on.
func TestHideFlag_ReportsAnUnwiredFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	hideFlag(cmd, "dry-runn") // deliberate typo: no such flag

	if got := errOut.String(); !strings.Contains(got, "dry-runn") {
		t.Fatalf("hideFlag must name the unwired flag on stderr, got %q", got)
	}
}

// TestHideFlag_HidesAndStaysSilent — a real flag is hidden, silently.
func TestHideFlag_HidesAndStaysSilent(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	var dryRun bool
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Deprecated no-op")
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	hideFlag(cmd, "dry-run")

	if errOut.Len() != 0 {
		t.Errorf("hideFlag must be silent for a registered flag, got %q", errOut.String())
	}
	if !cmd.Flags().Lookup("dry-run").Hidden {
		t.Error("hideFlag must actually hide the flag")
	}
}

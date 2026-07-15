package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewConformanceCmd creates the `adb conformance` command group — the
// conformance-drift check (#128). It flags entities drifting from their template
// or catalog expectations and is the deterministic logic a scheduled D7 rule
// drives (`adb schedule add --every 24h --run-exec "adb conformance check"`).
func NewConformanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conformance",
		Short: "Check the workspace for template/catalog conformance drift",
		Long: `Flag entities that have drifted from their expectations:

  stale-template      the workspace scaffolded from an older template version
  missing-file        a scaffolded file recorded in the manifest is now missing
  dangling-org        an initiative references an org not in the registry
  dangling-initiative a ticket or metric references an unknown initiative

Drive it on a schedule with the rule engine (#119), e.g.:
  adb schedule add --name conformance-drift --every 24h \
      --run-exec "adb conformance check"`,
	}
	cmd.AddCommand(newConformanceCheckCmd())
	return cmd
}

func newConformanceCheckCmd() *cobra.Command {
	var (
		jsonOutput bool
		exitCode   bool
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report conformance-drift findings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if App.DriftChecker == nil {
				return fmt.Errorf("drift checker not initialized")
			}
			report, err := App.DriftChecker.Check()
			if err != nil {
				return fmt.Errorf("conformance check failed: %w", err)
			}

			if jsonOutput {
				if err := printJSON(report); err != nil {
					return err
				}
			} else if !report.HasDrift() {
				fmt.Println("✓ No conformance drift.")
			} else {
				fmt.Printf("Conformance drift (%d finding(s)):\n", len(report.Findings))
				for _, f := range report.Findings {
					fmt.Printf("  [%s] %s — %s\n", f.Kind, f.Entity, f.Detail)
				}
			}

			// --exit-code makes drift a non-zero exit so the check is usable as a
			// CI/gate step; without it the command is a pure report (exit 0).
			if exitCode && report.HasDrift() {
				return fmt.Errorf("%d conformance drift finding(s)", len(report.Findings))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output findings as JSON")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit non-zero when drift is found (for CI/gates)")
	return cmd
}

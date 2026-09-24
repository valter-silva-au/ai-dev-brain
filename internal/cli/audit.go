package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// NewAuditCmd creates the `adb audit` command group — security & compliance
// posture audits (#131 step 17).
func NewAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Security & compliance posture audits",
	}
	cmd.AddCommand(newAuditSecurityCmd())
	return cmd
}

func newAuditSecurityCmd() *cobra.Command {
	var (
		jsonOutput bool
		framework  string
		exitCode   bool
	)
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Audit the workspace's security/compliance posture",
		Long: `Evaluate the workspace against a catalog of security/compliance controls.
Deterministic controls verify real facts (secret-scanning config, .env hygiene,
pre-commit, SLOs); framework controls that need human attestation report
"manual" and point at the scaffolded compliance doc (adb compliance scaffold).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if App.SecurityAuditor == nil {
				return fmt.Errorf("security auditor not initialized")
			}
			// Reject an unknown framework instead of running the deterministic
			// controls and reporting as though the framework had been covered.
			// Silently accepting `--framework pci` produces a clean-looking audit
			// that attests to nothing — the worst possible failure for this command.
			if framework != "" && !core.IsValidAuditFramework(framework) {
				return fmt.Errorf(
					"invalid --framework %q (want one of: %s)",
					framework, strings.Join(core.ValidAuditFrameworks(), ", "),
				)
			}
			report, err := App.SecurityAuditor.Audit(framework)
			if err != nil {
				return fmt.Errorf("security audit failed: %w", err)
			}

			if jsonOutput {
				if err := printJSON(report); err != nil {
					return err
				}
			} else {
				for _, f := range report.Findings {
					fmt.Printf("  [%-6s] %-20s %s — %s\n", f.Status, f.Control, f.Title, f.Detail)
				}
				s := report.Summary
				fmt.Printf("\n%d pass, %d fail, %d warn, %d manual\n", s.Pass, s.Fail, s.Warn, s.Manual)
			}

			// --exit-code makes a control failure a non-zero exit (for CI gates).
			if exitCode && report.HasFailures() {
				return fmt.Errorf("%d control(s) failed", report.Summary.Fail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output findings as JSON")
	cmd.Flags().StringVar(&framework, "framework", "", "limit to a framework's controls: soc2|gdpr|hipaa (default: all)")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit non-zero when a control fails (for CI/gates)")
	return cmd
}

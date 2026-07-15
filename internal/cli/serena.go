package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewSerenaCmd creates the `adb serena` command group: effectiveness telemetry
// (record + report) for the Serena code-nav integration (#203).
func NewSerenaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serena",
		Short: "Serena code-nav integration: effectiveness telemetry",
		Long:  `Record and report how effective Serena's code-nav is, so the operator can judge whether it's contributing.`,
	}
	cmd.AddCommand(newSerenaRecordCmd(), newSerenaReportCmd())
	return cmd
}

// newSerenaRecordCmd is the non-interactive self-report an agent runs at the end
// of a session; it emits exactly one serena.effectiveness_recorded event (#203).
func newSerenaRecordCmd() *cobra.Command {
	var rec models.SerenaRecord
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a Serena effectiveness scorecard (non-interactive)",
		Long: `Self-report how effective Serena was this session. Non-interactive: the
scorecard is carried entirely via flags, so an agent can run it unattended.
--verdict unused is a first-class, honest "not needed" outcome.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if !models.IsValidSerenaVerdict(rec.Verdict) {
				return fmt.Errorf("invalid --verdict %q (want one of: %s)", rec.Verdict, strings.Join(models.ValidSerenaVerdicts, ", "))
			}
			if rec.Score < 0 || rec.Score > 5 {
				return fmt.Errorf("invalid --score %d (want 1..5, or 0 to omit)", rec.Score)
			}
			if err := App.SerenaTelemetry.Record(rec); err != nil {
				return fmt.Errorf("failed to record: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ recorded serena effectiveness: %s (score %d)\n", rec.Verdict, rec.Score)
			return nil
		},
	}
	cmd.Flags().StringVar(&rec.Verdict, "verdict", "", "helped | neutral | hindered | unused (required)")
	cmd.Flags().IntVar(&rec.Score, "score", 0, "1..5 self-rating (0 = omit)")
	cmd.Flags().StringVar(&rec.UsedFor, "used-for", "", "What Serena was used for")
	cmd.Flags().StringVar(&rec.Beat, "beat", "", "What Serena beat / replaced")
	cmd.Flags().StringVar(&rec.Friction, "friction", "", "Any friction encountered")
	cmd.Flags().StringVar(&rec.TaskID, "task", "", "Optional task id to attribute the report to")
	_ = cmd.MarkFlagRequired("verdict")
	return cmd
}

// newSerenaReportCmd rolls the recorded history up for the operator (#203).
func newSerenaReportCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Roll up recorded Serena effectiveness (counts, average, recent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			rollup, err := App.SerenaTelemetry.Report()
			if err != nil {
				return fmt.Errorf("failed to build report: %w", err)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rollup)
			}
			printSerenaReport(cmd, rollup)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the rollup as JSON")
	return cmd
}

func printSerenaReport(cmd *cobra.Command, r models.SerenaRollup) {
	w := cmd.OutOrStdout()
	if r.Total == 0 {
		fmt.Fprintln(w, "No Serena effectiveness recorded yet. Run `adb serena record` to add one.")
		return
	}
	fmt.Fprintf(w, "Serena effectiveness: %d report(s), average score %.2f\n\n", r.Total, r.AverageScore)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "VERDICT\tCOUNT")
	verdicts := make([]string, 0, len(r.ByVerdict))
	for v := range r.ByVerdict {
		verdicts = append(verdicts, v)
	}
	sort.Strings(verdicts)
	for _, v := range verdicts {
		fmt.Fprintf(tw, "%s\t%d\n", v, r.ByVerdict[v])
	}
	_ = tw.Flush()

	if len(r.Recent) > 0 {
		fmt.Fprintln(w, "\nRecent:")
		for _, rec := range r.Recent {
			line := fmt.Sprintf("  %s (score %d)", rec.Verdict, rec.Score)
			if rec.UsedFor != "" {
				line += " — " + rec.UsedFor
			}
			if rec.TaskID != "" {
				line += " [" + rec.TaskID + "]"
			}
			fmt.Fprintln(w, line)
		}
	}
}

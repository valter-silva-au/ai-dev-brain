package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewPMFCmd creates the `adb pmf` command group for product/PMF metrics
// (decision D11, issue #122). Metrics are provenance-carrying graph nodes,
// manual-entry-first: `adb pmf record` writes a metric node against an
// initiative, and stage gates read those nodes for numeric thresholds (the
// MVP→Launch Sean-Ellis ≥40% + effort bar — closing #103).
func NewPMFCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pmf",
		Short: "Record and list product/PMF metric nodes for initiatives",
		Long: `Product/PMF metrics (decision D11) as provenance-carrying graph nodes.

Manual entry first (the schema leaves room for connector-fed values later).
Stage gates read these nodes for numeric thresholds — e.g. the MVP→Launch gate
requires a Sean-Ellis score ≥40% plus an effort/retention metric ≥40%.

  adb pmf record --initiative onboarding --metric sean-ellis --value 45 --unit %
  adb pmf record --initiative onboarding --metric retention --value 42
  adb pmf list [--initiative onboarding]`,
	}
	cmd.AddCommand(newPMFRecordCmd(), newPMFListCmd())
	return cmd
}

func newPMFRecordCmd() *cobra.Command {
	var (
		initiative string
		metric     string
		value      float64
		unit       string
		source     string
		note       string
	)
	cmd := &cobra.Command{
		Use:   "record --initiative <id> --metric <name> --value <n>",
		Short: "Record (or update) a metric node against an initiative",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.MetricStore == nil {
				return fmt.Errorf("app not initialized")
			}
			if initiative == "" || metric == "" {
				return fmt.Errorf("--initiative and --metric are required")
			}
			// Require --value explicitly (Changed) so a legitimate 0 metric records.
			if !cmd.Flags().Changed("value") {
				return fmt.Errorf("--value is required")
			}
			m, err := App.MetricStore.Record(models.Metric{
				Initiative: initiative,
				Name:       metric,
				Value:      value,
				Unit:       unit,
				Source:     source,
				Note:       note,
			})
			if err != nil {
				return fmt.Errorf("record metric: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Recorded %s = %g%s for %s (source: %s).\n",
				m.Name, m.Value, unitSuffix(m.Unit), m.Initiative, m.Source)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&initiative, "initiative", "", "initiative id the metric belongs to (required)")
	f.StringVar(&metric, "metric", "", "metric name, e.g. sean-ellis, retention (required)")
	f.Float64Var(&value, "value", 0, "the metric value (required)")
	f.StringVar(&unit, "unit", "", "unit, e.g. % or ratio")
	f.StringVar(&source, "source", "manual", "provenance: manual (default) or a connector id")
	f.StringVar(&note, "note", "", "optional note")
	return cmd
}

func newPMFListCmd() *cobra.Command {
	var (
		initiative string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recorded metrics (optionally for one initiative)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.MetricStore == nil {
				return fmt.Errorf("app not initialized")
			}
			all, err := App.MetricStore.List()
			if err != nil {
				return err
			}
			if initiative != "" {
				filtered := all[:0:0]
				for _, m := range all {
					if m.Initiative == initiative {
						filtered = append(filtered, m)
					}
				}
				all = filtered
			}
			if jsonOutput {
				return printJSON(all)
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No metrics recorded. Add one with `adb pmf record`.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "INITIATIVE\tMETRIC\tVALUE\tSOURCE\tRECORDED")
			for _, m := range all {
				fmt.Fprintf(w, "%s\t%s\t%g%s\t%s\t%s\n",
					m.Initiative, m.Name, m.Value, unitSuffix(m.Unit), m.Source, m.Recorded.Local().Format("2006-01-02"))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&initiative, "initiative", "", "limit to one initiative")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func unitSuffix(unit string) string {
	if unit == "" {
		return ""
	}
	if unit == "%" {
		return "%"
	}
	return " " + unit
}

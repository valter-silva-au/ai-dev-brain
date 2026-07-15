package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewGovernanceCmd creates the `adb governance` command group — a reader over the
// governance event stream (.governance.jsonl), which is DISTINCT from the
// high-volume dev-telemetry event log (decision D19, #137 step 19).
func NewGovernanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "governance",
		Short: "Inspect the governance event stream (stage advances/overrides)",
		Long: `Read the governance event stream — stage.advanced / stage.override decisions,
including the human-only Launch→Scale gate — kept in .governance.jsonl separate
from the task/agent dev telemetry so a compliance/audit reader sees only
governance decisions.`,
	}
	cmd.AddCommand(newGovernanceListCmd())
	return cmd
}

func newGovernanceListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List governance events (most recent last)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.GovernanceLog == nil {
				return fmt.Errorf("app not initialized")
			}
			events, err := App.GovernanceLog.ReadAll()
			if err != nil {
				return fmt.Errorf("read governance stream: %w", err)
			}
			if jsonOutput {
				return printJSON(events)
			}
			if len(events) == 0 {
				fmt.Println("No governance events yet. They are recorded on `adb stage advance`.")
				return nil
			}
			for _, e := range events {
				ts := e.Timestamp.Format("2006-01-02 15:04:05Z")
				init, _ := e.Data["initiative_id"].(string)
				from, _ := e.Data["from"].(string)
				to, _ := e.Data["to"].(string)
				line := fmt.Sprintf("%s  %-16s %s %s->%s", ts, e.Type, init, from, to)
				if reason, ok := e.Data["reason"].(string); ok && reason != "" {
					line += fmt.Sprintf("  reason=%q", reason)
				}
				if ov, ok := e.Data["overridden"].(bool); ok && ov {
					line += "  (overridden)"
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

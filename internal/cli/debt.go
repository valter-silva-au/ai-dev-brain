package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewDebtCmd creates the `adb debt` command group — the architecture-audit /
// tech-debt registry (#128 step 16).
func NewDebtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debt",
		Short: "Architecture-audit / tech-debt triage registry",
		Long: `Record and triage architecture-audit / tech-debt items (debt/index.yaml).
Items are lightweight, priority-triageable records — not tickets — so an audit
can enumerate debt without minting worktrees.
Each item is a debt:DEBT-NNNN node in the typed graph and shows up in adb catalog.`,
	}
	cmd.AddCommand(newDebtAddCmd(), newDebtListCmd(), newDebtResolveCmd())
	return cmd
}

func newDebtAddCmd() *cobra.Command {
	var (
		priority   string
		area       string
		note       string
		relatesTo  string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Record a tech-debt item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			// Same shape as `adb adr new --relates-to`: the flag takes a target,
			// never an edge type, so the closed vocabulary cannot be widened here.
			var links []models.Link
			if strings.TrimSpace(relatesTo) != "" {
				links = append(links, models.Link{Type: models.EdgeRelatesTo, Target: strings.TrimSpace(relatesTo)})
			}
			item, err := App.DebtManager.Add(args[0], area, note, models.Priority(priority), links)
			if err != nil {
				return fmt.Errorf("failed to add debt item: %w", err)
			}
			if jsonOutput {
				return printJSON(item)
			}
			fmt.Printf("Recorded %s [%s]: %s\n", item.ID, item.Priority, item.Title)
			fmt.Printf("  graph node %s\n", item.GraphID())
			return nil
		},
	}
	cmd.Flags().StringVar(&priority, "priority", "P2", "priority P0|P1|P2|P3")
	cmd.Flags().StringVar(&area, "area", "", "subsystem/package the debt lives in")
	cmd.Flags().StringVar(&note, "note", "", "optional detail")
	cmd.Flags().StringVar(&relatesTo, "relates-to", "", "entity id this debt relates to (a ticket/initiative), added as a relates_to edge")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newDebtListCmd() *cobra.Command {
	var (
		jsonOutput bool
		openOnly   bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tech-debt items (triage order: open first, then by priority)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			items, err := App.DebtManager.List()
			if err != nil {
				return err
			}
			if openOnly {
				filtered := items[:0:0]
				for _, it := range items {
					if it.Status == models.DebtOpen {
						filtered = append(filtered, it)
					}
				}
				items = filtered
			}
			if jsonOutput {
				return printJSON(items)
			}
			if len(items) == 0 {
				fmt.Println("No tech-debt items. Record one with `adb debt add \"<title>\"`.")
				return nil
			}
			// os.Stdout, not cmd.OutOrStdout(): printJSON above writes there too, so
			// both shapes of this command land on one stream.
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tPRI\tSTATUS\tAREA\tTITLE")
			for _, it := range items {
				area := it.Area
				if area == "" {
					area = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", it.ID, it.Priority, it.Status, area, it.Title)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&openOnly, "open", false, "only show open items")
	return cmd
}

func newDebtResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Mark a tech-debt item resolved",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			item, err := App.DebtManager.Resolve(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s resolved\n", item.ID)
			return nil
		},
	}
	return cmd
}

package cli

import (
	"fmt"

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
can enumerate debt without minting worktrees.`,
	}
	cmd.AddCommand(newDebtAddCmd(), newDebtListCmd(), newDebtResolveCmd())
	return cmd
}

func newDebtAddCmd() *cobra.Command {
	var (
		priority   string
		area       string
		note       string
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
			item, err := App.DebtManager.Add(args[0], area, note, models.Priority(priority))
			if err != nil {
				return fmt.Errorf("failed to add debt item: %w", err)
			}
			if jsonOutput {
				return printJSON(item)
			}
			fmt.Printf("Recorded %s [%s]: %s\n", item.ID, item.Priority, item.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&priority, "priority", "P2", "priority P0|P1|P2|P3")
	cmd.Flags().StringVar(&area, "area", "", "subsystem/package the debt lives in")
	cmd.Flags().StringVar(&note, "note", "", "optional detail")
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
			for _, it := range items {
				area := it.Area
				if area == "" {
					area = "-"
				}
				fmt.Printf("%-10s %-3s %-8s %-16s %s\n", it.ID, it.Priority, it.Status, area, it.Title)
			}
			return nil
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

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewCRMCmd creates the `adb crm` command group — the MEDDPICC/Bowtie deal
// registry (#135 step 18).
func NewCRMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crm",
		Short: "Sales deals — MEDDPICC qualification + Bowtie funnel",
		Long: `Track sales opportunities with MEDDPICC qualification (Metrics, Economic buyer,
Decision criteria, Decision process, Paper process, Identify pain, Champion,
Competition) and a Bowtie funnel stage (awareness → education → selection →
onboarding → impact → expansion). Stored in crm/index.yaml.`,
	}
	cmd.AddCommand(newCRMAddCmd(), newCRMListCmd(), newCRMShowCmd(), newCRMSetStageCmd())
	return cmd
}

func newCRMAddCmd() *cobra.Command {
	var (
		stage      string
		m          models.MEDDPICC
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Record a sales deal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			deal, err := App.CRMManager.Add(args[0], models.BowtieStage(strings.TrimSpace(stage)), m)
			if err != nil {
				return fmt.Errorf("failed to add deal: %w", err)
			}
			if jsonOutput {
				return printJSON(deal)
			}
			fmt.Printf("Recorded %s: %s [%s] MEDDPICC %d/8\n", deal.ID, deal.Name, deal.Stage, deal.Score())
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&stage, "stage", "", "Bowtie stage (default awareness)")
	f.StringVar(&m.Metrics, "metrics", "", "MEDDPICC: quantified value/impact")
	f.StringVar(&m.EconomicBuyer, "economic-buyer", "", "MEDDPICC: who controls the budget")
	f.StringVar(&m.DecisionCriteria, "decision-criteria", "", "MEDDPICC: how they'll decide")
	f.StringVar(&m.DecisionProcess, "decision-process", "", "MEDDPICC: the decision steps")
	f.StringVar(&m.PaperProcess, "paper-process", "", "MEDDPICC: procurement/legal path")
	f.StringVar(&m.IdentifyPain, "identify-pain", "", "MEDDPICC: the pain being solved")
	f.StringVar(&m.Champion, "champion", "", "MEDDPICC: internal advocate")
	f.StringVar(&m.Competition, "competition", "", "MEDDPICC: the competing alternatives")
	f.BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newCRMListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deals (ordered by Bowtie funnel stage)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			deals, err := App.CRMManager.List()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(deals)
			}
			if len(deals) == 0 {
				fmt.Println("No deals. Record one with `adb crm add \"<name>\"`.")
				return nil
			}
			for _, d := range deals {
				fmt.Printf("%-10s %-11s MEDDPICC %d/8  %s\n", d.ID, d.Stage, d.Score(), d.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newCRMShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a deal with its MEDDPICC qualification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			deal, found, err := App.CRMManager.Get(args[0])
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no deal %q", args[0])
			}
			if jsonOutput {
				return printJSON(deal)
			}
			fmt.Printf("%s: %s\nStage: %s   MEDDPICC: %d/8\n\n", deal.ID, deal.Name, deal.Stage, deal.Score())
			row := func(label, val string) {
				if val == "" {
					val = "—"
				}
				fmt.Printf("  %-18s %s\n", label, val)
			}
			row("Metrics", deal.MEDDPICC.Metrics)
			row("Economic buyer", deal.MEDDPICC.EconomicBuyer)
			row("Decision criteria", deal.MEDDPICC.DecisionCriteria)
			row("Decision process", deal.MEDDPICC.DecisionProcess)
			row("Paper process", deal.MEDDPICC.PaperProcess)
			row("Identify pain", deal.MEDDPICC.IdentifyPain)
			row("Champion", deal.MEDDPICC.Champion)
			row("Competition", deal.MEDDPICC.Competition)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newCRMSetStageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-stage <id> <stage>",
		Short: "Move a deal to a Bowtie stage (awareness|education|selection|onboarding|impact|expansion)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			deal, err := App.CRMManager.SetStage(args[0], models.BowtieStage(strings.ToLower(strings.TrimSpace(args[1]))))
			if err != nil {
				return err
			}
			fmt.Printf("%s is now at %s\n", deal.ID, deal.Stage)
			return nil
		},
	}
	return cmd
}

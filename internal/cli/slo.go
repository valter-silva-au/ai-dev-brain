package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewSLOCmd creates the `adb slo` command group — the service-level objective
// registry (#131 step 17).
func NewSLOCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slo",
		Short: "Service-level objectives / agreements",
	}
	cmd.AddCommand(newSLOSetCmd(), newSLOListCmd())
	return cmd
}

func newSLOSetCmd() *cobra.Command {
	var (
		objective   float64
		window      string
		description string
		jsonOutput  bool
	)
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Record or update an SLO target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			slo, err := App.SLOManager.Set(args[0], objective, window, description)
			if err != nil {
				return fmt.Errorf("failed to set SLO: %w", err)
			}
			if jsonOutput {
				return printJSON(slo)
			}
			win := slo.Window
			if win == "" {
				win = "-"
			}
			fmt.Printf("Set SLO %q: %.3g%% over %s\n", slo.Name, slo.Objective, win)
			return nil
		},
	}
	cmd.Flags().Float64Var(&objective, "objective", 0, "target percentage in (0,100], e.g. 99.9 (required)")
	cmd.Flags().StringVar(&window, "window", "", "measurement window, e.g. 30d")
	cmd.Flags().StringVar(&description, "description", "", "what this SLO measures")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	_ = cmd.MarkFlagRequired("objective")
	return cmd
}

func newSLOListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List SLO targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			slos, err := App.SLOManager.List()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(slos)
			}
			if len(slos) == 0 {
				fmt.Println("No SLOs. Set one with `adb slo set <name> --objective <n>`.")
				return nil
			}
			for _, s := range slos {
				win := s.Window
				if win == "" {
					win = "-"
				}
				fmt.Printf("%-24s %6.3g%%  over %-6s  %s\n", s.Name, s.Objective, win, s.Description)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewADRCmd creates the `adb adr` command group — MADR architecture decision
// records (#128 step 16).
func NewADRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adr",
		Short: "Architecture decision records (MADR)",
		Long: `Manage Markdown Any Decision Records. The registry (adr/index.yaml) holds
number/title/status/links; the decision body lives at docs/adr/NNNN-<slug>.md.
Each ADR is an adr:NNNN node in the typed graph and shows up in adb catalog.`,
	}
	cmd.AddCommand(newADRNewCmd(), newADRListCmd(), newADRShowCmd(), newADRSetStatusCmd())
	return cmd
}

func newADRNewCmd() *cobra.Command {
	var (
		relatesTo  string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create the next-numbered ADR (status: proposed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			var links []models.Link
			if strings.TrimSpace(relatesTo) != "" {
				links = append(links, models.Link{Type: models.EdgeRelatesTo, Target: strings.TrimSpace(relatesTo)})
			}
			adr, err := App.ADRManager.New(args[0], links)
			if err != nil {
				return fmt.Errorf("failed to create ADR: %w", err)
			}
			if jsonOutput {
				return printJSON(adr)
			}
			fmt.Printf("Created ADR %04d: %s [%s]\n", adr.Number, adr.Title, adr.Status)
			fmt.Printf("  docs/adr/%s\n", adr.Filename())
			return nil
		},
	}
	cmd.Flags().StringVar(&relatesTo, "relates-to", "", "entity id this ADR relates to (a ticket/initiative), added as a relates_to edge")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newADRListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List architecture decision records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			adrs, err := App.ADRManager.List()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(adrs)
			}
			if len(adrs) == 0 {
				fmt.Println("No ADRs. Create one with `adb adr new \"<title>\"`.")
				return nil
			}
			for _, a := range adrs {
				fmt.Printf("%04d  %-10s %s\n", a.Number, a.Status, a.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newADRShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <number>",
		Short: "Show an ADR (metadata + markdown body)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			number, err := parseADRNumber(args[0])
			if err != nil {
				return err
			}
			adr, body, err := App.ADRManager.Show(number)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(map[string]any{"adr": adr, "body": body})
			}
			fmt.Printf("ADR %04d: %s\n", adr.Number, adr.Title)
			fmt.Printf("Status: %s\n\n", adr.Status)
			fmt.Println(body)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newADRSetStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-status <number> <status>",
		Short: "Transition an ADR's status (proposed|accepted|rejected|superseded|deprecated)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			number, err := parseADRNumber(args[0])
			if err != nil {
				return err
			}
			adr, err := App.ADRManager.SetStatus(number, models.ADRStatus(strings.ToLower(strings.TrimSpace(args[1]))))
			if err != nil {
				return err
			}
			fmt.Printf("ADR %04d is now %s\n", adr.Number, adr.Status)
			return nil
		},
	}
	return cmd
}

// parseADRNumber accepts either "12" or "adr:0012"/"ADR-12" forms.
func parseADRNumber(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToLower(s), "adr:")
	s = strings.TrimPrefix(strings.ToLower(s), "adr-")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid ADR number %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid ADR number %q (must be > 0)", s)
	}
	return n, nil
}

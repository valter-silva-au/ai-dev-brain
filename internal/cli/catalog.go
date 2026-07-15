package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewCatalogCmd creates the `adb catalog` command group — a Backstage-style
// generated inventory of every entity in the workspace (#128).
func NewCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Generated entity catalog (orgs, initiatives, tickets, nodes, metrics)",
		Long: `Show a generated inventory of every entity in the workspace, derived from the
registries and the typed graph (#109). Each entity is annotated with its graph
degree (incident edge count). Use --json for a machine-readable snapshot.`,
	}
	cmd.AddCommand(newCatalogShowCmd())
	return cmd
}

func newCatalogShowCmd() *cobra.Command {
	var (
		jsonOutput bool
		kind       string
	)
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the entity catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if App.CatalogBuilder == nil {
				return fmt.Errorf("catalog builder not initialized")
			}
			cat, err := App.CatalogBuilder.Build()
			if err != nil {
				return fmt.Errorf("failed to build catalog: %w", err)
			}

			kind = strings.ToLower(strings.TrimSpace(kind))
			switch kind {
			case "", "orgs", "initiatives", "tickets", "nodes", "metrics", "adrs":
			default:
				return fmt.Errorf("unknown --kind %q (want orgs|initiatives|tickets|nodes|metrics|adrs)", kind)
			}

			if jsonOutput {
				return printJSON(catalogForKind(cat, kind))
			}
			renderCatalog(cat, kind)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().StringVar(&kind, "kind", "", "limit to one kind: orgs|initiatives|tickets|nodes|metrics|adrs")
	return cmd
}

// catalogForKind returns either the whole catalog (kind == "") or a catalog with
// only the requested section populated, so --json --kind emits just that slice's
// context while keeping a stable envelope.
func catalogForKind(cat *models.Catalog, kind string) any {
	switch kind {
	case "orgs":
		return cat.Orgs
	case "initiatives":
		return cat.Initiatives
	case "tickets":
		return cat.Tickets
	case "nodes":
		return cat.IngestedNodes
	case "metrics":
		return cat.Metrics
	case "adrs":
		return cat.ADRs
	default:
		return cat
	}
}

func renderCatalog(cat *models.Catalog, kind string) {
	show := func(k string) bool { return kind == "" || kind == k }

	if show("orgs") {
		fmt.Printf("Organizations (%d):\n", len(cat.Orgs))
		for _, o := range cat.Orgs {
			fmt.Printf("  %-24s %-24s initiatives=%d edges=%d\n", o.ID, o.Name, o.Initiatives, o.Edges)
		}
	}
	if show("initiatives") {
		fmt.Printf("Initiatives (%d):\n", len(cat.Initiatives))
		for _, in := range cat.Initiatives {
			gate := in.Gate
			if gate == "" {
				gate = "-"
			}
			fmt.Printf("  %-24s stage=%-8s org=%-16s tickets=%d gate=%s edges=%d\n",
				in.ID, in.Stage, in.Org, in.Tickets, gate, in.Edges)
		}
	}
	if show("tickets") {
		fmt.Printf("Tickets (%d):\n", len(cat.Tickets))
		for _, t := range cat.Tickets {
			init := t.Initiative
			if init == "" {
				init = "-"
			}
			fmt.Printf("  %-14s %-8s %-12s init=%-16s edges=%d\n", t.ID, t.Type, t.Status, init, t.Edges)
		}
	}
	if show("nodes") {
		fmt.Printf("Ingested nodes (%d):\n", len(cat.IngestedNodes))
		for _, n := range cat.IngestedNodes {
			fmt.Printf("  %-28s %-14s edges=%d\n", n.ID, n.Type, n.Edges)
		}
	}
	if show("metrics") {
		fmt.Printf("Metrics (%d):\n", len(cat.Metrics))
		for _, m := range cat.Metrics {
			fmt.Printf("  %-32s %g%s (%s) edges=%d\n", m.ID, m.Value, m.Unit, m.Source, m.Edges)
		}
	}
	if show("adrs") {
		fmt.Printf("ADRs (%d):\n", len(cat.ADRs))
		for _, a := range cat.ADRs {
			fmt.Printf("  %-10s %-10s %s (edges=%d)\n", a.ID, a.Status, a.Title, a.Edges)
		}
	}
	if kind == "" {
		s := cat.Summary
		fmt.Printf("\nTotal: %d orgs, %d initiatives, %d tickets, %d nodes, %d metrics, %d adrs, %d edges\n",
			s.Orgs, s.Initiatives, s.Tickets, s.IngestedNodes, s.Metrics, s.ADRs, s.Edges)
	}
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewGraphCmd creates the `adb graph` command group for the generic typed edge
// graph (decision D6). Edges are declared in each entity's frontmatter
// (links[]) — the source of truth; this command surface materialises and
// inspects the derived, rebuildable index.
func NewGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Inspect the typed entity graph (edges declared in frontmatter)",
		Long: `Work with the generic typed edge graph.

Entities (tasks, initiatives) declare typed links in their persisted frontmatter
(the source of truth). The derived index at graph/index.yaml is a rebuildable
cache: delete it and rebuild and you get the same graph. Edge types are the
closed vocabulary relates_to, part_of, blocks, depends_on, duplicates
(unknown types read off disk are tolerated, never rejected).`,
	}
	cmd.AddCommand(newGraphRebuildCmd())
	cmd.AddCommand(newGraphNeighborsCmd())
	return cmd
}

func newGraphRebuildCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the derived graph index from entity frontmatter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.GraphManager == nil {
				return fmt.Errorf("app not initialized")
			}
			g, err := App.GraphManager.Rebuild()
			if err != nil {
				return fmt.Errorf("rebuild graph index: %w", err)
			}
			idx := g.Index()
			if jsonOutput {
				return printJSON(idx)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Rebuilt graph index: %d edge(s) written to graph/index.yaml.\n", len(idx.Edges))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the rebuilt index as JSON")
	return cmd
}

func newGraphNeighborsCmd() *cobra.Command {
	var (
		edgeType   string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "neighbors <id>",
		Short: "Show the edges incident to an entity (both directions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil || App.GraphManager == nil {
				return fmt.Errorf("app not initialized")
			}
			id := args[0]
			var (
				edges []models.GraphEdge
				err   error
			)
			if edgeType != "" {
				edges, err = App.GraphManager.NeighborsByType(id, models.EdgeType(edgeType))
			} else {
				edges, err = App.GraphManager.Neighbors(id)
			}
			if err != nil {
				return fmt.Errorf("read neighbours: %w", err)
			}
			if jsonOutput {
				return printJSON(edges)
			}
			if len(edges) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No edges incident to %q.\n", id)
				return nil
			}
			for _, e := range edges {
				fmt.Fprintf(cmd.OutOrStdout(), "%s --%s--> %s\n", e.From, e.Type, e.To)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&edgeType, "type", "", "filter to a single edge type, e.g. depends_on")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

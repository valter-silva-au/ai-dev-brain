package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// NewGTMCmd creates the `adb gtm` command group — scaffoldable go-to-market
// packs (positioning/messaging + moat-narrative, #135 step 18).
func NewGTMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gtm",
		Short: "Scaffold go-to-market packs (positioning/messaging, moat-narrative)",
	}
	cmd.AddCommand(newGTMListCmd(), newGTMScaffoldCmd())
	return cmd
}

func newGTMListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available GTM packs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			packs, err := core.GTMPacks(claude.FS)
			if err != nil {
				return err
			}
			for _, p := range packs {
				fmt.Println(p)
			}
			return nil
		},
	}
	return cmd
}

func newGTMScaffoldCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "scaffold <pack> [dest]",
		Short: "Scaffold a GTM pack's docs into the workspace",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			pack := args[0]
			destDir := filepath.Join(App.BasePath, "gtm", pack)
			if len(args) == 2 {
				destDir = args[1]
			}
			res, err := core.ScaffoldGTMPack(claude.FS, pack, destDir, core.HarnessInstallOptions{DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to scaffold %s: %w", pack, err)
			}
			verb := "Scaffolded"
			if dryRun {
				verb = "Would scaffold"
			}
			fmt.Printf("%s %s into %s:\n", verb, res.Pack, res.DestDir)
			for _, e := range res.Entries {
				fmt.Printf("  %-10s %s\n", e.Action, e.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite locally-edited files")
	return cmd
}

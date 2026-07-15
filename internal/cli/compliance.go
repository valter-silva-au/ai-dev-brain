package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// NewComplianceCmd creates the `adb compliance` command group — scaffoldable
// SOC2/GDPR/HIPAA control-checklist packs (#131 step 17).
func NewComplianceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "Scaffold compliance control-checklist packs (SOC2/GDPR/HIPAA)",
	}
	cmd.AddCommand(newComplianceListCmd(), newComplianceScaffoldCmd())
	return cmd
}

func newComplianceListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available compliance frameworks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			frameworks, err := core.ComplianceFrameworks(claude.FS)
			if err != nil {
				return err
			}
			for _, f := range frameworks {
				fmt.Println(f)
			}
			return nil
		},
	}
	return cmd
}

func newComplianceScaffoldCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "scaffold <framework> [dest]",
		Short: "Scaffold a framework's control-checklist docs into the workspace",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			framework := args[0]
			destDir := filepath.Join(App.BasePath, "compliance", framework)
			if len(args) == 2 {
				destDir = args[1]
			}
			res, err := core.ScaffoldComplianceFramework(claude.FS, framework, destDir, core.HarnessInstallOptions{DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to scaffold %s: %w", framework, err)
			}
			verb := "Scaffolded"
			if dryRun {
				verb = "Would scaffold"
			}
			fmt.Printf("%s %s into %s:\n", verb, res.Framework, res.DestDir)
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

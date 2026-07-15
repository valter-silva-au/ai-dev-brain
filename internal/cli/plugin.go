package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

// NewPluginCmd creates the `adb plugin` command group — graduate the embedded
// harness into a distributable Claude Code plugin + marketplace (#139 step 20,
// D12 phase 2).
func NewPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Graduate the harness into a Claude Code plugin + marketplace",
		Long: `Emit the adb founder-playbook harness (the devil's-advocate agent + the
stage-gate/ingestion skills + the adb MCP server) as a Claude Code plugin and a
single-plugin marketplace, installable via ` + "`/plugin marketplace add <dir>`" + `.`,
	}
	cmd.AddCommand(newPluginBuildCmd(), newPluginManifestCmd())
	return cmd
}

func newPluginBuildCmd() *cobra.Command {
	var (
		version string
		dryRun  bool
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "build [dest]",
		Short: "Emit the plugin + marketplace into a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// dest defaults to <workspace>/dist/plugin; falls back to ./dist/plugin
			// when no App (the command is otherwise stateless over the embedded FS).
			dest := "dist/plugin"
			if App != nil && App.BasePath != "" {
				dest = filepath.Join(App.BasePath, "dist", "plugin")
			}
			if len(args) == 1 {
				dest = args[0]
			}
			res, err := core.BuildPlugin(claude.FS, dest, core.PluginBuildOptions{Version: version, DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to build plugin: %w", err)
			}
			verb := "Built"
			if dryRun {
				verb = "Would build"
			}
			fmt.Printf("%s plugin %q v%s into %s:\n", verb, res.Plugin.Name, res.Plugin.Version, res.DestDir)
			for _, e := range res.Entries {
				fmt.Printf("  %-10s %s\n", e.Action, e.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "plugin version (default "+core.DefaultPluginVersion+")")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite locally-edited files")
	return cmd
}

func newPluginManifestCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Print the plugin.json manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printJSON(core.PluginManifestFor(version))
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "plugin version (default "+core.DefaultPluginVersion+")")
	return cmd
}

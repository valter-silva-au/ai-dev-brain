package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// NewConfigCmd creates the `adb config` command group for inspecting the
// three-tier configuration (Global → Org → Repo).
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the layered configuration (Global → Org → Repo)",
		Long: `Inspect adb's three-tier configuration.

Precedence is most-specific-wins: Repo (.taskrc) > Org (orgs/<id>/config.yaml) >
Global (.taskconfig) > built-in defaults. The active org tier is selected by the
ADB_ORG env var, falling back to the ` + "`org`" + ` field in .taskrc; when neither is
set the org tier is inactive and behaviour is the historical two-tier merge.`,
	}
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigGetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the resolved configuration tiers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			mc := App.MergedConfig
			if mc == nil {
				return fmt.Errorf("no configuration loaded")
			}
			if jsonOutput {
				return printJSON(mc)
			}

			fmt.Println("Config tiers (precedence: Repo > Org > Global):")
			globalPrefix := ""
			if mc.Global != nil {
				globalPrefix = mc.Global.TaskIDPrefix
			}
			fmt.Printf("  Global  present=%v  task_id_prefix=%s\n", mc.Global != nil, globalPrefix)
			if mc.Org != nil {
				fmt.Printf("  Org     active=%s  (orgs/%s/config.yaml)\n", mc.Org.OrgID, mc.Org.OrgID)
			} else {
				fmt.Printf("  Org     (none — set `org:` in .taskrc or ADB_ORG to activate)\n")
			}
			repoName := ""
			if mc.Repo != nil {
				repoName = mc.Repo.RepoName
			}
			fmt.Printf("  Repo    present=%v  repo_name=%s\n", mc.Repo != nil, repoName)

			// Resolved custom settings across all tiers.
			resolved := resolveAllSettings(mc)
			if len(resolved) > 0 {
				fmt.Println("\nResolved custom settings (key = value [tier]):")
				keys := make([]string, 0, len(resolved))
				for k := range resolved {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					v, tier, _ := mc.SettingSource(k)
					fmt.Printf("  %s = %s [%s]\n", k, v, tier)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the merged config as JSON")
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	var showSource bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Resolve a custom setting across the tiers (Repo > Org > Global)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			mc := App.MergedConfig
			if mc == nil {
				return fmt.Errorf("no configuration loaded")
			}
			value, tier, ok := mc.SettingSource(args[0])
			if !ok {
				return fmt.Errorf("no config setting %q in any tier", args[0])
			}
			if showSource {
				fmt.Printf("%s\t[%s]\n", value, tier)
			} else {
				fmt.Println(value)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showSource, "source", false, "also print which tier the value came from")
	return cmd
}

// resolveAllSettings gathers the union of custom-setting keys across all tiers.
// The value chosen per key follows MergedConfig.SettingSource precedence.
func resolveAllSettings(mc *models.MergedConfig) map[string]struct{} {
	keys := map[string]struct{}{}
	if mc == nil {
		return keys
	}
	add := func(m map[string]string) {
		for k := range m {
			keys[k] = struct{}{}
		}
	}
	if mc.Global != nil {
		add(mc.Global.CustomSettings)
	}
	if mc.Org != nil {
		add(mc.Org.CustomSettings)
	}
	if mc.Repo != nil {
		add(mc.Repo.CustomSettings)
	}
	return keys
}

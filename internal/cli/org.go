package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewOrgCmd creates the `adb org` command group for managing organizations.
func NewOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations (businesses)",
		Long: `Manage organizations — the businesses tracked in this workspace.

An organization defaults to a git-host org and holds one or more initiatives.
Organizations are stored as workspace metadata (orgs/index.yaml); they are not
part of the ticket/worktree path layout.`,
	}
	cmd.AddCommand(NewOrgCreateCmd())
	cmd.AddCommand(newOrgListCmd())
	cmd.AddCommand(newOrgShowCmd())
	return cmd
}

// NewOrgCreateCmd creates the `adb org create` command. It is exported because
// the v3-composed root registers the legacy `org` tree with IncludeOrg:false —
// v3's `adb org` owns the .aidb trust-scope registry — and then mounts this
// command onto it. Without that, nothing public writes the playbook registry
// (orgs/index.yaml) that `adb initiative` reads, which left `adb initiative`
// and `adb stage` unreachable on a fresh workspace.
func NewOrgCreateCmd() *cobra.Command {
	var (
		gitHost    string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a playbook organization (holds initiatives)",
		Long: `Create a playbook organization — the business that holds initiatives.

This writes the workspace's playbook registry (orgs/index.yaml), which is what
` + "`adb initiative`" + ` and ` + "`adb stage`" + ` read. It is a different store from
` + "`adb org init`" + `, which registers a v3 .aidb trust scope; the two are not
interchangeable, and an initiative can only reference an org created here.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			org, err := App.StageManager.CreateOrganization(args[0], gitHost)
			if err != nil {
				return fmt.Errorf("failed to create organization: %w", err)
			}
			if jsonOutput {
				return printJSON(org)
			}
			fmt.Printf("Created organization %q (%s)\n", org.Name, org.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&gitHost, "git-host", "", "git host this org maps to, e.g. github.com")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newOrgListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			orgs, err := App.StageManager.ListOrganizations()
			if err != nil {
				return fmt.Errorf("failed to list organizations: %w", err)
			}
			if jsonOutput {
				return printJSON(orgs)
			}
			if len(orgs) == 0 {
				fmt.Println("No organizations. Create one with `adb org create <name>`.")
				return nil
			}
			for _, org := range orgs {
				host := org.GitHost
				if host == "" {
					host = "-"
				}
				fmt.Printf("%-24s %-24s %s\n", org.ID, org.Name, host)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func newOrgShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			org, err := App.StageManager.GetOrganization(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(org)
			}
			fmt.Printf("ID:       %s\n", org.ID)
			fmt.Printf("Name:     %s\n", org.Name)
			fmt.Printf("Git host: %s\n", org.GitHost)
			fmt.Printf("Created:  %s\n", org.Created.Format("2006-01-02 15:04:05 MST"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// printJSON marshals v as indented JSON to stdout.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

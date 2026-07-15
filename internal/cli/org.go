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
	cmd.AddCommand(newOrgCreateCmd())
	cmd.AddCommand(newOrgListCmd())
	cmd.AddCommand(newOrgShowCmd())
	return cmd
}

func newOrgCreateCmd() *cobra.Command {
	var (
		gitHost    string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an organization",
		Args:  cobra.ExactArgs(1),
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

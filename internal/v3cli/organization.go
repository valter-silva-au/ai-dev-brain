package v3cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

const (
	cliActorType = "human"
	cliTool      = "adb-cli"
)

func newOrganizationCommand(service Organization) *cobra.Command {
	command := &cobra.Command{
		Use:   "org",
		Short: "Manage organizations",
		Long: `Manage organizations.

Two registries live under this noun, and they are not interchangeable:

  init, list, show, update, adopt, move, archive, validate
      v3 trust scopes, stored as .aidb organization manifests.

  create
      playbook organizations, stored in the workspace registry that
      ` + "`adb initiative`" + ` and ` + "`adb stage`" + ` read. List them with
      ` + "`adb catalog show --kind orgs`" + `.

An initiative can only reference an organization made with ` + "`adb org create`" + `.`,
		Annotations: map[string]string{
			v3Annotation: "true",
		},
	}
	command.AddCommand(
		newOrganizationInitializeCommand(service),
		newOrganizationListCommand(service),
		newOrganizationShowCommand(service),
		newOrganizationUpdateCommand(service),
		newOrganizationAdoptCommand(service),
		newOrganizationMoveCommand(service),
		newOrganizationArchiveCommand(service),
		newOrganizationValidateCommand(service),
		// The playbook-registry creator. Composing the legacy tree with
		// IncludeOrg:false unregistered it along with the rest of legacy `org`,
		// which left `adb initiative create --org X` failing with
		// `organization "X" not found` and no public way to fix that.
		//
		// It deliberately carries NO v3 annotation: it needs the legacy App
		// (StageManager), and the annotation is what tells the root's
		// PersistentPreRunE to skip loading it.
		cli.NewOrgCreateCmd(),
	)
	return command
}

func newOrganizationInitializeCommand(service Organization) *cobra.Command {
	var (
		workspacePath string
		name          string
		parent        string
		owner         string
		description   string
		trust         string
		profile       string
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		organization.InitializeDescriptor.Command+" <slug>",
		organization.InitializeDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			displayName := name
			if displayName == "" {
				displayName = args[0]
			}
			result, err := service.Initialize(
				command.Context(),
				organization.InitializeRequest{
					WorkspaceRoot: root,
					Slug:          args[0],
					Name:          displayName,
					Parent:        parent,
					Owner:         owner,
					Description:   description,
					Trust:         trust,
					Profile:       profile,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationInitializeHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().StringVar(&name, "name", "", "Organization display name")
	addOrganizationMetadataFlags(
		command,
		&parent,
		&owner,
		&description,
		&trust,
		&profile,
	)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newOrganizationListCommand(service Organization) *cobra.Command {
	var (
		workspacePath   string
		includeArchived bool
		format          string
	)
	command := v3Command(
		organization.ListDescriptor.Command,
		organization.ListDescriptor.Summary,
		cobra.NoArgs,
		func(command *cobra.Command, _ []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.List(
				command.Context(),
				organization.ListRequest{
					WorkspaceRoot:   root,
					IncludeArchived: includeArchived,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationListHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().BoolVar(
		&includeArchived,
		"include-archived",
		false,
		"Include archived organizations",
	)
	addFormatFlag(command, &format)
	return command
}

func newOrganizationShowCommand(service Organization) *cobra.Command {
	var workspacePath, format string
	command := v3Command(
		organization.ShowDescriptor.Command+" <selector>",
		organization.ShowDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Show(
				command.Context(),
				organization.ShowRequest{
					WorkspaceRoot: root,
					Selector:      args[0],
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationShowHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	addFormatFlag(command, &format)
	return command
}

func newOrganizationUpdateCommand(service Organization) *cobra.Command {
	var (
		workspacePath string
		name          string
		parent        string
		owner         string
		description   string
		trust         string
		profile       string
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		organization.UpdateDescriptor.Command+" <selector>",
		organization.UpdateDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Update(
				command.Context(),
				organization.UpdateRequest{
					WorkspaceRoot: root,
					Selector:      args[0],
					Name:          changedString(command, "name", name),
					Parent:        changedString(command, "parent", parent),
					Owner:         changedString(command, "owner", owner),
					Description: changedString(
						command,
						"description",
						description,
					),
					Trust:     changedString(command, "trust", trust),
					Profile:   changedString(command, "profile", profile),
					ActorType: cliActorType,
					Tool:      cliTool,
					Apply:     apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationMutationHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().StringVar(&name, "name", "", "Organization display name")
	addOrganizationMetadataFlags(
		command,
		&parent,
		&owner,
		&description,
		&trust,
		&profile,
	)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newOrganizationAdoptCommand(service Organization) *cobra.Command {
	var (
		workspacePath string
		slug          string
		name          string
		parent        string
		owner         string
		description   string
		trust         string
		profile       string
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		organization.AdoptDescriptor.Command+" <path>",
		organization.AdoptDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			source, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve organization adoption path: %w", err)
			}
			result, err := service.Adopt(
				command.Context(),
				organization.AdoptRequest{
					WorkspaceRoot: root,
					Path:          source,
					Slug:          slug,
					Name:          name,
					Parent:        parent,
					Owner:         owner,
					Description:   description,
					Trust:         trust,
					Profile:       profile,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationMutationHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().StringVar(&slug, "slug", "", "Canonical organization slug")
	requireFlag(command, "slug")
	command.Flags().StringVar(&name, "name", "", "Organization display name")
	requireFlag(command, "name")
	addOrganizationMetadataFlags(
		command,
		&parent,
		&owner,
		&description,
		&trust,
		&profile,
	)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newOrganizationMoveCommand(service Organization) *cobra.Command {
	var workspacePath, slug, format string
	var apply, dryRun bool
	command := v3Command(
		organization.MoveDescriptor.Command+" <selector>",
		organization.MoveDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Move(
				command.Context(),
				organization.MoveRequest{
					WorkspaceRoot: root,
					Selector:      args[0],
					NewSlug:       slug,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationMutationHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().StringVar(&slug, "slug", "", "New canonical organization slug")
	requireFlag(command, "slug")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newOrganizationArchiveCommand(service Organization) *cobra.Command {
	var workspacePath, format string
	var restore, apply, dryRun bool
	command := v3Command(
		organization.ArchiveDescriptor.Command+" <selector>",
		organization.ArchiveDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Archive(
				command.Context(),
				organization.ArchiveRequest{
					WorkspaceRoot: root,
					Selector:      args[0],
					Restore:       restore,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationMutationHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	command.Flags().BoolVar(
		&restore,
		"restore",
		false,
		"Restore an archived organization",
	)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newOrganizationValidateCommand(service Organization) *cobra.Command {
	var workspacePath, format string
	command := v3Command(
		organization.ValidateDescriptor.Command+" <selector>",
		organization.ValidateDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("organization service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Validate(
				command.Context(),
				organization.ValidateRequest{
					WorkspaceRoot: root,
					Selector:      args[0],
				},
			)
			if err != nil {
				return err
			}
			return writeResult(
				command,
				format,
				result,
				renderOrganizationValidateHuman,
			)
		},
	)
	addWorkspaceFlag(command, &workspacePath)
	addFormatFlag(command, &format)
	return command
}

func v3Command(
	use string,
	short string,
	args cobra.PositionalArgs,
	run func(*cobra.Command, []string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		Annotations: map[string]string{
			v3Annotation: "true",
		},
		RunE: run,
	}
}

func addWorkspaceFlag(command *cobra.Command, target *string) {
	command.Flags().StringVar(
		target,
		"workspace",
		"",
		"Explicit v3 workspace path",
	)
	requireFlag(command, "workspace")
}

func addOrganizationMetadataFlags(
	command *cobra.Command,
	parent *string,
	owner *string,
	description *string,
	trust *string,
	profile *string,
) {
	command.Flags().StringVar(parent, "parent", "", "Parent organization selector")
	command.Flags().StringVar(owner, "owner", "", "Organization owner")
	command.Flags().StringVar(
		description,
		"description",
		"",
		"Organization description",
	)
	command.Flags().StringVar(trust, "trust", "", "Organization trust policy")
	command.Flags().StringVar(profile, "profile", "", "Organization profile")
}

func addMutationOutputFlags(
	command *cobra.Command,
	apply *bool,
	dryRun *bool,
	format *string,
) {
	command.Flags().BoolVar(apply, "apply", false, "Apply the plan")
	command.Flags().BoolVar(
		dryRun,
		"dry-run",
		false,
		"Preview without writing (the default)",
	)
	addFormatFlag(command, format)
}

func addFormatFlag(command *cobra.Command, format *string) {
	command.Flags().StringVar(
		format,
		"format",
		"human",
		"Output format: human or json",
	)
}

func resolveOrganizationWorkspace(path string) (string, error) {
	if path == "" {
		return "", errors.New("--workspace is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	return absolute, nil
}

func validateApplyFlags(apply bool, dryRun bool) error {
	if apply && dryRun {
		return errors.New("--apply and --dry-run are mutually exclusive")
	}
	return nil
}

func changedString(
	command *cobra.Command,
	name string,
	value string,
) *string {
	if !command.Flags().Changed(name) {
		return nil
	}
	return &value
}

func renderOrganizationInitializeHuman(
	writer io.Writer,
	result capability.Result[organization.InitializeData],
) error {
	if err := renderCapabilityHeader(
		writer,
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		writer,
		"Organization: %s\nSlug: %s\nPath: %s\n",
		result.Data.OrganizationID,
		result.Data.Slug,
		result.Data.Path,
	); err != nil {
		return err
	}
	if err := renderEffects(writer, result.Effects); err != nil {
		return err
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}

func renderOrganizationListHuman(
	writer io.Writer,
	result capability.Result[organization.ListData],
) error {
	if err := renderCapabilityHeader(
		writer,
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	for _, item := range result.Data.Organizations {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s %s %s\n",
			item.Slug,
			item.Status,
			item.DisplayName,
			item.ID,
		); err != nil {
			return err
		}
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}

func renderOrganizationShowHuman(
	writer io.Writer,
	result capability.Result[organization.ShowData],
) error {
	if err := renderCapabilityHeader(
		writer,
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if err := renderOrganizationData(writer, result.Data.Organization); err != nil {
		return err
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}

func renderOrganizationMutationHuman(
	writer io.Writer,
	result capability.Result[organization.MutationData],
) error {
	if err := renderCapabilityHeader(
		writer,
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if err := renderOrganizationData(writer, result.Data.Organization); err != nil {
		return err
	}
	if err := renderEffects(writer, result.Effects); err != nil {
		return err
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}

func renderOrganizationValidateHuman(
	writer io.Writer,
	result capability.Result[organization.ValidateData],
) error {
	if err := renderCapabilityHeader(
		writer,
		result.Capability,
		result.Version,
		result.Outcome,
	); err != nil {
		return err
	}
	if err := renderOrganizationData(writer, result.Data.Organization); err != nil {
		return err
	}
	for _, finding := range result.Data.Findings {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s: %s\n",
			finding.Severity,
			finding.ID,
			finding.Summary,
		); err != nil {
			return err
		}
	}
	return renderCommonHuman(
		writer,
		result.Warnings,
		result.NextActions,
		result.Recovery,
	)
}

func renderOrganizationData(
	writer io.Writer,
	data organization.Data,
) error {
	_, err := fmt.Fprintf(
		writer,
		"Organization: %s\nSlug: %s\nName: %s\nStatus: %s\nPath: %s\n",
		data.ID,
		data.Slug,
		data.DisplayName,
		data.Status,
		data.Path,
	)
	return err
}

func renderCapabilityHeader(
	writer io.Writer,
	name string,
	version string,
	outcome capability.Outcome,
) error {
	_, err := fmt.Fprintf(writer, "%s/%s: %s\n", name, version, outcome)
	return err
}

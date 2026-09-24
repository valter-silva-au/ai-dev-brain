package v3cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

func newRepositoryCommand(service Repository) *cobra.Command {
	command := &cobra.Command{
		Use:   "repo",
		Short: "Manage v3 repositories",
		Annotations: map[string]string{
			v3Annotation: "true",
		},
	}
	command.AddCommand(
		// The legacy `adb repos` children, folded onto this noun (TASK-00039 Q4).
		// `repos list` could not keep the name `list` — the v3 `repo list` below
		// already means registered repositories from the control plane, not clones
		// on disk — so it is `inventory`. Both read the legacy App, so like
		// `org create` they carry NO v3 annotation, which is what makes the root's
		// PersistentPreRunE load it.
		cli.NewRepoPullCmd(),
		cli.NewRepoInventoryCmd(),
		newRepositoryAddCommand(service),
		newRepositoryAdoptCommand(service),
		newRepositoryListCommand(service),
		newRepositoryShowCommand(service),
		newRepositoryHealthCommand(service),
		newRepositoryFetchCommand(service),
		newRepositoryUpdateCommand(service),
		newRepositoryMoveCommand(service),
		newRepositoryArchiveCommand(service),
		newRepositoryWorktreeCommand(service),
	)
	return command
}

func newRepositoryAddCommand(service Repository) *cobra.Command {
	var (
		workspacePath string
		organization  string
		mode          string
		host          string
		owner         string
		name          string
		displayName   string
		remoteName    string
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		repository.AddDescriptor.Command+" <remote>",
		repository.AddDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
			}
			if err := validateApplyFlags(apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Add(command.Context(), repository.AddRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Mode:          repository.AddMode(mode),
				Host:          host,
				Owner:         owner,
				Name:          name,
				DisplayName:   displayName,
				Remote:        args[0],
				RemoteName:    remoteName,
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().StringVar(
		&mode,
		"mode",
		string(repository.AddModeCloneNew),
		"Add mode: clone-new or initialize-local",
	)
	command.Flags().StringVar(&host, "host", "", "Repository host override")
	command.Flags().StringVar(&owner, "owner", "", "Repository owner override")
	command.Flags().StringVar(&name, "name", "", "Repository name override")
	command.Flags().StringVar(&displayName, "display-name", "", "Repository display name")
	command.Flags().StringVar(&remoteName, "remote-name", "", "Canonical Git remote name")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryAdoptCommand(service Repository) *cobra.Command {
	var (
		workspacePath string
		organization  string
		remoteName    string
		displayName   string
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		repository.AdoptDescriptor.Command+" <path>",
		repository.AdoptDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
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
				return fmt.Errorf("resolve repository path: %w", err)
			}
			result, err := service.Adopt(command.Context(), repository.AdoptRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Path:          source,
				RemoteName:    remoteName,
				DisplayName:   displayName,
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().StringVar(&remoteName, "remote-name", "", "Canonical Git remote name")
	command.Flags().StringVar(&displayName, "display-name", "", "Repository display name")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryListCommand(service Repository) *cobra.Command {
	var (
		workspacePath   string
		organization    string
		includeArchived bool
		format          string
	)
	command := v3Command(
		repository.ListDescriptor.Command,
		repository.ListDescriptor.Summary,
		cobra.NoArgs,
		func(command *cobra.Command, _ []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.List(command.Context(), repository.ListRequest{
				WorkspaceRoot:   root,
				Organization:    organization,
				IncludeArchived: includeArchived,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryListHuman)
		},
	)
	addRepositoryScopeFlagsWithOrg(
		command,
		&workspacePath,
		&organization,
		false,
	)
	command.Flags().BoolVar(
		&includeArchived,
		"include-archived",
		false,
		"Include archived repositories (and archived organizations)",
	)
	addFormatFlag(command, &format)
	return command
}

func newRepositoryShowCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	command := v3Command(
		repository.ShowDescriptor.Command+" <selector>",
		repository.ShowDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Show(command.Context(), repository.ShowRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryShowHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	addFormatFlag(command, &format)
	return command
}

func newRepositoryHealthCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	command := v3Command(
		repository.HealthDescriptor.Command+" <selector>",
		repository.HealthDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Health(command.Context(), repository.HealthRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryHealthHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	addFormatFlag(command, &format)
	return command
}

func newRepositoryFetchCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	var apply, dryRun bool
	command := v3Command(
		repository.FetchDescriptor.Command+" <selector>",
		repository.FetchDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Fetch(command.Context(), repository.FetchRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryUpdateCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	var apply, dryRun bool
	command := v3Command(
		repository.UpdateDescriptor.Command+" <selector>",
		repository.UpdateDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Update(command.Context(), repository.UpdateRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryMoveCommand(service Repository) *cobra.Command {
	var (
		workspacePath string
		organization  string
		newRemote     string
		updateRemote  bool
		apply         bool
		dryRun        bool
		format        string
	)
	command := v3Command(
		repository.MoveDescriptor.Command+" <selector>",
		repository.MoveDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Move(command.Context(), repository.MoveRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
				NewRemote:     newRemote,
				UpdateRemote:  updateRemote,
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().StringVar(&newRemote, "remote", "", "New canonical repository remote")
	requireFlag(command, "remote")
	command.Flags().BoolVar(
		&updateRemote,
		"update-remote",
		false,
		"Update the canonical clone's Git remote",
	)
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryArchiveCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	var restore, apply, dryRun bool
	command := v3Command(
		repository.ArchiveDescriptor.Command+" <selector>",
		repository.ArchiveDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.Archive(command.Context(), repository.ArchiveRequest{
				WorkspaceRoot: root,
				Organization:  organization,
				Selector:      args[0],
				Restore:       restore,
				ActorType:     cliActorType,
				Tool:          cliTool,
				Apply:         apply,
			})
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().BoolVar(&restore, "restore", false, "Restore an archived repository")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryWorktreeCommand(service Repository) *cobra.Command {
	command := &cobra.Command{
		Use:   "worktree",
		Short: "Manage repository worktree ownership",
		Annotations: map[string]string{
			v3Annotation: "true",
		},
	}
	command.AddCommand(
		newRepositoryWorktreeListCommand(service),
		newRepositoryWorktreeRepairCommand(service),
		newRepositoryWorktreePruneCommand(service),
	)
	return command
}

func newRepositoryWorktreeListCommand(service Repository) *cobra.Command {
	var workspacePath, organization, format string
	command := v3Command(
		repository.WorktreeListDescriptor.Command+" <selector>",
		repository.WorktreeListDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if service == nil {
				return errors.New("repository service is not configured")
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.WorktreeList(
				command.Context(),
				repository.WorktreeListRequest{
					WorkspaceRoot: root,
					Organization:  organization,
					Selector:      args[0],
				},
			)
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryWorktreeListHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	addFormatFlag(command, &format)
	return command
}

func newRepositoryWorktreeRepairCommand(service Repository) *cobra.Command {
	var workspacePath, organization, ticketKey, name, format string
	var apply, dryRun bool
	command := v3Command(
		repository.WorktreeRepairDescriptor.Command+" <selector>",
		repository.WorktreeRepairDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			result, err := service.WorktreeRepair(
				command.Context(),
				repository.WorktreeRepairRequest{
					WorkspaceRoot: root,
					Organization:  organization,
					Selector:      args[0],
					TicketKey:     ticketKey,
					Name:          name,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryWorktreeMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().StringVar(&ticketKey, "ticket", "", "Registered ticket key")
	requireFlag(command, "ticket")
	command.Flags().StringVar(&name, "name", "", "Registered worktree name")
	requireFlag(command, "name")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func newRepositoryWorktreePruneCommand(service Repository) *cobra.Command {
	var workspacePath, organization, targetPath, format string
	var apply, dryRun bool
	command := v3Command(
		repository.WorktreePruneDescriptor.Command+" <selector>",
		repository.WorktreePruneDescriptor.Summary,
		cobra.ExactArgs(1),
		func(command *cobra.Command, args []string) error {
			if err := ensureRepositoryMutation(service, apply, dryRun); err != nil {
				return err
			}
			root, err := resolveOrganizationWorkspace(workspacePath)
			if err != nil {
				return err
			}
			target, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("resolve worktree path: %w", err)
			}
			result, err := service.WorktreePrune(
				command.Context(),
				repository.WorktreePruneRequest{
					WorkspaceRoot: root,
					Organization:  organization,
					Selector:      args[0],
					Path:          target,
					ActorType:     cliActorType,
					Tool:          cliTool,
					Apply:         apply,
				},
			)
			if err != nil {
				return err
			}
			return writeResult(command, format, result, renderRepositoryWorktreeMutationHuman)
		},
	)
	addRepositoryScopeFlags(command, &workspacePath, &organization)
	command.Flags().StringVar(&targetPath, "path", "", "Exact worktree path to prune")
	requireFlag(command, "path")
	addMutationOutputFlags(command, &apply, &dryRun, &format)
	return command
}

func addRepositoryScopeFlags(
	command *cobra.Command,
	workspacePath *string,
	organization *string,
) {
	addRepositoryScopeFlagsWithOrg(command, workspacePath, organization, true)
}

// addRepositoryScopeFlagsWithOrg adds the shared --workspace/--org pair.
//
// orgRequired is false only for `list`. Every other subcommand addresses one
// repository by a selector that is unique within an organization, so a missing
// --org there would turn into an ambiguous lookup; `list` addresses a set, and
// the empty set-of-organizations is a legitimate, broader question.
func addRepositoryScopeFlagsWithOrg(
	command *cobra.Command,
	workspacePath *string,
	organization *string,
	orgRequired bool,
) {
	addWorkspaceFlag(command, workspacePath)
	usage := "Organization ID, slug, path, or alias"
	if !orgRequired {
		usage += " (omit to list every organization's repositories)"
	}
	command.Flags().StringVar(organization, "org", "", usage)
	if orgRequired {
		requireFlag(command, "org")
	}
}

func ensureRepositoryMutation(
	service Repository,
	apply bool,
	dryRun bool,
) error {
	if service == nil {
		return errors.New("repository service is not configured")
	}
	return validateApplyFlags(apply, dryRun)
}

func renderRepositoryMutationHuman(
	writer io.Writer,
	result capability.Result[repository.MutationData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	if err := renderRepositoryData(writer, result.Data.Repository); err != nil {
		return err
	}
	return renderRepositoryTail(writer, result.Effects, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryListHuman(
	writer io.Writer,
	result capability.Result[repository.ListData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	// A workspace-wide list (no --org, so no scope in the envelope) crosses
	// organizations, and a row that does not name its owner is ambiguous there.
	// A scoped list already knows the answer, so its output is unchanged.
	workspaceWide := result.Data.OrganizationID == ""
	for _, item := range result.Data.Repositories {
		if workspaceWide {
			// The slug, not the ID: it is the spelling --org accepts, so the row
			// doubles as the command that narrows to it.
			if _, err := fmt.Fprintf(writer, "%s %s/%s/%s %s %s\n", item.OrganizationSlug, item.Host, item.Owner, item.Name, item.Status, item.ID); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s/%s/%s %s %s\n", item.Host, item.Owner, item.Name, item.Status, item.ID); err != nil {
			return err
		}
	}
	return renderCommonHuman(writer, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryShowHuman(
	writer io.Writer,
	result capability.Result[repository.ShowData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	if err := renderRepositoryData(writer, result.Data.Repository); err != nil {
		return err
	}
	return renderCommonHuman(writer, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryHealthHuman(
	writer io.Writer,
	result capability.Result[repository.HealthData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	if err := renderRepositoryData(writer, result.Data.Repository); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Health: %s\n", result.Data.State); err != nil {
		return err
	}
	return renderCommonHuman(writer, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryWorktreeListHuman(
	writer io.Writer,
	result capability.Result[repository.WorktreeListData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	for _, worktree := range result.Data.Worktrees {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s registered=%t active=%t missing=%t unknown=%t dirty=%t ahead=%d\n",
			worktree.Path,
			worktree.Branch,
			worktree.Registered,
			worktree.Active,
			worktree.Missing,
			worktree.Unknown,
			worktree.Dirty,
			worktree.Ahead,
		); err != nil {
			return err
		}
	}
	return renderCommonHuman(writer, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryWorktreeMutationHuman(
	writer io.Writer,
	result capability.Result[repository.WorktreeMutationData],
) error {
	if err := renderRepositoryHeader(writer, result.Capability, result.Version, result.Outcome); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		writer,
		"Worktree: %s\nBranch: %s\n",
		result.Data.Worktree.Path,
		result.Data.Worktree.Branch,
	); err != nil {
		return err
	}
	return renderRepositoryTail(writer, result.Effects, result.Warnings, result.NextActions, result.Recovery)
}

func renderRepositoryHeader(
	writer io.Writer,
	capabilityName string,
	version string,
	outcome capability.Outcome,
) error {
	return renderCapabilityHeader(writer, capabilityName, version, outcome)
}

func renderRepositoryData(writer io.Writer, data repository.Data) error {
	_, err := fmt.Fprintf(
		writer,
		"Repository: %s\nIdentity: %s/%s/%s\nPath: %s\nClone: %s\n",
		data.ID,
		data.Host,
		data.Owner,
		data.Name,
		data.Path,
		data.ClonePath,
	)
	return err
}

func renderRepositoryTail(
	writer io.Writer,
	effects []capability.Effect,
	warnings []capability.Notice,
	actions []capability.Action,
	recovery capability.Recovery,
) error {
	if err := renderEffects(writer, effects); err != nil {
		return err
	}
	return renderCommonHuman(writer, warnings, actions, recovery)
}

package v3cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

const v3Annotation = "aidb.v3"

type Foundation interface {
	Initialize(
		context.Context,
		foundation.InitializeRequest,
	) (capability.Result[foundation.InitializeData], error)
	Doctor(
		context.Context,
		foundation.DoctorRequest,
	) (capability.Result[foundation.DoctorData], error)
}

type Organization interface {
	Initialize(
		context.Context,
		organization.InitializeRequest,
	) (capability.Result[organization.InitializeData], error)
	List(
		context.Context,
		organization.ListRequest,
	) (capability.Result[organization.ListData], error)
	Show(
		context.Context,
		organization.ShowRequest,
	) (capability.Result[organization.ShowData], error)
	Update(
		context.Context,
		organization.UpdateRequest,
	) (capability.Result[organization.MutationData], error)
	Adopt(
		context.Context,
		organization.AdoptRequest,
	) (capability.Result[organization.MutationData], error)
	Move(
		context.Context,
		organization.MoveRequest,
	) (capability.Result[organization.MutationData], error)
	Archive(
		context.Context,
		organization.ArchiveRequest,
	) (capability.Result[organization.MutationData], error)
	Validate(
		context.Context,
		organization.ValidateRequest,
	) (capability.Result[organization.ValidateData], error)
}

type Repository interface {
	Add(
		context.Context,
		repository.AddRequest,
	) (capability.Result[repository.MutationData], error)
	Adopt(
		context.Context,
		repository.AdoptRequest,
	) (capability.Result[repository.MutationData], error)
	List(
		context.Context,
		repository.ListRequest,
	) (capability.Result[repository.ListData], error)
	Show(
		context.Context,
		repository.ShowRequest,
	) (capability.Result[repository.ShowData], error)
	Health(
		context.Context,
		repository.HealthRequest,
	) (capability.Result[repository.HealthData], error)
	Fetch(
		context.Context,
		repository.FetchRequest,
	) (capability.Result[repository.MutationData], error)
	Update(
		context.Context,
		repository.UpdateRequest,
	) (capability.Result[repository.MutationData], error)
	Move(
		context.Context,
		repository.MoveRequest,
	) (capability.Result[repository.MutationData], error)
	Archive(
		context.Context,
		repository.ArchiveRequest,
	) (capability.Result[repository.MutationData], error)
	WorktreeList(
		context.Context,
		repository.WorktreeListRequest,
	) (capability.Result[repository.WorktreeListData], error)
	WorktreeRepair(
		context.Context,
		repository.WorktreeRepairRequest,
	) (capability.Result[repository.WorktreeMutationData], error)
	WorktreePrune(
		context.Context,
		repository.WorktreePruneRequest,
	) (capability.Result[repository.WorktreeMutationData], error)
}

type RootOptions struct {
	Foundation    Foundation
	Organization  Organization
	Repository    Repository
	LoadLegacyApp func() error
}

func NewRoot(options RootOptions) *cobra.Command {
	root := cli.NewRootCmdWithOptions(cli.RootOptions{
		IncludeInit: false,
		IncludeOrg:  false,
	})
	root.AddCommand(newInitCommand(options.Foundation))
	root.AddCommand(newDoctorCommand(options.Foundation))
	root.AddCommand(newOrganizationCommand(options.Organization))
	root.AddCommand(newRepositoryCommand(options.Repository))

	legacyLoaded := false
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.Annotations[v3Annotation] == "true" {
			return nil
		}
		if legacyLoaded {
			return nil
		}
		if options.LoadLegacyApp == nil {
			return errors.New("legacy app loader is not configured")
		}
		if err := options.LoadLegacyApp(); err != nil {
			return fmt.Errorf("load legacy app: %w", err)
		}
		legacyLoaded = true
		return nil
	}

	return root
}

package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

// ListRequest selects the repositories to report. An empty Organization is not
// an omission to reject: it asks the broader question "what does this workspace
// hold?", which every other repo subcommand has no reason to ask but which a
// caller who does not yet know the organization names needs.
type ListRequest struct {
	WorkspaceRoot   string `json:"workspace_root"`
	Organization    string `json:"organization"`
	IncludeArchived bool   `json:"include_archived"`
}

// ListData carries the repositories found. OrganizationID is the scope that was
// asked for, so it is empty for a workspace-wide read rather than naming an
// arbitrary one of the organizations crossed; each row's own OrganizationID is
// what identifies where that repository lives.
type ListData struct {
	OrganizationID string `json:"organization_id"`
	Repositories   []Data `json:"repositories"`
}

type ShowRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
}

type ShowData struct {
	Repository Data `json:"repository"`
}

type managedRepository struct {
	inspection repositoryInspection
	projection controlplane.RepositoryProjection
	manifest   Manifest
	content    []byte
	data       Data
}

func (service *Service) List(
	ctx context.Context,
	request ListRequest,
) (capability.Result[ListData], error) {
	selectors := []string{request.Organization}
	if request.Organization == "" {
		// Workspace-wide. Enumerate the organizations from the derived
		// projection, then read each one through the ordinary scoped path, so a
		// broad list is exactly the union of the narrow ones — no second code
		// path that could drift from the containment and manifest checks
		// inspectOrganizationScope performs.
		discovered, err := workspaceOrganizationSelectors(
			ctx,
			request.WorkspaceRoot,
			request.IncludeArchived,
		)
		if err != nil {
			return emptyListResult(), err
		}
		selectors = discovered
	}

	repositories := make([]Data, 0)
	scopeID := ""
	for _, selector := range selectors {
		scope, err := inspectOrganizationScope(
			ctx,
			request.WorkspaceRoot,
			selector,
		)
		if err != nil {
			return emptyListResult(), err
		}
		if request.Organization != "" {
			scopeID = scope.organizationID
		}
		scoped, err := listScopedRepositories(
			ctx,
			scope,
			request.IncludeArchived,
		)
		if err != nil {
			return emptyListResult(), err
		}
		repositories = append(repositories, scoped...)
	}

	return capability.Result[ListData]{
		Capability: ListDescriptor.Capability,
		Version:    ListDescriptor.Version,
		Outcome:    capability.OutcomeHealthy,
		Data: ListData{
			OrganizationID: scopeID,
			Repositories:   repositories,
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

// listScopedRepositories reads one organization's repositories. It is the body
// the scoped and workspace-wide reads share.
func listScopedRepositories(
	ctx context.Context,
	scope organizationScope,
	includeArchived bool,
) ([]Data, error) {
	state, err := controlplane.OpenReadOnly(
		ctx,
		scope.workspaceLayout.StatePath(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = state.Close()
	}()
	projections, err := state.Repositories(ctx, scope.organizationID)
	if err != nil {
		return nil, err
	}
	repositories := make([]Data, 0, len(projections))
	for _, projection := range projections {
		if !includeArchived &&
			projection.Status == controlplane.EntityStatusArchived {
			continue
		}
		managed, err := readManagedProjection(scope, projection)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, managed.data)
	}
	return repositories, nil
}

// workspaceOrganizationSelectors returns every organization ID in the workspace,
// in the projection's deterministic slug order, so a workspace-wide list is
// reproducible. Archived organizations are omitted unless asked for: archive is
// a filter, and it has to apply to the organization tier as well as the
// repository tier or `--include-archived` would mean two different things
// depending on which tier did the archiving.
func workspaceOrganizationSelectors(
	ctx context.Context,
	root string,
	includeArchived bool,
) ([]string, error) {
	layout, err := workspace.NewLayout(root)
	if err != nil {
		return nil, err
	}
	state, err := controlplane.OpenReadOnly(ctx, layout.StatePath())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = state.Close()
	}()
	projections, err := state.Organizations(ctx)
	if err != nil {
		return nil, err
	}
	selectors := make([]string, 0, len(projections))
	for _, projection := range projections {
		if !includeArchived &&
			projection.Status == controlplane.EntityStatusArchived {
			continue
		}
		selectors = append(selectors, projection.ID)
	}
	return selectors, nil
}

func (service *Service) Show(
	ctx context.Context,
	request ShowRequest,
) (capability.Result[ShowData], error) {
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyShowResult(), err
	}
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Outcome:     capability.OutcomeHealthy,
		Data:        ShowData{Repository: managed.data},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func loadManagedRepository(
	ctx context.Context,
	root string,
	organizationSelector string,
	repositorySelector string,
) (managedRepository, error) {
	if repositorySelector == "" {
		return managedRepository{}, errors.New(
			"repository selector is required",
		)
	}
	scope, err := inspectOrganizationScope(ctx, root, organizationSelector)
	if err != nil {
		return managedRepository{}, err
	}
	state, err := controlplane.OpenReadOnly(
		ctx,
		scope.workspaceLayout.StatePath(),
	)
	if err != nil {
		return managedRepository{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	projection, err := state.Repository(
		ctx,
		scope.organizationID,
		repositorySelector,
	)
	if err != nil {
		return managedRepository{}, err
	}
	return readManagedProjection(scope, projection)
}

func readManagedProjection(
	scope organizationScope,
	projection controlplane.RepositoryProjection,
) (managedRepository, error) {
	layout, err := NewLayout(
		scope.layout,
		scope.manifest.Roles.Repositories,
		projection.Host,
		projection.Owner,
		projection.Name,
	)
	if err != nil {
		return managedRepository{}, err
	}
	if layout.Root() != projection.Path {
		return managedRepository{}, fmt.Errorf(
			"repository projection path %q does not match %q",
			projection.Path,
			layout.Root(),
		)
	}
	content, err := readRepositoryTargetWithin(
		scope.workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err != nil {
		return managedRepository{}, err
	}
	manifest, err := DecodeManifest(bytes.NewReader(content))
	if err != nil {
		return managedRepository{}, err
	}
	if err := manifest.Validate(layout); err != nil {
		return managedRepository{}, err
	}
	inspection := repositoryInspection{
		scope:      scope,
		layout:     layout,
		remoteName: manifest.CanonicalRemote.Name,
	}
	inspection.remoteURL, err = NormalizeRemoteURL(
		manifest.CanonicalRemote.FetchURL,
	)
	if err != nil {
		return managedRepository{}, err
	}
	return managedRepository{
		inspection: inspection,
		projection: projection,
		manifest:   manifest,
		content:    content,
		data:       repositoryData(scope, layout, manifest),
	}, nil
}

func emptyListResult() capability.Result[ListData] {
	return capability.Result[ListData]{
		Capability:  ListDescriptor.Capability,
		Version:     ListDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func emptyShowResult() capability.Result[ShowData] {
	return capability.Result[ShowData]{
		Capability:  ShowDescriptor.Capability,
		Version:     ShowDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

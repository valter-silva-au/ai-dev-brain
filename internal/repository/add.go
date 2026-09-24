package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

type AddMode string

const (
	AddModeCloneNew        AddMode = "clone-new"
	AddModeInitializeLocal AddMode = "initialize-local"
)

type AddRequest struct {
	WorkspaceRoot string  `json:"workspace_root"`
	Organization  string  `json:"organization"`
	Mode          AddMode `json:"mode"`
	Host          string  `json:"host,omitempty"`
	Owner         string  `json:"owner,omitempty"`
	Name          string  `json:"name,omitempty"`
	DisplayName   string  `json:"display_name,omitempty"`
	Remote        string  `json:"remote"`
	RemoteName    string  `json:"remote_name,omitempty"`
	ActorType     string  `json:"actor_type,omitempty"`
	ActorID       string  `json:"actor_id,omitempty"`
	Tool          string  `json:"tool,omitempty"`
	Apply         bool    `json:"apply"`
}

func (service *Service) Add(
	ctx context.Context,
	request AddRequest,
) (capability.Result[MutationData], error) {
	if err := validateAddRequest(request); err != nil {
		return emptyResult(AddDescriptor), err
	}
	inspection, conflict, err := service.inspectAdd(ctx, request)
	if err != nil {
		return emptyResult(AddDescriptor), err
	}
	previewID := service.newID()
	if previewID == "" {
		return emptyResult(AddDescriptor), errors.New(
			"repository id generator returned an empty id",
		)
	}
	manifest := service.addManifest(
		request,
		inspection,
		previewID,
		"planned:"+previewID,
	)
	data := MutationData{
		Repository: repositoryData(
			inspection.scope,
			inspection.layout,
			manifest,
		),
	}
	if conflict != "" {
		return conflictResult(AddDescriptor, data, conflict), nil
	}
	if inspection.existing != nil {
		data.Repository = repositoryData(
			inspection.scope,
			inspection.layout,
			*inspection.existing,
		)
		return result(
			AddDescriptor,
			capability.OutcomeUnchanged,
			data,
			repositoryEffects(
				inspection,
				string(request.Mode),
				capability.EffectSkipped,
			),
		), nil
	}
	if !request.Apply {
		return result(
			AddDescriptor,
			capability.OutcomePlanned,
			data,
			repositoryEffects(
				inspection,
				string(request.Mode),
				capability.EffectPlanned,
			),
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.scope.workspaceLayout.Root(),
		filepath.Join(
			inspection.scope.workspaceLayout.ControlDir(),
			"workspace.lock",
		),
	)
	if err != nil {
		return emptyResult(AddDescriptor), err
	}
	defer unlock()

	inspection, conflict, err = service.inspectAdd(ctx, request)
	if err != nil {
		return emptyResult(AddDescriptor), err
	}
	if conflict != "" {
		return conflictResult(AddDescriptor, data, conflict), nil
	}
	if inspection.existing != nil {
		data.Repository = repositoryData(
			inspection.scope,
			inspection.layout,
			*inspection.existing,
		)
		return result(
			AddDescriptor,
			capability.OutcomeUnchanged,
			data,
			repositoryEffects(
				inspection,
				string(request.Mode),
				capability.EffectSkipped,
			),
		), nil
	}

	repositoryID := service.newID()
	operationID := service.newID()
	if repositoryID == "" || operationID == "" {
		return emptyResult(AddDescriptor), errors.New(
			"repository id generator returned an empty id",
		)
	}
	manifest = service.addManifest(
		request,
		inspection,
		repositoryID,
		operationID,
	)
	if err := manifest.Validate(inspection.layout); err != nil {
		return emptyResult(AddDescriptor), err
	}
	manifestContent, err := encodeManifest(manifest)
	if err != nil {
		return emptyResult(AddDescriptor), err
	}
	agentsContent, err := agentsPointer(inspection.layout)
	if err != nil {
		return emptyResult(AddDescriptor), err
	}
	data = MutationData{
		Repository: repositoryData(
			inspection.scope,
			inspection.layout,
			manifest,
		),
		OperationID: operationID,
	}
	effects := repositoryEffects(
		inspection,
		string(request.Mode),
		capability.EffectPlanned,
	)

	store, err := journal.NewStore(
		inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return result(AddDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: AddDescriptor.Capability + ":" + repositoryID,
		Kind:           AddDescriptor.Capability,
		Steps: []journal.Step{
			{
				Ordinal: 1,
				Action:  string(request.Mode),
				Target:  inspection.layout.CloneDir(),
			},
			{
				Ordinal:   2,
				Action:    "create",
				Target:    inspection.layout.ConfigPath(),
				AfterHash: journal.Digest([]byte(defaultConfig)),
			},
			{
				Ordinal:   3,
				Action:    "create",
				Target:    inspection.layout.AgentsPath(),
				AfterHash: journal.Digest([]byte(agentsContent)),
			},
			{
				Ordinal:   4,
				Action:    "create",
				Target:    inspection.layout.ManifestPath(),
				AfterHash: journal.Digest(manifestContent),
			},
		},
	}); err != nil {
		return result(AddDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return result(AddDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return result(
				AddDescriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := inspection.scope.inspectRepositoriesRole(); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if err := atomicfile.MkdirAllWithin(
		inspection.scope.workspaceLayout.Root(),
		inspection.layout.Root(),
		0o755,
	); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if err := inspectRepositoryContainer(inspection, false); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	switch request.Mode {
	case AddModeCloneNew:
		if err = inspectRepositoryClone(inspection, true); err != nil {
			break
		}
		err = service.git.Clone(
			ctx,
			inspection.layout.Root(),
			inspection.remoteURL.Normalized,
			inspection.layout.CloneDir(),
		)
		if err == nil {
			err = inspectRepositoryClone(inspection, false)
		}
	case AddModeInitializeLocal:
		if err = atomicfile.MkdirAllWithin(
			inspection.scope.workspaceLayout.Root(),
			inspection.layout.CloneDir(),
			0o755,
		); err == nil {
			err = inspectRepositoryClone(inspection, false)
		}
		if err == nil {
			err = service.git.Init(ctx, inspection.layout.CloneDir())
		}
		if err == nil {
			err = inspectRepositoryClone(inspection, false)
		}
		if err == nil {
			err = service.git.AddRemote(
				ctx,
				inspection.layout.CloneDir(),
				inspection.remoteName,
				inspection.remoteURL.Normalized,
			)
		}
	}
	if err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if err := service.verifyCanonicalClone(ctx, inspection); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if err := atomicfile.MkdirAllWithin(
		inspection.scope.workspaceLayout.Root(),
		inspection.layout.ControlDir(),
		0o755,
	); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	for _, file := range []struct {
		step    int
		path    string
		content []byte
	}{
		{2, inspection.layout.ConfigPath(), []byte(defaultConfig)},
		{3, inspection.layout.AgentsPath(), []byte(agentsContent)},
		{4, inspection.layout.ManifestPath(), manifestContent},
	} {
		if err := applyFile(
			inspection.scope.workspaceLayout.Root(),
			store,
			operationID,
			file.step,
			file.path,
			file.content,
		); err != nil {
			return service.failAdd(store, data, effects, err)
		}
	}
	if err := service.projectRepository(
		ctx,
		inspection,
		manifest,
		manifestContent,
	); err != nil {
		return service.failAdd(store, data, effects, err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return result(AddDescriptor, capability.OutcomeFailed, data, effects), err
	}
	return result(
		AddDescriptor,
		capability.OutcomeApplied,
		data,
		repositoryEffects(
			inspection,
			string(request.Mode),
			capability.EffectApplied,
		),
	), nil
}

func (service *Service) inspectAdd(
	ctx context.Context,
	request AddRequest,
) (repositoryInspection, string, error) {
	scope, err := inspectOrganizationScope(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
	)
	if err != nil {
		return repositoryInspection{}, "", err
	}
	remoteURL, err := NormalizeRemoteURL(request.Remote)
	if err != nil {
		return repositoryInspection{}, "", err
	}
	for _, check := range []struct {
		name      string
		requested string
		actual    string
	}{
		{"host", request.Host, remoteURL.Host},
		{"owner", request.Owner, remoteURL.Owner},
		{"name", request.Name, remoteURL.Repository},
	} {
		if check.requested != "" && check.requested != check.actual {
			layout, layoutErr := NewLayout(
				scope.layout,
				scope.manifest.Roles.Repositories,
				valueOr(check.name == "host", request.Host, remoteURL.Host),
				valueOr(check.name == "owner", request.Owner, remoteURL.Owner),
				valueOr(check.name == "name", request.Name, remoteURL.Repository),
			)
			if layoutErr != nil {
				return repositoryInspection{}, "", layoutErr
			}
			return repositoryInspection{
					scope:      scope,
					layout:     layout,
					remoteURL:  remoteURL,
					remoteName: canonicalRemoteName(request.RemoteName),
				}, fmt.Sprintf(
					"repository remote %s is %q, requested %q",
					check.name,
					check.actual,
					check.requested,
				), nil
		}
	}
	inspection, err := inspectRepositoryTarget(
		ctx,
		scope,
		remoteURL,
		canonicalRemoteName(request.RemoteName),
	)
	if err != nil {
		return repositoryInspection{}, "", err
	}
	if inspection.conflict != "" {
		return inspection, inspection.conflict, nil
	}
	if inspection.existing != nil && !existingMatches(
		inspection.existing,
		remoteURL,
		false,
		inspection.existing.Roles.Clone,
	) {
		return inspection, "existing repository metadata differs from the request", nil
	}
	return inspection, "", nil
}

func (service *Service) addManifest(
	request AddRequest,
	inspection repositoryInspection,
	repositoryID string,
	operationID string,
) Manifest {
	manifest := NewManifest(
		repositoryID,
		inspection.scope.organizationID,
		inspection.layout,
		Remote{
			Name:     inspection.remoteName,
			Type:     RemoteTypeCanonical,
			FetchURL: inspection.remoteURL.Normalized,
		},
		service.now(),
		mutationProvenance(
			operationID,
			request.ActorType,
			request.ActorID,
			request.Tool,
		),
	)
	if request.DisplayName != "" {
		manifest.DisplayName = request.DisplayName
	}
	return manifest
}

func (service *Service) verifyCanonicalClone(
	ctx context.Context,
	inspection repositoryInspection,
) error {
	if err := inspectRepositoryClone(inspection, false); err != nil {
		return err
	}
	inventory, err := service.git.Inventory(
		ctx,
		inspection.layout.CloneDir(),
		inspection.remoteName,
	)
	if err != nil {
		return err
	}
	if !inventory.IsRepository {
		return errors.New("canonical clone is not a git repository")
	}
	remote, ok := findRemote(inventory, inspection.remoteName)
	if !ok {
		return fmt.Errorf(
			"canonical clone is missing remote %q",
			inspection.remoteName,
		)
	}
	normalized, err := NormalizeRemoteURL(remote.FetchURL)
	if err != nil {
		return err
	}
	if normalized.Normalized != inspection.remoteURL.Normalized {
		return fmt.Errorf(
			"canonical remote is %q, want %q",
			normalized.Normalized,
			inspection.remoteURL.Normalized,
		)
	}
	return nil
}

func (service *Service) projectRepository(
	ctx context.Context,
	inspection repositoryInspection,
	manifest Manifest,
	content []byte,
) error {
	state, err := controlplane.Open(
		ctx,
		inspection.scope.workspaceLayout.StatePath(),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = state.Close()
	}()
	return state.ObserveRepository(
		ctx,
		repositoryProjection(
			inspection.layout,
			manifest,
			content,
			service.now(),
		),
	)
}

func (service *Service) failAdd(
	store *journal.Store,
	data MutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[MutationData], error) {
	if _, appendErr := store.Append(
		data.OperationID,
		journal.EventInput{
			Phase: journal.PhaseFailed,
			Error: &journal.ErrorInfo{
				Code:    "repository_add_failed",
				Message: cause.Error(),
			},
		},
	); appendErr != nil {
		// Losing this append is not cosmetic: with no failed phase on record the
		// journal folds to applying, so `adb doctor` reports
		// operation.journal.incomplete ("resume only when its target state is
		// unambiguous") for an operation that actually failed and should not be
		// resumed. The returned error is the only channel this helper has, so the
		// cause stays primary and the append failure rides alongside it.
		cause = fmt.Errorf(
			"%w (record repository add failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return result(AddDescriptor, capability.OutcomeFailed, data, effects), cause
}

func validateAddRequest(request AddRequest) error {
	switch request.Mode {
	case AddModeCloneNew, AddModeInitializeLocal:
	default:
		return fmt.Errorf("unsupported repository add mode %q", request.Mode)
	}
	if request.Remote == "" {
		return errors.New("repository canonical remote is required")
	}
	return validateActor(request.ActorType, request.Tool)
}

func canonicalRemoteName(value string) string {
	if value == "" {
		return "origin"
	}
	return value
}

func valueOr(useRequested bool, requested string, fallback string) string {
	if useRequested && requested != "" {
		return requested
	}
	return fallback
}

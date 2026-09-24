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
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type MoveRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	NewRemote     string `json:"new_remote"`
	UpdateRemote  bool   `json:"update_remote"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type moveInspection struct {
	managed   managedRepository
	layout    Layout
	remoteURL RemoteURL
	manifest  Manifest
	conflict  string
	unchanged bool
}

func (service *Service) Move(
	ctx context.Context,
	request MoveRequest,
) (capability.Result[MutationData], error) {
	if request.NewRemote == "" {
		return emptyResult(MoveDescriptor), errors.New(
			"repository new remote is required",
		)
	}
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyResult(MoveDescriptor), err
	}
	inspection, err := service.inspectMove(ctx, request)
	if err != nil {
		return emptyResult(MoveDescriptor), err
	}
	data := MutationData{
		Repository: repositoryData(
			inspection.managed.inspection.scope,
			inspection.layout,
			inspection.manifest,
		),
	}
	if inspection.conflict != "" {
		return conflictResult(
			MoveDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	if inspection.unchanged {
		return result(
			MoveDescriptor,
			capability.OutcomeUnchanged,
			data,
			moveEffects(inspection, capability.EffectSkipped),
		), nil
	}
	if !request.Apply {
		return result(
			MoveDescriptor,
			capability.OutcomePlanned,
			data,
			moveEffects(inspection, capability.EffectPlanned),
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.managed.inspection.scope.workspaceLayout.Root(),
		filepath.Join(
			inspection.managed.inspection.scope.workspaceLayout.ControlDir(),
			"workspace.lock",
		),
	)
	if err != nil {
		return emptyResult(MoveDescriptor), err
	}
	defer unlock()
	inspection, err = service.inspectMove(ctx, request)
	if err != nil {
		return emptyResult(MoveDescriptor), err
	}
	data.Repository = repositoryData(
		inspection.managed.inspection.scope,
		inspection.layout,
		inspection.manifest,
	)
	if inspection.conflict != "" {
		return conflictResult(
			MoveDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	if inspection.unchanged {
		return result(
			MoveDescriptor,
			capability.OutcomeUnchanged,
			data,
			moveEffects(inspection, capability.EffectSkipped),
		), nil
	}

	operationID := service.newID()
	if operationID == "" {
		return emptyResult(MoveDescriptor), errors.New(
			"repository operation id generator returned an empty id",
		)
	}
	now := service.now().UTC()
	inspection.manifest.UpdatedAt = now
	inspection.manifest.LastMutation = mutationProvenance(
		operationID,
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if err := inspection.manifest.Validate(inspection.layout); err != nil {
		return emptyResult(MoveDescriptor), err
	}
	content, err := encodeManifest(inspection.manifest)
	if err != nil {
		return emptyResult(MoveDescriptor), err
	}
	data = MutationData{
		Repository: repositoryData(
			inspection.managed.inspection.scope,
			inspection.layout,
			inspection.manifest,
		),
		OperationID: operationID,
	}
	effects := moveEffects(inspection, capability.EffectPlanned)
	store, err := journal.NewStore(
		inspection.managed.inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return result(MoveDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: MoveDescriptor.Capability + ":" + inspection.manifest.ID,
		Kind:           MoveDescriptor.Capability,
		Steps: []journal.Step{
			{
				Ordinal: 1,
				Action:  "set-remote",
				Target:  inspection.managed.data.ClonePath,
			},
			{
				Ordinal:    2,
				Action:     "move",
				Target:     inspection.managed.inspection.layout.ManifestPath(),
				BeforeHash: journal.Digest(inspection.managed.content),
			},
			{
				Ordinal:    3,
				Action:     "replace",
				Target:     inspection.layout.ManifestPath(),
				BeforeHash: journal.Digest(inspection.managed.content),
				AfterHash:  journal.Digest(content),
			},
		},
	}); err != nil {
		return result(MoveDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return result(MoveDescriptor, capability.OutcomeFailed, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return result(
				MoveDescriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := validateMoveDestination(inspection); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := validateMoveDestination(inspection); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := validateMoveSource(inspection); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := service.git.SetRemoteURL(
		ctx,
		inspection.managed.data.ClonePath,
		inspection.manifest.CanonicalRemote.Name,
		inspection.remoteURL.Normalized,
	); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  2,
	}); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := validateMoveDestination(inspection); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := validateMoveSource(inspection); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := atomicfile.MkdirAllWithin(
		inspection.managed.inspection.scope.workspaceLayout.Root(),
		filepath.Dir(inspection.layout.Root()),
		0o755,
	); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := atomicfile.RenameWithin(
		inspection.managed.inspection.scope.workspaceLayout.Root(),
		inspection.managed.inspection.layout.Root(),
		inspection.layout.Root(),
	); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			fmt.Errorf("move repository container: %w", err),
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  2,
	}); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := replaceFile(
		inspection.managed.inspection.scope.workspaceLayout.Root(),
		store,
		operationID,
		3,
		inspection.layout.ManifestPath(),
		inspection.managed.content,
		content,
	); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := service.projectRepository(
		ctx,
		repositoryInspection{
			scope:      inspection.managed.inspection.scope,
			layout:     inspection.layout,
			remoteURL:  inspection.remoteURL,
			remoteName: inspection.manifest.CanonicalRemote.Name,
		},
		inspection.manifest,
		content,
	); err != nil {
		return service.failGitOperation(
			store,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return result(MoveDescriptor, capability.OutcomeFailed, data, effects), err
	}
	return result(
		MoveDescriptor,
		capability.OutcomeApplied,
		data,
		moveEffects(inspection, capability.EffectApplied),
	), nil
}

func (service *Service) inspectMove(
	ctx context.Context,
	request MoveRequest,
) (moveInspection, error) {
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return moveInspection{}, err
	}
	inspection := moveInspection{
		managed:  managed,
		layout:   managed.inspection.layout,
		manifest: managed.manifest,
	}
	if managed.manifest.Status == StatusArchived {
		inspection.conflict = "archived repositories cannot be moved"
		return inspection, nil
	}
	remoteURL, err := NormalizeRemoteURL(request.NewRemote)
	if err != nil {
		return moveInspection{}, err
	}
	inspection.remoteURL = remoteURL
	if remoteURL.Normalized == managed.manifest.CanonicalRemote.FetchURL {
		inspection.unchanged = true
		return inspection, nil
	}
	if !request.UpdateRemote {
		inspection.conflict = "repository move requires explicit remote update authority"
		return inspection, nil
	}
	layout, err := NewLayout(
		managed.inspection.scope.layout,
		managed.inspection.scope.manifest.Roles.Repositories,
		remoteURL.Host,
		remoteURL.Owner,
		remoteURL.Repository,
	)
	if err != nil {
		return moveInspection{}, err
	}
	inspection.layout = layout
	destination, err := workspace.InspectPath(
		managed.inspection.scope.workspaceLayout.Root(),
		layout.Root(),
	)
	if err != nil {
		return moveInspection{}, err
	}
	if destination.State == workspace.RoleInspectionContained {
		inspection.conflict = fmt.Sprintf(
			"repository destination %q already exists",
			layout.Root(),
		)
		return inspection, nil
	}
	state, err := controlplane.OpenReadOnly(
		ctx,
		managed.inspection.scope.workspaceLayout.StatePath(),
	)
	if err != nil {
		return moveInspection{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	for _, selector := range []string{layout.Key(), layout.Root()} {
		projection, err := state.Repository(
			ctx,
			managed.manifest.OrganizationID,
			selector,
		)
		if err == nil && projection.ID != managed.manifest.ID {
			inspection.conflict = fmt.Sprintf(
				"repository selector %q belongs to %q",
				selector,
				projection.ID,
			)
			return inspection, nil
		}
	}
	inspection.manifest.Host = remoteURL.Host
	inspection.manifest.Owner = remoteURL.Owner
	inspection.manifest.Name = remoteURL.Repository
	inspection.manifest.CanonicalRemote.FetchURL = remoteURL.Normalized
	if inspection.manifest.CanonicalRemote.PushURL != "" {
		inspection.manifest.CanonicalRemote.PushURL = remoteURL.Normalized
	}
	inspection.manifest.Aliases = appendAliases(
		managed.manifest.Aliases,
		layout.Key(),
		layout.Root(),
		managed.inspection.layout.Key(),
		managed.inspection.layout.Root(),
	)
	if err := inspection.manifest.Validate(layout); err != nil {
		return moveInspection{}, err
	}
	return inspection, nil
}

func validateMoveDestination(inspection moveInspection) error {
	if err := inspection.managed.inspection.scope.inspectRepositoriesRole(); err != nil {
		return err
	}
	target, err := workspace.InspectPath(
		inspection.managed.inspection.scope.workspaceLayout.Root(),
		inspection.layout.Root(),
	)
	if err != nil {
		return err
	}
	if target.State == workspace.RoleInspectionContained {
		return fmt.Errorf(
			"repository destination %q already exists",
			inspection.layout.Root(),
		)
	}
	return nil
}

func validateMoveSource(inspection moveInspection) error {
	if err := inspectRepositoryContainer(
		inspection.managed.inspection,
		false,
	); err != nil {
		return err
	}
	return inspectCanonicalClone(inspection.managed, false)
}

func moveEffects(
	inspection moveInspection,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{
			Action: "set-remote",
			Target: inspection.managed.data.ClonePath,
			Status: status,
		},
		{
			Action: "move",
			Target: inspection.layout.Root(),
			Status: status,
		},
		{
			Action: "replace",
			Target: inspection.layout.ManifestPath(),
			Status: status,
		},
		{
			Action: "project",
			Target: inspection.managed.inspection.scope.workspaceLayout.StatePath(),
			Status: status,
		},
	}
}

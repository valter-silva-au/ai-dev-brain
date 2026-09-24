package repository

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

type ArchiveRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	Restore       bool   `json:"restore"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

func (service *Service) Archive(
	ctx context.Context,
	request ArchiveRequest,
) (capability.Result[MutationData], error) {
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	proposed := managed.manifest
	targetStatus := StatusArchived
	if request.Restore {
		targetStatus = StatusActive
	}
	data := MutationData{Repository: managed.data}
	if proposed.Status == targetStatus {
		return result(
			ArchiveDescriptor,
			capability.OutcomeUnchanged,
			data,
			manifestEffects(managed, capability.EffectSkipped),
		), nil
	}
	proposed.Status = targetStatus
	if request.Restore {
		proposed.ArchivedAt = nil
	} else {
		archivedAt := service.now().UTC()
		proposed.ArchivedAt = &archivedAt
	}
	data.Repository = repositoryData(
		managed.inspection.scope,
		managed.inspection.layout,
		proposed,
	)
	if !request.Apply {
		return result(
			ArchiveDescriptor,
			capability.OutcomePlanned,
			data,
			manifestEffects(managed, capability.EffectPlanned),
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		managed.inspection.scope.workspaceLayout.Root(),
		filepath.Join(
			managed.inspection.scope.workspaceLayout.ControlDir(),
			"workspace.lock",
		),
	)
	if err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	defer unlock()
	managed, err = loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	if managed.manifest.Status == targetStatus {
		return result(
			ArchiveDescriptor,
			capability.OutcomeUnchanged,
			MutationData{Repository: managed.data},
			manifestEffects(managed, capability.EffectSkipped),
		), nil
	}
	operationID := service.newID()
	if operationID == "" {
		return emptyResult(ArchiveDescriptor), errors.New(
			"repository operation id generator returned an empty id",
		)
	}
	proposed = managed.manifest
	proposed.Status = targetStatus
	now := service.now().UTC()
	proposed.UpdatedAt = now
	proposed.LastMutation = mutationProvenance(
		operationID,
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if request.Restore {
		proposed.ArchivedAt = nil
	} else {
		proposed.ArchivedAt = &now
	}
	if err := proposed.Validate(managed.inspection.layout); err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	content, err := encodeManifest(proposed)
	if err != nil {
		return emptyResult(ArchiveDescriptor), err
	}
	data = MutationData{
		Repository: repositoryData(
			managed.inspection.scope,
			managed.inspection.layout,
			proposed,
		),
		OperationID: operationID,
	}
	return service.applyManifestChange(
		ctx,
		ArchiveDescriptor,
		managed,
		managed.inspection.layout,
		proposed,
		content,
		data,
	)
}

func (service *Service) applyManifestChange(
	ctx context.Context,
	descriptor capability.Descriptor,
	managed managedRepository,
	layout Layout,
	manifest Manifest,
	content []byte,
	data MutationData,
) (capability.Result[MutationData], error) {
	effects := manifestEffects(managed, capability.EffectPlanned)
	store, err := journal.NewStore(
		managed.inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    data.OperationID,
		IdempotencyKey: descriptor.Capability + ":" + manifest.ID,
		Kind:           descriptor.Capability,
		Steps: []journal.Step{{
			Ordinal:    1,
			Action:     "replace",
			Target:     layout.ManifestPath(),
			BeforeHash: journal.Digest(managed.content),
			AfterHash:  journal.Digest(content),
		}},
	}); err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return result(
				descriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := managed.inspection.scope.inspectRepositoriesRole(); err != nil {
		return service.failGitOperation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if err := replaceFile(
		managed.inspection.scope.workspaceLayout.Root(),
		store,
		data.OperationID,
		1,
		layout.ManifestPath(),
		managed.content,
		content,
	); err != nil {
		return service.failGitOperation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if err := service.projectRepository(
		ctx,
		repositoryInspection{
			scope:      managed.inspection.scope,
			layout:     layout,
			remoteURL:  managed.inspection.remoteURL,
			remoteName: manifest.CanonicalRemote.Name,
		},
		manifest,
		content,
	); err != nil {
		return service.failGitOperation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	applied := manifestEffects(managed, capability.EffectApplied)
	applied[0].Target = layout.ManifestPath()
	return result(
		descriptor,
		capability.OutcomeApplied,
		data,
		applied,
	), nil
}

func manifestEffects(
	managed managedRepository,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{
			Action: "replace",
			Target: managed.inspection.layout.ManifestPath(),
			Status: status,
		},
		{
			Action: "project",
			Target: managed.inspection.scope.workspaceLayout.StatePath(),
			Status: status,
		},
	}
}

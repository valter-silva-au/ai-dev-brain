package organization

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

type ArchiveRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
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
	if request.Selector == "" {
		return emptyMutationResult(ArchiveDescriptor), errors.New(
			"organization selector is required",
		)
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}

	current, err := loadManagedOrganization(
		ctx,
		request.WorkspaceRoot,
		request.Selector,
	)
	if err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	proposed := archiveManifest(current.manifest, request.Restore, service.now())
	data := MutationData{
		Organization: organizationData(
			current.workspaceID,
			current.layout,
			proposed,
		),
	}
	if proposed.Status == current.manifest.Status {
		return mutationResult(
			ArchiveDescriptor,
			capability.OutcomeUnchanged,
			data,
			mutationEffects(
				current.workspaceLayout,
				current.layout.ManifestPath(),
				capability.EffectSkipped,
			),
		), nil
	}
	if !request.Apply {
		return mutationResult(
			ArchiveDescriptor,
			capability.OutcomePlanned,
			data,
			mutationEffects(
				current.workspaceLayout,
				current.layout.ManifestPath(),
				capability.EffectPlanned,
			),
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		current.workspaceLayout.Root(),
		filepath.Join(current.workspaceLayout.ControlDir(), "workspace.lock"),
	)
	if err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	defer unlock()

	current, err = loadManagedOrganization(
		ctx,
		request.WorkspaceRoot,
		request.Selector,
	)
	if err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	proposed = archiveManifest(current.manifest, request.Restore, service.now())
	data.Organization = organizationData(
		current.workspaceID,
		current.layout,
		proposed,
	)
	if proposed.Status == current.manifest.Status {
		return mutationResult(
			ArchiveDescriptor,
			capability.OutcomeUnchanged,
			data,
			mutationEffects(
				current.workspaceLayout,
				current.layout.ManifestPath(),
				capability.EffectSkipped,
			),
		), nil
	}

	operationID := service.newID()
	if operationID == "" {
		return emptyMutationResult(ArchiveDescriptor), errors.New(
			"organization operation id generator returned an empty id",
		)
	}
	now := service.now().UTC()
	proposed.UpdatedAt = now
	proposed.LastMutation = mutationProvenance(
		operationID,
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if proposed.Status == StatusArchived {
		proposed.ArchivedAt = &now
	}
	if err := proposed.Validate(current.layout); err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	content, err := encodeManifest(proposed)
	if err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	return service.commitManifestMutation(
		ctx,
		ArchiveDescriptor,
		current,
		current.layout,
		proposed,
		content,
		operationID,
		"",
	)
}

func archiveManifest(
	manifest Manifest,
	restore bool,
	now time.Time,
) Manifest {
	if restore {
		manifest.Status = StatusActive
		manifest.ArchivedAt = nil
		return manifest
	}
	manifest.Status = StatusArchived
	archivedAt := now.UTC()
	manifest.ArchivedAt = &archivedAt
	return manifest
}

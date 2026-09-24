package organization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
)

type UpdateRequest struct {
	WorkspaceRoot string  `json:"workspace_root"`
	Selector      string  `json:"selector"`
	Name          *string `json:"name,omitempty"`
	Parent        *string `json:"parent,omitempty"`
	Owner         *string `json:"owner,omitempty"`
	Description   *string `json:"description,omitempty"`
	Trust         *string `json:"trust,omitempty"`
	Profile       *string `json:"profile,omitempty"`
	ActorType     string  `json:"actor_type,omitempty"`
	ActorID       string  `json:"actor_id,omitempty"`
	Tool          string  `json:"tool,omitempty"`
	Apply         bool    `json:"apply"`
}

type updateInspection struct {
	current   managedOrganization
	proposed  Manifest
	conflict  string
	unchanged bool
}

func (service *Service) Update(
	ctx context.Context,
	request UpdateRequest,
) (capability.Result[MutationData], error) {
	if err := validateUpdateRequest(request); err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}

	inspection, err := inspectUpdate(ctx, request)
	if err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	data := MutationData{
		Organization: organizationData(
			inspection.current.workspaceID,
			inspection.current.layout,
			inspection.proposed,
		),
	}
	if inspection.conflict != "" {
		return conflictMutationResult(
			UpdateDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	effects := mutationEffects(
		inspection.current.workspaceLayout,
		inspection.current.layout.ManifestPath(),
		capability.EffectPlanned,
	)
	if inspection.unchanged {
		return mutationResult(
			UpdateDescriptor,
			capability.OutcomeUnchanged,
			data,
			mutationEffects(
				inspection.current.workspaceLayout,
				inspection.current.layout.ManifestPath(),
				capability.EffectSkipped,
			),
		), nil
	}
	if !request.Apply {
		return mutationResult(
			UpdateDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.current.workspaceLayout.Root(),
		filepath.Join(
			inspection.current.workspaceLayout.ControlDir(),
			"workspace.lock",
		),
	)
	if err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	defer unlock()

	inspection, err = inspectUpdate(ctx, request)
	if err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	data.Organization = organizationData(
		inspection.current.workspaceID,
		inspection.current.layout,
		inspection.proposed,
	)
	if inspection.conflict != "" {
		return conflictMutationResult(
			UpdateDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	if inspection.unchanged {
		return mutationResult(
			UpdateDescriptor,
			capability.OutcomeUnchanged,
			data,
			mutationEffects(
				inspection.current.workspaceLayout,
				inspection.current.layout.ManifestPath(),
				capability.EffectSkipped,
			),
		), nil
	}

	operationID := service.newID()
	if operationID == "" {
		return emptyMutationResult(UpdateDescriptor), errors.New(
			"organization operation id generator returned an empty id",
		)
	}
	inspection.proposed.UpdatedAt = service.now().UTC()
	inspection.proposed.LastMutation = mutationProvenance(
		operationID,
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if err := inspection.proposed.Validate(inspection.current.layout); err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	content, err := encodeManifest(inspection.proposed)
	if err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	return service.commitManifestMutation(
		ctx,
		UpdateDescriptor,
		inspection.current,
		inspection.current.layout,
		inspection.proposed,
		content,
		operationID,
		"",
	)
}

func inspectUpdate(
	ctx context.Context,
	request UpdateRequest,
) (updateInspection, error) {
	current, err := loadManagedOrganization(
		ctx,
		request.WorkspaceRoot,
		request.Selector,
	)
	if err != nil {
		return updateInspection{}, err
	}
	inspection := updateInspection{
		current:  current,
		proposed: current.manifest,
	}
	if current.manifest.Status == StatusArchived {
		inspection.conflict = fmt.Sprintf(
			"organization %q is archived",
			current.manifest.Slug,
		)
		return inspection, nil
	}

	if request.Name != nil {
		inspection.proposed.DisplayName = *request.Name
	}
	if request.Owner != nil {
		inspection.proposed.Owner = *request.Owner
	}
	if request.Description != nil {
		inspection.proposed.Description = *request.Description
	}
	if request.Trust != nil {
		inspection.proposed.Trust = *request.Trust
	}
	if request.Profile != nil {
		inspection.proposed.Profile = *request.Profile
	}
	if request.Parent != nil {
		parentID, conflict, err := resolveUpdatedParent(
			ctx,
			current.workspaceLayout.StatePath(),
			current.manifest.ID,
			*request.Parent,
		)
		if err != nil {
			return updateInspection{}, err
		}
		if conflict != "" {
			inspection.conflict = conflict
			return inspection, nil
		}
		inspection.proposed.ParentID = parentID
	}
	if err := inspection.proposed.Validate(current.layout); err != nil {
		return updateInspection{}, err
	}
	inspection.unchanged = reflect.DeepEqual(
		inspection.proposed,
		current.manifest,
	)
	return inspection, nil
}

func resolveUpdatedParent(
	ctx context.Context,
	statePath string,
	organizationID string,
	selector string,
) (string, string, error) {
	if selector == "" {
		return "", "", nil
	}
	state, err := controlplane.OpenReadOnly(ctx, statePath)
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = state.Close()
	}()

	parent, err := state.Organization(ctx, selector)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Sprintf(
				"parent organization %q is not registered",
				selector,
			), nil
		}
		return "", "", err
	}
	if parent.Status == controlplane.EntityStatusArchived {
		return "", fmt.Sprintf(
			"parent organization %q is archived",
			selector,
		), nil
	}

	seen := make(map[string]struct{})
	cursor := parent
	for {
		if cursor.ID == organizationID {
			return "", "organization parent would create a cycle", nil
		}
		if cursor.ParentID == "" {
			break
		}
		if _, ok := seen[cursor.ID]; ok {
			return "", "existing organization parent graph contains a cycle", nil
		}
		seen[cursor.ID] = struct{}{}
		cursor, err = state.Organization(ctx, cursor.ParentID)
		if err != nil {
			return "", "", fmt.Errorf(
				"follow organization parent %q: %w",
				cursor.ParentID,
				err,
			)
		}
	}
	return parent.ID, "", nil
}

func validateUpdateRequest(request UpdateRequest) error {
	if request.Selector == "" {
		return errors.New("organization selector is required")
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return err
	}
	if request.Name == nil &&
		request.Parent == nil &&
		request.Owner == nil &&
		request.Description == nil &&
		request.Trust == nil &&
		request.Profile == nil {
		return errors.New("organization update requires at least one field")
	}
	return nil
}

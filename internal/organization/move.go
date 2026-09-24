package organization

import (
	"context"
	"database/sql"
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
	Selector      string `json:"selector"`
	NewSlug       string `json:"new_slug"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type moveInspection struct {
	current   managedOrganization
	layout    Layout
	manifest  Manifest
	conflict  string
	unchanged bool
}

func (service *Service) Move(
	ctx context.Context,
	request MoveRequest,
) (capability.Result[MutationData], error) {
	if err := validateMoveRequest(request); err != nil {
		return emptyMutationResult(MoveDescriptor), err
	}
	inspection, err := inspectMove(ctx, request)
	if err != nil {
		return emptyMutationResult(MoveDescriptor), err
	}
	data := MutationData{
		Organization: organizationData(
			inspection.current.workspaceID,
			inspection.layout,
			inspection.manifest,
		),
		PreviousPath: inspection.current.layout.Root(),
	}
	if inspection.conflict != "" {
		return conflictMutationResult(
			MoveDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	effects := moveEffects(
		inspection.current,
		inspection.layout,
		capability.EffectPlanned,
	)
	if inspection.unchanged {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeUnchanged,
			data,
			moveEffects(
				inspection.current,
				inspection.layout,
				capability.EffectSkipped,
			),
		), nil
	}
	if !request.Apply {
		return mutationResult(
			MoveDescriptor,
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
		return emptyMutationResult(MoveDescriptor), err
	}
	defer unlock()

	inspection, err = inspectMove(ctx, request)
	if err != nil {
		return emptyMutationResult(MoveDescriptor), err
	}
	data = MutationData{
		Organization: organizationData(
			inspection.current.workspaceID,
			inspection.layout,
			inspection.manifest,
		),
		PreviousPath: inspection.current.layout.Root(),
	}
	if inspection.conflict != "" {
		return conflictMutationResult(
			MoveDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	if inspection.unchanged {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeUnchanged,
			data,
			moveEffects(
				inspection.current,
				inspection.layout,
				capability.EffectSkipped,
			),
		), nil
	}

	operationID := service.newID()
	if operationID == "" {
		return emptyMutationResult(MoveDescriptor), errors.New(
			"organization operation id generator returned an empty id",
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
		return emptyMutationResult(MoveDescriptor), err
	}
	content, err := encodeManifest(inspection.manifest)
	if err != nil {
		return emptyMutationResult(MoveDescriptor), err
	}
	data = MutationData{
		Organization: organizationData(
			inspection.current.workspaceID,
			inspection.layout,
			inspection.manifest,
		),
		OperationID:  operationID,
		PreviousPath: inspection.current.layout.Root(),
	}
	effects = moveEffects(
		inspection.current,
		inspection.layout,
		capability.EffectPlanned,
	)

	journalStore, err := journal.NewStore(
		inspection.current.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := journalStore.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: MoveDescriptor.Capability + ":" + inspection.manifest.ID,
		Kind:           MoveDescriptor.Capability,
		Steps: []journal.Step{
			{
				Ordinal:    1,
				Action:     "move",
				Target:     inspection.current.layout.ManifestPath(),
				BeforeHash: journal.Digest(inspection.current.manifestContent),
			},
			{
				Ordinal:    2,
				Action:     "replace",
				Target:     inspection.layout.ManifestPath(),
				BeforeHash: journal.Digest(inspection.current.manifestContent),
				AfterHash:  journal.Digest(content),
			},
		},
	}); err != nil {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return mutationResult(
				MoveDescriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}

	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := atomicfile.RenameWithin(
		inspection.current.workspaceLayout.Root(),
		inspection.current.layout.Root(),
		inspection.layout.Root(),
	); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			fmt.Errorf("move organization directory: %w", err),
		)
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := applyJournaledReplacement(
		inspection.current.workspaceLayout.Root(),
		journalStore,
		operationID,
		2,
		inspection.layout.ManifestPath(),
		inspection.current.manifestContent,
		content,
	); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}

	state, err := controlplane.Open(
		ctx,
		inspection.current.workspaceLayout.StatePath(),
	)
	if err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	stateClosed := false
	defer func() {
		if !stateClosed {
			_ = state.Close()
		}
	}()
	if err := state.ObserveOrganization(
		ctx,
		organizationProjection(
			inspection.layout,
			inspection.manifest,
			content,
			service.now(),
		),
	); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			journalStore,
			MoveDescriptor,
			data,
			effects,
			err,
		)
	}
	stateClosed = true
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return mutationResult(
			MoveDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	return mutationResult(
		MoveDescriptor,
		capability.OutcomeApplied,
		data,
		moveEffects(
			inspection.current,
			inspection.layout,
			capability.EffectApplied,
		),
	), nil
}

func inspectMove(
	ctx context.Context,
	request MoveRequest,
) (moveInspection, error) {
	current, err := loadManagedOrganization(
		ctx,
		request.WorkspaceRoot,
		request.Selector,
	)
	if err != nil {
		return moveInspection{}, err
	}
	inspection := moveInspection{
		current:  current,
		layout:   current.layout,
		manifest: current.manifest,
	}
	if current.manifest.Status == StatusArchived {
		inspection.conflict = fmt.Sprintf(
			"organization %q is archived",
			current.manifest.Slug,
		)
		return inspection, nil
	}
	if request.NewSlug == current.manifest.Slug {
		inspection.unchanged = true
		return inspection, nil
	}

	layout, err := NewLayoutForRole(
		current.workspaceLayout.Root(),
		current.organizationsRole,
		request.NewSlug,
	)
	if err != nil {
		return moveInspection{}, err
	}
	inspection.layout = layout
	destination, err := workspace.InspectPath(
		current.workspaceLayout.Root(),
		layout.Root(),
	)
	if err != nil {
		return moveInspection{}, fmt.Errorf(
			"inspect organization destination %q: %w",
			layout.Root(),
			err,
		)
	}
	if destination.State == workspace.RoleInspectionContained {
		inspection.conflict = fmt.Sprintf(
			"organization destination %q already exists",
			layout.Root(),
		)
		return inspection, nil
	}

	state, err := controlplane.OpenReadOnly(
		ctx,
		current.workspaceLayout.StatePath(),
	)
	if err != nil {
		return moveInspection{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	for _, selector := range []string{request.NewSlug, layout.Root()} {
		projection, err := state.Organization(ctx, selector)
		if err == nil && projection.ID != current.manifest.ID {
			inspection.conflict = fmt.Sprintf(
				"organization selector %q is already registered to %q",
				selector,
				projection.ID,
			)
			return inspection, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return moveInspection{}, err
		}
	}

	inspection.manifest.Slug = request.NewSlug
	inspection.manifest.Aliases = moveAliases(
		current.manifest.Aliases,
		request.NewSlug,
		layout.Root(),
		current.manifest.Slug,
		current.layout.Root(),
	)
	if err := inspection.manifest.Validate(layout); err != nil {
		return moveInspection{}, err
	}
	return inspection, nil
}

func moveAliases(
	aliases []string,
	currentSlug string,
	currentPath string,
	values ...string,
) []string {
	filtered := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias == currentSlug || alias == currentPath {
			continue
		}
		filtered = append(filtered, alias)
	}
	return appendUniqueAliases(filtered, currentSlug, values...)
}

func moveEffects(
	current managedOrganization,
	layout Layout,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{
			Action: "move",
			Target: layout.Root(),
			Status: status,
		},
		{
			Action: "replace",
			Target: layout.ManifestPath(),
			Status: status,
		},
		{
			Action: "project",
			Target: current.workspaceLayout.StatePath(),
			Status: status,
		},
	}
}

func validateMoveRequest(request MoveRequest) error {
	if request.Selector == "" {
		return errors.New("organization selector is required")
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return err
	}
	return ValidateSlug(request.NewSlug)
}

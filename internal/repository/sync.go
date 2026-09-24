package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

type FetchRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type UpdateRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

func (service *Service) Fetch(
	ctx context.Context,
	request FetchRequest,
) (capability.Result[MutationData], error) {
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyResult(FetchDescriptor), err
	}
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyResult(FetchDescriptor), err
	}
	data := MutationData{Repository: managed.data}
	if managed.manifest.Status == StatusArchived {
		return conflictResult(
			FetchDescriptor,
			data,
			"archived repositories cannot be fetched",
		), nil
	}
	effects := []capability.Effect{{
		Action: "fetch",
		Target: managed.data.ClonePath,
		Status: capability.EffectPlanned,
	}}
	if !request.Apply {
		return result(
			FetchDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}
	return service.applyGitOperation(
		FetchDescriptor,
		managed,
		data,
		effects,
		[]gitOperation{{
			action: "fetch",
			apply: func() error {
				return service.git.Fetch(
					ctx,
					managed.data.ClonePath,
					managed.manifest.CanonicalRemote.Name,
				)
			},
		}},
	)
}

func (service *Service) Update(
	ctx context.Context,
	request UpdateRequest,
) (capability.Result[MutationData], error) {
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyResult(UpdateDescriptor), err
	}
	managed, inventory, conflict, err := service.inspectUpdate(ctx, request)
	if err != nil {
		return emptyResult(UpdateDescriptor), err
	}
	data := MutationData{Repository: managed.data}
	if conflict != "" {
		return conflictResult(UpdateDescriptor, data, conflict), nil
	}
	if inventory.Behind == 0 {
		return result(
			UpdateDescriptor,
			capability.OutcomeUnchanged,
			data,
			[]capability.Effect{{
				Action: "fast-forward",
				Target: managed.data.ClonePath,
				Status: capability.EffectSkipped,
			}},
		), nil
	}
	effects := []capability.Effect{
		{
			Action: "fetch",
			Target: managed.data.ClonePath,
			Status: capability.EffectPlanned,
		},
		{
			Action: "fast-forward",
			Target: managed.data.ClonePath,
			Status: capability.EffectPlanned,
		},
	}
	if !request.Apply {
		return result(
			UpdateDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
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
		return emptyResult(UpdateDescriptor), err
	}
	defer unlock()
	managed, inventory, conflict, err = service.inspectUpdate(ctx, request)
	if err != nil {
		return emptyResult(UpdateDescriptor), err
	}
	data.Repository = managed.data
	if conflict != "" {
		return conflictResult(UpdateDescriptor, data, conflict), nil
	}
	if inventory.Behind == 0 {
		return result(
			UpdateDescriptor,
			capability.OutcomeUnchanged,
			data,
			[]capability.Effect{{
				Action: "fast-forward",
				Target: managed.data.ClonePath,
				Status: capability.EffectSkipped,
			}},
		), nil
	}
	branch := inventory.DefaultBranch
	if branch == "" {
		branch = inventory.Branch
	}
	if branch == "" {
		return conflictResult(
			UpdateDescriptor,
			data,
			"repository has no branch eligible for fast-forward",
		), nil
	}
	return service.applyGitOperation(
		UpdateDescriptor,
		managed,
		data,
		effects,
		[]gitOperation{
			{
				action: "fetch",
				apply: func() error {
					return service.git.Fetch(
						ctx,
						managed.data.ClonePath,
						managed.manifest.CanonicalRemote.Name,
					)
				},
			},
			{
				action: "fast-forward",
				apply: func() error {
					return service.git.FastForward(
						ctx,
						managed.data.ClonePath,
						managed.manifest.CanonicalRemote.Name,
						branch,
					)
				},
			},
		},
	)
}

type gitOperation struct {
	action string
	apply  func() error
}

// applyGitOperation journals and runs an already-planned sequence of git
// mutations. It takes no context.Context: each gitOperation.apply closure is
// built at the call site and captures the caller's ctx, so cancellation still
// reaches the git invocations without this frame holding a ctx it never uses.
func (service *Service) applyGitOperation(
	descriptor capability.Descriptor,
	managed managedRepository,
	data MutationData,
	effects []capability.Effect,
	operations []gitOperation,
) (capability.Result[MutationData], error) {
	operationID := service.newID()
	if operationID == "" {
		return emptyResult(descriptor), errors.New(
			"repository operation id generator returned an empty id",
		)
	}
	data.OperationID = operationID
	store, err := journal.NewStore(
		managed.inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	steps := make([]journal.Step, 0, len(operations))
	for index, operation := range operations {
		steps = append(steps, journal.Step{
			Ordinal: index + 1,
			Action:  operation.action,
			Target:  managed.data.ClonePath,
		})
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: descriptor.Capability + ":" + managed.manifest.ID,
		Kind:           descriptor.Capability,
		Steps:          steps,
	}); err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
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
	for index, operation := range operations {
		step := index + 1
		if _, err := store.Append(operationID, journal.EventInput{
			Phase: journal.PhaseStepApplying,
			Step:  step,
		}); err != nil {
			return service.failGitOperation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := inspectCanonicalClone(managed, false); err != nil {
			return service.failGitOperation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := operation.apply(); err != nil {
			return service.failGitOperation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if _, err := store.Append(operationID, journal.EventInput{
			Phase: journal.PhaseStepApplied,
			Step:  step,
		}); err != nil {
			return service.failGitOperation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return result(descriptor, capability.OutcomeFailed, data, effects), err
	}
	appliedEffects := make([]capability.Effect, len(effects))
	copy(appliedEffects, effects)
	for index := range appliedEffects {
		appliedEffects[index].Status = capability.EffectApplied
	}
	return result(
		descriptor,
		capability.OutcomeApplied,
		data,
		appliedEffects,
	), nil
}

func (service *Service) inspectUpdate(
	ctx context.Context,
	request UpdateRequest,
) (managedRepository, Inventory, string, error) {
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return managedRepository{}, Inventory{}, "", err
	}
	if managed.manifest.Status == StatusArchived {
		return managed, Inventory{}, "archived repositories cannot be updated", nil
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return managedRepository{}, Inventory{}, "", err
	}
	inventory, err := service.git.Inventory(
		ctx,
		managed.data.ClonePath,
		managed.manifest.CanonicalRemote.Name,
	)
	if errors.Is(err, ErrAuthentication) {
		return managed, Inventory{},
			"repository authentication failed; repair credentials before updating",
			nil
	}
	if err != nil {
		return managedRepository{}, Inventory{}, "", err
	}
	switch state := classifyHealth(
		inventory,
		managed.manifest.CanonicalRemote.Name,
	); state {
	case HealthDirty:
		return managed, inventory, "dirty repositories cannot be updated", nil
	case HealthAhead:
		return managed, inventory, "repositories with local-only commits cannot be updated", nil
	case HealthDiverged:
		return managed, inventory, "diverged repositories require manual resolution", nil
	case HealthMissingClone:
		return managed, inventory, "canonical clone is missing", nil
	case HealthMissingRemote:
		return managed, inventory, "canonical remote is missing", nil
	// The remaining states do not refuse an update here. Clean and Behind are
	// exactly what a fast-forward is for; Archived and AuthenticationFailure are
	// already refused above, before health is classified. Listed explicitly so a
	// NEW HealthState cannot default into "update permitted" — exhaustive fails
	// the build until it is classified.
	case HealthClean,
		HealthBehind,
		HealthAuthenticationFailure,
		HealthArchived:
	}
	return managed, inventory, "", nil
}

func (service *Service) failGitOperation(
	store *journal.Store,
	descriptor capability.Descriptor,
	data MutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[MutationData], error) {
	if _, appendErr := store.Append(
		data.OperationID,
		journal.EventInput{
			Phase: journal.PhaseFailed,
			Error: &journal.ErrorInfo{
				Code:    descriptor.Capability + "_failed",
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
			"%w (record repository git operation failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return result(descriptor, capability.OutcomeFailed, data, effects), cause
}

func validateSyncRequest(
	selector string,
	actorType string,
	tool string,
) error {
	if selector == "" {
		return errors.New("repository selector is required")
	}
	if err := validateActor(actorType, tool); err != nil {
		return err
	}
	return nil
}

package ticket

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

type mutationInspection struct {
	current     LocatedManifest
	next        Manifest
	nextLayout  Layout
	nextProfile profile.Profile
	findings    []Finding
	unchanged   bool
}

func (service *Service) Update(
	ctx context.Context,
	request UpdateRequest,
) (capability.Result[MutationData], error) {
	if err := validateUpdateRequest(request); err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	inspection, err := inspectUpdate(
		request,
		"preview-"+service.newID(),
		service.now(),
	)
	if err != nil {
		return emptyMutationResult(UpdateDescriptor), err
	}
	result := inspectedMutationResult(
		UpdateDescriptor,
		inspection,
		request.Apply,
	)
	if inspection.unchanged {
		recovered, handled, recoveryErr := service.recoverManifestProjection(
			ctx,
			UpdateDescriptor,
			inspection,
			request.Apply,
		)
		if recoveryErr != nil || handled {
			return recovered, recoveryErr
		}
	}
	if len(inspection.findings) > 0 ||
		inspection.unchanged ||
		!request.Apply {
		return result, nil
	}

	var applied capability.Result[MutationData]
	if service.beforeMutationLock != nil {
		if err := service.beforeMutationLock(); err != nil {
			return emptyMutationResult(UpdateDescriptor), err
		}
	}
	err = withWorkspaceMutationLock(request.Scope, func() error {
		current, inspectErr := inspectUpdate(
			request,
			service.newID(),
			service.now(),
		)
		if inspectErr != nil {
			return inspectErr
		}
		if len(current.findings) > 0 ||
			current.unchanged {
			if current.unchanged {
				recovered, handled, recoveryErr :=
					service.recoverManifestProjectionLocked(
						ctx,
						UpdateDescriptor,
						current.current,
						current.nextProfile,
					)
				if recoveryErr != nil || handled {
					applied = recovered
					return recoveryErr
				}
			}
			applied = inspectedMutationResult(
				UpdateDescriptor,
				current,
				true,
			)
			return nil
		}
		applied, inspectErr = service.applyManifestMutation(
			ctx,
			UpdateDescriptor,
			current,
		)
		return inspectErr
	})
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(UpdateDescriptor), err
	}
	return applied, nil
}

func validateUpdateRequest(request UpdateRequest) error {
	if request.Selector == "" {
		return errors.New("ticket selector is required")
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return err
	}
	if err := validateResolvedProfile(request.Profile); err != nil {
		return err
	}
	if request.NextProfile != nil &&
		!reflect.DeepEqual(*request.NextProfile, request.Profile) {
		return errors.New(
			"ticket profile upgrades require the profile upgrade/reconcile planner",
		)
	}
	return nil
}

func inspectUpdate(
	request UpdateRequest,
	operationID string,
	now time.Time,
) (mutationInspection, error) {
	current, err := findTicket(request.Scope, request.Selector)
	if err != nil {
		return mutationInspection{}, err
	}
	if err := current.Manifest.Validate(
		current.Layout,
		request.Profile,
	); err != nil {
		return mutationInspection{}, err
	}
	checkpoints, findings, err := inspectAppendOnly(
		current.Manifest,
		current.Layout,
		request.Profile,
	)
	if err != nil {
		return mutationInspection{}, err
	}
	inspection := mutationInspection{
		current:     current,
		next:        current.Manifest,
		nextLayout:  current.Layout,
		nextProfile: request.Profile,
		findings:    findings,
	}
	if len(findings) > 0 {
		return inspection, nil
	}
	inspection.next.AppendOnlyCheckpoints = checkpoints
	if request.NextProfile != nil {
		inspection.nextProfile = *request.NextProfile
		inspection.next.Profile = profile.ProfileReference{
			ID:      request.NextProfile.ID,
			Version: request.NextProfile.Version,
		}
	}
	if request.Title != nil {
		inspection.next.Title = *request.Title
	}
	if request.PathSlug != nil {
		inspection.next.PathSlug = *request.PathSlug
	}
	if request.BranchSlug != nil {
		inspection.next.Branch.Slug = *request.BranchSlug
	}
	if request.Type != nil {
		inspection.next.Type = *request.Type
	}
	if request.Status != nil {
		inspection.next.Status = *request.Status
	}
	if request.Priority != nil {
		inspection.next.Priority = *request.Priority
	}
	if request.Owner != nil {
		inspection.next.Owner = *request.Owner
	}
	if request.Tags != nil {
		inspection.next.Tags = append([]string(nil), (*request.Tags)...)
		sort.Slice(inspection.next.Tags, func(left int, right int) bool {
			return foldSelector(inspection.next.Tags[left]) <
				foldSelector(inspection.next.Tags[right])
		})
	}
	inspection.nextLayout, err = request.Scope.Ticket(
		inspection.next.LocalKey,
		inspection.next.PathSlug,
	)
	if err != nil {
		return mutationInspection{}, err
	}
	inspection.next.Branch.Intent, err = BranchIntent(
		inspection.next.Type,
		inspection.next.LocalKey,
		inspection.next.Branch.Slug,
		inspection.nextProfile,
	)
	if err != nil {
		return mutationInspection{}, err
	}
	if reflect.DeepEqual(inspection.next, current.Manifest) &&
		filepath.Clean(inspection.nextLayout.Root()) ==
			filepath.Clean(current.Layout.Root()) {
		inspection.unchanged = true
		return inspection, nil
	}
	now = nextMutationTime(current.Manifest.UpdatedAt, now)
	inspection.next.UpdatedAt = now.UTC()
	inspection.next.LastMutation = Provenance{
		OperationID: operationID,
		ActorType:   request.ActorType,
		ActorID:     request.ActorID,
		Tool:        request.Tool,
	}
	inspection.next, err = PrepareMutation(
		current.Manifest,
		inspection.next,
		current.Layout,
		inspection.nextLayout,
		request.Profile,
		inspection.nextProfile,
	)
	if err != nil {
		return mutationInspection{}, err
	}
	if err := validateMutationAvailability(
		inspection.current,
		inspection.nextLayout,
		inspection.next,
	); err != nil {
		return mutationInspection{}, err
	}
	return inspection, nil
}

func nextMutationTime(previous time.Time, candidate time.Time) time.Time {
	candidate = candidate.UTC()
	if !candidate.After(previous) {
		return previous.Add(time.Nanosecond).UTC()
	}
	return candidate
}

func validateMutationAvailability(
	current LocatedManifest,
	nextLayout Layout,
	next Manifest,
) error {
	existing, err := ReadWorkspaceManifests(
		nextLayout.Scope().WorkspaceRoot(),
	)
	if err != nil {
		return err
	}
	filtered := make([]LocatedManifest, 0, len(existing))
	for _, candidate := range existing {
		if candidate.Manifest.ID == current.Manifest.ID {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return ValidateAvailable(nextLayout, next, filtered)
}

func inspectedMutationResult(
	descriptor capability.Descriptor,
	inspection mutationInspection,
	apply bool,
) capability.Result[MutationData] {
	data := MutationData{
		Ticket:       ticketData(inspection.nextLayout, inspection.next),
		PreviousPath: inspection.current.Layout.Root(),
		Findings:     append([]Finding(nil), inspection.findings...),
	}
	if len(inspection.findings) > 0 {
		result := mutationResult(
			descriptor,
			capability.OutcomeConflict,
			data,
			[]capability.Effect{},
		)
		result.NextActions = findingsNextActions(inspection.findings)
		return result
	}
	effects := manifestMutationEffects(
		inspection,
		capability.EffectPlanned,
	)
	if inspection.unchanged {
		return mutationResult(
			descriptor,
			capability.OutcomeUnchanged,
			data,
			skippedEffects(effects),
		)
	}
	if !apply {
		return mutationResult(
			descriptor,
			capability.OutcomePlanned,
			data,
			effects,
		)
	}
	return mutationResult(descriptor, capability.OutcomePlanned, data, effects)
}

func (service *Service) applyManifestMutation(
	ctx context.Context,
	descriptor capability.Descriptor,
	inspection mutationInspection,
) (capability.Result[MutationData], error) {
	resources, err := loadWorkspaceResources(
		inspection.current.Layout.Scope().WorkspaceRoot(),
	)
	if err != nil {
		return emptyMutationResult(descriptor), err
	}
	before, err := os.ReadFile(inspection.current.Layout.StatusPath())
	if err != nil {
		return emptyMutationResult(descriptor), err
	}
	after, err := encodeManifest(inspection.next)
	if err != nil {
		return emptyMutationResult(descriptor), err
	}
	renderFile, err := os.Open(
		inspection.current.Layout.RenderManifestPath(),
	)
	if err != nil {
		return emptyMutationResult(descriptor), err
	}
	renderManifest, decodeErr := profile.DecodeRenderManifest(renderFile)
	closeErr := renderFile.Close()
	if decodeErr != nil {
		return emptyMutationResult(descriptor), decodeErr
	}
	if closeErr != nil {
		return emptyMutationResult(descriptor), closeErr
	}
	data := MutationData{
		Ticket:       ticketData(inspection.nextLayout, inspection.next),
		OperationID:  inspection.next.LastMutation.OperationID,
		PreviousPath: inspection.current.Layout.Root(),
		Findings:     []Finding{},
	}
	effects := manifestMutationEffects(
		inspection,
		capability.EffectPlanned,
	)
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	moved := filepath.Clean(inspection.current.Layout.Root()) !=
		filepath.Clean(inspection.nextLayout.Root())
	stagingRoot := ""
	stagingStatusPath := ""
	steps := make([]journal.Step, 0, 3)
	if moved {
		stagingRoot = filepath.Join(
			inspection.current.Layout.Scope().TicketsRoot(),
			".aidb",
			"relocations",
			inspection.current.Manifest.ID+"-"+data.OperationID,
		)
		statusRelative, relativeErr := filepath.Rel(
			inspection.current.Layout.Root(),
			inspection.current.Layout.StatusPath(),
		)
		if relativeErr != nil {
			return failedMutationResult(descriptor, data, effects), relativeErr
		}
		stagingStatusPath = filepath.Join(stagingRoot, statusRelative)
		data.RecoveryPath = stagingRoot
		steps = append(steps, journal.Step{
			Ordinal:       1,
			Action:        "move-to-staging",
			Target:        inspection.current.Layout.StatusPath(),
			AppliedTarget: stagingStatusPath,
			BeforeHash:    journal.Digest(before),
		})
		steps = append(steps, journal.Step{
			Ordinal:    2,
			Action:     "replace",
			Target:     stagingStatusPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		})
		steps = append(steps, journal.Step{
			Ordinal:   3,
			Action:    "publish-move",
			Target:    inspection.nextLayout.StatusPath(),
			AfterHash: journal.Digest(after),
		})
	} else {
		steps = append(steps, journal.Step{
			Ordinal:    1,
			Action:     "replace",
			Target:     inspection.nextLayout.StatusPath(),
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		})
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    data.OperationID,
		IdempotencyKey: descriptor.Capability + ":" + inspection.next.ID,
		Kind:           descriptor.Capability,
		Steps:          steps,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
	}
	if moved {
		if _, err := store.Append(data.OperationID, journal.EventInput{
			Phase: journal.PhaseStepApplying,
			Step:  1,
		}); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := atomicfile.MkdirAllWithin(
			inspection.current.Layout.Scope().TicketsRoot(),
			filepath.Dir(stagingRoot),
			0o755,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if _, err := os.Lstat(stagingRoot); err == nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				fmt.Errorf(
					"ticket relocation recovery path %q already exists",
					stagingRoot,
				),
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := atomicfile.Rename(
			inspection.current.Layout.Root(),
			stagingRoot,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if _, err := store.Append(data.OperationID, journal.EventInput{
			Phase: journal.PhaseStepApplied,
			Step:  1,
		}); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if service.afterFilesystemStage != nil {
			if err := service.afterFilesystemStage(stagingRoot); err != nil {
				return service.failMutation(
					store,
					descriptor,
					data,
					effects,
					err,
				)
			}
		}
		if err := applyReplacement(
			store,
			data.OperationID,
			2,
			stagingStatusPath,
			before,
			after,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if _, err := store.Append(data.OperationID, journal.EventInput{
			Phase: journal.PhaseStepApplying,
			Step:  3,
		}); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := atomicfile.MkdirAllWithin(
			inspection.current.Layout.Scope().TicketsRoot(),
			filepath.Dir(inspection.nextLayout.Root()),
			0o755,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if err := atomicfile.Rename(
			stagingRoot,
			inspection.nextLayout.Root(),
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		if _, err := store.Append(data.OperationID, journal.EventInput{
			Phase: journal.PhaseStepApplied,
			Step:  3,
		}); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
		data.RecoveryPath = ""
	} else if err := applyReplacement(
		store,
		data.OperationID,
		1,
		inspection.nextLayout.StatusPath(),
		before,
		after,
	); err != nil {
		return service.failMutation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if service.beforeProjection != nil {
		if err := service.beforeProjection(
			ctx,
			inspection.nextLayout,
			inspection.next,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
	}
	projection, err := buildLifecycleProjection(
		inspection.next,
		inspection.nextLayout,
		inspection.nextProfile,
		renderManifest,
		after,
		service.now(),
	)
	if err != nil {
		return service.failMutation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	state, err := controlplane.Open(ctx, resources.statePath)
	if err != nil {
		return service.failMutation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.ObserveTicket(ctx, projection); err != nil {
		_ = state.Close()
		return service.failMutation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			store,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterProjection != nil {
		if err := service.afterProjection(
			ctx,
			inspection.nextLayout,
			inspection.next,
		); err != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	return mutationResult(
		descriptor,
		capability.OutcomeApplied,
		data,
		manifestMutationEffects(
			inspection,
			capability.EffectApplied,
		),
	), nil
}

func applyReplacement(
	store *journal.Store,
	operationID string,
	step int,
	path string,
	before []byte,
	after []byte,
) error {
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switch {
	case bytes.Equal(current, after):
	case bytes.Equal(current, before):
		if err := atomicfile.Write(
			path,
			atomicfile.Options{Mode: 0o644},
			func(writer io.Writer) error {
				_, writeErr := writer.Write(after)
				return writeErr
			},
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"ticket manifest %q changed after planning",
			path,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step,
	}); err != nil {
		return err
	}
	return nil
}

func (service *Service) recoverManifestProjection(
	ctx context.Context,
	descriptor capability.Descriptor,
	inspection mutationInspection,
	apply bool,
) (capability.Result[MutationData], bool, error) {
	operationID := inspection.current.Manifest.LastMutation.OperationID
	if operationID == "" {
		return emptyMutationResult(descriptor), false, nil
	}
	resources, err := loadWorkspaceResources(
		inspection.current.Layout.Scope().WorkspaceRoot(),
	)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	plan, err := store.ReadPlan(operationID)
	if errors.Is(err, os.ErrNotExist) {
		return emptyMutationResult(descriptor), false, nil
	}
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	if !validManifestRecoveryPlan(plan, descriptor, inspection.current) {
		return emptyMutationResult(descriptor), false, nil
	}
	state, err := store.Inspect(operationID)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	if state.Status == journal.StatusCommitted {
		return emptyMutationResult(descriptor), false, nil
	}
	data := MutationData{
		Ticket:      ticketData(inspection.current.Layout, inspection.current.Manifest),
		OperationID: operationID,
		Findings:    []Finding{},
	}
	if state.Status == journal.StatusAttention {
		return failedMutationResult(
				descriptor,
				data,
				[]capability.Effect{},
			), true, fmt.Errorf(
				"ticket mutation operation %q needs journal attention: %s",
				operationID,
				state.Reason,
			)
	}
	needsProjection, err := service.manifestProjectionNeedsRepair(
		ctx,
		resources,
		inspection.current,
		inspection.nextProfile,
	)
	if err != nil {
		return failedMutationResult(
			descriptor,
			data,
			[]capability.Effect{},
		), true, err
	}
	effects := projectionRecoveryEffects(
		resources,
		operationID,
		needsProjection,
		capability.EffectPlanned,
	)
	if !apply {
		return mutationResult(
			descriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), true, nil
	}

	var applied capability.Result[MutationData]
	err = withWorkspaceMutationLock(
		inspection.current.Layout.Scope(),
		func() error {
			current, findErr := findTicket(
				inspection.current.Layout.Scope(),
				inspection.current.Manifest.ID,
			)
			if findErr != nil {
				return findErr
			}
			if current.Manifest.LastMutation.OperationID != operationID {
				applied = inspectedMutationResult(
					descriptor,
					mutationInspection{
						current:     current,
						next:        current.Manifest,
						nextLayout:  current.Layout,
						nextProfile: inspection.nextProfile,
						findings:    []Finding{},
						unchanged:   true,
					},
					true,
				)
				return nil
			}
			applied, findErr = service.applyManifestProjectionRecovery(
				ctx,
				descriptor,
				resources,
				store,
				current,
				inspection.nextProfile,
			)
			return findErr
		},
	)
	if err != nil {
		if applied.Capability != "" {
			return applied, true, err
		}
		return emptyMutationResult(descriptor), true, err
	}
	return applied, true, nil
}

func (service *Service) recoverManifestProjectionLocked(
	ctx context.Context,
	descriptor capability.Descriptor,
	current LocatedManifest,
	activeProfile profile.Profile,
) (capability.Result[MutationData], bool, error) {
	operationID := current.Manifest.LastMutation.OperationID
	if operationID == "" {
		return emptyMutationResult(descriptor), false, nil
	}
	resources, err := loadWorkspaceResources(
		current.Layout.Scope().WorkspaceRoot(),
	)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	plan, err := store.ReadPlan(operationID)
	if errors.Is(err, os.ErrNotExist) {
		return emptyMutationResult(descriptor), false, nil
	}
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	if !validManifestRecoveryPlan(plan, descriptor, current) {
		return emptyMutationResult(descriptor), false, nil
	}
	state, err := store.Inspect(operationID)
	if err != nil {
		return emptyMutationResult(descriptor), true, err
	}
	if state.Status == journal.StatusCommitted {
		return emptyMutationResult(descriptor), false, nil
	}
	if state.Status == journal.StatusAttention {
		data := MutationData{
			Ticket:      ticketData(current.Layout, current.Manifest),
			OperationID: operationID,
			Findings:    []Finding{},
		}
		return failedMutationResult(
				descriptor,
				data,
				[]capability.Effect{},
			), true, fmt.Errorf(
				"ticket mutation operation %q needs journal attention: %s",
				operationID,
				state.Reason,
			)
	}
	result, err := service.applyManifestProjectionRecovery(
		ctx,
		descriptor,
		resources,
		store,
		current,
		activeProfile,
	)
	return result, true, err
}

func validManifestRecoveryPlan(
	plan journal.Plan,
	descriptor capability.Descriptor,
	current LocatedManifest,
) bool {
	if plan.Kind != descriptor.Capability ||
		plan.IdempotencyKey !=
			descriptor.Capability+":"+current.Manifest.ID {
		return false
	}
	statusPath := filepath.Clean(current.Layout.StatusPath())
	for _, step := range plan.Steps {
		if filepath.Clean(step.Target) == statusPath ||
			(step.AppliedTarget != "" &&
				filepath.Clean(step.AppliedTarget) == statusPath) {
			return true
		}
	}
	return false
}

func (service *Service) applyManifestProjectionRecovery(
	ctx context.Context,
	descriptor capability.Descriptor,
	resources workspaceResources,
	store *journal.Store,
	current LocatedManifest,
	activeProfile profile.Profile,
) (capability.Result[MutationData], error) {
	operationID := current.Manifest.LastMutation.OperationID
	data := MutationData{
		Ticket:      ticketData(current.Layout, current.Manifest),
		OperationID: operationID,
		Findings:    []Finding{},
	}
	state, err := store.Inspect(operationID)
	if err != nil {
		return failedMutationResult(descriptor, data, []capability.Effect{}), err
	}
	if state.Status == journal.StatusCommitted {
		return mutationResult(
			descriptor,
			capability.OutcomeUnchanged,
			data,
			[]capability.Effect{},
		), nil
	}
	needsProjection, err := service.manifestProjectionNeedsRepair(
		ctx,
		resources,
		current,
		activeProfile,
	)
	if err != nil {
		return failedMutationResult(descriptor, data, []capability.Effect{}), err
	}
	effects := projectionRecoveryEffects(
		resources,
		operationID,
		needsProjection,
		capability.EffectPlanned,
	)
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	if needsProjection {
		manifestBytes, readErr := os.ReadFile(current.Layout.StatusPath())
		if readErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				readErr,
			)
		}
		renderFile, openErr := os.Open(current.Layout.RenderManifestPath())
		if openErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				openErr,
			)
		}
		renderManifest, decodeErr := profile.DecodeRenderManifest(renderFile)
		closeErr := renderFile.Close()
		if decodeErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				decodeErr,
			)
		}
		if closeErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				closeErr,
			)
		}
		if service.beforeProjection != nil {
			if hookErr := service.beforeProjection(
				ctx,
				current.Layout,
				current.Manifest,
			); hookErr != nil {
				return service.failMutation(
					store,
					descriptor,
					data,
					effects,
					hookErr,
				)
			}
		}
		projection, buildErr := buildLifecycleProjection(
			current.Manifest,
			current.Layout,
			activeProfile,
			renderManifest,
			manifestBytes,
			service.now(),
		)
		if buildErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				buildErr,
			)
		}
		controlState, openErr := controlplane.Open(ctx, resources.statePath)
		if openErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				openErr,
			)
		}
		if observeErr := controlState.ObserveTicket(ctx, projection); observeErr != nil {
			_ = controlState.Close()
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				observeErr,
			)
		}
		if closeErr := controlState.Close(); closeErr != nil {
			return service.failMutation(
				store,
				descriptor,
				data,
				effects,
				closeErr,
			)
		}
		if service.afterProjection != nil {
			if hookErr := service.afterProjection(
				ctx,
				current.Layout,
				current.Manifest,
			); hookErr != nil {
				return service.failMutation(
					store,
					descriptor,
					data,
					effects,
					hookErr,
				)
			}
		}
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), err
	}
	return mutationResult(
		descriptor,
		capability.OutcomeApplied,
		data,
		projectionRecoveryEffects(
			resources,
			operationID,
			needsProjection,
			capability.EffectApplied,
		),
	), nil
}

func (service *Service) manifestProjectionNeedsRepair(
	ctx context.Context,
	resources workspaceResources,
	current LocatedManifest,
	activeProfile profile.Profile,
) (bool, error) {
	manifestBytes, err := os.ReadFile(current.Layout.StatusPath())
	if err != nil {
		return false, err
	}
	renderFile, err := os.Open(current.Layout.RenderManifestPath())
	if err != nil {
		return false, err
	}
	renderManifest, decodeErr := profile.DecodeRenderManifest(renderFile)
	closeErr := renderFile.Close()
	if decodeErr != nil {
		return false, decodeErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	expected, err := buildLifecycleProjection(
		current.Manifest,
		current.Layout,
		activeProfile,
		renderManifest,
		manifestBytes,
		service.now(),
	)
	if err != nil {
		return false, err
	}
	state, err := controlplane.OpenReadOnly(ctx, resources.statePath)
	if err != nil {
		return false, err
	}
	actual, readErr := state.Ticket(
		ctx,
		controlplane.TicketScope{
			OrganizationID: current.Manifest.OrganizationID,
			RepositoryID:   current.Manifest.RepositoryID,
		},
		current.Manifest.ID,
	)
	closeStateErr := state.Close()
	if closeStateErr != nil {
		return false, closeStateErr
	}
	if errors.Is(readErr, sql.ErrNoRows) {
		return true, nil
	}
	if readErr != nil {
		return false, readErr
	}
	return !equivalentLifecycleProjection(expected, actual), nil
}

func equivalentLifecycleProjection(
	expected controlplane.TicketProjection,
	actual controlplane.TicketProjection,
) bool {
	expected.ObservedAt = time.Time{}
	actual.ObservedAt = time.Time{}
	expected.Sources = nil
	actual.Sources = nil
	expected.Dependencies = nil
	actual.Dependencies = nil
	sort.Strings(expected.Aliases)
	sort.Strings(actual.Aliases)
	sort.Slice(expected.Artifacts, func(left int, right int) bool {
		if expected.Artifacts[left].Role != expected.Artifacts[right].Role {
			return expected.Artifacts[left].Role < expected.Artifacts[right].Role
		}
		return expected.Artifacts[left].Path < expected.Artifacts[right].Path
	})
	sort.Slice(actual.Artifacts, func(left int, right int) bool {
		if actual.Artifacts[left].Role != actual.Artifacts[right].Role {
			return actual.Artifacts[left].Role < actual.Artifacts[right].Role
		}
		return actual.Artifacts[left].Path < actual.Artifacts[right].Path
	})
	return reflect.DeepEqual(expected, actual)
}

func projectionRecoveryEffects(
	resources workspaceResources,
	operationID string,
	needsProjection bool,
	status capability.EffectStatus,
) []capability.Effect {
	projectionStatus := status
	if !needsProjection {
		projectionStatus = capability.EffectSkipped
	}
	return []capability.Effect{
		{
			Action: "project",
			Target: resources.statePath,
			Status: projectionStatus,
		},
		{
			Action: "commit-operation",
			Target: filepath.Join(resources.eventsDir, operationID),
			Status: status,
		},
	}
}

func withWorkspaceMutationLock(
	scope ScopeLayout,
	mutate func() error,
) error {
	controlDir := filepath.Join(scope.WorkspaceRoot(), ".aidb")
	if err := os.MkdirAll(controlDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(controlDir, "workspace.lock")
	processLock := processWorkspaceMutex(lockPath)
	processLock.Lock()
	defer processLock.Unlock()

	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	unlock, err := lockfile.Lock(file)
	if err != nil {
		_ = file.Close()
		return err
	}
	defer func() {
		unlock()
		_ = file.Close()
	}()
	return mutate()
}

func manifestMutationEffects(
	inspection mutationInspection,
	status capability.EffectStatus,
) []capability.Effect {
	effects := make([]capability.Effect, 0, 3)
	if filepath.Clean(inspection.current.Layout.Root()) !=
		filepath.Clean(inspection.nextLayout.Root()) {
		effects = append(effects, capability.Effect{
			Action: "move",
			Target: inspection.current.Layout.Root() + " -> " +
				inspection.nextLayout.Root(),
			Status: status,
		})
	}
	effects = append(
		effects,
		capability.Effect{
			Action: "replace",
			Target: inspection.nextLayout.StatusPath(),
			Status: status,
		},
		capability.Effect{
			Action: "project",
			Target: filepath.Join(
				inspection.nextLayout.Scope().WorkspaceRoot(),
				".aidb",
				"state.sqlite",
			),
			Status: status,
		},
	)
	return effects
}

func skippedEffects(effects []capability.Effect) []capability.Effect {
	result := append([]capability.Effect(nil), effects...)
	for index := range result {
		result[index].Status = capability.EffectSkipped
	}
	return result
}

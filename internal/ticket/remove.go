package ticket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

type removeInspection struct {
	current  LocatedManifest
	findings []Finding
}

func (service *Service) Remove(
	ctx context.Context,
	request RemoveRequest,
) (capability.Result[MutationData], error) {
	if err := validateRemoveRequest(request); err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	inspection, err := service.inspectRemove(ctx, request)
	if errors.Is(err, sql.ErrNoRows) {
		return service.removedTicketResult(ctx, request)
	}
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	result := removeInspectionResult(inspection, request.Apply)
	if len(inspection.findings) > 0 || !request.Apply {
		return result, nil
	}

	var applied capability.Result[MutationData]
	err = withWorkspaceMutationLock(request.Scope, func() error {
		current, inspectErr := service.inspectRemove(ctx, request)
		if errors.Is(inspectErr, sql.ErrNoRows) {
			applied, inspectErr = service.removedTicketResult(ctx, request)
			return inspectErr
		}
		if inspectErr != nil {
			return inspectErr
		}
		if len(current.findings) > 0 {
			applied = removeInspectionResult(current, true)
			return nil
		}
		applied, inspectErr = service.applyRemove(ctx, current)
		return inspectErr
	})
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(RemoveDescriptor), err
	}
	return applied, nil
}

func validateRemoveRequest(request RemoveRequest) error {
	if request.Selector == "" {
		return errors.New("ticket selector is required")
	}
	if request.ExpectedID == "" {
		return errors.New("ticket remove requires the exact expected id")
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return err
	}
	return validateResolvedProfile(request.Profile)
}

func (service *Service) removedTicketResult(
	ctx context.Context,
	request RemoveRequest,
) (capability.Result[MutationData], error) {
	resources, err := loadWorkspaceResources(request.Scope.WorkspaceRoot())
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	state, err := controlplane.OpenReadOnly(ctx, resources.statePath)
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	_, projectionErr := state.Ticket(
		ctx,
		controlplane.TicketScope{
			OrganizationID: request.Scope.OrganizationID(),
			RepositoryID:   request.Scope.RepositoryID(),
		},
		request.ExpectedID,
	)
	closeErr := state.Close()
	if closeErr != nil {
		return emptyMutationResult(RemoveDescriptor), closeErr
	}
	data := MutationData{
		Ticket: TicketData{
			Manifest: Manifest{ID: request.ExpectedID},
		},
		Findings: []Finding{},
	}
	projectionMissing := errors.Is(projectionErr, sql.ErrNoRows)
	if projectionErr != nil && !projectionMissing {
		return emptyMutationResult(RemoveDescriptor), projectionErr
	}
	recoveryPath, operationID, recoveryState, recoveryErr :=
		service.removeRecovery(resources, request.ExpectedID)
	if recoveryErr != nil {
		return emptyMutationResult(RemoveDescriptor), recoveryErr
	}
	if recoveryPath != "" {
		data.OperationID = operationID
		data.RecoveryPath = recoveryPath
		if recoveryState.Status == journal.StatusCommitted &&
			projectionMissing {
			result := mutationResult(
				RemoveDescriptor,
				capability.OutcomeUnchanged,
				data,
				[]capability.Effect{},
			)
			result.Warnings = []capability.Notice{{
				Code:    "ticket.remove.cleanup_pending",
				Message: "ticket removal is committed but recovery trash cleanup remains pending",
			}}
			return result, nil
		}
		result := mutationResult(
			RemoveDescriptor,
			capability.OutcomeAttention,
			data,
			[]capability.Effect{},
		)
		message := fmt.Sprintf(
			"ticket removal operation is %s with retained recovery data",
			recoveryState.Status,
		)
		if !projectionMissing {
			message += " and the ticket projection is still present"
		}
		result.Warnings = []capability.Notice{{
			Code:    "ticket.remove.recovery_pending",
			Message: message,
		}}
		result.Recovery = capability.Recovery{
			Required: true,
			Guidance: []string{
				"Inspect the durable remove journal and retained trash before retrying or restoring source.",
			},
		}
		return result, nil
	}
	if projectionMissing {
		return mutationResult(
			RemoveDescriptor,
			capability.OutcomeUnchanged,
			data,
			[]capability.Effect{},
		), nil
	}
	return conflictMutationResult(
		RemoveDescriptor,
		data,
		"ticket.remove.source_missing",
		"portable ticket source is missing while its projection still exists",
	), nil
}

func (service *Service) removeRecovery(
	resources workspaceResources,
	ticketID string,
) (string, string, journal.State, error) {
	trashRoot := filepath.Join(resources.layout.ControlDir(), "trash")
	entries, err := os.ReadDir(trashRoot)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", journal.State{}, nil
	}
	if err != nil {
		return "", "", journal.State{}, err
	}
	prefix := ticketID + "-"
	recoveryPath := ""
	operationID := ""
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if recoveryPath != "" {
			return "", "", journal.State{}, fmt.Errorf(
				"multiple removal recovery roots exist for ticket %q",
				ticketID,
			)
		}
		recoveryPath = filepath.Join(trashRoot, entry.Name())
		operationID = strings.TrimPrefix(entry.Name(), prefix)
	}
	if recoveryPath == "" {
		return "", "", journal.State{}, nil
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return "", "", journal.State{}, err
	}
	state, err := store.Inspect(operationID)
	if err != nil {
		return "", "", journal.State{}, err
	}
	return recoveryPath, operationID, state, nil
}

func (service *Service) inspectRemove(
	ctx context.Context,
	request RemoveRequest,
) (removeInspection, error) {
	current, err := findTicket(request.Scope, request.Selector)
	if err != nil {
		return removeInspection{}, err
	}
	if err := current.Manifest.Validate(
		current.Layout,
		request.Profile,
	); err != nil {
		return removeInspection{}, err
	}
	inspection := removeInspection{current: current, findings: []Finding{}}
	if current.Manifest.ID != request.ExpectedID {
		inspection.findings = append(
			inspection.findings,
			Finding{
				Code:     "ticket.remove.identity_mismatch",
				Severity: "error",
				Summary:  "The selected ticket does not match the exact expected id.",
				Evidence: []string{
					"selected=" + current.Manifest.ID,
					"expected=" + request.ExpectedID,
				},
				NextAction: capability.Action{
					Code:    "review_remove_target",
					Message: "Review the exact ticket id before removing it.",
				},
			},
		)
	}
	if current.Manifest.ArchiveState != ArchiveStateArchived {
		inspection.findings = append(
			inspection.findings,
			Finding{
				Code:     "ticket.remove.not_archived",
				Severity: "error",
				Summary:  "Only an archived ticket can be removed.",
				Evidence: []string{string(current.Manifest.ArchiveState)},
				NextAction: capability.Action{
					Code:    "archive_ticket",
					Message: "Archive the ticket and review the remove preview again.",
				},
			},
		)
	}
	_, appendFindings, err := inspectAppendOnly(
		current.Manifest,
		current.Layout,
		request.Profile,
	)
	if err != nil {
		return removeInspection{}, err
	}
	inspection.findings = append(
		inspection.findings,
		appendFindings...,
	)
	worktreeFindings, err := service.worktreeFindings(ctx, current)
	if err != nil {
		return removeInspection{}, err
	}
	inspection.findings = append(
		inspection.findings,
		worktreeFindings...,
	)
	authoredFindings, err := unresolvedAuthoredFindings(
		current,
		request.Profile,
	)
	if err != nil {
		return removeInspection{}, err
	}
	inspection.findings = append(
		inspection.findings,
		authoredFindings...,
	)
	return inspection, nil
}

func (service *Service) worktreeFindings(
	ctx context.Context,
	current LocatedManifest,
) ([]Finding, error) {
	if current.Layout.Scope().Kind() != ScopeRepository {
		return []Finding{}, nil
	}
	manifest, err := repository.ReadManifest(filepath.Join(
		current.Layout.Scope().OwnerRoot(),
		".aidb",
		"manifest.yaml",
	))
	if err != nil {
		return nil, err
	}
	selectors := append(
		[]string{
			current.Manifest.LocalKey,
			current.Manifest.VisibleKey,
		},
		current.Manifest.Aliases...,
	)
	for _, worktree := range manifest.Worktrees {
		for _, selector := range selectors {
			if foldSelector(worktree.TicketKey) != foldSelector(selector) {
				continue
			}
			return []Finding{{
				Code:     "ticket.remove.worktree_registered",
				Severity: "error",
				Summary:  "A repository worktree remains registered to the ticket.",
				Evidence: []string{
					worktree.Path,
					worktree.Branch,
				},
				NextAction: capability.Action{
					Code:    "remove_ticket_worktree",
					Message: "Safely prune or unregister the ticket worktree first.",
				},
			}}, nil
		}
	}
	clonePath := manifest.CanonicalClone.Path
	if !manifest.CanonicalClone.External {
		clonePath = filepath.Join(current.Layout.Scope().OwnerRoot(), clonePath)
	}
	worktrees, err := service.git.Worktrees(ctx, clonePath)
	if err != nil {
		return nil, fmt.Errorf("inspect live repository worktrees: %w", err)
	}
	for _, worktree := range worktrees {
		if filepath.Clean(worktree.Path) == filepath.Clean(clonePath) ||
			worktree.Branch != current.Manifest.Branch.Intent {
			continue
		}
		return []Finding{{
			Code:     "ticket.remove.worktree_live",
			Severity: "error",
			Summary:  "A live Git worktree still uses the ticket branch.",
			Evidence: []string{
				worktree.Path,
				worktree.Branch,
			},
			NextAction: capability.Action{
				Code:    "remove_ticket_worktree",
				Message: "Safely remove or reassign the live ticket worktree first.",
			},
		}}, nil
	}
	return []Finding{}, nil
}

func unresolvedAuthoredFindings(
	current LocatedManifest,
	activeProfile profile.Profile,
) ([]Finding, error) {
	file, err := os.Open(current.Layout.RenderManifestPath())
	if err != nil {
		return nil, err
	}
	rendered, decodeErr := profile.DecodeRenderManifest(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	records := make(map[string]profile.RenderedRecord, len(rendered.Artifacts))
	for _, record := range rendered.Artifacts {
		records[record.Role] = record
	}
	checkpoints := make(
		map[string]AppendOnlyCheckpoint,
		len(current.Manifest.AppendOnlyCheckpoints),
	)
	for _, checkpoint := range current.Manifest.AppendOnlyCheckpoints {
		checkpoints[checkpoint.Role] = checkpoint
	}
	findings := make([]Finding, 0)
	for _, artifact := range activeProfile.Artifacts {
		if artifact.Authority != profile.AuthorityAuthored ||
			artifact.Kind != profile.ArtifactFile {
			continue
		}
		content, err := os.ReadFile(filepath.Join(
			current.Layout.Root(),
			filepath.FromSlash(artifact.Path),
		))
		if err != nil {
			return nil, err
		}
		hash := profile.HashContent(content)
		acceptedHash := ""
		if artifact.AppendOnly {
			acceptedHash = checkpoints[artifact.Role].Hash
		} else {
			acceptedHash = records[artifact.Role].ObservedHash
		}
		if acceptedHash == hash {
			continue
		}
		findings = append(findings, Finding{
			Code:     "ticket.remove.authored_changes",
			Severity: "error",
			Summary: fmt.Sprintf(
				"Authored role %q has unresolved changes.",
				artifact.Role,
			),
			Evidence: []string{artifact.Path},
			NextAction: capability.Action{
				Code:    "resolve_authored_changes",
				Message: "Review, preserve, or explicitly accept authored changes first.",
			},
		})
	}
	return findings, nil
}

func removeInspectionResult(
	inspection removeInspection,
	apply bool,
) capability.Result[MutationData] {
	data := MutationData{
		Ticket:   ticketData(inspection.current.Layout, inspection.current.Manifest),
		Findings: append([]Finding(nil), inspection.findings...),
	}
	if len(inspection.findings) > 0 {
		result := mutationResult(
			RemoveDescriptor,
			capability.OutcomeConflict,
			data,
			[]capability.Effect{},
		)
		result.NextActions = findingsNextActions(inspection.findings)
		return result
	}
	effects := removeEffects(
		inspection.current,
		capability.EffectPlanned,
	)
	if !apply {
		return mutationResult(
			RemoveDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		)
	}
	return mutationResult(
		RemoveDescriptor,
		capability.OutcomePlanned,
		data,
		effects,
	)
}

func (service *Service) applyRemove(
	ctx context.Context,
	inspection removeInspection,
) (capability.Result[MutationData], error) {
	resources, err := loadWorkspaceResources(
		inspection.current.Layout.Scope().WorkspaceRoot(),
	)
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	operationID := service.newID()
	data := MutationData{
		Ticket:      ticketData(inspection.current.Layout, inspection.current.Manifest),
		OperationID: operationID,
		Findings:    []Finding{},
	}
	effects := removeEffects(
		inspection.current,
		capability.EffectPlanned,
	)
	statusContent, err := os.ReadFile(inspection.current.Layout.StatusPath())
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	trashRoot := filepath.Join(
		resources.layout.ControlDir(),
		"trash",
		inspection.current.Manifest.ID+"-"+operationID,
	)
	statusRelative, err := filepath.Rel(
		inspection.current.Layout.Root(),
		inspection.current.Layout.StatusPath(),
	)
	if err != nil {
		return emptyMutationResult(RemoveDescriptor), err
	}
	trashStatusPath := filepath.Join(trashRoot, statusRelative)
	data.RecoveryPath = trashRoot
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return failedMutationResult(RemoveDescriptor, data, effects), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: RemoveDescriptor.Capability + ":" + inspection.current.Manifest.ID,
		Kind:           RemoveDescriptor.Capability,
		Steps: []journal.Step{{
			Ordinal:       1,
			Action:        "move-to-trash",
			Target:        inspection.current.Layout.StatusPath(),
			AppliedTarget: trashStatusPath,
			BeforeHash:    journal.Digest(statusContent),
		}},
	}); err != nil {
		return failedMutationResult(RemoveDescriptor, data, effects), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedMutationResult(RemoveDescriptor, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return service.failMutation(
				store,
				RemoveDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := atomicfile.MkdirAllWithin(
		resources.layout.ControlDir(),
		filepath.Dir(trashRoot),
		0o755,
	); err != nil {
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := atomicfile.Rename(
		inspection.current.Layout.Root(),
		trashRoot,
	); err != nil {
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterFilesystemStage != nil {
		if err := service.afterFilesystemStage(trashRoot); err != nil {
			return service.failMutation(
				store,
				RemoveDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	restoreSource := func(cause error) error {
		if err := atomicfile.MkdirAllWithin(
			inspection.current.Layout.Scope().TicketsRoot(),
			filepath.Dir(inspection.current.Layout.Root()),
			0o755,
		); err != nil {
			return errors.Join(cause, err)
		}
		if err := atomicfile.Rename(
			trashRoot,
			inspection.current.Layout.Root(),
		); err != nil {
			return errors.Join(cause, err)
		}
		data.RecoveryPath = ""
		return cause
	}
	if service.beforeProjection != nil {
		if err := service.beforeProjection(
			ctx,
			inspection.current.Layout,
			inspection.current.Manifest,
		); err != nil {
			cause := restoreSource(err)
			return service.failMutation(
				store,
				RemoveDescriptor,
				data,
				effects,
				cause,
			)
		}
	}
	state, err := controlplane.Open(ctx, resources.statePath)
	if err != nil {
		cause := restoreSource(err)
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			cause,
		)
	}
	scope := controlplane.TicketScope{
		OrganizationID: inspection.current.Manifest.OrganizationID,
		RepositoryID:   inspection.current.Manifest.RepositoryID,
	}
	if err := state.RemoveTicketProjection(
		ctx,
		scope,
		inspection.current.Manifest.ID,
	); err != nil {
		_ = state.Close()
		cause := restoreSource(err)
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			cause,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			store,
			RemoveDescriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterProjection != nil {
		if err := service.afterProjection(
			ctx,
			inspection.current.Layout,
			inspection.current.Manifest,
		); err != nil {
			return service.failMutation(
				store,
				RemoveDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedMutationResult(RemoveDescriptor, data, effects), err
	}
	result := mutationResult(
		RemoveDescriptor,
		capability.OutcomeApplied,
		data,
		removeEffects(
			inspection.current,
			capability.EffectApplied,
		),
	)
	if err := os.RemoveAll(trashRoot); err != nil {
		result.Warnings = append(result.Warnings, capability.Notice{
			Code: "ticket.remove.cleanup_pending",
			Message: fmt.Sprintf(
				"ticket removal committed but recovery trash cleanup failed: %v",
				err,
			),
		})
		result.Recovery = capability.Recovery{
			Required: false,
			Guidance: []string{
				"Remove the committed ticket recovery trash when convenient.",
			},
		}
		return result, nil
	}
	result.Data.RecoveryPath = ""
	return result, nil
}

func removeEffects(
	current LocatedManifest,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{
			Action: "remove",
			Target: current.Layout.Root(),
			Status: status,
		},
		{
			Action: "unproject",
			Target: filepath.Join(
				current.Layout.Scope().WorkspaceRoot(),
				".aidb",
				"state.sqlite",
			),
			Status: status,
		},
	}
}

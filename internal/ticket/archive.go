package ticket

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func (service *Service) Archive(
	ctx context.Context,
	request ArchiveRequest,
) (capability.Result[MutationData], error) {
	if request.Selector == "" {
		return emptyMutationResult(ArchiveDescriptor), errors.New(
			"ticket selector is required",
		)
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	if err := validateResolvedProfile(request.Profile); err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	preview, err := inspectDirectMutation(
		request.Scope,
		request.Selector,
		request.Profile,
		"preview-"+service.newID(),
		service.now(),
		false,
		func(next *Manifest, _ *profile.Profile, now time.Time) {
			if request.Restore {
				if next.ArchiveState == ArchiveStateActive {
					return
				}
				next.ArchiveState = ArchiveStateActive
				next.ArchivedAt = nil
				return
			}
			if next.ArchiveState == ArchiveStateArchived {
				return
			}
			next.ArchiveState = ArchiveStateArchived
			archivedAt := now.UTC()
			next.ArchivedAt = &archivedAt
		},
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if err != nil {
		return emptyMutationResult(ArchiveDescriptor), err
	}
	result := inspectedMutationResult(
		ArchiveDescriptor,
		preview,
		request.Apply,
	)
	if preview.unchanged {
		recovered, handled, recoveryErr := service.recoverManifestProjection(
			ctx,
			ArchiveDescriptor,
			preview,
			request.Apply,
		)
		if recoveryErr != nil || handled {
			return recovered, recoveryErr
		}
	}
	if len(preview.findings) > 0 ||
		preview.unchanged ||
		!request.Apply {
		return result, nil
	}

	var applied capability.Result[MutationData]
	if service.beforeMutationLock != nil {
		if err := service.beforeMutationLock(); err != nil {
			return emptyMutationResult(ArchiveDescriptor), err
		}
	}
	err = withWorkspaceMutationLock(request.Scope, func() error {
		current, inspectErr := inspectDirectMutation(
			request.Scope,
			request.Selector,
			request.Profile,
			service.newID(),
			service.now(),
			false,
			func(next *Manifest, _ *profile.Profile, now time.Time) {
				if request.Restore {
					if next.ArchiveState == ArchiveStateActive {
						return
					}
					next.ArchiveState = ArchiveStateActive
					next.ArchivedAt = nil
					return
				}
				if next.ArchiveState == ArchiveStateArchived {
					return
				}
				next.ArchiveState = ArchiveStateArchived
				archivedAt := now.UTC()
				next.ArchivedAt = &archivedAt
			},
			request.ActorType,
			request.ActorID,
			request.Tool,
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
						ArchiveDescriptor,
						current.current,
						current.nextProfile,
					)
				if recoveryErr != nil || handled {
					applied = recovered
					return recoveryErr
				}
			}
			applied = inspectedMutationResult(
				ArchiveDescriptor,
				current,
				true,
			)
			return nil
		}
		applied, inspectErr = service.applyManifestMutation(
			ctx,
			ArchiveDescriptor,
			current,
		)
		return inspectErr
	})
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(ArchiveDescriptor), err
	}
	return applied, nil
}

func inspectDirectMutation(
	scope ScopeLayout,
	selector string,
	activeProfile profile.Profile,
	operationID string,
	now time.Time,
	closeMutation bool,
	mutate func(*Manifest, *profile.Profile, time.Time),
	actorType string,
	actorID string,
	tool string,
) (mutationInspection, error) {
	current, err := findTicket(scope, selector)
	if err != nil {
		return mutationInspection{}, err
	}
	if err := current.Manifest.Validate(current.Layout, activeProfile); err != nil {
		return mutationInspection{}, err
	}
	checkpoints, findings, err := inspectAppendOnly(
		current.Manifest,
		current.Layout,
		activeProfile,
	)
	if err != nil {
		return mutationInspection{}, err
	}
	inspection := mutationInspection{
		current:     current,
		next:        current.Manifest,
		nextLayout:  current.Layout,
		nextProfile: activeProfile,
		findings:    findings,
	}
	if len(findings) > 0 {
		return inspection, nil
	}
	inspection.next.AppendOnlyCheckpoints = checkpoints
	now = nextMutationTime(current.Manifest.UpdatedAt, now)
	mutate(&inspection.next, &inspection.nextProfile, now)
	if manifestsEqualIgnoringAudit(
		inspection.next,
		current.Manifest,
	) {
		inspection.unchanged = true
		return inspection, nil
	}
	inspection.next.UpdatedAt = now.UTC()
	inspection.next.LastMutation = Provenance{
		OperationID: operationID,
		ActorType:   actorType,
		ActorID:     actorID,
		Tool:        tool,
	}
	if closeMutation {
		inspection.next, err = PrepareCloseMutation(
			current.Manifest,
			inspection.next,
			current.Layout,
			inspection.nextLayout,
			activeProfile,
			inspection.nextProfile,
		)
	} else {
		inspection.next, err = PrepareMutation(
			current.Manifest,
			inspection.next,
			current.Layout,
			inspection.nextLayout,
			activeProfile,
			inspection.nextProfile,
		)
	}
	return inspection, err
}

func manifestsEqualIgnoringAudit(left Manifest, right Manifest) bool {
	left.UpdatedAt = time.Time{}
	right.UpdatedAt = time.Time{}
	left.LastMutation = Provenance{}
	right.LastMutation = Provenance{}
	return reflect.DeepEqual(left, right)
}

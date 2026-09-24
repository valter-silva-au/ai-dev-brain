package ticket

import (
	"context"
	"errors"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func (service *Service) Close(
	ctx context.Context,
	request CloseRequest,
) (capability.Result[MutationData], error) {
	if request.Selector == "" {
		return emptyMutationResult(CloseDescriptor), errors.New(
			"ticket selector is required",
		)
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return emptyMutationResult(CloseDescriptor), err
	}
	if err := validateResolvedProfile(request.Profile); err != nil {
		return emptyMutationResult(CloseDescriptor), err
	}
	current, err := findTicket(request.Scope, request.Selector)
	if err != nil {
		return emptyMutationResult(CloseDescriptor), err
	}
	gateFindings, err := service.completionFindings(
		ctx,
		ticketData(current.Layout, current.Manifest),
	)
	if err != nil {
		return emptyMutationResult(CloseDescriptor), err
	}
	if len(gateFindings) > 0 {
		data := MutationData{
			Ticket:   ticketData(current.Layout, current.Manifest),
			Findings: gateFindings,
		}
		result := mutationResult(
			CloseDescriptor,
			capability.OutcomeConflict,
			data,
			[]capability.Effect{},
		)
		result.NextActions = findingsNextActions(gateFindings)
		return result, nil
	}

	preview, err := inspectDirectMutation(
		request.Scope,
		request.Selector,
		request.Profile,
		"preview-"+service.newID(),
		service.now(),
		true,
		func(next *Manifest, _ *profile.Profile, now time.Time) {
			if next.Status == StatusDone {
				return
			}
			next.Status = StatusDone
			closedAt := now.UTC()
			next.ClosedAt = &closedAt
		},
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if err != nil {
		return emptyMutationResult(CloseDescriptor), err
	}
	result := inspectedMutationResult(
		CloseDescriptor,
		preview,
		request.Apply,
	)
	if preview.unchanged {
		recovered, handled, recoveryErr := service.recoverManifestProjection(
			ctx,
			CloseDescriptor,
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
			return emptyMutationResult(CloseDescriptor), err
		}
	}
	err = withWorkspaceMutationLock(request.Scope, func() error {
		lockedCurrent, findErr := findTicket(
			request.Scope,
			request.Selector,
		)
		if findErr != nil {
			return findErr
		}
		lockedFindings, gateErr := service.completionFindings(
			ctx,
			ticketData(lockedCurrent.Layout, lockedCurrent.Manifest),
		)
		if gateErr != nil {
			return gateErr
		}
		if len(lockedFindings) > 0 {
			applied = mutationResult(
				CloseDescriptor,
				capability.OutcomeConflict,
				MutationData{
					Ticket: ticketData(
						lockedCurrent.Layout,
						lockedCurrent.Manifest,
					),
					Findings: lockedFindings,
				},
				[]capability.Effect{},
			)
			applied.NextActions = findingsNextActions(lockedFindings)
			return nil
		}
		inspection, inspectErr := inspectDirectMutation(
			request.Scope,
			request.Selector,
			request.Profile,
			service.newID(),
			service.now(),
			true,
			func(next *Manifest, _ *profile.Profile, now time.Time) {
				if next.Status == StatusDone {
					return
				}
				next.Status = StatusDone
				closedAt := now.UTC()
				next.ClosedAt = &closedAt
			},
			request.ActorType,
			request.ActorID,
			request.Tool,
		)
		if inspectErr != nil {
			return inspectErr
		}
		if len(inspection.findings) > 0 ||
			inspection.unchanged {
			if inspection.unchanged {
				recovered, handled, recoveryErr :=
					service.recoverManifestProjectionLocked(
						ctx,
						CloseDescriptor,
						inspection.current,
						inspection.nextProfile,
					)
				if recoveryErr != nil || handled {
					applied = recovered
					return recoveryErr
				}
			}
			applied = inspectedMutationResult(
				CloseDescriptor,
				inspection,
				true,
			)
			return nil
		}
		applied, inspectErr = service.applyManifestMutation(
			ctx,
			CloseDescriptor,
			inspection,
		)
		return inspectErr
	})
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(CloseDescriptor), err
	}
	return applied, nil
}

func (service *Service) completionFindings(
	ctx context.Context,
	ticket TicketData,
) ([]Finding, error) {
	if ticket.Manifest.Status == StatusDone {
		return []Finding{}, nil
	}
	if service.completionGate == nil {
		return []Finding{{
			Code:     "ticket.completion.gate_unavailable",
			Severity: "error",
			Summary:  "No completion/conformance gate is configured.",
			Evidence: []string{},
			NextAction: capability.Action{
				Code:    "configure_completion_gate",
				Message: "Configure and run the ticket completion gate.",
			},
		}}, nil
	}
	findings, err := service.completionGate.Check(ctx, ticket)
	if err != nil {
		return nil, err
	}
	return append([]Finding(nil), findings...), nil
}

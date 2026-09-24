package ticket

import (
	"errors"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

func mutationResult(
	descriptor capability.Descriptor,
	outcome capability.Outcome,
	data MutationData,
	effects []capability.Effect,
) capability.Result[MutationData] {
	result := capability.Result[MutationData]{
		Capability:  descriptor.Capability,
		Version:     descriptor.Version,
		Outcome:     outcome,
		Data:        data,
		Effects:     effects,
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
	if outcome == capability.OutcomePlanned {
		result.NextActions = []capability.Action{{
			Code:    "apply_" + descriptor.Capability,
			Message: "Run the ticket operation with apply enabled.",
		}}
	}
	return result
}

func conflictMutationResult(
	descriptor capability.Descriptor,
	data MutationData,
	code string,
	message string,
) capability.Result[MutationData] {
	result := mutationResult(
		descriptor,
		capability.OutcomeConflict,
		data,
		[]capability.Effect{},
	)
	result.Warnings = []capability.Notice{{
		Code:    code,
		Message: message,
	}}
	return result
}

func failedMutationResult(
	descriptor capability.Descriptor,
	data MutationData,
	effects []capability.Effect,
) capability.Result[MutationData] {
	result := mutationResult(
		descriptor,
		capability.OutcomeFailed,
		data,
		failedEffects(effects),
	)
	result.Recovery = capability.Recovery{
		Required: true,
		Guidance: []string{
			"Inspect the durable operation journal before retrying.",
			"Keep portable ticket source authoritative and rebuild the projection last.",
		},
	}
	return result
}

func emptyMutationResult(
	descriptor capability.Descriptor,
) capability.Result[MutationData] {
	return mutationResult(
		descriptor,
		"",
		MutationData{Findings: []Finding{}},
		[]capability.Effect{},
	)
}

func (service *Service) failMutation(
	store *journal.Store,
	descriptor capability.Descriptor,
	data MutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[MutationData], error) {
	errorInfo := &journal.ErrorInfo{
		Code:    descriptor.Capability + ".failed",
		Message: cause.Error(),
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseFailed,
		Error: errorInfo,
	}); err != nil {
		return failedMutationResult(descriptor, data, effects), errors.Join(
			cause,
			err,
		)
	}
	return failedMutationResult(descriptor, data, effects), cause
}

func failedEffects(effects []capability.Effect) []capability.Effect {
	result := append([]capability.Effect(nil), effects...)
	for index := range result {
		if result[index].Status != capability.EffectApplied {
			result[index].Status = capability.EffectFailed
		}
	}
	return result
}

func validateMutationActor(actorType string, tool string) error {
	if actorType == "" {
		return errors.New("ticket mutation actor type is required")
	}
	if tool == "" {
		return errors.New("ticket mutation tool is required")
	}
	return nil
}

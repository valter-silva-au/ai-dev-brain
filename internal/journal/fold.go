package journal

import (
	"errors"
	"fmt"
	"os"
)

func (store *Store) Inspect(operationID string) (State, error) {
	if err := validateOperationID(operationID); err != nil {
		return State{}, err
	}

	journalRoot, err := store.openJournalRoot(false)
	if err != nil {
		return State{}, err
	}
	defer func() {
		_ = journalRoot.Close()
	}()
	operationRoot, err := openOperationRoot(
		journalRoot,
		operationID,
		false,
	)
	if err != nil {
		return State{}, err
	}
	defer func() {
		_ = operationRoot.Close()
	}()
	plan, err := readPlan(operationRoot, operationID)
	if err != nil {
		return State{}, err
	}
	events, issue, err := readEvents(operationRoot, operationID)
	if err != nil {
		return State{}, err
	}
	if issue != "" {
		return State{
			Status:       StatusAttention,
			LastSequence: len(events),
			Reason:       issue,
		}, nil
	}

	state := State{Status: StatusPlanned}
	for _, event := range events {
		state.LastSequence = event.Sequence
		switch event.Phase {
		case PhaseApplying:
			state.Status = StatusApplying
		case PhaseStepApplying:
			step, ok := planStep(plan, event.Step)
			if !ok {
				state.Status = StatusAttention
				state.Reason = "unknown_step"
				return state, nil
			}
			status, appliedUnrecorded, reason, err := inspectTarget(step)
			if err != nil {
				return State{}, err
			}
			state.Status = status
			state.AppliedUnrecorded = appliedUnrecorded
			state.Reason = reason
			if status == StatusAttention {
				return state, nil
			}
		case PhaseStepApplied:
			state.Status = StatusApplying
			state.AppliedUnrecorded = false
			state.Reason = ""
		case PhaseCommitted:
			state.Status = StatusCommitted
			state.AppliedUnrecorded = false
			state.Reason = ""
		case PhaseFailed:
			state.Status = StatusFailed
			state.AppliedUnrecorded = false
			state.Reason = ""
		default:
			state.Status = StatusAttention
			state.Reason = "unknown_phase"
			return state, nil
		}
	}

	return state, nil
}

// InspectRecorded folds only durable journal events. It does not inspect step
// targets and is intended for callers that revalidate targets through their
// own rooted containment boundary.
func (store *Store) InspectRecorded(operationID string) (State, error) {
	if err := validateOperationID(operationID); err != nil {
		return State{}, err
	}
	journalRoot, err := store.openJournalRoot(false)
	if err != nil {
		return State{}, err
	}
	defer func() {
		_ = journalRoot.Close()
	}()
	operationRoot, err := openOperationRoot(
		journalRoot,
		operationID,
		false,
	)
	if err != nil {
		return State{}, err
	}
	defer func() {
		_ = operationRoot.Close()
	}()
	plan, err := readPlan(operationRoot, operationID)
	if err != nil {
		return State{}, err
	}
	events, issue, err := readEvents(operationRoot, operationID)
	if err != nil {
		return State{}, err
	}
	if issue != "" {
		return State{
			Status:       StatusAttention,
			LastSequence: len(events),
			Reason:       issue,
		}, nil
	}
	state := State{Status: StatusPlanned}
	for _, event := range events {
		state.LastSequence = event.Sequence
		switch event.Phase {
		case PhaseApplying:
			state.Status = StatusApplying
		case PhaseStepApplying, PhaseStepApplied:
			if _, ok := planStep(plan, event.Step); !ok {
				state.Status = StatusAttention
				state.Reason = "unknown_step"
				return state, nil
			}
			state.Status = StatusApplying
		case PhaseCommitted:
			state.Status = StatusCommitted
		case PhaseFailed:
			state.Status = StatusFailed
		default:
			state.Status = StatusAttention
			state.Reason = "unknown_phase"
			return state, nil
		}
	}
	return state, nil
}

func inspectTarget(step Step) (Status, bool, string, error) {
	if step.Action == "create-directory" {
		return inspectDirectoryTarget(step.Target)
	}

	content, err := os.ReadFile(step.Target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if step.AppliedTarget != "" {
				return inspectAppliedTarget(step)
			}
			if step.BeforeHash == "" {
				return StatusApplying, false, "", nil
			}
		}
		return StatusAttention, false, "target_unreadable", nil
	}

	hash := Digest(content)
	switch {
	case step.AfterHash != "" && hash == step.AfterHash:
		return StatusApplying, true, "", nil
	case step.BeforeHash != "" && hash == step.BeforeHash:
		return StatusApplying, false, "", nil
	default:
		return StatusAttention, false, "target_hash_ambiguous", nil
	}
}

func inspectDirectoryTarget(target string) (Status, bool, string, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StatusApplying, false, "", nil
		}
		return StatusAttention, false, "target_unreadable", nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return StatusAttention, false, "target_unsafe", nil
	}
	if !info.IsDir() {
		return StatusAttention, false, "target_not_directory", nil
	}
	return StatusApplying, true, "", nil
}

func inspectAppliedTarget(step Step) (Status, bool, string, error) {
	content, err := os.ReadFile(step.AppliedTarget)
	if err != nil {
		// A target that cannot be read is a fold *state*, not a fold failure —
		// the same contract inspectTarget and inspectDirectoryTarget hold. The
		// error's detail is the only thing dropped (State carries a reason enum,
		// not an error), and the state chosen is the fail-safe one: attention,
		// which State.Error() surfaces and which no caller may resume past. It
		// must never fold to applying/applied-unrecorded, because an unreadable
		// applied target is not evidence that the step landed. Pinned by
		// TestInspectFlagsUnreadableAppliedTargetForAttention.
		//nolint:nilerr // an unreadable target is reported as attention, not as an error.
		return StatusAttention, false, "target_unreadable", nil
	}

	hash := Digest(content)
	if (step.BeforeHash != "" && hash == step.BeforeHash) ||
		(step.AfterHash != "" && hash == step.AfterHash) {
		return StatusApplying, true, "", nil
	}
	return StatusAttention, false, "target_hash_ambiguous", nil
}

func planStep(plan Plan, ordinal int) (Step, bool) {
	if ordinal < 1 || ordinal > len(plan.Steps) {
		return Step{}, false
	}
	step := plan.Steps[ordinal-1]
	if step.Ordinal != ordinal {
		return Step{}, false
	}
	return step, true
}

func (state State) Error() error {
	if state.Status != StatusAttention {
		return nil
	}
	return fmt.Errorf("journal needs attention: %s", state.Reason)
}

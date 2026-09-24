package repository

import (
	"context"
	"errors"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

type HealthState string

const (
	HealthClean                 HealthState = "clean"
	HealthDirty                 HealthState = "dirty"
	HealthAhead                 HealthState = "ahead"
	HealthBehind                HealthState = "behind"
	HealthDiverged              HealthState = "diverged"
	HealthMissingClone          HealthState = "missing_clone"
	HealthMissingRemote         HealthState = "missing_remote"
	HealthAuthenticationFailure HealthState = "authentication_failure"
	HealthArchived              HealthState = "archived"
)

type HealthRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
}

type HealthData struct {
	Repository Data        `json:"repository"`
	State      HealthState `json:"state"`
	Inventory  Inventory   `json:"inventory"`
}

func (service *Service) Health(
	ctx context.Context,
	request HealthRequest,
) (capability.Result[HealthData], error) {
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyHealthResult(), err
	}
	state := HealthArchived
	inventory := Inventory{}
	if managed.manifest.Status != StatusArchived {
		if err := inspectCanonicalClone(managed, true); err != nil {
			return emptyHealthResult(), err
		}
		inventory, err = service.git.Inventory(
			ctx,
			managed.data.ClonePath,
			managed.manifest.CanonicalRemote.Name,
		)
		switch {
		case errors.Is(err, ErrAuthentication):
			state = HealthAuthenticationFailure
		case err != nil:
			return emptyHealthResult(), err
		default:
			state = classifyHealth(
				inventory,
				managed.manifest.CanonicalRemote.Name,
			)
		}
	}
	outcome := capability.OutcomeHealthy
	actions := []capability.Action{}
	if state != HealthClean {
		outcome = capability.OutcomeAttention
		actions = healthActions(state)
	}
	return capability.Result[HealthData]{
		Capability: HealthDescriptor.Capability,
		Version:    HealthDescriptor.Version,
		Outcome:    outcome,
		Data: HealthData{
			Repository: managed.data,
			State:      state,
			Inventory:  inventory,
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: actions,
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func classifyHealth(
	inventory Inventory,
	canonicalRemote string,
) HealthState {
	if !inventory.IsRepository {
		return HealthMissingClone
	}
	if _, ok := findRemote(inventory, canonicalRemote); !ok {
		return HealthMissingRemote
	}
	switch {
	case inventory.Dirty:
		return HealthDirty
	case inventory.Diverged ||
		(inventory.Ahead > 0 && inventory.Behind > 0):
		return HealthDiverged
	case inventory.Ahead > 0:
		return HealthAhead
	case inventory.Behind > 0:
		return HealthBehind
	default:
		return HealthClean
	}
}

func healthActions(state HealthState) []capability.Action {
	messages := map[HealthState]string{
		HealthDirty:                 "Commit or intentionally preserve local changes before updating.",
		HealthAhead:                 "Review unpublished local commits; adb will not push them.",
		HealthBehind:                "Run a reviewed fast-forward repository update.",
		HealthDiverged:              "Resolve divergence manually; adb will not merge or rebase.",
		HealthMissingClone:          "Restore or re-register the canonical clone.",
		HealthMissingRemote:         "Restore the registered canonical remote explicitly.",
		HealthAuthenticationFailure: "Repair credentials outside AI Dev Brain and retry.",
		HealthArchived:              "Restore the repository before ordinary mutation.",
	}
	message := messages[state]
	if message == "" {
		return []capability.Action{}
	}
	return []capability.Action{{
		Code:    "repository_health_" + string(state),
		Message: message,
	}}
}

func emptyHealthResult() capability.Result[HealthData] {
	return capability.Result[HealthData]{
		Capability:  HealthDescriptor.Capability,
		Version:     HealthDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

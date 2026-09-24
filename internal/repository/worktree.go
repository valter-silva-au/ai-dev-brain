package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

type WorktreeData struct {
	TicketKey      string `json:"ticket_key,omitempty"`
	Name           string `json:"name,omitempty"`
	Path           string `json:"path"`
	Branch         string `json:"branch,omitempty"`
	Active         bool   `json:"active"`
	Registered     bool   `json:"registered"`
	Missing        bool   `json:"missing"`
	Unknown        bool   `json:"unknown"`
	BranchConflict bool   `json:"branch_conflict"`
	Dirty          bool   `json:"dirty"`
	Ahead          int    `json:"ahead"`
	Behind         int    `json:"behind"`
	Locked         bool   `json:"locked"`
	Prunable       bool   `json:"prunable"`
}

type WorktreeListRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
}

type WorktreeListData struct {
	Repository Data           `json:"repository"`
	Worktrees  []WorktreeData `json:"worktrees"`
}

type WorktreeRepairRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	TicketKey     string `json:"ticket_key"`
	Name          string `json:"name"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type WorktreePruneRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Selector      string `json:"selector"`
	Path          string `json:"path"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type WorktreeMutationData struct {
	Repository  Data         `json:"repository"`
	Worktree    WorktreeData `json:"worktree"`
	OperationID string       `json:"operation_id,omitempty"`
}

func (service *Service) WorktreeList(
	ctx context.Context,
	request WorktreeListRequest,
) (capability.Result[WorktreeListData], error) {
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyWorktreeListResult(), err
	}
	worktrees, err := service.inspectWorktrees(ctx, managed, true)
	if err != nil {
		return emptyWorktreeListResult(), err
	}
	outcome := capability.OutcomeHealthy
	for _, worktree := range worktrees {
		if worktree.Missing ||
			worktree.Unknown ||
			worktree.BranchConflict ||
			worktree.Dirty ||
			worktree.Ahead > 0 {
			outcome = capability.OutcomeAttention
			break
		}
	}
	return capability.Result[WorktreeListData]{
		Capability: WorktreeListDescriptor.Capability,
		Version:    WorktreeListDescriptor.Version,
		Outcome:    outcome,
		Data: WorktreeListData{
			Repository: managed.data,
			Worktrees:  worktrees,
		},
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
}

func (service *Service) inspectWorktrees(
	ctx context.Context,
	managed managedRepository,
	allowMissingClone bool,
) ([]WorktreeData, error) {
	if err := inspectCanonicalClone(managed, allowMissingClone); err != nil {
		return nil, err
	}
	actual, err := service.git.Worktrees(
		ctx,
		managed.data.ClonePath,
	)
	if err != nil {
		return nil, err
	}
	actualByPath := make(map[string]GitWorktree, len(actual))
	branchOwners := make(map[string]int)
	for _, worktree := range actual {
		path := filepath.Clean(worktree.Path)
		actualByPath[path] = worktree
		if worktree.Branch != "" {
			branchOwners[worktree.Branch]++
		}
	}
	registeredPaths := make(map[string]struct{}, len(managed.manifest.Worktrees))
	result := make([]WorktreeData, 0, len(managed.manifest.Worktrees)+len(actual))
	for _, registration := range managed.manifest.Worktrees {
		path := filepath.Clean(filepath.Join(
			managed.inspection.layout.Root(),
			registration.Path,
		))
		registeredPaths[path] = struct{}{}
		data := WorktreeData{
			TicketKey:  registration.TicketKey,
			Name:       registration.Name,
			Path:       path,
			Branch:     registration.Branch,
			Active:     registration.Active,
			Registered: true,
		}
		worktree, ok := actualByPath[path]
		if !ok {
			data.Missing = true
			result = append(result, data)
			continue
		}
		data.Locked = worktree.Locked
		data.Prunable = worktree.Prunable
		if worktree.Branch != "" {
			data.Branch = worktree.Branch
		}
		data.BranchConflict = branchOwners[data.Branch] > 1
		if err := inspectManagedDirectory(
			managed.inspection.scope,
			path,
			true,
		); err != nil {
			return nil, err
		}
		inventory, err := service.git.Inventory(
			ctx,
			path,
			managed.manifest.CanonicalRemote.Name,
		)
		if err != nil && !errors.Is(err, ErrAuthentication) {
			return nil, err
		}
		data.Dirty = inventory.Dirty
		data.Ahead = inventory.Ahead
		data.Behind = inventory.Behind
		result = append(result, data)
	}
	for _, worktree := range actual {
		path := filepath.Clean(worktree.Path)
		if path == filepath.Clean(managed.data.ClonePath) {
			continue
		}
		if _, ok := registeredPaths[path]; ok {
			continue
		}
		data := WorktreeData{
			Path:           path,
			Branch:         worktree.Branch,
			Unknown:        true,
			BranchConflict: branchOwners[worktree.Branch] > 1,
			Locked:         worktree.Locked,
			Prunable:       worktree.Prunable,
		}
		if err := inspectManagedDirectory(
			managed.inspection.scope,
			path,
			true,
		); err != nil {
			return nil, err
		}
		inventory, err := service.git.Inventory(
			ctx,
			path,
			managed.manifest.CanonicalRemote.Name,
		)
		if err != nil && !errors.Is(err, ErrAuthentication) {
			return nil, err
		}
		data.Dirty = inventory.Dirty
		data.Ahead = inventory.Ahead
		data.Behind = inventory.Behind
		result = append(result, data)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].Path < result[right].Path
	})
	return result, nil
}

func (service *Service) WorktreeRepair(
	ctx context.Context,
	request WorktreeRepairRequest,
) (capability.Result[WorktreeMutationData], error) {
	if request.TicketKey == "" || request.Name == "" {
		return emptyWorktreeMutationResult(WorktreeRepairDescriptor),
			errors.New("worktree repair requires ticket key and name")
	}
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyWorktreeMutationResult(WorktreeRepairDescriptor), err
	}
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreeRepairDescriptor), err
	}
	registration, ok := findWorktreeRegistration(
		managed.manifest.Worktrees,
		request.TicketKey,
		request.Name,
	)
	if !ok {
		return conflictWorktreeResult(
			WorktreeRepairDescriptor,
			WorktreeMutationData{Repository: managed.data},
			"worktree repair requires an existing registration",
		), nil
	}
	worktrees, err := service.inspectWorktrees(ctx, managed, false)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreeRepairDescriptor), err
	}
	target := filepath.Clean(filepath.Join(
		managed.inspection.layout.Root(),
		registration.Path,
	))
	data := WorktreeMutationData{
		Repository: managed.data,
		Worktree: WorktreeData{
			TicketKey:  registration.TicketKey,
			Name:       registration.Name,
			Path:       target,
			Branch:     registration.Branch,
			Active:     registration.Active,
			Registered: true,
			Missing:    true,
		},
	}
	for _, worktree := range worktrees {
		if worktree.Path == target && !worktree.Missing {
			data.Worktree = worktree
			return worktreeResult(
				WorktreeRepairDescriptor,
				capability.OutcomeUnchanged,
				data,
				[]capability.Effect{{
					Action: "create",
					Target: target,
					Status: capability.EffectSkipped,
				}},
			), nil
		}
		if worktree.Path != target &&
			worktree.Branch == registration.Branch &&
			!worktree.Missing {
			return conflictWorktreeResult(
				WorktreeRepairDescriptor,
				data,
				fmt.Sprintf(
					"branch %q is already owned by %q",
					registration.Branch,
					worktree.Path,
				),
			), nil
		}
	}
	effects := []capability.Effect{{
		Action: "create",
		Target: target,
		Status: capability.EffectPlanned,
	}}
	if !request.Apply {
		return worktreeResult(
			WorktreeRepairDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}
	return service.applyWorktreeGitOperation(
		WorktreeRepairDescriptor,
		managed,
		data,
		effects,
		"create",
		target,
		func() error {
			return service.git.AddWorktree(
				ctx,
				managed.data.ClonePath,
				target,
				registration.Branch,
			)
		},
	)
}

func (service *Service) WorktreePrune(
	ctx context.Context,
	request WorktreePruneRequest,
) (capability.Result[WorktreeMutationData], error) {
	if request.Path == "" {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor),
			errors.New("worktree prune path is required")
	}
	if err := validateSyncRequest(
		request.Selector,
		request.ActorType,
		request.Tool,
	); err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	managed, err := loadManagedRepository(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
		request.Selector,
	)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	target, err := filepath.Abs(request.Path)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	registration, index, ok := findWorktreeByPath(
		managed,
		filepath.Clean(target),
	)
	data := WorktreeMutationData{
		Repository: managed.data,
		Worktree:   WorktreeData{Path: filepath.Clean(target)},
	}
	if !ok {
		return conflictWorktreeResult(
			WorktreePruneDescriptor,
			data,
			"unknown worktrees cannot be pruned",
		), nil
	}
	data.Worktree = WorktreeData{
		TicketKey:  registration.TicketKey,
		Name:       registration.Name,
		Path:       filepath.Clean(target),
		Branch:     registration.Branch,
		Active:     registration.Active,
		Registered: true,
	}
	if registration.Active {
		return conflictWorktreeResult(
			WorktreePruneDescriptor,
			data,
			"active worktrees cannot be pruned",
		), nil
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	actual, err := service.git.Worktrees(ctx, managed.data.ClonePath)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	present := false
	for _, worktree := range actual {
		if filepath.Clean(worktree.Path) == filepath.Clean(target) {
			present = true
			data.Worktree.Locked = worktree.Locked
			data.Worktree.Prunable = worktree.Prunable
			break
		}
	}
	if !present {
		return conflictWorktreeResult(
			WorktreePruneDescriptor,
			data,
			"missing worktrees must be repaired or unregistered explicitly",
		), nil
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		target,
		false,
	); err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	inventory, err := service.git.Inventory(
		ctx,
		target,
		managed.manifest.CanonicalRemote.Name,
	)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	data.Worktree.Dirty = inventory.Dirty
	data.Worktree.Ahead = inventory.Ahead
	data.Worktree.Behind = inventory.Behind
	switch {
	case inventory.Dirty:
		return conflictWorktreeResult(
			WorktreePruneDescriptor,
			data,
			"dirty worktrees cannot be pruned",
		), nil
	case inventory.Ahead > 0 || inventory.Diverged:
		return conflictWorktreeResult(
			WorktreePruneDescriptor,
			data,
			"worktrees with unpushed or diverged commits cannot be pruned",
		), nil
	}
	effects := []capability.Effect{
		{
			Action: "remove",
			Target: target,
			Status: capability.EffectPlanned,
		},
		{
			Action: "replace",
			Target: managed.inspection.layout.ManifestPath(),
			Status: capability.EffectPlanned,
		},
	}
	if !request.Apply {
		return worktreeResult(
			WorktreePruneDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}
	return service.applyWorktreePrune(
		ctx,
		request,
		managed,
		data,
		effects,
		target,
		index,
	)
}

// applyWorktreeGitOperation journals and runs one already-planned worktree git
// mutation. Like applyGitOperation it takes no context.Context: the apply
// closure is built at the call site and captures the caller's ctx.
func (service *Service) applyWorktreeGitOperation(
	descriptor capability.Descriptor,
	managed managedRepository,
	data WorktreeMutationData,
	effects []capability.Effect,
	action string,
	target string,
	apply func() error,
) (capability.Result[WorktreeMutationData], error) {
	operationID := service.newID()
	if operationID == "" {
		return emptyWorktreeMutationResult(descriptor), errors.New(
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
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: descriptor.Capability + ":" + managed.manifest.ID + ":" + target,
		Kind:           descriptor.Capability,
		Steps: []journal.Step{{
			Ordinal: 1,
			Action:  action,
			Target:  target,
		}},
	}); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return worktreeResult(
				descriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	parent := filepath.Dir(target)
	if err := atomicfile.MkdirAllWithin(
		managed.inspection.scope.workspaceLayout.Root(),
		parent,
		0o755,
	); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		parent,
		false,
	); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		target,
		true,
	); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		parent,
		false,
	); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		target,
		true,
	); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if err := apply(); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return worktreeResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	for index := range effects {
		effects[index].Status = capability.EffectApplied
	}
	data.Worktree.Missing = false
	return worktreeResult(
		descriptor,
		capability.OutcomeApplied,
		data,
		effects,
	), nil
}

func (service *Service) applyWorktreePrune(
	ctx context.Context,
	request WorktreePruneRequest,
	managed managedRepository,
	data WorktreeMutationData,
	effects []capability.Effect,
	target string,
	registrationIndex int,
) (capability.Result[WorktreeMutationData], error) {
	operationID := service.newID()
	if operationID == "" {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor),
			errors.New("repository operation id generator returned an empty id")
	}
	data.OperationID = operationID
	manifest := managed.manifest
	manifest.Worktrees = append(
		append(
			[]WorktreeRegistration(nil),
			manifest.Worktrees[:registrationIndex]...,
		),
		manifest.Worktrees[registrationIndex+1:]...,
	)
	now := service.now().UTC()
	manifest.UpdatedAt = now
	manifest.LastMutation = mutationProvenance(
		operationID,
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	if err := manifest.Validate(managed.inspection.layout); err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	content, err := encodeManifest(manifest)
	if err != nil {
		return emptyWorktreeMutationResult(WorktreePruneDescriptor), err
	}
	store, err := journal.NewStore(
		managed.inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return worktreeResult(
			WorktreePruneDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: WorktreePruneDescriptor.Capability + ":" + managed.manifest.ID + ":" + target,
		Kind:           WorktreePruneDescriptor.Capability,
		Steps: []journal.Step{
			{Ordinal: 1, Action: "remove", Target: target},
			{
				Ordinal:    2,
				Action:     "replace",
				Target:     managed.inspection.layout.ManifestPath(),
				BeforeHash: journal.Digest(managed.content),
				AfterHash:  journal.Digest(content),
			},
		},
	}); err != nil {
		return worktreeResult(
			WorktreePruneDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return service.failWorktreeMutation(
				store,
				WorktreePruneDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  1,
	}); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := inspectCanonicalClone(managed, false); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := inspectManagedDirectory(
		managed.inspection.scope,
		target,
		false,
	); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := service.git.RemoveWorktree(
		ctx,
		managed.data.ClonePath,
		target,
	); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  1,
	}); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := replaceFile(
		managed.inspection.scope.workspaceLayout.Root(),
		store,
		operationID,
		2,
		managed.inspection.layout.ManifestPath(),
		managed.content,
		content,
	); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := service.projectRepository(
		ctx,
		managed.inspection,
		manifest,
		content,
	); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return service.failWorktreeMutation(
			store,
			WorktreePruneDescriptor,
			data,
			effects,
			err,
		)
	}
	for index := range effects {
		effects[index].Status = capability.EffectApplied
	}
	return worktreeResult(
		WorktreePruneDescriptor,
		capability.OutcomeApplied,
		data,
		effects,
	), nil
}

func (service *Service) failWorktreeMutation(
	store *journal.Store,
	descriptor capability.Descriptor,
	data WorktreeMutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[WorktreeMutationData], error) {
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
			"%w (record repository worktree operation failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return worktreeResult(
		descriptor,
		capability.OutcomeFailed,
		data,
		effects,
	), cause
}

func findWorktreeRegistration(
	registrations []WorktreeRegistration,
	ticketKey string,
	name string,
) (WorktreeRegistration, bool) {
	for _, registration := range registrations {
		if registration.TicketKey == ticketKey &&
			registration.Name == name {
			return registration, true
		}
	}
	return WorktreeRegistration{}, false
}

func findWorktreeByPath(
	managed managedRepository,
	path string,
) (WorktreeRegistration, int, bool) {
	for index, registration := range managed.manifest.Worktrees {
		resolved := filepath.Clean(filepath.Join(
			managed.inspection.layout.Root(),
			registration.Path,
		))
		if resolved == path {
			return registration, index, true
		}
	}
	return WorktreeRegistration{}, -1, false
}

func worktreeResult(
	descriptor capability.Descriptor,
	outcome capability.Outcome,
	data WorktreeMutationData,
	effects []capability.Effect,
) capability.Result[WorktreeMutationData] {
	value := capability.Result[WorktreeMutationData]{
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
		value.NextActions = []capability.Action{{
			Code:    "apply_" + descriptor.Capability,
			Message: "Run the worktree operation with apply enabled.",
		}}
	}
	if outcome == capability.OutcomeFailed {
		value.Recovery = capability.Recovery{
			Required: true,
			Guidance: []string{
				"Run adb doctor before retrying the worktree operation.",
			},
		}
	}
	return value
}

func conflictWorktreeResult(
	descriptor capability.Descriptor,
	data WorktreeMutationData,
	message string,
) capability.Result[WorktreeMutationData] {
	value := worktreeResult(
		descriptor,
		capability.OutcomeConflict,
		data,
		[]capability.Effect{},
	)
	value.Warnings = []capability.Notice{{
		Code:    "repository_worktree_conflict",
		Message: message,
	}}
	return value
}

func emptyWorktreeMutationResult(
	descriptor capability.Descriptor,
) capability.Result[WorktreeMutationData] {
	return worktreeResult(
		descriptor,
		"",
		WorktreeMutationData{},
		[]capability.Effect{},
	)
}

func emptyWorktreeListResult() capability.Result[WorktreeListData] {
	return capability.Result[WorktreeListData]{
		Capability:  WorktreeListDescriptor.Capability,
		Version:     WorktreeListDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
)

type AdoptRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Organization  string `json:"organization"`
	Path          string `json:"path"`
	RemoteName    string `json:"remote_name,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type adoptInspection struct {
	repositoryInspection
	source    string
	inventory Inventory
}

func (service *Service) Adopt(
	ctx context.Context,
	request AdoptRequest,
) (capability.Result[MutationData], error) {
	if err := validateAdoptRequest(request); err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	inspection, conflict, err := service.inspectAdopt(ctx, request)
	if err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	previewID := service.newID()
	if previewID == "" {
		return emptyResult(AdoptDescriptor), errors.New(
			"repository id generator returned an empty id",
		)
	}
	manifest := service.adoptManifest(
		request,
		inspection,
		previewID,
		"planned:"+previewID,
	)
	data := MutationData{
		Repository: repositoryData(
			inspection.scope,
			inspection.layout,
			manifest,
		),
	}
	if conflict != "" {
		return conflictResult(AdoptDescriptor, data, conflict), nil
	}
	if inspection.existing != nil {
		data.Repository = repositoryData(
			inspection.scope,
			inspection.layout,
			*inspection.existing,
		)
		return result(
			AdoptDescriptor,
			capability.OutcomeUnchanged,
			data,
			repositoryEffects(
				inspection.repositoryInspection,
				"",
				capability.EffectSkipped,
			),
		), nil
	}
	if !request.Apply {
		return result(
			AdoptDescriptor,
			capability.OutcomePlanned,
			data,
			repositoryEffects(
				inspection.repositoryInspection,
				"",
				capability.EffectPlanned,
			),
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.scope.workspaceLayout.Root(),
		filepath.Join(
			inspection.scope.workspaceLayout.ControlDir(),
			"workspace.lock",
		),
	)
	if err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	defer unlock()

	inspection, conflict, err = service.inspectAdopt(ctx, request)
	if err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	if conflict != "" {
		return conflictResult(AdoptDescriptor, data, conflict), nil
	}
	if inspection.existing != nil {
		data.Repository = repositoryData(
			inspection.scope,
			inspection.layout,
			*inspection.existing,
		)
		return result(
			AdoptDescriptor,
			capability.OutcomeUnchanged,
			data,
			repositoryEffects(
				inspection.repositoryInspection,
				"",
				capability.EffectSkipped,
			),
		), nil
	}

	repositoryID := service.newID()
	operationID := service.newID()
	if repositoryID == "" || operationID == "" {
		return emptyResult(AdoptDescriptor), errors.New(
			"repository id generator returned an empty id",
		)
	}
	manifest = service.adoptManifest(
		request,
		inspection,
		repositoryID,
		operationID,
	)
	if err := manifest.Validate(inspection.layout); err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	manifestContent, err := encodeManifest(manifest)
	if err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	agentsContent, err := agentsPointer(inspection.layout)
	if err != nil {
		return emptyResult(AdoptDescriptor), err
	}
	data = MutationData{
		Repository: repositoryData(
			inspection.scope,
			inspection.layout,
			manifest,
		),
		OperationID: operationID,
	}
	effects := repositoryEffects(
		inspection.repositoryInspection,
		"",
		capability.EffectPlanned,
	)

	store, err := journal.NewStore(
		inspection.scope.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return result(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: AdoptDescriptor.Capability + ":" + repositoryID,
		Kind:           AdoptDescriptor.Capability,
		Steps: []journal.Step{
			{
				Ordinal:   1,
				Action:    "create",
				Target:    inspection.layout.ConfigPath(),
				AfterHash: journal.Digest([]byte(defaultConfig)),
			},
			{
				Ordinal:   2,
				Action:    "create",
				Target:    inspection.layout.AgentsPath(),
				AfterHash: journal.Digest([]byte(agentsContent)),
			},
			{
				Ordinal:   3,
				Action:    "create",
				Target:    inspection.layout.ManifestPath(),
				AfterHash: journal.Digest(manifestContent),
			},
		},
	}); err != nil {
		return result(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return result(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return result(
				AdoptDescriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := inspection.scope.inspectRepositoriesRole(); err != nil {
		return service.failAdopt(store, data, effects, err)
	}
	if err := atomicfile.MkdirAllWithin(
		inspection.scope.workspaceLayout.Root(),
		inspection.layout.ControlDir(),
		0o755,
	); err != nil {
		return service.failAdopt(store, data, effects, err)
	}
	for _, file := range []struct {
		step    int
		path    string
		content []byte
	}{
		{1, inspection.layout.ConfigPath(), []byte(defaultConfig)},
		{2, inspection.layout.AgentsPath(), []byte(agentsContent)},
		{3, inspection.layout.ManifestPath(), manifestContent},
	} {
		if err := applyFile(
			inspection.scope.workspaceLayout.Root(),
			store,
			operationID,
			file.step,
			file.path,
			file.content,
		); err != nil {
			return service.failAdopt(store, data, effects, err)
		}
	}
	if err := service.projectRepository(
		ctx,
		inspection.repositoryInspection,
		manifest,
		manifestContent,
	); err != nil {
		return service.failAdopt(store, data, effects, err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return result(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	return result(
		AdoptDescriptor,
		capability.OutcomeApplied,
		data,
		repositoryEffects(
			inspection.repositoryInspection,
			"",
			capability.EffectApplied,
		),
	), nil
}

func (service *Service) inspectAdopt(
	ctx context.Context,
	request AdoptRequest,
) (adoptInspection, string, error) {
	source, err := filepath.Abs(request.Path)
	if err != nil {
		return adoptInspection{}, "", fmt.Errorf(
			"resolve repository adoption path: %w",
			err,
		)
	}
	source = filepath.Clean(source)
	info, err := os.Stat(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return adoptInspection{source: source},
				fmt.Sprintf(
					"repository adoption source %q does not exist",
					source,
				), nil
		}
		return adoptInspection{}, "", err
	}
	if !info.IsDir() {
		return adoptInspection{source: source},
			fmt.Sprintf(
				"repository adoption source %q is not a directory",
				source,
			), nil
	}
	scope, err := inspectOrganizationScope(
		ctx,
		request.WorkspaceRoot,
		request.Organization,
	)
	if err != nil {
		return adoptInspection{}, "", err
	}
	if err := scope.inspectRepositoriesRole(); err != nil {
		return adoptInspection{}, "", err
	}
	remoteName := canonicalRemoteName(request.RemoteName)
	inventory, err := service.git.Inventory(ctx, source, remoteName)
	if err != nil {
		return adoptInspection{}, "", err
	}
	if !inventory.IsRepository {
		return adoptInspection{
				source:    source,
				inventory: inventory,
			}, fmt.Sprintf(
				"repository adoption source %q is not a git repository",
				source,
			), nil
	}
	remote, ok := findRemote(inventory, remoteName)
	if !ok {
		return adoptInspection{
				source:    source,
				inventory: inventory,
			}, fmt.Sprintf(
				"repository adoption source is missing remote %q",
				remoteName,
			), nil
	}
	remoteURL, err := NormalizeRemoteURL(remote.FetchURL)
	if err != nil {
		return adoptInspection{}, "", err
	}
	target, err := inspectRepositoryTarget(
		ctx,
		scope,
		remoteURL,
		remoteName,
	)
	if err != nil {
		return adoptInspection{}, "", err
	}
	inspection := adoptInspection{
		repositoryInspection: target,
		source:               source,
		inventory:            inventory,
	}
	if target.conflict != "" {
		return inspection, target.conflict, nil
	}
	if target.existing != nil && !existingMatches(
		target.existing,
		remoteURL,
		true,
		source,
	) {
		return inspection, "existing repository metadata differs from the adoption request", nil
	}
	return inspection, "", nil
}

func (service *Service) adoptManifest(
	request AdoptRequest,
	inspection adoptInspection,
	repositoryID string,
	operationID string,
) Manifest {
	manifest := NewManifest(
		repositoryID,
		inspection.scope.organizationID,
		inspection.layout,
		Remote{
			Name:     inspection.remoteName,
			Type:     RemoteTypeCanonical,
			FetchURL: inspection.remoteURL.Normalized,
		},
		service.now(),
		mutationProvenance(
			operationID,
			request.ActorType,
			request.ActorID,
			request.Tool,
		),
	)
	manifest.CanonicalClone = CloneLocation{
		Path:     inspection.source,
		External: true,
	}
	if request.DisplayName != "" {
		manifest.DisplayName = request.DisplayName
	}
	return manifest
}

func (service *Service) failAdopt(
	store *journal.Store,
	data MutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[MutationData], error) {
	if _, appendErr := store.Append(
		data.OperationID,
		journal.EventInput{
			Phase: journal.PhaseFailed,
			Error: &journal.ErrorInfo{
				Code:    "repository_adopt_failed",
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
			"%w (record repository adopt failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return result(
		AdoptDescriptor,
		capability.OutcomeFailed,
		data,
		effects,
	), cause
}

func validateAdoptRequest(request AdoptRequest) error {
	if request.Path == "" {
		return errors.New("repository adoption path is required")
	}
	return validateActor(request.ActorType, request.Tool)
}

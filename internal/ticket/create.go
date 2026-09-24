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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

const defaultTicketConfig = "schema_version: aidb.config/v1\n"

type createBundle struct {
	layout         Layout
	manifest       Manifest
	profile        profile.Profile
	manifestBytes  []byte
	render         profile.RenderResult
	renderBytes    []byte
	tombstoneBytes []byte
	files          []plannedFile
	directories    []string
}

type plannedFile struct {
	path    string
	content []byte
}

type workspaceResources struct {
	layout    workspace.Layout
	manifest  workspace.Manifest
	statePath string
	eventsDir string
}

func (service *Service) Create(
	ctx context.Context,
	request CreateRequest,
) (capability.Result[MutationData], error) {
	normalized, err := normalizeCreateRequest(request)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	resources, err := loadWorkspaceResources(normalized.Scope.WorkspaceRoot())
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}

	previewExisting, err := ReadWorkspaceManifests(
		normalized.Scope.WorkspaceRoot(),
	)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	operationID := normalized.OperationID
	if operationID == "" {
		operationID = service.newID()
	}
	if existing, found, findErr := createByOperationID(
		previewExisting,
		normalized.Scope,
		operationID,
	); findErr != nil {
		return emptyMutationResult(CreateDescriptor), findErr
	} else if found {
		return service.resumeCreate(
			ctx,
			resources,
			normalized,
			existing,
		)
	}
	if plan, found, findErr := service.createPlan(
		resources,
		operationID,
	); findErr != nil {
		return emptyMutationResult(CreateDescriptor), findErr
	} else if found {
		return service.resumePlannedCreate(
			ctx,
			resources,
			normalized,
			plan,
		)
	}
	key, err := allocatePreviewKey(
		normalized.Scope,
		normalized.KeyPolicy,
		previewExisting,
	)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	previewBundle, err := service.buildCreateBundle(
		normalized,
		key,
		service.newID(),
		operationID,
		service.now(),
	)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	if err := ValidateAvailable(
		previewBundle.layout,
		previewBundle.manifest,
		previewExisting,
	); err != nil {
		return conflictMutationResult(
			CreateDescriptor,
			MutationData{
				Ticket:   ticketData(previewBundle.layout, previewBundle.manifest),
				Findings: []Finding{},
			},
			"ticket.create.conflict",
			err.Error(),
		), nil
	}
	previewData := MutationData{
		Ticket:   ticketData(previewBundle.layout, previewBundle.manifest),
		Findings: []Finding{},
	}
	if !normalized.Apply {
		return mutationResult(
			CreateDescriptor,
			capability.OutcomePlanned,
			previewData,
			createEffects(previewBundle, resources, capability.EffectPlanned),
		), nil
	}

	var applied capability.Result[MutationData]
	_, err = service.WithAllocatedLocalKey(
		normalized.Scope,
		normalized.KeyPolicy,
		func(allocated string, existing []LocatedManifest) error {
			var buildErr error
			if previous, found, findErr := createByOperationID(
				existing,
				normalized.Scope,
				operationID,
			); findErr != nil {
				return findErr
			} else if found {
				applied, buildErr = service.resumeCreate(
					ctx,
					resources,
					normalized,
					previous,
				)
				return buildErr
			}
			bundle, buildErr := service.buildCreateBundle(
				normalized,
				allocated,
				service.newID(),
				operationID,
				service.now(),
			)
			if buildErr != nil {
				return buildErr
			}
			if availableErr := ValidateAvailable(
				bundle.layout,
				bundle.manifest,
				existing,
			); availableErr != nil {
				applied = conflictMutationResult(
					CreateDescriptor,
					MutationData{
						Ticket:   ticketData(bundle.layout, bundle.manifest),
						Findings: []Finding{},
					},
					"ticket.create.conflict",
					availableErr.Error(),
				)
				return nil
			}
			applied, buildErr = service.applyCreate(
				ctx,
				resources,
				bundle,
			)
			return buildErr
		},
	)
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(CreateDescriptor), err
	}
	return applied, nil
}

func normalizeCreateRequest(request CreateRequest) (CreateRequest, error) {
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return CreateRequest{}, err
	}
	if strings.TrimSpace(request.Title) == "" ||
		request.Title != strings.TrimSpace(request.Title) {
		return CreateRequest{}, errors.New(
			"ticket title is required without surrounding whitespace",
		)
	}
	if request.PathSlug == "" {
		request.PathSlug = Slugify(request.Title)
	}
	if request.BranchSlug == "" {
		request.BranchSlug = Slugify(request.Title)
	}
	if request.Type == "" {
		request.Type = "chore"
	}
	if request.Priority == "" {
		request.Priority = PriorityP2
	}
	if request.KeyPolicy == (KeyPolicy{}) {
		request.KeyPolicy = DefaultKeyPolicy()
	}
	if err := validateResolvedProfile(request.Profile); err != nil {
		return CreateRequest{}, err
	}
	if request.OperationID != "" && !validUUID(request.OperationID) {
		return CreateRequest{}, errors.New(
			"ticket create operation id must be a canonical non-zero UUID",
		)
	}
	if err := ValidateSlug(request.PathSlug); err != nil {
		return CreateRequest{}, err
	}
	if err := ValidateSlug(request.BranchSlug); err != nil {
		return CreateRequest{}, err
	}
	request.Tags = append([]string(nil), request.Tags...)
	sort.Slice(request.Tags, func(left int, right int) bool {
		return foldSelector(request.Tags[left]) <
			foldSelector(request.Tags[right])
	})
	return request, nil
}

func createByOperationID(
	existing []LocatedManifest,
	scope ScopeLayout,
	operationID string,
) (LocatedManifest, bool, error) {
	var matched LocatedManifest
	found := false
	for _, candidate := range existing {
		if !sameScope(scope, candidate.Layout.Scope()) ||
			candidate.Manifest.Provenance.OperationID != operationID {
			continue
		}
		if found {
			return LocatedManifest{}, false, fmt.Errorf(
				"ticket create operation %q owns multiple portable tickets",
				operationID,
			)
		}
		matched = candidate
		found = true
	}
	return matched, found, nil
}

func (service *Service) createPlan(
	resources workspaceResources,
	operationID string,
) (journal.Plan, bool, error) {
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return journal.Plan{}, false, err
	}
	plan, err := store.ReadPlan(operationID)
	if errors.Is(err, os.ErrNotExist) {
		return journal.Plan{}, false, nil
	}
	if err != nil {
		return journal.Plan{}, false, err
	}
	return plan, true, nil
}

func (service *Service) resumePlannedCreate(
	ctx context.Context,
	resources workspaceResources,
	request CreateRequest,
	plan journal.Plan,
) (capability.Result[MutationData], error) {
	const idempotencyPrefix = "ticket.create:"

	if plan.Kind != CreateDescriptor.Capability ||
		!strings.HasPrefix(plan.IdempotencyKey, idempotencyPrefix) ||
		len(plan.Steps) == 0 {
		return conflictMutationResult(
			CreateDescriptor,
			MutationData{
				OperationID: plan.OperationID,
				Findings:    []Finding{},
			},
			"ticket.create.operation_conflict",
			"operation id already belongs to a different durable plan",
		), nil
	}
	ticketID := strings.TrimPrefix(plan.IdempotencyKey, idempotencyPrefix)
	if !validUUID(ticketID) {
		return emptyMutationResult(CreateDescriptor), fmt.Errorf(
			"ticket create plan %q has invalid ticket id %q",
			plan.OperationID,
			ticketID,
		)
	}
	statusTarget := ""
	for _, step := range plan.Steps {
		candidateRoot := filepath.Dir(step.Target)
		if filepath.Base(step.Target) != "status.yaml" ||
			filepath.Clean(filepath.Dir(candidateRoot)) !=
				filepath.Clean(request.Scope.TicketsRoot()) {
			continue
		}
		if statusTarget != "" {
			return emptyMutationResult(CreateDescriptor), fmt.Errorf(
				"ticket create plan %q has multiple status targets",
				plan.OperationID,
			)
		}
		statusTarget = filepath.Clean(step.Target)
	}
	if statusTarget == "" {
		return emptyMutationResult(CreateDescriptor), fmt.Errorf(
			"ticket create plan %q has no status target",
			plan.OperationID,
		)
	}
	ticketRoot := filepath.Dir(statusTarget)
	if filepath.Clean(filepath.Dir(ticketRoot)) !=
		filepath.Clean(request.Scope.TicketsRoot()) {
		return emptyMutationResult(CreateDescriptor), fmt.Errorf(
			"ticket create plan target %q is outside the requested scope",
			statusTarget,
		)
	}
	directoryName := filepath.Base(ticketRoot)
	pathSuffix := "-" + request.PathSlug
	if !strings.HasSuffix(directoryName, pathSuffix) {
		return conflictMutationResult(
			CreateDescriptor,
			MutationData{
				OperationID: plan.OperationID,
				Findings:    []Finding{},
			},
			"ticket.create.operation_conflict",
			"operation id was already planned for a different ticket path",
		), nil
	}
	localKey := strings.TrimSuffix(directoryName, pathSuffix)
	bundle, err := service.buildCreateBundle(
		request,
		localKey,
		ticketID,
		plan.OperationID,
		plan.CreatedAt,
	)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	expectedPlan := createJournalPlan(bundle)
	data := MutationData{
		Ticket:      ticketData(bundle.layout, bundle.manifest),
		OperationID: plan.OperationID,
		Findings:    []Finding{},
	}
	effects := createEffects(
		bundle,
		resources,
		capability.EffectPlanned,
	)
	if expectedPlan.IdempotencyKey != plan.IdempotencyKey ||
		expectedPlan.Kind != plan.Kind ||
		!slices.Equal(expectedPlan.Steps, plan.Steps) {
		return conflictMutationResult(
			CreateDescriptor,
			data,
			"ticket.create.operation_conflict",
			"operation id was already planned with different create intent",
		), nil
	}
	if !request.Apply {
		return mutationResult(
			CreateDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}

	var applied capability.Result[MutationData]
	err = withWorkspaceMutationLock(request.Scope, func() error {
		existing, readErr := ReadWorkspaceManifests(
			request.Scope.WorkspaceRoot(),
		)
		if readErr != nil {
			return readErr
		}
		if current, found, findErr := createByOperationID(
			existing,
			request.Scope,
			plan.OperationID,
		); findErr != nil {
			return findErr
		} else if found {
			applied, readErr = service.resumeCreate(
				ctx,
				resources,
				request,
				current,
			)
			return readErr
		}
		if availableErr := ValidateAvailable(
			bundle.layout,
			bundle.manifest,
			existing,
		); availableErr != nil {
			applied = conflictMutationResult(
				CreateDescriptor,
				data,
				"ticket.create.conflict",
				availableErr.Error(),
			)
			return nil
		}
		applied, readErr = service.applyCreate(ctx, resources, bundle)
		return readErr
	})
	if err != nil {
		if applied.Capability != "" {
			return applied, err
		}
		return emptyMutationResult(CreateDescriptor), err
	}
	return applied, nil
}

func (service *Service) resumeCreate(
	ctx context.Context,
	resources workspaceResources,
	request CreateRequest,
	current LocatedManifest,
) (capability.Result[MutationData], error) {
	data := MutationData{
		Ticket:      ticketData(current.Layout, current.Manifest),
		OperationID: current.Manifest.Provenance.OperationID,
		Findings:    []Finding{},
	}
	if err := validateCreateRetry(request, current); err != nil {
		return conflictMutationResult(
			CreateDescriptor,
			data,
			"ticket.create.operation_conflict",
			err.Error(),
		), nil
	}
	projected, err := ticketProjectionExists(
		ctx,
		resources.statePath,
		current.Manifest,
	)
	if err != nil {
		return emptyMutationResult(CreateDescriptor), err
	}
	effects := []capability.Effect{{
		Action: "project",
		Target: resources.statePath,
		Status: capability.EffectPlanned,
	}}
	if projected {
		store, storeErr := journal.NewStore(
			resources.eventsDir,
			service.now,
			service.newID,
		)
		if storeErr != nil {
			return failedMutationResult(
				CreateDescriptor,
				data,
				effects,
			), storeErr
		}
		operation, inspectErr := store.Inspect(data.OperationID)
		if inspectErr != nil {
			return failedMutationResult(
				CreateDescriptor,
				data,
				effects,
			), inspectErr
		}
		if operation.Status == journal.StatusCommitted {
			return mutationResult(
				CreateDescriptor,
				capability.OutcomeUnchanged,
				data,
				skippedEffects(effects),
			), nil
		}
		if operation.Status == journal.StatusAttention {
			return failedMutationResult(
					CreateDescriptor,
					data,
					effects,
				), fmt.Errorf(
					"ticket create operation %q needs journal attention: %s",
					data.OperationID,
					operation.Reason,
				)
		}
		resumeEffects := []capability.Effect{
			{
				Action: "project",
				Target: resources.statePath,
				Status: capability.EffectSkipped,
			},
			{
				Action: "commit-operation",
				Target: filepath.Join(resources.eventsDir, data.OperationID),
				Status: capability.EffectPlanned,
			},
		}
		if !request.Apply {
			return mutationResult(
				CreateDescriptor,
				capability.OutcomePlanned,
				data,
				resumeEffects,
			), nil
		}
		if _, appendErr := store.Append(
			data.OperationID,
			journal.EventInput{Phase: journal.PhaseApplying},
		); appendErr != nil {
			return failedMutationResult(
				CreateDescriptor,
				data,
				resumeEffects,
			), appendErr
		}
		if _, appendErr := store.Append(
			data.OperationID,
			journal.EventInput{Phase: journal.PhaseCommitted},
		); appendErr != nil {
			return failedMutationResult(
				CreateDescriptor,
				data,
				resumeEffects,
			), appendErr
		}
		resumeEffects[1].Status = capability.EffectApplied
		return mutationResult(
			CreateDescriptor,
			capability.OutcomeApplied,
			data,
			resumeEffects,
		), nil
	}
	if !request.Apply {
		return mutationResult(
			CreateDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}

	manifestBytes, err := os.ReadFile(current.Layout.StatusPath())
	if err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	renderFile, err := os.Open(current.Layout.RenderManifestPath())
	if err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	renderManifest, decodeErr := profile.DecodeRenderManifest(renderFile)
	closeErr := renderFile.Close()
	if decodeErr != nil {
		return failedMutationResult(CreateDescriptor, data, effects), decodeErr
	}
	if closeErr != nil {
		return failedMutationResult(CreateDescriptor, data, effects), closeErr
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	if service.beforeProjection != nil {
		if err := service.beforeProjection(
			ctx,
			current.Layout,
			current.Manifest,
		); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	projection, err := buildLifecycleProjection(
		current.Manifest,
		current.Layout,
		request.Profile,
		renderManifest,
		manifestBytes,
		service.now(),
	)
	if err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	state, err := controlplane.Open(ctx, resources.statePath)
	if err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.ObserveTicket(ctx, projection); err != nil {
		_ = state.Close()
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterProjection != nil {
		if err := service.afterProjection(
			ctx,
			current.Layout,
			current.Manifest,
		); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	effects[0].Status = capability.EffectApplied
	return mutationResult(
		CreateDescriptor,
		capability.OutcomeApplied,
		data,
		effects,
	), nil
}

func validateCreateRetry(
	request CreateRequest,
	current LocatedManifest,
) error {
	manifest := current.Manifest
	if err := manifest.Validate(current.Layout, request.Profile); err != nil {
		return err
	}
	if manifest.Title != request.Title ||
		manifest.PathSlug != request.PathSlug ||
		manifest.Branch.Slug != request.BranchSlug ||
		manifest.Type != request.Type ||
		manifest.Priority != request.Priority ||
		manifest.Owner != request.Owner ||
		!slices.Equal(manifest.Tags, request.Tags) ||
		manifest.Provenance.ActorType != request.ActorType ||
		manifest.Provenance.ActorID != request.ActorID ||
		manifest.Provenance.Tool != request.Tool {
		return errors.New(
			"ticket create operation id was already used with different intent",
		)
	}
	return nil
}

func ticketProjectionExists(
	ctx context.Context,
	statePath string,
	manifest Manifest,
) (bool, error) {
	state, err := controlplane.OpenReadOnly(ctx, statePath)
	if err != nil {
		return false, err
	}
	_, projectionErr := state.Ticket(
		ctx,
		controlplane.TicketScope{
			OrganizationID: manifest.OrganizationID,
			RepositoryID:   manifest.RepositoryID,
		},
		manifest.ID,
	)
	closeErr := state.Close()
	if closeErr != nil {
		return false, closeErr
	}
	if errors.Is(projectionErr, sql.ErrNoRows) {
		return false, nil
	}
	if projectionErr != nil {
		return false, projectionErr
	}
	return true, nil
}

func allocatePreviewKey(
	scope ScopeLayout,
	policy KeyPolicy,
	existing []LocatedManifest,
) (string, error) {
	identities := make([]Identity, 0, len(existing))
	for _, candidate := range existing {
		if !sameScope(scope, candidate.Layout.Scope()) {
			continue
		}
		identities = append(identities, Identity{
			LocalKey: candidate.Manifest.LocalKey,
			Aliases:  append([]string(nil), candidate.Manifest.Aliases...),
		})
	}
	return AllocateLocalKey(policy, identities)
}

func (service *Service) buildCreateBundle(
	request CreateRequest,
	localKey string,
	ticketID string,
	operationID string,
	createdAt time.Time,
) (createBundle, error) {
	now := createdAt.UTC()
	if now.IsZero() {
		return createBundle{}, errors.New("ticket create timestamp is required")
	}
	provenance := Provenance{
		OperationID: operationID,
		ActorType:   request.ActorType,
		ActorID:     request.ActorID,
		Tool:        request.Tool,
	}
	manifest, err := NewManifest(
		ticketID,
		request.Scope,
		localKey,
		request.Title,
		request.PathSlug,
		request.BranchSlug,
		request.Type,
		request.Profile,
		now,
		provenance,
	)
	if err != nil {
		return createBundle{}, err
	}
	manifest.Priority = request.Priority
	manifest.Owner = request.Owner
	manifest.Tags = append([]string(nil), request.Tags...)

	render, err := profile.Render(profile.RenderRequest{
		Profile:  request.Profile,
		Resolver: service.templateResolver,
		Data: map[string]string{
			"Title":     request.Title,
			"TicketID":  localKey,
			"CreatedAt": now.Format("2006-01-02T15:04:05Z07:00"),
		},
	})
	if err != nil {
		return createBundle{}, err
	}
	filesByRole := make(map[string]profile.RenderedFile, len(render.Files))
	for _, file := range render.Files {
		filesByRole[file.Role] = file
	}
	checkpoints := make([]AppendOnlyCheckpoint, 0)
	for _, artifact := range request.Profile.Artifacts {
		if !artifact.AppendOnly {
			continue
		}
		file, exists := filesByRole[artifact.Role]
		if !exists {
			return createBundle{}, fmt.Errorf(
				"append-only role %q has no rendered bootstrap content",
				artifact.Role,
			)
		}
		checkpoints = append(checkpoints, AppendOnlyCheckpoint{
			Role:   artifact.Role,
			Path:   artifact.Path,
			Length: int64(len(file.Content)),
			Hash:   profile.HashContent(file.Content),
		})
	}
	sort.Slice(checkpoints, func(left int, right int) bool {
		return checkpoints[left].Role < checkpoints[right].Role
	})
	manifest.AppendOnlyCheckpoints = checkpoints
	layout, err := request.Scope.Ticket(localKey, request.PathSlug)
	if err != nil {
		return createBundle{}, err
	}
	if err := manifest.Validate(layout, request.Profile); err != nil {
		return createBundle{}, err
	}
	manifestBytes, err := encodeManifest(manifest)
	if err != nil {
		return createBundle{}, err
	}
	renderBytes, err := encodeRenderManifest(render.Manifest)
	if err != nil {
		return createBundle{}, err
	}
	tombstoneBytes, err := encodeTombstones(profile.TombstoneManifest{
		SchemaVersion: profile.TombstoneManifestSchemaVersion,
		Tombstones:    []profile.Tombstone{},
	})
	if err != nil {
		return createBundle{}, err
	}
	files := []plannedFile{
		{path: layout.StatusPath(), content: manifestBytes},
		{path: layout.ConfigPath(), content: []byte(defaultTicketConfig)},
		{path: layout.RenderManifestPath(), content: renderBytes},
		{path: layout.TombstonesPath(), content: tombstoneBytes},
	}
	for _, file := range render.Files {
		files = append(files, plannedFile{
			path:    filepath.Join(layout.Root(), filepath.FromSlash(file.Path)),
			content: append([]byte(nil), file.Content...),
		})
	}
	sort.Slice(files, func(left int, right int) bool {
		return files[left].path < files[right].path
	})
	directories := []string{layout.Root(), layout.ControlDir()}
	for _, artifact := range request.Profile.Artifacts {
		if artifact.Kind == profile.ArtifactDirectory {
			directories = append(
				directories,
				filepath.Join(
					layout.Root(),
					filepath.FromSlash(artifact.Path),
				),
			)
		}
	}
	sort.Strings(directories)
	return createBundle{
		layout:         layout,
		manifest:       manifest,
		profile:        request.Profile,
		manifestBytes:  manifestBytes,
		render:         render,
		renderBytes:    renderBytes,
		tombstoneBytes: tombstoneBytes,
		files:          files,
		directories:    directories,
	}, nil
}

func (service *Service) applyCreate(
	ctx context.Context,
	resources workspaceResources,
	bundle createBundle,
) (capability.Result[MutationData], error) {
	data := MutationData{
		Ticket:      ticketData(bundle.layout, bundle.manifest),
		OperationID: bundle.manifest.Provenance.OperationID,
		Findings:    []Finding{},
	}
	effects := createEffects(
		bundle,
		resources,
		capability.EffectPlanned,
	)
	store, err := journal.NewStore(
		resources.eventsDir,
		service.now,
		service.newID,
	)
	if err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	plan := createJournalPlan(bundle)
	if _, err := store.Begin(plan); err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if err := publishCreateBundle(bundle); err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	for index, file := range bundle.files {
		if err := recordPublishedFile(
			store,
			data.OperationID,
			index+1,
			file,
		); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if service.beforeProjection != nil {
		if err := service.beforeProjection(
			ctx,
			bundle.layout,
			bundle.manifest,
		); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	projection, err := buildLifecycleProjection(
		bundle.manifest,
		bundle.layout,
		bundle.profile,
		bundle.render.Manifest,
		bundle.manifestBytes,
		service.now(),
	)
	if err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	state, err := controlplane.Open(ctx, resources.statePath)
	if err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.ObserveTicket(ctx, projection); err != nil {
		_ = state.Close()
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			store,
			CreateDescriptor,
			data,
			effects,
			err,
		)
	}
	if service.afterProjection != nil {
		if err := service.afterProjection(
			ctx,
			bundle.layout,
			bundle.manifest,
		); err != nil {
			return service.failMutation(
				store,
				CreateDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if _, err := store.Append(data.OperationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedMutationResult(CreateDescriptor, data, effects), err
	}
	return mutationResult(
		CreateDescriptor,
		capability.OutcomeApplied,
		data,
		createEffects(bundle, resources, capability.EffectApplied),
	), nil
}

func createJournalPlan(bundle createBundle) journal.Plan {
	steps := make([]journal.Step, 0, len(bundle.files))
	for index, file := range bundle.files {
		steps = append(steps, journal.Step{
			Ordinal:   index + 1,
			Action:    "create",
			Target:    file.path,
			AfterHash: journal.Digest(file.content),
		})
	}
	return journal.Plan{
		OperationID:    bundle.manifest.Provenance.OperationID,
		IdempotencyKey: CreateDescriptor.Capability + ":" + bundle.manifest.ID,
		Kind:           CreateDescriptor.Capability,
		CreatedAt:      bundle.manifest.CreatedAt,
		Steps:          steps,
	}
}

func publishCreateBundle(bundle createBundle) error {
	info, err := os.Lstat(bundle.layout.Root())
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf(
				"ticket create target %q is not a regular directory",
				bundle.layout.Root(),
			)
		}
		return verifyPublishedBundle(bundle)
	case !errors.Is(err, os.ErrNotExist):
		return err
	}

	stagingRoot := filepath.Join(
		bundle.layout.Scope().TicketsRoot(),
		".aidb",
		"staging",
		bundle.manifest.ID+"-"+bundle.manifest.Provenance.OperationID,
	)
	if err := os.RemoveAll(stagingRoot); err != nil {
		return fmt.Errorf("reset ticket staging root: %w", err)
	}
	for _, directory := range bundle.directories {
		relative, err := filepath.Rel(bundle.layout.Root(), directory)
		if err != nil {
			return err
		}
		target := filepath.Join(stagingRoot, relative)
		if err := atomicfile.MkdirAllWithin(
			bundle.layout.Scope().OwnerRoot(),
			target,
			0o755,
		); err != nil {
			return fmt.Errorf(
				"create staged ticket directory %q: %w",
				target,
				err,
			)
		}
	}
	for _, file := range bundle.files {
		relative, err := filepath.Rel(bundle.layout.Root(), file.path)
		if err != nil {
			return err
		}
		target := filepath.Join(stagingRoot, relative)
		if err := atomicfile.MkdirAllWithin(
			bundle.layout.Scope().OwnerRoot(),
			filepath.Dir(target),
			0o755,
		); err != nil {
			return err
		}
		if err := atomicfile.Write(
			target,
			atomicfile.Options{
				Mode: 0o644,
			},
			func(writer io.Writer) error {
				_, writeErr := writer.Write(file.content)
				return writeErr
			},
		); err != nil {
			return err
		}
	}
	if err := atomicfile.Rename(stagingRoot, bundle.layout.Root()); err != nil {
		return fmt.Errorf("publish staged ticket root: %w", err)
	}
	return verifyPublishedBundle(bundle)
}

func verifyPublishedBundle(bundle createBundle) error {
	for _, directory := range bundle.directories {
		info, err := os.Stat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf(
				"ticket directory %q is not a directory",
				directory,
			)
		}
	}
	for _, file := range bundle.files {
		content, err := os.ReadFile(file.path)
		if err != nil {
			return err
		}
		if !bytes.Equal(content, file.content) {
			return fmt.Errorf(
				"ticket create target %q already has different content",
				file.path,
			)
		}
	}
	return nil
}

func recordPublishedFile(
	store *journal.Store,
	operationID string,
	step int,
	file plannedFile,
) error {
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}
	existing, err := os.ReadFile(file.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, file.content) {
		return fmt.Errorf(
			"published ticket target %q has different content",
			file.path,
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

func buildLifecycleProjection(
	manifest Manifest,
	layout Layout,
	activeProfile profile.Profile,
	render profile.RenderManifest,
	manifestBytes []byte,
	observedAt time.Time,
) (controlplane.TicketProjection, error) {
	projection, err := BuildProjection(
		manifest,
		layout,
		activeProfile,
		journal.Digest(manifestBytes),
		observedAt,
	)
	if err != nil {
		return controlplane.TicketProjection{}, err
	}
	rendered := make(map[string]profile.RenderedRecord, len(render.Artifacts))
	for _, record := range render.Artifacts {
		rendered[record.Role] = record
	}
	for _, artifact := range activeProfile.Artifacts {
		item := controlplane.ArtifactProjection{
			TicketID: manifest.ID,
			Role:     artifact.Role,
			Path: filepath.Join(
				layout.Root(),
				filepath.FromSlash(artifact.Path),
			),
			Authority:    string(artifact.Authority),
			SearchPolicy: string(artifact.Search),
		}
		if record, exists := rendered[artifact.Role]; exists {
			item.RenderedHash = record.GeneratedHash
		}
		if artifact.Kind == profile.ArtifactFile {
			content, readErr := os.ReadFile(item.Path)
			if readErr != nil {
				return controlplane.TicketProjection{}, fmt.Errorf(
					"read projected artifact %q: %w",
					artifact.Role,
					readErr,
				)
			}
			item.SourceHash = profile.HashContent(content)
		}
		projection.Artifacts = append(projection.Artifacts, item)
	}
	return projection, nil
}

func loadWorkspaceResources(root string) (workspaceResources, error) {
	layout, err := workspace.NewLayout(root)
	if err != nil {
		return workspaceResources{}, err
	}
	manifest, err := workspace.ReadManifest(layout.ManifestPath())
	if err != nil {
		return workspaceResources{}, err
	}
	if err := manifest.Validate(layout); err != nil {
		return workspaceResources{}, err
	}
	statePath, err := layout.ResolveRole(manifest.Roles.State)
	if err != nil {
		return workspaceResources{}, err
	}
	eventsDir, err := layout.ResolveRole(manifest.Roles.Events)
	if err != nil {
		return workspaceResources{}, err
	}
	return workspaceResources{
		layout:    layout,
		manifest:  manifest,
		statePath: statePath,
		eventsDir: eventsDir,
	}, nil
}

func createEffects(
	bundle createBundle,
	resources workspaceResources,
	status capability.EffectStatus,
) []capability.Effect {
	effects := make([]capability.Effect, 0, len(bundle.files)+len(bundle.directories)+1)
	for _, directory := range bundle.directories {
		effects = append(effects, capability.Effect{
			Action: "create-directory",
			Target: directory,
			Status: status,
		})
	}
	for _, file := range bundle.files {
		effects = append(effects, capability.Effect{
			Action: "create",
			Target: file.path,
			Status: status,
		})
	}
	effects = append(effects, capability.Effect{
		Action: "project",
		Target: resources.statePath,
		Status: status,
	})
	return effects
}

func encodeRenderManifest(value profile.RenderManifest) ([]byte, error) {
	var buffer bytes.Buffer
	if err := profile.EncodeRenderManifest(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func encodeTombstones(value profile.TombstoneManifest) ([]byte, error) {
	var buffer bytes.Buffer
	if err := profile.EncodeTombstones(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

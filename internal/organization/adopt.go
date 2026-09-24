package organization

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type AdoptRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	Path          string `json:"path"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Parent        string `json:"parent,omitempty"`
	Owner         string `json:"owner,omitempty"`
	Description   string `json:"description,omitempty"`
	Trust         string `json:"trust,omitempty"`
	Profile       string `json:"profile,omitempty"`
	ActorType     string `json:"actor_type,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	Tool          string `json:"tool,omitempty"`
	Apply         bool   `json:"apply"`
}

type adoptInspection struct {
	workspaceLayout workspace.Layout
	workspaceID     string
	source          string
	layout          Layout
	parentID        string
	agentsContent   []byte
	conflict        string
}

func (service *Service) Adopt(
	ctx context.Context,
	request AdoptRequest,
) (capability.Result[MutationData], error) {
	if err := validateAdoptRequest(request); err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	inspection, err := inspectAdopt(ctx, request)
	if err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	previewID := service.newID()
	if previewID == "" {
		return emptyMutationResult(AdoptDescriptor), errors.New(
			"organization id generator returned an empty id",
		)
	}
	previewManifest := adoptedManifest(
		request,
		inspection.parentID,
		previewID,
		"planned:"+previewID,
		service.now(),
	)
	data := MutationData{
		Organization: organizationData(
			inspection.workspaceID,
			inspection.layout,
			previewManifest,
		),
		PreviousPath: inspection.source,
	}
	if inspection.conflict != "" {
		return conflictMutationResult(
			AdoptDescriptor,
			data,
			inspection.conflict,
		), nil
	}
	effects := adoptEffects(
		inspection,
		capability.EffectPlanned,
	)
	if !request.Apply {
		return mutationResult(
			AdoptDescriptor,
			capability.OutcomePlanned,
			data,
			effects,
		), nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.workspaceLayout.Root(),
		filepath.Join(inspection.workspaceLayout.ControlDir(), "workspace.lock"),
	)
	if err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	defer unlock()

	inspection, err = inspectAdopt(ctx, request)
	if err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	if inspection.conflict != "" {
		data.Organization = organizationData(
			inspection.workspaceID,
			inspection.layout,
			previewManifest,
		)
		data.PreviousPath = inspection.source
		return conflictMutationResult(
			AdoptDescriptor,
			data,
			inspection.conflict,
		), nil
	}

	organizationID := service.newID()
	if organizationID == "" {
		return emptyMutationResult(AdoptDescriptor), errors.New(
			"organization id generator returned an empty id",
		)
	}
	operationID := service.newID()
	if operationID == "" {
		return emptyMutationResult(AdoptDescriptor), errors.New(
			"organization operation id generator returned an empty id",
		)
	}
	manifest := adoptedManifest(
		request,
		inspection.parentID,
		organizationID,
		operationID,
		service.now(),
	)
	if err := manifest.Validate(inspection.layout); err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	manifestContent, err := encodeManifest(manifest)
	if err != nil {
		return emptyMutationResult(AdoptDescriptor), err
	}
	data = MutationData{
		Organization: organizationData(
			inspection.workspaceID,
			inspection.layout,
			manifest,
		),
		OperationID:  operationID,
		PreviousPath: inspection.source,
	}
	effects = adoptEffects(inspection, capability.EffectPlanned)

	journalStore, err := journal.NewStore(
		inspection.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return mutationResult(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	steps, fileStep := adoptJournalSteps(
		inspection,
		manifestContent,
	)
	if _, err := journalStore.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: AdoptDescriptor.Capability + ":" + organizationID,
		Kind:           AdoptDescriptor.Capability,
		Steps:          steps,
	}); err != nil {
		return mutationResult(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return mutationResult(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return mutationResult(
				AdoptDescriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}

	if inspection.source != inspection.layout.Root() {
		if _, err := journalStore.Append(operationID, journal.EventInput{
			Phase: journal.PhaseStepApplying,
			Step:  1,
		}); err != nil {
			return service.failMutation(
				journalStore,
				AdoptDescriptor,
				data,
				effects,
				err,
			)
		}
		if err := atomicfile.MkdirAllWithin(
			inspection.workspaceLayout.Root(),
			filepath.Dir(inspection.layout.Root()),
			0o755,
		); err != nil {
			return service.failMutation(
				journalStore,
				AdoptDescriptor,
				data,
				effects,
				fmt.Errorf("create organization parent directory: %w", err),
			)
		}
		if err := atomicfile.RenameWithin(
			inspection.workspaceLayout.Root(),
			inspection.source,
			inspection.layout.Root(),
		); err != nil {
			return service.failMutation(
				journalStore,
				AdoptDescriptor,
				data,
				effects,
				fmt.Errorf("move adopted organization directory: %w", err),
			)
		}
		if _, err := journalStore.Append(operationID, journal.EventInput{
			Phase: journal.PhaseStepApplied,
			Step:  1,
		}); err != nil {
			return service.failMutation(
				journalStore,
				AdoptDescriptor,
				data,
				effects,
				err,
			)
		}
	}
	if err := atomicfile.MkdirAllWithin(
		inspection.workspaceLayout.Root(),
		inspection.layout.ControlDir(),
		0o755,
	); err != nil {
		return service.failMutation(
			journalStore,
			AdoptDescriptor,
			data,
			effects,
			fmt.Errorf("create organization control directory: %w", err),
		)
	}
	for _, file := range []struct {
		path    string
		content []byte
	}{
		{inspection.layout.ConfigPath(), []byte(defaultConfig)},
		{inspection.layout.AgentsPath(), inspection.agentsContent},
		{inspection.layout.ManifestPath(), manifestContent},
	} {
		if err := applyJournaledFile(
			inspection.workspaceLayout.Root(),
			journalStore,
			operationID,
			fileStep,
			file.path,
			file.content,
		); err != nil {
			return service.failMutation(
				journalStore,
				AdoptDescriptor,
				data,
				effects,
				err,
			)
		}
		fileStep++
	}

	state, err := controlplane.Open(
		ctx,
		inspection.workspaceLayout.StatePath(),
	)
	if err != nil {
		return service.failMutation(
			journalStore,
			AdoptDescriptor,
			data,
			effects,
			err,
		)
	}
	stateClosed := false
	defer func() {
		if !stateClosed {
			_ = state.Close()
		}
	}()
	if err := state.ObserveOrganization(
		ctx,
		organizationProjection(
			inspection.layout,
			manifest,
			manifestContent,
			service.now(),
		),
	); err != nil {
		return service.failMutation(
			journalStore,
			AdoptDescriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			journalStore,
			AdoptDescriptor,
			data,
			effects,
			err,
		)
	}
	stateClosed = true
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return mutationResult(
			AdoptDescriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	return mutationResult(
		AdoptDescriptor,
		capability.OutcomeApplied,
		data,
		adoptEffects(inspection, capability.EffectApplied),
	), nil
}

func inspectAdopt(
	ctx context.Context,
	request AdoptRequest,
) (adoptInspection, error) {
	workspaceLayout, err := workspace.NewLayout(request.WorkspaceRoot)
	if err != nil {
		return adoptInspection{}, err
	}
	workspaceManifest, err := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if err != nil {
		return adoptInspection{}, err
	}
	if err := workspaceManifest.Validate(workspaceLayout); err != nil {
		return adoptInspection{}, err
	}
	if _, err := workspaceLayout.InspectRole(
		workspaceManifest.Roles.Organizations,
	); err != nil {
		return adoptInspection{}, fmt.Errorf(
			"inspect workspace organizations role: %w",
			err,
		)
	}
	layout, err := NewLayoutForRole(
		workspaceLayout.Root(),
		workspaceManifest.Roles.Organizations,
		request.Slug,
	)
	if err != nil {
		return adoptInspection{}, err
	}
	source, err := filepath.Abs(request.Path)
	if err != nil {
		return adoptInspection{}, fmt.Errorf(
			"resolve organization adoption path %q: %w",
			request.Path,
			err,
		)
	}
	source = filepath.Clean(source)
	sourceInspection, err := workspace.InspectPath(
		workspaceLayout.Root(),
		source,
	)
	if err != nil {
		return adoptInspection{}, fmt.Errorf(
			"inspect organization adoption source containment: %w",
			err,
		)
	}
	destinationInspection, err := workspace.InspectPath(
		workspaceLayout.Root(),
		layout.Root(),
	)
	if err != nil {
		return adoptInspection{}, fmt.Errorf(
			"inspect organization adoption destination %q: %w",
			layout.Root(),
			err,
		)
	}
	inspection := adoptInspection{
		workspaceLayout: workspaceLayout,
		workspaceID:     workspaceManifest.ID,
		source:          source,
		layout:          layout,
	}

	if sourceInspection.State == workspace.RoleInspectionMissingTail {
		inspection.conflict = fmt.Sprintf(
			"organization adoption source %q does not exist",
			source,
		)
		return inspection, nil
	}
	info, err := statOrganizationTargetWithin(workspaceLayout.Root(), source)
	switch {
	case err != nil:
		return adoptInspection{}, fmt.Errorf(
			"inspect organization adoption source %q: %w",
			source,
			err,
		)
	case !info.IsDir():
		inspection.conflict = fmt.Sprintf(
			"organization adoption source %q is not a directory",
			source,
		)
		return inspection, nil
	}
	if unsafeAdoptionRelationship(
		workspaceLayout.Root(),
		source,
		layout.Root(),
	) {
		inspection.conflict = fmt.Sprintf(
			"organization adoption source %q has an unsafe path relationship with workspace %q",
			source,
			workspaceLayout.Root(),
		)
		return inspection, nil
	}
	adoptionManifest := filepath.Join(source, ".aidb", "manifest.yaml")
	manifestInspection, err := workspace.InspectPath(
		workspaceLayout.Root(),
		adoptionManifest,
	)
	if err != nil {
		return adoptInspection{}, fmt.Errorf(
			"inspect organization adoption manifest: %w",
			err,
		)
	}
	if manifestInspection.State == workspace.RoleInspectionContained {
		inspection.conflict = fmt.Sprintf(
			"organization adoption source %q already has a managed manifest",
			source,
		)
		return inspection, nil
	}
	if source != layout.Root() {
		if destinationInspection.State == workspace.RoleInspectionContained {
			inspection.conflict = fmt.Sprintf(
				"organization adoption destination %q already exists",
				layout.Root(),
			)
			return inspection, nil
		}
	}

	agents, err := AgentsPointer(layout)
	if err != nil {
		return adoptInspection{}, err
	}
	inspection.agentsContent = []byte(agents)
	for _, target := range []struct {
		path    string
		content []byte
		name    string
	}{
		{
			path:    filepath.Join(source, ".aidb", "config.yaml"),
			content: []byte(defaultConfig),
			name:    "configuration",
		},
		{
			path:    filepath.Join(source, "AGENTS.md"),
			content: inspection.agentsContent,
			name:    "AGENTS.md",
		},
	} {
		conflict, err := adoptionFileConflict(
			workspaceLayout.Root(),
			target.path,
			target.content,
			target.name,
		)
		if err != nil {
			return adoptInspection{}, err
		}
		if conflict != "" {
			inspection.conflict = conflict
			return inspection, nil
		}
	}

	state, err := controlplane.OpenReadOnly(ctx, workspaceLayout.StatePath())
	if err != nil {
		return adoptInspection{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	if request.Parent != "" {
		parent, err := state.Organization(ctx, request.Parent)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				inspection.conflict = fmt.Sprintf(
					"parent organization %q is not registered",
					request.Parent,
				)
				return inspection, nil
			}
			return adoptInspection{}, err
		}
		if parent.Status == controlplane.EntityStatusArchived {
			inspection.conflict = fmt.Sprintf(
				"parent organization %q is archived",
				request.Parent,
			)
			return inspection, nil
		}
		inspection.parentID = parent.ID
	}
	for _, selector := range []string{request.Slug, layout.Root()} {
		projection, err := state.Organization(ctx, selector)
		if err == nil {
			inspection.conflict = fmt.Sprintf(
				"organization selector %q is already registered to %q",
				selector,
				projection.ID,
			)
			return inspection, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return adoptInspection{}, err
		}
	}
	return inspection, nil
}

func adoptedManifest(
	request AdoptRequest,
	parentID string,
	organizationID string,
	operationID string,
	createdAt time.Time,
) Manifest {
	manifest := NewManifest(
		organizationID,
		request.Slug,
		request.Name,
		createdAt,
		mutationProvenance(
			operationID,
			request.ActorType,
			request.ActorID,
			request.Tool,
		),
	)
	manifest.ParentID = parentID
	manifest.Owner = request.Owner
	manifest.Description = request.Description
	manifest.Trust = request.Trust
	manifest.Profile = request.Profile
	return manifest
}

func adoptionFileConflict(
	root string,
	path string,
	expected []byte,
	name string,
) (string, error) {
	inspection, err := workspace.InspectPath(root, path)
	if err != nil {
		return "", fmt.Errorf(
			"inspect organization adoption %s %q: %w",
			name,
			path,
			err,
		)
	}
	if inspection.State == workspace.RoleInspectionMissingTail {
		return "", nil
	}
	content, err := readOrganizationTargetWithin(root, path)
	switch {
	case err == nil && bytes.Equal(content, expected):
		return "", nil
	case err == nil:
		return fmt.Sprintf(
			"organization adoption refuses to overwrite authored %s at %q",
			name,
			path,
		), nil
	default:
		return "", fmt.Errorf(
			"inspect organization adoption %s %q: %w",
			name,
			path,
			err,
		)
	}
}

func unsafeAdoptionRelationship(
	workspaceRoot string,
	source string,
	target string,
) bool {
	if source == workspaceRoot {
		return true
	}
	if pathContains(source, workspaceRoot) {
		return true
	}
	if source != target &&
		(pathContains(source, target) || pathContains(target, source)) {
		return true
	}
	return false
}

func pathContains(parent string, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative != "." &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func adoptJournalSteps(
	inspection adoptInspection,
	manifestContent []byte,
) ([]journal.Step, int) {
	steps := make([]journal.Step, 0, 4)
	ordinal := 1
	if inspection.source != inspection.layout.Root() {
		steps = append(steps, journal.Step{
			Ordinal: ordinal,
			Action:  "move",
			Target:  inspection.source,
		})
		ordinal++
	}
	fileStep := ordinal
	for _, file := range []struct {
		path    string
		content []byte
	}{
		{inspection.layout.ConfigPath(), []byte(defaultConfig)},
		{inspection.layout.AgentsPath(), inspection.agentsContent},
		{inspection.layout.ManifestPath(), manifestContent},
	} {
		steps = append(steps, journal.Step{
			Ordinal:   ordinal,
			Action:    "create",
			Target:    file.path,
			AfterHash: journal.Digest(file.content),
		})
		ordinal++
	}
	return steps, fileStep
}

func adoptEffects(
	inspection adoptInspection,
	status capability.EffectStatus,
) []capability.Effect {
	effects := make([]capability.Effect, 0, 6)
	if inspection.source != inspection.layout.Root() {
		effects = append(effects, capability.Effect{
			Action: "move",
			Target: inspection.layout.Root(),
			Status: status,
		})
	}
	effects = append(
		effects,
		capability.Effect{
			Action: "create",
			Target: inspection.layout.ControlDir(),
			Status: status,
		},
		capability.Effect{
			Action: "create",
			Target: inspection.layout.ConfigPath(),
			Status: status,
		},
		capability.Effect{
			Action: "create",
			Target: inspection.layout.AgentsPath(),
			Status: status,
		},
		capability.Effect{
			Action: "create",
			Target: inspection.layout.ManifestPath(),
			Status: status,
		},
		capability.Effect{
			Action: "project",
			Target: inspection.workspaceLayout.StatePath(),
			Status: status,
		},
	)
	return effects
}

func validateAdoptRequest(request AdoptRequest) error {
	if request.Path == "" {
		return errors.New("organization adoption path is required")
	}
	if request.Name == "" {
		return errors.New("organization display name is required")
	}
	if err := validateMutationActor(request.ActorType, request.Tool); err != nil {
		return err
	}
	return ValidateSlug(request.Slug)
}

package organization

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

const defaultConfig = "schema_version: aidb.config/v1\n"

type InitializeRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
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

type InitializeData struct {
	WorkspaceID    string `json:"workspace_id"`
	OrganizationID string `json:"organization_id"`
	OperationID    string `json:"operation_id"`
	ParentID       string `json:"parent_id,omitempty"`
	Slug           string `json:"slug"`
	Path           string `json:"path"`
	Manifest       string `json:"manifest"`
	Config         string `json:"config"`
	Agents         string `json:"agents"`
}

type initializeInspection struct {
	workspaceLayout workspace.Layout
	workspaceID     string
	layout          Layout
	parentID        string
	existing        *Manifest
	conflict        string
}

func (service *Service) Initialize(
	ctx context.Context,
	request InitializeRequest,
) (capability.Result[InitializeData], error) {
	if err := validateInitializeRequest(request); err != nil {
		return emptyInitializeResult(), err
	}

	inspection, err := inspectInitialize(ctx, request)
	if err != nil {
		return emptyInitializeResult(), err
	}
	if inspection.conflict != "" {
		return conflictInitializeResult(
			initializeData(
				inspection,
				existingOrganizationID(inspection.existing),
			),
			inspection.conflict,
		), nil
	}
	if inspection.existing != nil {
		return existingInitializeResult(inspection), nil
	}

	organizationID := service.newID()
	if organizationID == "" {
		return emptyInitializeResult(), errors.New(
			"organization id generator returned an empty id",
		)
	}
	data := initializeData(inspection, organizationID)
	effects := initializeEffects(
		inspection.workspaceLayout,
		inspection.layout,
		capability.EffectPlanned,
	)
	if !request.Apply {
		return capability.Result[InitializeData]{
			Capability: InitializeDescriptor.Capability,
			Version:    InitializeDescriptor.Version,
			Outcome:    capability.OutcomePlanned,
			Data:       data,
			Effects:    effects,
			Warnings:   []capability.Notice{},
			NextActions: []capability.Action{{
				Code:    "apply_organization_initialization",
				Message: "Run organization initialization with apply enabled.",
			}},
			Recovery: capability.Recovery{Guidance: []string{}},
		}, nil
	}

	unlock, err := acquireWorkspaceLock(
		inspection.workspaceLayout.Root(),
		filepath.Join(inspection.workspaceLayout.ControlDir(), "workspace.lock"),
	)
	if err != nil {
		return emptyInitializeResult(), err
	}
	defer unlock()

	inspection, err = inspectInitialize(ctx, request)
	if err != nil {
		return emptyInitializeResult(), err
	}
	if inspection.conflict != "" {
		return conflictInitializeResult(
			initializeData(
				inspection,
				existingOrganizationID(inspection.existing),
			),
			inspection.conflict,
		), nil
	}
	if inspection.existing != nil {
		return existingInitializeResult(inspection), nil
	}
	data = initializeData(inspection, organizationID)

	operationID := service.newID()
	if operationID == "" {
		return emptyInitializeResult(), errors.New(
			"organization operation id generator returned an empty id",
		)
	}
	data.OperationID = operationID

	createdAt := service.now().UTC()
	manifest := NewManifest(
		organizationID,
		request.Slug,
		request.Name,
		createdAt,
		Provenance{
			OperationID: operationID,
			ActorType:   request.ActorType,
			ActorID:     request.ActorID,
			Tool:        request.Tool,
		},
	)
	manifest.ParentID = inspection.parentID
	manifest.Owner = request.Owner
	manifest.Description = request.Description
	manifest.Trust = request.Trust
	manifest.Profile = request.Profile
	if err := manifest.Validate(inspection.layout); err != nil {
		return emptyInitializeResult(), err
	}
	manifestContent, err := encodeManifest(manifest)
	if err != nil {
		return emptyInitializeResult(), err
	}
	agentsContent, err := AgentsPointer(inspection.layout)
	if err != nil {
		return emptyInitializeResult(), err
	}

	journalStore, err := journal.NewStore(
		inspection.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return emptyInitializeResult(), err
	}
	plan := journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: "organization.initialize:" + organizationID,
		Kind:           InitializeDescriptor.Capability,
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
	}
	if _, err := journalStore.Begin(plan); err != nil {
		return failedInitializeResult(data, effects), err
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedInitializeResult(data, effects), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return failedInitializeResult(data, effects), err
		}
	}

	if err := atomicfile.MkdirAllWithin(
		inspection.workspaceLayout.Root(),
		inspection.layout.ControlDir(),
		0o755,
	); err != nil {
		return service.failInitialize(
			journalStore,
			data,
			effects,
			fmt.Errorf("create organization control directory: %w", err),
		)
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
		if err := applyJournaledFile(
			inspection.workspaceLayout.Root(),
			journalStore,
			operationID,
			file.step,
			file.path,
			file.content,
		); err != nil {
			return service.failInitialize(journalStore, data, effects, err)
		}
	}

	state, err := controlplane.Open(ctx, inspection.workspaceLayout.StatePath())
	if err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	stateClosed := false
	defer func() {
		if !stateClosed {
			_ = state.Close()
		}
	}()
	if err := state.ObserveOrganization(
		ctx,
		controlplane.OrganizationProjection{
			ID:           organizationID,
			Slug:         request.Slug,
			Path:         inspection.layout.Root(),
			DisplayName:  request.Name,
			ParentID:     inspection.parentID,
			ManifestHash: journal.Digest(manifestContent),
			Status:       controlplane.EntityStatusActive,
			Aliases:      []string{},
			ObservedAt:   service.now().UTC(),
		},
	); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	if err := state.Close(); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	stateClosed = true

	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedInitializeResult(data, effects), err
	}

	return capability.Result[InitializeData]{
		Capability: InitializeDescriptor.Capability,
		Version:    InitializeDescriptor.Version,
		Outcome:    capability.OutcomeApplied,
		Data:       data,
		Effects: initializeEffects(
			inspection.workspaceLayout,
			inspection.layout,
			capability.EffectApplied,
		),
		Warnings: []capability.Notice{},
		NextActions: []capability.Action{{
			Code:    "validate_organization",
			Message: "Run organization validation.",
		}},
		Recovery: capability.Recovery{Guidance: []string{}},
	}, nil
}

func inspectInitialize(
	ctx context.Context,
	request InitializeRequest,
) (initializeInspection, error) {
	workspaceLayout, err := workspace.NewLayout(request.WorkspaceRoot)
	if err != nil {
		return initializeInspection{}, err
	}
	workspaceManifest, err := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if err != nil {
		return initializeInspection{}, err
	}
	if err := workspaceManifest.Validate(workspaceLayout); err != nil {
		return initializeInspection{}, err
	}
	if _, err := workspaceLayout.InspectRole(
		workspaceManifest.Roles.Organizations,
	); err != nil {
		return initializeInspection{}, fmt.Errorf(
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
		return initializeInspection{}, err
	}
	pathInspection, err := workspace.InspectPath(
		workspaceLayout.Root(),
		layout.Root(),
	)
	if err != nil {
		return initializeInspection{}, fmt.Errorf(
			"inspect canonical organization path %q: %w",
			layout.Root(),
			err,
		)
	}
	inspection := initializeInspection{
		workspaceLayout: workspaceLayout,
		workspaceID:     workspaceManifest.ID,
		layout:          layout,
	}

	state, err := controlplane.OpenReadOnly(ctx, workspaceLayout.StatePath())
	if err != nil {
		return initializeInspection{}, err
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
			return initializeInspection{}, err
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

	existingContent, err := readOrganizationTargetWithin(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err == nil {
		existing, decodeErr := DecodeManifest(bytes.NewReader(existingContent))
		if decodeErr != nil {
			return initializeInspection{}, fmt.Errorf(
				"read organization manifest %q: %w",
				layout.ManifestPath(),
				decodeErr,
			)
		}
		if err := existing.Validate(layout); err != nil {
			return initializeInspection{}, err
		}
		inspection.existing = &existing
		if conflict := existingManifestConflict(
			existing,
			request,
			inspection.parentID,
		); conflict != "" {
			inspection.conflict = conflict
		}
		return inspection, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return initializeInspection{}, err
	}

	if pathInspection.State == workspace.RoleInspectionContained {
		info, statErr := statOrganizationTargetWithin(
			workspaceLayout.Root(),
			layout.Root(),
		)
		if statErr != nil {
			return initializeInspection{}, fmt.Errorf(
				"inspect organization path %q: %w",
				layout.Root(),
				statErr,
			)
		}
		if !info.IsDir() {
			inspection.conflict = fmt.Sprintf(
				"organization path %q is not a directory",
				layout.Root(),
			)
		} else {
			inspection.conflict = fmt.Sprintf(
				"organization path %q exists without a managed manifest; use organization adoption",
				layout.Root(),
			)
		}
		return inspection, nil
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
			return initializeInspection{}, err
		}
	}
	return inspection, nil
}

func validateInitializeRequest(request InitializeRequest) error {
	if request.Name == "" {
		return errors.New("organization display name is required")
	}
	if request.ActorType == "" {
		return errors.New("organization actor type is required")
	}
	if request.Tool == "" {
		return errors.New("organization tool is required")
	}
	return ValidateSlug(request.Slug)
}

func existingManifestConflict(
	existing Manifest,
	request InitializeRequest,
	parentID string,
) string {
	if existing.Status == StatusArchived {
		return fmt.Sprintf("organization %q is archived", existing.Slug)
	}
	if existing.DisplayName != request.Name {
		return fmt.Sprintf(
			"organization display name is %q, requested %q",
			existing.DisplayName,
			request.Name,
		)
	}
	checks := []struct {
		name      string
		requested string
		existing  string
	}{
		{"parent", parentID, existing.ParentID},
		{"owner", request.Owner, existing.Owner},
		{"description", request.Description, existing.Description},
		{"trust", request.Trust, existing.Trust},
		{"profile", request.Profile, existing.Profile},
	}
	for _, check := range checks {
		if check.requested != "" && check.requested != check.existing {
			return fmt.Sprintf(
				"organization %s is %q, requested %q",
				check.name,
				check.existing,
				check.requested,
			)
		}
	}
	return ""
}

func existingInitializeResult(
	inspection initializeInspection,
) capability.Result[InitializeData] {
	data := initializeData(inspection, inspection.existing.ID)
	data.ParentID = inspection.existing.ParentID
	return capability.Result[InitializeData]{
		Capability: InitializeDescriptor.Capability,
		Version:    InitializeDescriptor.Version,
		Outcome:    capability.OutcomeUnchanged,
		Data:       data,
		Effects: initializeEffects(
			inspection.workspaceLayout,
			inspection.layout,
			capability.EffectSkipped,
		),
		Warnings: []capability.Notice{},
		NextActions: []capability.Action{{
			Code:    "validate_organization",
			Message: "Run organization validation.",
		}},
		Recovery: capability.Recovery{Guidance: []string{}},
	}
}

func conflictInitializeResult(
	data InitializeData,
	message string,
) capability.Result[InitializeData] {
	return capability.Result[InitializeData]{
		Capability: InitializeDescriptor.Capability,
		Version:    InitializeDescriptor.Version,
		Outcome:    capability.OutcomeConflict,
		Data:       data,
		Effects:    []capability.Effect{},
		Warnings: []capability.Notice{{
			Code:    "organization_conflict",
			Message: message,
		}},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func failedInitializeResult(
	data InitializeData,
	effects []capability.Effect,
) capability.Result[InitializeData] {
	return capability.Result[InitializeData]{
		Capability:  InitializeDescriptor.Capability,
		Version:     InitializeDescriptor.Version,
		Outcome:     capability.OutcomeFailed,
		Data:        data,
		Effects:     effects,
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery: capability.Recovery{
			Required: true,
			Guidance: []string{
				"Run adb doctor before retrying organization initialization.",
			},
		},
	}
}

func emptyInitializeResult() capability.Result[InitializeData] {
	return capability.Result[InitializeData]{
		Capability:  InitializeDescriptor.Capability,
		Version:     InitializeDescriptor.Version,
		Effects:     []capability.Effect{},
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

// initializeData builds the result payload from an inspection. It deliberately
// takes no operation id: an operation exists only on the apply path, which sets
// data.OperationID itself once journal.Store has minted one, so every caller
// here starts from the empty value.
func initializeData(
	inspection initializeInspection,
	organizationID string,
) InitializeData {
	return InitializeData{
		WorkspaceID:    inspection.workspaceID,
		OrganizationID: organizationID,
		ParentID:       inspection.parentID,
		Slug:           inspection.layout.Slug(),
		Path:           inspection.layout.Root(),
		Manifest:       inspection.layout.ManifestPath(),
		Config:         inspection.layout.ConfigPath(),
		Agents:         inspection.layout.AgentsPath(),
	}
}

func initializeEffects(
	workspaceLayout workspace.Layout,
	layout Layout,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{Action: "create", Target: layout.Root(), Status: status},
		{Action: "create", Target: layout.ControlDir(), Status: status},
		{Action: "create", Target: layout.ConfigPath(), Status: status},
		{Action: "create", Target: layout.AgentsPath(), Status: status},
		{Action: "create", Target: layout.ManifestPath(), Status: status},
		{Action: "project", Target: workspaceLayout.StatePath(), Status: status},
	}
}

func existingOrganizationID(manifest *Manifest) string {
	if manifest == nil {
		return ""
	}
	return manifest.ID
}

func (service *Service) failInitialize(
	journalStore *journal.Store,
	data InitializeData,
	effects []capability.Effect,
	cause error,
) (capability.Result[InitializeData], error) {
	if _, appendErr := journalStore.Append(
		data.OperationID,
		journal.EventInput{
			Phase: journal.PhaseFailed,
			Error: &journal.ErrorInfo{
				Code:    "organization_initialize_failed",
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
			"%w (record organization initialization failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return failedInitializeResult(data, effects), cause
}

func applyJournaledFile(
	root string,
	journalStore *journal.Store,
	operationID string,
	step int,
	path string,
	content []byte,
) error {
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}

	existing, err := readOrganizationTargetWithin(root, path)
	switch {
	case err == nil && bytes.Equal(existing, content):
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("read organization target %q: %w", path, err)
	default:
		return fmt.Errorf(
			"organization target %q contains different content",
			path,
		)
	}
	if err := atomicfile.WriteWithin(
		root,
		path,
		atomicfile.Options{Mode: 0o644},
		func(writer io.Writer) error {
			_, writeErr := writer.Write(content)
			return writeErr
		},
	); err != nil {
		return err
	}

	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step,
	}); err != nil {
		return err
	}
	return nil
}

func acquireWorkspaceLock(root string, path string) (func(), error) {
	if _, err := workspace.InspectPath(root, path); err != nil {
		return nil, err
	}
	openedRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open workspace root %q: %w", root, err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		_ = openedRoot.Close()
		return nil, fmt.Errorf(
			"resolve workspace lock %q within root %q: %w",
			path,
			root,
			err,
		)
	}
	file, err := openedRoot.OpenFile(relative, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		closeErr := openedRoot.Close()
		return nil, errors.Join(
			fmt.Errorf("open workspace lock %q: %w", path, err),
			closeErr,
		)
	}
	unlock, err := lockfile.Lock(file)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("acquire workspace lock %q: %w", path, err),
			file.Close(),
			openedRoot.Close(),
		)
	}
	return func() {
		unlock()
		_ = file.Close()
		_ = openedRoot.Close()
	}, nil
}

func encodeManifest(manifest Manifest) ([]byte, error) {
	var buffer bytes.Buffer
	if err := EncodeManifest(&buffer, manifest); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

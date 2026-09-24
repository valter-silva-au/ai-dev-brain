package foundation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

const defaultConfig = "schema_version: aidb.config/v1\n"

type InitializeRequest struct {
	Root  string `json:"root"`
	Name  string `json:"name"`
	Apply bool   `json:"apply"`
}

type InitializeData struct {
	WorkspaceID string `json:"workspace_id"`
	OperationID string `json:"operation_id"`
	Root        string `json:"root"`
	Manifest    string `json:"manifest"`
	Config      string `json:"config"`
	State       string `json:"state"`
}

type Options struct {
	Clock       func() time.Time
	IDGenerator func() string
	AfterPlan   func() error
}

type Service struct {
	now       func() time.Time
	newID     func() string
	afterPlan func() error
}

func NewService(options Options) (*Service, error) {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.IDGenerator == nil {
		options.IDGenerator = uuid.NewString
	}

	return &Service{
		now:       options.Clock,
		newID:     options.IDGenerator,
		afterPlan: options.AfterPlan,
	}, nil
}

func (service *Service) Initialize(
	ctx context.Context,
	request InitializeRequest,
) (capability.Result[InitializeData], error) {
	layout, err := workspace.NewLayout(request.Root)
	if err != nil {
		return emptyInitializeResult(), err
	}
	if request.Name == "" {
		return emptyInitializeResult(), errors.New("workspace name is required")
	}

	existing, found, err := existingManifest(layout)
	if err != nil {
		return emptyInitializeResult(), err
	}
	if found {
		return existingInitializeResult(layout, existing, request.Name), nil
	}

	workspaceID := service.newID()
	if workspaceID == "" {
		return emptyInitializeResult(), errors.New(
			"workspace id generator returned an empty id",
		)
	}
	manifest := workspace.NewManifest(
		workspaceID,
		request.Name,
		service.now().UTC(),
	)
	manifestContent, err := encodeManifest(manifest)
	if err != nil {
		return emptyInitializeResult(), err
	}

	data := initializeData(layout, workspaceID, "")
	effects := initializeEffects(layout, capability.EffectPlanned)
	if !request.Apply {
		return capability.Result[InitializeData]{
			Capability:  InitializeDescriptor.Capability,
			Version:     InitializeDescriptor.Version,
			Outcome:     capability.OutcomePlanned,
			Data:        data,
			Effects:     effects,
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{applyInitializeAction()},
			Recovery:    capability.Recovery{Guidance: []string{}},
		}, nil
	}

	if err := ensureRootDirectory(layout.Root()); err != nil {
		return emptyInitializeResult(), err
	}
	if err := os.MkdirAll(layout.EventsDir(), 0o755); err != nil {
		return emptyInitializeResult(), fmt.Errorf(
			"create workspace journal directory: %w",
			err,
		)
	}

	unlock, err := acquireWorkspaceLock(
		filepath.Join(layout.ControlDir(), "workspace.lock"),
	)
	if err != nil {
		return emptyInitializeResult(), err
	}
	defer unlock()

	existing, found, err = existingManifest(layout)
	if err != nil {
		return emptyInitializeResult(), err
	}
	if found {
		return existingInitializeResult(layout, existing, request.Name), nil
	}
	if err := ensureConfigCompatible(layout.ConfigPath()); err != nil {
		return conflictInitializeResult(layout, workspaceID, err.Error()), nil
	}

	operationID := service.newID()
	if operationID == "" {
		return emptyInitializeResult(), errors.New(
			"operation id generator returned an empty id",
		)
	}
	data.OperationID = operationID

	journalStore, err := journal.NewStore(
		layout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return emptyInitializeResult(), err
	}
	plan := journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: "workspace.initialize:" + workspaceID,
		Kind:           InitializeDescriptor.Capability,
		Steps: []journal.Step{
			{
				Ordinal:   1,
				Action:    "create",
				Target:    layout.ConfigPath(),
				AfterHash: journal.Digest([]byte(defaultConfig)),
			},
			{
				Ordinal:   2,
				Action:    "create",
				Target:    layout.ManifestPath(),
				AfterHash: journal.Digest(manifestContent),
			},
		},
	}
	if _, err := journalStore.Begin(plan); err != nil {
		return failedInitializeResult(data, effects, operationID), err
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return failedInitializeResult(data, effects, operationID), err
	}

	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return failedInitializeResult(data, effects, operationID), err
		}
	}

	if err := os.MkdirAll(layout.CacheDir(), 0o755); err != nil {
		return service.failInitialize(
			journalStore,
			data,
			effects,
			fmt.Errorf("create workspace cache directory: %w", err),
		)
	}
	if err := os.MkdirAll(layout.OrganizationsDir(), 0o755); err != nil {
		return service.failInitialize(
			journalStore,
			data,
			effects,
			fmt.Errorf("create organizations directory: %w", err),
		)
	}

	if err := applyJournaledFile(
		journalStore,
		operationID,
		1,
		layout.ConfigPath(),
		[]byte(defaultConfig),
	); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}

	stateStore, err := controlplane.Open(ctx, layout.StatePath())
	if err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	stateClosed := false
	defer func() {
		if !stateClosed {
			_ = stateStore.Close()
		}
	}()

	if err := applyJournaledFile(
		journalStore,
		operationID,
		2,
		layout.ManifestPath(),
		manifestContent,
	); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	if err := stateStore.ObserveWorkspace(
		ctx,
		controlplane.WorkspaceProjection{
			ID:           workspaceID,
			Root:         layout.Root(),
			ManifestHash: journal.Digest(manifestContent),
			ObservedAt:   service.now().UTC(),
		},
	); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	if err := stateStore.Close(); err != nil {
		return service.failInitialize(journalStore, data, effects, err)
	}
	stateClosed = true

	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseCommitted,
	}); err != nil {
		return failedInitializeResult(data, effects, operationID), err
	}

	return capability.Result[InitializeData]{
		Capability:  InitializeDescriptor.Capability,
		Version:     InitializeDescriptor.Version,
		Outcome:     capability.OutcomeApplied,
		Data:        data,
		Effects:     initializeEffects(layout, capability.EffectApplied),
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{runDoctorAction()},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}, nil
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
				Code:    "initialize_failed",
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
			"%w (record initialization failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return failedInitializeResult(
		data,
		effects,
		data.OperationID,
	), cause
}

func applyJournaledFile(
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

	existing, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(existing, content):
	case errors.Is(err, os.ErrNotExist):
		if err := atomicfile.Write(
			path,
			atomicfile.Options{Mode: 0o644},
			func(writer io.Writer) error {
				_, writeErr := writer.Write(content)
				return writeErr
			},
		); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("read initialization target %q: %w", path, err)
	default:
		return fmt.Errorf("initialization target %q contains different content", path)
	}

	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step,
	}); err != nil {
		return err
	}
	return nil
}

func existingManifest(
	layout workspace.Layout,
) (workspace.Manifest, bool, error) {
	manifest, err := workspace.ReadManifest(layout.ManifestPath())
	if err == nil {
		if validateErr := manifest.Validate(layout); validateErr != nil {
			return workspace.Manifest{}, false, validateErr
		}
		return manifest, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return workspace.Manifest{}, false, nil
	}
	return workspace.Manifest{}, false, err
}

func existingInitializeResult(
	layout workspace.Layout,
	manifest workspace.Manifest,
	requestedName string,
) capability.Result[InitializeData] {
	if manifest.Name != requestedName {
		return conflictInitializeResult(
			layout,
			manifest.ID,
			fmt.Sprintf(
				"workspace name is %q, requested %q",
				manifest.Name,
				requestedName,
			),
		)
	}

	return capability.Result[InitializeData]{
		Capability: InitializeDescriptor.Capability,
		Version:    InitializeDescriptor.Version,
		Outcome:    capability.OutcomeUnchanged,
		Data:       initializeData(layout, manifest.ID, ""),
		Effects:    initializeEffects(layout, capability.EffectSkipped),
		Warnings:   []capability.Notice{},
		NextActions: []capability.Action{
			runDoctorAction(),
		},
		Recovery: capability.Recovery{Guidance: []string{}},
	}
}

func conflictInitializeResult(
	layout workspace.Layout,
	workspaceID string,
	message string,
) capability.Result[InitializeData] {
	return capability.Result[InitializeData]{
		Capability: InitializeDescriptor.Capability,
		Version:    InitializeDescriptor.Version,
		Outcome:    capability.OutcomeConflict,
		Data:       initializeData(layout, workspaceID, ""),
		Effects:    initializeEffects(layout, capability.EffectSkipped),
		Warnings: []capability.Notice{{
			Code:    "workspace_conflict",
			Message: message,
		}},
		NextActions: []capability.Action{},
		Recovery:    capability.Recovery{Guidance: []string{}},
	}
}

func failedInitializeResult(
	data InitializeData,
	effects []capability.Effect,
	operationID string,
) capability.Result[InitializeData] {
	data.OperationID = operationID
	return capability.Result[InitializeData]{
		Capability:  InitializeDescriptor.Capability,
		Version:     InitializeDescriptor.Version,
		Outcome:     capability.OutcomeFailed,
		Data:        data,
		Effects:     effects,
		Warnings:    []capability.Notice{},
		NextActions: []capability.Action{runDoctorAction()},
		Recovery: capability.Recovery{
			Required: true,
			Guidance: []string{
				"Run adb doctor before retrying initialization.",
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

func initializeData(
	layout workspace.Layout,
	workspaceID string,
	operationID string,
) InitializeData {
	return InitializeData{
		WorkspaceID: workspaceID,
		OperationID: operationID,
		Root:        layout.Root(),
		Manifest:    layout.ManifestPath(),
		Config:      layout.ConfigPath(),
		State:       layout.StatePath(),
	}
}

func initializeEffects(
	layout workspace.Layout,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{Action: "create", Target: layout.Root(), Status: status},
		{Action: "create", Target: layout.ControlDir(), Status: status},
		{Action: "create", Target: layout.EventsDir(), Status: status},
		{Action: "create", Target: layout.CacheDir(), Status: status},
		{Action: "create", Target: layout.OrganizationsDir(), Status: status},
		{Action: "create", Target: layout.ConfigPath(), Status: status},
		{Action: "create", Target: layout.StatePath(), Status: status},
		{Action: "create", Target: layout.ManifestPath(), Status: status},
	}
}

func ensureRootDirectory(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("workspace root %q is not a directory", path)
	case err == nil:
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect workspace root %q: %w", path, err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create workspace root %q: %w", path, err)
	}
	return nil
}

func ensureConfigCompatible(path string) error {
	content, err := os.ReadFile(path)
	switch {
	case err == nil && string(content) == defaultConfig:
		return nil
	case err == nil:
		return fmt.Errorf("workspace config %q already contains different content", path)
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("read workspace config %q: %w", path, err)
	}
}

func acquireWorkspaceLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock %q: %w", path, err)
	}
	unlock, err := lockfile.Lock(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire workspace lock %q: %w", path, err)
	}

	return func() {
		unlock()
		_ = file.Close()
	}, nil
}

func encodeManifest(manifest workspace.Manifest) ([]byte, error) {
	var buffer bytes.Buffer
	if err := workspace.EncodeManifest(&buffer, manifest); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func applyInitializeAction() capability.Action {
	return capability.Action{
		Code:    "apply_initialization",
		Message: "Run initialization with apply enabled.",
	}
}

func runDoctorAction() capability.Action {
	return capability.Action{
		Code:    "run_doctor",
		Message: "Run adb doctor.",
	}
}

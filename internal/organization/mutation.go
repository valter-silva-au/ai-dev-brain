package organization

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/atomicfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type MutationData struct {
	Organization Data   `json:"organization"`
	OperationID  string `json:"operation_id,omitempty"`
	PreviousPath string `json:"previous_path,omitempty"`
}

type managedOrganization struct {
	workspaceLayout   workspace.Layout
	workspaceID       string
	organizationsRole string
	projection        controlplane.OrganizationProjection
	layout            Layout
	manifest          Manifest
	manifestContent   []byte
	data              Data
}

func loadManagedOrganization(
	ctx context.Context,
	root string,
	selector string,
) (managedOrganization, error) {
	if selector == "" {
		return managedOrganization{}, errors.New(
			"organization selector is required",
		)
	}

	workspaceLayout, workspaceManifest, state, err := openReadOnlyWorkspace(
		ctx,
		root,
	)
	if err != nil {
		return managedOrganization{}, err
	}
	defer func() {
		_ = state.Close()
	}()

	projection, err := state.Organization(ctx, selector)
	if err != nil {
		return managedOrganization{}, err
	}
	data, manifest, err := readOrganizationData(
		workspaceLayout,
		workspaceManifest.ID,
		workspaceManifest.Roles.Organizations,
		projection,
	)
	if err != nil {
		return managedOrganization{}, err
	}
	layout, err := NewLayoutForRole(
		workspaceLayout.Root(),
		workspaceManifest.Roles.Organizations,
		manifest.Slug,
	)
	if err != nil {
		return managedOrganization{}, err
	}
	content, err := readOrganizationTargetWithin(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err != nil {
		return managedOrganization{}, fmt.Errorf(
			"read organization manifest content %q: %w",
			layout.ManifestPath(),
			err,
		)
	}
	return managedOrganization{
		workspaceLayout:   workspaceLayout,
		workspaceID:       workspaceManifest.ID,
		organizationsRole: workspaceManifest.Roles.Organizations,
		projection:        projection,
		layout:            layout,
		manifest:          manifest,
		manifestContent:   content,
		data:              data,
	}, nil
}

func organizationData(
	workspaceID string,
	layout Layout,
	manifest Manifest,
) Data {
	return Data{
		ID:           manifest.ID,
		WorkspaceID:  workspaceID,
		Slug:         manifest.Slug,
		DisplayName:  manifest.DisplayName,
		ParentID:     manifest.ParentID,
		Owner:        manifest.Owner,
		Description:  manifest.Description,
		Trust:        manifest.Trust,
		Profile:      manifest.Profile,
		Status:       manifest.Status,
		Path:         layout.Root(),
		Aliases:      append([]string(nil), manifest.Aliases...),
		Roles:        manifest.Roles,
		Provenance:   manifest.Provenance,
		LastMutation: manifest.LastMutation,
	}
}

func organizationProjection(
	layout Layout,
	manifest Manifest,
	content []byte,
	observedAt time.Time,
) controlplane.OrganizationProjection {
	return controlplane.OrganizationProjection{
		ID:           manifest.ID,
		Slug:         manifest.Slug,
		Path:         layout.Root(),
		DisplayName:  manifest.DisplayName,
		ParentID:     manifest.ParentID,
		ManifestHash: journal.Digest(content),
		Status:       controlplane.EntityStatus(manifest.Status),
		Aliases:      append([]string(nil), manifest.Aliases...),
		ObservedAt:   observedAt.UTC(),
	}
}

func validateMutationActor(actorType string, tool string) error {
	if actorType == "" {
		return errors.New("organization actor type is required")
	}
	if tool == "" {
		return errors.New("organization tool is required")
	}
	return nil
}

func mutationProvenance(
	operationID string,
	actorType string,
	actorID string,
	tool string,
) Provenance {
	return Provenance{
		OperationID: operationID,
		ActorType:   actorType,
		ActorID:     actorID,
		Tool:        tool,
	}
}

func mutationEffects(
	workspaceLayout workspace.Layout,
	manifestPath string,
	status capability.EffectStatus,
) []capability.Effect {
	return []capability.Effect{
		{
			Action: "replace",
			Target: manifestPath,
			Status: status,
		},
		{
			Action: "project",
			Target: workspaceLayout.StatePath(),
			Status: status,
		},
	}
}

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
	switch outcome {
	case capability.OutcomePlanned:
		result.NextActions = []capability.Action{{
			Code:    "apply_" + descriptor.Capability,
			Message: "Run the organization operation with apply enabled.",
		}}
	case capability.OutcomeApplied:
		result.NextActions = []capability.Action{{
			Code:    "validate_organization",
			Message: "Run organization validation.",
		}}
	case capability.OutcomeFailed:
		result.Recovery = capability.Recovery{
			Required: true,
			Guidance: []string{
				"Run adb doctor before retrying the organization operation.",
			},
		}
	// The remaining outcomes carry no generic next action or recovery guidance
	// from here. Unchanged and Conflict are attached by their own constructors
	// (conflictMutationResult adds the conflict warning), and Healthy/Attention
	// belong to the read/doctor surfaces, not to a mutation. Listed explicitly so
	// a new capability.Outcome cannot slip through with an empty envelope.
	case capability.OutcomeUnchanged,
		capability.OutcomeConflict,
		capability.OutcomeHealthy,
		capability.OutcomeAttention:
	}
	return result
}

func conflictMutationResult(
	descriptor capability.Descriptor,
	data MutationData,
	message string,
) capability.Result[MutationData] {
	result := mutationResult(
		descriptor,
		capability.OutcomeConflict,
		data,
		[]capability.Effect{},
	)
	result.Warnings = []capability.Notice{{
		Code:    "organization_conflict",
		Message: message,
	}}
	return result
}

func emptyMutationResult(
	descriptor capability.Descriptor,
) capability.Result[MutationData] {
	return mutationResult(
		descriptor,
		"",
		MutationData{},
		[]capability.Effect{},
	)
}

func (service *Service) commitManifestMutation(
	ctx context.Context,
	descriptor capability.Descriptor,
	current managedOrganization,
	layout Layout,
	manifest Manifest,
	content []byte,
	operationID string,
	previousPath string,
) (capability.Result[MutationData], error) {
	data := MutationData{
		Organization: organizationData(current.workspaceID, layout, manifest),
		OperationID:  operationID,
		PreviousPath: previousPath,
	}
	effects := mutationEffects(
		current.workspaceLayout,
		layout.ManifestPath(),
		capability.EffectPlanned,
	)
	journalStore, err := journal.NewStore(
		current.workspaceLayout.EventsDir(),
		service.now,
		service.newID,
	)
	if err != nil {
		return mutationResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := journalStore.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: descriptor.Capability + ":" + manifest.ID,
		Kind:           descriptor.Capability,
		Steps: []journal.Step{{
			Ordinal:    1,
			Action:     "replace",
			Target:     layout.ManifestPath(),
			BeforeHash: journal.Digest(current.manifestContent),
			AfterHash:  journal.Digest(content),
		}},
	}); err != nil {
		return mutationResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		return mutationResult(
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	if service.afterPlan != nil {
		if err := service.afterPlan(); err != nil {
			return mutationResult(
				descriptor,
				capability.OutcomeFailed,
				data,
				effects,
			), err
		}
	}
	if err := applyJournaledReplacement(
		current.workspaceLayout.Root(),
		journalStore,
		operationID,
		1,
		layout.ManifestPath(),
		current.manifestContent,
		content,
	); err != nil {
		return service.failMutation(
			journalStore,
			descriptor,
			data,
			effects,
			err,
		)
	}

	state, err := controlplane.Open(
		ctx,
		current.workspaceLayout.StatePath(),
	)
	if err != nil {
		return service.failMutation(
			journalStore,
			descriptor,
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
		organizationProjection(layout, manifest, content, service.now()),
	); err != nil {
		return service.failMutation(
			journalStore,
			descriptor,
			data,
			effects,
			err,
		)
	}
	if err := state.Close(); err != nil {
		return service.failMutation(
			journalStore,
			descriptor,
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
			descriptor,
			capability.OutcomeFailed,
			data,
			effects,
		), err
	}
	return mutationResult(
		descriptor,
		capability.OutcomeApplied,
		data,
		mutationEffects(
			current.workspaceLayout,
			layout.ManifestPath(),
			capability.EffectApplied,
		),
	), nil
}

func (service *Service) failMutation(
	journalStore *journal.Store,
	descriptor capability.Descriptor,
	data MutationData,
	effects []capability.Effect,
	cause error,
) (capability.Result[MutationData], error) {
	if _, appendErr := journalStore.Append(
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
			"%w (record organization operation failure in operation journal %q: %w)",
			cause,
			data.OperationID,
			appendErr,
		)
	}
	return mutationResult(
		descriptor,
		capability.OutcomeFailed,
		data,
		effects,
	), cause
}

func readOrganizationTargetWithin(
	root string,
	target string,
) (content []byte, err error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("workspace root must be absolute: %q", root)
	}
	if !filepath.IsAbs(target) {
		return nil, fmt.Errorf("organization target must be absolute: %q", target)
	}
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve organization target %q within workspace root %q: %w",
			cleanTarget,
			cleanRoot,
			err,
		)
	}
	if relative == "." ||
		filepath.IsAbs(relative) ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf(
			"organization target %q escapes workspace root %q",
			cleanTarget,
			cleanRoot,
		)
	}

	rooted, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("open workspace root %q: %w", cleanRoot, err)
	}
	defer func() {
		if closeErr := rooted.Close(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("close workspace root %q: %w", cleanRoot, closeErr),
			)
		}
	}()
	file, err := rooted.Open(relative)
	if err != nil {
		return nil, fmt.Errorf(
			"open organization target %q within workspace root: %w",
			cleanTarget,
			err,
		)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("close organization target %q: %w", cleanTarget, closeErr),
			)
		}
	}()
	content, err = io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf(
			"read organization target %q within workspace root: %w",
			cleanTarget,
			err,
		)
	}
	return content, nil
}

func statOrganizationTargetWithin(
	root string,
	target string,
) (info os.FileInfo, err error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("workspace root must be absolute: %q", root)
	}
	if !filepath.IsAbs(target) {
		return nil, fmt.Errorf("organization target must be absolute: %q", target)
	}
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve organization target %q within workspace root %q: %w",
			cleanTarget,
			cleanRoot,
			err,
		)
	}
	if relative == "." ||
		filepath.IsAbs(relative) ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf(
			"organization target %q escapes workspace root %q",
			cleanTarget,
			cleanRoot,
		)
	}

	rooted, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("open workspace root %q: %w", cleanRoot, err)
	}
	defer func() {
		if closeErr := rooted.Close(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("close workspace root %q: %w", cleanRoot, closeErr),
			)
		}
	}()
	info, err = rooted.Stat(relative)
	if err != nil {
		return nil, fmt.Errorf(
			"stat organization target %q within workspace root: %w",
			cleanTarget,
			err,
		)
	}
	return info, nil
}

func applyJournaledReplacement(
	root string,
	journalStore *journal.Store,
	operationID string,
	step int,
	path string,
	before []byte,
	after []byte,
) error {
	if _, err := journalStore.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}

	existing, err := readOrganizationTargetWithin(root, path)
	switch {
	case err == nil && bytes.Equal(existing, after):
	case err == nil && bytes.Equal(existing, before):
	case err != nil:
		return fmt.Errorf("read organization target %q: %w", path, err)
	default:
		return fmt.Errorf(
			"organization target %q changed after planning",
			path,
		)
	}
	if err := atomicfile.WriteWithin(
		root,
		path,
		atomicfile.Options{Mode: 0o644},
		func(writer io.Writer) error {
			_, writeErr := writer.Write(after)
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

func appendUniqueAliases(
	aliases []string,
	currentSlug string,
	values ...string,
) []string {
	result := make([]string, 0, len(aliases)+len(values))
	seen := make(map[string]struct{}, len(aliases)+len(values))
	for _, value := range append(append([]string(nil), aliases...), values...) {
		if value == "" || value == currentSlug {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

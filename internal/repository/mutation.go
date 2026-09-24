package repository

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

const defaultConfig = "schema_version: aidb.config/v1\n"

type organizationScope struct {
	workspaceLayout workspace.Layout
	organizationID  string
	layout          organization.Layout
	manifest        organization.Manifest
	conflict        string
}

type repositoryInspection struct {
	scope      organizationScope
	layout     Layout
	remoteURL  RemoteURL
	remoteName string
	existing   *Manifest
	conflict   string
}

func inspectOrganizationScope(
	ctx context.Context,
	root string,
	selector string,
) (organizationScope, error) {
	if selector == "" {
		return organizationScope{}, errors.New(
			"repository organization selector is required",
		)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		return organizationScope{}, err
	}
	workspaceManifest, err := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if err != nil {
		return organizationScope{}, err
	}
	if err := workspaceManifest.Validate(workspaceLayout); err != nil {
		return organizationScope{}, err
	}
	if _, err := workspaceLayout.InspectRole(
		workspaceManifest.Roles.Organizations,
	); err != nil {
		return organizationScope{}, err
	}
	state, err := controlplane.OpenReadOnly(ctx, workspaceLayout.StatePath())
	if err != nil {
		return organizationScope{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	projection, err := state.Organization(ctx, selector)
	if err != nil {
		return organizationScope{}, err
	}
	layout, err := organization.NewLayoutForRole(
		workspaceLayout.Root(),
		workspaceManifest.Roles.Organizations,
		projection.Slug,
	)
	if err != nil {
		return organizationScope{}, err
	}
	if _, err := workspace.InspectPath(
		workspaceLayout.Root(),
		layout.Root(),
	); err != nil {
		return organizationScope{}, err
	}
	if layout.Root() != projection.Path {
		return organizationScope{}, fmt.Errorf(
			"organization projection path %q does not match %q",
			projection.Path,
			layout.Root(),
		)
	}
	manifestContent, err := readRepositoryTargetWithin(
		workspaceLayout.Root(),
		layout.ManifestPath(),
	)
	if err != nil {
		return organizationScope{}, err
	}
	manifest, err := organization.DecodeManifest(
		bytes.NewReader(manifestContent),
	)
	if err != nil {
		return organizationScope{}, err
	}
	if err := manifest.Validate(layout); err != nil {
		return organizationScope{}, err
	}
	if _, err := layout.InspectRole(manifest.Roles.Repositories); err != nil {
		return organizationScope{}, err
	}
	scope := organizationScope{
		workspaceLayout: workspaceLayout,
		organizationID:  manifest.ID,
		layout:          layout,
		manifest:        manifest,
	}
	if manifest.Status == organization.StatusArchived {
		scope.conflict = fmt.Sprintf(
			"organization %q is archived",
			manifest.Slug,
		)
	}
	return scope, nil
}

func (scope organizationScope) inspectRepositoriesRole() error {
	_, err := scope.layout.InspectRole(scope.manifest.Roles.Repositories)
	return err
}

func inspectManagedDirectory(
	scope organizationScope,
	target string,
	allowMissing bool,
) (err error) {
	inspection, err := workspace.InspectPath(
		scope.workspaceLayout.Root(),
		target,
	)
	if err != nil {
		return err
	}
	if inspection.State == workspace.RoleInspectionMissingTail {
		if allowMissing {
			return nil
		}
		return fmt.Errorf("managed repository path %q does not exist", target)
	}

	rooted, err := os.OpenRoot(scope.workspaceLayout.Root())
	if err != nil {
		return fmt.Errorf(
			"open workspace root %q: %w",
			scope.workspaceLayout.Root(),
			err,
		)
	}
	defer func() {
		if closeErr := rooted.Close(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf(
					"close workspace root %q: %w",
					scope.workspaceLayout.Root(),
					closeErr,
				),
			)
		}
	}()
	relative, err := filepath.Rel(scope.workspaceLayout.Root(), target)
	if err != nil {
		return fmt.Errorf("resolve managed repository path %q: %w", target, err)
	}
	info, err := rooted.Lstat(relative)
	if err != nil {
		return fmt.Errorf("inspect managed repository path %q: %w", target, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("managed repository path %q is not a directory", target)
	}
	return nil
}

func inspectRepositoryContainer(
	inspection repositoryInspection,
	allowMissing bool,
) error {
	return inspectManagedDirectory(
		inspection.scope,
		inspection.layout.Root(),
		allowMissing,
	)
}

func inspectRepositoryClone(
	inspection repositoryInspection,
	allowMissing bool,
) error {
	return inspectManagedDirectory(
		inspection.scope,
		inspection.layout.CloneDir(),
		allowMissing,
	)
}

func inspectCanonicalClone(
	managed managedRepository,
	allowMissing bool,
) error {
	if managed.manifest.CanonicalClone.External {
		return nil
	}
	return inspectManagedDirectory(
		managed.inspection.scope,
		managed.data.ClonePath,
		allowMissing,
	)
}

func inspectRepositoryTarget(
	ctx context.Context,
	scope organizationScope,
	remoteURL RemoteURL,
	remoteName string,
) (repositoryInspection, error) {
	layout, err := NewLayout(
		scope.layout,
		scope.manifest.Roles.Repositories,
		remoteURL.Host,
		remoteURL.Owner,
		remoteURL.Repository,
	)
	if err != nil {
		return repositoryInspection{}, err
	}
	inspection := repositoryInspection{
		scope:      scope,
		layout:     layout,
		remoteURL:  remoteURL,
		remoteName: remoteName,
		conflict:   scope.conflict,
	}
	if inspection.conflict != "" {
		return inspection, nil
	}

	pathInspection, err := workspace.InspectPath(
		scope.workspaceLayout.Root(),
		layout.Root(),
	)
	if err != nil {
		return repositoryInspection{}, err
	}
	if pathInspection.State == workspace.RoleInspectionContained {
		content, readErr := readRepositoryTargetWithin(
			scope.workspaceLayout.Root(),
			layout.ManifestPath(),
		)
		if readErr == nil {
			existing, decodeErr := DecodeManifest(bytes.NewReader(content))
			if decodeErr != nil {
				return repositoryInspection{}, decodeErr
			}
			if err := existing.Validate(layout); err != nil {
				return repositoryInspection{}, err
			}
			inspection.existing = &existing
			return inspection, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return repositoryInspection{}, readErr
		}
		inspection.conflict = fmt.Sprintf(
			"repository path %q exists without a managed manifest",
			layout.Root(),
		)
		return inspection, nil
	}

	state, err := controlplane.OpenReadOnly(
		ctx,
		scope.workspaceLayout.StatePath(),
	)
	if err != nil {
		return repositoryInspection{}, err
	}
	defer func() {
		_ = state.Close()
	}()
	for _, selector := range []string{layout.Key(), layout.Root()} {
		projection, err := state.Repository(
			ctx,
			scope.organizationID,
			selector,
		)
		if err == nil {
			inspection.conflict = fmt.Sprintf(
				"repository selector %q is already registered to %q",
				selector,
				projection.ID,
			)
			return inspection, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return repositoryInspection{}, err
		}
	}
	return inspection, nil
}

func existingMatches(
	existing *Manifest,
	remoteURL RemoteURL,
	external bool,
	clonePath string,
) bool {
	if existing == nil {
		return false
	}
	return existing.CanonicalRemote.FetchURL == remoteURL.Normalized &&
		existing.CanonicalClone.External == external &&
		existing.CanonicalClone.Path == clonePath
}

func repositoryProjection(
	layout Layout,
	manifest Manifest,
	content []byte,
	observedAt time.Time,
) controlplane.RepositoryProjection {
	return controlplane.RepositoryProjection{
		ID:              manifest.ID,
		OrganizationID:  manifest.OrganizationID,
		Host:            manifest.Host,
		Owner:           manifest.Owner,
		Name:            manifest.Name,
		Path:            layout.Root(),
		ManifestHash:    journal.Digest(content),
		CanonicalRemote: manifest.CanonicalRemote.FetchURL,
		Status:          controlplane.EntityStatus(manifest.Status),
		Aliases:         append([]string(nil), manifest.Aliases...),
		ObservedAt:      observedAt.UTC(),
	}
}

func repositoryEffects(
	inspection repositoryInspection,
	gitAction string,
	status capability.EffectStatus,
) []capability.Effect {
	effects := make([]capability.Effect, 0, 6)
	if gitAction != "" {
		effects = append(effects, capability.Effect{
			Action: gitAction,
			Target: inspection.layout.CloneDir(),
			Status: status,
		})
	}
	effects = append(
		effects,
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
			Target: inspection.scope.workspaceLayout.StatePath(),
			Status: status,
		},
	)
	return effects
}

func result(
	descriptor capability.Descriptor,
	outcome capability.Outcome,
	data MutationData,
	effects []capability.Effect,
) capability.Result[MutationData] {
	value := capability.Result[MutationData]{
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
			Message: "Run the repository operation with apply enabled.",
		}}
	}
	if outcome == capability.OutcomeFailed {
		value.Recovery = capability.Recovery{
			Required: true,
			Guidance: []string{
				"Run adb doctor before retrying the repository operation.",
			},
		}
	}
	return value
}

func conflictResult(
	descriptor capability.Descriptor,
	data MutationData,
	message string,
) capability.Result[MutationData] {
	value := result(
		descriptor,
		capability.OutcomeConflict,
		data,
		[]capability.Effect{},
	)
	value.Warnings = []capability.Notice{{
		Code:    "repository_conflict",
		Message: message,
	}}
	return value
}

func emptyResult(
	descriptor capability.Descriptor,
) capability.Result[MutationData] {
	return result(descriptor, "", MutationData{}, []capability.Effect{})
}

func agentsPointer(layout Layout) (string, error) {
	workspaceAgents := filepath.Join(
		layout.OrganizationRoot(),
		"..",
		"..",
		"AGENTS.md",
	)
	workspaceAgents = filepath.Clean(workspaceAgents)
	relative, err := filepath.Rel(layout.Root(), workspaceAgents)
	if err != nil {
		return "", fmt.Errorf("resolve repository AGENTS.md pointer: %w", err)
	}
	return "# Repository instructions\n\n" +
		"Canonical workspace instructions: [AGENTS.md](" +
		filepath.ToSlash(relative) +
		").\n", nil
}

func applyFile(
	root string,
	store *journal.Store,
	operationID string,
	step int,
	path string,
	content []byte,
) error {
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}
	existing, err := readRepositoryTargetWithin(root, path)
	switch {
	case err == nil && bytes.Equal(existing, content):
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("read repository target %q: %w", path, err)
	default:
		return fmt.Errorf(
			"repository target %q contains different content",
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
	_, err = store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step,
	})
	return err
}

func replaceFile(
	root string,
	store *journal.Store,
	operationID string,
	step int,
	path string,
	before []byte,
	after []byte,
) error {
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplying,
		Step:  step,
	}); err != nil {
		return err
	}
	existing, err := readRepositoryTargetWithin(root, path)
	switch {
	case err == nil && bytes.Equal(existing, after):
	case err == nil && bytes.Equal(existing, before):
	case err != nil:
		return fmt.Errorf("read repository target %q: %w", path, err)
	default:
		return fmt.Errorf(
			"repository target %q changed after planning",
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
	_, err = store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseStepApplied,
		Step:  step,
	})
	return err
}

func readRepositoryTargetWithin(
	root string,
	target string,
) (content []byte, err error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("workspace root must be absolute: %q", root)
	}
	if !filepath.IsAbs(target) {
		return nil, fmt.Errorf("repository target must be absolute: %q", target)
	}
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve repository target %q within workspace root %q: %w",
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
			"repository target %q escapes workspace root %q",
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
			"open repository target %q within workspace root: %w",
			cleanTarget,
			err,
		)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("close repository target %q: %w", cleanTarget, closeErr),
			)
		}
	}()
	content, err = io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf(
			"read repository target %q within workspace root: %w",
			cleanTarget,
			err,
		)
	}
	return content, nil
}

func appendAliases(
	aliases []string,
	currentKey string,
	currentPath string,
	values ...string,
) []string {
	result := make([]string, 0, len(aliases)+len(values))
	seen := make(map[string]struct{}, len(aliases)+len(values))
	for _, value := range append(append([]string(nil), aliases...), values...) {
		if value == "" || value == currentKey || value == currentPath {
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

func findRemote(inventory Inventory, name string) (RemoteState, bool) {
	for _, remote := range inventory.Remotes {
		if remote.Name == name {
			return remote, true
		}
	}
	return RemoteState{}, false
}

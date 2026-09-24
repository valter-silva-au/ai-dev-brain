package ticket

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

type ScopeKind string

const (
	ScopeOrganization ScopeKind = "organization"
	ScopeRepository   ScopeKind = "repository"
)

type ScopeLayout struct {
	kind           ScopeKind
	workspaceRoot  string
	organizationID string
	repositoryID   string
	trustRoot      string
	ownerRoot      string
	ticketsRoot    string
}

type Layout struct {
	scope        ScopeLayout
	localKey     string
	pathSlug     string
	relativePath string
	root         string
}

const workspacePathAliasPrefix = "@workspace/"

func NewOrganizationScope(
	layout organization.Layout,
	manifest organization.Manifest,
) (ScopeLayout, error) {
	return newOrganizationScope(layout, manifest, true)
}

func newOrganizationScope(
	layout organization.Layout,
	manifest organization.Manifest,
	requireActive bool,
) (ScopeLayout, error) {
	if err := manifest.Validate(layout); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate ticket organization owner: %w",
			err,
		)
	}
	if requireActive && manifest.Status != organization.StatusActive {
		return ScopeLayout{}, errors.New(
			"archived organization cannot own a mutable ticket scope",
		)
	}
	if err := validateResolvedContainment(
		layout.WorkspaceRoot(),
		layout.Root(),
	); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate organization owner containment: %w",
			err,
		)
	}
	ticketsRoot, err := layout.ResolveRole(manifest.Roles.Tickets)
	if err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"resolve organization tickets role: %w",
			err,
		)
	}
	if err := validateResolvedContainment(layout.Root(), ticketsRoot); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate organization tickets role containment: %w",
			err,
		)
	}
	return ScopeLayout{
		kind:           ScopeOrganization,
		workspaceRoot:  layout.WorkspaceRoot(),
		organizationID: manifest.ID,
		trustRoot:      layout.WorkspaceRoot(),
		ownerRoot:      layout.Root(),
		ticketsRoot:    ticketsRoot,
	}, nil
}

func NewRepositoryScope(
	organizationLayout organization.Layout,
	organizationManifest organization.Manifest,
	repositoryLayout repository.Layout,
	repositoryManifest repository.Manifest,
) (ScopeLayout, error) {
	return newRepositoryScope(
		organizationLayout,
		organizationManifest,
		repositoryLayout,
		repositoryManifest,
		true,
	)
}

func newRepositoryScope(
	organizationLayout organization.Layout,
	organizationManifest organization.Manifest,
	repositoryLayout repository.Layout,
	repositoryManifest repository.Manifest,
	requireActive bool,
) (ScopeLayout, error) {
	if err := organizationManifest.Validate(organizationLayout); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate ticket organization owner: %w",
			err,
		)
	}
	if err := repositoryManifest.Validate(repositoryLayout); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate ticket repository owner: %w",
			err,
		)
	}
	if requireActive &&
		organizationManifest.Status != organization.StatusActive {
		return ScopeLayout{}, errors.New(
			"archived organization cannot own a mutable ticket scope",
		)
	}
	if requireActive && repositoryManifest.Status != repository.StatusActive {
		return ScopeLayout{}, errors.New(
			"archived repository cannot own a mutable ticket scope",
		)
	}
	if repositoryManifest.OrganizationID != organizationManifest.ID {
		return ScopeLayout{}, fmt.Errorf(
			"repository organization %q does not match ticket trust scope %q",
			repositoryManifest.OrganizationID,
			organizationManifest.ID,
		)
	}
	if filepath.Clean(repositoryLayout.OrganizationRoot()) !=
		filepath.Clean(organizationLayout.Root()) {
		return ScopeLayout{}, errors.New(
			"repository layout is outside its organization trust scope",
		)
	}
	if err := validateResolvedContainment(
		organizationLayout.WorkspaceRoot(),
		organizationLayout.Root(),
	); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate repository organization containment: %w",
			err,
		)
	}
	if err := validateResolvedContainment(
		organizationLayout.Root(),
		repositoryLayout.Root(),
	); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate repository owner containment: %w",
			err,
		)
	}
	ticketsRoot, err := repositoryLayout.ResolveRole(
		repositoryManifest.Roles.Tickets,
	)
	if err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"resolve repository tickets role: %w",
			err,
		)
	}
	if err := validateResolvedContainment(
		repositoryLayout.Root(),
		ticketsRoot,
	); err != nil {
		return ScopeLayout{}, fmt.Errorf(
			"validate repository tickets role containment: %w",
			err,
		)
	}
	return ScopeLayout{
		kind:           ScopeRepository,
		workspaceRoot:  organizationLayout.WorkspaceRoot(),
		organizationID: organizationManifest.ID,
		repositoryID:   repositoryManifest.ID,
		trustRoot:      organizationLayout.Root(),
		ownerRoot:      repositoryLayout.Root(),
		ticketsRoot:    ticketsRoot,
	}, nil
}

func (scope ScopeLayout) Ticket(
	localKey string,
	pathSlug string,
) (Layout, error) {
	if err := ValidateLocalKey(localKey); err != nil {
		return Layout{}, err
	}
	if err := ValidateSlug(pathSlug); err != nil {
		return Layout{}, err
	}
	if scope.kind != ScopeOrganization && scope.kind != ScopeRepository {
		return Layout{}, errors.New("ticket scope kind is invalid")
	}
	if scope.workspaceRoot == "" ||
		scope.organizationID == "" ||
		scope.trustRoot == "" ||
		scope.ownerRoot == "" ||
		scope.ticketsRoot == "" {
		return Layout{}, errors.New("ticket scope layout is incomplete")
	}
	if scope.kind == ScopeRepository && scope.repositoryID == "" {
		return Layout{}, errors.New("repository ticket scope id is required")
	}
	if scope.kind == ScopeOrganization && scope.repositoryID != "" {
		return Layout{}, errors.New(
			"organization ticket scope cannot carry a repository id",
		)
	}
	if err := scope.ValidateFilesystemContainment(); err != nil {
		return Layout{}, err
	}

	relativePath := localKey + "-" + pathSlug
	root := filepath.Join(scope.ticketsRoot, relativePath)
	if err := requireContainedPath(scope.ticketsRoot, root); err != nil {
		return Layout{}, err
	}
	return Layout{
		scope:        scope,
		localKey:     localKey,
		pathSlug:     pathSlug,
		relativePath: relativePath,
		root:         root,
	}, nil
}

func (scope ScopeLayout) Kind() ScopeKind {
	return scope.kind
}

func (scope ScopeLayout) WorkspaceRoot() string {
	return scope.workspaceRoot
}

func (scope ScopeLayout) OrganizationID() string {
	return scope.organizationID
}

func (scope ScopeLayout) RepositoryID() string {
	return scope.repositoryID
}

func (scope ScopeLayout) OwnerRoot() string {
	return scope.ownerRoot
}

func (scope ScopeLayout) TrustRoot() string {
	return scope.trustRoot
}

func (scope ScopeLayout) TicketsRoot() string {
	return scope.ticketsRoot
}

func (scope ScopeLayout) ValidateFilesystemContainment() error {
	if err := validateResolvedContainment(
		scope.trustRoot,
		scope.ownerRoot,
	); err != nil {
		return fmt.Errorf("ticket owner containment changed: %w", err)
	}
	if err := validateResolvedContainment(
		scope.ownerRoot,
		scope.ticketsRoot,
	); err != nil {
		return fmt.Errorf("ticket scope role containment changed: %w", err)
	}
	return nil
}

func (layout Layout) Scope() ScopeLayout {
	return layout.scope
}

func (layout Layout) LocalKey() string {
	return layout.localKey
}

func (layout Layout) PathSlug() string {
	return layout.pathSlug
}

func (layout Layout) RelativePath() string {
	return layout.relativePath
}

func (layout Layout) OwnerRelativePath() string {
	relative, err := filepath.Rel(layout.scope.ownerRoot, layout.root)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

func (layout Layout) WorkspaceRelativePath() string {
	relative, err := filepath.Rel(
		layout.scope.workspaceRoot,
		layout.root,
	)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

func (layout Layout) Root() string {
	return layout.root
}

func (layout Layout) StatusPath() string {
	return filepath.Join(layout.root, "status.yaml")
}

func (layout Layout) ControlDir() string {
	return filepath.Join(layout.root, ".aidb")
}

func (layout Layout) ConfigPath() string {
	return filepath.Join(layout.ControlDir(), "config.yaml")
}

func (layout Layout) RenderManifestPath() string {
	return filepath.Join(layout.ControlDir(), "rendered.yaml")
}

func (layout Layout) TombstonesPath() string {
	return filepath.Join(layout.ControlDir(), "tombstones.yaml")
}

func (layout Layout) EventsDir() string {
	return filepath.Join(layout.ControlDir(), "events")
}

func workspacePathAlias(relative string) string {
	return workspacePathAliasPrefix + filepath.ToSlash(relative)
}

func parseWorkspacePathAlias(alias string) (string, bool) {
	if !strings.HasPrefix(alias, workspacePathAliasPrefix) {
		return "", false
	}
	relative := strings.TrimPrefix(alias, workspacePathAliasPrefix)
	if validatePortableRelativeSelector(relative) != nil {
		return "", false
	}
	return relative, true
}

func requireContainedPath(root string, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve contained ticket path: %w", err)
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("ticket path escapes scope root: %q", target)
	}
	return nil
}

func validateResolvedContainment(root string, target string) error {
	resolvedRoot, err := resolveExistingPrefix(root)
	if err != nil {
		return fmt.Errorf("resolve ticket owner root %q: %w", root, err)
	}
	resolvedTarget, err := resolveExistingPrefix(target)
	if err != nil {
		return fmt.Errorf("resolve ticket role path %q: %w", target, err)
	}
	if err := requireContainedPath(resolvedRoot, resolvedTarget); err != nil {
		return fmt.Errorf(
			"resolved ticket role %q escapes owner root %q",
			resolvedTarget,
			resolvedRoot,
		)
	}
	return nil
}

func resolveExistingPrefix(path string) (string, error) {
	path = filepath.Clean(path)
	current := path
	missing := make([]string, 0)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing path prefix for %q", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

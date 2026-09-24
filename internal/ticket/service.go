package ticket

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

type LocatedManifest struct {
	Layout   Layout
	Manifest Manifest
}

type Service struct {
	now                       func() time.Time
	newID                     func() string
	templateResolver          profile.TemplateResolver
	profileResolver           ProfileResolver
	completionGate            CompletionGate
	git                       repository.Git
	afterPlan                 func() error
	beforeProjection          func(context.Context, Layout, Manifest) error
	afterProjection           func(context.Context, Layout, Manifest) error
	afterFilesystemStage      func(string) error
	beforeMutationLock        func() error
	beforeRootedPublish       func(string) error
	afterRootedDisplace       func(string) error
	beforeGuardRelease        func(string) error
	beforeGuardMove           func(string) error
	beforeRecoveryGuardSettle func(string) error
}

func NewService() *Service {
	service, err := NewServiceWithOptions(Options{})
	if err != nil {
		panic(err)
	}
	return service
}

func NewServiceWithOptions(options Options) (*Service, error) {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.IDGenerator == nil {
		options.IDGenerator = uuid.NewString
	}
	if options.TemplateResolver == nil {
		catalog, err := profile.NewTemplateCatalog(profile.BuiltinTemplates())
		if err != nil {
			return nil, fmt.Errorf("load built-in ticket templates: %w", err)
		}
		options.TemplateResolver = catalog
	}
	if options.ProfileResolver == nil {
		options.ProfileResolver = ProfileResolverFunc(
			func(reference profile.ProfileReference) (profile.Profile, error) {
				if reference.ID != profile.BuiltinTicketProfileID ||
					reference.Version !=
						profile.BuiltinTicketProfileVersion {
					return profile.Profile{}, fmt.Errorf(
						"profile %s is not available",
						reference.String(),
					)
				}
				return profile.BuiltinTicketProfile()
			},
		)
	}
	if options.Git == nil {
		options.Git = repository.NewClient(nil)
	}
	if options.Clock == nil {
		return nil, errors.New("ticket clock is required")
	}
	if options.IDGenerator == nil {
		return nil, errors.New("ticket id generator is required")
	}
	return &Service{
		now:                  options.Clock,
		newID:                options.IDGenerator,
		templateResolver:     options.TemplateResolver,
		profileResolver:      options.ProfileResolver,
		completionGate:       options.CompletionGate,
		git:                  options.Git,
		afterPlan:            options.AfterPlan,
		beforeProjection:     options.BeforeProjection,
		afterProjection:      options.AfterProjection,
		afterFilesystemStage: options.AfterFilesystemStage,
		beforeMutationLock:   options.BeforeMutationLock,
	}, nil
}

var processWorkspaceLocks sync.Map

func (service *Service) WithAllocatedLocalKey(
	scope ScopeLayout,
	policy KeyPolicy,
	reserve func(string, []LocatedManifest) error,
) (string, error) {
	if reserve == nil {
		return "", errors.New("ticket key reservation callback is required")
	}
	if err := validateKeyPolicy(policy); err != nil {
		return "", err
	}
	if _, err := scope.Ticket(
		fmt.Sprintf("%s-%0*d", policy.Prefix, policy.Width, 1),
		"reservation-probe",
	); err != nil {
		return "", fmt.Errorf("validate ticket key scope: %w", err)
	}

	controlDir := filepath.Join(scope.WorkspaceRoot(), ".aidb")
	if err := os.MkdirAll(controlDir, 0o755); err != nil {
		return "", fmt.Errorf(
			"create workspace control directory for ticket key allocation: %w",
			err,
		)
	}
	lockPath := filepath.Join(controlDir, "workspace.lock")
	processLock := processWorkspaceMutex(lockPath)
	processLock.Lock()
	defer processLock.Unlock()

	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return "", fmt.Errorf("open workspace lock %q: %w", lockPath, err)
	}
	unlock, err := lockfile.Lock(file)
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("acquire workspace lock %q: %w", lockPath, err)
	}
	defer func() {
		unlock()
		_ = file.Close()
	}()

	located, err := ReadWorkspaceManifests(scope.WorkspaceRoot())
	if err != nil {
		return "", err
	}
	currentScope, err := ReadScopeManifests(scope)
	if err != nil {
		return "", err
	}
	located = mergeLocatedManifests(located, currentScope)
	if err := ValidateCatalog(located); err != nil {
		return "", err
	}
	identities := make([]Identity, 0, len(located))
	for _, ticket := range located {
		if !sameScope(scope, ticket.Layout.Scope()) {
			continue
		}
		identities = append(identities, Identity{
			LocalKey: ticket.Manifest.LocalKey,
			Aliases:  append([]string(nil), ticket.Manifest.Aliases...),
		})
	}
	key, err := AllocateLocalKey(policy, identities)
	if err != nil {
		return "", err
	}
	if err := reserve(
		key,
		append([]LocatedManifest(nil), located...),
	); err != nil {
		return "", fmt.Errorf("reserve ticket key %q: %w", key, err)
	}
	return key, nil
}

func ReadScopeManifests(scope ScopeLayout) ([]LocatedManifest, error) {
	return readScopeManifests(scope, nil)
}

func readScopeManifests(
	scope ScopeLayout,
	afterContainmentCheck func() error,
) ([]LocatedManifest, error) {
	if err := scope.ValidateFilesystemContainment(); err != nil {
		return nil, err
	}
	if afterContainmentCheck != nil {
		if err := afterContainmentCheck(); err != nil {
			return nil, fmt.Errorf(
				"run ticket scope scan checkpoint: %w",
				err,
			)
		}
	}
	ticketsRoot, err := openScopeTicketsRoot(scope)
	if errors.Is(err, os.ErrNotExist) {
		return []LocatedManifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"open rooted ticket scope %q: %w",
			scope.TicketsRoot(),
			err,
		)
	}
	defer func() {
		_ = ticketsRoot.Close()
	}()
	directory, err := ticketsRoot.Open(".")
	if err != nil {
		return nil, fmt.Errorf(
			"open ticket scope directory %q: %w",
			scope.TicketsRoot(),
			err,
		)
	}
	entries, err := directory.ReadDir(-1)
	closeErr := directory.Close()
	if err != nil {
		return nil, fmt.Errorf(
			"read ticket scope %q: %w",
			scope.TicketsRoot(),
			err,
		)
	}
	if closeErr != nil {
		return nil, fmt.Errorf(
			"close ticket scope %q: %w",
			scope.TicketsRoot(),
			closeErr,
		)
	}
	sort.Slice(entries, func(left int, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})

	result := make([]LocatedManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == ".aidb" {
			continue
		}
		entryInfo, lstatErr := ticketsRoot.Lstat(entry.Name())
		if lstatErr != nil {
			return nil, fmt.Errorf(
				"inspect ticket scope entry %q: %w",
				entry.Name(),
				lstatErr,
			)
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"ticket scope contains symlink entry %q",
				entry.Name(),
			)
		}
		if !entryInfo.IsDir() {
			continue
		}
		ticketRoot, openRootErr := ticketsRoot.OpenRoot(entry.Name())
		if openRootErr != nil {
			return nil, fmt.Errorf(
				"open rooted ticket directory %q: %w",
				entry.Name(),
				openRootErr,
			)
		}
		openedEntryInfo, statErr := ticketRoot.Stat(".")
		if statErr != nil {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"inspect rooted ticket directory %q: %w",
				entry.Name(),
				statErr,
			)
		}
		if !os.SameFile(entryInfo, openedEntryInfo) {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"ticket directory %q changed during inspection",
				entry.Name(),
			)
		}
		statusPath := filepath.Join(
			scope.TicketsRoot(),
			entry.Name(),
			"status.yaml",
		)
		statusInfo, lstatErr := ticketRoot.Lstat("status.yaml")
		if errors.Is(lstatErr, os.ErrNotExist) {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"ticket directory %q has no authoritative status.yaml",
				entry.Name(),
			)
		}
		if lstatErr != nil {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"inspect ticket manifest %q: %w",
				statusPath,
				lstatErr,
			)
		}
		if statusInfo.Mode()&os.ModeSymlink != 0 ||
			!statusInfo.Mode().IsRegular() {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"ticket manifest %q must be a regular file",
				statusPath,
			)
		}
		file, openErr := ticketRoot.Open("status.yaml")
		if errors.Is(openErr, os.ErrNotExist) {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"ticket manifest %q changed during inspection",
				statusPath,
			)
		}
		if openErr != nil {
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"open ticket manifest %q: %w",
				statusPath,
				openErr,
			)
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"inspect opened ticket manifest %q: %w",
				statusPath,
				statErr,
			)
		}
		if !os.SameFile(statusInfo, openedInfo) {
			_ = file.Close()
			_ = ticketRoot.Close()
			return nil, fmt.Errorf(
				"ticket manifest %q changed during inspection",
				statusPath,
			)
		}
		manifest, decodeErr := DecodeManifest(file)
		closeErr := file.Close()
		rootCloseErr := ticketRoot.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf(
				"read ticket manifest %q: %w",
				statusPath,
				decodeErr,
			)
		}
		if closeErr != nil {
			return nil, fmt.Errorf(
				"close ticket manifest %q: %w",
				statusPath,
				closeErr,
			)
		}
		if rootCloseErr != nil {
			return nil, fmt.Errorf(
				"close rooted ticket directory %q: %w",
				entry.Name(),
				rootCloseErr,
			)
		}
		if manifest.OrganizationID != scope.OrganizationID() ||
			manifest.RepositoryID != scope.RepositoryID() {
			return nil, fmt.Errorf(
				"ticket manifest %q crosses its owning trust scope",
				statusPath,
			)
		}
		layout, layoutErr := scope.Ticket(
			manifest.LocalKey,
			manifest.PathSlug,
		)
		if layoutErr != nil {
			return nil, fmt.Errorf(
				"resolve ticket manifest %q layout: %w",
				statusPath,
				layoutErr,
			)
		}
		if layout.RelativePath() != entry.Name() {
			return nil, fmt.Errorf(
				"ticket manifest path %q does not match identity %q",
				entry.Name(),
				layout.RelativePath(),
			)
		}
		if err := validateStoredManifest(manifest, layout); err != nil {
			return nil, fmt.Errorf(
				"validate ticket manifest %q: %w",
				statusPath,
				err,
			)
		}
		result = append(result, LocatedManifest{
			Layout:   layout,
			Manifest: manifest,
		})
	}
	sort.Slice(result, func(left int, right int) bool {
		if result[left].Manifest.VisibleKey !=
			result[right].Manifest.VisibleKey {
			return result[left].Manifest.VisibleKey <
				result[right].Manifest.VisibleKey
		}
		return result[left].Manifest.ID < result[right].Manifest.ID
	})
	if err := ValidateCatalog(result); err != nil {
		return nil, err
	}
	return result, nil
}

func openScopeTicketsRoot(scope ScopeLayout) (*os.Root, error) {
	workspaceRoot, err := os.OpenRoot(scope.WorkspaceRoot())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = workspaceRoot.Close()
	}()

	trustRoot := workspaceRoot
	closeTrustRoot := false
	if filepath.Clean(scope.TrustRoot()) !=
		filepath.Clean(scope.WorkspaceRoot()) {
		trustRoot, err = openRootWithin(
			workspaceRoot,
			scope.WorkspaceRoot(),
			scope.TrustRoot(),
		)
		if err != nil {
			return nil, err
		}
		closeTrustRoot = true
	}
	if closeTrustRoot {
		defer func() {
			_ = trustRoot.Close()
		}()
	}

	ownerRoot, err := openRootWithin(
		trustRoot,
		scope.TrustRoot(),
		scope.OwnerRoot(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ownerRoot.Close()
	}()
	return openRootWithin(
		ownerRoot,
		scope.OwnerRoot(),
		scope.TicketsRoot(),
	)
}

func openRootWithin(
	root *os.Root,
	rootPath string,
	targetPath string,
) (*os.Root, error) {
	if err := requireContainedPath(rootPath, targetPath); err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return nil, fmt.Errorf("resolve rooted path: %w", err)
	}
	return root.OpenRoot(relative)
}

func validateStoredManifest(manifest Manifest, layout Layout) error {
	if err := manifest.validateCore(layout); err != nil {
		return err
	}
	if manifest.Profile.ID != profile.BuiltinTicketProfileID ||
		manifest.Profile.Version != profile.BuiltinTicketProfileVersion {
		return nil
	}
	activeProfile, err := profile.BuiltinTicketProfile()
	if err != nil {
		return fmt.Errorf("load built-in ticket profile: %w", err)
	}
	return manifest.Validate(layout, activeProfile)
}

func ReadWorkspaceManifests(
	workspaceRoot string,
) ([]LocatedManifest, error) {
	workspaceLayout, err := workspace.NewLayout(workspaceRoot)
	if err != nil {
		return nil, err
	}
	workspaceManifest, err := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if err != nil {
		return nil, err
	}
	if err := workspaceManifest.Validate(workspaceLayout); err != nil {
		return nil, fmt.Errorf("validate workspace manifest: %w", err)
	}
	organizationsRoot, err := workspaceLayout.ResolveRole(
		workspaceManifest.Roles.Organizations,
	)
	if err != nil {
		return nil, err
	}
	if err := validateResolvedContainment(
		workspaceLayout.Root(),
		organizationsRoot,
	); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(organizationsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []LocatedManifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace organizations: %w", err)
	}

	result := make([]LocatedManifest, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"workspace organization entry %q is a symlink",
				entry.Name(),
			)
		}
		if !entry.IsDir() {
			continue
		}
		organizationLayout, layoutErr := organization.NewLayoutForRole(
			workspaceLayout.Root(),
			workspaceManifest.Roles.Organizations,
			entry.Name(),
		)
		if layoutErr != nil {
			return nil, fmt.Errorf(
				"resolve organization %q: %w",
				entry.Name(),
				layoutErr,
			)
		}
		organizationManifest, readErr := organization.ReadManifest(
			organizationLayout.ManifestPath(),
		)
		if readErr != nil {
			return nil, readErr
		}
		organizationScope, scopeErr := newOrganizationScope(
			organizationLayout,
			organizationManifest,
			false,
		)
		if scopeErr != nil {
			return nil, scopeErr
		}
		organizationTickets, ticketsErr := ReadScopeManifests(
			organizationScope,
		)
		if ticketsErr != nil {
			return nil, ticketsErr
		}
		result = append(result, organizationTickets...)

		repositoryTickets, repositoriesErr := readRepositoryTickets(
			organizationLayout,
			organizationManifest,
		)
		if repositoriesErr != nil {
			return nil, repositoriesErr
		}
		result = append(result, repositoryTickets...)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left].Layout.Root() < result[right].Layout.Root()
	})
	if err := ValidateCatalog(result); err != nil {
		return nil, err
	}
	return result, nil
}

func readRepositoryTickets(
	organizationLayout organization.Layout,
	organizationManifest organization.Manifest,
) ([]LocatedManifest, error) {
	repositoriesRoot, err := organizationLayout.ResolveRole(
		organizationManifest.Roles.Repositories,
	)
	if err != nil {
		return nil, err
	}
	if err := validateResolvedContainment(
		organizationLayout.Root(),
		repositoriesRoot,
	); err != nil {
		return nil, err
	}
	hosts, err := os.ReadDir(repositoriesRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []LocatedManifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read organization repositories: %w", err)
	}

	result := make([]LocatedManifest, 0)
	for _, host := range hosts {
		if host.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"repository host entry %q is a symlink",
				host.Name(),
			)
		}
		if host.Name() == ".aidb" || !host.IsDir() {
			continue
		}
		hostRoot := filepath.Join(repositoriesRoot, host.Name())
		owners, readErr := os.ReadDir(hostRoot)
		if readErr != nil {
			return nil, fmt.Errorf(
				"read repository host %q: %w",
				host.Name(),
				readErr,
			)
		}
		for _, owner := range owners {
			if owner.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf(
					"repository owner entry %q/%q is a symlink",
					host.Name(),
					owner.Name(),
				)
			}
			if owner.Name() == ".aidb" || !owner.IsDir() {
				continue
			}
			ownerRoot := filepath.Join(hostRoot, owner.Name())
			names, namesErr := os.ReadDir(ownerRoot)
			if namesErr != nil {
				return nil, fmt.Errorf(
					"read repository owner %q/%q: %w",
					host.Name(),
					owner.Name(),
					namesErr,
				)
			}
			for _, name := range names {
				if name.Type()&os.ModeSymlink != 0 {
					return nil, fmt.Errorf(
						"repository entry %q/%q/%q is a symlink",
						host.Name(),
						owner.Name(),
						name.Name(),
					)
				}
				if name.Name() == ".aidb" || !name.IsDir() {
					continue
				}
				repositoryLayout, layoutErr := repository.NewLayout(
					organizationLayout,
					organizationManifest.Roles.Repositories,
					host.Name(),
					owner.Name(),
					name.Name(),
				)
				if layoutErr != nil {
					return nil, layoutErr
				}
				repositoryManifest, manifestErr := repository.ReadManifest(
					repositoryLayout.ManifestPath(),
				)
				if manifestErr != nil {
					return nil, manifestErr
				}
				repositoryScope, scopeErr := newRepositoryScope(
					organizationLayout,
					organizationManifest,
					repositoryLayout,
					repositoryManifest,
					false,
				)
				if scopeErr != nil {
					return nil, scopeErr
				}
				tickets, ticketsErr := ReadScopeManifests(
					repositoryScope,
				)
				if ticketsErr != nil {
					return nil, ticketsErr
				}
				result = append(result, tickets...)
			}
		}
	}
	return result, nil
}

func ValidateAvailable(
	layout Layout,
	manifest Manifest,
	existing []LocatedManifest,
) error {
	candidateSelectors := identitySelectors(layout, manifest)
	for _, located := range existing {
		if located.Manifest.ID == manifest.ID {
			return fmt.Errorf(
				"ticket id %q already belongs to %q",
				manifest.ID,
				located.Layout.Root(),
			)
		}
		if filepath.Clean(located.Layout.Root()) ==
			filepath.Clean(layout.Root()) {
			return fmt.Errorf(
				"ticket path %q already belongs to ticket %q",
				layout.Root(),
				located.Manifest.ID,
			)
		}
		if selectorMatches(
			manifest.ID,
			located.Manifest.Aliases,
		) || selectorMatches(
			located.Manifest.ID,
			manifest.Aliases,
		) {
			return errors.New("ticket id collides with a historical alias")
		}
		if !sameScope(layout.Scope(), located.Layout.Scope()) {
			continue
		}
		occupied := identitySelectors(located.Layout, located.Manifest)
		for selector := range candidateSelectors {
			if _, ok := occupied[selector]; ok {
				return fmt.Errorf(
					"ticket selector %q collides with ticket %q",
					candidateSelectors[selector],
					located.Manifest.ID,
				)
			}
		}
	}
	return nil
}

func ValidateCatalog(existing []LocatedManifest) error {
	ids := make(map[string]string, len(existing))
	paths := make(map[string]string, len(existing))
	for _, located := range existing {
		id := foldSelector(located.Manifest.ID)
		if owner, ok := ids[id]; ok {
			return fmt.Errorf(
				"ticket id %q collides with ticket %q",
				located.Manifest.ID,
				owner,
			)
		}
		ids[id] = located.Manifest.ID

		path := filepath.Clean(located.Layout.Root())
		if owner, ok := paths[path]; ok {
			return fmt.Errorf(
				"ticket path %q collides with ticket %q",
				path,
				owner,
			)
		}
		paths[path] = located.Manifest.ID
	}

	scopes := make(map[string]map[string]string)
	for _, located := range existing {
		for _, alias := range located.Manifest.Aliases {
			if owner, ok := ids[foldSelector(alias)]; ok &&
				owner != located.Manifest.ID {
				return fmt.Errorf(
					"ticket alias %q collides with immutable id %q",
					alias,
					owner,
				)
			}
		}
		scopeKey := located.Manifest.OrganizationID + "\x00" +
			located.Manifest.RepositoryID
		selectors, ok := scopes[scopeKey]
		if !ok {
			selectors = make(map[string]string)
			scopes[scopeKey] = selectors
		}
		for selector, display := range identitySelectors(
			located.Layout,
			located.Manifest,
		) {
			if owner, exists := selectors[selector]; exists &&
				owner != located.Manifest.ID {
				return fmt.Errorf(
					"ticket selector %q collides with ticket %q",
					display,
					owner,
				)
			}
			selectors[selector] = located.Manifest.ID
		}
	}
	return nil
}

func BuildProjection(
	manifest Manifest,
	layout Layout,
	activeProfile profile.Profile,
	manifestHash string,
	observedAt time.Time,
) (controlplane.TicketProjection, error) {
	if err := manifest.Validate(layout, activeProfile); err != nil {
		return controlplane.TicketProjection{}, err
	}
	if strings.TrimSpace(manifestHash) == "" {
		return controlplane.TicketProjection{}, errors.New(
			"ticket projection manifest hash is required",
		)
	}
	if observedAt.IsZero() {
		return controlplane.TicketProjection{}, errors.New(
			"ticket projection observed_at is required",
		)
	}

	aliases := append([]string(nil), manifest.Aliases...)
	for _, alias := range manifest.Aliases {
		if !isPortablePathAlias(alias) {
			continue
		}
		root := layout.Scope().OwnerRoot()
		relative := alias
		if workspaceRelative, ok := parseWorkspacePathAlias(alias); ok {
			root = layout.Scope().WorkspaceRoot()
			relative = workspaceRelative
		}
		absolute := filepath.Clean(filepath.Join(
			root,
			filepath.FromSlash(relative),
		))
		if err := validateResolvedContainment(
			root,
			absolute,
		); err != nil {
			return controlplane.TicketProjection{}, err
		}
		aliases = appendUniqueAlias(
			aliases,
			absolute,
			manifest.ID,
			manifest.VisibleKey,
			layout.Root(),
		)
	}
	if manifest.VisibleKey != manifest.LocalKey {
		aliases = appendUniqueAlias(
			aliases,
			manifest.LocalKey,
			manifest.ID,
			manifest.VisibleKey,
			layout.RelativePath(),
			layout.Root(),
		)
	}
	return controlplane.TicketProjection{
		ID:             manifest.ID,
		OrganizationID: manifest.OrganizationID,
		RepositoryID:   manifest.RepositoryID,
		VisibleKey:     manifest.VisibleKey,
		Path:           layout.Root(),
		Status:         string(manifest.Status),
		Type:           manifest.Type,
		Priority:       string(manifest.Priority),
		ProfileID:      manifest.Profile.ID,
		ProfileVersion: manifest.Profile.Version,
		ManifestHash:   manifestHash,
		ArchiveState: controlplane.TicketArchiveState(
			manifest.ArchiveState,
		),
		ObservedAt:   observedAt.UTC(),
		Aliases:      aliases,
		Artifacts:    []controlplane.ArtifactProjection{},
		Sources:      []controlplane.SourceObservation{},
		Dependencies: []controlplane.SourceDependency{},
	}, nil
}

func identitySelectors(
	layout Layout,
	manifest Manifest,
) map[string]string {
	values := append(
		[]string{
			manifest.LocalKey,
			manifest.VisibleKey,
			layout.RelativePath(),
			layout.OwnerRelativePath(),
		},
		manifest.Aliases...,
	)
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[foldSelector(value)] = value
	}
	return result
}

func sameScope(left ScopeLayout, right ScopeLayout) bool {
	return left.OrganizationID() == right.OrganizationID() &&
		left.RepositoryID() == right.RepositoryID()
}

func selectorMatches(selector string, aliases []string) bool {
	folded := foldSelector(selector)
	for _, alias := range aliases {
		if foldSelector(alias) == folded {
			return true
		}
	}
	return false
}

func processWorkspaceMutex(lockPath string) *sync.Mutex {
	value, _ := processWorkspaceLocks.LoadOrStore(
		filepath.Clean(lockPath),
		&sync.Mutex{},
	)
	mutex, ok := value.(*sync.Mutex)
	if !ok {
		// processWorkspaceLocks is package-private and this is its only writer,
		// so a different type means a future edit started storing something
		// else under the same key. Returning a fresh mutex instead would look
		// like it worked while silently dropping the in-process serialisation
		// that keeps two goroutines from contending on one workspace lock file.
		panic(fmt.Sprintf(
			"workspace lock %q holds %T, want *sync.Mutex",
			lockPath,
			value,
		))
	}
	return mutex
}

func mergeLocatedManifests(
	left []LocatedManifest,
	right []LocatedManifest,
) []LocatedManifest {
	result := append([]LocatedManifest(nil), left...)
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, located := range left {
		seen[located.Manifest.ID+"\x00"+located.Layout.Root()] = struct{}{}
	}
	for _, located := range right {
		key := located.Manifest.ID + "\x00" + located.Layout.Root()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, located)
	}
	sort.Slice(result, func(leftIndex int, rightIndex int) bool {
		return result[leftIndex].Layout.Root() <
			result[rightIndex].Layout.Root()
	})
	return result
}

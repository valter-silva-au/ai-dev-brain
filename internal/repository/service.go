package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

var (
	AddDescriptor = capability.Descriptor{
		Capability: "repository.add",
		Version:    "v1",
		Command:    "add",
		Tool:       "adb_repository_add",
		Summary:    "Plan or add a managed repository.",
		Mutating:   true,
	}
	AdoptDescriptor = capability.Descriptor{
		Capability: "repository.adopt",
		Version:    "v1",
		Command:    "adopt",
		Tool:       "adb_repository_adopt",
		Summary:    "Plan or adopt an existing repository clone.",
		Mutating:   true,
	}
	ListDescriptor = capability.Descriptor{
		Capability: "repository.list",
		Version:    "v1",
		Command:    "list",
		Tool:       "adb_repository_list",
		Summary:    "List registered repositories.",
		Mutating:   false,
	}
	ShowDescriptor = capability.Descriptor{
		Capability: "repository.show",
		Version:    "v1",
		Command:    "show",
		Tool:       "adb_repository_show",
		Summary:    "Show one registered repository.",
		Mutating:   false,
	}
	HealthDescriptor = capability.Descriptor{
		Capability: "repository.health",
		Version:    "v1",
		Command:    "health",
		Tool:       "adb_repository_health",
		Summary:    "Classify repository health without mutation.",
		Mutating:   false,
	}
	FetchDescriptor = capability.Descriptor{
		Capability: "repository.fetch",
		Version:    "v1",
		Command:    "fetch",
		Tool:       "adb_repository_fetch",
		Summary:    "Plan or fetch the canonical repository remote.",
		Mutating:   true,
	}
	UpdateDescriptor = capability.Descriptor{
		Capability: "repository.update",
		Version:    "v1",
		Command:    "update",
		Tool:       "adb_repository_update",
		Summary:    "Plan or fast-forward a repository safely.",
		Mutating:   true,
	}
	MoveDescriptor = capability.Descriptor{
		Capability: "repository.move",
		Version:    "v1",
		Command:    "move",
		Tool:       "adb_repository_move",
		Summary:    "Plan or move repository identity and container.",
		Mutating:   true,
	}
	ArchiveDescriptor = capability.Descriptor{
		Capability: "repository.archive",
		Version:    "v1",
		Command:    "archive",
		Tool:       "adb_repository_archive",
		Summary:    "Plan or change repository archive state.",
		Mutating:   true,
	}
	WorktreeListDescriptor = capability.Descriptor{
		Capability: "repository.worktree.list",
		Version:    "v1",
		Command:    "list",
		Tool:       "adb_repository_worktree_list",
		Summary:    "List repository worktrees and ownership.",
		Mutating:   false,
	}
	WorktreeRepairDescriptor = capability.Descriptor{
		Capability: "repository.worktree.repair",
		Version:    "v1",
		Command:    "repair",
		Tool:       "adb_repository_worktree_repair",
		Summary:    "Plan or repair a missing registered worktree.",
		Mutating:   true,
	}
	WorktreePruneDescriptor = capability.Descriptor{
		Capability: "repository.worktree.prune",
		Version:    "v1",
		Command:    "prune",
		Tool:       "adb_repository_worktree_prune",
		Summary:    "Plan or safely prune an inactive worktree.",
		Mutating:   true,
	}
)

type Options struct {
	Clock       func() time.Time
	IDGenerator func() string
	Git         Git
	AfterPlan   func() error
}

type Service struct {
	now       func() time.Time
	newID     func() string
	git       Git
	afterPlan func() error
}

type Data struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	// OrganizationSlug is the owning organization's human selector — the
	// spelling `--org` accepts. It is what makes a workspace-wide `repo list`
	// legible, where OrganizationID alone is an opaque UUID.
	OrganizationSlug string     `json:"organization_slug"`
	Host             string     `json:"host"`
	Owner            string     `json:"owner"`
	Name             string     `json:"name"`
	DisplayName      string     `json:"display_name"`
	Status           string     `json:"status"`
	Path             string     `json:"path"`
	ClonePath        string     `json:"clone_path"`
	ExternalClone    bool       `json:"external_clone"`
	CanonicalRemote  Remote     `json:"canonical_remote"`
	Aliases          []string   `json:"aliases"`
	Roles            Roles      `json:"roles"`
	Provenance       Provenance `json:"provenance"`
	LastMutation     Provenance `json:"last_mutation"`
}

type MutationData struct {
	Repository  Data   `json:"repository"`
	OperationID string `json:"operation_id,omitempty"`
}

func NewService(options Options) (*Service, error) {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.IDGenerator == nil {
		options.IDGenerator = uuid.NewString
	}
	if options.Git == nil {
		options.Git = NewClient(nil)
	}
	if options.Clock == nil {
		return nil, errors.New("repository clock is required")
	}
	if options.IDGenerator == nil {
		return nil, errors.New("repository id generator is required")
	}
	return &Service{
		now:       options.Clock,
		newID:     options.IDGenerator,
		git:       options.Git,
		afterPlan: options.AfterPlan,
	}, nil
}

// repositoryData projects a manifest into the wire type. It takes the owning
// scope rather than the workspace ID (which it never used) so that every path —
// read and mutation alike — can name the organization by its slug as well as its
// immutable ID. A workspace-wide `repo list` crosses organizations, and a row
// identified only by a UUID is one a human can neither recognize nor feed back
// to `--org`.
func repositoryData(
	scope organizationScope,
	layout Layout,
	manifest Manifest,
) Data {
	clonePath := manifest.CanonicalClone.Path
	if !manifest.CanonicalClone.External {
		resolved, err := layout.ResolveRole(manifest.CanonicalClone.Path)
		if err == nil {
			clonePath = resolved
		}
	}
	return Data{
		ID:               manifest.ID,
		OrganizationID:   manifest.OrganizationID,
		OrganizationSlug: scope.manifest.Slug,
		Host:             manifest.Host,
		Owner:            manifest.Owner,
		Name:             manifest.Name,
		DisplayName:      manifest.DisplayName,
		Status:           manifest.Status,
		Path:             layout.Root(),
		ClonePath:        clonePath,
		ExternalClone:    manifest.CanonicalClone.External,
		CanonicalRemote:  manifest.CanonicalRemote,
		Aliases:          append([]string(nil), manifest.Aliases...),
		Roles:            manifest.Roles,
		Provenance:       manifest.Provenance,
		LastMutation:     manifest.LastMutation,
	}
}

func validateActor(actorType string, tool string) error {
	if actorType == "" {
		return errors.New("repository actor type is required")
	}
	if tool == "" {
		return errors.New("repository tool is required")
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

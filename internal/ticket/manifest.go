package ticket

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const (
	ManifestSchema = "aidb.ticket/v1"
	ManifestKind   = "Ticket"

	maxManifestBytes int64 = 1 << 20
)

type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusReview     Status = "review"
	StatusDone       Status = "done"
)

type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

type ArchiveState string

const (
	ArchiveStateActive   ArchiveState = "active"
	ArchiveStateArchived ArchiveState = "archived"
)

type RelationshipType string

const (
	RelationshipRelatesTo  RelationshipType = "relates_to"
	RelationshipPartOf     RelationshipType = "part_of"
	RelationshipBlocks     RelationshipType = "blocks"
	RelationshipDependsOn  RelationshipType = "depends_on"
	RelationshipDuplicates RelationshipType = "duplicates"
)

type Provenance struct {
	OperationID string `yaml:"operation_id" json:"operation_id"`
	ActorType   string `yaml:"actor_type" json:"actor_type"`
	ActorID     string `yaml:"actor_id,omitempty" json:"actor_id,omitempty"`
	Tool        string `yaml:"tool" json:"tool"`
}

type Relationship struct {
	Type     RelationshipType `yaml:"type" json:"type"`
	TargetID string           `yaml:"target_id" json:"target_id"`
}

type Branch struct {
	Slug   string `yaml:"slug" json:"slug"`
	Intent string `yaml:"intent" json:"intent"`
}

type RemoteReference struct {
	Provider string `yaml:"provider" json:"provider"`
	Resource string `yaml:"resource" json:"resource"`
	ID       string `yaml:"id" json:"id"`
	URL      string `yaml:"url,omitempty" json:"url,omitempty"`
	Primary  bool   `yaml:"primary" json:"primary"`
}

type AppendOnlyCheckpoint struct {
	Role   string `yaml:"role" json:"role"`
	Path   string `yaml:"path" json:"path"`
	Length int64  `yaml:"length" json:"length"`
	Hash   string `yaml:"hash" json:"hash"`
}

type Manifest struct {
	SchemaVersion         string                   `yaml:"schema_version" json:"schema_version"`
	Kind                  string                   `yaml:"kind" json:"kind"`
	ID                    string                   `yaml:"id" json:"id"`
	OrganizationID        string                   `yaml:"organization_id" json:"organization_id"`
	RepositoryID          string                   `yaml:"repository_id,omitempty" json:"repository_id,omitempty"`
	LocalKey              string                   `yaml:"local_key" json:"local_key"`
	VisibleKey            string                   `yaml:"visible_key" json:"visible_key"`
	Aliases               []string                 `yaml:"aliases" json:"aliases"`
	Title                 string                   `yaml:"title" json:"title"`
	PathSlug              string                   `yaml:"path_slug" json:"path_slug"`
	Branch                Branch                   `yaml:"branch" json:"branch"`
	Type                  string                   `yaml:"type" json:"type"`
	Status                Status                   `yaml:"status" json:"status"`
	Priority              Priority                 `yaml:"priority" json:"priority"`
	Owner                 string                   `yaml:"owner,omitempty" json:"owner,omitempty"`
	Tags                  []string                 `yaml:"tags" json:"tags"`
	Relationships         []Relationship           `yaml:"relationships" json:"relationships"`
	Profile               profile.ProfileReference `yaml:"profile" json:"profile"`
	RemoteReferences      []RemoteReference        `yaml:"remote_references" json:"remote_references"`
	AppendOnlyCheckpoints []AppendOnlyCheckpoint   `yaml:"append_only_checkpoints" json:"append_only_checkpoints"`
	CreatedAt             time.Time                `yaml:"created_at" json:"created_at"`
	UpdatedAt             time.Time                `yaml:"updated_at" json:"updated_at"`
	ClosedAt              *time.Time               `yaml:"closed_at,omitempty" json:"closed_at,omitempty"`
	Provenance            Provenance               `yaml:"provenance" json:"provenance"`
	LastMutation          Provenance               `yaml:"last_mutation" json:"last_mutation"`
	ArchiveState          ArchiveState             `yaml:"archive_state" json:"archive_state"`
	ArchivedAt            *time.Time               `yaml:"archived_at,omitempty" json:"archived_at,omitempty"`
}

var remoteComponentPattern = regexp.MustCompile(
	`^[a-z][a-z0-9.-]*$`,
)

func NewManifest(
	id string,
	scope ScopeLayout,
	localKey string,
	title string,
	pathSlug string,
	branchSlug string,
	workType string,
	activeProfile profile.Profile,
	createdAt time.Time,
	provenance Provenance,
) (Manifest, error) {
	layout, err := scope.Ticket(localKey, pathSlug)
	if err != nil {
		return Manifest{}, err
	}
	branch, err := BranchIntent(
		workType,
		localKey,
		branchSlug,
		activeProfile,
	)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{
		SchemaVersion:  ManifestSchema,
		Kind:           ManifestKind,
		ID:             id,
		OrganizationID: scope.OrganizationID(),
		RepositoryID:   scope.RepositoryID(),
		LocalKey:       localKey,
		VisibleKey:     localKey,
		Aliases:        []string{},
		Title:          title,
		PathSlug:       pathSlug,
		Branch: Branch{
			Slug:   branchSlug,
			Intent: branch,
		},
		Type:          workType,
		Status:        StatusBacklog,
		Priority:      PriorityP2,
		Tags:          []string{},
		Relationships: []Relationship{},
		Profile: profile.ProfileReference{
			ID:      activeProfile.ID,
			Version: activeProfile.Version,
		},
		RemoteReferences:      []RemoteReference{},
		AppendOnlyCheckpoints: []AppendOnlyCheckpoint{},
		CreatedAt:             createdAt.UTC(),
		UpdatedAt:             createdAt.UTC(),
		Provenance:            provenance,
		LastMutation:          provenance,
		ArchiveState:          ArchiveStateActive,
	}
	if err := manifest.Validate(layout, activeProfile); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) Validate(
	layout Layout,
	activeProfile profile.Profile,
) error {
	if err := manifest.validateCore(layout); err != nil {
		return err
	}
	if err := validateResolvedProfile(activeProfile); err != nil {
		return err
	}
	if manifest.Profile.ID != activeProfile.ID ||
		manifest.Profile.Version != activeProfile.Version {
		return fmt.Errorf(
			"ticket profile %s does not match active profile %s@%s",
			manifest.Profile.String(),
			activeProfile.ID,
			activeProfile.Version,
		)
	}
	branch, err := BranchIntent(
		manifest.Type,
		manifest.LocalKey,
		manifest.Branch.Slug,
		activeProfile,
	)
	if err != nil {
		return err
	}
	if manifest.Branch.Intent != branch {
		return fmt.Errorf(
			"ticket branch intent %q does not match %q",
			manifest.Branch.Intent,
			branch,
		)
	}
	return validateCheckpointsAgainstProfile(
		manifest.AppendOnlyCheckpoints,
		activeProfile,
	)
}

func (manifest Manifest) validateCore(layout Layout) error {
	switch {
	case manifest.SchemaVersion != ManifestSchema:
		return fmt.Errorf(
			"unsupported ticket manifest schema %q",
			manifest.SchemaVersion,
		)
	case manifest.Kind != ManifestKind:
		return fmt.Errorf("ticket manifest kind must be %q", ManifestKind)
	case !validUUID(manifest.ID):
		return errors.New("ticket manifest id must be a UUID")
	case manifest.OrganizationID != layout.Scope().OrganizationID():
		return errors.New(
			"ticket organization id does not match its owning scope",
		)
	case manifest.RepositoryID != layout.Scope().RepositoryID():
		return errors.New(
			"ticket repository id does not match its owning scope",
		)
	case manifest.LocalKey != layout.LocalKey():
		return errors.New("ticket local key does not match its path")
	case manifest.PathSlug != layout.PathSlug():
		return errors.New("ticket path slug does not match its path")
	case strings.TrimSpace(manifest.Title) == "":
		return errors.New("ticket title is required")
	case manifest.Title != strings.TrimSpace(manifest.Title):
		return errors.New("ticket title cannot have surrounding whitespace")
	case containsControl(manifest.Title):
		return errors.New("ticket title contains a control character")
	case manifest.Owner != strings.TrimSpace(manifest.Owner):
		return errors.New("ticket owner cannot have surrounding whitespace")
	case containsControl(manifest.Owner):
		return errors.New("ticket owner contains a control character")
	case !validStatus(manifest.Status):
		return fmt.Errorf("unsupported ticket status %q", manifest.Status)
	case !validPriority(manifest.Priority):
		return fmt.Errorf("unsupported ticket priority %q", manifest.Priority)
	case manifest.CreatedAt.IsZero():
		return errors.New("ticket created_at is required")
	case manifest.UpdatedAt.IsZero():
		return errors.New("ticket updated_at is required")
	case manifest.UpdatedAt.Before(manifest.CreatedAt):
		return errors.New("ticket updated_at precedes created_at")
	case !isUTC(manifest.CreatedAt) || !isUTC(manifest.UpdatedAt):
		return errors.New("ticket timestamps must use UTC")
	case manifest.Status == StatusDone && manifest.ClosedAt == nil:
		return errors.New("done ticket requires closed_at")
	case manifest.Status != StatusDone && manifest.ClosedAt != nil:
		return errors.New("non-done ticket cannot have closed_at")
	case !validArchiveState(manifest.ArchiveState):
		return fmt.Errorf(
			"unsupported ticket archive state %q",
			manifest.ArchiveState,
		)
	case manifest.ArchiveState == ArchiveStateActive &&
		manifest.ArchivedAt != nil:
		return errors.New("active ticket cannot have archived_at")
	case manifest.ArchiveState == ArchiveStateArchived &&
		manifest.ArchivedAt == nil:
		return errors.New("archived ticket requires archived_at")
	}
	if manifest.ArchivedAt != nil {
		if !isUTC(*manifest.ArchivedAt) {
			return errors.New("ticket archived_at must use UTC")
		}
		if manifest.ArchivedAt.Before(manifest.CreatedAt) {
			return errors.New("ticket archived_at precedes created_at")
		}
		if manifest.ArchivedAt.After(manifest.UpdatedAt) {
			return errors.New("ticket archived_at follows updated_at")
		}
	}
	if manifest.ClosedAt != nil {
		if !isUTC(*manifest.ClosedAt) {
			return errors.New("ticket closed_at must use UTC")
		}
		if manifest.ClosedAt.Before(manifest.CreatedAt) {
			return errors.New("ticket closed_at precedes created_at")
		}
		if manifest.ClosedAt.After(manifest.UpdatedAt) {
			return errors.New("ticket closed_at follows updated_at")
		}
	}
	if err := ValidateLocalKey(manifest.LocalKey); err != nil {
		return err
	}
	if err := ValidateVisibleKey(manifest.VisibleKey); err != nil {
		return err
	}
	if err := ValidateSlug(manifest.PathSlug); err != nil {
		return err
	}
	if err := ValidateSlug(manifest.Branch.Slug); err != nil {
		return err
	}
	if _, err := profile.ParseProfileReference(
		manifest.Profile.String(),
	); err != nil {
		return errors.New("ticket profile must be an exact id@version reference")
	}
	if !validWorkType(manifest.Type) {
		return fmt.Errorf("ticket work type %q is invalid", manifest.Type)
	}
	branchType, branchTail, found := strings.Cut(
		manifest.Branch.Intent,
		"/",
	)
	if !found ||
		branchTail != manifest.LocalKey+"-"+manifest.Branch.Slug {
		return errors.New("ticket branch intent has an invalid identity suffix")
	}
	if manifest.Type == "spike" {
		if branchType != "spike" && branchType != "chore" {
			return errors.New("spike ticket branch type must be spike or chore")
		}
	} else if branchType != manifest.Type {
		return errors.New("ticket branch type does not match its work type")
	}
	if err := validateAliases(manifest, layout); err != nil {
		return err
	}
	if err := validateTags(manifest.Tags); err != nil {
		return err
	}
	if err := validateRelationships(manifest); err != nil {
		return err
	}
	if err := validateRemoteReferences(manifest); err != nil {
		return err
	}
	if err := validateAppendOnlyCheckpoints(
		manifest.AppendOnlyCheckpoints,
	); err != nil {
		return err
	}
	if err := validateProvenance("provenance", manifest.Provenance); err != nil {
		return err
	}
	return validateProvenance("last_mutation", manifest.LastMutation)
}

var checkpointHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateAppendOnlyCheckpoints(
	checkpoints []AppendOnlyCheckpoint,
) error {
	seen := make(map[string]struct{}, len(checkpoints))
	for index, checkpoint := range checkpoints {
		switch {
		case strings.TrimSpace(checkpoint.Role) == "":
			return errors.New("append-only checkpoint role is required")
		case checkpoint.Role != strings.TrimSpace(checkpoint.Role):
			return errors.New(
				"append-only checkpoint role has surrounding whitespace",
			)
		case validatePortableRelativeSelector(checkpoint.Path) != nil:
			return fmt.Errorf(
				"append-only checkpoint %q has an invalid path",
				checkpoint.Role,
			)
		case checkpoint.Length < 0:
			return fmt.Errorf(
				"append-only checkpoint %q length cannot be negative",
				checkpoint.Role,
			)
		case !checkpointHashPattern.MatchString(checkpoint.Hash):
			return fmt.Errorf(
				"append-only checkpoint %q has an invalid hash",
				checkpoint.Role,
			)
		}
		if _, exists := seen[checkpoint.Role]; exists {
			return fmt.Errorf(
				"duplicate append-only checkpoint role %q",
				checkpoint.Role,
			)
		}
		if index > 0 && checkpoints[index-1].Role >= checkpoint.Role {
			return errors.New(
				"append-only checkpoints must be sorted by role",
			)
		}
		seen[checkpoint.Role] = struct{}{}
	}
	return nil
}

func validateCheckpointsAgainstProfile(
	checkpoints []AppendOnlyCheckpoint,
	activeProfile profile.Profile,
) error {
	artifacts := make(
		map[string]profile.Artifact,
		len(activeProfile.Artifacts),
	)
	for _, artifact := range activeProfile.Artifacts {
		artifacts[artifact.Role] = artifact
	}
	for _, checkpoint := range checkpoints {
		artifact, exists := artifacts[checkpoint.Role]
		if !exists {
			return fmt.Errorf(
				"append-only checkpoint role %q is not declared by the active profile",
				checkpoint.Role,
			)
		}
		if !artifact.AppendOnly ||
			artifact.Kind != profile.ArtifactFile {
			return fmt.Errorf(
				"append-only checkpoint role %q is not an append-only file",
				checkpoint.Role,
			)
		}
		if checkpoint.Path != artifact.Path {
			return fmt.Errorf(
				"append-only checkpoint role %q path %q does not match profile path %q",
				checkpoint.Role,
				checkpoint.Path,
				artifact.Path,
			)
		}
	}
	return nil
}

func PrepareMutation(
	previous Manifest,
	next Manifest,
	previousLayout Layout,
	nextLayout Layout,
	previousProfile profile.Profile,
	nextProfile profile.Profile,
) (Manifest, error) {
	return prepareMutation(
		previous,
		next,
		previousLayout,
		nextLayout,
		previousProfile,
		nextProfile,
		false,
	)
}

func PrepareCloseMutation(
	previous Manifest,
	next Manifest,
	previousLayout Layout,
	nextLayout Layout,
	previousProfile profile.Profile,
	nextProfile profile.Profile,
) (Manifest, error) {
	return prepareMutation(
		previous,
		next,
		previousLayout,
		nextLayout,
		previousProfile,
		nextProfile,
		true,
	)
}

func prepareMutation(
	previous Manifest,
	next Manifest,
	previousLayout Layout,
	nextLayout Layout,
	previousProfile profile.Profile,
	nextProfile profile.Profile,
	closeMutation bool,
) (Manifest, error) {
	if err := previous.Validate(previousLayout, previousProfile); err != nil {
		return Manifest{}, fmt.Errorf(
			"validate previous ticket manifest: %w",
			err,
		)
	}
	switch {
	case next.ID != previous.ID:
		return Manifest{}, errors.New("ticket immutable id cannot change")
	case next.OrganizationID != previous.OrganizationID:
		return Manifest{}, errors.New(
			"ticket organization ownership cannot change",
		)
	case next.RepositoryID != previous.RepositoryID:
		return Manifest{}, errors.New(
			"ticket repository ownership cannot change",
		)
	case !next.CreatedAt.Equal(previous.CreatedAt):
		return Manifest{}, errors.New("ticket created_at cannot change")
	case next.LocalKey != previous.LocalKey:
		return Manifest{}, errors.New("ticket local key cannot change")
	case !reflect.DeepEqual(next.Provenance, previous.Provenance):
		return Manifest{}, errors.New("ticket provenance cannot change")
	case filepath.Clean(previousLayout.Scope().WorkspaceRoot()) !=
		filepath.Clean(nextLayout.Scope().WorkspaceRoot()):
		return Manifest{}, errors.New("ticket workspace ownership cannot change")
	}
	if manifestMutationChanged(
		previous,
		next,
		previousLayout,
		nextLayout,
	) {
		if !next.UpdatedAt.After(previous.UpdatedAt) {
			return Manifest{}, errors.New(
				"ticket mutation requires a newer updated_at",
			)
		}
		if next.LastMutation.OperationID ==
			previous.LastMutation.OperationID {
			return Manifest{}, errors.New(
				"ticket mutation requires a new last_mutation operation",
			)
		}
	}
	if err := ValidateArchiveTransition(
		previous.ArchiveState,
		next.ArchiveState,
	); err != nil {
		return Manifest{}, err
	}
	if previous.ArchiveState != next.ArchiveState {
		if err := validateArchiveOnlyMutation(previous, next); err != nil {
			return Manifest{}, err
		}
	}
	if previous.Status == StatusDone &&
		!closeMutation &&
		previous.ArchiveState == next.ArchiveState &&
		manifestMutationChanged(
			previous,
			next,
			previousLayout,
			nextLayout,
		) {
		return Manifest{}, errors.New(
			"done ticket is terminal outside archive or restore",
		)
	}
	if closeMutation {
		if err := ValidateCloseTransition(
			previous.Status,
			next.Status,
		); err != nil {
			return Manifest{}, err
		}
	} else {
		if err := ValidateStatusTransition(
			previous.Status,
			next.Status,
		); err != nil {
			return Manifest{}, err
		}
	}
	if previous.ArchiveState == ArchiveStateArchived &&
		next.ArchiveState == ArchiveStateArchived &&
		!reflect.DeepEqual(previous, next) {
		return Manifest{}, errors.New(
			"archived ticket must be restored before ordinary mutation",
		)
	}

	requestedAliases := append([]string(nil), next.Aliases...)
	next.Aliases = append([]string(nil), previous.Aliases...)
	for _, alias := range requestedAliases {
		next.Aliases = appendUniqueAlias(
			next.Aliases,
			alias,
			next.ID,
			next.LocalKey,
			next.VisibleKey,
			nextLayout.RelativePath(),
			nextLayout.OwnerRelativePath(),
		)
	}
	if previous.VisibleKey != next.VisibleKey &&
		foldSelector(previous.VisibleKey) != foldSelector(next.LocalKey) {
		next.Aliases = appendUniqueAlias(
			next.Aliases,
			previous.VisibleKey,
			next.ID,
			next.LocalKey,
			next.VisibleKey,
			nextLayout.RelativePath(),
		)
	}
	if filepath.Clean(previousLayout.Root()) !=
		filepath.Clean(nextLayout.Root()) {
		pathAlias := previousLayout.OwnerRelativePath()
		if filepath.Clean(previousLayout.Scope().OwnerRoot()) !=
			filepath.Clean(nextLayout.Scope().OwnerRoot()) {
			pathAlias = workspacePathAlias(
				previousLayout.WorkspaceRelativePath(),
			)
		}
		next.Aliases = appendUniqueAlias(
			next.Aliases,
			pathAlias,
			next.ID,
			next.LocalKey,
			next.VisibleKey,
			nextLayout.OwnerRelativePath(),
			nextLayout.RelativePath(),
			workspacePathAlias(nextLayout.WorkspaceRelativePath()),
		)
	}
	if err := next.Validate(nextLayout, nextProfile); err != nil {
		return Manifest{}, fmt.Errorf("validate next ticket manifest: %w", err)
	}
	return next, nil
}

func manifestMutationChanged(
	previous Manifest,
	next Manifest,
	previousLayout Layout,
	nextLayout Layout,
) bool {
	if filepath.Clean(previousLayout.Root()) !=
		filepath.Clean(nextLayout.Root()) {
		return true
	}
	left := previous
	right := next
	left.UpdatedAt = time.Time{}
	right.UpdatedAt = time.Time{}
	left.LastMutation = Provenance{}
	right.LastMutation = Provenance{}
	left.AppendOnlyCheckpoints = nil
	right.AppendOnlyCheckpoints = nil
	return !reflect.DeepEqual(left, right)
}

func validateArchiveOnlyMutation(previous Manifest, next Manifest) error {
	left := previous
	right := next
	left.UpdatedAt = time.Time{}
	right.UpdatedAt = time.Time{}
	left.LastMutation = Provenance{}
	right.LastMutation = Provenance{}
	left.ArchiveState = ""
	right.ArchiveState = ""
	left.ArchivedAt = nil
	right.ArchivedAt = nil
	if !reflect.DeepEqual(left, right) {
		return errors.New(
			"archive or restore cannot include ordinary ticket mutations",
		)
	}
	return nil
}

func ValidateStatusTransition(from Status, to Status) error {
	if !validStatus(from) || !validStatus(to) {
		return fmt.Errorf("unsupported ticket status transition %q -> %q", from, to)
	}
	if from == to {
		return nil
	}
	allowed := map[Status]map[Status]struct{}{
		StatusBacklog: {
			StatusInProgress: {},
			StatusBlocked:    {},
		},
		StatusInProgress: {
			StatusBlocked: {},
			StatusReview:  {},
		},
		StatusBlocked: {
			StatusBacklog:    {},
			StatusInProgress: {},
		},
		StatusReview: {
			StatusInProgress: {},
			StatusBlocked:    {},
		},
		StatusDone: {},
	}
	if _, ok := allowed[from][to]; !ok {
		return fmt.Errorf("invalid ticket status transition %q -> %q", from, to)
	}
	return nil
}

func ValidateCloseTransition(from Status, to Status) error {
	if to != StatusDone ||
		(from != StatusInProgress &&
			from != StatusReview &&
			from != StatusDone) {
		return fmt.Errorf(
			"invalid ticket close transition %q -> %q",
			from,
			to,
		)
	}
	return nil
}

func ValidateArchiveTransition(from ArchiveState, to ArchiveState) error {
	if !validArchiveState(from) || !validArchiveState(to) {
		return fmt.Errorf(
			"unsupported ticket archive transition %q -> %q",
			from,
			to,
		)
	}
	return nil
}

func EncodeManifest(writer io.Writer, manifest Manifest) error {
	content, err := encodeManifest(manifest)
	if err != nil {
		return err
	}
	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("write ticket manifest: %w", err)
	}
	return nil
}

func encodeManifest(manifest Manifest) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(manifest); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("encode ticket manifest: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close ticket manifest encoder: %w", err)
	}
	if int64(buffer.Len()) > maxManifestBytes {
		return nil, errors.New("ticket manifest exceeds the supported size limit")
	}
	return buffer.Bytes(), nil
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, errors.New("ticket manifest cannot be read")
	}
	if int64(len(content)) > maxManifestBytes {
		return Manifest{}, errors.New(
			"ticket manifest exceeds the supported size limit",
		)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode ticket manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New(
				"ticket manifest contains multiple YAML documents",
			)
		}
		return Manifest{}, fmt.Errorf(
			"decode ticket manifest trailer: %w",
			err,
		)
	}
	return manifest, nil
}

func validStatus(value Status) bool {
	switch value {
	case StatusBacklog,
		StatusInProgress,
		StatusBlocked,
		StatusReview,
		StatusDone:
		return true
	default:
		return false
	}
}

func validPriority(value Priority) bool {
	switch value {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	default:
		return false
	}
}

func validArchiveState(value ArchiveState) bool {
	return value == ArchiveStateActive || value == ArchiveStateArchived
}

func validRelationshipType(value RelationshipType) bool {
	switch value {
	case RelationshipRelatesTo,
		RelationshipPartOf,
		RelationshipBlocks,
		RelationshipDependsOn,
		RelationshipDuplicates:
		return true
	default:
		return false
	}
}

func validateAliases(manifest Manifest, layout Layout) error {
	current := map[string]struct{}{
		foldSelector(manifest.ID):                {},
		foldSelector(manifest.LocalKey):          {},
		foldSelector(manifest.VisibleKey):        {},
		foldSelector(layout.RelativePath()):      {},
		foldSelector(layout.OwnerRelativePath()): {},
	}
	seen := make(map[string]struct{}, len(manifest.Aliases))
	for _, alias := range manifest.Aliases {
		if err := validatePortableSelector(alias); err != nil {
			return fmt.Errorf("validate ticket alias: %w", err)
		}
		folded := foldSelector(alias)
		if _, ok := current[folded]; ok {
			return fmt.Errorf(
				"ticket alias %q duplicates the current identity",
				alias,
			)
		}
		if _, ok := seen[folded]; ok {
			return fmt.Errorf("duplicate ticket alias %q", alias)
		}
		seen[folded] = struct{}{}
	}
	return nil
}

func validatePortableSelector(value string) error {
	if ValidateVisibleKey(value) == nil {
		return nil
	}
	if strings.Contains(value, "/") {
		return validatePortableRelativeSelector(value)
	}
	switch {
	case value == "":
		return errors.New("ticket selector is required")
	case value != strings.TrimSpace(value):
		return errors.New("ticket selector has surrounding whitespace")
	case len(value) > 160:
		return errors.New("ticket selector exceeds 160 characters")
	case !norm.NFC.IsNormalString(value):
		return errors.New("ticket selector must use NFC normalization")
	case value == "." || value == "..":
		return fmt.Errorf("ticket selector %q is reserved", value)
	case strings.ContainsAny(value, `/\:`):
		return fmt.Errorf("ticket selector %q is not a portable path name", value)
	case strings.HasSuffix(value, ".") || strings.HasSuffix(value, " "):
		return fmt.Errorf(
			"ticket selector %q has an unsafe trailing character",
			value,
		)
	case containsControl(value):
		return fmt.Errorf("ticket selector %q contains a control character", value)
	}
	return nil
}

func validatePortableRelativeSelector(value string) error {
	switch {
	case value == "":
		return errors.New("ticket path selector is required")
	case value != strings.TrimSpace(value):
		return errors.New("ticket path selector has surrounding whitespace")
	case len(value) > 512:
		return errors.New("ticket path selector exceeds 512 characters")
	case !norm.NFC.IsNormalString(value):
		return errors.New("ticket path selector must use NFC normalization")
	case pathpkg.IsAbs(value):
		return errors.New("ticket path selector must be relative")
	case strings.ContainsAny(value, `\:`):
		return errors.New("ticket path selector is not portable")
	case pathpkg.Clean(value) != value ||
		value == "." ||
		value == ".." ||
		strings.HasPrefix(value, "../"):
		return errors.New("ticket path selector contains traversal")
	}
	for _, component := range strings.Split(value, "/") {
		if err := validatePortableSelector(component); err != nil {
			return fmt.Errorf(
				"validate ticket path selector component: %w",
				err,
			)
		}
	}
	return nil
}

func isPortablePathAlias(value string) bool {
	return strings.Contains(value, "/") &&
		validatePortableRelativeSelector(value) == nil
}

func validateTags(tags []string) error {
	seen := make(map[string]struct{}, len(tags))
	for index, tag := range tags {
		switch {
		case tag == "":
			return errors.New("ticket tag cannot be empty")
		case tag != strings.TrimSpace(tag):
			return fmt.Errorf("ticket tag %q has surrounding whitespace", tag)
		case containsControl(tag):
			return fmt.Errorf("ticket tag %q contains a control character", tag)
		}
		folded := foldSelector(tag)
		if _, ok := seen[folded]; ok {
			return fmt.Errorf("duplicate ticket tag %q", tag)
		}
		if index > 0 &&
			foldSelector(tags[index-1]) >= folded {
			return errors.New(
				"ticket tags must be unique and sorted case-insensitively",
			)
		}
		seen[folded] = struct{}{}
	}
	return nil
}

func validateRelationships(manifest Manifest) error {
	seen := make(map[string]struct{}, len(manifest.Relationships))
	for _, relationship := range manifest.Relationships {
		if !validRelationshipType(relationship.Type) {
			return fmt.Errorf(
				"unsupported ticket relationship type %q",
				relationship.Type,
			)
		}
		if relationship.TargetID == "" ||
			relationship.TargetID != strings.TrimSpace(relationship.TargetID) ||
			containsControl(relationship.TargetID) {
			return errors.New("ticket relationship target is invalid")
		}
		if relationship.TargetID == manifest.ID ||
			foldSelector(relationship.TargetID) ==
				foldSelector(manifest.LocalKey) ||
			foldSelector(relationship.TargetID) ==
				foldSelector(manifest.VisibleKey) {
			return errors.New("ticket cannot relate to itself")
		}
		key := string(relationship.Type) + "\x00" +
			foldSelector(relationship.TargetID)
		if _, ok := seen[key]; ok {
			return fmt.Errorf(
				"duplicate ticket relationship %s -> %q",
				relationship.Type,
				relationship.TargetID,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateRemoteReferences(manifest Manifest) error {
	seen := make(map[string]struct{}, len(manifest.RemoteReferences))
	selectors := make(map[string]struct{}, len(manifest.RemoteReferences))
	primarySelector := ""
	for _, reference := range manifest.RemoteReferences {
		if !remoteComponentPattern.MatchString(reference.Provider) {
			return fmt.Errorf(
				"ticket remote provider %q is invalid",
				reference.Provider,
			)
		}
		if !remoteComponentPattern.MatchString(reference.Resource) {
			return fmt.Errorf(
				"ticket remote resource %q is invalid",
				reference.Resource,
			)
		}
		if reference.ID == "" ||
			reference.ID != strings.TrimSpace(reference.ID) ||
			!norm.NFC.IsNormalString(reference.ID) ||
			containsControl(reference.ID) {
			return errors.New("ticket remote id is invalid")
		}
		if reference.URL != "" {
			parsed, err := url.Parse(reference.URL)
			if err != nil ||
				(parsed.Scheme != "https" && parsed.Scheme != "http") ||
				parsed.Host == "" ||
				parsed.User != nil {
				return fmt.Errorf(
					"ticket remote URL %q must be an uncredentialed HTTP(S) URL",
					reference.URL,
				)
			}
		}
		key := reference.Provider + "\x00" +
			reference.Resource + "\x00" +
			foldSelector(reference.ID)
		if _, ok := seen[key]; ok {
			return fmt.Errorf(
				"duplicate ticket remote reference %s:%s:%s",
				reference.Provider,
				reference.Resource,
				reference.ID,
			)
		}
		seen[key] = struct{}{}
		selector := foldSelector(reference.Selector())
		if _, ok := selectors[selector]; ok {
			return fmt.Errorf(
				"duplicate ticket remote selector %q",
				reference.Selector(),
			)
		}
		selectors[selector] = struct{}{}
		if reference.Primary {
			if primarySelector != "" {
				return errors.New(
					"ticket remote references contain multiple primary entries",
				)
			}
			primarySelector = reference.Selector()
		}
	}
	if manifest.VisibleKey != manifest.LocalKey &&
		primarySelector != manifest.VisibleKey {
		return errors.New(
			"remote visible key must match the primary remote reference",
		)
	}
	if primarySelector != "" && manifest.VisibleKey != primarySelector {
		return errors.New(
			"primary remote reference must be the ticket visible key",
		)
	}
	return nil
}

func (reference RemoteReference) Selector() string {
	return reference.Provider + ":" + reference.ID
}

func validateProvenance(name string, value Provenance) error {
	switch {
	case strings.TrimSpace(value.OperationID) == "":
		return fmt.Errorf("ticket %s operation_id is required", name)
	case strings.TrimSpace(value.ActorType) == "":
		return fmt.Errorf("ticket %s actor_type is required", name)
	case strings.TrimSpace(value.Tool) == "":
		return fmt.Errorf("ticket %s tool is required", name)
	case containsControl(value.OperationID) ||
		containsControl(value.ActorType) ||
		containsControl(value.ActorID) ||
		containsControl(value.Tool):
		return fmt.Errorf("ticket %s contains a control character", name)
	}
	return nil
}

func appendUniqueAlias(
	aliases []string,
	value string,
	current ...string,
) []string {
	if value == "" {
		return aliases
	}
	for _, selector := range current {
		if foldSelector(value) == foldSelector(selector) {
			return aliases
		}
	}
	for _, alias := range aliases {
		if foldSelector(alias) == foldSelector(value) {
			return aliases
		}
	}
	return append(aliases, value)
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

package repository

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ManifestSchema = "aidb.repository/v1"
	ManifestKind   = "Repository"
	StatusActive   = "active"
	StatusArchived = "archived"

	RemoteTypeCanonical = "canonical"
	RemoteTypeUpstream  = "upstream"
	RemoteTypeFork      = "fork"
	RemoteTypeMirror    = "mirror"
)

var scpRemotePattern = regexp.MustCompile(
	`^(?:([^@/:]+)@)?([^/:]+):([^\\]+)$`,
)

type Roles struct {
	Config    string `yaml:"config" json:"config"`
	Agents    string `yaml:"agents" json:"agents"`
	Clone     string `yaml:"clone" json:"clone"`
	Tickets   string `yaml:"tickets" json:"tickets"`
	Worktrees string `yaml:"worktrees" json:"worktrees"`
	Knowledge string `yaml:"knowledge" json:"knowledge"`
}

type Provenance struct {
	OperationID string `yaml:"operation_id" json:"operation_id"`
	ActorType   string `yaml:"actor_type" json:"actor_type"`
	ActorID     string `yaml:"actor_id,omitempty" json:"actor_id,omitempty"`
	Tool        string `yaml:"tool" json:"tool"`
}

type Remote struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	FetchURL string `yaml:"fetch_url" json:"fetch_url"`
	PushURL  string `yaml:"push_url,omitempty" json:"push_url,omitempty"`
}

type CloneLocation struct {
	Path     string `yaml:"path" json:"path"`
	External bool   `yaml:"external" json:"external"`
}

type ExternalClone struct {
	Path    string `yaml:"path" json:"path"`
	Purpose string `yaml:"purpose,omitempty" json:"purpose,omitempty"`
}

type Subproject struct {
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
}

type WorktreeRegistration struct {
	TicketKey string `yaml:"ticket_key" json:"ticket_key"`
	Name      string `yaml:"name" json:"name"`
	Path      string `yaml:"path" json:"path"`
	Branch    string `yaml:"branch" json:"branch"`
	Active    bool   `yaml:"active" json:"active"`
}

type Manifest struct {
	SchemaVersion     string                 `yaml:"schema_version" json:"schema_version"`
	Kind              string                 `yaml:"kind" json:"kind"`
	ID                string                 `yaml:"id" json:"id"`
	OrganizationID    string                 `yaml:"organization_id" json:"organization_id"`
	Host              string                 `yaml:"host" json:"host"`
	Owner             string                 `yaml:"owner" json:"owner"`
	Name              string                 `yaml:"name" json:"name"`
	DisplayName       string                 `yaml:"display_name" json:"display_name"`
	CanonicalClone    CloneLocation          `yaml:"canonical_clone" json:"canonical_clone"`
	CanonicalRemote   Remote                 `yaml:"canonical_remote" json:"canonical_remote"`
	AdditionalRemotes []Remote               `yaml:"additional_remotes" json:"additional_remotes"`
	ExternalClones    []ExternalClone        `yaml:"external_clones" json:"external_clones"`
	Subprojects       []Subproject           `yaml:"subprojects" json:"subprojects"`
	Worktrees         []WorktreeRegistration `yaml:"worktrees" json:"worktrees"`
	CreatedAt         time.Time              `yaml:"created_at" json:"created_at"`
	UpdatedAt         time.Time              `yaml:"updated_at" json:"updated_at"`
	Status            string                 `yaml:"status" json:"status"`
	ArchivedAt        *time.Time             `yaml:"archived_at,omitempty" json:"archived_at,omitempty"`
	Aliases           []string               `yaml:"aliases" json:"aliases"`
	Provenance        Provenance             `yaml:"provenance" json:"provenance"`
	LastMutation      Provenance             `yaml:"last_mutation" json:"last_mutation"`
	Roles             Roles                  `yaml:"roles" json:"roles"`
}

type RemoteURL struct {
	Scheme     string
	User       string
	Host       string
	Owner      string
	Repository string
	Normalized string
}

func DefaultRoles() Roles {
	return Roles{
		Config:    ".aidb/config.yaml",
		Agents:    "AGENTS.md",
		Clone:     "repo",
		Tickets:   "tickets",
		Worktrees: "work",
		Knowledge: "knowledge",
	}
}

func NewManifest(
	id string,
	organizationID string,
	layout Layout,
	canonicalRemote Remote,
	createdAt time.Time,
	provenance Provenance,
) Manifest {
	if normalized, err := NormalizeRemoteURL(canonicalRemote.FetchURL); err == nil {
		canonicalRemote.FetchURL = normalized.Normalized
	}
	if canonicalRemote.PushURL != "" {
		if normalized, err := NormalizeRemoteURL(canonicalRemote.PushURL); err == nil {
			canonicalRemote.PushURL = normalized.Normalized
		}
	}
	return Manifest{
		SchemaVersion:  ManifestSchema,
		Kind:           ManifestKind,
		ID:             id,
		OrganizationID: organizationID,
		Host:           layout.Host(),
		Owner:          layout.Owner(),
		Name:           layout.Name(),
		DisplayName:    layout.Name(),
		CanonicalClone: CloneLocation{
			Path:     DefaultRoles().Clone,
			External: false,
		},
		CanonicalRemote:   canonicalRemote,
		AdditionalRemotes: []Remote{},
		ExternalClones:    []ExternalClone{},
		Subprojects:       []Subproject{},
		Worktrees:         []WorktreeRegistration{},
		CreatedAt:         createdAt.UTC(),
		UpdatedAt:         createdAt.UTC(),
		Status:            StatusActive,
		Aliases:           []string{},
		Provenance:        provenance,
		LastMutation:      provenance,
		Roles:             DefaultRoles(),
	}
}

func (manifest Manifest) Validate(layout Layout) error {
	switch {
	case manifest.SchemaVersion != ManifestSchema:
		return fmt.Errorf(
			"unsupported repository manifest schema %q",
			manifest.SchemaVersion,
		)
	case manifest.Kind != ManifestKind:
		return fmt.Errorf("repository manifest kind must be %q", ManifestKind)
	case manifest.ID == "":
		return errors.New("repository manifest id is required")
	case manifest.OrganizationID == "":
		return errors.New("repository organization id is required")
	case manifest.Host != layout.Host() ||
		manifest.Owner != layout.Owner() ||
		manifest.Name != layout.Name():
		return fmt.Errorf(
			"repository manifest identity %q does not match path identity %q",
			manifest.Key(),
			layout.Key(),
		)
	case manifest.DisplayName == "":
		return errors.New("repository display name is required")
	case manifest.CreatedAt.IsZero():
		return errors.New("repository created_at is required")
	case manifest.UpdatedAt.IsZero():
		return errors.New("repository updated_at is required")
	case manifest.UpdatedAt.Before(manifest.CreatedAt):
		return errors.New("repository updated_at precedes created_at")
	case manifest.Status != StatusActive && manifest.Status != StatusArchived:
		return fmt.Errorf("unsupported repository status %q", manifest.Status)
	case manifest.Status == StatusActive && manifest.ArchivedAt != nil:
		return errors.New("active repository cannot have archived_at")
	case manifest.Status == StatusArchived && manifest.ArchivedAt == nil:
		return errors.New("archived repository requires archived_at")
	case manifest.Provenance.OperationID == "":
		return errors.New("repository provenance operation_id is required")
	case manifest.Provenance.ActorType == "":
		return errors.New("repository provenance actor_type is required")
	case manifest.Provenance.Tool == "":
		return errors.New("repository provenance tool is required")
	case manifest.LastMutation.OperationID == "":
		return errors.New("repository last_mutation operation_id is required")
	case manifest.LastMutation.ActorType == "":
		return errors.New("repository last_mutation actor_type is required")
	case manifest.LastMutation.Tool == "":
		return errors.New("repository last_mutation tool is required")
	}
	if err := validateCanonicalRemote(manifest.CanonicalRemote, layout); err != nil {
		return err
	}
	if err := validateCanonicalClone(
		manifest.CanonicalClone,
		manifest.Roles,
		layout,
	); err != nil {
		return err
	}

	remoteNames := map[string]struct{}{
		manifest.CanonicalRemote.Name: {},
	}
	for _, remote := range manifest.AdditionalRemotes {
		if err := validateAdditionalRemote(remote); err != nil {
			return err
		}
		if _, ok := remoteNames[remote.Name]; ok {
			return fmt.Errorf("duplicate repository remote %q", remote.Name)
		}
		remoteNames[remote.Name] = struct{}{}
	}

	externalPaths := make(map[string]struct{}, len(manifest.ExternalClones))
	for _, clone := range manifest.ExternalClones {
		if clone.Path == "" || !filepath.IsAbs(clone.Path) {
			return fmt.Errorf(
				"repository external clone path must be absolute: %q",
				clone.Path,
			)
		}
		path := filepath.Clean(clone.Path)
		if path == layout.CloneDir() {
			return errors.New(
				"repository external clone duplicates the canonical clone",
			)
		}
		if _, ok := externalPaths[path]; ok {
			return fmt.Errorf("duplicate repository external clone %q", path)
		}
		externalPaths[path] = struct{}{}
	}

	subprojectNames := make(map[string]struct{}, len(manifest.Subprojects))
	subprojectPaths := make(map[string]struct{}, len(manifest.Subprojects))
	for _, subproject := range manifest.Subprojects {
		if err := ValidateComponent(subproject.Name); err != nil {
			return fmt.Errorf("validate repository subproject name: %w", err)
		}
		if subproject.Path == "" ||
			filepath.IsAbs(subproject.Path) ||
			filepath.VolumeName(subproject.Path) != "" {
			return fmt.Errorf(
				"repository subproject path must be relative: %q",
				subproject.Path,
			)
		}
		path := filepath.Clean(subproject.Path)
		if path == "." {
			return errors.New(
				"repository subproject cannot represent the repository root",
			)
		}
		if err := requireContainedPath(
			layout.CloneDir(),
			filepath.Join(layout.CloneDir(), path),
		); err != nil {
			return fmt.Errorf("validate repository subproject path: %w", err)
		}
		if _, ok := subprojectNames[subproject.Name]; ok {
			return fmt.Errorf(
				"duplicate repository subproject name %q",
				subproject.Name,
			)
		}
		if _, ok := subprojectPaths[path]; ok {
			return fmt.Errorf("duplicate repository subproject path %q", path)
		}
		subprojectNames[subproject.Name] = struct{}{}
		subprojectPaths[path] = struct{}{}
	}

	worktreeNames := make(map[string]struct{}, len(manifest.Worktrees))
	worktreePaths := make(map[string]struct{}, len(manifest.Worktrees))
	worktreeBranches := make(map[string]struct{}, len(manifest.Worktrees))
	worktreesRoot, err := layout.ResolveRole(manifest.Roles.Worktrees)
	if err != nil {
		return fmt.Errorf("resolve repository worktrees role: %w", err)
	}
	for _, worktree := range manifest.Worktrees {
		if worktree.TicketKey == "" {
			return errors.New("repository worktree ticket key is required")
		}
		if worktree.Name == "" {
			return errors.New("repository worktree name is required")
		}
		if worktree.Path == "" ||
			filepath.IsAbs(worktree.Path) ||
			filepath.VolumeName(worktree.Path) != "" {
			return fmt.Errorf(
				"repository worktree path must be relative: %q",
				worktree.Path,
			)
		}
		resolved := filepath.Join(layout.Root(), filepath.Clean(worktree.Path))
		if err := requireContainedPath(worktreesRoot, resolved); err != nil {
			return fmt.Errorf("validate repository worktree path: %w", err)
		}
		if worktree.Branch == "" {
			return errors.New("repository worktree branch is required")
		}
		nameKey := worktree.TicketKey + "\x00" + worktree.Name
		if _, ok := worktreeNames[nameKey]; ok {
			return fmt.Errorf(
				"duplicate repository worktree %q/%q",
				worktree.TicketKey,
				worktree.Name,
			)
		}
		if _, ok := worktreePaths[resolved]; ok {
			return fmt.Errorf(
				"duplicate repository worktree path %q",
				worktree.Path,
			)
		}
		if _, ok := worktreeBranches[worktree.Branch]; ok {
			return fmt.Errorf(
				"repository branch %q has multiple registered worktrees",
				worktree.Branch,
			)
		}
		worktreeNames[nameKey] = struct{}{}
		worktreePaths[resolved] = struct{}{}
		worktreeBranches[worktree.Branch] = struct{}{}
	}

	seenAliases := make(map[string]struct{}, len(manifest.Aliases))
	for _, alias := range manifest.Aliases {
		if alias == "" {
			return errors.New("repository alias cannot be empty")
		}
		if alias == manifest.Key() || alias == layout.Root() {
			return fmt.Errorf(
				"repository alias %q duplicates current identity",
				alias,
			)
		}
		if _, ok := seenAliases[alias]; ok {
			return fmt.Errorf("duplicate repository alias %q", alias)
		}
		seenAliases[alias] = struct{}{}
	}

	for name, role := range map[string]string{
		"config":    manifest.Roles.Config,
		"agents":    manifest.Roles.Agents,
		"clone":     manifest.Roles.Clone,
		"tickets":   manifest.Roles.Tickets,
		"worktrees": manifest.Roles.Worktrees,
		"knowledge": manifest.Roles.Knowledge,
	} {
		if _, err := layout.ResolveRole(role); err != nil {
			return fmt.Errorf("validate repository role %s: %w", name, err)
		}
	}
	return nil
}

func validateCanonicalClone(
	clone CloneLocation,
	roles Roles,
	layout Layout,
) error {
	if clone.Path == "" {
		return errors.New("repository canonical clone path is required")
	}
	if clone.External {
		if !filepath.IsAbs(clone.Path) {
			return fmt.Errorf(
				"external canonical clone path must be absolute: %q",
				clone.Path,
			)
		}
		if filepath.Clean(clone.Path) == layout.CloneDir() {
			return errors.New(
				"external canonical clone duplicates the managed clone role",
			)
		}
		return nil
	}
	if filepath.IsAbs(clone.Path) || filepath.VolumeName(clone.Path) != "" {
		return fmt.Errorf(
			"managed canonical clone path must be relative: %q",
			clone.Path,
		)
	}
	resolved, err := layout.ResolveRole(clone.Path)
	if err != nil {
		return fmt.Errorf("resolve repository canonical clone: %w", err)
	}
	role, err := layout.ResolveRole(roles.Clone)
	if err != nil {
		return fmt.Errorf("resolve repository clone role: %w", err)
	}
	if resolved != role {
		return fmt.Errorf(
			"repository canonical clone %q does not match clone role %q",
			clone.Path,
			roles.Clone,
		)
	}
	return nil
}

func (manifest Manifest) Key() string {
	return manifest.Host + "/" + manifest.Owner + "/" + manifest.Name
}

func NormalizeRemoteURL(raw string) (RemoteURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RemoteURL{}, errors.New("repository remote URL is required")
	}
	if filepath.VolumeName(raw) != "" || strings.Contains(raw, `\`) {
		return RemoteURL{}, fmt.Errorf(
			"repository remote URL %q resembles a local filesystem path",
			raw,
		)
	}

	if match := scpRemotePattern.FindStringSubmatch(raw); len(match) > 0 &&
		!strings.Contains(raw, "://") {
		return normalizeRemoteParts("ssh", match[1], match[2], match[3])
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return RemoteURL{}, fmt.Errorf("parse repository remote URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "ssh" {
		// Name the offending value and the accepted set. The bare
		// `unsupported repository remote scheme ""` this used to emit was
		// undiagnosable in its most common cause: `adb repo adopt <path>` reads
		// the clone's OWN origin, so a repo cloned from a local path or a
		// file:// URL failed here without the message ever mentioning the
		// origin, the clone, or what would have been acceptable.
		if scheme == "" || scheme == "file" {
			return RemoteURL{}, fmt.Errorf(
				"repository remote %q is a local path or file:// URL; "+
					"a canonical remote identity must be an https or ssh URL "+
					"(or scp-style git@host:owner/name) — set one with "+
					"`git remote set-url origin <https-or-ssh-url>` before adopting",
				raw,
			)
		}
		return RemoteURL{}, fmt.Errorf(
			"unsupported repository remote scheme %q in %q (want https or ssh)",
			parsed.Scheme, raw,
		)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return RemoteURL{}, errors.New(
			"repository remote URL cannot contain query or fragment data",
		)
	}
	if parsed.Port() != "" {
		return RemoteURL{}, errors.New(
			"repository remote URL ports are not supported in canonical identity",
		)
	}
	user := ""
	if parsed.User != nil {
		if _, present := parsed.User.Password(); present || scheme == "https" {
			return RemoteURL{}, errors.New(
				"repository remote URL cannot contain credentials",
			)
		}
		user = parsed.User.Username()
	}
	return normalizeRemoteParts(
		scheme,
		user,
		parsed.Hostname(),
		strings.TrimPrefix(parsed.EscapedPath(), "/"),
	)
}

func normalizeRemoteParts(
	scheme string,
	user string,
	host string,
	path string,
) (RemoteURL, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if err := ValidateHost(host); err != nil {
		return RemoteURL{}, err
	}
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return RemoteURL{}, fmt.Errorf(
			"repository remote path %q must contain owner and repository",
			path,
		)
	}
	owner, err := url.PathUnescape(parts[0])
	if err != nil {
		return RemoteURL{}, fmt.Errorf("decode repository remote owner: %w", err)
	}
	name, err := url.PathUnescape(parts[1])
	if err != nil {
		return RemoteURL{}, fmt.Errorf("decode repository remote name: %w", err)
	}
	if err := ValidateComponent(owner); err != nil {
		return RemoteURL{}, fmt.Errorf("validate repository remote owner: %w", err)
	}
	if err := ValidateComponent(name); err != nil {
		return RemoteURL{}, fmt.Errorf("validate repository remote name: %w", err)
	}
	userPrefix := ""
	if user != "" {
		if strings.ContainsAny(user, "@/:\\") {
			return RemoteURL{}, fmt.Errorf(
				"repository remote user %q is invalid",
				user,
			)
		}
		userPrefix = user + "@"
	}
	return RemoteURL{
		Scheme:     scheme,
		User:       user,
		Host:       host,
		Owner:      owner,
		Repository: name,
		Normalized: scheme + "://" + userPrefix + host + "/" +
			owner + "/" + name + ".git",
	}, nil
}

func validateCanonicalRemote(remote Remote, layout Layout) error {
	if remote.Name == "" {
		return errors.New("repository canonical remote name is required")
	}
	if remote.Type != RemoteTypeCanonical {
		return fmt.Errorf(
			"repository canonical remote type must be %q",
			RemoteTypeCanonical,
		)
	}
	normalized, err := NormalizeRemoteURL(remote.FetchURL)
	if err != nil {
		return fmt.Errorf("validate repository canonical remote: %w", err)
	}
	if normalized.Normalized != remote.FetchURL {
		return errors.New(
			"repository canonical fetch remote must be normalized",
		)
	}
	if normalized.Host != layout.Host() ||
		normalized.Owner != layout.Owner() ||
		normalized.Repository != layout.Name() {
		return fmt.Errorf(
			"repository canonical remote identity %q/%q/%q does not match %q",
			normalized.Host,
			normalized.Owner,
			normalized.Repository,
			layout.Key(),
		)
	}
	if remote.PushURL != "" {
		push, err := NormalizeRemoteURL(remote.PushURL)
		if err != nil {
			return fmt.Errorf("validate repository canonical push remote: %w", err)
		}
		if push.Normalized != remote.PushURL {
			return errors.New(
				"repository canonical push remote must be normalized",
			)
		}
	}
	return nil
}

func validateAdditionalRemote(remote Remote) error {
	if remote.Name == "" {
		return errors.New("repository additional remote name is required")
	}
	switch remote.Type {
	case RemoteTypeUpstream, RemoteTypeFork, RemoteTypeMirror:
	default:
		return fmt.Errorf(
			"unsupported repository additional remote type %q",
			remote.Type,
		)
	}
	normalized, err := NormalizeRemoteURL(remote.FetchURL)
	if err != nil {
		return fmt.Errorf("validate repository remote %q: %w", remote.Name, err)
	}
	if normalized.Normalized != remote.FetchURL {
		return fmt.Errorf(
			"repository remote %q fetch URL must be normalized",
			remote.Name,
		)
	}
	if remote.PushURL != "" {
		push, err := NormalizeRemoteURL(remote.PushURL)
		if err != nil {
			return fmt.Errorf(
				"validate repository remote %q push URL: %w",
				remote.Name,
				err,
			)
		}
		if push.Normalized != remote.PushURL {
			return fmt.Errorf(
				"repository remote %q push URL must be normalized",
				remote.Name,
			)
		}
	}
	return nil
}

func EncodeManifest(writer io.Writer, manifest Manifest) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	defer func() {
		_ = encoder.Close()
	}()
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encode repository manifest: %w", err)
	}
	return nil
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode repository manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New(
				"repository manifest contains multiple documents",
			)
		}
		return Manifest{}, fmt.Errorf(
			"decode repository manifest trailer: %w",
			err,
		)
	}
	return manifest, nil
}

func ReadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf(
			"open repository manifest %q: %w",
			path,
			err,
		)
	}
	defer func() {
		_ = file.Close()
	}()
	manifest, err := DecodeManifest(file)
	if err != nil {
		return Manifest{}, fmt.Errorf(
			"read repository manifest %q: %w",
			path,
			err,
		)
	}
	return manifest, nil
}

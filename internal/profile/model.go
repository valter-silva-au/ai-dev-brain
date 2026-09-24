package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = "aidb.profile/v1"

const maxProfileDocumentBytes int64 = 1 << 20

var errContainedPortablePath = errors.New(
	"path must remain contained within its artifact root",
)

func IsContainedPathError(err error) bool {
	return errors.Is(err, errContainedPortablePath)
}

type ArtifactKind string

const (
	ArtifactFile      ArtifactKind = "file"
	ArtifactDirectory ArtifactKind = "directory"
)

type Authority string

const (
	AuthorityAuthored   Authority = "authored"
	AuthorityGenerated  Authority = "generated"
	AuthorityStructural Authority = "structural"
)

type SearchPolicy string

const (
	SearchNone     SearchPolicy = "none"
	SearchLexical  SearchPolicy = "lexical"
	SearchSemantic SearchPolicy = "semantic"
	SearchHybrid   SearchPolicy = "lexical-semantic"
)

type RetentionPolicy string

const (
	RetentionDurable   RetentionPolicy = "durable"
	RetentionEphemeral RetentionPolicy = "ephemeral"
)

type GitPolicy string

const (
	GitTracked GitPolicy = "tracked"
	GitIgnored GitPolicy = "ignored"
)

type ProfileReference struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

type TemplateReference struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

type Artifact struct {
	Role       string             `yaml:"role" json:"role"`
	Path       string             `yaml:"path" json:"path"`
	Kind       ArtifactKind       `yaml:"kind" json:"kind"`
	Authority  Authority          `yaml:"authority" json:"authority"`
	Required   bool               `yaml:"required" json:"required"`
	AppendOnly bool               `yaml:"append_only" json:"append_only"`
	Search     SearchPolicy       `yaml:"search" json:"search"`
	Retention  RetentionPolicy    `yaml:"retention" json:"retention"`
	Git        GitPolicy          `yaml:"git" json:"git"`
	Template   *TemplateReference `yaml:"template,omitempty" json:"template,omitempty"`
}

type Profile struct {
	SchemaVersion string             `yaml:"schema_version" json:"schema_version"`
	ID            string             `yaml:"id" json:"id"`
	Version       string             `yaml:"version" json:"version"`
	Inherits      []ProfileReference `yaml:"inherits,omitempty" json:"inherits,omitempty"`
	WorkTypes     []string           `yaml:"work_types" json:"work_types"`
	Artifacts     []Artifact         `yaml:"artifacts" json:"artifacts"`
}

var (
	identityPattern = regexp.MustCompile(
		`^[a-z0-9][a-z0-9._/-]*[a-z0-9]$|^[a-z0-9]$`,
	)
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	workTypePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	rolePattern     = regexp.MustCompile(
		`^[a-z0-9][a-z0-9._/-]*[a-z0-9]$|^[a-z0-9]$`,
	)
)

func DecodeProfile(reader io.Reader) (Profile, error) {
	content, err := readBounded(
		reader,
		maxProfileDocumentBytes,
		"profile document",
	)
	if err != nil {
		return Profile{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	var value Profile
	if err := decoder.Decode(&value); err != nil {
		return Profile{}, errors.New("profile document is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Profile{}, errors.New("profile contains multiple YAML documents")
	}
	if err := ValidateProfile(value); err != nil {
		return Profile{}, err
	}
	return cloneProfile(value), nil
}

func readBounded(
	reader io.Reader,
	limit int64,
	documentName string,
) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read", documentName)
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("%s exceeds the supported size limit", documentName)
	}
	return content, nil
}

func ParseProfileReference(value string) (ProfileReference, error) {
	if value != strings.TrimSpace(value) ||
		strings.Count(value, "@") != 1 {
		return ProfileReference{}, errors.New(
			"profile selector must be an exact id@version reference",
		)
	}
	id, version, _ := strings.Cut(value, "@")
	reference := ProfileReference{ID: id, Version: version}
	if err := validateProfileReference(reference); err != nil {
		return ProfileReference{}, err
	}
	return reference, nil
}

func (reference ProfileReference) String() string {
	return reference.ID + "@" + reference.Version
}

func ValidateProfile(value Profile) error {
	if value.SchemaVersion != SchemaVersion {
		return errors.New("unsupported profile schema")
	}
	if err := validateIdentity("profile id", value.ID); err != nil {
		return err
	}
	if err := validateVersion("profile version", value.Version); err != nil {
		return err
	}
	if len(value.Inherits) == 0 && len(value.WorkTypes) == 0 {
		return errors.New("profile requires at least one work type")
	}

	parentKeys := make(map[string]struct{}, len(value.Inherits))
	if len(value.Inherits) > 1 {
		return errors.New(
			"profile inheritance supports exactly one explicit parent",
		)
	}
	for _, parent := range value.Inherits {
		if err := validateProfileReference(parent); err != nil {
			return err
		}
		if parent.ID == value.ID && parent.Version == value.Version {
			return errors.New("profile cannot inherit itself")
		}
		key := referenceKey(parent)
		if _, exists := parentKeys[key]; exists {
			return errors.New("profile contains a duplicate parent")
		}
		parentKeys[key] = struct{}{}
	}

	workTypes := make(map[string]struct{}, len(value.WorkTypes))
	for _, workType := range value.WorkTypes {
		if !workTypePattern.MatchString(workType) {
			return fmt.Errorf("invalid profile work type %q", workType)
		}
		if _, exists := workTypes[workType]; exists {
			return fmt.Errorf("duplicate profile work type %q", workType)
		}
		workTypes[workType] = struct{}{}
	}

	if len(value.Inherits) == 0 && len(value.Artifacts) == 0 {
		return errors.New("profile requires at least one artifact")
	}
	roles := make(map[string]struct{}, len(value.Artifacts))
	for _, artifact := range value.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return err
		}
		if _, exists := roles[artifact.Role]; exists {
			return fmt.Errorf("duplicate artifact role %q", artifact.Role)
		}
		roles[artifact.Role] = struct{}{}
	}
	return validateArtifactLayout(value.Artifacts)
}

func ResolveInheritance(
	catalog []Profile,
	target ProfileReference,
) (Profile, error) {
	if err := validateProfileReference(target); err != nil {
		return Profile{}, err
	}
	byReference := make(map[string]Profile, len(catalog))
	for _, candidate := range catalog {
		if err := ValidateProfile(candidate); err != nil {
			return Profile{}, fmt.Errorf(
				"validate profile %s@%s: %w",
				candidate.ID,
				candidate.Version,
				err,
			)
		}
		key := referenceKey(profileReferenceOf(candidate))
		if _, exists := byReference[key]; exists {
			return Profile{}, fmt.Errorf(
				"duplicate profile identity %s@%s",
				candidate.ID,
				candidate.Version,
			)
		}
		byReference[key] = cloneProfile(candidate)
	}

	resolved := make(map[string]Profile, len(catalog))
	visiting := make(map[string]bool, len(catalog))
	var resolve func(ProfileReference) (Profile, error)
	resolve = func(reference ProfileReference) (Profile, error) {
		key := referenceKey(reference)
		if value, exists := resolved[key]; exists {
			return cloneProfile(value), nil
		}
		if visiting[key] {
			return Profile{}, fmt.Errorf(
				"profile inheritance cycle at %s@%s",
				reference.ID,
				reference.Version,
			)
		}
		current, exists := byReference[key]
		if !exists {
			return Profile{}, fmt.Errorf(
				"profile %s@%s not found",
				reference.ID,
				reference.Version,
			)
		}
		visiting[key] = true

		effective := Profile{
			SchemaVersion: SchemaVersion,
			ID:            current.ID,
			Version:       current.Version,
			WorkTypes:     []string{},
			Artifacts:     []Artifact{},
		}
		for _, parentReference := range current.Inherits {
			parent, err := resolve(parentReference)
			if err != nil {
				return Profile{}, err
			}
			effective = mergeProfile(effective, parent)
		}
		effective = mergeProfile(effective, current)
		effective.ID = current.ID
		effective.Version = current.Version
		effective.Inherits = []ProfileReference{}
		sort.Strings(effective.WorkTypes)
		sort.Slice(effective.Artifacts, func(left, right int) bool {
			return effective.Artifacts[left].Role <
				effective.Artifacts[right].Role
		})
		if err := ValidateProfile(effective); err != nil {
			return Profile{}, fmt.Errorf(
				"validate resolved profile %s@%s: %w",
				current.ID,
				current.Version,
				err,
			)
		}

		visiting[key] = false
		resolved[key] = cloneProfile(effective)
		return cloneProfile(effective), nil
	}

	return resolve(target)
}

func validateArtifact(artifact Artifact) error {
	if !rolePattern.MatchString(artifact.Role) ||
		strings.Contains(artifact.Role, "..") ||
		strings.Contains(artifact.Role, "//") {
		return fmt.Errorf("invalid artifact role %q", artifact.Role)
	}
	if err := validatePortablePath(artifact.Path); err != nil {
		return fmt.Errorf("invalid path for artifact %q: %w", artifact.Role, err)
	}
	switch artifact.Kind {
	case ArtifactFile, ArtifactDirectory:
	default:
		return fmt.Errorf("invalid kind for artifact %q", artifact.Role)
	}
	switch artifact.Authority {
	case AuthorityAuthored, AuthorityGenerated, AuthorityStructural:
	default:
		return fmt.Errorf("invalid authority for artifact %q", artifact.Role)
	}
	switch artifact.Search {
	case SearchNone, SearchLexical, SearchSemantic, SearchHybrid:
	default:
		return fmt.Errorf("invalid search policy for artifact %q", artifact.Role)
	}
	switch artifact.Retention {
	case RetentionDurable, RetentionEphemeral:
	default:
		return fmt.Errorf("invalid retention policy for artifact %q", artifact.Role)
	}
	switch artifact.Git {
	case GitTracked, GitIgnored:
	default:
		return fmt.Errorf("invalid git policy for artifact %q", artifact.Role)
	}
	if artifact.Template != nil {
		if err := validateTemplateReference(*artifact.Template); err != nil {
			return fmt.Errorf("artifact %q: %w", artifact.Role, err)
		}
	}
	if artifact.Kind == ArtifactDirectory {
		if artifact.Authority != AuthorityStructural {
			return fmt.Errorf(
				"directory artifact %q must be structural",
				artifact.Role,
			)
		}
		if artifact.Template != nil {
			return fmt.Errorf(
				"directory artifact %q cannot declare a template",
				artifact.Role,
			)
		}
		if artifact.AppendOnly {
			return fmt.Errorf(
				"directory artifact %q cannot be append-only",
				artifact.Role,
			)
		}
		if artifact.Search != SearchNone {
			return fmt.Errorf(
				"directory artifact %q cannot be searchable",
				artifact.Role,
			)
		}
	}
	if artifact.Authority == AuthorityGenerated && artifact.Template == nil {
		return fmt.Errorf(
			"generated artifact %q requires a template",
			artifact.Role,
		)
	}
	if artifact.AppendOnly && artifact.Authority != AuthorityAuthored {
		return fmt.Errorf(
			"append-only artifact %q must be authored",
			artifact.Role,
		)
	}
	if artifact.Authority == AuthorityStructural &&
		artifact.Search != SearchNone {
		return fmt.Errorf(
			"structural artifact %q cannot be searchable",
			artifact.Role,
		)
	}
	return nil
}

func validatePortablePath(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("path is required")
	}
	if isAbsolutePortablePath(value) ||
		hasPortableParentTraversal(value) {
		return fmt.Errorf(
			"%w: path must be a normalized contained portable path",
			errContainedPortablePath,
		)
	}
	if value != strings.TrimSpace(value) ||
		strings.Contains(value, "\\") ||
		strings.Contains(value, ":") ||
		path.Clean(value) != value {
		return errors.New("path must be a normalized contained portable path")
	}
	if !norm.NFC.IsNormalString(value) {
		return errors.New("path must use NFC Unicode normalization")
	}
	firstComponent := strings.ToLower(strings.SplitN(value, "/", 2)[0])
	if firstComponent == ".aidb" {
		return errors.New("path is reserved for adb metadata")
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" ||
			strings.HasSuffix(component, ".") ||
			strings.HasSuffix(component, " ") ||
			isWindowsDeviceName(component) {
			return errors.New("path is not portable across supported systems")
		}
		for _, character := range component {
			if character < 0x20 ||
				character == '<' ||
				character == '>' ||
				character == '"' ||
				character == '|' ||
				character == '?' ||
				character == '*' ||
				unicode.IsControl(character) {
				return errors.New(
					"path is not portable across supported systems",
				)
			}
		}
	}
	return nil
}

func validateArtifactLayout(artifacts []Artifact) error {
	for left := 0; left < len(artifacts); left++ {
		for right := left + 1; right < len(artifacts); right++ {
			first := artifacts[left]
			second := artifacts[right]
			firstFolded := cases.Fold().String(first.Path)
			secondFolded := cases.Fold().String(second.Path)
			if firstFolded == secondFolded {
				return fmt.Errorf(
					"artifact paths collide across supported filesystems: %q and %q",
					first.Path,
					second.Path,
				)
			}
			if pathDescendsFrom(secondFolded, firstFolded) {
				if first.Kind != ArtifactDirectory ||
					!pathDescendsFrom(second.Path, first.Path) {
					return fmt.Errorf(
						"artifact path %q conflicts with parent %q",
						second.Path,
						first.Path,
					)
				}
			}
			if pathDescendsFrom(firstFolded, secondFolded) {
				if second.Kind != ArtifactDirectory ||
					!pathDescendsFrom(first.Path, second.Path) {
					return fmt.Errorf(
						"artifact path %q conflicts with parent %q",
						first.Path,
						second.Path,
					)
				}
			}
		}
	}
	return nil
}

func pathDescendsFrom(candidate string, parent string) bool {
	return strings.HasPrefix(candidate, parent+"/")
}

func validateResolvedProfile(value Profile) error {
	if err := ValidateProfile(value); err != nil {
		return err
	}
	if len(value.Inherits) != 0 {
		return errors.New(
			"profile inheritance must be resolved before this operation",
		)
	}
	return nil
}

func hasPortableParentTraversal(value string) bool {
	slashed := strings.ReplaceAll(value, "\\", "/")
	return slashed == "." ||
		slashed == ".." ||
		strings.HasPrefix(slashed, "../") ||
		strings.Contains(slashed, "/../") ||
		strings.HasSuffix(slashed, "/..")
}

func isAbsolutePortablePath(value string) bool {
	if strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "\\") {
		return true
	}
	if len(value) < 3 ||
		value[1] != ':' ||
		(value[2] != '/' && value[2] != '\\') {
		return false
	}
	drive := value[0]
	return drive >= 'A' && drive <= 'Z' ||
		drive >= 'a' && drive <= 'z'
}

func isWindowsDeviceName(component string) bool {
	base := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 &&
		(base[:3] == "COM" || base[:3] == "LPT") &&
		base[3] >= '1' &&
		base[3] <= '9' {
		return true
	}
	return false
}

func validateProfileReference(reference ProfileReference) error {
	if err := validateIdentity("profile reference id", reference.ID); err != nil {
		return err
	}
	return validateVersion("profile reference version", reference.Version)
}

func validateTemplateReference(reference TemplateReference) error {
	if err := validateIdentity("template id", reference.ID); err != nil {
		return err
	}
	return validateVersion("template version", reference.Version)
}

func validateIdentity(field string, value string) error {
	if !identityPattern.MatchString(value) ||
		strings.Contains(value, "..") ||
		strings.Contains(value, "//") {
		return fmt.Errorf("invalid %s", field)
	}
	return nil
}

func validateVersion(field string, value string) error {
	if !versionPattern.MatchString(value) {
		return fmt.Errorf("invalid %s", field)
	}
	return nil
}

func mergeProfile(base Profile, overlay Profile) Profile {
	result := cloneProfile(base)
	workTypes := make(map[string]struct{}, len(result.WorkTypes))
	for _, workType := range result.WorkTypes {
		workTypes[workType] = struct{}{}
	}
	for _, workType := range overlay.WorkTypes {
		if _, exists := workTypes[workType]; !exists {
			result.WorkTypes = append(result.WorkTypes, workType)
			workTypes[workType] = struct{}{}
		}
	}
	indexByRole := make(map[string]int, len(result.Artifacts))
	for index, artifact := range result.Artifacts {
		indexByRole[artifact.Role] = index
	}
	for _, artifact := range overlay.Artifacts {
		if index, exists := indexByRole[artifact.Role]; exists {
			result.Artifacts[index] = cloneArtifact(artifact)
			continue
		}
		result.Artifacts = append(result.Artifacts, cloneArtifact(artifact))
		indexByRole[artifact.Role] = len(result.Artifacts) - 1
	}
	return result
}

func cloneProfile(value Profile) Profile {
	result := value
	result.Inherits = append([]ProfileReference(nil), value.Inherits...)
	result.WorkTypes = append([]string(nil), value.WorkTypes...)
	result.Artifacts = make([]Artifact, 0, len(value.Artifacts))
	for _, artifact := range value.Artifacts {
		result.Artifacts = append(result.Artifacts, cloneArtifact(artifact))
	}
	return result
}

func cloneArtifact(value Artifact) Artifact {
	result := value
	if value.Template != nil {
		template := *value.Template
		result.Template = &template
	}
	return result
}

func profileReferenceOf(value Profile) ProfileReference {
	return ProfileReference{ID: value.ID, Version: value.Version}
}

func referenceKey(reference ProfileReference) string {
	return reference.ID + "\x00" + reference.Version
}

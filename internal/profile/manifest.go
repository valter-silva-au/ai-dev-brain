package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	RenderManifestSchemaVersion          = "aidb.rendered/v1"
	TombstoneManifestSchemaVersion       = "aidb.tombstones/v1"
	maxManifestDocumentBytes       int64 = 1 << 20
)

type TemplateProvenance struct {
	ID          string `yaml:"id" json:"id"`
	Version     string `yaml:"version" json:"version"`
	SourceScope string `yaml:"source_scope" json:"source_scope"`
}

type RenderedRecord struct {
	Role          string             `yaml:"role" json:"role"`
	Path          string             `yaml:"path" json:"path"`
	Template      TemplateProvenance `yaml:"template" json:"template"`
	GeneratedHash string             `yaml:"generated_hash" json:"generated_hash"`
	ObservedHash  string             `yaml:"observed_hash" json:"observed_hash"`
}

type RenderManifest struct {
	SchemaVersion string           `yaml:"schema_version" json:"schema_version"`
	Profile       ProfileReference `yaml:"profile" json:"profile"`
	Artifacts     []RenderedRecord `yaml:"artifacts" json:"artifacts"`
}

type Tombstone struct {
	Actor     string `yaml:"actor" json:"actor"`
	Reason    string `yaml:"reason" json:"reason"`
	Scope     string `yaml:"scope" json:"scope"`
	Role      string `yaml:"role" json:"role"`
	Path      string `yaml:"path" json:"path"`
	RemovedAt string `yaml:"removed_at" json:"removed_at"`
}

type TombstoneManifest struct {
	SchemaVersion string      `yaml:"schema_version" json:"schema_version"`
	Tombstones    []Tombstone `yaml:"tombstones" json:"tombstones"`
}

type Observation struct {
	Role    string
	Path    string
	Kind    ArtifactKind
	Exists  bool
	Content []byte
}

type UpgradeDisposition string

const (
	UpgradeUnchanged  UpgradeDisposition = "unchanged"
	UpgradeCreate     UpgradeDisposition = "create"
	UpgradeUpdate     UpgradeDisposition = "update"
	UpgradePreserve   UpgradeDisposition = "preserve"
	UpgradeReview     UpgradeDisposition = "review"
	UpgradeTombstoned UpgradeDisposition = "tombstoned"
)

type UpgradeChange struct {
	Role        string             `json:"role" yaml:"role"`
	CurrentPath string             `json:"current_path,omitempty" yaml:"current_path,omitempty"`
	TargetPath  string             `json:"target_path,omitempty" yaml:"target_path,omitempty"`
	Disposition UpgradeDisposition `json:"disposition" yaml:"disposition"`
	Safe        bool               `json:"safe" yaml:"safe"`
	Reason      string             `json:"reason" yaml:"reason"`
}

type UpgradePlan struct {
	Preview bool             `json:"preview" yaml:"preview"`
	From    ProfileReference `json:"from" yaml:"from"`
	To      ProfileReference `json:"to" yaml:"to"`
	Changes []UpgradeChange  `json:"changes" yaml:"changes"`
}

type UpgradeRequest struct {
	Scope           string
	Current         Profile
	Target          Profile
	CurrentManifest RenderManifest
	TargetRender    RenderResult
	Observations    []Observation
	Tombstones      TombstoneManifest
}

var contentHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func DecodeRenderManifest(reader io.Reader) (RenderManifest, error) {
	var value RenderManifest
	if err := decodeStrictYAML(reader, &value, "render manifest"); err != nil {
		return RenderManifest{}, err
	}
	if err := ValidateRenderManifest(value); err != nil {
		return RenderManifest{}, err
	}
	sort.Slice(value.Artifacts, func(left, right int) bool {
		return value.Artifacts[left].Role < value.Artifacts[right].Role
	})
	return cloneRenderManifest(value), nil
}

func EncodeRenderManifest(writer io.Writer, value RenderManifest) error {
	if err := ValidateRenderManifest(value); err != nil {
		return err
	}
	normalized := cloneRenderManifest(value)
	sort.Slice(normalized.Artifacts, func(left, right int) bool {
		return normalized.Artifacts[left].Role <
			normalized.Artifacts[right].Role
	})
	return encodeYAML(writer, normalized)
}

func ValidateRenderManifest(value RenderManifest) error {
	if value.SchemaVersion != RenderManifestSchemaVersion {
		return errors.New("unsupported render manifest schema")
	}
	if err := validateProfileReference(value.Profile); err != nil {
		return err
	}
	roles := make(map[string]struct{}, len(value.Artifacts))
	paths := make(map[string]struct{}, len(value.Artifacts))
	layout := make([]Artifact, 0, len(value.Artifacts))
	for _, artifact := range value.Artifacts {
		if !rolePattern.MatchString(artifact.Role) ||
			strings.Contains(artifact.Role, "..") ||
			strings.Contains(artifact.Role, "//") {
			return errors.New("render manifest contains an invalid role")
		}
		if err := validatePortablePath(artifact.Path); err != nil {
			return fmt.Errorf(
				"render manifest role %q has an invalid path: %w",
				artifact.Role,
				err,
			)
		}
		if err := validateTemplateReference(TemplateReference{
			ID:      artifact.Template.ID,
			Version: artifact.Template.Version,
		}); err != nil {
			return err
		}
		if strings.TrimSpace(artifact.Template.SourceScope) == "" ||
			artifact.Template.SourceScope !=
				strings.TrimSpace(artifact.Template.SourceScope) {
			return errors.New(
				"render manifest template source scope is required",
			)
		}
		if !validTemplateSourceScope(artifact.Template.SourceScope) {
			return errors.New(
				"render manifest template source scope is not recognized",
			)
		}
		if !contentHashPattern.MatchString(artifact.GeneratedHash) ||
			!contentHashPattern.MatchString(artifact.ObservedHash) {
			return errors.New("render manifest contains an invalid content hash")
		}
		if _, exists := roles[artifact.Role]; exists {
			return errors.New("render manifest contains a duplicate role")
		}
		roles[artifact.Role] = struct{}{}
		if _, exists := paths[artifact.Path]; exists {
			return errors.New("render manifest contains a duplicate path")
		}
		paths[artifact.Path] = struct{}{}
		layout = append(layout, Artifact{
			Role: artifact.Role,
			Path: artifact.Path,
			Kind: ArtifactFile,
		})
	}
	return validateArtifactLayout(layout)
}

func DecodeTombstones(reader io.Reader) (TombstoneManifest, error) {
	var value TombstoneManifest
	if err := decodeStrictYAML(reader, &value, "tombstone manifest"); err != nil {
		return TombstoneManifest{}, err
	}
	if err := ValidateTombstones(value); err != nil {
		return TombstoneManifest{}, err
	}
	return cloneTombstoneManifest(value), nil
}

func EncodeTombstones(writer io.Writer, value TombstoneManifest) error {
	if err := ValidateTombstones(value); err != nil {
		return err
	}
	return encodeYAML(writer, cloneTombstoneManifest(value))
}

func ValidateTombstones(value TombstoneManifest) error {
	if value.SchemaVersion != TombstoneManifestSchemaVersion {
		return errors.New("unsupported tombstone manifest schema")
	}
	for _, tombstone := range value.Tombstones {
		if err := validateTombstone(tombstone); err != nil {
			return err
		}
	}
	return nil
}

func AddTombstone(
	manifest TombstoneManifest,
	tombstone Tombstone,
) (TombstoneManifest, error) {
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = TombstoneManifestSchemaVersion
	}
	if err := ValidateTombstones(manifest); err != nil {
		return TombstoneManifest{}, err
	}
	if err := validateTombstone(tombstone); err != nil {
		return TombstoneManifest{}, err
	}
	result := cloneTombstoneManifest(manifest)
	result.Tombstones = append(result.Tombstones, tombstone)
	return result, nil
}

func PlanUpgrade(request UpgradeRequest) (UpgradePlan, error) {
	if strings.TrimSpace(request.Scope) == "" ||
		request.Scope != strings.TrimSpace(request.Scope) {
		return UpgradePlan{}, errors.New("upgrade scope is required")
	}
	if err := validateResolvedProfile(request.Current); err != nil {
		return UpgradePlan{}, fmt.Errorf("validate current profile: %w", err)
	}
	if err := validateResolvedProfile(request.Target); err != nil {
		return UpgradePlan{}, fmt.Errorf("validate target profile: %w", err)
	}
	if request.Current.ID != request.Target.ID {
		return UpgradePlan{}, errors.New(
			"profile upgrade cannot change profile identity",
		)
	}
	if request.Current.Version == request.Target.Version {
		return UpgradePlan{}, errors.New(
			"profile upgrade requires a different target version",
		)
	}
	if err := ValidateRenderManifest(request.CurrentManifest); err != nil {
		return UpgradePlan{}, fmt.Errorf("validate current manifest: %w", err)
	}
	if request.CurrentManifest.Profile != profileReferenceOf(request.Current) {
		return UpgradePlan{}, errors.New(
			"current manifest does not match the current profile",
		)
	}
	if err := validateManifestAgainstProfile(
		request.Current,
		request.CurrentManifest,
	); err != nil {
		return UpgradePlan{}, fmt.Errorf(
			"validate current profile provenance: %w",
			err,
		)
	}
	if err := validateRenderResult(request.TargetRender); err != nil {
		return UpgradePlan{}, fmt.Errorf("validate target render: %w", err)
	}
	if request.TargetRender.Manifest.Profile != profileReferenceOf(request.Target) {
		return UpgradePlan{}, errors.New(
			"target render does not match the target profile",
		)
	}
	if err := validateManifestAgainstProfile(
		request.Target,
		request.TargetRender.Manifest,
	); err != nil {
		return UpgradePlan{}, fmt.Errorf(
			"validate target profile provenance: %w",
			err,
		)
	}
	if request.Tombstones.SchemaVersion != "" ||
		len(request.Tombstones.Tombstones) > 0 {
		if err := ValidateTombstones(request.Tombstones); err != nil {
			return UpgradePlan{}, fmt.Errorf("validate tombstones: %w", err)
		}
	}

	observations := make(map[string]Observation, len(request.Observations))
	for _, observation := range request.Observations {
		if !rolePattern.MatchString(observation.Role) {
			return UpgradePlan{}, errors.New("observation has an invalid role")
		}
		if err := validatePortablePath(observation.Path); err != nil {
			return UpgradePlan{}, fmt.Errorf(
				"observation %q has an invalid path: %w",
				observation.Role,
				err,
			)
		}
		if observation.Kind != ArtifactFile &&
			observation.Kind != ArtifactDirectory {
			return UpgradePlan{}, errors.New("observation has an invalid kind")
		}
		if _, exists := observations[observation.Role]; exists {
			return UpgradePlan{}, errors.New(
				"upgrade observations contain a duplicate role",
			)
		}
		observation.Content = append([]byte(nil), observation.Content...)
		observations[observation.Role] = observation
	}

	currentArtifacts := artifactsByRole(request.Current.Artifacts)
	targetArtifacts := artifactsByRole(request.Target.Artifacts)
	currentRecords := recordsByRole(request.CurrentManifest.Artifacts)
	targetRecords := recordsByRole(request.TargetRender.Manifest.Artifacts)
	tombstoned := matchingTombstones(
		request.Tombstones,
		request.Scope,
	)

	roles := make([]string, 0, len(currentArtifacts)+len(targetArtifacts))
	seen := make(map[string]struct{}, len(currentArtifacts)+len(targetArtifacts))
	for role := range currentArtifacts {
		roles = append(roles, role)
		seen[role] = struct{}{}
	}
	for role := range targetArtifacts {
		if _, exists := seen[role]; !exists {
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)

	plan := UpgradePlan{
		Preview: true,
		From:    profileReferenceOf(request.Current),
		To:      profileReferenceOf(request.Target),
		Changes: make([]UpgradeChange, 0, len(roles)),
	}
	for _, role := range roles {
		current, hasCurrent := currentArtifacts[role]
		target, hasTarget := targetArtifacts[role]
		observation, observed := observations[role]

		if !hasTarget {
			if observed && observation.Exists {
				plan.Changes = append(plan.Changes, UpgradeChange{
					Role:        role,
					CurrentPath: current.Path,
					Disposition: UpgradePreserve,
					Safe:        false,
					Reason:      "artifact is not declared by the target profile; removal requires explicit review",
				})
			}
			continue
		}
		if _, removed := tombstoned[role]; removed && !target.Required {
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				TargetPath:  target.Path,
				Disposition: UpgradeTombstoned,
				Safe:        false,
				Reason:      "optional artifact has an intentional tombstone in this scope",
			})
			continue
		}
		if hasCurrent && current.Path != target.Path {
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: current.Path,
				TargetPath:  target.Path,
				Disposition: UpgradeReview,
				Safe:        false,
				Reason:      "semantic role path changed and requires a reviewed move",
			})
			continue
		}
		if target.Kind == ArtifactDirectory {
			plan.Changes = append(
				plan.Changes,
				planDirectoryUpgrade(target, observation, observed),
			)
			continue
		}
		if !observed {
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				TargetPath:  target.Path,
				Disposition: UpgradeReview,
				Safe:        false,
				Reason:      "artifact observation is missing or incomplete",
			})
			continue
		}
		if observation.Path != target.Path ||
			observation.Kind != target.Kind {
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: observation.Path,
				TargetPath:  target.Path,
				Disposition: UpgradeReview,
				Safe:        false,
				Reason:      "observed artifact kind or path does not match the target profile",
			})
			continue
		}
		if !observation.Exists {
			disposition := UpgradeUnchanged
			safe := false
			reason := "optional artifact is absent"
			if target.Required {
				disposition = UpgradeCreate
				safe = target.Authority != AuthorityAuthored
				reason = "required artifact is missing"
				if target.Authority == AuthorityAuthored {
					disposition = UpgradeReview
					reason = "missing authored artifact requires reviewed creation"
				}
			}
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				TargetPath:  target.Path,
				Disposition: disposition,
				Safe:        safe,
				Reason:      reason,
			})
			continue
		}
		if hasCurrent && current.Authority != target.Authority {
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: observation.Path,
				TargetPath:  target.Path,
				Disposition: UpgradeReview,
				Safe:        false,
				Reason:      "artifact authority changed and requires explicit review",
			})
			continue
		}
		switch target.Authority {
		case AuthorityAuthored:
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: observation.Path,
				TargetPath:  target.Path,
				Disposition: UpgradePreserve,
				Safe:        false,
				Reason:      "authored content is preserved for review",
			})
		case AuthorityGenerated:
			currentRecord, hasCurrentRecord := currentRecords[role]
			targetRecord, hasTargetRecord := targetRecords[role]
			if !hasCurrentRecord || !hasTargetRecord {
				plan.Changes = append(plan.Changes, UpgradeChange{
					Role:        role,
					CurrentPath: observation.Path,
					TargetPath:  target.Path,
					Disposition: UpgradeReview,
					Safe:        false,
					Reason:      "generated artifact lacks complete render provenance",
				})
				continue
			}
			observedHash := HashContent(observation.Content)
			if observedHash != currentRecord.GeneratedHash {
				plan.Changes = append(plan.Changes, UpgradeChange{
					Role:        role,
					CurrentPath: observation.Path,
					TargetPath:  target.Path,
					Disposition: UpgradePreserve,
					Safe:        false,
					Reason:      "generated artifact has customized or diverged content",
				})
				continue
			}
			disposition := UpgradeUpdate
			reason := "generated artifact matches its baseline and can be updated"
			if observedHash == targetRecord.GeneratedHash {
				disposition = UpgradeUnchanged
				reason = "generated artifact already matches the target"
			}
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: observation.Path,
				TargetPath:  target.Path,
				Disposition: disposition,
				Safe:        true,
				Reason:      reason,
			})
		case AuthorityStructural:
			plan.Changes = append(plan.Changes, UpgradeChange{
				Role:        role,
				CurrentPath: observation.Path,
				TargetPath:  target.Path,
				Disposition: UpgradeUnchanged,
				Safe:        true,
				Reason:      "required structure already exists",
			})
		}
	}
	return plan, nil
}

func planDirectoryUpgrade(
	target Artifact,
	observation Observation,
	observed bool,
) UpgradeChange {
	change := UpgradeChange{
		Role:       target.Role,
		TargetPath: target.Path,
	}
	if !observed {
		change.Disposition = UpgradeReview
		change.Reason = "directory observation is missing or incomplete"
		return change
	}
	change.CurrentPath = observation.Path
	if observation.Path != target.Path ||
		observation.Kind != ArtifactDirectory {
		change.Disposition = UpgradeReview
		change.Reason = "observed artifact does not match the declared directory"
		return change
	}
	if !observation.Exists {
		if target.Required {
			change.Disposition = UpgradeCreate
			change.Safe = true
			change.Reason = "required structural directory is missing"
		} else {
			change.Disposition = UpgradeUnchanged
			change.Reason = "optional structural directory is absent"
		}
		return change
	}
	change.Disposition = UpgradeUnchanged
	change.Safe = true
	change.Reason = "structural directory already exists"
	return change
}

func validateRenderResult(value RenderResult) error {
	if err := ValidateRenderManifest(value.Manifest); err != nil {
		return err
	}
	records := recordsByRole(value.Manifest.Artifacts)
	seen := make(map[string]struct{}, len(value.Files))
	for _, file := range value.Files {
		if _, exists := seen[file.Role]; exists {
			return errors.New("render result contains a duplicate file role")
		}
		seen[file.Role] = struct{}{}
		record, exists := records[file.Role]
		if !exists ||
			record.Path != file.Path ||
			record.GeneratedHash != HashContent(file.Content) ||
			record.ObservedHash != record.GeneratedHash {
			return errors.New(
				"render result file does not match its manifest record",
			)
		}
	}
	if len(seen) != len(records) {
		return errors.New("render result is missing a manifested file")
	}
	return nil
}

func validateManifestAgainstProfile(
	profile Profile,
	manifest RenderManifest,
) error {
	expected := make(map[string]Artifact)
	for _, artifact := range profile.Artifacts {
		if artifact.Template != nil {
			expected[artifact.Role] = artifact
		}
	}
	records := recordsByRole(manifest.Artifacts)
	if len(records) != len(expected) {
		return errors.New(
			"render manifest does not cover every template-managed artifact",
		)
	}
	for role, artifact := range expected {
		record, exists := records[role]
		if !exists ||
			record.Path != artifact.Path ||
			record.Template.ID != artifact.Template.ID ||
			record.Template.Version != artifact.Template.Version {
			return fmt.Errorf(
				"render provenance does not match profile role %q",
				role,
			)
		}
	}
	return nil
}

func validateTombstone(value Tombstone) error {
	if strings.TrimSpace(value.Actor) == "" ||
		value.Actor != strings.TrimSpace(value.Actor) {
		return errors.New("tombstone actor is required")
	}
	if strings.TrimSpace(value.Reason) == "" ||
		value.Reason != strings.TrimSpace(value.Reason) {
		return errors.New("tombstone reason is required")
	}
	if strings.TrimSpace(value.Scope) == "" ||
		value.Scope != strings.TrimSpace(value.Scope) {
		return errors.New("tombstone scope is required")
	}
	if !rolePattern.MatchString(value.Role) ||
		strings.Contains(value.Role, "..") ||
		strings.Contains(value.Role, "//") {
		return errors.New("tombstone role is invalid")
	}
	if err := validatePortablePath(value.Path); err != nil {
		return fmt.Errorf("tombstone path is invalid: %w", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, value.RemovedAt); err != nil {
		return errors.New("tombstone removal time must be RFC3339")
	}
	return nil
}

func decodeStrictYAML(
	reader io.Reader,
	target any,
	documentName string,
) error {
	content, err := readBounded(
		reader,
		maxManifestDocumentBytes,
		documentName,
	)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s is invalid", documentName)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s contains multiple YAML documents", documentName)
	}
	return nil
}

func encodeYAML(writer io.Writer, value any) error {
	buffer := boundedBuffer{limit: int(maxManifestDocumentBytes)}
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return errors.New("encode YAML document")
	}
	if err := encoder.Close(); err != nil {
		return errors.New("close YAML encoder")
	}
	written, err := writer.Write(buffer.Bytes())
	if err != nil {
		return errors.New("write YAML document")
	}
	if written != buffer.Len() {
		return io.ErrShortWrite
	}
	return nil
}

func artifactsByRole(values []Artifact) map[string]Artifact {
	result := make(map[string]Artifact, len(values))
	for _, value := range values {
		result[value.Role] = cloneArtifact(value)
	}
	return result
}

func recordsByRole(values []RenderedRecord) map[string]RenderedRecord {
	result := make(map[string]RenderedRecord, len(values))
	for _, value := range values {
		result[value.Role] = value
	}
	return result
}

func matchingTombstones(
	manifest TombstoneManifest,
	scope string,
) map[string]struct{} {
	result := make(map[string]struct{})
	for _, tombstone := range manifest.Tombstones {
		if tombstone.Scope == scope {
			result[tombstone.Role] = struct{}{}
		}
	}
	return result
}

func cloneRenderManifest(value RenderManifest) RenderManifest {
	result := value
	result.Artifacts = append([]RenderedRecord(nil), value.Artifacts...)
	return result
}

func cloneTombstoneManifest(value TombstoneManifest) TombstoneManifest {
	result := value
	result.Tombstones = append([]Tombstone(nil), value.Tombstones...)
	return result
}

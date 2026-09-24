package profile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ConformanceMode string

const (
	ConformanceOff        ConformanceMode = "off"
	ConformanceObserve    ConformanceMode = "observe"
	ConformanceWarn       ConformanceMode = "warn"
	ConformanceBlock      ConformanceMode = "block"
	ConformanceRepairSafe ConformanceMode = "repair-safe"
)

type ConformanceClass string

const (
	ConformanceMissingRequired     ConformanceClass = "missing_required"
	ConformanceTombstonedOptional  ConformanceClass = "tombstoned_optional"
	ConformanceStaleProfile        ConformanceClass = "stale_profile"
	ConformanceStaleTemplate       ConformanceClass = "stale_template"
	ConformanceAuthoredCustomized  ConformanceClass = "authored_customized"
	ConformanceGeneratedDrift      ConformanceClass = "generated_drift"
	ConformanceAppendOnlyViolation ConformanceClass = "append_only_violation"
	ConformanceInvalidMetadata     ConformanceClass = "invalid_metadata"
	ConformanceContainedPathError  ConformanceClass = "contained_path_error"
	ConformanceConfigurationError  ConformanceClass = "configuration_error"
)

type ConformanceSeverity string

const (
	ConformanceInfo    ConformanceSeverity = "info"
	ConformanceWarning ConformanceSeverity = "warning"
	ConformanceError   ConformanceSeverity = "error"
)

type ArtifactObservation struct {
	Role           string       `json:"role" yaml:"role"`
	Path           string       `json:"path" yaml:"path"`
	Kind           ArtifactKind `json:"kind" yaml:"kind"`
	Exists         bool         `json:"exists" yaml:"exists"`
	Hash           string       `json:"hash,omitempty" yaml:"hash,omitempty"`
	Size           int64        `json:"size,omitempty" yaml:"size,omitempty"`
	BaselineLength int64        `json:"baseline_length,omitempty" yaml:"baseline_length,omitempty"`
	BaselineHash   string       `json:"baseline_hash,omitempty" yaml:"baseline_hash,omitempty"`
	PrefixHash     string       `json:"prefix_hash,omitempty" yaml:"prefix_hash,omitempty"`
}

type ConformanceIssue struct {
	Code    string           `json:"code" yaml:"code"`
	Class   ConformanceClass `json:"class" yaml:"class"`
	Role    string           `json:"role,omitempty" yaml:"role,omitempty"`
	Path    string           `json:"path,omitempty" yaml:"path,omitempty"`
	Summary string           `json:"summary" yaml:"summary"`
}

type ConformanceFinding struct {
	Code         string              `json:"code" yaml:"code"`
	Class        ConformanceClass    `json:"class" yaml:"class"`
	Severity     ConformanceSeverity `json:"severity" yaml:"severity"`
	Role         string              `json:"role,omitempty" yaml:"role,omitempty"`
	Path         string              `json:"path,omitempty" yaml:"path,omitempty"`
	Authority    Authority           `json:"authority,omitempty" yaml:"authority,omitempty"`
	Required     bool                `json:"required" yaml:"required"`
	SafeRepair   bool                `json:"safe_repair" yaml:"safe_repair"`
	Violation    bool                `json:"violation" yaml:"violation"`
	CurrentHash  string              `json:"current_hash,omitempty" yaml:"current_hash,omitempty"`
	ExpectedHash string              `json:"expected_hash,omitempty" yaml:"expected_hash,omitempty"`
	DesiredHash  string              `json:"desired_hash,omitempty" yaml:"desired_hash,omitempty"`
	Summary      string              `json:"summary" yaml:"summary"`
}

type ConformanceRequest struct {
	Scope          string
	Mode           ConformanceMode
	CurrentProfile Profile
	ActiveProfile  Profile
	RenderManifest RenderManifest
	DesiredRender  RenderResult
	Tombstones     TombstoneManifest
	Observations   []ArtifactObservation
	Issues         []ConformanceIssue
}

type ConformanceReport struct {
	Mode       ConformanceMode      `json:"mode" yaml:"mode"`
	Conformant bool                 `json:"conformant" yaml:"conformant"`
	Blocked    bool                 `json:"blocked" yaml:"blocked"`
	Attention  bool                 `json:"attention" yaml:"attention"`
	Findings   []ConformanceFinding `json:"findings" yaml:"findings"`
}

func CheckConformance(
	request ConformanceRequest,
) (ConformanceReport, error) {
	if strings.TrimSpace(request.Scope) == "" ||
		request.Scope != strings.TrimSpace(request.Scope) {
		return ConformanceReport{}, errors.New("conformance scope is required")
	}
	if !validConformanceMode(request.Mode) {
		return ConformanceReport{}, fmt.Errorf(
			"unsupported conformance mode %q",
			request.Mode,
		)
	}
	if err := validateResolvedProfile(request.CurrentProfile); err != nil {
		return ConformanceReport{}, fmt.Errorf(
			"validate current conformance profile: %w",
			err,
		)
	}
	if err := validateResolvedProfile(request.ActiveProfile); err != nil {
		return ConformanceReport{}, fmt.Errorf(
			"validate active conformance profile: %w",
			err,
		)
	}
	if err := ValidateRenderManifest(request.RenderManifest); err != nil {
		return ConformanceReport{}, fmt.Errorf(
			"validate conformance render manifest: %w",
			err,
		)
	}
	if request.RenderManifest.Profile !=
		profileReferenceOf(request.CurrentProfile) {
		return ConformanceReport{}, errors.New(
			"conformance render manifest does not match current profile",
		)
	}
	if err := validateManifestAgainstProfile(
		request.CurrentProfile,
		request.RenderManifest,
	); err != nil {
		return ConformanceReport{}, err
	}
	if err := validateRenderResult(request.DesiredRender); err != nil {
		return ConformanceReport{}, fmt.Errorf(
			"validate desired conformance render: %w",
			err,
		)
	}
	if request.DesiredRender.Manifest.Profile !=
		profileReferenceOf(request.ActiveProfile) {
		return ConformanceReport{}, errors.New(
			"desired conformance render does not match active profile",
		)
	}
	if request.Tombstones.SchemaVersion != "" ||
		len(request.Tombstones.Tombstones) > 0 {
		if err := ValidateTombstones(request.Tombstones); err != nil {
			return ConformanceReport{}, err
		}
	}

	observations, err := conformanceObservations(request.Observations)
	if err != nil {
		return ConformanceReport{}, err
	}
	currentRecords := recordsByRole(request.RenderManifest.Artifacts)
	desiredRecords := recordsByRole(request.DesiredRender.Manifest.Artifacts)
	desiredFiles := renderedFilesByRole(request.DesiredRender.Files)
	tombstoned := matchingTombstones(request.Tombstones, request.Scope)

	findings := make([]ConformanceFinding, 0)
	for _, issue := range request.Issues {
		finding, issueErr := conformanceIssueFinding(issue)
		if issueErr != nil {
			return ConformanceReport{}, issueErr
		}
		findings = append(findings, finding)
	}
	if profileReferenceOf(request.CurrentProfile) !=
		profileReferenceOf(request.ActiveProfile) {
		findings = append(findings, ConformanceFinding{
			Code:      "profile.stale",
			Class:     ConformanceStaleProfile,
			Violation: true,
			Summary:   "The stored profile reference differs from the active profile.",
		})
	}

	artifacts := append([]Artifact(nil), request.ActiveProfile.Artifacts...)
	sort.Slice(artifacts, func(left int, right int) bool {
		return artifacts[left].Role < artifacts[right].Role
	})
	for _, artifact := range artifacts {
		observation, observed := observations[artifact.Role]
		if !observed {
			findings = append(findings, ConformanceFinding{
				Code:      "artifact.observation_missing",
				Class:     ConformanceInvalidMetadata,
				Role:      artifact.Role,
				Path:      artifact.Path,
				Authority: artifact.Authority,
				Required:  artifact.Required,
				Violation: true,
				Summary:   "The artifact observation is missing.",
			})
			continue
		}
		if observation.Path != artifact.Path ||
			observation.Kind != artifact.Kind {
			findings = append(findings, ConformanceFinding{
				Code:      "artifact.identity_mismatch",
				Class:     ConformanceInvalidMetadata,
				Role:      artifact.Role,
				Path:      observation.Path,
				Authority: artifact.Authority,
				Required:  artifact.Required,
				Violation: true,
				Summary:   "The observed artifact path or kind differs from the active profile.",
			})
			continue
		}
		if _, removed := tombstoned[artifact.Role]; removed && !artifact.Required && !observation.Exists {
			findings = append(findings, ConformanceFinding{
				Code:      "artifact.tombstoned_optional",
				Class:     ConformanceTombstonedOptional,
				Role:      artifact.Role,
				Path:      artifact.Path,
				Authority: artifact.Authority,
				Required:  false,
				Violation: false,
				Summary:   "The optional artifact is intentionally tombstoned.",
			})
			continue
		}
		if !observation.Exists {
			if artifact.Required {
				_, hasDesired := desiredFiles[artifact.Role]
				safe := artifact.Kind == ArtifactDirectory ||
					artifact.Authority == AuthorityStructural ||
					(artifact.Authority != AuthorityAuthored && hasDesired)
				findings = append(findings, ConformanceFinding{
					Code:       "artifact.missing_required",
					Class:      ConformanceMissingRequired,
					Role:       artifact.Role,
					Path:       artifact.Path,
					Authority:  artifact.Authority,
					Required:   true,
					SafeRepair: safe,
					Violation:  true,
					Summary:    "A required profile artifact is missing.",
				})
			}
			continue
		}
		if artifact.Kind == ArtifactDirectory {
			continue
		}
		if artifact.AppendOnly {
			if observation.BaselineLength < 0 ||
				!contentHashPattern.MatchString(observation.BaselineHash) ||
				!contentHashPattern.MatchString(observation.PrefixHash) {
				findings = append(findings, ConformanceFinding{
					Code:      "append_only.baseline_invalid",
					Class:     ConformanceInvalidMetadata,
					Role:      artifact.Role,
					Path:      artifact.Path,
					Authority: artifact.Authority,
					Required:  artifact.Required,
					Violation: true,
					Summary:   "Append-only baseline metadata is missing or invalid.",
				})
			} else if observation.Size < observation.BaselineLength ||
				observation.PrefixHash != observation.BaselineHash {
				findings = append(findings, ConformanceFinding{
					Code:        "append_only.violation",
					Class:       ConformanceAppendOnlyViolation,
					Role:        artifact.Role,
					Path:        artifact.Path,
					Authority:   artifact.Authority,
					Required:    artifact.Required,
					Violation:   true,
					CurrentHash: observation.Hash,
					Summary:     "Append-only content was truncated or its accepted prefix changed.",
				})
			}
			continue
		}
		currentRecord, hasCurrentRecord := currentRecords[artifact.Role]
		desiredRecord, hasDesiredRecord := desiredRecords[artifact.Role]
		if artifact.Template != nil &&
			(!hasCurrentRecord || !hasDesiredRecord) {
			findings = append(findings, ConformanceFinding{
				Code:      "render.provenance_missing",
				Class:     ConformanceInvalidMetadata,
				Role:      artifact.Role,
				Path:      artifact.Path,
				Authority: artifact.Authority,
				Required:  artifact.Required,
				Violation: true,
				Summary:   "Managed artifact render provenance is incomplete.",
			})
			continue
		}
		if !hasCurrentRecord {
			continue
		}
		if artifact.Authority == AuthorityGenerated &&
			observation.Hash != currentRecord.GeneratedHash {
			desiredHash := ""
			if hasDesiredRecord &&
				(currentRecord.Template != desiredRecord.Template ||
					currentRecord.GeneratedHash != desiredRecord.GeneratedHash) {
				desiredHash = desiredRecord.GeneratedHash
			}
			findings = append(findings, ConformanceFinding{
				Code:         "generated.drift",
				Class:        ConformanceGeneratedDrift,
				Role:         artifact.Role,
				Path:         artifact.Path,
				Authority:    artifact.Authority,
				Required:     artifact.Required,
				Violation:    true,
				CurrentHash:  observation.Hash,
				ExpectedHash: currentRecord.GeneratedHash,
				DesiredHash:  desiredHash,
				Summary:      "Generated content diverged from its recorded baseline.",
			})
			continue
		}
		if artifact.Authority == AuthorityAuthored &&
			observation.Hash != currentRecord.GeneratedHash {
			findings = append(findings, ConformanceFinding{
				Code:         "authored.customized",
				Class:        ConformanceAuthoredCustomized,
				Role:         artifact.Role,
				Path:         artifact.Path,
				Authority:    artifact.Authority,
				Required:     artifact.Required,
				Violation:    false,
				CurrentHash:  observation.Hash,
				ExpectedHash: currentRecord.GeneratedHash,
				Summary:      "Authored content differs from its bootstrap rendering.",
			})
		}
		if hasDesiredRecord &&
			(currentRecord.Template != desiredRecord.Template ||
				currentRecord.GeneratedHash != desiredRecord.GeneratedHash) {
			safe := artifact.Authority == AuthorityGenerated &&
				observation.Hash == currentRecord.GeneratedHash
			findings = append(findings, ConformanceFinding{
				Code:         "render.stale_template",
				Class:        ConformanceStaleTemplate,
				Role:         artifact.Role,
				Path:         artifact.Path,
				Authority:    artifact.Authority,
				Required:     artifact.Required,
				SafeRepair:   safe,
				Violation:    true,
				CurrentHash:  observation.Hash,
				ExpectedHash: desiredRecord.GeneratedHash,
				Summary:      "Managed artifact rendering is stale.",
			})
		}
	}
	return applyConformanceMode(request.Mode, findings), nil
}

func ReportIssues(
	mode ConformanceMode,
	issues []ConformanceIssue,
) (ConformanceReport, error) {
	if !validConformanceMode(mode) {
		return ConformanceReport{}, fmt.Errorf(
			"unsupported conformance mode %q",
			mode,
		)
	}
	findings := make([]ConformanceFinding, 0, len(issues))
	for _, issue := range issues {
		finding, err := conformanceIssueFinding(issue)
		if err != nil {
			return ConformanceReport{}, err
		}
		findings = append(findings, finding)
	}
	return applyConformanceMode(mode, findings), nil
}

func validConformanceMode(mode ConformanceMode) bool {
	switch mode {
	case ConformanceOff,
		ConformanceObserve,
		ConformanceWarn,
		ConformanceBlock,
		ConformanceRepairSafe:
		return true
	default:
		return false
	}
}

func conformanceObservations(
	values []ArtifactObservation,
) (map[string]ArtifactObservation, error) {
	result := make(map[string]ArtifactObservation, len(values))
	for _, value := range values {
		if !rolePattern.MatchString(value.Role) {
			return nil, errors.New("conformance observation has an invalid role")
		}
		if err := validatePortablePath(value.Path); err != nil {
			return nil, err
		}
		if value.Kind != ArtifactFile && value.Kind != ArtifactDirectory {
			return nil, errors.New("conformance observation has an invalid kind")
		}
		if value.Exists && value.Kind == ArtifactFile &&
			!contentHashPattern.MatchString(value.Hash) {
			return nil, errors.New(
				"existing conformance file observation requires a content hash",
			)
		}
		if _, exists := result[value.Role]; exists {
			return nil, errors.New(
				"conformance observations contain a duplicate role",
			)
		}
		result[value.Role] = value
	}
	return result, nil
}

// alwaysReportedConformanceClass reports whether a class is one of the three
// integrity classes that are reported in every conformance mode — including
// "off" — and always block. Every class is listed explicitly so that adding a
// class to the enum trips the exhaustive linter here rather than silently
// landing in the "not always reported" half.
func alwaysReportedConformanceClass(class ConformanceClass) bool {
	switch class {
	case ConformanceInvalidMetadata,
		ConformanceContainedPathError,
		ConformanceConfigurationError:
		return true
	case ConformanceMissingRequired,
		ConformanceTombstonedOptional,
		ConformanceStaleProfile,
		ConformanceStaleTemplate,
		ConformanceAuthoredCustomized,
		ConformanceGeneratedDrift,
		ConformanceAppendOnlyViolation:
		return false
	}
	return false
}

func conformanceIssueFinding(
	issue ConformanceIssue,
) (ConformanceFinding, error) {
	if strings.TrimSpace(issue.Code) == "" ||
		strings.TrimSpace(issue.Summary) == "" {
		return ConformanceFinding{}, errors.New(
			"conformance issue requires code and summary",
		)
	}
	if !alwaysReportedConformanceClass(issue.Class) {
		return ConformanceFinding{}, errors.New(
			"conformance issue class is not an always-reported class",
		)
	}
	return ConformanceFinding{
		Code:      issue.Code,
		Class:     issue.Class,
		Severity:  ConformanceError,
		Role:      issue.Role,
		Path:      issue.Path,
		Violation: true,
		Summary:   issue.Summary,
	}, nil
}

func applyConformanceMode(
	mode ConformanceMode,
	findings []ConformanceFinding,
) ConformanceReport {
	result := ConformanceReport{
		Mode:       mode,
		Conformant: true,
		Findings:   make([]ConformanceFinding, 0, len(findings)),
	}
	for _, finding := range findings {
		always := alwaysReportedConformanceClass(finding.Class)
		if mode == ConformanceOff && !always {
			continue
		}
		if finding.Violation {
			result.Conformant = false
		}
		switch {
		case always:
			finding.Severity = ConformanceError
			result.Blocked = true
			result.Attention = true
		case !finding.Violation:
			finding.Severity = ConformanceInfo
		case mode == ConformanceObserve:
			finding.Severity = ConformanceInfo
		case mode == ConformanceWarn ||
			mode == ConformanceRepairSafe:
			finding.Severity = ConformanceWarning
			result.Attention = true
		case mode == ConformanceBlock:
			finding.Severity = ConformanceError
			result.Blocked = true
			result.Attention = true
		}
		result.Findings = append(result.Findings, finding)
	}
	sort.Slice(result.Findings, func(left int, right int) bool {
		if result.Findings[left].Role != result.Findings[right].Role {
			return result.Findings[left].Role < result.Findings[right].Role
		}
		if result.Findings[left].Class != result.Findings[right].Class {
			return result.Findings[left].Class < result.Findings[right].Class
		}
		if result.Findings[left].Code != result.Findings[right].Code {
			return result.Findings[left].Code < result.Findings[right].Code
		}
		if result.Findings[left].Path != result.Findings[right].Path {
			return result.Findings[left].Path < result.Findings[right].Path
		}
		return result.Findings[left].Summary <
			result.Findings[right].Summary
	})
	return result
}

func renderedFilesByRole(
	files []RenderedFile,
) map[string]RenderedFile {
	result := make(map[string]RenderedFile, len(files))
	for _, file := range files {
		result[file.Role] = RenderedFile{
			Role:    file.Role,
			Path:    file.Path,
			Content: append([]byte(nil), file.Content...),
		}
	}
	return result
}

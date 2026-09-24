package profile

import (
	"errors"
	"sort"
)

type ReconcileAction string

const (
	ReconcileCreateDirectory  ReconcileAction = "create_directory"
	ReconcileCreateFile       ReconcileAction = "create_file"
	ReconcileReplaceGenerated ReconcileAction = "replace_generated"
	ReconcileReview           ReconcileAction = "review"
)

type ReconcileEffect struct {
	Action       ReconcileAction `json:"action" yaml:"action"`
	Role         string          `json:"role,omitempty" yaml:"role,omitempty"`
	Path         string          `json:"path,omitempty" yaml:"path,omitempty"`
	Safe         bool            `json:"safe" yaml:"safe"`
	BeforeHash   string          `json:"before_hash,omitempty" yaml:"before_hash,omitempty"`
	BaselineHash string          `json:"baseline_hash,omitempty" yaml:"baseline_hash,omitempty"`
	AfterHash    string          `json:"after_hash,omitempty" yaml:"after_hash,omitempty"`
	Reason       string          `json:"reason" yaml:"reason"`
	Content      []byte          `json:"-" yaml:"-"`
}

type ReconcilePlan struct {
	Preview bool              `json:"preview" yaml:"preview"`
	Blocked bool              `json:"blocked" yaml:"blocked"`
	Effects []ReconcileEffect `json:"effects" yaml:"effects"`
}

func PlanReconciliation(
	request ConformanceRequest,
	report ConformanceReport,
) (ReconcilePlan, error) {
	if report.Mode != request.Mode {
		return ReconcilePlan{}, errors.New(
			"conformance report mode does not match reconciliation request",
		)
	}
	artifacts := artifactsByRole(request.ActiveProfile.Artifacts)
	desiredFiles := renderedFilesByRole(request.DesiredRender.Files)
	plan := ReconcilePlan{
		Preview: true,
		Blocked: report.Blocked,
		Effects: make([]ReconcileEffect, 0, len(report.Findings)),
	}
	staleProfile := false
	for _, finding := range report.Findings {
		if finding.Class == ConformanceStaleProfile {
			staleProfile = true
			break
		}
	}
	for _, finding := range report.Findings {
		artifact, hasArtifact := artifacts[finding.Role]
		switch finding.Class {
		case ConformanceTombstonedOptional:
			continue
		case ConformanceMissingRequired:
			if staleProfile {
				plan.Effects = append(
					plan.Effects,
					reviewReconcileEffect(finding),
				)
				continue
			}
			if !hasArtifact {
				continue
			}
			if artifact.Kind == ArtifactDirectory && finding.SafeRepair {
				plan.Effects = append(plan.Effects, ReconcileEffect{
					Action: ReconcileCreateDirectory,
					Role:   finding.Role,
					Path:   finding.Path,
					Safe:   true,
					Reason: finding.Summary,
				})
				continue
			}
			if finding.SafeRepair {
				if file, exists := desiredFiles[finding.Role]; exists {
					plan.Effects = append(plan.Effects, ReconcileEffect{
						Action:    ReconcileCreateFile,
						Role:      finding.Role,
						Path:      finding.Path,
						Safe:      true,
						AfterHash: HashContent(file.Content),
						Reason:    finding.Summary,
						Content:   append([]byte(nil), file.Content...),
					})
					continue
				}
				if artifact.Authority == AuthorityStructural &&
					artifact.Kind == ArtifactFile {
					plan.Effects = append(plan.Effects, ReconcileEffect{
						Action:    ReconcileCreateFile,
						Role:      finding.Role,
						Path:      finding.Path,
						Safe:      true,
						AfterHash: HashContent(nil),
						Reason:    finding.Summary,
						Content:   []byte{},
					})
					continue
				}
			}
			plan.Effects = append(
				plan.Effects,
				reviewReconcileEffect(finding),
			)
		case ConformanceStaleTemplate:
			if staleProfile {
				plan.Effects = append(
					plan.Effects,
					reviewReconcileEffect(finding),
				)
				continue
			}
			if finding.SafeRepair {
				file, exists := desiredFiles[finding.Role]
				if exists {
					plan.Effects = append(plan.Effects, ReconcileEffect{
						Action:     ReconcileReplaceGenerated,
						Role:       finding.Role,
						Path:       finding.Path,
						Safe:       true,
						BeforeHash: finding.CurrentHash,
						AfterHash:  finding.ExpectedHash,
						Reason:     finding.Summary,
						Content:    append([]byte(nil), file.Content...),
					})
					continue
				}
			}
			plan.Effects = append(
				plan.Effects,
				reviewReconcileEffect(finding),
			)
		case ConformanceAuthoredCustomized,
			ConformanceGeneratedDrift,
			ConformanceAppendOnlyViolation,
			ConformanceStaleProfile:
			plan.Effects = append(
				plan.Effects,
				reviewReconcileEffect(finding),
			)
		case ConformanceInvalidMetadata,
			ConformanceContainedPathError,
			ConformanceConfigurationError:
			// The always-reported integrity classes plan no effect, and this is
			// deliberate rather than an omission: any one of them makes the
			// report Blocked, which this plan carries, and a blocked plan is
			// never applied. There is nothing to repair — the workspace's own
			// metadata or containment is broken — so a "review" effect would
			// only restate a finding the caller already has.
			continue
		}
	}
	sort.Slice(plan.Effects, func(left int, right int) bool {
		if plan.Effects[left].Role != plan.Effects[right].Role {
			return plan.Effects[left].Role < plan.Effects[right].Role
		}
		if plan.Effects[left].Action != plan.Effects[right].Action {
			return plan.Effects[left].Action < plan.Effects[right].Action
		}
		if plan.Effects[left].Path != plan.Effects[right].Path {
			return plan.Effects[left].Path < plan.Effects[right].Path
		}
		if plan.Effects[left].Reason != plan.Effects[right].Reason {
			return plan.Effects[left].Reason < plan.Effects[right].Reason
		}
		if plan.Effects[left].BeforeHash != plan.Effects[right].BeforeHash {
			return plan.Effects[left].BeforeHash <
				plan.Effects[right].BeforeHash
		}
		if plan.Effects[left].BaselineHash !=
			plan.Effects[right].BaselineHash {
			return plan.Effects[left].BaselineHash <
				plan.Effects[right].BaselineHash
		}
		return plan.Effects[left].AfterHash <
			plan.Effects[right].AfterHash
	})
	return plan, nil
}

func reviewReconcileEffect(
	finding ConformanceFinding,
) ReconcileEffect {
	afterHash := finding.ExpectedHash
	baselineHash := ""
	if finding.Class == ConformanceGeneratedDrift {
		baselineHash = finding.ExpectedHash
		if finding.DesiredHash != "" {
			afterHash = finding.DesiredHash
		}
	}
	return ReconcileEffect{
		Action:       ReconcileReview,
		Role:         finding.Role,
		Path:         finding.Path,
		Safe:         false,
		BeforeHash:   finding.CurrentHash,
		BaselineHash: baselineHash,
		AfterHash:    afterHash,
		Reason:       finding.Summary,
	}
}

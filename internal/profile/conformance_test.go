package profile

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckConformanceClassifiesArtifactStatesDeterministically(t *testing.T) {
	t.Parallel()

	request := conformanceFixture(t)
	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check conformance: %v", err)
	}
	if report.Conformant || report.Blocked || !report.Attention {
		t.Fatalf("report state = %#v", report)
	}
	assertConformanceFinding(
		t,
		report,
		"ticket.artifacts",
		ConformanceMissingRequired,
		true,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.context",
		ConformanceAuthoredCustomized,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.generated_custom",
		ConformanceGeneratedDrift,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.generated_safe",
		ConformanceStaleTemplate,
		true,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.missing_authored",
		ConformanceMissingRequired,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.notes",
		ConformanceAppendOnlyViolation,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"ticket.optional",
		ConformanceTombstonedOptional,
		false,
	)
	for index := 1; index < len(report.Findings); index++ {
		previous := report.Findings[index-1]
		current := report.Findings[index]
		if previous.Role > current.Role ||
			(previous.Role == current.Role && previous.Class > current.Class) {
			t.Fatalf("findings are not deterministic: %#v", report.Findings)
		}
	}
}

func TestCheckConformanceAppliesFivePolicyModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        ConformanceMode
		operational bool
		findings    int
		class       ConformanceClass
		severity    ConformanceSeverity
		conformant  bool
		blocked     bool
		attention   bool
	}{
		{
			name:       "off hides ordinary violations",
			mode:       ConformanceOff,
			findings:   0,
			conformant: true,
		},
		{
			name:       "observe reports informational violation",
			mode:       ConformanceObserve,
			findings:   1,
			class:      ConformanceMissingRequired,
			severity:   ConformanceInfo,
			conformant: false,
		},
		{
			name:       "warn requests attention",
			mode:       ConformanceWarn,
			findings:   1,
			class:      ConformanceMissingRequired,
			severity:   ConformanceWarning,
			conformant: false,
			attention:  true,
		},
		{
			name:       "block rejects violation",
			mode:       ConformanceBlock,
			findings:   1,
			class:      ConformanceMissingRequired,
			severity:   ConformanceError,
			conformant: false,
			blocked:    true,
			attention:  true,
		},
		{
			name:       "repair safe requests attention",
			mode:       ConformanceRepairSafe,
			findings:   1,
			class:      ConformanceMissingRequired,
			severity:   ConformanceWarning,
			conformant: false,
			attention:  true,
		},
		{
			name:        "off still blocks operational issue",
			mode:        ConformanceOff,
			operational: true,
			findings:    1,
			class:       ConformanceInvalidMetadata,
			severity:    ConformanceError,
			conformant:  false,
			blocked:     true,
			attention:   true,
		},
		{
			name:        "observe still blocks operational issue",
			mode:        ConformanceObserve,
			operational: true,
			findings:    1,
			class:       ConformanceInvalidMetadata,
			severity:    ConformanceError,
			conformant:  false,
			blocked:     true,
			attention:   true,
		},
		{
			name:        "warn still blocks operational issue",
			mode:        ConformanceWarn,
			operational: true,
			findings:    1,
			class:       ConformanceInvalidMetadata,
			severity:    ConformanceError,
			conformant:  false,
			blocked:     true,
			attention:   true,
		},
		{
			name:        "block reports operational issue",
			mode:        ConformanceBlock,
			operational: true,
			findings:    1,
			class:       ConformanceInvalidMetadata,
			severity:    ConformanceError,
			conformant:  false,
			blocked:     true,
			attention:   true,
		},
		{
			name:        "repair safe still blocks operational issue",
			mode:        ConformanceRepairSafe,
			operational: true,
			findings:    1,
			class:       ConformanceInvalidMetadata,
			severity:    ConformanceError,
			conformant:  false,
			blocked:     true,
			attention:   true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				report ConformanceReport
				err    error
			)
			if test.operational {
				report, err = ReportIssues(test.mode, []ConformanceIssue{{
					Code:    "ticket.metadata.invalid",
					Class:   ConformanceInvalidMetadata,
					Summary: "status metadata is invalid",
				}})
			} else {
				request := minimalMissingDirectoryRequest(t)
				request.Mode = test.mode
				report, err = CheckConformance(request)
			}
			if err != nil {
				t.Fatalf("check conformance: %v", err)
			}
			if len(report.Findings) != test.findings ||
				report.Conformant != test.conformant ||
				report.Blocked != test.blocked ||
				report.Attention != test.attention {
				t.Fatalf("mode report = %#v", report)
			}
			if test.findings == 0 {
				return
			}
			if report.Findings[0].Class != test.class ||
				report.Findings[0].Severity != test.severity {
				t.Fatalf(
					"finding = %#v, want class=%s severity=%s",
					report.Findings[0],
					test.class,
					test.severity,
				)
			}
		})
	}
}

func TestCheckConformanceRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*ConformanceRequest)
		wantErr string
	}{
		{
			name: "empty scope",
			mutate: func(request *ConformanceRequest) {
				request.Scope = ""
			},
			wantErr: "conformance scope is required",
		},
		{
			name: "scope with surrounding whitespace",
			mutate: func(request *ConformanceRequest) {
				request.Scope = " ticket:conformance-1"
			},
			wantErr: "conformance scope is required",
		},
		{
			name: "unsupported mode",
			mutate: func(request *ConformanceRequest) {
				request.Mode = ConformanceMode("enforce")
			},
			wantErr: "unsupported conformance mode",
		},
		{
			name: "unresolved current profile",
			mutate: func(request *ConformanceRequest) {
				request.CurrentProfile.Inherits = []ProfileReference{{
					ID: "ticket/base", Version: "v1",
				}}
			},
			wantErr: "validate current conformance profile",
		},
		{
			name: "current manifest profile mismatch",
			mutate: func(request *ConformanceRequest) {
				request.RenderManifest.Profile.ID = "ticket/other"
			},
			wantErr: "render manifest does not match current profile",
		},
		{
			name: "desired manifest profile mismatch",
			mutate: func(request *ConformanceRequest) {
				request.DesiredRender.Manifest.Profile.ID = "ticket/other"
			},
			wantErr: "desired conformance render does not match active profile",
		},
		{
			name: "duplicate observation role",
			mutate: func(request *ConformanceRequest) {
				request.Observations = append(
					request.Observations,
					request.Observations[0],
				)
			},
			wantErr: "duplicate role",
		},
		{
			name: "existing file observation without hash",
			mutate: func(request *ConformanceRequest) {
				request.Observations[1].Hash = ""
			},
			wantErr: "requires a content hash",
		},
		{
			name: "issue class is not operational",
			mutate: func(request *ConformanceRequest) {
				request.Issues = []ConformanceIssue{{
					Code:    "artifact.missing",
					Class:   ConformanceMissingRequired,
					Summary: "artifact is missing",
				}}
			},
			wantErr: "not an always-reported class",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := conformanceFixture(t)
			test.mutate(&request)
			_, err := CheckConformance(request)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"CheckConformance() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestReportIssuesOrdersFindingTiesByPathAndSummary(t *testing.T) {
	t.Parallel()

	issues := []ConformanceIssue{
		{
			Code:    "catalog.collision",
			Class:   ConformanceInvalidMetadata,
			Role:    "ticket.shared",
			Path:    "z/status.yaml",
			Summary: "alpha",
		},
		{
			Code:    "catalog.collision",
			Class:   ConformanceInvalidMetadata,
			Role:    "ticket.shared",
			Path:    "a/status.yaml",
			Summary: "zulu",
		},
		{
			Code:    "catalog.collision",
			Class:   ConformanceInvalidMetadata,
			Role:    "ticket.shared",
			Path:    "a/status.yaml",
			Summary: "alpha",
		},
	}
	report, err := ReportIssues(ConformanceWarn, issues)
	if err != nil {
		t.Fatalf("report issues: %v", err)
	}
	got := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		got = append(got, finding.Path+"|"+finding.Summary)
	}
	want := []string{
		"a/status.yaml|alpha",
		"a/status.yaml|zulu",
		"z/status.yaml|alpha",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("finding order = %#v, want %#v", got, want)
	}
}

// TestReportIssuesAcceptsExactlyTheAlwaysReportedClasses pins the membership of
// the always-reported set, class by class. The three integrity classes are
// reported and block in every mode — including "off" — while the other seven
// derive from artifact comparison and cannot arrive as an issue. The set is
// consulted from two places (the issue-to-finding conversion and the severity
// mapping), so a class silently moving sides would either hide a broken
// workspace or block on ordinary drift.
func TestReportIssuesAcceptsExactlyTheAlwaysReportedClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		class          ConformanceClass
		alwaysReported bool
	}{
		{class: ConformanceInvalidMetadata, alwaysReported: true},
		{class: ConformanceContainedPathError, alwaysReported: true},
		{class: ConformanceConfigurationError, alwaysReported: true},
		{class: ConformanceMissingRequired},
		{class: ConformanceTombstonedOptional},
		{class: ConformanceStaleProfile},
		{class: ConformanceStaleTemplate},
		{class: ConformanceAuthoredCustomized},
		{class: ConformanceGeneratedDrift},
		{class: ConformanceAppendOnlyViolation},
		{class: ConformanceClass("not_a_class")},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			t.Parallel()

			// Mode "off" is the strict case: an always-reported class must
			// still be reported and still block there.
			report, err := ReportIssues(ConformanceOff, []ConformanceIssue{{
				Code:    "ticket.probe",
				Class:   test.class,
				Path:    "status.yaml",
				Summary: "probe",
			}})
			if !test.alwaysReported {
				if err == nil {
					t.Fatalf(
						"class %q accepted as an issue, want rejected: %#v",
						test.class,
						report,
					)
				}
				return
			}
			if err != nil {
				t.Fatalf("report issues for %q: %v", test.class, err)
			}
			if len(report.Findings) != 1 {
				t.Fatalf("findings = %#v, want exactly one", report.Findings)
			}
			if report.Findings[0].Severity != ConformanceError {
				t.Fatalf(
					"severity = %q, want %q",
					report.Findings[0].Severity,
					ConformanceError,
				)
			}
			if !report.Blocked || !report.Attention || report.Conformant {
				t.Fatalf(
					"report = %#v, want blocked and non-conformant in mode off",
					report,
				)
			}
		})
	}
}

func TestPlanReconciliationRepairsOnlySafeTargets(t *testing.T) {
	t.Parallel()

	request := conformanceFixture(t)
	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check conformance: %v", err)
	}
	plan, err := PlanReconciliation(request, report)
	if err != nil {
		t.Fatalf("plan reconciliation: %v", err)
	}
	if !plan.Preview || plan.Blocked {
		t.Fatalf("plan header = %#v", plan)
	}
	assertReconcileEffect(
		t,
		plan,
		"ticket.artifacts",
		ReconcileCreateDirectory,
		true,
	)
	assertReconcileEffect(
		t,
		plan,
		"ticket.generated_safe",
		ReconcileReplaceGenerated,
		true,
	)
	for _, role := range []string{
		"ticket.context",
		"ticket.generated_custom",
		"ticket.missing_authored",
		"ticket.notes",
	} {
		assertReconcileEffect(t, plan, role, ReconcileReview, false)
	}
	for _, effect := range plan.Effects {
		if !effect.Safe && len(effect.Content) != 0 {
			t.Fatalf("unsafe effect carries replacement content: %#v", effect)
		}
	}
	if !reflect.DeepEqual(request, conformanceFixture(t)) {
		t.Fatal("conformance request was mutated")
	}
}

func TestPlanReconciliationStateTransitions(t *testing.T) {
	t.Parallel()

	generatedContent := []byte("generated content\n")
	staleContent := []byte("fresh generated content\n")
	currentHash := HashContent([]byte("current content\n"))
	baselineHash := HashContent([]byte("generated baseline\n"))
	desiredHash := HashContent(staleContent)
	request := ConformanceRequest{
		Mode: ConformanceRepairSafe,
		ActiveProfile: Profile{
			SchemaVersion: SchemaVersion,
			ID:            "ticket/reconcile",
			Version:       "v1",
			WorkTypes:     []string{"fix"},
			Artifacts: []Artifact{
				testDirectory("ticket.artifacts", "artifacts", true),
				testArtifact(
					"ticket.generated",
					"generated.md",
					AuthorityGenerated,
				),
				testArtifact(
					"ticket.stale",
					"stale.md",
					AuthorityGenerated,
				),
				{
					Role:      "ticket.marker",
					Path:      "marker",
					Kind:      ArtifactFile,
					Authority: AuthorityStructural,
					Required:  true,
					Search:    SearchNone,
					Retention: RetentionDurable,
					Git:       GitTracked,
				},
				testArtifact(
					"ticket.context",
					"context.md",
					AuthorityAuthored,
				),
			},
		},
		DesiredRender: RenderResult{Files: []RenderedFile{
			{
				Role:    "ticket.generated",
				Path:    "generated.md",
				Content: generatedContent,
			},
			{
				Role:    "ticket.stale",
				Path:    "stale.md",
				Content: staleContent,
			},
		}},
	}
	missingFinding := func(
		role string,
		path string,
		safe bool,
	) ConformanceFinding {
		return ConformanceFinding{
			Class:      ConformanceMissingRequired,
			Role:       role,
			Path:       path,
			SafeRepair: safe,
			Violation:  true,
			Summary:    "required artifact is missing",
		}
	}
	tests := []struct {
		name             string
		reportMode       ConformanceMode
		findings         []ConformanceFinding
		role             string
		wantAction       ReconcileAction
		wantSafe         bool
		wantEffects      int
		wantContent      []byte
		wantBeforeHash   string
		wantBaselineHash string
		wantAfterHash    string
		wantErr          string
	}{
		{
			name:        "missing directory becomes safe create",
			reportMode:  ConformanceRepairSafe,
			findings:    []ConformanceFinding{missingFinding("ticket.artifacts", "artifacts", true)},
			role:        "ticket.artifacts",
			wantAction:  ReconcileCreateDirectory,
			wantSafe:    true,
			wantEffects: 1,
		},
		{
			name:        "missing generated file becomes rendered create",
			reportMode:  ConformanceRepairSafe,
			findings:    []ConformanceFinding{missingFinding("ticket.generated", "generated.md", true)},
			role:        "ticket.generated",
			wantAction:  ReconcileCreateFile,
			wantSafe:    true,
			wantEffects: 1,
			wantContent: generatedContent,
			wantAfterHash: HashContent(
				generatedContent,
			),
		},
		{
			name:        "missing structural file becomes empty create",
			reportMode:  ConformanceRepairSafe,
			findings:    []ConformanceFinding{missingFinding("ticket.marker", "marker", true)},
			role:        "ticket.marker",
			wantAction:  ReconcileCreateFile,
			wantSafe:    true,
			wantEffects: 1,
			wantContent: []byte{},
			wantAfterHash: HashContent(
				nil,
			),
		},
		{
			name:        "missing authored file requires review",
			reportMode:  ConformanceRepairSafe,
			findings:    []ConformanceFinding{missingFinding("ticket.context", "context.md", false)},
			role:        "ticket.context",
			wantAction:  ReconcileReview,
			wantEffects: 1,
		},
		{
			name:       "stale generated file becomes safe replacement",
			reportMode: ConformanceRepairSafe,
			findings: []ConformanceFinding{{
				Class:        ConformanceStaleTemplate,
				Role:         "ticket.stale",
				Path:         "stale.md",
				SafeRepair:   true,
				Violation:    true,
				CurrentHash:  currentHash,
				ExpectedHash: desiredHash,
				Summary:      "managed rendering is stale",
			}},
			role:           "ticket.stale",
			wantAction:     ReconcileReplaceGenerated,
			wantSafe:       true,
			wantEffects:    1,
			wantContent:    staleContent,
			wantBeforeHash: currentHash,
			wantAfterHash:  desiredHash,
		},
		{
			name:       "customized generated file preserves review hashes",
			reportMode: ConformanceRepairSafe,
			findings: []ConformanceFinding{{
				Class:        ConformanceGeneratedDrift,
				Role:         "ticket.generated",
				Path:         "generated.md",
				Violation:    true,
				CurrentHash:  currentHash,
				ExpectedHash: baselineHash,
				DesiredHash:  desiredHash,
				Summary:      "generated content was customized",
			}},
			role:             "ticket.generated",
			wantAction:       ReconcileReview,
			wantEffects:      1,
			wantBeforeHash:   currentHash,
			wantBaselineHash: baselineHash,
			wantAfterHash:    desiredHash,
		},
		{
			name:       "optional tombstone emits no effect",
			reportMode: ConformanceRepairSafe,
			findings: []ConformanceFinding{{
				Class: ConformanceTombstonedOptional,
				Role:  "ticket.context",
				Path:  "context.md",
			}},
			wantEffects: 0,
		},
		{
			name:       "stale profile downgrades safe repair to review",
			reportMode: ConformanceRepairSafe,
			findings: []ConformanceFinding{
				{
					Class:     ConformanceStaleProfile,
					Violation: true,
					Summary:   "profile upgrade is required",
				},
				missingFinding(
					"ticket.generated",
					"generated.md",
					true,
				),
			},
			role:        "ticket.generated",
			wantAction:  ReconcileReview,
			wantEffects: 2,
		},
		{
			name:       "unknown artifact finding is ignored",
			reportMode: ConformanceRepairSafe,
			findings: []ConformanceFinding{
				missingFinding("ticket.unknown", "unknown.md", true),
			},
			wantEffects: 0,
		},
		{
			name:       "report mode mismatch is rejected",
			reportMode: ConformanceWarn,
			wantErr:    "mode does not match",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan, err := PlanReconciliation(request, ConformanceReport{
				Mode:     test.reportMode,
				Findings: test.findings,
			})
			if test.wantErr != "" {
				if err == nil ||
					!strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf(
						"PlanReconciliation() error = %v, want containing %q",
						err,
						test.wantErr,
					)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan reconciliation: %v", err)
			}
			if !plan.Preview || len(plan.Effects) != test.wantEffects {
				t.Fatalf(
					"plan = %#v, want preview with %d effects",
					plan,
					test.wantEffects,
				)
			}
			if test.role == "" {
				return
			}
			effect := reconcileEffectByRole(t, plan, test.role)
			if effect.Action != test.wantAction ||
				effect.Safe != test.wantSafe ||
				effect.BeforeHash != test.wantBeforeHash ||
				effect.BaselineHash != test.wantBaselineHash ||
				effect.AfterHash != test.wantAfterHash ||
				!reflect.DeepEqual(effect.Content, test.wantContent) {
				t.Fatalf(
					"effect = %#v, want action=%s safe=%t before=%q baseline=%q after=%q content=%q",
					effect,
					test.wantAction,
					test.wantSafe,
					test.wantBeforeHash,
					test.wantBaselineHash,
					test.wantAfterHash,
					test.wantContent,
				)
			}
		})
	}
}

func TestGeneratedDriftReviewExposesDesiredHashAndRecordedBaseline(
	t *testing.T,
) {
	t.Parallel()

	request := conformanceFixture(t)
	activeArtifact := artifactByRole(
		t,
		request.ActiveProfile,
		"ticket.generated_custom",
	)
	activeArtifact.Template.Version = "v2"
	for index := range request.ActiveProfile.Artifacts {
		if request.ActiveProfile.Artifacts[index].Role ==
			activeArtifact.Role {
			request.ActiveProfile.Artifacts[index] = activeArtifact
			break
		}
	}
	desiredContent := []byte("new active generated content\n")
	for index := range request.DesiredRender.Files {
		if request.DesiredRender.Files[index].Role ==
			activeArtifact.Role {
			request.DesiredRender.Files[index].Content =
				append([]byte(nil), desiredContent...)
			break
		}
	}
	for index := range request.DesiredRender.Manifest.Artifacts {
		if request.DesiredRender.Manifest.Artifacts[index].Role ==
			activeArtifact.Role {
			request.DesiredRender.Manifest.Artifacts[index] =
				renderedRecord(
					activeArtifact,
					activeArtifact.Template.Version,
					desiredContent,
				)
			break
		}
	}

	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check conformance: %v", err)
	}
	var drift ConformanceFinding
	for _, finding := range report.Findings {
		if finding.Role == activeArtifact.Role &&
			finding.Class == ConformanceGeneratedDrift {
			drift = finding
			break
		}
	}
	recordedBaseline := HashContent([]byte("generated baseline\n"))
	activeDesired := HashContent(desiredContent)
	if drift.ExpectedHash != recordedBaseline ||
		drift.DesiredHash != activeDesired {
		t.Fatalf(
			"generated drift hashes = %#v, want baseline=%s desired=%s",
			drift,
			recordedBaseline,
			activeDesired,
		)
	}

	plan, err := PlanReconciliation(request, report)
	if err != nil {
		t.Fatalf("plan reconciliation: %v", err)
	}
	for _, effect := range plan.Effects {
		if effect.Role != activeArtifact.Role {
			continue
		}
		if effect.Action != ReconcileReview ||
			effect.BaselineHash != recordedBaseline ||
			effect.AfterHash != activeDesired {
			t.Fatalf(
				"generated drift review = %#v, want baseline=%s desired=%s",
				effect,
				recordedBaseline,
				activeDesired,
			)
		}
		return
	}
	t.Fatalf(
		"generated drift review missing from %#v",
		plan.Effects,
	)
}

func TestPlanReconciliationRequiresProfileUpgradeBeforeArtifactRepair(
	t *testing.T,
) {
	t.Parallel()

	request := conformanceFixture(t)
	request.ActiveProfile.Version = "v2"
	request.DesiredRender.Manifest.Profile =
		profileReference(request.ActiveProfile)

	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check conformance: %v", err)
	}
	assertConformanceFinding(
		t,
		report,
		"",
		ConformanceStaleProfile,
		false,
	)

	plan, err := PlanReconciliation(request, report)
	if err != nil {
		t.Fatalf("plan reconciliation: %v", err)
	}
	for _, effect := range plan.Effects {
		if effect.Safe || len(effect.Content) != 0 {
			t.Fatalf(
				"stale-profile plan emitted artifact repair: %#v",
				effect,
			)
		}
	}
	for _, role := range []string{
		"ticket.artifacts",
		"ticket.generated_safe",
	} {
		assertReconcileEffect(
			t,
			plan,
			role,
			ReconcileReview,
			false,
		)
	}
}

func TestCheckConformanceAcceptsEmptyAppendOnlyCheckpoint(t *testing.T) {
	t.Parallel()

	request := conformanceFixture(t)
	emptyHash := HashContent(nil)
	for index := range request.Observations {
		if request.Observations[index].Role != "ticket.notes" {
			continue
		}
		request.Observations[index].Hash = emptyHash
		request.Observations[index].Size = 0
		request.Observations[index].BaselineLength = 0
		request.Observations[index].BaselineHash = emptyHash
		request.Observations[index].PrefixHash = emptyHash
	}

	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check empty append-only checkpoint: %v", err)
	}
	for _, finding := range report.Findings {
		if finding.Role == "ticket.notes" {
			t.Fatalf(
				"valid empty append-only checkpoint was rejected: %#v",
				finding,
			)
		}
	}
}

func TestCheckConformanceClassifiesStaleProfileAndOperationalIssues(
	t *testing.T,
) {
	t.Parallel()

	request := conformanceFixture(t)
	request.ActiveProfile.Version = "v2"
	request.DesiredRender.Manifest.Profile =
		profileReference(request.ActiveProfile)
	request.Issues = []ConformanceIssue{
		{
			Code:    "ticket.path.escape",
			Class:   ConformanceContainedPathError,
			Summary: "artifact path escaped its ticket root",
		},
		{
			Code:    "ticket.config.invalid",
			Class:   ConformanceConfigurationError,
			Summary: "resolved policy configuration is invalid",
		},
	}
	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check conformance: %v", err)
	}
	assertConformanceFinding(
		t,
		report,
		"",
		ConformanceStaleProfile,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"",
		ConformanceContainedPathError,
		false,
	)
	assertConformanceFinding(
		t,
		report,
		"",
		ConformanceConfigurationError,
		false,
	)
	if !report.Blocked {
		t.Fatalf("operational issues did not block: %#v", report)
	}
}

func TestPlanReconciliationCreatesMissingStructuralFileEmpty(
	t *testing.T,
) {
	t.Parallel()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/structural",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{{
			Role:      "ticket.marker",
			Path:      "marker",
			Kind:      ArtifactFile,
			Authority: AuthorityStructural,
			Required:  true,
			Search:    SearchNone,
			Retention: RetentionDurable,
			Git:       GitTracked,
		}},
	}
	request := ConformanceRequest{
		Scope:          "ticket:structural-1",
		Mode:           ConformanceRepairSafe,
		CurrentProfile: current,
		ActiveProfile:  cloneProfile(current),
		RenderManifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(current),
			Artifacts:     []RenderedRecord{},
		},
		DesiredRender: RenderResult{
			Files: []RenderedFile{},
			Manifest: RenderManifest{
				SchemaVersion: RenderManifestSchemaVersion,
				Profile:       profileReference(current),
				Artifacts:     []RenderedRecord{},
			},
		},
		Observations: []ArtifactObservation{{
			Role: "ticket.marker", Path: "marker",
			Kind: ArtifactFile, Exists: false,
		}},
	}
	report, err := CheckConformance(request)
	if err != nil {
		t.Fatalf("check structural conformance: %v", err)
	}
	plan, err := PlanReconciliation(request, report)
	if err != nil {
		t.Fatalf("plan structural reconciliation: %v", err)
	}
	assertReconcileEffect(
		t,
		plan,
		"ticket.marker",
		ReconcileCreateFile,
		true,
	)
	for _, effect := range plan.Effects {
		if effect.Role == "ticket.marker" &&
			(effect.AfterHash != HashContent(nil) ||
				effect.Content == nil ||
				len(effect.Content) != 0) {
			t.Fatalf("structural file effect = %#v", effect)
		}
	}
}

func conformanceFixture(t *testing.T) ConformanceRequest {
	t.Helper()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/conformance",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testDirectory("ticket.artifacts", "artifacts", true),
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
			testArtifact("ticket.generated_custom", "custom.md", AuthorityGenerated),
			testArtifact("ticket.generated_safe", "safe.md", AuthorityGenerated),
			testArtifact("ticket.missing_authored", "missing.md", AuthorityAuthored),
			testArtifact("ticket.notes", "notes.md", AuthorityAuthored),
			testArtifact("ticket.optional", "optional.md", AuthorityAuthored),
		},
	}
	current.Artifacts[5].AppendOnly = true
	current.Artifacts[6].Required = false
	active := cloneProfile(current)
	active.Artifacts[3].Template.Version = "v2"

	currentContent := map[string][]byte{
		"ticket.context":          []byte("context bootstrap\n"),
		"ticket.generated_custom": []byte("generated baseline\n"),
		"ticket.generated_safe":   []byte("old generated\n"),
		"ticket.missing_authored": []byte("missing bootstrap\n"),
		"ticket.notes":            []byte("notes bootstrap\n"),
		"ticket.optional":         []byte("optional bootstrap\n"),
	}
	desiredContent := map[string][]byte{
		"ticket.context":          currentContent["ticket.context"],
		"ticket.generated_custom": currentContent["ticket.generated_custom"],
		"ticket.generated_safe":   []byte("new generated\n"),
		"ticket.missing_authored": currentContent["ticket.missing_authored"],
		"ticket.notes":            currentContent["ticket.notes"],
		"ticket.optional":         currentContent["ticket.optional"],
	}
	currentManifest := RenderManifest{
		SchemaVersion: RenderManifestSchemaVersion,
		Profile:       profileReference(current),
		Artifacts:     []RenderedRecord{},
	}
	desired := RenderResult{
		Files: []RenderedFile{},
		Manifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(active),
			Artifacts:     []RenderedRecord{},
		},
	}
	for _, artifact := range current.Artifacts {
		if artifact.Template == nil {
			continue
		}
		currentManifest.Artifacts = append(
			currentManifest.Artifacts,
			renderedRecord(
				artifact,
				artifact.Template.Version,
				currentContent[artifact.Role],
			),
		)
		activeArtifact := artifactByRole(t, active, artifact.Role)
		desired.Files = append(desired.Files, RenderedFile{
			Role:    artifact.Role,
			Path:    artifact.Path,
			Content: append([]byte(nil), desiredContent[artifact.Role]...),
		})
		desired.Manifest.Artifacts = append(
			desired.Manifest.Artifacts,
			renderedRecord(
				activeArtifact,
				activeArtifact.Template.Version,
				desiredContent[artifact.Role],
			),
		)
	}
	return ConformanceRequest{
		Scope:          "ticket:conformance-1",
		Mode:           ConformanceRepairSafe,
		CurrentProfile: current,
		ActiveProfile:  active,
		RenderManifest: currentManifest,
		DesiredRender:  desired,
		Tombstones: TombstoneManifest{
			SchemaVersion: TombstoneManifestSchemaVersion,
			Tombstones: []Tombstone{{
				Actor:     "human",
				Reason:    "intentionally omitted",
				Scope:     "ticket:conformance-1",
				Role:      "ticket.optional",
				Path:      "optional.md",
				RemovedAt: "2026-09-10T12:00:00Z",
			}},
		},
		Observations: []ArtifactObservation{
			{
				Role: "ticket.artifacts", Path: "artifacts",
				Kind: ArtifactDirectory, Exists: false,
			},
			{
				Role: "ticket.context", Path: "context.md",
				Kind: ArtifactFile, Exists: true,
				Hash: HashContent([]byte("custom context\n")),
			},
			{
				Role: "ticket.generated_custom", Path: "custom.md",
				Kind: ArtifactFile, Exists: true,
				Hash: HashContent([]byte("custom generated\n")),
			},
			{
				Role: "ticket.generated_safe", Path: "safe.md",
				Kind: ArtifactFile, Exists: true,
				Hash: HashContent(currentContent["ticket.generated_safe"]),
			},
			{
				Role: "ticket.missing_authored", Path: "missing.md",
				Kind: ArtifactFile, Exists: false,
			},
			{
				Role: "ticket.notes", Path: "notes.md",
				Kind: ArtifactFile, Exists: true,
				Hash: HashContent([]byte("not")),
				Size: 3, BaselineLength: 8,
				BaselineHash: HashContent([]byte("baseline")),
				PrefixHash:   HashContent([]byte("not")),
			},
			{
				Role: "ticket.optional", Path: "optional.md",
				Kind: ArtifactFile, Exists: false,
			},
		},
	}
}

func minimalMissingDirectoryRequest(t *testing.T) ConformanceRequest {
	t.Helper()

	value := conformanceFixture(t)
	value.ActiveProfile.Artifacts = []Artifact{
		artifactByRole(t, value.ActiveProfile, "ticket.artifacts"),
	}
	value.CurrentProfile.Artifacts = []Artifact{
		artifactByRole(t, value.CurrentProfile, "ticket.artifacts"),
	}
	value.RenderManifest = RenderManifest{
		SchemaVersion: RenderManifestSchemaVersion,
		Profile:       profileReference(value.CurrentProfile),
		Artifacts:     []RenderedRecord{},
	}
	value.DesiredRender = RenderResult{
		Files: []RenderedFile{},
		Manifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(value.ActiveProfile),
			Artifacts:     []RenderedRecord{},
		},
	}
	value.Tombstones = TombstoneManifest{}
	value.Observations = []ArtifactObservation{{
		Role: "ticket.artifacts", Path: "artifacts",
		Kind: ArtifactDirectory, Exists: false,
	}}
	return value
}

func assertConformanceFinding(
	t *testing.T,
	report ConformanceReport,
	role string,
	class ConformanceClass,
	safe bool,
) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.Role == role && finding.Class == class {
			if finding.SafeRepair != safe {
				t.Fatalf("finding = %#v, want safe=%t", finding, safe)
			}
			return
		}
	}
	t.Fatalf("finding %s/%s missing from %#v", role, class, report.Findings)
}

func assertReconcileEffect(
	t *testing.T,
	plan ReconcilePlan,
	role string,
	action ReconcileAction,
	safe bool,
) {
	t.Helper()
	for _, effect := range plan.Effects {
		if effect.Role == role {
			if effect.Action != action || effect.Safe != safe {
				t.Fatalf(
					"effect = %#v, want action=%s safe=%t",
					effect,
					action,
					safe,
				)
			}
			return
		}
	}
	t.Fatalf("effect for %q missing from %#v", role, plan.Effects)
}

func reconcileEffectByRole(
	t *testing.T,
	plan ReconcilePlan,
	role string,
) ReconcileEffect {
	t.Helper()
	for _, effect := range plan.Effects {
		if effect.Role == role {
			return effect
		}
	}
	t.Fatalf("effect for %q missing from %#v", role, plan.Effects)
	return ReconcileEffect{}
}

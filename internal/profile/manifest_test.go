package profile

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRenderManifestRoundTripIsStrictAndDeterministic(t *testing.T) {
	t.Parallel()

	manifest := RenderManifest{
		SchemaVersion: RenderManifestSchemaVersion,
		Profile: ProfileReference{
			ID:      "ticket/default",
			Version: "v1",
		},
		Artifacts: []RenderedRecord{
			{
				Role: "ticket.notes",
				Path: "notes.md",
				Template: TemplateProvenance{
					ID:          "ticket/notes",
					Version:     "v1",
					SourceScope: "builtin",
				},
				GeneratedHash: HashContent([]byte("# Notes\n")),
				ObservedHash:  HashContent([]byte("# Notes\n")),
			},
			{
				Role: "ticket.context",
				Path: "context.md",
				Template: TemplateProvenance{
					ID:          "ticket/context",
					Version:     "v1",
					SourceScope: "builtin",
				},
				GeneratedHash: HashContent([]byte("# Context\n")),
				ObservedHash:  HashContent([]byte("# Context\n")),
			},
		},
	}
	var first bytes.Buffer
	if err := EncodeRenderManifest(&first, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	var second bytes.Buffer
	if err := EncodeRenderManifest(&second, manifest); err != nil {
		t.Fatalf("encode manifest again: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("manifest encoding is not deterministic:\n%s\n%s", first.String(), second.String())
	}
	decoded, err := DecodeRenderManifest(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if decoded.Artifacts[0].Role != "ticket.context" ||
		decoded.Artifacts[1].Role != "ticket.notes" {
		t.Fatalf("manifest records were not normalized: %#v", decoded.Artifacts)
	}

	invalid := []string{
		strings.Replace(first.String(), "schema_version:", "unknown: true\nschema_version:", 1),
		strings.Replace(first.String(), "context.md", "../context.md", 1),
		strings.Replace(first.String(), manifest.Artifacts[0].GeneratedHash, "not-a-hash", 1),
		first.String() + "---\nschema_version: aidb.rendered/v1\n",
	}
	for _, input := range invalid {
		if _, err := DecodeRenderManifest(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid render manifest was accepted:\n%s", input)
		}
	}

	ancestorConflict := manifest
	ancestorConflict.Artifacts[0].Path = "managed"
	ancestorConflict.Artifacts[1].Path = "managed/context.md"
	if err := ValidateRenderManifest(ancestorConflict); err == nil {
		t.Fatal("manifest with a file/descendant path conflict was accepted")
	}
}

func TestTombstonesRecordIntentionalRemovalAppendOnly(t *testing.T) {
	t.Parallel()

	manifest := TombstoneManifest{
		SchemaVersion: TombstoneManifestSchemaVersion,
		Tombstones: []Tombstone{{
			Actor:     "valter",
			Reason:    "not needed for this ticket",
			Scope:     "ticket:TASK-00039",
			Role:      "ticket.design",
			Path:      "design.md",
			RemovedAt: "2026-09-10T12:00:00+08:00",
		}},
	}
	updated, err := AddTombstone(manifest, Tombstone{
		Actor:     "valter",
		Reason:    "replaced with an external artifact",
		Scope:     "ticket:TASK-00039",
		Role:      "ticket.handoff",
		Path:      "handoff.md",
		RemovedAt: "2026-09-10T13:00:00+08:00",
	})
	if err != nil {
		t.Fatalf("add tombstone: %v", err)
	}
	if len(manifest.Tombstones) != 1 || len(updated.Tombstones) != 2 {
		t.Fatalf("append-only update mutated input: before=%#v after=%#v", manifest, updated)
	}

	var encoded bytes.Buffer
	if err := EncodeTombstones(&encoded, updated); err != nil {
		t.Fatalf("encode tombstones: %v", err)
	}
	decoded, err := DecodeTombstones(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode tombstones: %v", err)
	}
	if !reflect.DeepEqual(decoded, updated) {
		t.Fatalf("tombstone round trip:\ngot=%#v\nwant=%#v", decoded, updated)
	}

	invalid := []Tombstone{
		{Actor: "", Reason: "x", Scope: "ticket:x", Role: "x", Path: "x", RemovedAt: "2026-09-10T12:00:00Z"},
		{Actor: "x", Reason: "", Scope: "ticket:x", Role: "x", Path: "x", RemovedAt: "2026-09-10T12:00:00Z"},
		{Actor: "x", Reason: "x", Scope: "", Role: "x", Path: "x", RemovedAt: "2026-09-10T12:00:00Z"},
		{Actor: "x", Reason: "x", Scope: "ticket:x", Role: "", Path: "x", RemovedAt: "2026-09-10T12:00:00Z"},
		{Actor: "x", Reason: "x", Scope: "ticket:x", Role: "x", Path: "../x", RemovedAt: "2026-09-10T12:00:00Z"},
		{Actor: "x", Reason: "x", Scope: "ticket:x", Role: "x", Path: "x", RemovedAt: "yesterday"},
	}
	for _, tombstone := range invalid {
		if _, err := AddTombstone(TombstoneManifest{}, tombstone); err == nil {
			t.Fatalf("invalid tombstone was accepted: %#v", tombstone)
		}
	}

	unknownField := encoded.String() + "unknown: true\n"
	if _, err := DecodeTombstones(strings.NewReader(unknownField)); err == nil {
		t.Fatal("tombstone manifest with an unknown field was accepted")
	}
	multiple := encoded.String() +
		"---\nschema_version: aidb.tombstones/v1\ntombstones: []\n"
	if _, err := DecodeTombstones(strings.NewReader(multiple)); err == nil {
		t.Fatal("multi-document tombstone manifest was accepted")
	}
}

func TestHashContentUsesFullSHA256(t *testing.T) {
	t.Parallel()

	const want = "ba7816bf8f01cfea414140de5dae2223" +
		"b00361a396177a9cb410ff61f20015ad"
	if got := HashContent([]byte("abc")); got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
}

func TestManifestEncoderRefusesDocumentsItCannotReread(t *testing.T) {
	t.Parallel()

	manifest := TombstoneManifest{
		SchemaVersion: TombstoneManifestSchemaVersion,
		Tombstones: []Tombstone{{
			Actor:     "valter",
			Reason:    strings.Repeat("x", int(maxManifestDocumentBytes)),
			Scope:     "ticket:TASK-00039",
			Role:      "ticket.design",
			Path:      "design.md",
			RemovedAt: "2026-09-10T12:00:00Z",
		}},
	}
	var output bytes.Buffer
	if err := EncodeTombstones(&output, manifest); err == nil {
		t.Fatal("oversized tombstone manifest was encoded")
	}
}

func TestPlanUpgradePreservesAuthoredAndCustomizedContent(t *testing.T) {
	t.Parallel()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/default",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
			testArtifact("ticket.status", "status.yaml", AuthorityGenerated),
			testArtifact("ticket.summary", "summary.md", AuthorityGenerated),
			testArtifact("ticket.legacy", "legacy.md", AuthorityAuthored),
		},
	}
	target := cloneProfile(current)
	target.Version = "v2"
	target.Artifacts = []Artifact{
		testArtifact("ticket.context", "context.md", AuthorityAuthored),
		testArtifact("ticket.status", "status.yaml", AuthorityGenerated),
		testArtifact("ticket.summary", "summary.md", AuthorityGenerated),
		testDirectory("ticket.artifacts", "artifacts", true),
		testArtifact("ticket.optional", "optional.md", AuthorityAuthored),
	}
	for index := range target.Artifacts {
		if target.Artifacts[index].Template != nil {
			target.Artifacts[index].Template.Version = "v2"
		}
	}
	target.Artifacts[4].Required = false

	currentContext := []byte("authored context\n")
	currentStatus := []byte("status: old\n")
	customSummary := []byte("custom summary\n")
	currentLegacy := []byte("legacy authored\n")
	currentManifest := RenderManifest{
		SchemaVersion: RenderManifestSchemaVersion,
		Profile:       profileReference(current),
		Artifacts: []RenderedRecord{
			renderedRecord(current.Artifacts[0], "v1", currentContext),
			renderedRecord(current.Artifacts[1], "v1", currentStatus),
			renderedRecord(current.Artifacts[2], "v1", []byte("generated summary\n")),
			renderedRecord(current.Artifacts[3], "v1", currentLegacy),
		},
	}
	targetRender := RenderResult{
		Files: []RenderedFile{
			{Role: "ticket.context", Path: "context.md", Content: []byte("new context scaffold\n")},
			{Role: "ticket.status", Path: "status.yaml", Content: []byte("status: new\n")},
			{Role: "ticket.summary", Path: "summary.md", Content: []byte("new generated summary\n")},
			{Role: "ticket.optional", Path: "optional.md", Content: []byte("optional scaffold\n")},
		},
		Manifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(target),
			Artifacts: []RenderedRecord{
				renderedRecord(target.Artifacts[0], "v2", []byte("new context scaffold\n")),
				renderedRecord(target.Artifacts[1], "v2", []byte("status: new\n")),
				renderedRecord(target.Artifacts[2], "v2", []byte("new generated summary\n")),
				renderedRecord(target.Artifacts[4], "v2", []byte("optional scaffold\n")),
			},
		},
	}

	plan, err := PlanUpgrade(UpgradeRequest{
		Scope:           "ticket:TASK-00039",
		Current:         current,
		Target:          target,
		CurrentManifest: currentManifest,
		TargetRender:    targetRender,
		Observations: []Observation{
			{Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: currentContext},
			{Role: "ticket.status", Path: "status.yaml", Kind: ArtifactFile, Exists: true, Content: currentStatus},
			{Role: "ticket.summary", Path: "summary.md", Kind: ArtifactFile, Exists: true, Content: customSummary},
			{Role: "ticket.legacy", Path: "legacy.md", Kind: ArtifactFile, Exists: true, Content: currentLegacy},
			{Role: "ticket.artifacts", Path: "artifacts", Kind: ArtifactDirectory, Exists: false},
		},
		Tombstones: TombstoneManifest{
			SchemaVersion: TombstoneManifestSchemaVersion,
			Tombstones: []Tombstone{{
				Actor:     "valter",
				Reason:    "not wanted",
				Scope:     "ticket:TASK-00039",
				Role:      "ticket.optional",
				Path:      "optional.md",
				RemovedAt: "2026-09-10T12:00:00+08:00",
			}},
		},
	})
	if err != nil {
		t.Fatalf("plan upgrade: %v", err)
	}
	if !plan.Preview ||
		plan.From != profileReference(current) ||
		plan.To != profileReference(target) {
		t.Fatalf("upgrade plan header = %#v", plan)
	}
	assertChange(t, plan, "ticket.context", UpgradePreserve, false)
	assertChange(t, plan, "ticket.status", UpgradeUpdate, true)
	assertChange(t, plan, "ticket.summary", UpgradePreserve, false)
	assertChange(t, plan, "ticket.artifacts", UpgradeCreate, true)
	assertChange(t, plan, "ticket.optional", UpgradeTombstoned, false)
	assertChange(t, plan, "ticket.legacy", UpgradePreserve, false)

	requiredTombstone := target
	requiredTombstone.Artifacts[4].Required = true
	requiredPlan, err := PlanUpgrade(UpgradeRequest{
		Scope:           "ticket:TASK-00039",
		Current:         current,
		Target:          requiredTombstone,
		CurrentManifest: currentManifest,
		TargetRender:    targetRender,
		Observations: []Observation{
			{Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: currentContext},
			{Role: "ticket.status", Path: "status.yaml", Kind: ArtifactFile, Exists: true, Content: currentStatus},
			{Role: "ticket.summary", Path: "summary.md", Kind: ArtifactFile, Exists: true, Content: customSummary},
			{Role: "ticket.legacy", Path: "legacy.md", Kind: ArtifactFile, Exists: true, Content: currentLegacy},
			{Role: "ticket.artifacts", Path: "artifacts", Kind: ArtifactDirectory, Exists: false},
		},
		Tombstones: TombstoneManifest{
			SchemaVersion: TombstoneManifestSchemaVersion,
			Tombstones: []Tombstone{{
				Actor:     "valter",
				Reason:    "not wanted",
				Scope:     "ticket:TASK-00039",
				Role:      "ticket.optional",
				Path:      "optional.md",
				RemovedAt: "2026-09-10T12:00:00+08:00",
			}},
		},
	})
	if err != nil {
		t.Fatalf("plan required tombstone upgrade: %v", err)
	}
	assertChange(t, requiredPlan, "ticket.optional", UpgradeReview, false)
}

func TestPlanUpgradeRequiresExplicitAbsenceBeforeSafeCreation(t *testing.T) {
	t.Parallel()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/default",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	target := cloneProfile(current)
	target.Version = "v2"
	target.Artifacts = append(
		target.Artifacts,
		testArtifact("ticket.generated", "generated.md", AuthorityGenerated),
		testDirectory("ticket.artifacts", "artifacts", true),
	)
	target.Artifacts[0].Template.Version = "v2"
	target.Artifacts[1].Template.Version = "v2"
	context := []byte("context\n")
	request := UpgradeRequest{
		Scope:   "ticket:TASK-00039",
		Current: current,
		Target:  target,
		CurrentManifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(current),
			Artifacts: []RenderedRecord{
				renderedRecord(current.Artifacts[0], "v1", context),
			},
		},
		TargetRender: RenderResult{
			Files: []RenderedFile{
				{Role: "ticket.context", Path: "context.md", Content: context},
				{Role: "ticket.generated", Path: "generated.md", Content: []byte("generated\n")},
			},
			Manifest: RenderManifest{
				SchemaVersion: RenderManifestSchemaVersion,
				Profile:       profileReference(target),
				Artifacts: []RenderedRecord{
					renderedRecord(target.Artifacts[0], "v2", context),
					renderedRecord(target.Artifacts[1], "v2", []byte("generated\n")),
				},
			},
		},
		Observations: []Observation{{
			Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: context,
		}},
	}
	incomplete, err := PlanUpgrade(request)
	if err != nil {
		t.Fatalf("plan with incomplete observations: %v", err)
	}
	assertChange(t, incomplete, "ticket.generated", UpgradeReview, false)
	assertChange(t, incomplete, "ticket.artifacts", UpgradeReview, false)

	request.Observations = append(
		request.Observations,
		Observation{
			Role: "ticket.generated", Path: "generated.md", Kind: ArtifactFile, Exists: false,
		},
		Observation{
			Role: "ticket.artifacts", Path: "artifacts", Kind: ArtifactDirectory, Exists: false,
		},
	)
	explicit, err := PlanUpgrade(request)
	if err != nil {
		t.Fatalf("plan with explicit absence: %v", err)
	}
	assertChange(t, explicit, "ticket.generated", UpgradeCreate, true)
	assertChange(t, explicit, "ticket.artifacts", UpgradeCreate, true)

	stale := request
	stale.Observations = []Observation{
		{Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: context},
		{Role: "ticket.generated", Path: "elsewhere.md", Kind: ArtifactFile, Exists: false},
		{Role: "ticket.artifacts", Path: "elsewhere", Kind: ArtifactDirectory, Exists: false},
	}
	stalePlan, err := PlanUpgrade(stale)
	if err != nil {
		t.Fatalf("plan with stale absence observations: %v", err)
	}
	assertChange(t, stalePlan, "ticket.generated", UpgradeReview, false)
	assertChange(t, stalePlan, "ticket.artifacts", UpgradeReview, false)
}

func TestPlanUpgradeReviewsAuthorityTransitions(t *testing.T) {
	t.Parallel()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/default",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	target := cloneProfile(current)
	target.Version = "v2"
	target.Artifacts[0].Authority = AuthorityGenerated
	target.Artifacts[0].Template.Version = "v2"
	currentContent := []byte("authored content\n")
	targetContent := []byte("generated replacement\n")
	plan, err := PlanUpgrade(UpgradeRequest{
		Scope:   "ticket:TASK-00039",
		Current: current,
		Target:  target,
		CurrentManifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(current),
			Artifacts: []RenderedRecord{
				renderedRecord(current.Artifacts[0], "v1", currentContent),
			},
		},
		TargetRender: RenderResult{
			Files: []RenderedFile{{
				Role: "ticket.context", Path: "context.md", Content: targetContent,
			}},
			Manifest: RenderManifest{
				SchemaVersion: RenderManifestSchemaVersion,
				Profile:       profileReference(target),
				Artifacts: []RenderedRecord{
					renderedRecord(target.Artifacts[0], "v2", targetContent),
				},
			},
		},
		Observations: []Observation{{
			Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: currentContent,
		}},
	})
	if err != nil {
		t.Fatalf("plan authority transition: %v", err)
	}
	assertChange(t, plan, "ticket.context", UpgradeReview, false)
}

func TestPlanUpgradeRejectsCrossProfileAndPathChangesAreReviewOnly(t *testing.T) {
	t.Parallel()

	current := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/default",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	target := cloneProfile(current)
	target.Version = "v2"
	target.Artifacts[0].Path = "brief.md"
	target.Artifacts[0].Template.Version = "v2"
	currentContent := []byte("context\n")
	targetContent := []byte("brief\n")
	request := UpgradeRequest{
		Scope:   "ticket:TASK-00039",
		Current: current,
		Target:  target,
		CurrentManifest: RenderManifest{
			SchemaVersion: RenderManifestSchemaVersion,
			Profile:       profileReference(current),
			Artifacts: []RenderedRecord{
				renderedRecord(current.Artifacts[0], "v1", currentContent),
			},
		},
		TargetRender: RenderResult{
			Files: []RenderedFile{{
				Role: "ticket.context", Path: "brief.md", Content: targetContent,
			}},
			Manifest: RenderManifest{
				SchemaVersion: RenderManifestSchemaVersion,
				Profile:       profileReference(target),
				Artifacts: []RenderedRecord{
					renderedRecord(target.Artifacts[0], "v2", targetContent),
				},
			},
		},
		Observations: []Observation{{
			Role: "ticket.context", Path: "context.md", Kind: ArtifactFile, Exists: true, Content: currentContent,
		}},
	}
	plan, err := PlanUpgrade(request)
	if err != nil {
		t.Fatalf("plan path-changing upgrade: %v", err)
	}
	assertChange(t, plan, "ticket.context", UpgradeReview, false)

	crossProfile := request
	crossProfile.Target.ID = "ticket/other"
	crossProfile.TargetRender.Manifest.Profile.ID = "ticket/other"
	if _, err := PlanUpgrade(crossProfile); err == nil {
		t.Fatal("cross-profile upgrade was accepted")
	}

	unresolved := request
	unresolved.Target = cloneProfile(request.Target)
	unresolved.Target.Inherits = []ProfileReference{{
		ID: "ticket/base", Version: "v1",
	}}
	if _, err := PlanUpgrade(unresolved); err == nil {
		t.Fatal("upgrade accepted an unresolved inherited target profile")
	}

	incompleteRender := request
	incompleteRender.TargetRender.Files = nil
	incompleteRender.TargetRender.Manifest.Artifacts = nil
	if _, err := PlanUpgrade(incompleteRender); err == nil {
		t.Fatal("target render missing a profile-managed role was accepted")
	}

	mismatchedTemplate := request
	mismatchedTemplate.TargetRender.Manifest.Artifacts[0].Template.Version = "v9"
	if _, err := PlanUpgrade(mismatchedTemplate); err == nil {
		t.Fatal("target render with mismatched template provenance was accepted")
	}
}

func renderedRecord(artifact Artifact, version string, content []byte) RenderedRecord {
	return RenderedRecord{
		Role: artifact.Role,
		Path: artifact.Path,
		Template: TemplateProvenance{
			ID:          artifact.Template.ID,
			Version:     version,
			SourceScope: "builtin",
		},
		GeneratedHash: HashContent(content),
		ObservedHash:  HashContent(content),
	}
}

func profileReference(profile Profile) ProfileReference {
	return ProfileReference{ID: profile.ID, Version: profile.Version}
}

func assertChange(
	t *testing.T,
	plan UpgradePlan,
	role string,
	disposition UpgradeDisposition,
	safe bool,
) {
	t.Helper()
	for _, change := range plan.Changes {
		if change.Role == role {
			if change.Disposition != disposition || change.Safe != safe {
				t.Fatalf("%s change = %#v, want disposition=%s safe=%t", role, change, disposition, safe)
			}
			return
		}
	}
	t.Fatalf("change for %q not found in %#v", role, plan.Changes)
}

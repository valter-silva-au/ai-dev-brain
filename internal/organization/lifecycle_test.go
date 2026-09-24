package organization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestUpdateOrganizationPlansThenAppliesWithoutChangingIdentity(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	initialized, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			Owner:         "platform",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize organization: %v", err)
	}

	before := snapshotOrganizationWorkspace(t, root)
	planned, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Name:          stringPointer("Amazon Web Services"),
			Owner:         stringPointer(""),
			Description:   stringPointer("Cloud and software"),
			ActorType:     "human",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan organization update: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan outcome = %q, want planned", planned.Outcome)
	}
	afterPlan := snapshotOrganizationWorkspace(t, root)
	if !reflect.DeepEqual(afterPlan, before) {
		t.Fatalf(
			"organization update plan mutated workspace\nbefore=%v\nafter=%v",
			before,
			afterPlan,
		)
	}

	applied, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Name:          stringPointer("Amazon Web Services"),
			Owner:         stringPointer(""),
			Description:   stringPointer("Cloud and software"),
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("apply organization update: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.Organization.ID != initialized.Data.OrganizationID {
		t.Fatalf(
			"organization id = %q, want %q",
			applied.Data.Organization.ID,
			initialized.Data.OrganizationID,
		)
	}
	if applied.Data.Organization.DisplayName != "Amazon Web Services" ||
		applied.Data.Organization.Owner != "" ||
		applied.Data.Organization.Description != "Cloud and software" {
		t.Fatalf("updated organization = %#v", applied.Data.Organization)
	}

	repeated, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Name:          stringPointer("Amazon Web Services"),
			Owner:         stringPointer(""),
			Description:   stringPointer("Cloud and software"),
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("repeat organization update: %v", err)
	}
	if repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat outcome = %q, want unchanged", repeated.Outcome)
	}
}

func TestUpdateOrganizationRejectsParentCycle(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "parent",
			Name:          "Parent",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("initialize parent: %v", err)
	}
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "child",
			Name:          "Child",
			Parent:        "parent",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("initialize child: %v", err)
	}

	before := snapshotOrganizationWorkspace(t, root)
	result, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "parent",
			Parent:        stringPointer("child"),
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("cycle update returned transport error: %v", err)
	}
	if result.Outcome != capability.OutcomeConflict {
		t.Fatalf("cycle outcome = %q, want conflict", result.Outcome)
	}
	after := snapshotOrganizationWorkspace(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"cycle rejection mutated workspace\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func TestMoveOrganizationPreservesIdentityAndOldSelectors(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	initialized, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize organization: %v", err)
	}
	oldLayout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new old layout: %v", err)
	}
	newLayout, err := NewLayout(root, "aws")
	if err != nil {
		t.Fatalf("new moved layout: %v", err)
	}

	before := snapshotOrganizationWorkspace(t, root)
	planned, err := service.Move(
		context.Background(),
		MoveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			NewSlug:       "aws",
			ActorType:     "human",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan organization move: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("move plan outcome = %q, want planned", planned.Outcome)
	}
	if after := snapshotOrganizationWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatalf("move plan mutated workspace")
	}

	applied, err := service.Move(
		context.Background(),
		MoveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			NewSlug:       "aws",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("apply organization move: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("move outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.Organization.ID != initialized.Data.OrganizationID ||
		applied.Data.Organization.Slug != "aws" ||
		applied.Data.Organization.Path != newLayout.Root() {
		t.Fatalf("moved organization = %#v", applied.Data.Organization)
	}
	if _, err := os.Stat(oldLayout.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old organization path still exists: %v", err)
	}
	if _, err := os.Stat(newLayout.ManifestPath()); err != nil {
		t.Fatalf("new organization manifest missing: %v", err)
	}

	selectors := []struct {
		name     string
		selector string
	}{
		{name: "new slug", selector: "aws"},
		{name: "old slug alias", selector: "amazon"},
		{name: "old path alias", selector: oldLayout.Root()},
		{name: "immutable ID", selector: initialized.Data.OrganizationID},
	}
	for _, test := range selectors {
		t.Run(test.name, func(t *testing.T) {
			shown, err := service.Show(
				context.Background(),
				ShowRequest{
					WorkspaceRoot: root,
					Selector:      test.selector,
				},
			)
			if err != nil {
				t.Fatalf(
					"show moved organization by %s: %v",
					test.name,
					err,
				)
			}
			if shown.Data.Organization.ID != initialized.Data.OrganizationID {
				t.Fatalf(
					"show by %s returned %#v",
					test.name,
					shown.Data.Organization,
				)
			}
		})
	}
}

func TestArchiveOrganizationIsReversibleAndBlocksUpdate(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("initialize organization: %v", err)
	}

	archived, err := service.Archive(
		context.Background(),
		ArchiveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("archive organization: %v", err)
	}
	if archived.Outcome != capability.OutcomeApplied ||
		archived.Data.Organization.Status != StatusArchived {
		t.Fatalf("archive result = %#v", archived)
	}

	blocked, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Name:          stringPointer("Blocked"),
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("update archived organization returned transport error: %v", err)
	}
	if blocked.Outcome != capability.OutcomeConflict {
		t.Fatalf("blocked outcome = %q, want conflict", blocked.Outcome)
	}

	restored, err := service.Archive(
		context.Background(),
		ArchiveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Restore:       true,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("restore organization: %v", err)
	}
	if restored.Outcome != capability.OutcomeApplied ||
		restored.Data.Organization.Status != StatusActive {
		t.Fatalf("restore result = %#v", restored)
	}
}

func TestAdoptOrganizationMovesContainedSourceOnlyOnApplyAndPreservesAuthoredContent(
	t *testing.T,
) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	source := filepath.Join(root, "imports", "existing-amazon")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create adoption source: %v", err)
	}
	keep := filepath.Join(source, "keep.txt")
	if err := os.WriteFile(keep, []byte("authored"), 0o644); err != nil {
		t.Fatalf("write authored content: %v", err)
	}

	service := newTestOrganizationService(t, nil)
	planned, err := service.Adopt(
		context.Background(),
		AdoptRequest{
			WorkspaceRoot: root,
			Path:          source,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan organization adoption: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("adoption plan outcome = %q, want planned", planned.Outcome)
	}
	if content, err := os.ReadFile(keep); err != nil ||
		string(content) != "authored" {
		t.Fatalf("adoption plan changed source: %q, %v", content, err)
	}

	applied, err := service.Adopt(
		context.Background(),
		AdoptRequest{
			WorkspaceRoot: root,
			Path:          source,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("apply organization adoption: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("adoption outcome = %q, want applied", applied.Outcome)
	}
	target, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new adopted layout: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adoption source still exists: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(target.Root(), "keep.txt"))
	if err != nil || string(content) != "authored" {
		t.Fatalf("adoption lost authored content: %q, %v", content, err)
	}
	if _, err := os.Stat(target.ManifestPath()); err != nil {
		t.Fatalf("adopted manifest missing: %v", err)
	}
}

func TestAdoptOrganizationRejectsExternalSourceBeforeJournal(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	source := filepath.Join(t.TempDir(), "existing-amazon")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create external adoption source: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(source, "keep.txt"),
		[]byte("authored"),
		0o644,
	); err != nil {
		t.Fatalf("write external authored content: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	beforeSource := snapshotOrganizationPath(t, source)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())
	service := newTestOrganizationService(t, nil)

	for _, apply := range []bool{false, true} {
		_, err := service.Adopt(
			context.Background(),
			AdoptRequest{
				WorkspaceRoot: root,
				Path:          source,
				Slug:          "amazon",
				Name:          "Amazon",
				ActorType:     "human",
				Tool:          "adb-cli",
				Apply:         apply,
			},
		)
		if err == nil {
			t.Fatalf("adoption apply=%t accepted external source", apply)
		}
		var violation *workspace.RoleViolation
		if !errors.As(err, &violation) {
			t.Fatalf("adoption apply=%t error = %v, want role violation", apply, err)
		}
		if violation.Reason != workspace.RoleViolationLexicalEscape {
			t.Fatalf(
				"adoption apply=%t violation = %q, want lexical escape",
				apply,
				violation.Reason,
			)
		}
	}

	assertOrganizationPathUnchanged(t, source, beforeSource)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external adoption created canonical target: %v", err)
	}
}

func TestAdoptOrganizationRefusesToOverwriteAuthoredAgentsFile(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	source := filepath.Join(root, "imports", "existing-amazon")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create adoption source: %v", err)
	}
	agents := filepath.Join(source, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Authored\n"), 0o644); err != nil {
		t.Fatalf("write authored AGENTS.md: %v", err)
	}

	service := newTestOrganizationService(t, nil)
	before := snapshotOrganizationWorkspace(t, root)
	result, err := service.Adopt(
		context.Background(),
		AdoptRequest{
			WorkspaceRoot: root,
			Path:          source,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("authored AGENTS collision returned transport error: %v", err)
	}
	if result.Outcome != capability.OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", result.Outcome)
	}
	if content, err := os.ReadFile(agents); err != nil ||
		string(content) != "# Authored\n" {
		t.Fatalf("authored AGENTS.md changed: %q, %v", content, err)
	}
	after := snapshotOrganizationWorkspace(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("refused adoption mutated workspace")
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestUpdateAfterPlanRetargetMatchingExternalContentFails(t *testing.T) {
	t.Parallel()

	root, _ := seedOrganizationForRetargetTest(t)
	request := UpdateRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		Name:          stringPointer("Amazon Web Services"),
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}
	inspection, err := inspectUpdate(context.Background(), request)
	if err != nil {
		t.Fatalf("inspect expected update: %v", err)
	}
	expected := inspection.proposed
	expected.UpdatedAt = organizationTestTime(10)
	expected.LastMutation = mutationProvenance(
		"retarget-update",
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	expectedContent, err := encodeManifest(expected)
	if err != nil {
		t.Fatalf("encode expected update manifest: %v", err)
	}
	external, beforeExternal, afterPlan := prepareMatchingRetargetForTest(
		t,
		root,
		func(externalRole string) {
			if err := os.WriteFile(
				filepath.Join(externalRole, "amazon", ".aidb", "manifest.yaml"),
				expectedContent,
				0o644,
			); err != nil {
				t.Fatalf("write matching external update manifest: %v", err)
			}
		},
	)
	service := newFixedOperationOrganizationService(
		t,
		"retarget-update",
		organizationTestTime(10),
		afterPlan,
	)
	result, err := service.Update(context.Background(), request)
	assertRetargetedOperationFailed(t, root, result.Outcome, result.Data.OperationID, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
}

func TestArchiveAfterPlanRetargetMatchingExternalContentFails(t *testing.T) {
	t.Parallel()

	root, _ := seedOrganizationForRetargetTest(t)
	request := ArchiveRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}
	current, err := loadManagedOrganization(context.Background(), root, "amazon")
	if err != nil {
		t.Fatalf("load expected archive source: %v", err)
	}
	expected := current.manifest
	now := organizationTestTime(10)
	expected.Status = StatusArchived
	expected.UpdatedAt = now
	expected.ArchivedAt = &now
	expected.LastMutation = mutationProvenance(
		"retarget-archive",
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	expectedContent, err := encodeManifest(expected)
	if err != nil {
		t.Fatalf("encode expected archive manifest: %v", err)
	}
	external, beforeExternal, afterPlan := prepareMatchingRetargetForTest(
		t,
		root,
		func(externalRole string) {
			if err := os.WriteFile(
				filepath.Join(externalRole, "amazon", ".aidb", "manifest.yaml"),
				expectedContent,
				0o644,
			); err != nil {
				t.Fatalf("write matching external archive manifest: %v", err)
			}
		},
	)
	service := newFixedOperationOrganizationService(
		t,
		"retarget-archive",
		organizationTestTime(10),
		afterPlan,
	)
	result, err := service.Archive(context.Background(), request)
	assertRetargetedOperationFailed(t, root, result.Outcome, result.Data.OperationID, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
}

func TestMoveAfterPlanRetargetMatchingExternalContentFails(t *testing.T) {
	t.Parallel()

	root, _ := seedOrganizationForRetargetTest(t)
	request := MoveRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		NewSlug:       "aws",
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}
	inspection, err := inspectMove(context.Background(), request)
	if err != nil {
		t.Fatalf("inspect expected move: %v", err)
	}
	expected := inspection.manifest
	expected.UpdatedAt = organizationTestTime(10)
	expected.LastMutation = mutationProvenance(
		"retarget-move",
		request.ActorType,
		request.ActorID,
		request.Tool,
	)
	expectedContent, err := encodeManifest(expected)
	if err != nil {
		t.Fatalf("encode expected move manifest: %v", err)
	}
	external, beforeExternal, afterPlan := prepareMatchingRetargetForTest(
		t,
		root,
		func(externalRole string) {
			copyOrganizationTreeForTest(
				t,
				filepath.Join(externalRole, "amazon"),
				filepath.Join(externalRole, "aws"),
			)
			if err := os.WriteFile(
				filepath.Join(externalRole, "aws", ".aidb", "manifest.yaml"),
				expectedContent,
				0o644,
			); err != nil {
				t.Fatalf("write matching external move manifest: %v", err)
			}
		},
	)
	service := newFixedOperationOrganizationService(
		t,
		"retarget-move",
		organizationTestTime(10),
		afterPlan,
	)
	result, err := service.Move(context.Background(), request)
	assertRetargetedOperationFailed(t, root, result.Outcome, result.Data.OperationID, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
}

func TestAdoptAfterPlanRetargetMatchingExternalContentFails(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new adoption layout: %v", err)
	}
	if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
		t.Fatalf("create canonical adoption source: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(layout.Root(), "keep.txt"),
		[]byte("authored"),
		0o644,
	); err != nil {
		t.Fatalf("write adoption source content: %v", err)
	}
	request := AdoptRequest{
		WorkspaceRoot: root,
		Path:          layout.Root(),
		Slug:          "amazon",
		Name:          "Amazon",
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}
	expected := adoptedManifest(
		request,
		"",
		"organization-test-02",
		"organization-test-03",
		organizationTestTime(3),
	)
	agents, err := AgentsPointer(layout)
	if err != nil {
		t.Fatalf("organization agents pointer: %v", err)
	}
	external, beforeExternal, afterPlan := prepareMatchingRetargetForTest(
		t,
		root,
		func(externalRole string) {
			writeMatchingOrganizationFilesForTest(
				t,
				filepath.Join(externalRole, "amazon"),
				expected,
				agents,
			)
		},
	)
	service := newTestOrganizationService(t, afterPlan)
	result, err := service.Adopt(context.Background(), request)
	assertRetargetedOperationFailed(t, root, result.Outcome, result.Data.OperationID, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
}

func TestOrganizationLifecycleUsesContainedNonDefaultOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	customRole := "managed/organizations"
	customRoot := configureOrganizationsRoleForTest(t, root, customRole)
	service := newTestOrganizationService(t, nil)
	initializeRequest := InitializeRequest{
		WorkspaceRoot: root,
		Slug:          "amazon",
		Name:          "Amazon",
		ActorType:     "human",
		Tool:          "adb-cli",
	}
	plannedInitialize, err := service.Initialize(
		context.Background(),
		initializeRequest,
	)
	if err != nil || plannedInitialize.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan custom-role initialize: %#v, %v", plannedInitialize, err)
	}
	assertOrganizationPathForTest(
		t,
		plannedInitialize.Data.Path,
		filepath.Join(customRoot, "amazon"),
	)
	assertDefaultOrganizationsRoleMissingForTest(t, root)
	initializeRequest.Apply = true
	appliedInitialize, err := service.Initialize(
		context.Background(),
		initializeRequest,
	)
	if err != nil || appliedInitialize.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply custom-role initialize: %#v, %v", appliedInitialize, err)
	}
	assertOrganizationPathForTest(
		t,
		appliedInitialize.Data.Path,
		filepath.Join(customRoot, "amazon"),
	)
	assertDefaultOrganizationsRoleMissingForTest(t, root)

	updateRequest := UpdateRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		Name:          stringPointer("Amazon Web Services"),
		ActorType:     "human",
		Tool:          "adb-cli",
	}
	plannedUpdate, err := service.Update(context.Background(), updateRequest)
	if err != nil || plannedUpdate.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan custom-role update: %#v, %v", plannedUpdate, err)
	}
	assertOrganizationPathForTest(
		t,
		plannedUpdate.Data.Organization.Path,
		filepath.Join(customRoot, "amazon"),
	)
	updateRequest.Apply = true
	appliedUpdate, err := service.Update(context.Background(), updateRequest)
	if err != nil || appliedUpdate.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply custom-role update: %#v, %v", appliedUpdate, err)
	}
	assertOrganizationPathForTest(
		t,
		appliedUpdate.Data.Organization.Path,
		filepath.Join(customRoot, "amazon"),
	)
	assertDefaultOrganizationsRoleMissingForTest(t, root)

	archiveRequest := ArchiveRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		ActorType:     "human",
		Tool:          "adb-cli",
	}
	plannedArchive, err := service.Archive(context.Background(), archiveRequest)
	if err != nil || plannedArchive.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan custom-role archive: %#v, %v", plannedArchive, err)
	}
	assertOrganizationPathForTest(
		t,
		plannedArchive.Data.Organization.Path,
		filepath.Join(customRoot, "amazon"),
	)
	archiveRequest.Apply = true
	appliedArchive, err := service.Archive(context.Background(), archiveRequest)
	if err != nil || appliedArchive.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply custom-role archive: %#v, %v", appliedArchive, err)
	}
	assertOrganizationPathForTest(
		t,
		appliedArchive.Data.Organization.Path,
		filepath.Join(customRoot, "amazon"),
	)
	archiveRequest.Restore = true
	if restored, err := service.Archive(context.Background(), archiveRequest); err != nil || restored.Outcome != capability.OutcomeApplied {
		t.Fatalf("restore custom-role organization: %#v, %v", restored, err)
	}
	assertDefaultOrganizationsRoleMissingForTest(t, root)

	moveRequest := MoveRequest{
		WorkspaceRoot: root,
		Selector:      "amazon",
		NewSlug:       "aws",
		ActorType:     "human",
		Tool:          "adb-cli",
	}
	plannedMove, err := service.Move(context.Background(), moveRequest)
	if err != nil || plannedMove.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan custom-role move: %#v, %v", plannedMove, err)
	}
	assertOrganizationPathForTest(
		t,
		plannedMove.Data.Organization.Path,
		filepath.Join(customRoot, "aws"),
	)
	moveRequest.Apply = true
	appliedMove, err := service.Move(context.Background(), moveRequest)
	if err != nil || appliedMove.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply custom-role move: %#v, %v", appliedMove, err)
	}
	assertOrganizationPathForTest(
		t,
		appliedMove.Data.Organization.Path,
		filepath.Join(customRoot, "aws"),
	)
	assertDefaultOrganizationsRoleMissingForTest(t, root)

	source := filepath.Join(root, "imports", "existing-acme")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create contained adoption source: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(source, "keep.txt"),
		[]byte("authored"),
		0o644,
	); err != nil {
		t.Fatalf("write contained adoption source: %v", err)
	}
	adoptRequest := AdoptRequest{
		WorkspaceRoot: root,
		Path:          source,
		Slug:          "acme",
		Name:          "Acme",
		ActorType:     "human",
		Tool:          "adb-cli",
	}
	plannedAdopt, err := service.Adopt(context.Background(), adoptRequest)
	if err != nil || plannedAdopt.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan custom-role adoption: %#v, %v", plannedAdopt, err)
	}
	assertOrganizationPathForTest(
		t,
		plannedAdopt.Data.Organization.Path,
		filepath.Join(customRoot, "acme"),
	)
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("adoption preview changed source: %v", err)
	}
	adoptRequest.Apply = true
	appliedAdopt, err := service.Adopt(context.Background(), adoptRequest)
	if err != nil || appliedAdopt.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply custom-role adoption: %#v, %v", appliedAdopt, err)
	}
	assertOrganizationPathForTest(
		t,
		appliedAdopt.Data.Organization.Path,
		filepath.Join(customRoot, "acme"),
	)
	if content, err := os.ReadFile(filepath.Join(customRoot, "acme", "keep.txt")); err != nil || string(content) != "authored" {
		t.Fatalf("custom-role adoption lost authored content: %q, %v", content, err)
	}
	assertDefaultOrganizationsRoleMissingForTest(t, root)
}

func seedOrganizationForRetargetTest(t *testing.T) (string, Layout) {
	t.Helper()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new seeded organization layout: %v", err)
	}
	return root, layout
}

func prepareMatchingRetargetForTest(
	t *testing.T,
	root string,
	prepare func(string),
) (string, map[string]string, func() error) {
	t.Helper()

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	manifest, err := workspace.ReadManifest(workspaceLayout.ManifestPath())
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	role, err := workspaceLayout.ResolveRole(manifest.Roles.Organizations)
	if err != nil {
		t.Fatalf("resolve organizations role: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external-organizations")
	copyOrganizationTreeForTest(t, role, external)
	prepare(external)
	beforeExternal := snapshotOrganizationPath(t, external)
	backup := filepath.Join(t.TempDir(), "original-organizations")
	return external, beforeExternal, func() error {
		return replaceOrganizationsRoleWithSymlinkForTest(
			root,
			external,
			backup,
		)
	}
}

func assertRetargetedOperationFailed(
	t *testing.T,
	root string,
	outcome capability.Outcome,
	operationID string,
	err error,
) {
	t.Helper()

	if err == nil {
		t.Fatal("operation committed after organizations role was retargeted")
	}
	if outcome == capability.OutcomeApplied {
		t.Fatalf("retargeted operation outcome = %q, want failure", outcome)
	}
	if operationID == "" {
		t.Fatalf("retargeted operation omitted its journal operation id: %v", err)
	}
	assertOrganizationOperationNotCommitted(t, root, operationID)
}

func assertOrganizationPathForTest(t *testing.T, got string, want string) {
	t.Helper()

	if got != want {
		t.Fatalf("organization path = %q, want %q", got, want)
	}
}

func TestUpdateRejectsRedirectedOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	external := redirectOrganizationsRoleForTest(t, root)
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())

	_, err = service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			Name:          stringPointer("Changed"),
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	assertOrganizationsRoleViolation(t, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
}

func TestArchiveRejectsRedirectedOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	external := redirectOrganizationsRoleForTest(t, root)
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())

	_, err = service.Archive(
		context.Background(),
		ArchiveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	assertOrganizationsRoleViolation(t, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
}

func TestMoveRejectsRedirectedOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	if _, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	external := redirectOrganizationsRoleForTest(t, root)
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())

	_, err = service.Move(
		context.Background(),
		MoveRequest{
			WorkspaceRoot: root,
			Selector:      "amazon",
			NewSlug:       "aws",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	assertOrganizationsRoleViolation(t, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
}

func TestAdoptRejectsRedirectedOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	external := redirectOrganizationsRoleForTest(t, root)
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	source := filepath.Join(t.TempDir(), "existing-amazon")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create adoption source: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(source, "keep.txt"),
		[]byte("authored"),
		0o644,
	); err != nil {
		t.Fatalf("write authored source: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	beforeSource := snapshotOrganizationPath(t, source)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())

	service := newTestOrganizationService(t, nil)
	_, err = service.Adopt(
		context.Background(),
		AdoptRequest{
			WorkspaceRoot: root,
			Path:          source,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	assertOrganizationsRoleViolation(t, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	assertOrganizationPathUnchanged(t, source, beforeSource)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
}

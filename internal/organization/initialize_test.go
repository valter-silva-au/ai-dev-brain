package organization

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestInitializePlansMinimalOrganizationWithoutWriting(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	result, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			ActorID:       "local-user",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan organization initialization: %v", err)
	}
	if result.Outcome != capability.OutcomePlanned {
		t.Fatalf("outcome = %q, want planned", result.Outcome)
	}
	if result.Data.OrganizationID == "" {
		t.Fatal("planned organization id is empty")
	}

	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	wantEffects := []capability.Effect{
		{
			Action: "create",
			Target: layout.Root(),
			Status: capability.EffectPlanned,
		},
		{
			Action: "create",
			Target: layout.ControlDir(),
			Status: capability.EffectPlanned,
		},
		{
			Action: "create",
			Target: layout.ConfigPath(),
			Status: capability.EffectPlanned,
		},
		{
			Action: "create",
			Target: layout.AgentsPath(),
			Status: capability.EffectPlanned,
		},
		{
			Action: "create",
			Target: layout.ManifestPath(),
			Status: capability.EffectPlanned,
		},
		{
			Action: "project",
			Target: filepath.Join(root, ".aidb", "state.sqlite"),
			Status: capability.EffectPlanned,
		},
	}
	if !reflect.DeepEqual(result.Effects, wantEffects) {
		t.Fatalf("planned effects\n got: %#v\nwant: %#v", result.Effects, wantEffects)
	}
	if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plan created organization root: %v", err)
	}
}

func TestInitializeAppliesMinimalOrganizationAndIsIdempotent(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	request := InitializeRequest{
		WorkspaceRoot: root,
		Slug:          "amazon",
		Name:          "Amazon",
		Owner:         "platform",
		Description:   "Amazon software organization",
		Trust:         "confidential",
		Profile:       "default",
		ActorType:     "human",
		ActorID:       "local-user",
		Tool:          "adb-cli",
		Apply:         true,
	}
	applied, err := service.Initialize(context.Background(), request)
	if err != nil {
		t.Fatalf("apply organization initialization: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.OrganizationID == "" || applied.Data.OperationID == "" {
		t.Fatalf("applied result omitted identifiers: %#v", applied.Data)
	}

	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	for _, path := range []string{
		layout.ManifestPath(),
		layout.ConfigPath(),
		layout.AgentsPath(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected initialized path %q: %v", path, err)
		}
	}
	for _, path := range []string{
		layout.KnowledgeDir(),
		layout.StakeholdersDir(),
		layout.TicketsDir(),
		layout.RepositoriesDir(),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("optional role %q was eagerly created: %v", path, err)
		}
	}

	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read organization manifest: %v", err)
	}
	if manifest.ID != applied.Data.OrganizationID ||
		manifest.Owner != "platform" ||
		manifest.Trust != "confidential" ||
		manifest.Provenance.OperationID != applied.Data.OperationID {
		t.Fatalf("organization manifest = %#v", manifest)
	}

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	state, err := controlplane.Open(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	projection, err := state.Organization(
		context.Background(),
		applied.Data.OrganizationID,
	)
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("close control plane: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("read organization projection: %v", err)
	}
	if projection.Slug != "amazon" ||
		projection.Path != layout.Root() ||
		projection.ManifestHash == "" {
		t.Fatalf("organization projection = %#v", projection)
	}

	journals, err := journal.NewStore(
		workspaceLayout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	journalState, err := journals.Inspect(applied.Data.OperationID)
	if err != nil {
		t.Fatalf("inspect organization operation: %v", err)
	}
	if journalState.Status != journal.StatusCommitted {
		t.Fatalf("journal status = %q, want committed", journalState.Status)
	}

	repeated, err := service.Initialize(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat organization initialization: %v", err)
	}
	if repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat outcome = %q, want unchanged", repeated.Outcome)
	}
	if repeated.Data.OrganizationID != applied.Data.OrganizationID {
		t.Fatalf(
			"repeat organization id = %q, want %q",
			repeated.Data.OrganizationID,
			applied.Data.OrganizationID,
		)
	}
	if repeated.Data.OperationID != "" {
		t.Fatalf("repeat operation id = %q, want empty", repeated.Data.OperationID)
	}
}

func TestInitializeResolvesExistingParentByStableSelector(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	parent, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "parent",
			Name:          "Parent",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize parent: %v", err)
	}

	child, err := service.Initialize(
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
	)
	if err != nil {
		t.Fatalf("initialize child: %v", err)
	}
	childLayout, err := NewLayout(root, "child")
	if err != nil {
		t.Fatalf("new child layout: %v", err)
	}
	manifest, err := ReadManifest(childLayout.ManifestPath())
	if err != nil {
		t.Fatalf("read child manifest: %v", err)
	}
	if manifest.ParentID != parent.Data.OrganizationID {
		t.Fatalf(
			"child parent id = %q, want %q",
			manifest.ParentID,
			parent.Data.OrganizationID,
		)
	}
	if child.Data.OrganizationID == parent.Data.OrganizationID {
		t.Fatal("child reused parent immutable identity")
	}
}

func TestInitializeReturnsConflictWithoutMutatingManagedOrUnmanagedPaths(
	t *testing.T,
) {
	t.Parallel()

	t.Run("managed metadata collision", func(t *testing.T) {
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
		layout, err := NewLayout(root, "amazon")
		if err != nil {
			t.Fatalf("new organization layout: %v", err)
		}
		before, err := os.ReadFile(layout.ManifestPath())
		if err != nil {
			t.Fatalf("read manifest before collision: %v", err)
		}

		result, err := service.Initialize(
			context.Background(),
			InitializeRequest{
				WorkspaceRoot: root,
				Slug:          "amazon",
				Name:          "Different",
				ActorType:     "human",
				Tool:          "adb-cli",
				Apply:         true,
			},
		)
		if err != nil {
			t.Fatalf("managed collision returned transport error: %v", err)
		}
		if result.Outcome != capability.OutcomeConflict {
			t.Fatalf("outcome = %q, want conflict", result.Outcome)
		}
		after, err := os.ReadFile(layout.ManifestPath())
		if err != nil {
			t.Fatalf("read manifest after collision: %v", err)
		}
		if string(after) != string(before) {
			t.Fatal("managed collision overwrote manifest")
		}
	})

	t.Run("unmanaged directory", func(t *testing.T) {
		t.Parallel()

		root := initializeTestWorkspace(t)
		layout, err := NewLayout(root, "amazon")
		if err != nil {
			t.Fatalf("new organization layout: %v", err)
		}
		if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
			t.Fatalf("create unmanaged organization directory: %v", err)
		}
		unmanaged := filepath.Join(layout.Root(), "keep.txt")
		if err := os.WriteFile(unmanaged, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write unmanaged content: %v", err)
		}

		service := newTestOrganizationService(t, nil)
		result, err := service.Initialize(
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
			t.Fatalf("unmanaged collision returned transport error: %v", err)
		}
		if result.Outcome != capability.OutcomeConflict {
			t.Fatalf("outcome = %q, want conflict", result.Outcome)
		}
		content, err := os.ReadFile(unmanaged)
		if err != nil || string(content) != "keep" {
			t.Fatalf("unmanaged content changed: %q, %v", content, err)
		}
		if _, err := os.Stat(layout.ManifestPath()); !errors.Is(
			err,
			os.ErrNotExist,
		) {
			t.Fatalf("initialize claimed unmanaged directory: %v", err)
		}
	})
}

func TestInitializeRejectsMissingParentWithoutWriting(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	service := newTestOrganizationService(t, nil)
	result, err := service.Initialize(
		context.Background(),
		InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "child",
			Name:          "Child",
			Parent:        "missing",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("missing parent returned transport error: %v", err)
	}
	if result.Outcome != capability.OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", result.Outcome)
	}
	layout, err := NewLayout(root, "child")
	if err != nil {
		t.Fatalf("new child layout: %v", err)
	}
	if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-parent initialize wrote child root: %v", err)
	}
}

func TestInitializeFailureAfterPlanLeavesInspectableJournal(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	injected := errors.New("injected after-plan failure")
	service := newTestOrganizationService(t, func() error { return injected })
	result, err := service.Initialize(
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
	if !errors.Is(err, injected) {
		t.Fatalf("initialize error = %v, want injected failure", err)
	}
	if result.Outcome != capability.OutcomeFailed ||
		!result.Recovery.Required ||
		result.Data.OperationID == "" {
		t.Fatalf("failed result = %#v", result)
	}

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	journals, err := journal.NewStore(
		workspaceLayout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := journals.Inspect(result.Data.OperationID)
	if err != nil {
		t.Fatalf("inspect failed operation: %v", err)
	}
	if state.Status != journal.StatusApplying {
		t.Fatalf("journal status = %q, want applying", state.Status)
	}

	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failure after plan wrote organization root: %v", err)
	}
}

func initializeTestWorkspace(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "AWS")
	service, err := foundation.NewService(foundation.Options{})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	if _, err := service.Initialize(
		context.Background(),
		foundation.InitializeRequest{
			Root:  root,
			Name:  "AWS",
			Apply: true,
		},
	); err != nil {
		t.Fatalf("initialize test workspace: %v", err)
	}
	return root
}

func newTestOrganizationService(
	t *testing.T,
	afterPlan func() error,
) *Service {
	t.Helper()

	index := 0
	service, err := NewService(Options{
		Clock: func() time.Time {
			return organizationTestTime(index)
		},
		IDGenerator: func() string {
			index++
			return fmt.Sprintf("organization-test-%02d", index)
		},
		AfterPlan: afterPlan,
	})
	if err != nil {
		t.Fatalf("new organization service: %v", err)
	}
	return service
}

func newFixedOperationOrganizationService(
	t *testing.T,
	operationID string,
	now time.Time,
	afterPlan func() error,
) *Service {
	t.Helper()

	index := 0
	service, err := NewService(Options{
		Clock: func() time.Time { return now },
		IDGenerator: func() string {
			index++
			if index == 1 {
				return operationID
			}
			return fmt.Sprintf("%s-event-%02d", operationID, index)
		},
		AfterPlan: afterPlan,
	})
	if err != nil {
		t.Fatalf("new fixed-operation organization service: %v", err)
	}
	return service
}

func organizationTestTime(index int) time.Time {
	return time.Date(
		2026,
		time.September,
		10,
		11,
		0,
		index,
		0,
		time.UTC,
	)
}

func writeMatchingOrganizationFilesForTest(
	t *testing.T,
	root string,
	manifest Manifest,
	agents string,
) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(root, ".aidb"), 0o755); err != nil {
		t.Fatalf("create external organization control directory: %v", err)
	}
	content, err := encodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode external organization manifest: %v", err)
	}
	for _, file := range []struct {
		path    string
		content []byte
	}{
		{filepath.Join(root, ".aidb", "config.yaml"), []byte(defaultConfig)},
		{filepath.Join(root, "AGENTS.md"), []byte(agents)},
		{filepath.Join(root, ".aidb", "manifest.yaml"), content},
	} {
		if err := os.WriteFile(file.path, file.content, 0o644); err != nil {
			t.Fatalf("write matching external file %q: %v", file.path, err)
		}
	}
}

func assertOrganizationOperationNotCommitted(
	t *testing.T,
	root string,
	operationID string,
) {
	t.Helper()

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	store, err := journal.NewStore(
		workspaceLayout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := store.Inspect(operationID)
	if err != nil {
		t.Fatalf("inspect organization operation %q: %v", operationID, err)
	}
	if state.Status == journal.StatusCommitted {
		t.Fatalf("operation %q committed after containment failure", operationID)
	}
}

func TestInitializeAfterPlanRetargetMatchingExternalContentFails(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	layout, err := NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	manifest := NewManifest(
		"organization-test-01",
		"amazon",
		"Amazon",
		organizationTestTime(2),
		Provenance{
			OperationID: "organization-test-02",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	agents, err := AgentsPointer(layout)
	if err != nil {
		t.Fatalf("organization agents pointer: %v", err)
	}
	externalRole := filepath.Join(t.TempDir(), "external-organizations")
	writeMatchingOrganizationFilesForTest(
		t,
		filepath.Join(externalRole, "amazon"),
		manifest,
		agents,
	)
	beforeExternal := snapshotOrganizationPath(t, externalRole)

	originalRoleBackup := filepath.Join(t.TempDir(), "original-organizations")
	service := newTestOrganizationService(t, func() error {
		return replaceOrganizationsRoleWithSymlinkForTest(
			root,
			externalRole,
			originalRoleBackup,
		)
	})
	result, err := service.Initialize(
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
	if err == nil {
		t.Fatal("initialize committed after organizations role was retargeted")
	}
	if result.Outcome == capability.OutcomeApplied {
		t.Fatalf("initialize outcome = %q, want failure", result.Outcome)
	}
	assertOrganizationOperationNotCommitted(t, root, "organization-test-02")
	assertOrganizationPathUnchanged(t, externalRole, beforeExternal)
}

func TestApplyJournaledFileRejectsRetargetedMatchingContent(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	events := filepath.Join(root, ".aidb", "events")
	store, err := journal.NewStore(
		events,
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	const operationID = "retargeted-create"
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: "test:retargeted-create",
		Kind:           InitializeDescriptor.Capability,
		Steps: []journal.Step{{
			Ordinal: 1,
			Action:  "create",
			Target:  filepath.Join(root, "organizations", "amazon", "AGENTS.md"),
		}},
	}); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		t.Fatalf("append applying event: %v", err)
	}

	role := filepath.Join(root, "organizations")
	if err := os.MkdirAll(role, 0o755); err != nil {
		t.Fatalf("create organizations role: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external-organizations")
	target := filepath.Join(external, "amazon", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create external target parent: %v", err)
	}
	content := []byte("matching external content\n")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatalf("write matching external content: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	backup := filepath.Join(t.TempDir(), "original-organizations")
	if err := replaceOrganizationsRoleWithSymlinkForTest(
		root,
		external,
		backup,
	); err != nil {
		t.Fatalf("retarget organizations role: %v", err)
	}

	err = applyJournaledFile(
		root,
		store,
		operationID,
		1,
		filepath.Join(role, "amazon", "AGENTS.md"),
		content,
	)
	if err == nil {
		t.Fatal("journaled create accepted matching content through a retargeted role")
	}
	assertOrganizationPathUnchanged(t, external, beforeExternal)
}

func TestApplyJournaledReplacementRejectsRetargetedMatchingAfterContent(
	t *testing.T,
) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	events := filepath.Join(root, ".aidb", "events")
	store, err := journal.NewStore(
		events,
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	const operationID = "retargeted-replacement"
	target := filepath.Join(
		root,
		"organizations",
		"amazon",
		".aidb",
		"manifest.yaml",
	)
	before := []byte("before\n")
	after := []byte("matching after content\n")
	if _, err := store.Begin(journal.Plan{
		OperationID:    operationID,
		IdempotencyKey: "test:retargeted-replacement",
		Kind:           UpdateDescriptor.Capability,
		Steps: []journal.Step{{
			Ordinal:    1,
			Action:     "replace",
			Target:     target,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		}},
	}); err != nil {
		t.Fatalf("begin journal: %v", err)
	}
	if _, err := store.Append(operationID, journal.EventInput{
		Phase: journal.PhaseApplying,
	}); err != nil {
		t.Fatalf("append applying event: %v", err)
	}

	role := filepath.Join(root, "organizations")
	external := filepath.Join(t.TempDir(), "external-organizations")
	externalTarget := filepath.Join(
		external,
		"amazon",
		".aidb",
		"manifest.yaml",
	)
	if err := os.MkdirAll(filepath.Dir(externalTarget), 0o755); err != nil {
		t.Fatalf("create external target parent: %v", err)
	}
	if err := os.WriteFile(externalTarget, after, 0o644); err != nil {
		t.Fatalf("write matching external replacement: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	backup := filepath.Join(t.TempDir(), "original-organizations")
	if err := replaceOrganizationsRoleWithSymlinkForTest(
		root,
		external,
		backup,
	); err != nil {
		t.Fatalf("retarget organizations role: %v", err)
	}

	err = applyJournaledReplacement(
		root,
		store,
		operationID,
		1,
		target,
		before,
		after,
	)
	if err == nil {
		t.Fatal("journaled replacement accepted matching after content through a retargeted role")
	}
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	if _, err := os.Lstat(role); err != nil {
		t.Fatalf("inspect retargeted role: %v", err)
	}
}

func TestInitializeRejectsRedirectedOrganizationsRole(t *testing.T) {
	t.Parallel()

	root := initializeTestWorkspace(t)
	external := redirectOrganizationsRoleForTest(t, root)
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	beforeExternal := snapshotOrganizationPath(t, external)
	beforeEvents := snapshotOrganizationPath(t, workspaceLayout.EventsDir())

	service := newTestOrganizationService(t, nil)
	_, err = service.Initialize(
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
	assertOrganizationsRoleViolation(t, err)
	assertOrganizationPathUnchanged(t, external, beforeExternal)
	assertOrganizationPathUnchanged(t, workspaceLayout.EventsDir(), beforeEvents)
}

func replaceOrganizationsRoleWithSymlinkForTest(
	root string,
	external string,
	backup string,
) error {
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		return err
	}
	manifest, err := workspace.ReadManifest(workspaceLayout.ManifestPath())
	if err != nil {
		return err
	}
	role, err := workspaceLayout.ResolveRole(manifest.Roles.Organizations)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(role); err == nil {
		if err := os.Rename(role, backup); err != nil {
			return fmt.Errorf("move organizations role aside: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect organizations role: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(role), 0o755); err != nil {
		return fmt.Errorf("create organizations role parent: %w", err)
	}
	if err := os.Symlink(external, role); err != nil {
		return fmt.Errorf("symlink organizations role: %w", err)
	}
	return nil
}

func configureOrganizationsRoleForTest(
	t *testing.T,
	root string,
	role string,
) string {
	t.Helper()

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	manifest, err := workspace.ReadManifest(workspaceLayout.ManifestPath())
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	defaultRoot, err := workspaceLayout.ResolveRole(
		workspace.DefaultRoles().Organizations,
	)
	if err != nil {
		t.Fatalf("resolve default organizations role: %v", err)
	}
	if err := os.Remove(defaultRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove empty default organizations role: %v", err)
	}
	manifest.Roles.Organizations = role
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode workspace manifest: %v", err)
	}
	if err := os.WriteFile(
		workspaceLayout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write workspace manifest: %v", err)
	}
	customRoot, err := workspaceLayout.ResolveRole(role)
	if err != nil {
		t.Fatalf("resolve custom organizations role: %v", err)
	}
	return customRoot
}

func assertDefaultOrganizationsRoleMissingForTest(t *testing.T, root string) {
	t.Helper()

	defaultRoot := filepath.Join(root, workspace.DefaultRoles().Organizations)
	if _, err := os.Lstat(defaultRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default organizations role was touched: %v", err)
	}
}

func copyOrganizationTreeForTest(t *testing.T, source string, target string) {
	t.Helper()

	if err := filepath.WalkDir(
		source,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			destination := filepath.Join(target, relative)
			if entry.IsDir() {
				return os.MkdirAll(destination, 0o755)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(destination, content, 0o644)
		},
	); err != nil {
		t.Fatalf("copy organization tree: %v", err)
	}
}

func redirectOrganizationsRoleForTest(t *testing.T, root string) string {
	t.Helper()

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	manifest, err := workspace.ReadManifest(workspaceLayout.ManifestPath())
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	original, err := workspaceLayout.ResolveRole(manifest.Roles.Organizations)
	if err != nil {
		t.Fatalf("resolve original organizations role: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external-organizations")
	if _, err := os.Stat(original); err == nil {
		if err := os.Rename(original, external); err != nil {
			t.Fatalf("move organizations role outside workspace: %v", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(external, 0o755); err != nil {
			t.Fatalf("create external organizations directory: %v", err)
		}
	} else {
		t.Fatalf("inspect original organizations role: %v", err)
	}

	manifest.Roles.Organizations = "redirected-organizations"
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode redirected workspace manifest: %v", err)
	}
	if err := os.WriteFile(
		workspaceLayout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write redirected workspace manifest: %v", err)
	}
	redirected, err := workspaceLayout.ResolveRole(manifest.Roles.Organizations)
	if err != nil {
		t.Fatalf("resolve redirected organizations role: %v", err)
	}
	if err := os.Symlink(external, redirected); err != nil {
		t.Fatalf("symlink redirected organizations role: %v", err)
	}

	state, err := controlplane.Open(context.Background(), workspaceLayout.StatePath())
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	projections, err := state.Organizations(context.Background())
	if err != nil {
		_ = state.Close()
		t.Fatalf("list organization projections: %v", err)
	}
	for _, projection := range projections {
		projection.Path = filepath.Join(redirected, projection.Slug)
		if err := state.ObserveOrganization(
			context.Background(),
			projection,
		); err != nil {
			_ = state.Close()
			t.Fatalf("reproject redirected organization: %v", err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatalf("close control plane: %v", err)
	}
	return external
}

func assertOrganizationsRoleViolation(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("operation accepted redirected organizations role")
	}
	var violation *workspace.RoleViolation
	if !errors.As(err, &violation) {
		t.Fatalf("error = %v, want workspace role violation", err)
	}
	if violation.Reason != workspace.RoleViolationSymlink {
		t.Fatalf("violation reason = %q, want symlink", violation.Reason)
	}
}

func snapshotOrganizationPath(t *testing.T, path string) map[string]string {
	t.Helper()

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return map[string]string{".": "missing"}
	} else if err != nil {
		t.Fatalf("inspect snapshot path %q: %v", path, err)
	}
	return snapshotOrganizationWorkspace(t, path)
}

func assertOrganizationPathUnchanged(
	t *testing.T,
	path string,
	before map[string]string,
) {
	t.Helper()

	after := snapshotOrganizationPath(t, path)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"path %q changed\nbefore=%v\nafter=%v",
			path,
			before,
			after,
		)
	}
}

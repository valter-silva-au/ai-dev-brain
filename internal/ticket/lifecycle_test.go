package ticket

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestCreatePreviewApplyBootstrapsPortableArtifactsBeforeProjection(
	t *testing.T,
) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	var checkedProjectionOrder atomic.Bool
	service := testLifecycleService(t, Options{
		BeforeProjection: func(
			ctx context.Context,
			layout Layout,
			manifest Manifest,
		) error {
			if _, err := os.Stat(layout.StatusPath()); err != nil {
				return fmt.Errorf("portable status was not written first: %w", err)
			}
			state, err := controlplane.OpenReadOnly(
				ctx,
				workspaceLayout.StatePath(),
			)
			if err != nil {
				return err
			}
			defer state.Close()
			_, err = state.Ticket(
				ctx,
				controlplane.TicketScope{
					OrganizationID: manifest.OrganizationID,
				},
				manifest.ID,
			)
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf(
					"ticket projection existed before projection step: %v",
					err,
				)
			}
			checkedProjectionOrder.Store(true)
			return nil
		},
	})
	request := CreateRequest{
		Scope:     scope,
		Title:     "Ship v3 lifecycle — unrestricted title",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		ActorID:   "local-user",
		Tool:      "adb-cli",
	}

	planned, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("preview create: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("preview outcome = %q", planned.Outcome)
	}
	if _, err := os.Stat(planned.Data.Ticket.Path); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatalf("preview mutated ticket path: %v", err)
	}

	request.Apply = true
	applied, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply outcome = %q", applied.Outcome)
	}
	if !checkedProjectionOrder.Load() {
		t.Fatal("projection-order checkpoint did not run")
	}
	layout := applied.Data.Ticket.Layout
	for _, path := range []string{
		layout.StatusPath(),
		filepath.Join(layout.Root(), "context.md"),
		filepath.Join(layout.Root(), "notes.md"),
		layout.ConfigPath(),
		layout.RenderManifestPath(),
		layout.TombstonesPath(),
	} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("portable file %q missing or invalid: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(layout.Root(), "artifacts"),
		filepath.Join(layout.Root(), "scratch"),
	} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("portable directory %q missing or invalid: %v", path, err)
		}
	}
	manifest := readTicketManifest(t, layout)
	notes := readFile(t, filepath.Join(layout.Root(), "notes.md"))
	checkpoint := checkpointByRole(
		t,
		manifest.AppendOnlyCheckpoints,
		"ticket.notes",
	)
	if checkpoint.Length != int64(len(notes)) ||
		checkpoint.Hash != profile.HashContent(notes) {
		t.Fatalf("notes checkpoint = %#v", checkpoint)
	}

	state, err := controlplane.OpenReadOnly(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer state.Close()
	projection, err := state.Ticket(
		context.Background(),
		controlplane.TicketScope{OrganizationID: manifest.OrganizationID},
		manifest.ID,
	)
	if err != nil {
		t.Fatalf("read ticket projection: %v", err)
	}
	if projection.Path != layout.Root() ||
		len(projection.Artifacts) != len(activeProfile.Artifacts) {
		t.Fatalf("ticket projection = %#v", projection)
	}
}

func TestListShowUpdateAndAppendOnlyCheckpointing(t *testing.T) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	first := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Zulu ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	second := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Alpha ticket",
		Type:      "fix",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})

	listed, err := service.List(context.Background(), ListRequest{Scope: scope})
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if listed.Outcome != capability.OutcomeHealthy ||
		len(listed.Data.Tickets) != 2 ||
		listed.Data.Tickets[0].Manifest.VisibleKey !=
			first.Ticket.Manifest.VisibleKey ||
		listed.Data.Tickets[1].Manifest.VisibleKey !=
			second.Ticket.Manifest.VisibleKey {
		t.Fatalf("deterministic list = %#v", listed)
	}
	shown, err := service.Show(context.Background(), ShowRequest{
		Scope:    scope,
		Selector: second.Ticket.Manifest.ID,
	})
	if err != nil {
		t.Fatalf("show ticket: %v", err)
	}
	if shown.Data.Ticket.Manifest.ID != second.Ticket.Manifest.ID {
		t.Fatalf("shown ticket = %#v", shown.Data.Ticket)
	}
	originalPath := shown.Data.Ticket.Path
	originalBranch := shown.Data.Ticket.Manifest.Branch
	updatedTitle := "A title with punctuation — and no path rewrite!"
	titleUpdate, err := service.Update(
		context.Background(),
		UpdateRequest{
			Scope:     scope,
			Selector:  second.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Title:     &updatedTitle,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("update unrestricted title: %v", err)
	}
	if titleUpdate.Data.Ticket.Manifest.Title != updatedTitle ||
		titleUpdate.Data.Ticket.Path != originalPath ||
		titleUpdate.Data.Ticket.Manifest.Branch != originalBranch {
		t.Fatalf("title update changed independent identity: %#v", titleUpdate)
	}

	notesPath := filepath.Join(titleUpdate.Data.Ticket.Path, "notes.md")
	notes, err := os.OpenFile(notesPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("open notes for append: %v", err)
	}
	if _, err := notes.WriteString("\n2026-09-10: lifecycle progress.\n"); err != nil {
		_ = notes.Close()
		t.Fatalf("append notes: %v", err)
	}
	if err := notes.Close(); err != nil {
		t.Fatalf("close notes: %v", err)
	}
	updated, err := service.Update(context.Background(), UpdateRequest{
		Scope:     scope,
		Selector:  titleUpdate.Data.Ticket.Manifest.ID,
		Profile:   activeProfile,
		ActorType: "agent",
		Tool:      "adb-cli",
		Apply:     true,
	})
	if err != nil {
		t.Fatalf("checkpoint append: %v", err)
	}
	if updated.Outcome != capability.OutcomeApplied {
		t.Fatalf("append checkpoint outcome = %q", updated.Outcome)
	}
	accepted := readTicketManifest(t, updated.Data.Ticket.Layout)
	currentNotes := readFile(t, notesPath)
	checkpoint := checkpointByRole(
		t,
		accepted.AppendOnlyCheckpoints,
		"ticket.notes",
	)
	if checkpoint.Length != int64(len(currentNotes)) ||
		checkpoint.Hash != profile.HashContent(currentNotes) {
		t.Fatalf("advanced checkpoint = %#v", checkpoint)
	}
	state, err := controlplane.OpenReadOnly(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open state after append: %v", err)
	}
	projection, err := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: accepted.OrganizationID,
		},
		accepted.ID,
	)
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("close state after append: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("read projection after append: %v", err)
	}
	foundNotes := false
	for _, artifact := range projection.Artifacts {
		if artifact.Role != "ticket.notes" {
			continue
		}
		foundNotes = true
		if artifact.SourceHash != profile.HashContent(currentNotes) {
			t.Fatalf("projected notes hash = %q", artifact.SourceHash)
		}
	}
	if !foundNotes {
		t.Fatal("notes artifact was not projected")
	}

	currentNotes[0] ^= 1
	if err := os.WriteFile(notesPath, currentNotes, 0o644); err != nil {
		t.Fatalf("rewrite notes prefix: %v", err)
	}
	refused, err := service.Update(context.Background(), UpdateRequest{
		Scope:     scope,
		Selector:  accepted.ID,
		Profile:   activeProfile,
		ActorType: "agent",
		Tool:      "adb-cli",
		Apply:     true,
	})
	if err != nil {
		t.Fatalf("inspect append-only rewrite: %v", err)
	}
	if refused.Outcome != capability.OutcomeConflict ||
		!hasTicketFinding(
			refused.Data.Findings,
			"ticket.append_only.prefix_changed",
		) {
		t.Fatalf("append-only rewrite refusal = %#v", refused)
	}
}

func TestUpdateMovesPathAndBranchIndependently(t *testing.T) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Stable display title",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	oldPath := created.Ticket.Path
	newPathSlug := "relocated-ticket"
	newBranchSlug := "independent-branch"
	updated, err := service.Update(
		context.Background(),
		UpdateRequest{
			Scope:      scope,
			Selector:   created.Ticket.Manifest.ID,
			Profile:    activeProfile,
			PathSlug:   &newPathSlug,
			BranchSlug: &newBranchSlug,
			ActorType:  "human",
			Tool:       "adb-cli",
			Apply:      true,
		},
	)
	if err != nil {
		t.Fatalf("move ticket path and branch: %v", err)
	}
	if updated.Data.Ticket.Manifest.Title != "Stable display title" ||
		updated.Data.Ticket.Manifest.PathSlug != newPathSlug ||
		updated.Data.Ticket.Manifest.Branch.Slug != newBranchSlug ||
		updated.Data.Ticket.Manifest.Branch.Intent !=
			"feat/"+updated.Data.Ticket.Manifest.LocalKey+"-"+newBranchSlug {
		t.Fatalf("independent path/branch update = %#v", updated.Data.Ticket)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old ticket path remains after move: %v", err)
	}
	if _, err := os.Stat(updated.Data.Ticket.Path); err != nil {
		t.Fatalf("new ticket path missing after move: %v", err)
	}
	state, err := controlplane.OpenReadOnly(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open state after move: %v", err)
	}
	projection, err := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: updated.Data.Ticket.Manifest.OrganizationID,
		},
		oldPath,
	)
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("close state after move: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("lookup moved ticket by old path: %v", err)
	}
	if projection.ID != updated.Data.Ticket.Manifest.ID {
		t.Fatalf("old path resolved to %q", projection.ID)
	}
}

func TestUpdateRejectsProfileUpgradeWithoutReconcilePlan(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Profile-managed ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	before := readFile(t, created.Ticket.Layout.StatusPath())
	nextProfile := activeProfile
	nextProfile.Artifacts = append(
		[]profile.Artifact(nil),
		activeProfile.Artifacts...,
	)
	nextProfile.Artifacts[0].Path = "different-structural-path.md"

	_, err := service.Update(context.Background(), UpdateRequest{
		Scope:       scope,
		Selector:    created.Ticket.Manifest.ID,
		Profile:     activeProfile,
		NextProfile: &nextProfile,
		ActorType:   "human",
		Tool:        "adb-cli",
		Apply:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "upgrade/reconcile planner") {
		t.Fatalf("profile upgrade error = %v", err)
	}
	if after := readFile(t, created.Ticket.Layout.StatusPath()); !bytes.Equal(
		before,
		after,
	) {
		t.Fatal("rejected profile upgrade changed portable source")
	}
}

func TestUpdateRelocationLeavesInspectableRecoveryAfterStagingInterruption(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	var stagedPath string
	service := testLifecycleService(t, Options{
		AfterFilesystemStage: func(path string) error {
			stagedPath = path
			return errors.New("injected relocation interruption")
		},
	})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Relocation recovery",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	pathSlug := "relocation-recovery-moved"
	result, err := service.Update(context.Background(), UpdateRequest{
		Scope:     scope,
		Selector:  created.Ticket.Manifest.ID,
		Profile:   activeProfile,
		PathSlug:  &pathSlug,
		ActorType: "human",
		Tool:      "adb-cli",
		Apply:     true,
	})
	if err == nil {
		t.Fatal("injected relocation interruption returned no error")
	}
	if result.Outcome != capability.OutcomeFailed ||
		result.Data.RecoveryPath == "" ||
		result.Data.RecoveryPath != stagedPath {
		t.Fatalf("relocation interruption result = %#v", result)
	}
	if _, statErr := os.Stat(created.Ticket.Path); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("old ticket path remains after staging: %v", statErr)
	}
	if _, statErr := os.Stat(result.Data.Ticket.Path); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("new ticket path published during interruption: %v", statErr)
	}
	statusRelative, relativeErr := filepath.Rel(
		created.Ticket.Path,
		created.Ticket.Layout.StatusPath(),
	)
	if relativeErr != nil {
		t.Fatalf("resolve recovery status path: %v", relativeErr)
	}
	file, openErr := os.Open(filepath.Join(result.Data.RecoveryPath, statusRelative))
	if openErr != nil {
		t.Fatalf("open recovery manifest: %v", openErr)
	}
	recovered, decodeErr := DecodeManifest(file)
	closeErr := file.Close()
	if decodeErr != nil {
		t.Fatalf("decode recovery manifest: %v", decodeErr)
	}
	if closeErr != nil {
		t.Fatalf("close recovery manifest: %v", closeErr)
	}
	if recovered.ID != created.Ticket.Manifest.ID ||
		recovered.PathSlug != created.Ticket.Manifest.PathSlug {
		t.Fatalf("recovered manifest = %#v", recovered)
	}
	inventory, inventoryErr := ReadScopeManifests(scope)
	if inventoryErr != nil {
		t.Fatalf("scan scope with staged recovery: %v", inventoryErr)
	}
	if len(inventory) != 0 {
		t.Fatalf("staged recovery leaked into inventory: %#v", inventory)
	}
}

func TestManifestMutationRetryRepairsProjectionAndCommitsJournal(t *testing.T) {
	t.Parallel()

	t.Run("update", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Interrupted update projection",
			Type:      "feat",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		service.beforeProjection = func(context.Context, Layout, Manifest) error {
			return errors.New("injected update projection interruption")
		}
		status := StatusInProgress
		request := UpdateRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Status:    &status,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		}
		failed, err := service.Update(context.Background(), request)
		if err == nil || failed.Outcome != capability.OutcomeFailed {
			t.Fatalf("interrupted update = %#v, %v", failed, err)
		}
		service.beforeProjection = nil
		resumed, resumeErr := service.Update(context.Background(), request)
		if resumeErr != nil || resumed.Outcome != capability.OutcomeApplied {
			t.Fatalf("resumed update = %#v, %v", resumed, resumeErr)
		}
		assertTicketProjectionState(
			t,
			workspaceLayout,
			resumed.Data.Ticket.Manifest,
			string(StatusInProgress),
			controlplane.TicketArchiveStateActive,
		)
		assertJournalCommitted(t, scope, failed.Data.OperationID)
	})

	t.Run("close", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{
			CompletionGate: CompletionGateFunc(
				func(context.Context, TicketData) ([]Finding, error) {
					return []Finding{}, nil
				},
			),
		})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Interrupted close projection",
			Type:      "feat",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		status := StatusInProgress
		if _, err := service.Update(context.Background(), UpdateRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Status:    &status,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		}); err != nil {
			t.Fatalf("start ticket: %v", err)
		}
		service.beforeProjection = func(context.Context, Layout, Manifest) error {
			return errors.New("injected close projection interruption")
		}
		request := CloseRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		}
		failed, err := service.Close(context.Background(), request)
		if err == nil || failed.Outcome != capability.OutcomeFailed {
			t.Fatalf("interrupted close = %#v, %v", failed, err)
		}
		service.beforeProjection = nil
		resumed, resumeErr := service.Close(context.Background(), request)
		if resumeErr != nil || resumed.Outcome != capability.OutcomeApplied {
			t.Fatalf("resumed close = %#v, %v", resumed, resumeErr)
		}
		assertTicketProjectionState(
			t,
			workspaceLayout,
			resumed.Data.Ticket.Manifest,
			string(StatusDone),
			controlplane.TicketArchiveStateActive,
		)
		assertJournalCommitted(t, scope, failed.Data.OperationID)
	})

	t.Run("archive", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Interrupted archive projection",
			Type:      "chore",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		service.beforeProjection = func(context.Context, Layout, Manifest) error {
			return errors.New("injected archive projection interruption")
		}
		request := ArchiveRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		}
		failed, err := service.Archive(context.Background(), request)
		if err == nil || failed.Outcome != capability.OutcomeFailed {
			t.Fatalf("interrupted archive = %#v, %v", failed, err)
		}
		service.beforeProjection = nil
		resumed, resumeErr := service.Archive(context.Background(), request)
		if resumeErr != nil || resumed.Outcome != capability.OutcomeApplied {
			t.Fatalf("resumed archive = %#v, %v", resumed, resumeErr)
		}
		assertTicketProjectionState(
			t,
			workspaceLayout,
			resumed.Data.Ticket.Manifest,
			string(StatusBacklog),
			controlplane.TicketArchiveStateArchived,
		)
		assertJournalCommitted(t, scope, failed.Data.OperationID)
	})

	t.Run("locked reinspection", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		primary := testLifecycleService(t, Options{})
		created := applyCreate(t, primary, CreateRequest{
			Scope:     scope,
			Title:     "Concurrent projection recovery",
			Type:      "feat",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		status := StatusInProgress
		request := UpdateRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Status:    &status,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		}
		concurrent := testLifecycleService(t, Options{
			BeforeProjection: func(context.Context, Layout, Manifest) error {
				return errors.New("injected concurrent projection interruption")
			},
		})
		var injected atomic.Bool
		primary.beforeMutationLock = func() error {
			if !injected.CompareAndSwap(false, true) {
				return nil
			}
			failed, err := concurrent.Update(context.Background(), request)
			if err == nil || failed.Outcome != capability.OutcomeFailed {
				return fmt.Errorf(
					"concurrent interrupted update = %#v, %v",
					failed,
					err,
				)
			}
			return nil
		}
		recovered, err := primary.Update(context.Background(), request)
		if err != nil || recovered.Outcome != capability.OutcomeApplied {
			t.Fatalf("locked reinspection recovery = %#v, %v", recovered, err)
		}
		if !injected.Load() {
			t.Fatal("concurrent mutation was not injected")
		}
		assertTicketProjectionState(
			t,
			workspaceLayout,
			recovered.Data.Ticket.Manifest,
			string(StatusInProgress),
			controlplane.TicketArchiveStateActive,
		)
	})
}

func TestManifestProjectionRecoveryPlanMustBelongToTicket(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Recovery plan ownership",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	current, err := findTicket(scope, created.Ticket.Manifest.ID)
	if err != nil {
		t.Fatalf("find ticket: %v", err)
	}
	plan := journal.Plan{
		Kind:           UpdateDescriptor.Capability,
		IdempotencyKey: UpdateDescriptor.Capability + ":" + current.Manifest.ID,
		Steps: []journal.Step{{
			Ordinal: 1,
			Action:  "replace",
			Target:  current.Layout.StatusPath(),
		}},
	}
	if !validManifestRecoveryPlan(plan, UpdateDescriptor, current) {
		t.Fatal("valid recovery plan was rejected")
	}
	plan.IdempotencyKey = UpdateDescriptor.Capability + ":another-ticket"
	if validManifestRecoveryPlan(plan, UpdateDescriptor, current) {
		t.Fatal("recovery accepted another ticket's journal plan")
	}
	plan.IdempotencyKey = UpdateDescriptor.Capability + ":" + current.Manifest.ID
	plan.Steps[0].Target = filepath.Join(scope.TicketsRoot(), "other", "status.yaml")
	if validManifestRecoveryPlan(plan, UpdateDescriptor, current) {
		t.Fatal("recovery accepted a plan without the ticket status target")
	}
}

func TestCloseArchiveAndRestoreUseExplicitGates(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	blockedService := testLifecycleService(t, Options{
		CompletionGate: CompletionGateFunc(
			func(
				context.Context,
				TicketData,
			) ([]Finding, error) {
				return []Finding{{
					Code:     "ticket.completion.tests_missing",
					Severity: "error",
					Summary:  "Tests are not complete.",
					NextAction: capability.Action{
						Code:    "run_tests",
						Message: "Run the configured test suite.",
					},
				}}, nil
			},
		),
	})
	created := applyCreate(t, blockedService, CreateRequest{
		Scope:     scope,
		Title:     "Completion-gated ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	inProgress := StatusInProgress
	started, err := blockedService.Update(
		context.Background(),
		UpdateRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Status:    &inProgress,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("start ticket: %v", err)
	}

	blocked, err := blockedService.Close(
		context.Background(),
		CloseRequest{
			Scope:     scope,
			Selector:  started.Data.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("blocked close: %v", err)
	}
	if blocked.Outcome != capability.OutcomeConflict ||
		!hasTicketFinding(
			blocked.Data.Findings,
			"ticket.completion.tests_missing",
		) ||
		len(blocked.NextActions) != 1 {
		t.Fatalf("blocked close result = %#v", blocked)
	}

	passService := testLifecycleService(t, Options{
		CompletionGate: CompletionGateFunc(
			func(context.Context, TicketData) ([]Finding, error) {
				return []Finding{}, nil
			},
		),
	})
	closed, err := passService.Close(
		context.Background(),
		CloseRequest{
			Scope:     scope,
			Selector:  started.Data.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("close ticket: %v", err)
	}
	if closed.Data.Ticket.Manifest.Status != StatusDone ||
		closed.Data.Ticket.Manifest.ClosedAt == nil {
		t.Fatalf("closed ticket = %#v", closed.Data.Ticket.Manifest)
	}

	archived, err := passService.Archive(
		context.Background(),
		ArchiveRequest{
			Scope:     scope,
			Selector:  closed.Data.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("archive ticket: %v", err)
	}
	if archived.Data.Ticket.Manifest.ArchiveState != ArchiveStateArchived {
		t.Fatalf("archived ticket = %#v", archived.Data.Ticket.Manifest)
	}
	if _, err := os.Stat(archived.Data.Ticket.Path); err != nil {
		t.Fatalf("archive removed ticket artifacts: %v", err)
	}
	restored, err := passService.Archive(
		context.Background(),
		ArchiveRequest{
			Scope:     scope,
			Selector:  archived.Data.Ticket.Manifest.ID,
			Profile:   activeProfile,
			Restore:   true,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		},
	)
	if err != nil {
		t.Fatalf("restore ticket: %v", err)
	}
	if restored.Data.Ticket.Manifest.ArchiveState != ArchiveStateActive {
		t.Fatalf("restored ticket = %#v", restored.Data.Ticket.Manifest)
	}
}

func TestRemoveIsExactDestructiveAndRefusesUnresolvedAuthoredChanges(
	t *testing.T,
) {
	t.Parallel()

	t.Run("clean archived ticket", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Disposable ticket",
			Type:      "chore",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(
			context.Background(),
			ArchiveRequest{
				Scope:     scope,
				Selector:  created.Ticket.Manifest.ID,
				Profile:   activeProfile,
				ActorType: "human",
				Tool:      "adb-cli",
				Apply:     true,
			},
		)
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		request := RemoveRequest{
			Scope:      scope,
			Selector:   archived.Data.Ticket.Manifest.VisibleKey,
			ExpectedID: archived.Data.Ticket.Manifest.ID,
			Profile:    activeProfile,
			ActorType:  "human",
			Tool:       "adb-cli",
		}
		planned, err := service.Remove(context.Background(), request)
		if err != nil {
			t.Fatalf("preview remove: %v", err)
		}
		if planned.Outcome != capability.OutcomePlanned ||
			len(planned.Effects) < 2 {
			t.Fatalf("remove preview = %#v", planned)
		}
		if _, err := os.Stat(archived.Data.Ticket.Path); err != nil {
			t.Fatalf("remove preview mutated source: %v", err)
		}
		request.Apply = true
		removed, err := service.Remove(context.Background(), request)
		if err != nil {
			t.Fatalf("apply remove: %v", err)
		}
		if removed.Outcome != capability.OutcomeApplied {
			t.Fatalf("remove outcome = %q", removed.Outcome)
		}
		if _, err := os.Stat(archived.Data.Ticket.Path); !errors.Is(
			err,
			os.ErrNotExist,
		) {
			t.Fatalf("removed ticket path still exists: %v", err)
		}
		state, err := controlplane.OpenReadOnly(
			context.Background(),
			workspaceLayout.StatePath(),
		)
		if err != nil {
			t.Fatalf("open state: %v", err)
		}
		_, err = state.Ticket(
			context.Background(),
			controlplane.TicketScope{
				OrganizationID: archived.Data.Ticket.Manifest.OrganizationID,
			},
			archived.Data.Ticket.Manifest.ID,
		)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("removed projection remains: %v", err)
		}
		if closeErr := state.Close(); closeErr != nil {
			t.Fatalf("close state: %v", closeErr)
		}
		repeated, repeatErr := service.Remove(context.Background(), request)
		if repeatErr != nil {
			t.Fatalf("repeat remove: %v", repeatErr)
		}
		if repeated.Outcome != capability.OutcomeUnchanged {
			t.Fatalf("repeated remove = %#v", repeated)
		}
	})

	t.Run("authored drift", func(t *testing.T) {
		scope, _ := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Authored ticket",
			Type:      "docs",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		contextPath := filepath.Join(created.Ticket.Path, "context.md")
		if err := os.WriteFile(
			contextPath,
			[]byte("authored change\n"),
			0o644,
		); err != nil {
			t.Fatalf("edit context: %v", err)
		}
		archived, err := service.Archive(
			context.Background(),
			ArchiveRequest{
				Scope:     scope,
				Selector:  created.Ticket.Manifest.ID,
				Profile:   activeProfile,
				ActorType: "human",
				Tool:      "adb-cli",
				Apply:     true,
			},
		)
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		refused, err := service.Remove(
			context.Background(),
			RemoveRequest{
				Scope:      scope,
				Selector:   archived.Data.Ticket.Manifest.ID,
				ExpectedID: archived.Data.Ticket.Manifest.ID,
				Profile:    activeProfile,
				ActorType:  "human",
				Tool:       "adb-cli",
				Apply:      true,
			},
		)
		if err != nil {
			t.Fatalf("inspect authored drift: %v", err)
		}
		if refused.Outcome != capability.OutcomeConflict ||
			!hasTicketFinding(
				refused.Data.Findings,
				"ticket.remove.authored_changes",
			) {
			t.Fatalf("authored remove refusal = %#v", refused)
		}
		if _, err := os.Stat(created.Ticket.Path); err != nil {
			t.Fatalf("refused remove changed source: %v", err)
		}
	})

	t.Run("registered worktree", func(t *testing.T) {
		scope, repositoryLayout, repositoryManifest := lifecycleRepositoryScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Worktree-owned ticket",
			Type:      "feat",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(
			context.Background(),
			ArchiveRequest{
				Scope:     scope,
				Selector:  created.Ticket.Manifest.ID,
				Profile:   activeProfile,
				ActorType: "human",
				Tool:      "adb-cli",
				Apply:     true,
			},
		)
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		repositoryManifest.Worktrees = append(
			repositoryManifest.Worktrees,
			repository.WorktreeRegistration{
				TicketKey: created.Ticket.Manifest.LocalKey,
				Name:      "default",
				Path: filepath.Join(
					repositoryManifest.Roles.Worktrees,
					created.Ticket.Manifest.LocalKey,
				),
				Branch: created.Ticket.Manifest.Branch.Intent,
				Active: true,
			},
		)
		if err := repositoryManifest.Validate(repositoryLayout); err != nil {
			t.Fatalf("validate repository worktree registration: %v", err)
		}
		var encoded bytes.Buffer
		if err := repository.EncodeManifest(
			&encoded,
			repositoryManifest,
		); err != nil {
			t.Fatalf("encode repository manifest: %v", err)
		}
		if err := os.WriteFile(
			repositoryLayout.ManifestPath(),
			encoded.Bytes(),
			0o644,
		); err != nil {
			t.Fatalf("write repository manifest: %v", err)
		}
		refused, err := service.Remove(
			context.Background(),
			RemoveRequest{
				Scope:      scope,
				Selector:   archived.Data.Ticket.Manifest.ID,
				ExpectedID: archived.Data.Ticket.Manifest.ID,
				Profile:    activeProfile,
				ActorType:  "human",
				Tool:       "adb-cli",
				Apply:      true,
			},
		)
		if err != nil {
			t.Fatalf("inspect registered worktree: %v", err)
		}
		if refused.Outcome != capability.OutcomeConflict ||
			!hasTicketFinding(
				refused.Data.Findings,
				"ticket.remove.worktree_registered",
			) {
			t.Fatalf("registered worktree refusal = %#v", refused)
		}
	})

	t.Run("unregistered live worktree", func(t *testing.T) {
		scope, repositoryLayout, repositoryManifest := lifecycleRepositoryScope(t)
		activeProfile := testBuiltinProfile(t)
		git := &lifecycleFakeGit{}
		service := testLifecycleService(t, Options{Git: git})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Live worktree ticket",
			Type:      "feat",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(
			context.Background(),
			ArchiveRequest{
				Scope:     scope,
				Selector:  created.Ticket.Manifest.ID,
				Profile:   activeProfile,
				ActorType: "human",
				Tool:      "adb-cli",
				Apply:     true,
			},
		)
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		clonePath := repositoryManifest.CanonicalClone.Path
		if !repositoryManifest.CanonicalClone.External {
			clonePath = filepath.Join(repositoryLayout.Root(), clonePath)
		}
		git.worktrees = []repository.GitWorktree{
			{Path: clonePath, Branch: "main"},
			{
				Path:   filepath.Join(repositoryLayout.Root(), "unregistered-live"),
				Branch: created.Ticket.Manifest.Branch.Intent,
			},
		}
		refused, err := service.Remove(
			context.Background(),
			RemoveRequest{
				Scope:      scope,
				Selector:   archived.Data.Ticket.Manifest.ID,
				ExpectedID: archived.Data.Ticket.Manifest.ID,
				Profile:    activeProfile,
				ActorType:  "human",
				Tool:       "adb-cli",
				Apply:      true,
			},
		)
		if err != nil {
			t.Fatalf("inspect live worktree: %v", err)
		}
		if refused.Outcome != capability.OutcomeConflict ||
			!hasTicketFinding(
				refused.Data.Findings,
				"ticket.remove.worktree_live",
			) {
			t.Fatalf("live worktree refusal = %#v", refused)
		}
	})
}

func TestRemovePreservesRecoveryAcrossProjectionInterruptions(t *testing.T) {
	t.Parallel()

	t.Run("before projection restores portable source", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Restore before projection",
			Type:      "chore",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(context.Background(), ArchiveRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		})
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		service.beforeProjection = func(context.Context, Layout, Manifest) error {
			return errors.New("injected pre-projection interruption")
		}
		result, err := service.Remove(context.Background(), RemoveRequest{
			Scope:      scope,
			Selector:   archived.Data.Ticket.Manifest.ID,
			ExpectedID: archived.Data.Ticket.Manifest.ID,
			Profile:    activeProfile,
			ActorType:  "human",
			Tool:       "adb-cli",
			Apply:      true,
		})
		if err == nil || result.Outcome != capability.OutcomeFailed {
			t.Fatalf("pre-projection interruption = %#v, %v", result, err)
		}
		if _, statErr := os.Stat(archived.Data.Ticket.Path); statErr != nil {
			t.Fatalf("portable source was not restored: %v", statErr)
		}
		assertTicketProjectionExists(
			t,
			workspaceLayout,
			archived.Data.Ticket.Manifest,
		)
	})

	t.Run("staged interruption exposes trash while projection remains", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Staged remove recovery",
			Type:      "chore",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(context.Background(), ArchiveRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		})
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		service.afterFilesystemStage = func(string) error {
			return errors.New("injected remove staging interruption")
		}
		request := RemoveRequest{
			Scope:      scope,
			Selector:   archived.Data.Ticket.Manifest.ID,
			ExpectedID: archived.Data.Ticket.Manifest.ID,
			Profile:    activeProfile,
			ActorType:  "human",
			Tool:       "adb-cli",
			Apply:      true,
		}
		failed, err := service.Remove(context.Background(), request)
		if err == nil ||
			failed.Outcome != capability.OutcomeFailed ||
			failed.Data.RecoveryPath == "" {
			t.Fatalf("staged remove interruption = %#v, %v", failed, err)
		}
		assertTicketProjectionExists(
			t,
			workspaceLayout,
			archived.Data.Ticket.Manifest,
		)
		service.afterFilesystemStage = nil
		repeated, repeatErr := service.Remove(context.Background(), request)
		if repeatErr != nil {
			t.Fatalf("repeat staged remove: %v", repeatErr)
		}
		if repeated.Outcome != capability.OutcomeAttention ||
			repeated.Data.RecoveryPath != failed.Data.RecoveryPath ||
			!strings.Contains(
				repeated.Warnings[0].Message,
				"projection is still present",
			) {
			t.Fatalf("repeated staged remove = %#v", repeated)
		}
	})

	t.Run("after projection keeps recovery trash", func(t *testing.T) {
		scope, workspaceLayout := lifecycleOrganizationScope(t)
		activeProfile := testBuiltinProfile(t)
		service := testLifecycleService(t, Options{})
		created := applyCreate(t, service, CreateRequest{
			Scope:     scope,
			Title:     "Recover after projection",
			Type:      "chore",
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
		})
		archived, err := service.Archive(context.Background(), ArchiveRequest{
			Scope:     scope,
			Selector:  created.Ticket.Manifest.ID,
			Profile:   activeProfile,
			ActorType: "human",
			Tool:      "adb-cli",
			Apply:     true,
		})
		if err != nil {
			t.Fatalf("archive ticket: %v", err)
		}
		service.afterProjection = func(context.Context, Layout, Manifest) error {
			return errors.New("injected post-projection interruption")
		}
		result, err := service.Remove(context.Background(), RemoveRequest{
			Scope:      scope,
			Selector:   archived.Data.Ticket.Manifest.ID,
			ExpectedID: archived.Data.Ticket.Manifest.ID,
			Profile:    activeProfile,
			ActorType:  "human",
			Tool:       "adb-cli",
			Apply:      true,
		})
		if err == nil ||
			result.Outcome != capability.OutcomeFailed ||
			result.Data.RecoveryPath == "" {
			t.Fatalf("post-projection interruption = %#v, %v", result, err)
		}
		if _, statErr := os.Stat(archived.Data.Ticket.Path); !errors.Is(
			statErr,
			os.ErrNotExist,
		) {
			t.Fatalf("portable source remains after projection removal: %v", statErr)
		}
		if _, statErr := os.Stat(result.Data.RecoveryPath); statErr != nil {
			t.Fatalf("recovery trash missing: %v", statErr)
		}
		assertTicketProjectionMissing(
			t,
			workspaceLayout,
			archived.Data.Ticket.Manifest,
		)
		service.afterProjection = nil
		repeated, repeatErr := service.Remove(
			context.Background(),
			RemoveRequest{
				Scope:      scope,
				Selector:   archived.Data.Ticket.Manifest.ID,
				ExpectedID: archived.Data.Ticket.Manifest.ID,
				Profile:    activeProfile,
				ActorType:  "human",
				Tool:       "adb-cli",
				Apply:      true,
			},
		)
		if repeatErr != nil {
			t.Fatalf("repeat interrupted remove: %v", repeatErr)
		}
		if repeated.Outcome != capability.OutcomeAttention ||
			repeated.Data.RecoveryPath != result.Data.RecoveryPath ||
			!repeated.Recovery.Required {
			t.Fatalf("repeated interrupted remove = %#v", repeated)
		}
	})
}

func TestLifecycleMutationsAreIdempotent(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{
		CompletionGate: CompletionGateFunc(
			func(context.Context, TicketData) ([]Finding, error) {
				return []Finding{}, nil
			},
		),
	})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Idempotent ticket",
		Type:      "test",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	inProgress := StatusInProgress
	update := UpdateRequest{
		Scope:     scope,
		Selector:  created.Ticket.Manifest.ID,
		Profile:   activeProfile,
		Status:    &inProgress,
		ActorType: "human",
		Tool:      "adb-cli",
		Apply:     true,
	}
	if result, err := service.Update(
		context.Background(),
		update,
	); err != nil || result.Outcome != capability.OutcomeApplied {
		t.Fatalf("first update = %#v, %v", result, err)
	}
	if result, err := service.Update(
		context.Background(),
		update,
	); err != nil || result.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeated update = %#v, %v", result, err)
	}
	closeRequest := CloseRequest{
		Scope:     scope,
		Selector:  created.Ticket.Manifest.ID,
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
		Apply:     true,
	}
	if result, err := service.Close(
		context.Background(),
		closeRequest,
	); err != nil || result.Outcome != capability.OutcomeApplied {
		t.Fatalf("first close = %#v, %v", result, err)
	}
	if result, err := service.Close(
		context.Background(),
		closeRequest,
	); err != nil || result.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeated close = %#v, %v", result, err)
	}
	archiveRequest := ArchiveRequest{
		Scope:     scope,
		Selector:  created.Ticket.Manifest.ID,
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
		Apply:     true,
	}
	if result, err := service.Archive(
		context.Background(),
		archiveRequest,
	); err != nil || result.Outcome != capability.OutcomeApplied {
		t.Fatalf("first archive = %#v, %v", result, err)
	}
	if result, err := service.Archive(
		context.Background(),
		archiveRequest,
	); err != nil || result.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeated archive = %#v, %v", result, err)
	}
}

func TestCreateInterruptionReturnsInspectableRecovery(t *testing.T) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	operationID := "550e8400-e29b-41d4-a716-999999999999"
	service := testLifecycleService(t, Options{
		BeforeProjection: func(
			context.Context,
			Layout,
			Manifest,
		) error {
			return errors.New("injected projection interruption")
		},
	})
	request := CreateRequest{
		Scope:       scope,
		Title:       "Interrupted ticket",
		Type:        "fix",
		Profile:     activeProfile,
		ActorType:   "agent",
		Tool:        "adb-cli",
		OperationID: operationID,
		Apply:       true,
	}
	result, err := service.Create(context.Background(), request)
	if err == nil {
		t.Fatal("injected interruption returned no error")
	}
	if result.Outcome != capability.OutcomeFailed ||
		!result.Recovery.Required ||
		len(result.Recovery.Guidance) == 0 {
		t.Fatalf("interruption result = %#v", result)
	}
	if _, statErr := os.Stat(result.Data.Ticket.Layout.StatusPath()); statErr != nil {
		t.Fatalf("portable source missing after interruption: %v", statErr)
	}
	state, openErr := controlplane.OpenReadOnly(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if openErr != nil {
		t.Fatalf("open state: %v", openErr)
	}
	_, projectionErr := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: result.Data.Ticket.Manifest.OrganizationID,
		},
		result.Data.Ticket.Manifest.ID,
	)
	if !errors.Is(projectionErr, sql.ErrNoRows) {
		t.Fatalf("interrupted create projected ticket: %v", projectionErr)
	}
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("close state: %v", closeErr)
	}

	workspaceManifest, readErr := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if readErr != nil {
		t.Fatalf("read workspace manifest: %v", readErr)
	}
	eventsRoot, resolveErr := workspaceLayout.ResolveRole(
		workspaceManifest.Roles.Events,
	)
	if resolveErr != nil {
		t.Fatalf("resolve events root: %v", resolveErr)
	}
	store, storeErr := journal.NewStore(
		eventsRoot,
		time.Now,
		func() string { return "unused" },
	)
	if storeErr != nil {
		t.Fatalf("open journal store: %v", storeErr)
	}
	operation, inspectErr := store.Inspect(result.Data.OperationID)
	if inspectErr != nil {
		t.Fatalf("inspect interrupted operation: %v", inspectErr)
	}
	if operation.Status != journal.StatusFailed {
		t.Fatalf("interrupted operation = %#v", operation)
	}

	resumer := testLifecycleService(t, Options{})
	resumed, resumeErr := resumer.Create(context.Background(), request)
	if resumeErr != nil {
		t.Fatalf("resume interrupted create: %v", resumeErr)
	}
	if resumed.Outcome != capability.OutcomeApplied ||
		resumed.Data.Ticket.Manifest.ID != result.Data.Ticket.Manifest.ID ||
		resumed.Data.Ticket.Path != result.Data.Ticket.Path {
		t.Fatalf("resumed create = %#v", resumed)
	}
	assertTicketProjectionExists(
		t,
		workspaceLayout,
		resumed.Data.Ticket.Manifest,
	)
	inventory, inventoryErr := ReadScopeManifests(scope)
	if inventoryErr != nil {
		t.Fatalf("scan resumed create: %v", inventoryErr)
	}
	if len(inventory) != 1 {
		t.Fatalf("resumed create inventory = %#v", inventory)
	}
	repeated, repeatErr := resumer.Create(context.Background(), request)
	if repeatErr != nil {
		t.Fatalf("repeat resumed create: %v", repeatErr)
	}
	if repeated.Outcome != capability.OutcomeUnchanged ||
		repeated.Data.Ticket.Manifest.ID != result.Data.Ticket.Manifest.ID {
		t.Fatalf("repeated resumed create = %#v", repeated)
	}
}

func TestCreateRetryCommitsJournalAfterProjectionInterruption(t *testing.T) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	operationID := "550e8400-e29b-41d4-a716-888888888888"
	service := testLifecycleService(t, Options{
		AfterProjection: func(context.Context, Layout, Manifest) error {
			return errors.New("injected interruption after projection")
		},
	})
	request := CreateRequest{
		Scope:       scope,
		Title:       "Projected interrupted ticket",
		Type:        "fix",
		Profile:     activeProfile,
		ActorType:   "agent",
		Tool:        "adb-cli",
		OperationID: operationID,
		Apply:       true,
	}
	failed, err := service.Create(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("post-projection interruption = %#v, %v", failed, err)
	}
	assertTicketProjectionExists(t, workspaceLayout, failed.Data.Ticket.Manifest)

	resumer := testLifecycleService(t, Options{})
	resumed, resumeErr := resumer.Create(context.Background(), request)
	if resumeErr != nil {
		t.Fatalf("resume projected create: %v", resumeErr)
	}
	if resumed.Outcome != capability.OutcomeApplied ||
		resumed.Data.Ticket.Manifest.ID != failed.Data.Ticket.Manifest.ID {
		t.Fatalf("projected create resume = %#v", resumed)
	}
	resources, resourcesErr := loadWorkspaceResources(scope.WorkspaceRoot())
	if resourcesErr != nil {
		t.Fatalf("load workspace resources: %v", resourcesErr)
	}
	store, storeErr := journal.NewStore(
		resources.eventsDir,
		time.Now,
		func() string { return "unused" },
	)
	if storeErr != nil {
		t.Fatalf("open journal store: %v", storeErr)
	}
	operation, inspectErr := store.Inspect(operationID)
	if inspectErr != nil {
		t.Fatalf("inspect resumed operation: %v", inspectErr)
	}
	if operation.Status != journal.StatusCommitted {
		t.Fatalf("resumed operation = %#v", operation)
	}
}

func TestCreateRetryResumesBeforeSourcePublication(t *testing.T) {
	t.Parallel()

	scope, workspaceLayout := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	activeProfile.ID = "custom-nested-status"
	activeProfile.Artifacts = append(
		append([]profile.Artifact(nil), activeProfile.Artifacts...),
		profile.Artifact{
			Role:      "ticket.review_status",
			Path:      "review/status.yaml",
			Kind:      profile.ArtifactFile,
			Authority: profile.AuthorityGenerated,
			Required:  true,
			Search:    profile.SearchNone,
			Retention: profile.RetentionDurable,
			Git:       profile.GitTracked,
			Template: &profile.TemplateReference{
				ID:      "review-status",
				Version: "v1",
			},
		},
	)
	templates := append(
		profile.BuiltinTemplates(),
		profile.Template{
			ID:          "review-status",
			Version:     "v1",
			SourceScope: "workspace",
			Content:     []byte("state: pending\n"),
		},
	)
	catalog, catalogErr := profile.NewTemplateCatalog(templates)
	if catalogErr != nil {
		t.Fatalf("new template catalog: %v", catalogErr)
	}
	operationID := "550e8400-e29b-41d4-a716-777777777777"
	service := testLifecycleService(t, Options{
		TemplateResolver: catalog,
		AfterPlan: func() error {
			return errors.New("injected interruption before source publication")
		},
	})
	request := CreateRequest{
		Scope:       scope,
		Title:       "Pre-publication interrupted ticket",
		Type:        "feat",
		Profile:     activeProfile,
		ActorType:   "agent",
		Tool:        "adb-cli",
		OperationID: operationID,
		Apply:       true,
	}
	failed, err := service.Create(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("pre-publication interruption = %#v, %v", failed, err)
	}
	if _, statErr := os.Stat(failed.Data.Ticket.Path); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("source published before injected interruption: %v", statErr)
	}

	resumer := testLifecycleService(t, Options{TemplateResolver: catalog})
	resumed, resumeErr := resumer.Create(context.Background(), request)
	if resumeErr != nil {
		t.Fatalf("resume pre-publication create: %v", resumeErr)
	}
	if resumed.Outcome != capability.OutcomeApplied ||
		resumed.Data.Ticket.Manifest.ID != failed.Data.Ticket.Manifest.ID ||
		resumed.Data.Ticket.Path != failed.Data.Ticket.Path {
		t.Fatalf("pre-publication create resume = %#v", resumed)
	}
	assertTicketProjectionExists(t, workspaceLayout, resumed.Data.Ticket.Manifest)
}

func lifecycleOrganizationScope(
	t *testing.T,
) (ScopeLayout, workspace.Layout) {
	t.Helper()

	organizationLayout, organizationManifest := testOrganization(t)
	scope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization scope: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(scope.WorkspaceRoot())
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	content := readFile(t, organizationLayout.ManifestPath())
	state, err := controlplane.Open(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	if err := state.ObserveOrganization(
		context.Background(),
		controlplane.OrganizationProjection{
			ID:           organizationManifest.ID,
			Slug:         organizationManifest.Slug,
			Path:         organizationLayout.Root(),
			DisplayName:  organizationManifest.DisplayName,
			ParentID:     organizationManifest.ParentID,
			ManifestHash: journal.Digest(content),
			Status:       controlplane.EntityStatusActive,
			Aliases:      []string{},
			ObservedAt:   organizationManifest.UpdatedAt,
		},
	); err != nil {
		_ = state.Close()
		t.Fatalf("project organization: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("close control plane: %v", err)
	}
	return scope, workspaceLayout
}

func lifecycleRepositoryScope(
	t *testing.T,
) (ScopeLayout, repository.Layout, repository.Manifest) {
	t.Helper()

	organizationLayout, organizationManifest := testOrganization(t)
	repositoryLayout, repositoryManifest := testRepository(
		t,
		organizationLayout,
		organizationManifest,
	)
	scope, err := NewRepositoryScope(
		organizationLayout,
		organizationManifest,
		repositoryLayout,
		repositoryManifest,
	)
	if err != nil {
		t.Fatalf("new repository scope: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(scope.WorkspaceRoot())
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
	if err := state.ObserveOrganization(
		context.Background(),
		controlplane.OrganizationProjection{
			ID:          organizationManifest.ID,
			Slug:        organizationManifest.Slug,
			Path:        organizationLayout.Root(),
			DisplayName: organizationManifest.DisplayName,
			ManifestHash: journal.Digest(
				readFile(t, organizationLayout.ManifestPath()),
			),
			Status:     controlplane.EntityStatusActive,
			Aliases:    []string{},
			ObservedAt: organizationManifest.UpdatedAt,
		},
	); err != nil {
		_ = state.Close()
		t.Fatalf("project organization: %v", err)
	}
	if err := state.ObserveRepository(
		context.Background(),
		controlplane.RepositoryProjection{
			ID:              repositoryManifest.ID,
			OrganizationID:  organizationManifest.ID,
			Host:            repositoryManifest.Host,
			Owner:           repositoryManifest.Owner,
			Name:            repositoryManifest.Name,
			Path:            repositoryLayout.Root(),
			ManifestHash:    journal.Digest(readFile(t, repositoryLayout.ManifestPath())),
			CanonicalRemote: repositoryManifest.CanonicalRemote.FetchURL,
			Status:          controlplane.EntityStatusActive,
			Aliases:         []string{},
			ObservedAt:      repositoryManifest.UpdatedAt,
		},
	); err != nil {
		_ = state.Close()
		t.Fatalf("project repository: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("close control plane: %v", err)
	}
	return scope, repositoryLayout, repositoryManifest
}

func testLifecycleService(t *testing.T, options Options) *Service {
	t.Helper()

	var clockCounter atomic.Int64
	base := time.Date(2026, time.September, 10, 16, 0, 0, 0, time.UTC)
	options.Clock = func() time.Time {
		offset := clockCounter.Add(1)
		return base.Add(time.Duration(offset) * time.Second)
	}
	var idCounter atomic.Uint64
	options.IDGenerator = func() string {
		value := idCounter.Add(1)
		return fmt.Sprintf(
			"550e8400-e29b-41d4-a716-%012d",
			value,
		)
	}
	service, err := NewServiceWithOptions(options)
	if err != nil {
		t.Fatalf("new ticket service: %v", err)
	}
	return service
}

func applyCreate(
	t *testing.T,
	service *Service,
	request CreateRequest,
) MutationData {
	t.Helper()

	request.Apply = true
	result, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if result.Outcome != capability.OutcomeApplied {
		t.Fatalf("create outcome = %q: %#v", result.Outcome, result)
	}
	return result.Data
}

func readTicketManifest(t *testing.T, layout Layout) Manifest {
	t.Helper()

	file, err := os.Open(layout.StatusPath())
	if err != nil {
		t.Fatalf("open ticket manifest: %v", err)
	}
	defer file.Close()
	manifest, err := DecodeManifest(file)
	if err != nil {
		t.Fatalf("decode ticket manifest: %v", err)
	}
	return manifest
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return content
}

func assertTicketProjectionExists(
	t *testing.T,
	layout workspace.Layout,
	manifest Manifest,
) {
	t.Helper()

	state, err := controlplane.OpenReadOnly(
		context.Background(),
		layout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	_, projectionErr := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: manifest.OrganizationID,
			RepositoryID:   manifest.RepositoryID,
		},
		manifest.ID,
	)
	closeErr := state.Close()
	if projectionErr != nil {
		t.Fatalf("ticket projection missing: %v", projectionErr)
	}
	if closeErr != nil {
		t.Fatalf("close control plane: %v", closeErr)
	}
}

func assertTicketProjectionMissing(
	t *testing.T,
	layout workspace.Layout,
	manifest Manifest,
) {
	t.Helper()

	state, err := controlplane.OpenReadOnly(
		context.Background(),
		layout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	_, projectionErr := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: manifest.OrganizationID,
			RepositoryID:   manifest.RepositoryID,
		},
		manifest.ID,
	)
	closeErr := state.Close()
	if !errors.Is(projectionErr, sql.ErrNoRows) {
		t.Fatalf("ticket projection remains: %v", projectionErr)
	}
	if closeErr != nil {
		t.Fatalf("close control plane: %v", closeErr)
	}
}

func assertTicketProjectionState(
	t *testing.T,
	layout workspace.Layout,
	manifest Manifest,
	status string,
	archiveState controlplane.TicketArchiveState,
) {
	t.Helper()

	state, err := controlplane.OpenReadOnly(
		context.Background(),
		layout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	projection, projectionErr := state.Ticket(
		context.Background(),
		controlplane.TicketScope{
			OrganizationID: manifest.OrganizationID,
			RepositoryID:   manifest.RepositoryID,
		},
		manifest.ID,
	)
	closeErr := state.Close()
	if projectionErr != nil {
		t.Fatalf("read ticket projection: %v", projectionErr)
	}
	if closeErr != nil {
		t.Fatalf("close control plane: %v", closeErr)
	}
	if projection.Status != status || projection.ArchiveState != archiveState {
		t.Fatalf("ticket projection state = %#v", projection)
	}
}

func assertJournalCommitted(
	t *testing.T,
	scope ScopeLayout,
	operationID string,
) {
	t.Helper()

	resources, err := loadWorkspaceResources(scope.WorkspaceRoot())
	if err != nil {
		t.Fatalf("load workspace resources: %v", err)
	}
	store, err := journal.NewStore(
		resources.eventsDir,
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	operation, err := store.Inspect(operationID)
	if err != nil {
		t.Fatalf("inspect operation: %v", err)
	}
	if operation.Status != journal.StatusCommitted {
		t.Fatalf("operation = %#v", operation)
	}
}

func checkpointByRole(
	t *testing.T,
	checkpoints []AppendOnlyCheckpoint,
	role string,
) AppendOnlyCheckpoint {
	t.Helper()

	for _, checkpoint := range checkpoints {
		if checkpoint.Role == role {
			return checkpoint
		}
	}
	t.Fatalf("checkpoint %q not found in %#v", role, checkpoints)
	return AppendOnlyCheckpoint{}
}

type lifecycleFakeGit struct {
	worktrees []repository.GitWorktree
}

func (git *lifecycleFakeGit) Inventory(
	context.Context,
	string,
	string,
) (repository.Inventory, error) {
	return repository.Inventory{}, nil
}

func (git *lifecycleFakeGit) Clone(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func (git *lifecycleFakeGit) Init(context.Context, string) error {
	return nil
}

func (git *lifecycleFakeGit) AddRemote(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func (git *lifecycleFakeGit) SetRemoteURL(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func (git *lifecycleFakeGit) Fetch(context.Context, string, string) error {
	return nil
}

func (git *lifecycleFakeGit) FastForward(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func (git *lifecycleFakeGit) Worktrees(
	context.Context,
	string,
) ([]repository.GitWorktree, error) {
	return append([]repository.GitWorktree(nil), git.worktrees...), nil
}

func (git *lifecycleFakeGit) AddWorktree(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func (git *lifecycleFakeGit) RemoveWorktree(
	context.Context,
	string,
	string,
) error {
	return nil
}

func hasTicketFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func TestLifecycleTestHelpersRemainDeterministic(t *testing.T) {
	t.Parallel()

	service := testLifecycleService(t, Options{})
	left := service.newID()
	right := service.newID()
	if left == right || !reflect.DeepEqual(
		[]string{left, right},
		[]string{
			"550e8400-e29b-41d4-a716-000000000001",
			"550e8400-e29b-41d4-a716-000000000002",
		},
	) {
		t.Fatalf("deterministic IDs = %q, %q", left, right)
	}
	if !service.now().Before(service.now()) {
		t.Fatal("deterministic clock did not advance")
	}
}

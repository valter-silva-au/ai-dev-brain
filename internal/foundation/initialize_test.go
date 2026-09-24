package foundation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestInitializeDryRunPlansWithoutWriting(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)

	result, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: false,
	})
	if err != nil {
		t.Fatalf("plan initialization: %v", err)
	}
	if result.Outcome != capability.OutcomePlanned {
		t.Fatalf("outcome = %q, want planned", result.Outcome)
	}
	if result.Data.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace id = %q, want workspace-1", result.Data.WorkspaceID)
	}
	if result.Data.Root != root {
		t.Fatalf("root = %q, want %q", result.Data.Root, root)
	}
	if len(result.Effects) == 0 {
		t.Fatal("planned effects are empty")
	}
	for _, effect := range result.Effects {
		if effect.Status != capability.EffectPlanned {
			t.Fatalf("effect %#v is not planned", effect)
		}
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote workspace root: %v", err)
	}
}

func TestInitializeApplyCreatesFoundationAndIsIdempotent(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)

	applied, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	})
	if err != nil {
		t.Fatalf("apply initialization: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace id = %q, want workspace-1", applied.Data.WorkspaceID)
	}
	if applied.Data.OperationID != "operation-1" {
		t.Fatalf("operation id = %q, want operation-1", applied.Data.OperationID)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	for _, path := range []string{
		layout.ManifestPath(),
		layout.ConfigPath(),
		layout.StatePath(),
		layout.EventsDir(),
		layout.CacheDir(),
		layout.OrganizationsDir(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected initialized path %q: %v", path, err)
		}
	}

	manifest, err := workspace.ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read initialized manifest: %v", err)
	}
	if manifest.ID != "workspace-1" {
		t.Fatalf("manifest id = %q, want workspace-1", manifest.ID)
	}

	stateStore, err := controlplane.Open(
		context.Background(),
		layout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open initialized control plane: %v", err)
	}
	projection, err := stateStore.Workspace(context.Background())
	if closeErr := stateStore.Close(); closeErr != nil {
		t.Fatalf("close initialized control plane: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("read workspace projection: %v", err)
	}
	if projection.ID != "workspace-1" || projection.Root != root {
		t.Fatalf("workspace projection = %#v", projection)
	}

	journalStore, err := journal.NewStore(
		layout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := journalStore.Inspect("operation-1")
	if err != nil {
		t.Fatalf("inspect initialize journal: %v", err)
	}
	if state.Status != journal.StatusCommitted {
		t.Fatalf("journal status = %q, want committed", state.Status)
	}

	repeated, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	})
	if err != nil {
		t.Fatalf("repeat initialization: %v", err)
	}
	if repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat outcome = %q, want unchanged", repeated.Outcome)
	}
	if repeated.Data.WorkspaceID != "workspace-1" {
		t.Fatalf(
			"repeat workspace id = %q, want workspace-1",
			repeated.Data.WorkspaceID,
		)
	}
	if repeated.Data.OperationID != "" {
		t.Fatalf("repeat operation id = %q, want empty", repeated.Data.OperationID)
	}
}

func TestInitializeReturnsConflictWithoutOverwritingManifest(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	before, err := os.ReadFile(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read manifest before conflict: %v", err)
	}

	result, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "Different",
		Apply: true,
	})
	if err != nil {
		t.Fatalf("conflicting initialization returned transport error: %v", err)
	}
	if result.Outcome != capability.OutcomeConflict {
		t.Fatalf("outcome = %q, want conflict", result.Outcome)
	}

	after, err := os.ReadFile(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read manifest after conflict: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("conflicting initialization overwrote manifest")
	}
}

func TestInitializeFailureAfterPlanIsDurablyRecoverable(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	injected := errors.New("injected after-plan failure")
	service := newTestService(t, func() error { return injected })

	result, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	})
	if !errors.Is(err, injected) {
		t.Fatalf("initialize error = %v, want injected failure", err)
	}
	if result.Outcome != capability.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", result.Outcome)
	}
	if !result.Recovery.Required {
		t.Fatal("failed initialization did not require recovery")
	}
	if result.Data.OperationID != "operation-1" {
		t.Fatalf("operation id = %q, want operation-1", result.Data.OperationID)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	journalStore, err := journal.NewStore(
		layout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := journalStore.Inspect("operation-1")
	if err != nil {
		t.Fatalf("inspect failed initialization: %v", err)
	}
	if state.Status != journal.StatusApplying {
		t.Fatalf("journal status = %q, want applying", state.Status)
	}
	if _, err := os.Stat(layout.ManifestPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest written despite injected failure: %v", err)
	}
}

func newTestService(t *testing.T, afterPlan func() error) *Service {
	t.Helper()

	ids := []string{
		"workspace-1",
		"operation-1",
		"event-1",
		"event-2",
		"event-3",
		"event-4",
		"event-5",
		"event-6",
	}
	index := 0
	service, err := NewService(Options{
		Clock: func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				8,
				0,
				index,
				0,
				time.UTC,
			)
		},
		IDGenerator: func() string {
			id := ids[index]
			index++
			return id
		},
		AfterPlan: afterPlan,
	})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	return service
}

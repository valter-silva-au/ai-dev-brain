package journal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBeginWritesImmutablePlan(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	plan := Plan{
		OperationID:    "operation-1",
		IdempotencyKey: "workspace.initialize:workspace-1",
		Kind:           "workspace.initialize",
		Steps: []Step{{
			Ordinal: 1,
			Action:  "create",
			Target:  "/workspace/.aidb/manifest.yaml",
		}},
	}

	created, err := store.Begin(plan)
	if err != nil {
		t.Fatalf("begin journal plan: %v", err)
	}
	if created.SchemaVersion != SchemaVersion {
		t.Fatalf(
			"schema version = %q, want %q",
			created.SchemaVersion,
			SchemaVersion,
		)
	}
	if created.Hash == "" {
		t.Fatal("plan hash is empty")
	}
	wantCreatedAt := time.Date(
		2026,
		time.September,
		10,
		8,
		0,
		0,
		0,
		time.UTC,
	)
	if created.CreatedAt != wantCreatedAt {
		t.Fatalf(
			"created at = %v, want store clock %v",
			created.CreatedAt,
			wantCreatedAt,
		)
	}

	repeated, err := store.Begin(plan)
	if err != nil {
		t.Fatalf("repeat identical journal plan: %v", err)
	}
	if repeated.Hash != created.Hash {
		t.Fatalf("repeated plan hash = %q, want %q", repeated.Hash, created.Hash)
	}

	conflict := plan
	conflict.Steps = []Step{{
		Ordinal: 1,
		Action:  "replace",
		Target:  "/workspace/.aidb/manifest.yaml",
	}}
	if _, err := store.Begin(conflict); !errors.Is(err, ErrPlanConflict) {
		t.Fatalf("conflicting plan error = %v, want ErrPlanConflict", err)
	}
}

func TestBeginPreservesExplicitCreatedAtInUTC(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	explicit := time.Date(
		2026,
		time.September,
		9,
		23,
		15,
		0,
		0,
		time.FixedZone("AWST", 8*60*60),
	)
	created, err := store.Begin(Plan{
		OperationID:    "operation-explicit-time",
		IdempotencyKey: "test:explicit-time",
		Kind:           "test",
		CreatedAt:      explicit,
	})
	if err != nil {
		t.Fatalf("begin journal plan: %v", err)
	}
	if got, want := created.CreatedAt, explicit.UTC(); !got.Equal(want) ||
		got.Location() != time.UTC {
		t.Fatalf("created at = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}

func TestReadPlanReturnsDurablePlanWithoutMutation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	created := beginTestPlan(t, store, "operation-read", []Step{{
		Ordinal: 1,
		Action:  "create",
		Target:  "/workspace/.aidb/manifest.yaml",
	}})

	read, err := store.ReadPlan("operation-read")
	if err != nil {
		t.Fatalf("read durable plan: %v", err)
	}
	if read.Hash != created.Hash || read.CreatedAt != created.CreatedAt {
		t.Fatalf("read plan = %#v, want durable plan %#v", read, created)
	}

	read.Kind = "mutated-in-memory"
	repeated, err := store.ReadPlan("operation-read")
	if err != nil {
		t.Fatalf("reread durable plan: %v", err)
	}
	if repeated.Kind != created.Kind {
		t.Fatalf("durable plan mutated: kind = %q, want %q", repeated.Kind, created.Kind)
	}
}

func TestReadPlanValidatesOperationID(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if _, err := store.ReadPlan(""); err == nil {
		t.Fatal("expected empty operation id to fail")
	}
	if _, err := store.ReadPlan("missing-operation"); err == nil {
		t.Fatal("expected missing durable plan to fail")
	}
}

func TestOperationsRejectPlanCopiedFromAnotherOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(store *Store, operationID string) error
	}{
		{
			name: "read plan",
			run: func(store *Store, operationID string) error {
				_, err := store.ReadPlan(operationID)
				return err
			},
		},
		{
			name: "begin",
			run: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:" + operationID,
					Kind:           "test",
				})
				return err
			},
		},
		{
			name: "append",
			run: func(store *Store, operationID string) error {
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
		{
			name: "inspect",
			run: func(store *Store, operationID string) error {
				_, err := store.Inspect(operationID)
				return err
			},
		},
		{
			name: "inspect recorded",
			run: func(store *Store, operationID string) error {
				_, err := store.InspectRecorded(operationID)
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			const (
				sourceOperation = "operation-source"
				targetOperation = "operation-target"
			)
			beginTestPlan(t, store, sourceOperation, nil)
			beginTestPlan(t, store, targetOperation, nil)

			sourcePath := filepath.Join(store.root, sourceOperation, "plan.json")
			content, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read source plan: %v", err)
			}
			targetPath := filepath.Join(store.root, targetOperation, "plan.json")
			if err := os.WriteFile(targetPath, content, 0o644); err != nil {
				t.Fatalf("copy plan into target operation: %v", err)
			}

			err = test.run(store, targetOperation)
			if err == nil {
				t.Fatal("expected copied plan to be rejected")
			}
			want := `journal plan operation id = "operation-source", want "operation-target"`
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want detail %q", err, want)
			}
		})
	}
}

func TestOperationsRejectUnsafeOperationIDs(t *testing.T) {
	t.Parallel()

	operationIDs := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "current directory", id: "."},
		{name: "parent directory", id: ".."},
		{name: "forward slash", id: "nested/operation"},
		{name: "backslash", id: `nested\operation`},
		{name: "absolute path", id: "/tmp/operation"},
		{name: "path traversal", id: "nested/../operation"},
		{name: "drive relative volume", id: `C:operation`},
		{name: "drive absolute volume", id: `C:\operation`},
	}
	operations := []struct {
		name string
		run  func(store *Store, operationID string) error
	}{
		{
			name: "begin",
			run: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:unsafe-operation-id",
					Kind:           "test",
				})
				return err
			},
		},
		{
			name: "read plan",
			run: func(store *Store, operationID string) error {
				_, err := store.ReadPlan(operationID)
				return err
			},
		},
		{
			name: "append",
			run: func(store *Store, operationID string) error {
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
		{
			name: "inspect",
			run: func(store *Store, operationID string) error {
				_, err := store.Inspect(operationID)
				return err
			},
		},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			for _, operationID := range operationIDs {
				operationID := operationID
				t.Run(operationID.name, func(t *testing.T) {
					t.Parallel()

					sandbox := t.TempDir()
					store, err := NewStore(
						filepath.Join(sandbox, "journal"),
						func() time.Time { return time.Time{} },
						func() string { return "event-1" },
					)
					if err != nil {
						t.Fatalf("new journal store: %v", err)
					}

					err = operation.run(store, operationID.id)
					if err == nil {
						t.Fatal("expected unsafe operation id to fail")
					}
					if !strings.Contains(err.Error(), "journal operation id") {
						t.Fatalf(
							"error = %q, want operation id validation error",
							err,
						)
					}
				})
			}
		})
	}
}

func TestAppendUsesMonotonicSequencesAndFoldsCommittedState(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	beginTestPlan(t, store, "operation-1", nil)

	first, err := store.Append("operation-1", EventInput{
		Phase: PhaseApplying,
		Step:  1,
	})
	if err != nil {
		t.Fatalf("append applying event: %v", err)
	}
	second, err := store.Append("operation-1", EventInput{
		Phase: PhaseCommitted,
	})
	if err != nil {
		t.Fatalf("append committed event: %v", err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf(
			"event sequences = %d,%d, want 1,2",
			first.Sequence,
			second.Sequence,
		)
	}

	entries, err := os.ReadDir(filepath.Join(store.root, "operation-1"))
	if err != nil {
		t.Fatalf("read operation directory: %v", err)
	}
	var eventNames []string
	for _, entry := range entries {
		if isEventFilename(entry.Name()) {
			eventNames = append(eventNames, entry.Name())
		}
	}
	if len(eventNames) != 2 {
		t.Fatalf("event file count = %d, want 2", len(eventNames))
	}
	if eventNames[0] != "000001-event-1.json" {
		t.Fatalf("first event filename = %q", eventNames[0])
	}
	if eventNames[1] != "000002-event-2.json" {
		t.Fatalf("second event filename = %q", eventNames[1])
	}

	state, err := store.Inspect("operation-1")
	if err != nil {
		t.Fatalf("inspect committed operation: %v", err)
	}
	if state.Status != StatusCommitted {
		t.Fatalf("status = %q, want %q", state.Status, StatusCommitted)
	}
	if state.LastSequence != 2 {
		t.Fatalf("last sequence = %d, want 2", state.LastSequence)
	}
}

func TestInspectDetectsMissingEventSequence(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	beginTestPlan(t, store, "operation-1", nil)

	if _, err := store.Append("operation-1", EventInput{
		Phase: PhaseApplying,
	}); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	second, err := store.Append("operation-1", EventInput{
		Phase: PhaseCommitted,
	})
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}

	operationDir := filepath.Join(store.root, "operation-1")
	oldPath := filepath.Join(
		operationDir,
		eventFilename(second.Sequence, second.ID),
	)
	newPath := filepath.Join(operationDir, eventFilename(3, second.ID))
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("create event sequence gap: %v", err)
	}

	state, err := store.Inspect("operation-1")
	if err != nil {
		t.Fatalf("inspect sequence gap: %v", err)
	}
	if state.Status != StatusAttention {
		t.Fatalf("status = %q, want %q", state.Status, StatusAttention)
	}
	if state.Reason != "event_sequence_gap" {
		t.Fatalf("reason = %q, want event_sequence_gap", state.Reason)
	}
}

func TestInspectionRejectsEventCopiedFromAnotherOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inspect func(store *Store, operationID string) (State, error)
	}{
		{
			name: "inspect",
			inspect: func(store *Store, operationID string) (State, error) {
				return store.Inspect(operationID)
			},
		},
		{
			name: "inspect recorded",
			inspect: func(store *Store, operationID string) (State, error) {
				return store.InspectRecorded(operationID)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			const (
				sourceOperation = "operation-source"
				targetOperation = "operation-target"
			)
			beginTestPlan(t, store, sourceOperation, nil)
			event, err := store.Append(sourceOperation, EventInput{
				Phase: PhaseCommitted,
			})
			if err != nil {
				t.Fatalf("append source event: %v", err)
			}
			beginTestPlan(t, store, targetOperation, nil)

			eventName := eventFilename(event.Sequence, event.ID)
			sourcePath := filepath.Join(store.root, sourceOperation, eventName)
			content, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read source event: %v", err)
			}
			targetPath := filepath.Join(store.root, targetOperation, eventName)
			if err := os.WriteFile(targetPath, content, 0o644); err != nil {
				t.Fatalf("copy event into target operation: %v", err)
			}

			state, err := test.inspect(store, targetOperation)
			if err != nil {
				t.Fatalf("inspect target operation: %v", err)
			}
			if state.Status != StatusAttention {
				t.Fatalf("status = %q, want %q", state.Status, StatusAttention)
			}
			if state.LastSequence != 0 {
				t.Fatalf("last sequence = %d, want 0", state.LastSequence)
			}
			if state.Reason != "event_operation_mismatch" {
				t.Fatalf(
					"reason = %q, want event_operation_mismatch",
					state.Reason,
				)
			}
		})
	}
}

func TestInspectClassifiesInterruptedStepWithoutMutatingTarget(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(target, []byte("unexpected\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	store := newTestStore(t)
	beginTestPlan(t, store, "operation-1", []Step{{
		Ordinal:    1,
		Action:     "replace",
		Target:     target,
		BeforeHash: Digest([]byte("before\n")),
		AfterHash:  Digest([]byte("after\n")),
	}})
	if _, err := store.Append("operation-1", EventInput{
		Phase: PhaseStepApplying,
		Step:  1,
	}); err != nil {
		t.Fatalf("append interrupted step event: %v", err)
	}

	state, err := store.Inspect("operation-1")
	if err != nil {
		t.Fatalf("inspect interrupted operation: %v", err)
	}
	if state.Status != StatusAttention {
		t.Fatalf("status = %q, want %q", state.Status, StatusAttention)
	}
	if state.Reason != "target_hash_ambiguous" {
		t.Fatalf("reason = %q, want target_hash_ambiguous", state.Reason)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after inspection: %v", err)
	}
	if got, want := string(content), "unexpected\n"; got != want {
		t.Fatalf("inspection mutated target: got %q, want %q", got, want)
	}
}

func TestInspectClassifiesRenameAppliedBeforeStepAppliedEvent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	original := filepath.Join(root, "tickets", "TASK-00039", "status.yaml")
	applied := filepath.Join(root, ".aidb", "relocations", "operation-1", "status.yaml")
	if err := os.MkdirAll(filepath.Dir(applied), 0o755); err != nil {
		t.Fatalf("create applied target parent: %v", err)
	}
	content := []byte("id: TASK-00039\n")
	if err := os.WriteFile(applied, content, 0o644); err != nil {
		t.Fatalf("seed applied target: %v", err)
	}

	store := newTestStore(t)
	beginTestPlan(t, store, "operation-rename-window", []Step{{
		Ordinal:       1,
		Action:        "rename",
		Target:        original,
		AppliedTarget: applied,
		BeforeHash:    Digest(content),
	}})
	if _, err := store.Append("operation-rename-window", EventInput{
		Phase: PhaseStepApplying,
		Step:  1,
	}); err != nil {
		t.Fatalf("append interrupted rename event: %v", err)
	}

	state, err := store.Inspect("operation-rename-window")
	if err != nil {
		t.Fatalf("inspect interrupted rename: %v", err)
	}
	if state.Status != StatusApplying {
		t.Fatalf("status = %q, want %q", state.Status, StatusApplying)
	}
	if !state.AppliedUnrecorded {
		t.Fatal("applied unrecorded = false, want true")
	}
	if state.Reason != "" {
		t.Fatalf("reason = %q, want empty", state.Reason)
	}
}

// TestInspectFlagsUnreadableAppliedTargetForAttention pins the fail-safe
// direction of the applied-target fold. The read error is deliberately turned
// into a state rather than returned (the State model carries a reason enum, not
// an error), and the state it must become is attention: an applied target that
// cannot be read is NOT evidence that the step landed, so folding it to
// applying/applied-unrecorded would let a caller resume — or commit — an
// operation whose effect on disk is unknown.
func TestInspectFlagsUnreadableAppliedTargetForAttention(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	original := filepath.Join(root, "tickets", "TASK-00039", "status.yaml")
	applied := filepath.Join(
		root,
		".aidb",
		"relocations",
		"operation-1",
		"status.yaml",
	)
	// A directory at the applied path makes the read fail with a real IO error
	// (EISDIR) rather than ErrNotExist, which is the arm under test.
	if err := os.MkdirAll(applied, 0o755); err != nil {
		t.Fatalf("seed unreadable applied target: %v", err)
	}

	store := newTestStore(t)
	beginTestPlan(t, store, "operation-unreadable-applied", []Step{{
		Ordinal:       1,
		Action:        "rename",
		Target:        original,
		AppliedTarget: applied,
		BeforeHash:    Digest([]byte("id: TASK-00039\n")),
	}})
	if _, err := store.Append("operation-unreadable-applied", EventInput{
		Phase: PhaseStepApplying,
		Step:  1,
	}); err != nil {
		t.Fatalf("append interrupted rename event: %v", err)
	}

	state, err := store.Inspect("operation-unreadable-applied")
	if err != nil {
		t.Fatalf("inspect unreadable applied target: %v", err)
	}
	if state.Status != StatusAttention {
		t.Fatalf("status = %q, want %q", state.Status, StatusAttention)
	}
	if state.Reason != "target_unreadable" {
		t.Fatalf("reason = %q, want %q", state.Reason, "target_unreadable")
	}
	if state.AppliedUnrecorded {
		t.Fatal("applied unrecorded = true, want false for an unreadable target")
	}
	if state.Error() == nil {
		t.Fatal("state error = nil, want an attention error")
	}
}

func TestInspectClassifiesInterruptedCreateDirectoryStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		seedTarget            func(t *testing.T, target string)
		wantStatus            Status
		wantAppliedUnrecorded bool
		wantReason            string
	}{
		{
			name: "existing real directory is applied but unrecorded",
			seedTarget: func(t *testing.T, target string) {
				t.Helper()
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("seed target directory: %v", err)
				}
			},
			wantStatus:            StatusApplying,
			wantAppliedUnrecorded: true,
		},
		{
			name:       "missing target is not yet applied",
			seedTarget: func(t *testing.T, target string) {},
			wantStatus: StatusApplying,
		},
		{
			name: "regular file needs attention",
			seedTarget: func(t *testing.T, target string) {
				t.Helper()
				if err := os.WriteFile(target, []byte("not a directory"), 0o644); err != nil {
					t.Fatalf("seed regular file target: %v", err)
				}
			},
			wantStatus: StatusAttention,
			wantReason: "target_not_directory",
		},
		{
			name: "directory symlink needs attention",
			seedTarget: func(t *testing.T, target string) {
				t.Helper()
				destination := filepath.Join(filepath.Dir(target), "destination")
				if err := os.Mkdir(destination, 0o755); err != nil {
					t.Fatalf("seed symlink destination: %v", err)
				}
				if err := os.Symlink(destination, target); err != nil {
					t.Skipf("create directory symlink: %v", err)
				}
			},
			wantStatus: StatusAttention,
			wantReason: "target_unsafe",
		},
		{
			name: "unreadable target needs attention",
			seedTarget: func(t *testing.T, target string) {
				t.Helper()
				if runtime.GOOS == "windows" {
					t.Skip("directory permission semantics differ on Windows")
				}
				parent := filepath.Dir(target)
				if err := os.Chmod(parent, 0); err != nil {
					t.Fatalf("make target parent unreadable: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(parent, 0o755); err != nil {
						t.Errorf("restore target parent permissions: %v", err)
					}
				})
			},
			wantStatus: StatusAttention,
			wantReason: "target_unreadable",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			targetParent := filepath.Join(root, test.name)
			if err := os.Mkdir(targetParent, 0o755); err != nil {
				t.Fatalf("create target parent: %v", err)
			}
			target := filepath.Join(targetParent, "target")
			test.seedTarget(t, target)

			store := newTestStore(t)
			beginTestPlan(t, store, "operation-create-directory", []Step{{
				Ordinal: 1,
				Action:  "create-directory",
				Target:  target,
			}})
			if _, err := store.Append("operation-create-directory", EventInput{
				Phase: PhaseStepApplying,
				Step:  1,
			}); err != nil {
				t.Fatalf("append interrupted create-directory event: %v", err)
			}

			state, err := store.Inspect("operation-create-directory")
			if err != nil {
				t.Fatalf("inspect interrupted create-directory: %v", err)
			}
			if state.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q", state.Status, test.wantStatus)
			}
			if state.AppliedUnrecorded != test.wantAppliedUnrecorded {
				t.Fatalf(
					"applied unrecorded = %t, want %t",
					state.AppliedUnrecorded,
					test.wantAppliedUnrecorded,
				)
			}
			if state.Reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", state.Reason, test.wantReason)
			}
		})
	}
}

func TestInspectRecordedDoesNotDereferenceStepTargets(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	target := filepath.Join(t.TempDir(), "missing-target")
	beginTestPlan(t, store, "operation-recorded-only", []Step{{
		Ordinal:    1,
		Action:     "replace",
		Target:     target,
		BeforeHash: Digest([]byte("before")),
		AfterHash:  Digest([]byte("after")),
	}})
	if _, err := store.Append("operation-recorded-only", EventInput{
		Phase: PhaseStepApplying,
		Step:  1,
	}); err != nil {
		t.Fatalf("append step applying: %v", err)
	}

	state, err := store.InspectRecorded("operation-recorded-only")
	if err != nil {
		t.Fatalf("inspect recorded state: %v", err)
	}
	if state.Status != StatusApplying ||
		state.AppliedUnrecorded ||
		state.Reason != "" {
		t.Fatalf("recorded state = %#v", state)
	}
}

func TestBeginRejectsInvalidAppliedTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step Step
	}{
		{
			name: "same as original target",
			step: Step{
				Ordinal:       1,
				Action:        "rename",
				Target:        "/workspace/ticket/status.yaml",
				AppliedTarget: "/workspace/ticket/status.yaml",
				BeforeHash:    Digest([]byte("before")),
			},
		},
		{
			name: "missing expected hash",
			step: Step{
				Ordinal:       1,
				Action:        "rename",
				Target:        "/workspace/ticket/status.yaml",
				AppliedTarget: "/workspace/trash/ticket/status.yaml",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := newTestStore(t)
			_, err := store.Begin(Plan{
				OperationID:    "operation-invalid-applied-target",
				IdempotencyKey: "test:invalid-applied-target",
				Kind:           "test",
				Steps:          []Step{test.step},
			})
			if err == nil {
				t.Fatal("expected invalid applied target to fail")
			}
		})
	}
}

func TestBeginRejectsMkdirStepWithAppliedTargetOrHashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		step       Step
		wantDetail string
	}{
		{
			name: "before hash",
			step: Step{
				BeforeHash: Digest([]byte("before")),
			},
			wantDetail: "before hash",
		},
		{
			name: "after hash",
			step: Step{
				AfterHash: Digest([]byte("after")),
			},
			wantDetail: "after hash",
		},
		{
			name: "applied target",
			step: Step{
				AppliedTarget: "/workspace/ticket/applied",
				BeforeHash:    Digest([]byte("before")),
			},
			wantDetail: "applied target",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			test.step.Ordinal = 1
			test.step.Action = "create-directory"
			test.step.Target = "/workspace/ticket"

			_, err := store.Begin(Plan{
				OperationID:    "operation-invalid-create-directory",
				IdempotencyKey: "test:invalid-create-directory",
				Kind:           "test",
				Steps:          []Step{test.step},
			})
			if err == nil {
				t.Fatal("expected create-directory metadata to fail")
			}
			if !strings.Contains(err.Error(), test.wantDetail) {
				t.Fatalf(
					"error = %q, want detail %q",
					err,
					test.wantDetail,
				)
			}
		})
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	ids := []string{"event-1", "event-2", "event-3"}
	idIndex := 0
	store, err := NewStore(
		t.TempDir(),
		func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				8,
				0,
				idIndex,
				0,
				time.UTC,
			)
		},
		func() string {
			id := ids[idIndex]
			idIndex++
			return id
		},
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	return store
}

func beginTestPlan(
	t *testing.T,
	store *Store,
	operationID string,
	steps []Step,
) Plan {
	t.Helper()

	plan, err := store.Begin(Plan{
		OperationID:    operationID,
		IdempotencyKey: "test:" + operationID,
		Kind:           "test",
		Steps:          steps,
	})
	if err != nil {
		t.Fatalf("begin test plan: %v", err)
	}
	return plan
}

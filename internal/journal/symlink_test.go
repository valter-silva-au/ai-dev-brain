package journal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOperationsRejectSymlinkedOperationDirectory(t *testing.T) {
	t.Parallel()

	for _, operation := range journalOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			sandbox := t.TempDir()
			journalRoot := filepath.Join(sandbox, "journal")
			if err := os.Mkdir(journalRoot, 0o755); err != nil {
				t.Fatalf("create journal root: %v", err)
			}

			const operationID = "operation-symlink"
			externalOperation := seedExternalOperation(t, operationID)
			redirect := filepath.Join(journalRoot, operationID)
			if err := os.Symlink(externalOperation, redirect); err != nil {
				t.Skipf("create operation directory symlink: %v", err)
			}

			before := snapshotTree(t, externalOperation)
			store := newStoreAt(t, journalRoot)
			if err := operation.run(store, operationID); err == nil {
				t.Fatal("expected symlinked operation directory to be rejected")
			}
			after := snapshotTree(t, externalOperation)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf(
					"external operation changed through symlink:\nbefore: %#v\nafter:  %#v",
					before,
					after,
				)
			}
		})
	}
}

func TestOperationsRejectSymlinkedEventsRoot(t *testing.T) {
	t.Parallel()

	for _, operation := range journalOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			sandbox := t.TempDir()
			externalRoot := filepath.Join(sandbox, "external-journal")
			const operationID = "operation-symlink"
			seedExternalOperationAt(t, externalRoot, operationID)

			redirect := filepath.Join(sandbox, "journal")
			if err := os.Symlink(externalRoot, redirect); err != nil {
				t.Skipf("create events root symlink: %v", err)
			}

			before := snapshotTree(t, externalRoot)
			store := newStoreAt(t, redirect)
			if err := operation.run(store, operationID); err == nil {
				t.Fatal("expected symlinked events root to be rejected")
			}
			after := snapshotTree(t, externalRoot)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf(
					"external journal changed through symlink:\nbefore: %#v\nafter:  %#v",
					before,
					after,
				)
			}
		})
	}
}

func TestOperationsRejectSymlinkedImmediateJournalRootAncestor(
	t *testing.T,
) {
	t.Parallel()

	for _, operation := range journalOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			sandbox := t.TempDir()
			workspace := filepath.Join(sandbox, "workspace")
			if err := os.Mkdir(workspace, 0o755); err != nil {
				t.Fatalf("create workspace: %v", err)
			}

			externalAidb := filepath.Join(sandbox, "external-aidb")
			externalEvents := filepath.Join(externalAidb, "events")
			const operationID = "operation-symlink"
			seedExternalOperationAt(t, externalEvents, operationID)

			ancestor := filepath.Join(workspace, ".aidb")
			if err := os.Symlink(externalAidb, ancestor); err != nil {
				t.Skipf("create journal ancestor symlink: %v", err)
			}

			before := snapshotTree(t, externalAidb)
			store := newStoreAt(t, filepath.Join(ancestor, "events"))
			if err := operation.run(store, operationID); err == nil {
				t.Fatal(
					"expected symlinked immediate journal-root ancestor to be rejected",
				)
			}
			after := snapshotTree(t, externalAidb)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf(
					"external tree changed through journal ancestor symlink:\nbefore: %#v\nafter:  %#v",
					before,
					after,
				)
			}
		})
	}
}

func TestOperationsRejectSymlinkedNestedJournalRootAncestor(t *testing.T) {
	t.Parallel()

	for _, operation := range journalOperations() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()

			sandbox := t.TempDir()
			workspace := filepath.Join(sandbox, "workspace")
			if err := os.Mkdir(workspace, 0o755); err != nil {
				t.Fatalf("create workspace: %v", err)
			}

			externalWorkspace := filepath.Join(sandbox, "external-workspace")
			externalEvents := filepath.Join(
				externalWorkspace,
				"project",
				".aidb",
				"events",
			)
			const operationID = "operation-symlink"
			seedExternalOperationAt(t, externalEvents, operationID)

			nestedAncestor := filepath.Join(workspace, "project")
			if err := os.Symlink(
				filepath.Join(externalWorkspace, "project"),
				nestedAncestor,
			); err != nil {
				t.Skipf("create nested journal ancestor symlink: %v", err)
			}

			before := snapshotTree(t, externalWorkspace)
			store := newStoreAt(
				t,
				filepath.Join(nestedAncestor, ".aidb", "events"),
			)
			if err := operation.run(store, operationID); err == nil {
				t.Fatal("expected nested journal-root ancestor symlink rejection")
			}
			after := snapshotTree(t, externalWorkspace)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf(
					"external tree changed through nested journal ancestor symlink:\nbefore: %#v\nafter:  %#v",
					before,
					after,
				)
			}
		})
	}
}

type journalOperation struct {
	name string
	run  func(store *Store, operationID string) error
}

func journalOperations() []journalOperation {
	return []journalOperation{
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
		{
			name: "inspect recorded",
			run: func(store *Store, operationID string) error {
				_, err := store.InspectRecorded(operationID)
				return err
			},
		},
	}
}

func seedExternalOperation(t *testing.T, operationID string) string {
	t.Helper()

	externalRoot := filepath.Join(t.TempDir(), "external-journal")
	seedExternalOperationAt(t, externalRoot, operationID)
	return filepath.Join(externalRoot, operationID)
}

func seedExternalOperationAt(
	t *testing.T,
	externalRoot string,
	operationID string,
) {
	t.Helper()

	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatalf("create external journal root: %v", err)
	}
	store := newStoreAt(t, externalRoot)
	if _, err := store.Begin(Plan{
		OperationID:    operationID,
		IdempotencyKey: "test:" + operationID,
		Kind:           "test",
	}); err != nil {
		t.Fatalf("seed external operation: %v", err)
	}
	lockPath := filepath.Join(externalRoot, operationID, ".lock")
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove seed lock file: %v", err)
	}
}

func newStoreAt(t *testing.T, root string) *Store {
	t.Helper()

	store, err := NewStore(
		root,
		func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				12,
				0,
				0,
				0,
				time.UTC,
			)
		},
		func() string { return "event-symlink" },
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	return store
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s:%#o", info.Mode().Type(), info.Mode().Perm())
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(content)
		}
		snapshot[relative] = value
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %q: %v", root, err)
	}
	return snapshot
}

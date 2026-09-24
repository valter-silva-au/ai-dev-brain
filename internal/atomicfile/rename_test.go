package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRenameMovesEntryAcrossDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	targetDir := filepath.Join(root, "target")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create target directory: %v", err)
	}

	source := filepath.Join(sourceDir, "manifest.yaml")
	target := filepath.Join(targetDir, "manifest.yaml")
	if err := os.WriteFile(source, []byte("schema_version: v1\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	if err := Rename(source, target); err != nil {
		t.Fatalf("durable rename: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists after rename: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read renamed target: %v", err)
	}
	if got, want := string(content), "schema_version: v1\n"; got != want {
		t.Fatalf("target content = %q, want %q", got, want)
	}
}

func TestSyncRenameParentsDeduplicatesSameParent(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "tickets")
	var synced []string
	err := syncRenameParents(
		filepath.Join(parent, "before"),
		filepath.Join(parent, "after"),
		func(path string) error {
			synced = append(synced, path)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("sync rename parents: %v", err)
	}
	if want := []string{parent}; !reflect.DeepEqual(synced, want) {
		t.Fatalf("synced parents = %#v, want %#v", synced, want)
	}
}

func TestSyncRenameParentsSyncsBothAffectedParents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceParent := filepath.Join(root, "source")
	targetParent := filepath.Join(root, "target")
	var synced []string
	err := syncRenameParents(
		filepath.Join(sourceParent, "ticket"),
		filepath.Join(targetParent, "ticket"),
		func(path string) error {
			synced = append(synced, path)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("sync rename parents: %v", err)
	}
	if want := []string{sourceParent, targetParent}; !reflect.DeepEqual(synced, want) {
		t.Fatalf("synced parents = %#v, want %#v", synced, want)
	}
}

func TestRenameRejectsEmptyPaths(t *testing.T) {
	t.Parallel()

	if err := Rename("", "target"); err == nil {
		t.Fatal("expected empty source to fail")
	}
	if err := Rename("source", ""); err == nil {
		t.Fatal("expected empty target to fail")
	}
}

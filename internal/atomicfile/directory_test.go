package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMkdirAllSyncsEveryNewParentEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "one", "two", "three")
	synced := make([]string, 0)
	if err := mkdirAllWithin(root, target, 0o755, func(path string) error {
		synced = append(synced, filepath.Clean(path))
		return nil
	}); err != nil {
		t.Fatalf("create durable directory tree: %v", err)
	}
	want := []string{
		filepath.Join(root, "one", "two"),
		filepath.Join(root, "one"),
		root,
	}
	if !reflect.DeepEqual(synced, want) {
		t.Fatalf("synced parents = %#v, want %#v", synced, want)
	}
}

func TestMkdirAllWithinRetriesSynchronizationAfterCreation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "one", "two")
	failed := false
	injected := errors.New("injected directory sync failure")
	if err := mkdirAllWithin(root, target, 0o755, func(string) error {
		if !failed {
			failed = true
			return injected
		}
		return nil
	}); err == nil {
		t.Fatal("expected injected synchronization failure")
	}
	synced := make([]string, 0)
	if err := mkdirAllWithin(root, target, 0o755, func(path string) error {
		synced = append(synced, filepath.Clean(path))
		return nil
	}); err != nil {
		t.Fatalf("retry durable directory sync: %v", err)
	}
	want := []string{filepath.Join(root, "one"), root}
	if !reflect.DeepEqual(synced, want) {
		t.Fatalf("retry synced parents = %#v, want %#v", synced, want)
	}
}

func TestMkdirAllWithinRejectsExternalSymlinkAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	redirect := filepath.Join(root, "redirect")
	if err := os.Symlink(external, redirect); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	target := filepath.Join(redirect, "nested")
	if err := MkdirAllWithin(root, target, 0o755); err == nil {
		t.Fatal("expected redirected directory creation to fail")
	}
	if _, err := os.Stat(filepath.Join(external, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external directory was created: %v", err)
	}
}

func TestMkdirAllWithinRequiresAbsoluteTarget(t *testing.T) {
	t.Parallel()

	err := MkdirAllWithin(t.TempDir(), filepath.Join("relative", "target"), 0o755)
	if err == nil {
		t.Fatal("expected relative target to fail")
	}
	if !strings.Contains(err.Error(), "directory path must be absolute") {
		t.Fatalf("error = %v, want explicit absolute directory path failure", err)
	}
}

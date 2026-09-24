//go:build windows

package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteWithinReplacesExistingFileWindowsRuntime(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "manifest.yaml")
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	if err := WriteWithin(
		root,
		target,
		Options{Mode: 0o600},
		func(writer io.Writer) error {
			_, err := io.WriteString(writer, "replacement\n")
			return err
		},
	); err != nil {
		t.Fatalf("replace existing file on Windows: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if got, want := string(content), "replacement\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRenameWithinReplacesExistingFileWindowsRuntime(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.yaml")
	target := filepath.Join(root, "target.yaml")
	if err := os.WriteFile(source, []byte("source\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := os.WriteFile(target, []byte("target\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	if err := RenameWithin(root, source, target); err != nil {
		t.Fatalf("replace target with rename on Windows: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if got, want := string(content), "source\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestSyncRootDirectoryUsesOpenedHandleAfterRootRename(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "root")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatalf("create root: %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer root.Close()

	movedPath := filepath.Join(parent, "moved")
	if err := os.Rename(rootPath, movedPath); err != nil {
		t.Fatalf("rename opened root to exercise handle-relative sync: %v", err)
	}
	if err := syncRootDirectory(root, "."); err != nil {
		t.Fatalf("sync renamed rooted directory handle: %v", err)
	}
}

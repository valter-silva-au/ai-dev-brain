package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePublishesOnlyAfterCallbackCompletes(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "manifest.yaml")
	err := Write(target, Options{Mode: 0o640}, func(writer io.Writer) error {
		if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("target visible before publication: %v", statErr)
		}
		_, writeErr := io.WriteString(writer, "schema_version: aidb.workspace/v1\n")
		return writeErr
	})
	if err != nil {
		t.Fatalf("write atomic file: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if got, want := string(content), "schema_version: aidb.workspace/v1\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
}

func TestWriteFailurePreservesExistingTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	writeFailure := errors.New("render failed")
	err := Write(target, Options{Mode: 0o600}, func(writer io.Writer) error {
		if _, writeErr := io.WriteString(writer, "replacement\n"); writeErr != nil {
			return writeErr
		}
		return writeFailure
	})
	if !errors.Is(err, writeFailure) {
		t.Fatalf("write error = %v, want render failure", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read preserved target: %v", err)
	}
	if got, want := string(content), "original\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read target directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.yaml" {
		t.Fatalf("temporary files remain after failure: %#v", entries)
	}
}

func TestWriteCreatesParentsOnlyWhenRequested(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "nested", "state", "config.yaml")
	write := func(writer io.Writer) error {
		_, err := io.WriteString(writer, "version: 1\n")
		return err
	}

	if err := Write(target, Options{Mode: 0o644}, write); err == nil {
		t.Fatal("expected missing parent to fail without CreateParents")
	}
	if _, err := os.Stat(filepath.Dir(target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("parent unexpectedly created: %v", err)
	}

	if err := Write(target, Options{
		Mode:          0o644,
		CreateParents: true,
	}, write); err != nil {
		t.Fatalf("write with parent creation: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target missing after parent creation: %v", err)
	}
}

func TestWriteRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		options Options
		write   func(io.Writer) error
	}{
		{
			name:    "empty path",
			options: Options{Mode: 0o644},
			write:   func(io.Writer) error { return nil },
		},
		{
			name:  "zero mode",
			path:  filepath.Join(t.TempDir(), "file"),
			write: func(io.Writer) error { return nil },
		},
		{
			name:    "nil callback",
			path:    filepath.Join(t.TempDir(), "file"),
			options: Options{Mode: 0o644},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := Write(test.path, test.options, test.write); err == nil {
				t.Fatal("expected invalid input to fail")
			}
		})
	}
}

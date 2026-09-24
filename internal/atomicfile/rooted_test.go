package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWithinRejectsExternalSymlinkAncestor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	redirect := filepath.Join(root, "redirect")
	if err := os.Symlink(external, redirect); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	externalTarget := filepath.Join(external, "manifest.yaml")
	if err := os.WriteFile(externalTarget, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("seed external target: %v", err)
	}

	err := WriteWithin(
		root,
		filepath.Join(redirect, "manifest.yaml"),
		Options{Mode: 0o644},
		func(writer io.Writer) error {
			_, writeErr := io.WriteString(writer, "replacement\n")
			return writeErr
		},
	)
	if err == nil {
		t.Fatal("expected redirected write to fail")
	}

	content, readErr := os.ReadFile(externalTarget)
	if readErr != nil {
		t.Fatalf("read external target: %v", readErr)
	}
	if got, want := string(content), "original\n"; got != want {
		t.Fatalf("external content = %q, want %q", got, want)
	}
	assertNoTemporaryFiles(t, external)
}

func TestWriteWithinPublishesAndCleansUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "nested", "manifest.yaml")
	if err := WriteWithin(
		root,
		target,
		Options{Mode: 0o640, CreateParents: true},
		func(writer io.Writer) error {
			_, err := io.WriteString(writer, "schema_version: v1\n")
			return err
		},
	); err != nil {
		t.Fatalf("write within root: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read published target: %v", err)
	}
	if got, want := string(content), "schema_version: v1\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat published target: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
	assertNoTemporaryFiles(t, filepath.Dir(target))
}

func TestWriteWithinReplacesExistingFile(t *testing.T) {
	t.Parallel()

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
		t.Fatalf("replace existing file: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	if got, want := string(content), "replacement\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat replacement: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
	assertNoTemporaryFiles(t, root)
}

func TestWriteWithinFailurePreservesTargetAndRemovesTemporaryFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "manifest.yaml")
	if err := os.WriteFile(target, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	writeFailure := errors.New("render failed")

	err := WriteWithin(
		root,
		target,
		Options{Mode: 0o600},
		func(writer io.Writer) error {
			if _, writeErr := io.WriteString(writer, "replacement\n"); writeErr != nil {
				return writeErr
			}
			return writeFailure
		},
	)
	if !errors.Is(err, writeFailure) {
		t.Fatalf("write error = %v, want render failure", err)
	}

	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read preserved target: %v", readErr)
	}
	if got, want := string(content), "original\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertNoTemporaryFiles(t, root)
}

func TestWriteWithinPublicationFailureRemovesTemporaryFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "manifest.yaml")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create conflicting target directory: %v", err)
	}

	err := WriteWithin(
		root,
		target,
		Options{Mode: 0o600},
		func(writer io.Writer) error {
			_, writeErr := io.WriteString(writer, "replacement\n")
			return writeErr
		},
	)
	if err == nil {
		t.Fatal("expected publication over directory to fail")
	}
	info, statErr := os.Stat(target)
	if statErr != nil {
		t.Fatalf("stat conflicting target: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("conflicting target directory was replaced")
	}
	assertNoTemporaryFiles(t, root)
}

func TestWriteWithinPropagatesTemporaryCloseCleanupFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFailure := errors.New("render failed")
	err := WriteWithin(
		root,
		filepath.Join(root, "manifest.yaml"),
		Options{Mode: 0o600},
		func(writer io.Writer) error {
			file, ok := writer.(*os.File)
			if !ok {
				t.Fatalf("writer type = %T, want *os.File", writer)
			}
			if closeErr := file.Close(); closeErr != nil {
				t.Fatalf("close temporary file in callback: %v", closeErr)
			}
			return writeFailure
		},
	)
	if !errors.Is(err, writeFailure) {
		t.Fatalf("error = %v, want render failure", err)
	}
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("error = %v, want temporary close cleanup failure", err)
	}
	assertNoTemporaryFiles(t, root)
}

func TestWriteWithinPropagatesTemporaryRemoveCleanupFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFailure := errors.New("render failed")
	err := WriteWithin(
		root,
		filepath.Join(root, "manifest.yaml"),
		Options{Mode: 0o600},
		func(io.Writer) error {
			entries, readErr := os.ReadDir(root)
			if readErr != nil {
				t.Fatalf("read temporary directory: %v", readErr)
			}
			var temporaryPath string
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".tmp-") {
					temporaryPath = filepath.Join(root, entry.Name())
					break
				}
			}
			if temporaryPath == "" {
				t.Fatal("temporary file not found")
			}
			if removeErr := os.Remove(temporaryPath); removeErr != nil {
				t.Fatalf("remove temporary file in callback: %v", removeErr)
			}
			return writeFailure
		},
	)
	if !errors.Is(err, writeFailure) {
		t.Fatalf("error = %v, want render failure", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want temporary remove cleanup failure", err)
	}
	assertNoTemporaryFiles(t, root)
}

func TestRenameWithinRejectsExternalSymlinkAncestors(t *testing.T) {
	t.Parallel()

	t.Run("target", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		external := t.TempDir()
		redirect := filepath.Join(root, "redirect")
		if err := os.Symlink(external, redirect); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		source := filepath.Join(root, "source.yaml")
		if err := os.WriteFile(source, []byte("source\n"), 0o644); err != nil {
			t.Fatalf("seed source: %v", err)
		}

		if err := RenameWithin(
			root,
			source,
			filepath.Join(redirect, "target.yaml"),
		); err == nil {
			t.Fatal("expected redirected target rename to fail")
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatalf("source changed after failed rename: %v", err)
		}
		if _, err := os.Stat(filepath.Join(external, "target.yaml")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("external target was created: %v", err)
		}
	})

	t.Run("source", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		external := t.TempDir()
		externalSource := filepath.Join(external, "source.yaml")
		if err := os.WriteFile(externalSource, []byte("source\n"), 0o644); err != nil {
			t.Fatalf("seed external source: %v", err)
		}
		redirect := filepath.Join(root, "redirect")
		if err := os.Symlink(external, redirect); err != nil {
			t.Skipf("create symlink: %v", err)
		}

		if err := RenameWithin(
			root,
			filepath.Join(redirect, "source.yaml"),
			filepath.Join(root, "target.yaml"),
		); err == nil {
			t.Fatal("expected redirected source rename to fail")
		}
		if _, err := os.Stat(externalSource); err != nil {
			t.Fatalf("external source changed after failed rename: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "target.yaml")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("contained target was created: %v", err)
		}
	})
}

func TestRenameWithinMovesContainedEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	targetDir := filepath.Join(root, "target")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	source := filepath.Join(sourceDir, "manifest.yaml")
	target := filepath.Join(targetDir, "manifest.yaml")
	if err := os.WriteFile(source, []byte("schema_version: v1\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	if err := RenameWithin(root, source, target); err != nil {
		t.Fatalf("rename within root: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if got, want := string(content), "schema_version: v1\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRenameWithinReplacesExistingFile(t *testing.T) {
	t.Parallel()

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
		t.Fatalf("replace target with rename: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if got, want := string(content), "source\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRootedOperationsRequireAbsoluteEndpoints(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absolute := filepath.Join(root, "absolute")
	writer := func(io.Writer) error { return nil }
	tests := []struct {
		name string
		want string
		run  func() error
	}{
		{
			name: "write target",
			want: "atomic file target must be absolute",
			run: func() error {
				return WriteWithin(
					root,
					filepath.Join("relative", "file"),
					Options{Mode: 0o644},
					writer,
				)
			},
		},
		{
			name: "rename source",
			want: "atomic rename source must be absolute",
			run: func() error {
				return RenameWithin(root, filepath.Join("relative", "source"), absolute)
			},
		},
		{
			name: "rename target",
			want: "atomic rename target must be absolute",
			run: func() error {
				return RenameWithin(root, absolute, filepath.Join("relative", "target"))
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.run()
			if err == nil {
				t.Fatal("expected relative endpoint to fail")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRootedOperationsRequireAbsoluteContainedPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	outside := filepath.Join(t.TempDir(), "outside")
	writer := func(io.Writer) error { return nil }

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "mkdir relative root",
			run: func() error {
				return MkdirAllWithin("relative", filepath.Join("relative", "child"), 0o755)
			},
		},
		{
			name: "write relative root",
			run: func() error {
				return WriteWithin("relative", filepath.Join("relative", "file"), Options{Mode: 0o644}, writer)
			},
		},
		{
			name: "rename relative root",
			run: func() error {
				return RenameWithin("relative", filepath.Join("relative", "source"), filepath.Join("relative", "target"))
			},
		},
		{
			name: "mkdir outside target",
			run: func() error {
				return MkdirAllWithin(root, outside, 0o755)
			},
		},
		{
			name: "write outside target",
			run: func() error {
				return WriteWithin(root, outside, Options{Mode: 0o644}, writer)
			},
		},
		{
			name: "rename outside source",
			run: func() error {
				return RenameWithin(root, outside, inside)
			},
		},
		{
			name: "rename outside target",
			run: func() error {
				return RenameWithin(root, inside, outside)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.run(); err == nil {
				t.Fatal("expected invalid rooted path to fail")
			}
		})
	}
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory %q: %v", directory, err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temporary file remains: %q", entry.Name())
		}
	}
}

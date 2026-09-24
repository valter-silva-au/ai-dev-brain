package repository

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// probeKey is the one command Inventory runs before it will admit a path is a
// repository at all.
func probeKey(path string) string {
	return commandKey(path, "rev-parse", "--is-inside-work-tree")
}

func commandFailure(path string, stderr string, cause error) error {
	return &CommandError{
		Invocation: Invocation{
			Dir:  path,
			Args: []string{"rev-parse", "--is-inside-work-tree"},
		},
		Stderr: stderr,
		Cause:  cause,
	}
}

// TestGitInventoryReportsUnrunnableGit pins the distinction Inventory has to
// make. `git rev-parse --is-inside-work-tree` exits non-zero when the path is
// simply not in a work tree, so a command failure is the probe's ANSWER. Failing
// to run git at all is not an answer: if it were reported as "not a repository",
// then on a machine without git every path on disk would look un-adoptable and
// `adb repo adopt` / `repo health` would blame the path.
func TestGitInventoryReportsUnrunnableGit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "repo")
	runner := &fakeRunner{
		errors: map[string]error{
			probeKey(path): commandFailure(
				path,
				"",
				&exec.Error{Name: "git", Err: exec.ErrNotFound},
			),
		},
	}

	inventory, err := NewClient(runner).Inventory(
		context.Background(),
		path,
		"origin",
	)
	if err == nil {
		t.Fatalf(
			"Inventory reported %#v with no error; an unrunnable git must not "+
				"read as \"not a repository\"",
			inventory,
		)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Inventory error %v does not unwrap to exec.ErrNotFound", err)
	}
	if inventory.IsRepository {
		t.Fatal("Inventory claimed IsRepository on an unrunnable git")
	}
}

// TestGitInventoryTreatsNonRepositoryAsNotAnError is the other half: the probe
// failing because the path genuinely is not a work tree must stay a clean
// negative answer, not an error. Callers branch on IsRepository.
func TestGitInventoryTreatsNonRepositoryAsNotAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "plain")
	runner := &fakeRunner{
		errors: map[string]error{
			probeKey(path): commandFailure(
				path,
				"fatal: not a git repository (or any of the parent directories): .git",
				errors.New("exit status 128"),
			),
		},
	}

	inventory, err := NewClient(runner).Inventory(
		context.Background(),
		path,
		"origin",
	)
	if err != nil {
		t.Fatalf("Inventory on a non-repository path: %v", err)
	}
	if inventory.IsRepository {
		t.Fatal("Inventory claimed IsRepository for a non-repository path")
	}
	if inventory.Path != filepath.Clean(path) {
		t.Fatalf("Inventory.Path = %q, want %q", inventory.Path, path)
	}
}

// TestGitInventoryTreatsFalseProbeAsNotARepository covers the third shape: git
// ran fine and answered "false".
func TestGitInventoryTreatsFalseProbeAsNotARepository(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bare")
	runner := &fakeRunner{
		responses: map[string]RunResult{
			probeKey(path): {Stdout: "false\n"},
		},
	}

	inventory, err := NewClient(runner).Inventory(
		context.Background(),
		path,
		"origin",
	)
	if err != nil {
		t.Fatalf("Inventory on a false probe: %v", err)
	}
	if inventory.IsRepository {
		t.Fatal("Inventory claimed IsRepository after a false probe")
	}
}

// TestGitInventoryReportsCancelledContext keeps a cancelled probe from reading
// as a verdict about the path.
func TestGitInventoryReportsCancelledContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "repo")
	runner := &fakeRunner{
		errors: map[string]error{
			probeKey(path): commandFailure(path, "", context.Canceled),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inventory, err := NewClient(runner).Inventory(ctx, path, "origin")
	if err == nil {
		t.Fatalf(
			"Inventory reported %#v with no error for a cancelled probe",
			inventory,
		)
	}
	if !strings.Contains(err.Error(), "rev-parse") {
		t.Fatalf("Inventory error %q does not name the probe that failed", err)
	}
}

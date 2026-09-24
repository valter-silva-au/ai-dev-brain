package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestAddRejectsWorkspaceLockSymlinkBeforeMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		live     bool
		ancestor bool
	}{
		{name: "live", live: true},
		{name: "dangling"},
		{name: "symlinked ancestor", ancestor: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, _ := initializeRepositoryTestWorkspace(t)
			workspaceLayout, err := workspace.NewLayout(root)
			if err != nil {
				t.Fatalf("new workspace layout: %v", err)
			}
			lockPath := filepath.Join(
				workspaceLayout.ControlDir(),
				"workspace.lock",
			)

			external := t.TempDir()
			if test.ancestor {
				externalControl := filepath.Join(external, "control")
				if err := os.Rename(
					workspaceLayout.ControlDir(),
					externalControl,
				); err != nil {
					t.Fatalf("move workspace control directory: %v", err)
				}
				if err := os.Symlink(
					externalControl,
					workspaceLayout.ControlDir(),
				); err != nil {
					t.Fatalf("redirect workspace control directory: %v", err)
				}
			} else {
				if err := os.Remove(lockPath); err != nil {
					t.Fatalf("remove workspace lock: %v", err)
				}
				externalLock := filepath.Join(external, "workspace.lock")
				if test.live {
					if err := os.WriteFile(
						externalLock,
						[]byte("outside\n"),
						0o644,
					); err != nil {
						t.Fatalf("write external lock: %v", err)
					}
				}
				if err := os.Symlink(externalLock, lockPath); err != nil {
					t.Fatalf("symlink workspace lock: %v", err)
				}
			}

			beforeExternal := snapshotRepositoryWorkspace(t, external)
			beforeEvents := snapshotRepositoryEvents(t, root)
			git := &fakeGit{}
			service := newTestRepositoryService(t, git, nil)
			_, err = service.Add(
				context.Background(),
				AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeCloneNew,
					Remote: "https://github.com/valter-silva-au/" +
						"ai-dev-brain.git",
					ActorType: "human",
					Tool:      "adb-cli",
					Apply:     true,
				},
			)
			assertRepositoryWorkspaceLockViolation(t, err)

			if after := snapshotRepositoryWorkspace(
				t,
				external,
			); !reflect.DeepEqual(after, beforeExternal) {
				t.Fatalf(
					"external lock target changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			if after := snapshotRepositoryEvents(t, root); !reflect.DeepEqual(
				after,
				beforeEvents,
			) {
				t.Fatalf(
					"repository journal changed\n got: %#v\nwant: %#v",
					after,
					beforeEvents,
				)
			}
			assertNoRepositoryGitCalls(t, git)
			layout := repositoryTestLayout(t, root)
			if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("repository apply created managed content: %v", err)
			}
		})
	}
}

func assertRepositoryWorkspaceLockViolation(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("operation accepted symlinked workspace lock")
	}
	var violation *workspace.RoleViolation
	if !errors.As(err, &violation) {
		t.Fatalf("error = %v, want workspace role violation", err)
	}
	if violation.Reason != workspace.RoleViolationSymlink {
		t.Fatalf("violation reason = %q, want symlink", violation.Reason)
	}
}

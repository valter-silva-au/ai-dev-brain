package organization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestInitializeRejectsWorkspaceLockSymlinkBeforeMutation(t *testing.T) {
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

			root := initializeTestWorkspace(t)
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

			beforeExternal := snapshotOrganizationPath(t, external)
			beforeEvents := snapshotOrganizationPath(
				t,
				workspaceLayout.EventsDir(),
			)

			service := newTestOrganizationService(t, nil)
			_, err = service.Initialize(
				context.Background(),
				InitializeRequest{
					WorkspaceRoot: root,
					Slug:          "amazon",
					Name:          "Amazon",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				},
			)
			assertWorkspaceLockViolation(t, err)

			if after := snapshotOrganizationPath(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"external lock target changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			if after := snapshotOrganizationPath(
				t,
				workspaceLayout.EventsDir(),
			); !reflect.DeepEqual(after, beforeEvents) {
				t.Fatalf(
					"organization journal changed\n got: %#v\nwant: %#v",
					after,
					beforeEvents,
				)
			}
			layout, err := NewLayout(root, "amazon")
			if err != nil {
				t.Fatalf("new organization layout: %v", err)
			}
			if _, err := os.Stat(layout.Root()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("organization apply created managed content: %v", err)
			}
		})
	}
}

func assertWorkspaceLockViolation(t *testing.T, err error) {
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

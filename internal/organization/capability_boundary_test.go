package organization

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestInitializeRejectsCanonicalOrganizationSymlinksBeforePlanning(
	t *testing.T,
) {
	t.Parallel()

	for _, link := range []struct {
		name string
		live bool
	}{
		{name: "live", live: true},
		{name: "dangling"},
	} {
		link := link
		for _, apply := range []bool{false, true} {
			apply := apply
			t.Run(fmt.Sprintf("%s/apply=%t", link.name, apply), func(t *testing.T) {
				t.Parallel()

				root := initializeTestWorkspace(t)
				layout, err := NewLayout(root, "amazon")
				if err != nil {
					t.Fatalf("new organization layout: %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(layout.Root()), 0o755); err != nil {
					t.Fatalf("create organization parent: %v", err)
				}
				external := filepath.Join(t.TempDir(), "external-organization")
				var beforeExternal map[string]string
				if link.live {
					if err := os.MkdirAll(external, 0o755); err != nil {
						t.Fatalf("create live symlink target: %v", err)
					}
					if err := os.WriteFile(
						filepath.Join(external, "boundary-marker"),
						[]byte("outside\n"),
						0o644,
					); err != nil {
						t.Fatalf("write boundary marker: %v", err)
					}
					beforeExternal = snapshotOrganizationPath(t, external)
				}
				if err := os.Symlink(external, layout.Root()); err != nil {
					t.Fatalf("create canonical organization symlink: %v", err)
				}
				workspaceLayout, err := workspace.NewLayout(root)
				if err != nil {
					t.Fatalf("new workspace layout: %v", err)
				}
				beforeEvents := snapshotOrganizationPath(
					t,
					workspaceLayout.EventsDir(),
				)

				service := newTestOrganizationService(t, nil)
				result, err := service.Initialize(
					context.Background(),
					InitializeRequest{
						WorkspaceRoot: root,
						Slug:          "amazon",
						Name:          "Amazon",
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         apply,
					},
				)
				assertOrganizationsRoleViolation(t, err)
				assertOrganizationBoundaryOutcome(t, result.Outcome)
				assertOrganizationPathUnchanged(
					t,
					workspaceLayout.EventsDir(),
					beforeEvents,
				)
				assertOrganizationSymlinkTargetUnchanged(
					t,
					external,
					link.live,
					beforeExternal,
				)
			})
		}
	}
}

func TestOrganizationMoveRejectsDestinationSymlinksBeforePlanning(
	t *testing.T,
) {
	t.Parallel()

	for _, link := range []struct {
		name string
		live bool
	}{
		{name: "live", live: true},
		{name: "dangling"},
	} {
		link := link
		for _, apply := range []bool{false, true} {
			apply := apply
			t.Run(fmt.Sprintf("%s/apply=%t", link.name, apply), func(t *testing.T) {
				t.Parallel()

				root := initializeTestWorkspace(t)
				service := newTestOrganizationService(t, nil)
				if _, err := service.Initialize(
					context.Background(),
					InitializeRequest{
						WorkspaceRoot: root,
						Slug:          "amazon",
						Name:          "Amazon",
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         true,
					},
				); err != nil {
					t.Fatalf("seed organization: %v", err)
				}
				destination, err := NewLayout(root, "aws")
				if err != nil {
					t.Fatalf("new destination layout: %v", err)
				}
				external := filepath.Join(t.TempDir(), "external-destination")
				beforeExternal := prepareOrganizationSymlinkTarget(
					t,
					external,
					link.live,
				)
				if err := os.Symlink(external, destination.Root()); err != nil {
					t.Fatalf("create destination symlink: %v", err)
				}
				workspaceLayout, err := workspace.NewLayout(root)
				if err != nil {
					t.Fatalf("new workspace layout: %v", err)
				}
				beforeEvents := snapshotOrganizationPath(
					t,
					workspaceLayout.EventsDir(),
				)

				result, err := service.Move(
					context.Background(),
					MoveRequest{
						WorkspaceRoot: root,
						Selector:      "amazon",
						NewSlug:       "aws",
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         apply,
					},
				)
				assertOrganizationsRoleViolation(t, err)
				assertOrganizationBoundaryOutcome(t, result.Outcome)
				assertOrganizationPathUnchanged(
					t,
					workspaceLayout.EventsDir(),
					beforeEvents,
				)
				assertOrganizationSymlinkTargetUnchanged(
					t,
					external,
					link.live,
					beforeExternal,
				)
			})
		}
	}
}

func TestManagedOrganizationRejectsSymlinkedCanonicalSourceBeforePlanning(
	t *testing.T,
) {
	t.Parallel()

	for _, apply := range []bool{false, true} {
		apply := apply
		t.Run(fmt.Sprintf("apply=%t", apply), func(t *testing.T) {
			t.Parallel()

			root, layout := seedOrganizationForRetargetTest(t)
			external := filepath.Join(t.TempDir(), "external-organization")
			if err := os.Rename(layout.Root(), external); err != nil {
				t.Fatalf("move organization outside workspace: %v", err)
			}
			if err := os.Symlink(external, layout.Root()); err != nil {
				t.Fatalf("redirect canonical organization: %v", err)
			}
			workspaceLayout, err := workspace.NewLayout(root)
			if err != nil {
				t.Fatalf("new workspace layout: %v", err)
			}
			beforeExternal := snapshotOrganizationPath(t, external)
			beforeEvents := snapshotOrganizationPath(
				t,
				workspaceLayout.EventsDir(),
			)

			service := newTestOrganizationService(t, nil)
			result, err := service.Update(
				context.Background(),
				UpdateRequest{
					WorkspaceRoot: root,
					Selector:      "amazon",
					Name:          stringPointer("Amazon Web Services"),
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         apply,
				},
			)
			assertOrganizationsRoleViolation(t, err)
			assertOrganizationBoundaryOutcome(t, result.Outcome)
			assertOrganizationPathUnchanged(t, external, beforeExternal)
			assertOrganizationPathUnchanged(
				t,
				workspaceLayout.EventsDir(),
				beforeEvents,
			)
		})
	}
}

func TestOrganizationAdoptRejectsDestinationSymlinksBeforePlanning(
	t *testing.T,
) {
	t.Parallel()

	for _, link := range []struct {
		name string
		live bool
	}{
		{name: "live", live: true},
		{name: "dangling"},
	} {
		link := link
		for _, apply := range []bool{false, true} {
			apply := apply
			t.Run(fmt.Sprintf("%s/apply=%t", link.name, apply), func(t *testing.T) {
				t.Parallel()

				root := initializeTestWorkspace(t)
				source := filepath.Join(root, "imports", "existing-amazon")
				if err := os.MkdirAll(source, 0o755); err != nil {
					t.Fatalf("create adoption source: %v", err)
				}
				if err := os.WriteFile(
					filepath.Join(source, "keep.txt"),
					[]byte("authored"),
					0o644,
				); err != nil {
					t.Fatalf("write adoption source: %v", err)
				}
				destination, err := NewLayout(root, "amazon")
				if err != nil {
					t.Fatalf("new destination layout: %v", err)
				}
				external := filepath.Join(t.TempDir(), "external-destination")
				beforeExternal := prepareOrganizationSymlinkTarget(
					t,
					external,
					link.live,
				)
				if err := os.Symlink(external, destination.Root()); err != nil {
					t.Fatalf("create adoption destination symlink: %v", err)
				}
				workspaceLayout, err := workspace.NewLayout(root)
				if err != nil {
					t.Fatalf("new workspace layout: %v", err)
				}
				beforeSource := snapshotOrganizationPath(t, source)
				beforeEvents := snapshotOrganizationPath(
					t,
					workspaceLayout.EventsDir(),
				)

				service := newTestOrganizationService(t, nil)
				result, err := service.Adopt(
					context.Background(),
					AdoptRequest{
						WorkspaceRoot: root,
						Path:          source,
						Slug:          "amazon",
						Name:          "Amazon",
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         apply,
					},
				)
				assertOrganizationsRoleViolation(t, err)
				assertOrganizationBoundaryOutcome(t, result.Outcome)
				assertOrganizationPathUnchanged(t, source, beforeSource)
				assertOrganizationPathUnchanged(
					t,
					workspaceLayout.EventsDir(),
					beforeEvents,
				)
				assertOrganizationSymlinkTargetUnchanged(
					t,
					external,
					link.live,
					beforeExternal,
				)
			})
		}
	}
}

func prepareOrganizationSymlinkTarget(
	t *testing.T,
	path string,
	live bool,
) map[string]string {
	t.Helper()

	if !live {
		return nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create live symlink target: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(path, "boundary-marker"),
		[]byte("outside\n"),
		0o644,
	); err != nil {
		t.Fatalf("write boundary marker: %v", err)
	}
	return snapshotOrganizationPath(t, path)
}

func assertOrganizationBoundaryOutcome(
	t *testing.T,
	outcome capability.Outcome,
) {
	t.Helper()

	if outcome == capability.OutcomePlanned ||
		outcome == capability.OutcomeApplied {
		t.Fatalf("boundary violation outcome = %q", outcome)
	}
}

func assertOrganizationSymlinkTargetUnchanged(
	t *testing.T,
	path string,
	live bool,
	before map[string]string,
) {
	t.Helper()

	if live {
		assertOrganizationPathUnchanged(t, path, before)
		return
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dangling symlink target changed: %v", err)
	}
}

package ticket

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestScopeLayoutsUseOwningManifestTicketRoles(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	organizationManifest.Roles.Tickets = "work-items"

	organizationScope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization ticket scope: %v", err)
	}
	assertTicketLayout(
		t,
		organizationScope,
		filepath.Join(organizationLayout.Root(), "work-items"),
	)

	repositoryLayout, repositoryManifest := testRepository(
		t,
		organizationLayout,
		organizationManifest,
	)
	repositoryManifest.Roles.Tickets = "portable-tickets"

	repositoryScope, err := NewRepositoryScope(
		organizationLayout,
		organizationManifest,
		repositoryLayout,
		repositoryManifest,
	)
	if err != nil {
		t.Fatalf("new repository ticket scope: %v", err)
	}
	assertTicketLayout(
		t,
		repositoryScope,
		filepath.Join(repositoryLayout.Root(), "portable-tickets"),
	)
}

func TestRepositoryScopeRejectsCrossTrustOwnership(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	repositoryLayout, repositoryManifest := testRepository(
		t,
		organizationLayout,
		organizationManifest,
	)
	repositoryManifest.OrganizationID = "another-organization"

	if _, err := NewRepositoryScope(
		organizationLayout,
		organizationManifest,
		repositoryLayout,
		repositoryManifest,
	); err == nil {
		t.Fatal("cross-organization repository ticket scope was accepted")
	}
}

func TestTicketLayoutRejectsUnsafeIdentityComponents(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	scope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization ticket scope: %v", err)
	}

	for _, testCase := range []struct {
		key  string
		slug string
	}{
		{key: "../TASK-00001", slug: "safe"},
		{key: "task-00001", slug: "safe"},
		{key: "TASK-00001", slug: "../escape"},
		{key: "TASK-00001", slug: "CON"},
		{key: "TASK-00001", slug: "trailing."},
	} {
		if _, err := scope.Ticket(testCase.key, testCase.slug); err == nil {
			t.Fatalf(
				"unsafe ticket identity key=%q slug=%q was accepted",
				testCase.key,
				testCase.slug,
			)
		}
	}
}

func TestScopeRejectsResolvedTicketRoleSymlinkEscape(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	organizationManifest.Roles.Tickets = "work-items"
	if err := os.MkdirAll(organizationLayout.Root(), 0o755); err != nil {
		t.Fatalf("create organization root: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(
		outside,
		filepath.Join(organizationLayout.Root(), "work-items"),
	); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	); err == nil {
		t.Fatal("ticket role symlink escaping its owner was accepted")
	}
}

func TestScopeRejectsArchivedOwners(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	archivedAt := organizationManifest.UpdatedAt
	organizationManifest.Status = organization.StatusArchived
	organizationManifest.ArchivedAt = &archivedAt
	if _, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	); err == nil {
		t.Fatal("archived organization was accepted for ticket mutation")
	}
}

func TestScopeRejectsOwnerRootsResolvedOutsideTheirParentTrust(t *testing.T) {
	t.Parallel()

	t.Run("organization root", func(t *testing.T) {
		workspaceRoot := t.TempDir()
		organizationsRoot := filepath.Join(workspaceRoot, "organizations")
		if err := os.MkdirAll(organizationsRoot, 0o755); err != nil {
			t.Fatalf("create organizations root: %v", err)
		}
		if err := os.Symlink(
			t.TempDir(),
			filepath.Join(organizationsRoot, "amazon"),
		); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		layout, err := organization.NewLayout(workspaceRoot, "amazon")
		if err != nil {
			t.Fatalf("new organization layout: %v", err)
		}
		manifest := organization.NewManifest(
			"organization-1",
			"amazon",
			"Amazon",
			time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
			organization.Provenance{
				OperationID: "operation-1",
				ActorType:   "human",
				Tool:        "adb-cli",
			},
		)
		if _, err := NewOrganizationScope(layout, manifest); err == nil {
			t.Fatal("organization root symlink escape was accepted")
		}
	})

	t.Run("repository root", func(t *testing.T) {
		organizationLayout, organizationManifest := testOrganization(t)
		layout, err := repository.NewLayout(
			organizationLayout,
			organizationManifest.Roles.Repositories,
			"github.com",
			"valter-silva-au",
			"escaped",
		)
		if err != nil {
			t.Fatalf("new repository layout: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(layout.Root()), 0o755); err != nil {
			t.Fatalf("create repository parent: %v", err)
		}
		if err := os.Symlink(t.TempDir(), layout.Root()); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		manifest := repository.NewManifest(
			"repository-escaped",
			organizationManifest.ID,
			layout,
			repository.Remote{
				Name:     "origin",
				Type:     repository.RemoteTypeCanonical,
				FetchURL: "https://github.com/valter-silva-au/escaped.git",
			},
			time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
			repository.Provenance{
				OperationID: "operation-2",
				ActorType:   "human",
				Tool:        "adb-cli",
			},
		)
		if _, err := NewRepositoryScope(
			organizationLayout,
			organizationManifest,
			layout,
			manifest,
		); err == nil {
			t.Fatal("repository root symlink escape was accepted")
		}
	})
}

func TestScopeRechecksContainmentBeforeManifestScanning(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	scope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization scope: %v", err)
	}
	if err := os.MkdirAll(scope.TicketsRoot(), 0o755); err != nil {
		t.Fatalf("create tickets root: %v", err)
	}
	if err := os.Remove(scope.TicketsRoot()); err != nil {
		t.Fatalf("remove tickets root: %v", err)
	}
	if err := os.Symlink(t.TempDir(), scope.TicketsRoot()); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ReadScopeManifests(scope); err == nil {
		t.Fatal("ticket role symlink replacement was accepted")
	}
}

func TestScopeRootedScanRejectsSymlinkSwapAfterContainmentCheck(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	scope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization scope: %v", err)
	}
	if err := os.MkdirAll(scope.TicketsRoot(), 0o755); err != nil {
		t.Fatalf("create tickets root: %v", err)
	}
	outside := t.TempDir()

	_, err = readScopeManifests(scope, func() error {
		if err := os.Remove(scope.TicketsRoot()); err != nil {
			return err
		}
		return os.Symlink(outside, scope.TicketsRoot())
	})
	if err == nil {
		t.Fatal("post-containment ticket-role symlink swap was accepted")
	}
}

func assertTicketLayout(
	t *testing.T,
	scope ScopeLayout,
	wantTicketsRoot string,
) {
	t.Helper()

	layout, err := scope.Ticket("TASK-00039", "design-ai-dev-brain-v3")
	if err != nil {
		t.Fatalf("new ticket layout: %v", err)
	}
	if scope.TicketsRoot() != wantTicketsRoot {
		t.Fatalf(
			"tickets root = %q, want %q",
			scope.TicketsRoot(),
			wantTicketsRoot,
		)
	}
	wantRoot := filepath.Join(
		wantTicketsRoot,
		"TASK-00039-design-ai-dev-brain-v3",
	)
	if layout.Root() != wantRoot {
		t.Fatalf("ticket root = %q, want %q", layout.Root(), wantRoot)
	}
	if layout.StatusPath() != filepath.Join(wantRoot, "status.yaml") {
		t.Fatalf("status path = %q", layout.StatusPath())
	}
	if layout.RelativePath() != "TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf("relative path = %q", layout.RelativePath())
	}
	if layout.Scope() != scope {
		t.Fatal("ticket layout did not preserve its owning scope")
	}
}

func testOrganization(
	t *testing.T,
) (organization.Layout, organization.Manifest) {
	t.Helper()

	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	writeWorkspaceManifest(t, workspaceRoot, workspace.DefaultRoles())
	layout, err := organization.NewLayout(
		workspaceRoot,
		"amazon",
	)
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	manifest := organization.NewManifest(
		"organization-1",
		"amazon",
		"Amazon",
		time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		organization.Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create organization control directory: %v", err)
	}
	var encoded bytes.Buffer
	if err := organization.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode organization manifest: %v", err)
	}
	if err := os.WriteFile(
		layout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write organization manifest: %v", err)
	}
	return layout, manifest
}

func writeWorkspaceManifest(
	t *testing.T,
	root string,
	roles workspace.Roles,
) workspace.Manifest {
	t.Helper()

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	manifest := workspace.NewManifest(
		"workspace-1",
		"Test workspace",
		time.Date(2026, time.September, 10, 11, 0, 0, 0, time.UTC),
	)
	manifest.Roles = roles
	if err := manifest.Validate(layout); err != nil {
		t.Fatalf("validate workspace manifest: %v", err)
	}
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create workspace control directory: %v", err)
	}
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode workspace manifest: %v", err)
	}
	if err := os.WriteFile(
		layout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write workspace manifest: %v", err)
	}
	return manifest
}

func testRepository(
	t *testing.T,
	organizationLayout organization.Layout,
	organizationManifest organization.Manifest,
) (repository.Layout, repository.Manifest) {
	t.Helper()

	layout, err := repository.NewLayout(
		organizationLayout,
		organizationManifest.Roles.Repositories,
		"github.com",
		"valter-silva-au",
		"ai-dev-brain",
	)
	if err != nil {
		t.Fatalf("new repository layout: %v", err)
	}
	manifest := repository.NewManifest(
		"repository-1",
		organizationManifest.ID,
		layout,
		repository.Remote{
			Name:     "origin",
			Type:     repository.RemoteTypeCanonical,
			FetchURL: "https://github.com/valter-silva-au/ai-dev-brain.git",
		},
		time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		repository.Provenance{
			OperationID: "operation-2",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create repository control directory: %v", err)
	}
	var encoded bytes.Buffer
	if err := repository.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode repository manifest: %v", err)
	}
	if err := os.WriteFile(
		layout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write repository manifest: %v", err)
	}
	return layout, manifest
}

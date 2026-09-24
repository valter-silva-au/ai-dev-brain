package ticket

import (
	"bytes"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestServiceAllocatesFromPortableManifestsWhileHoldingWorkspaceLock(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	existingLayout, err := scope.Ticket(
		"TASK-00003",
		"existing-ticket",
	)
	if err != nil {
		t.Fatalf("existing ticket layout: %v", err)
	}
	existing := validManifest(t, scope, activeProfile)
	existing.LocalKey = "TASK-00003"
	existing.VisibleKey = "TASK-00003"
	existing.PathSlug = "existing-ticket"
	existing.Branch.Slug = "existing-ticket"
	existing.Branch.Intent, err = BranchIntent(
		existing.Type,
		existing.LocalKey,
		existing.Branch.Slug,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("existing branch intent: %v", err)
	}
	if err := os.MkdirAll(existingLayout.Root(), 0o755); err != nil {
		t.Fatalf("create existing ticket root: %v", err)
	}
	status, err := encodeManifest(existing)
	if err != nil {
		t.Fatalf("encode existing manifest: %v", err)
	}
	if err := os.WriteFile(existingLayout.StatusPath(), status, 0o644); err != nil {
		t.Fatalf("write existing status: %v", err)
	}

	// A derived SQLite file containing a higher-looking number is deliberately
	// irrelevant to portable key allocation.
	if err := os.MkdirAll(
		filepath.Join(scope.WorkspaceRoot(), ".aidb"),
		0o755,
	); err != nil {
		t.Fatalf("create workspace control directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(scope.WorkspaceRoot(), ".aidb", "state.sqlite"),
		[]byte("TASK-99999"),
		0o644,
	); err != nil {
		t.Fatalf("write fake derived state: %v", err)
	}

	service := NewService()
	called := false
	key, err := service.WithAllocatedLocalKey(
		scope,
		KeyPolicy{Prefix: "TASK", Width: 5},
		func(
			allocated string,
			existing []LocatedManifest,
		) error {
			called = true
			if len(existing) != 1 {
				t.Fatalf("workspace snapshot = %#v, want one ticket", existing)
			}
			if _, err := os.Stat(
				filepath.Join(scope.WorkspaceRoot(), ".aidb", "workspace.lock"),
			); err != nil {
				t.Fatalf("workspace lock does not exist in reservation: %v", err)
			}
			if allocated != "TASK-00004" {
				t.Fatalf("allocated key in reservation = %q", allocated)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("allocate local key: %v", err)
	}
	if !called {
		t.Fatal("reservation callback was not called")
	}
	if key != "TASK-00004" {
		t.Fatalf("allocated key = %q, want TASK-00004", key)
	}
}

func TestValidateAvailableRejectsKeyPathAndAliasCollisions(t *testing.T) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	existingLayout, err := scope.Ticket("TASK-00003", "existing")
	if err != nil {
		t.Fatalf("existing layout: %v", err)
	}
	existing := validManifest(t, scope, activeProfile)
	existing.ID = "550e8400-e29b-41d4-a716-446655440003"
	existing.LocalKey = "TASK-00003"
	existing.VisibleKey = "TASK-00003"
	existing.PathSlug = "existing"
	existing.Branch.Slug = "existing"
	existing.Aliases = []string{"TASK-00002", "TASK-00001-old"}
	existing.Branch.Intent, err = BranchIntent(
		existing.Type,
		existing.LocalKey,
		existing.Branch.Slug,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("existing branch intent: %v", err)
	}
	located := []LocatedManifest{{
		Layout:   existingLayout,
		Manifest: existing,
	}}

	tests := []struct {
		key  string
		slug string
	}{
		{key: "TASK-00003", slug: "new"},
		{key: "TASK-00002", slug: "new"},
		{key: "TASK-00001", slug: "old"},
	}
	for _, testCase := range tests {
		layout, err := scope.Ticket(testCase.key, testCase.slug)
		if err != nil {
			t.Fatalf("candidate layout: %v", err)
		}
		candidate := validManifest(t, scope, activeProfile)
		candidate.ID = "550e8400-e29b-41d4-a716-446655440004"
		candidate.LocalKey = testCase.key
		candidate.VisibleKey = testCase.key
		candidate.PathSlug = testCase.slug
		candidate.Branch.Slug = testCase.slug
		candidate.Branch.Intent, err = BranchIntent(
			candidate.Type,
			candidate.LocalKey,
			candidate.Branch.Slug,
			activeProfile,
		)
		if err != nil {
			t.Fatalf("candidate branch intent: %v", err)
		}
		if err := ValidateAvailable(
			layout,
			candidate,
			located,
		); err == nil {
			t.Fatalf(
				"collision key=%q slug=%q was accepted",
				testCase.key,
				testCase.slug,
			)
		}
	}
}

func TestServiceSerializesReservationCallbacks(t *testing.T) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	service := NewService()
	secondService := NewService()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var callbacks atomic.Int32

	go func() {
		_, err := service.WithAllocatedLocalKey(
			scope,
			KeyPolicy{Prefix: "TASK", Width: 5},
			func(string, []LocatedManifest) error {
				callbacks.Add(1)
				close(firstEntered)
				<-releaseFirst
				return nil
			},
		)
		firstDone <- err
	}()
	<-firstEntered

	go func() {
		_, err := secondService.WithAllocatedLocalKey(
			scope,
			KeyPolicy{Prefix: "TASK", Width: 5},
			func(string, []LocatedManifest) error {
				callbacks.Add(1)
				return nil
			},
		)
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second reservation completed while first held lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if callbacks.Load() != 1 {
		t.Fatalf("callbacks entered concurrently: %d", callbacks.Load())
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second reservation: %v", err)
	}
}

func TestBuildProjectionRetainsStableLocalKeyAsAlias(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	manifest := validManifest(t, scope, activeProfile)
	manifest.VisibleKey = "github:valter-silva-au/ai-dev-brain#39"
	manifest.RemoteReferences = []RemoteReference{{
		Provider: "github",
		Resource: "issue",
		ID:       "valter-silva-au/ai-dev-brain#39",
		Primary:  true,
	}}
	observedAt := time.Date(
		2026,
		time.September,
		10,
		14,
		0,
		0,
		0,
		time.UTC,
	)

	projection, err := BuildProjection(
		manifest,
		layout,
		activeProfile,
		"manifest-sha256",
		observedAt,
	)
	if err != nil {
		t.Fatalf("build ticket projection: %v", err)
	}
	if projection.VisibleKey != manifest.VisibleKey ||
		projection.Path != layout.Root() ||
		!contains(projection.Aliases, manifest.LocalKey) {
		t.Fatalf("projection lost portable identity: %#v", projection)
	}
}

func TestValidateAvailableAllowsSameLocalKeyInDifferentExactScope(t *testing.T) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	otherScope := scope
	otherScope.kind = ScopeRepository
	otherScope.repositoryID = "repository-2"
	otherScope.trustRoot = filepath.Dir(otherScope.ownerRoot)
	otherScope.ownerRoot = filepath.Join(t.TempDir(), "repository-2")
	otherScope.trustRoot = filepath.Dir(otherScope.ownerRoot)
	otherScope.ticketsRoot = filepath.Join(otherScope.ownerRoot, "tickets")

	existingLayout, err := otherScope.Ticket(
		"TASK-00039",
		"other-scope-ticket",
	)
	if err != nil {
		t.Fatalf("other-scope layout: %v", err)
	}
	existing := validManifest(t, otherScope, activeProfile)
	existing.ID = "550e8400-e29b-41d4-a716-446655440099"
	existing.PathSlug = "other-scope-ticket"
	existing.Branch.Slug = "other-scope-ticket"
	existing.Branch.Intent, err = BranchIntent(
		existing.Type,
		existing.LocalKey,
		existing.Branch.Slug,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("other-scope branch: %v", err)
	}

	candidateLayout, err := scope.Ticket(
		"TASK-00039",
		"design-ai-dev-brain-v3",
	)
	if err != nil {
		t.Fatalf("candidate layout: %v", err)
	}
	candidate := validManifest(t, scope, activeProfile)
	located := []LocatedManifest{{
		Layout:   existingLayout,
		Manifest: existing,
	}}
	if err := ValidateAvailable(
		candidateLayout,
		candidate,
		located,
	); err != nil {
		t.Fatalf("same local key in a different scope was rejected: %v", err)
	}

	candidate.ID = existing.ID
	if err := ValidateAvailable(
		candidateLayout,
		candidate,
		located,
	); err == nil {
		t.Fatal("workspace-global immutable id collision was accepted")
	}
}

func TestReadScopeManifestsRejectsOrphansAndSymlinks(t *testing.T) {
	t.Parallel()

	t.Run("orphan directory", func(t *testing.T) {
		scope, _ := testOrganizationTicketLayout(t)
		orphan := filepath.Join(
			scope.TicketsRoot(),
			"TASK-00001-orphan",
		)
		if err := os.MkdirAll(orphan, 0o755); err != nil {
			t.Fatalf("create orphan ticket: %v", err)
		}
		if _, err := ReadScopeManifests(scope); err == nil {
			t.Fatal("ticket directory without status.yaml was accepted")
		}
	})

	t.Run("symlink entry", func(t *testing.T) {
		scope, _ := testOrganizationTicketLayout(t)
		if err := os.MkdirAll(scope.TicketsRoot(), 0o755); err != nil {
			t.Fatalf("create tickets root: %v", err)
		}
		if err := os.Symlink(
			t.TempDir(),
			filepath.Join(scope.TicketsRoot(), "TASK-00001-linked"),
		); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := ReadScopeManifests(scope); err == nil {
			t.Fatal("symlinked ticket entry was accepted")
		}
	})
}

func TestReadScopeManifestsRejectsInvalidLifecycleMetadata(t *testing.T) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	layout, err := scope.Ticket("TASK-00001", "invalid-status")
	if err != nil {
		t.Fatalf("ticket layout: %v", err)
	}
	manifest, err := NewManifest(
		"550e8400-e29b-41d4-a716-446655440011",
		scope,
		"TASK-00001",
		"Invalid status",
		"invalid-status",
		"invalid-status",
		"feat",
		activeProfile,
		time.Date(2026, time.September, 10, 15, 0, 0, 0, time.UTC),
		Provenance{
			OperationID: "operation-invalid",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	manifest.Status = "closed"
	writeTicketManifest(t, layout, manifest)

	if _, err := ReadScopeManifests(scope); err == nil {
		t.Fatal("identity-valid manifest with invalid status was accepted")
	}
}

func TestReadScopeManifestsRejectsDefaultProfileSemanticDrift(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "undeclared work type",
			mutate: func(manifest *Manifest) {
				manifest.Type = "prototype"
				manifest.Branch.Intent = "prototype/" +
					manifest.LocalKey + "-" + manifest.Branch.Slug
			},
		},
		{
			name: "profile-incorrect spike branch",
			mutate: func(manifest *Manifest) {
				manifest.Type = "spike"
				manifest.Branch.Intent = "spike/" +
					manifest.LocalKey + "-" + manifest.Branch.Slug
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			scope, layout := testOrganizationTicketLayout(t)
			manifest := validManifest(t, scope, testBuiltinProfile(t))
			testCase.mutate(&manifest)
			writeTicketManifest(t, layout, manifest)

			if _, err := ReadScopeManifests(scope); err == nil {
				t.Fatal("default-profile semantic drift was accepted")
			}
		})
	}
}

func TestReadWorkspaceManifestsUsesRegisteredOrganizationsRole(t *testing.T) {
	t.Parallel()

	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	roles := workspace.DefaultRoles()
	roles.Organizations = "orgs"
	writeWorkspaceManifest(t, workspaceRoot, roles)

	activeProfile := testBuiltinProfile(t)
	const sharedID = "550e8400-e29b-41d4-a716-446655440088"
	for index, slug := range []string{"amazon", "aws"} {
		layout, err := organization.NewLayoutForRole(
			workspaceRoot,
			roles.Organizations,
			slug,
		)
		if err != nil {
			t.Fatalf("new organization layout: %v", err)
		}
		manifest := organization.NewManifest(
			"organization-"+slug,
			slug,
			slug,
			time.Date(
				2026,
				time.September,
				10,
				12+index,
				0,
				0,
				0,
				time.UTC,
			),
			organization.Provenance{
				OperationID: "operation-" + slug,
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
		scope, err := NewOrganizationScope(layout, manifest)
		if err != nil {
			t.Fatalf("new organization scope: %v", err)
		}
		ticketLayout, err := scope.Ticket(
			"TASK-00001",
			"duplicate-id-"+slug,
		)
		if err != nil {
			t.Fatalf("ticket layout: %v", err)
		}
		ticketManifest, err := NewManifest(
			sharedID,
			scope,
			"TASK-00001",
			"Duplicate ID "+slug,
			"duplicate-id-"+slug,
			"duplicate-id-"+slug,
			"feat",
			activeProfile,
			time.Date(
				2026,
				time.September,
				10,
				14+index,
				0,
				0,
				0,
				time.UTC,
			),
			Provenance{
				OperationID: "ticket-" + slug,
				ActorType:   "human",
				Tool:        "adb-cli",
			},
		)
		if err != nil {
			t.Fatalf("new ticket manifest: %v", err)
		}
		writeTicketManifest(t, ticketLayout, ticketManifest)
	}

	if _, err := ReadWorkspaceManifests(workspaceRoot); err == nil {
		t.Fatal("duplicate ticket IDs beneath custom organizations role passed")
	}
}

func TestLockedAllocationProvidesWorkspaceWideCollisionSnapshot(t *testing.T) {
	t.Parallel()

	organizationLayout, organizationManifest := testOrganization(t)
	organizationScope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("organization scope: %v", err)
	}
	repositoryLayout, repositoryManifest := testRepository(
		t,
		organizationLayout,
		organizationManifest,
	)
	repositoryScope, err := NewRepositoryScope(
		organizationLayout,
		organizationManifest,
		repositoryLayout,
		repositoryManifest,
	)
	if err != nil {
		t.Fatalf("repository scope: %v", err)
	}
	activeProfile := testBuiltinProfile(t)
	repositoryTicketLayout, err := repositoryScope.Ticket(
		"TASK-00001",
		"repository-ticket",
	)
	if err != nil {
		t.Fatalf("repository ticket layout: %v", err)
	}
	const sharedID = "550e8400-e29b-41d4-a716-446655440012"
	repositoryTicket, err := NewManifest(
		sharedID,
		repositoryScope,
		"TASK-00001",
		"Repository ticket",
		"repository-ticket",
		"repository-ticket",
		"feat",
		activeProfile,
		time.Date(2026, time.September, 10, 15, 0, 0, 0, time.UTC),
		Provenance{
			OperationID: "repository-ticket",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("new repository ticket: %v", err)
	}
	writeTicketManifest(t, repositoryTicketLayout, repositoryTicket)

	service := NewService()
	_, err = service.WithAllocatedLocalKey(
		organizationScope,
		DefaultKeyPolicy(),
		func(
			key string,
			existing []LocatedManifest,
		) error {
			candidateLayout, layoutErr := organizationScope.Ticket(
				key,
				"organization-ticket",
			)
			if layoutErr != nil {
				return layoutErr
			}
			candidate, manifestErr := NewManifest(
				sharedID,
				organizationScope,
				key,
				"Organization ticket",
				"organization-ticket",
				"organization-ticket",
				"feat",
				activeProfile,
				time.Date(
					2026,
					time.September,
					10,
					15,
					1,
					0,
					0,
					time.UTC,
				),
				Provenance{
					OperationID: "organization-ticket",
					ActorType:   "human",
					Tool:        "adb-cli",
				},
			)
			if manifestErr != nil {
				return manifestErr
			}
			return ValidateAvailable(
				candidateLayout,
				candidate,
				existing,
			)
		},
	)
	if err == nil {
		t.Fatal("workspace-global immutable id collision was accepted")
	}
}

func writeTicketManifest(t *testing.T, layout Layout, manifest Manifest) {
	t.Helper()

	if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
		t.Fatalf("create ticket root: %v", err)
	}
	content, err := encodeManifest(manifest)
	if err != nil {
		t.Fatalf("encode ticket manifest: %v", err)
	}
	if err := os.WriteFile(layout.StatusPath(), content, 0o644); err != nil {
		t.Fatalf("write ticket manifest: %v", err)
	}
}

package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

// TestListWithoutOrganizationSpansEveryOrganization pins the workspace-wide
// inventory. `List` with an empty Organization is not an error to be rejected
// but the broader question — "what repositories does this workspace hold?" —
// which previously could not be asked at all: every caller had to name an
// organization it might not know yet.
//
// The per-organization behaviour is unchanged and covered by
// TestRepositoryListShowAndHealthAreDeterministicAndReadOnly; this test is
// specifically about crossing organization boundaries.
func TestListWithoutOrganizationSpansEveryOrganization(t *testing.T) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	addOrganization(t, root, "zeta-corp", "Zeta Corp")

	amazonLayout := repositoryTestLayout(t, root)
	zetaOrgLayout, err := organization.NewLayout(root, "zeta-corp")
	if err != nil {
		t.Fatalf("new zeta-corp organization layout: %v", err)
	}
	zetaLayout, err := NewLayout(
		zetaOrgLayout,
		"repos",
		"github.com",
		"valter-silva-au",
		"zeta",
	)
	if err != nil {
		t.Fatalf("new zeta repository layout: %v", err)
	}

	git := &fakeGit{
		inventories: map[string]Inventory{
			amazonLayout.CloneDir(): repositoryInventory(
				amazonLayout.CloneDir(),
				"origin",
				"https://github.com/valter-silva-au/ai-dev-brain.git",
			),
			zetaLayout.CloneDir(): repositoryInventory(
				zetaLayout.CloneDir(),
				"origin",
				"https://github.com/valter-silva-au/zeta.git",
			),
		},
	}
	service := newTestRepositoryService(t, git, nil)

	for _, seed := range []struct {
		organization string
		remote       string
	}{
		{
			organization: "amazon",
			remote:       "https://github.com/valter-silva-au/ai-dev-brain.git",
		},
		{
			organization: "zeta-corp",
			remote:       "https://github.com/valter-silva-au/zeta.git",
		},
	} {
		if _, err := service.Add(context.Background(), AddRequest{
			WorkspaceRoot: root,
			Organization:  seed.organization,
			Mode:          AddModeCloneNew,
			Remote:        seed.remote,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		}); err != nil {
			t.Fatalf("add repository to %s: %v", seed.organization, err)
		}
	}

	before := snapshotRepositoryWorkspace(t, root)
	listed, err := service.List(context.Background(), ListRequest{
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("list repositories workspace-wide: %v", err)
	}

	if len(listed.Data.Repositories) != 2 {
		t.Fatalf(
			"workspace-wide repositories = %#v, want 2",
			listed.Data.Repositories,
		)
	}
	// Ordering is by organization, then by the per-organization order the scoped
	// list already guarantees — so a workspace-wide read is reproducible.
	if listed.Data.Repositories[0].Name != "ai-dev-brain" ||
		listed.Data.Repositories[1].Name != "zeta" {
		t.Fatalf("repository order = %#v", listed.Data.Repositories)
	}
	// Every row must name its owner both ways: by immutable ID, and by the slug
	// --org accepts. A row identified only by a UUID is one a human can neither
	// recognize nor feed back to the CLI to narrow the list.
	if listed.Data.Repositories[0].OrganizationID ==
		listed.Data.Repositories[1].OrganizationID {
		t.Fatalf(
			"rows do not distinguish their organizations: %#v",
			listed.Data.Repositories,
		)
	}
	for want, index := range map[string]int{"amazon": 0, "zeta-corp": 1} {
		if got := listed.Data.Repositories[index].OrganizationSlug; got != want {
			t.Fatalf(
				"repository %q organization slug = %q, want %q",
				listed.Data.Repositories[index].Name,
				got,
				want,
			)
		}
	}
	// The envelope's own OrganizationID is the scope that was asked for. An
	// empty request scope must not claim one organization's id.
	if listed.Data.OrganizationID != "" {
		t.Fatalf(
			"workspace-wide list reported organization %q",
			listed.Data.OrganizationID,
		)
	}

	after := snapshotRepositoryWorkspace(t, root)
	if len(after) != len(before) {
		t.Fatalf("workspace-wide list mutated the workspace")
	}
	for path, digest := range before {
		if after[path] != digest {
			t.Fatalf("workspace-wide list changed %s", path)
		}
	}
}

// TestListWithoutOrganizationOnAnEmptyWorkspaceIsHealthy keeps "nothing here"
// distinct from "something failed". A fresh workspace has no organizations, and
// asking what it contains should answer "nothing", not error.
func TestListWithoutOrganizationOnAnEmptyWorkspaceIsHealthy(t *testing.T) {
	t.Parallel()

	root := initializeEmptyRepositoryTestWorkspace(t)
	service := newTestRepositoryService(t, &fakeGit{}, nil)

	listed, err := service.List(context.Background(), ListRequest{
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("list repositories on an empty workspace: %v", err)
	}
	if len(listed.Data.Repositories) != 0 {
		t.Fatalf("repositories = %#v, want none", listed.Data.Repositories)
	}
	if listed.Data.Repositories == nil {
		t.Fatal("repositories must marshal as [], not null")
	}
}

// TestListArchivedOrganizationsAreOmittedUnlessRequested keeps the
// workspace-wide read consistent with the scoped one: archive is a filter, and
// it applies to the organization tier too. An archived organization's
// repositories are not part of the ordinary inventory.
func TestListArchivedOrganizationsAreOmittedUnlessRequested(t *testing.T) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	amazonLayout := repositoryTestLayout(t, root)
	git := &fakeGit{
		inventories: map[string]Inventory{
			amazonLayout.CloneDir(): repositoryInventory(
				amazonLayout.CloneDir(),
				"origin",
				"https://github.com/valter-silva-au/ai-dev-brain.git",
			),
		},
	}
	service := newTestRepositoryService(t, git, nil)
	if _, err := service.Add(context.Background(), AddRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Mode:          AddModeCloneNew,
		Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}); err != nil {
		t.Fatalf("add repository: %v", err)
	}

	archiveOrganization(t, root, "amazon")

	listed, err := service.List(context.Background(), ListRequest{
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("list repositories workspace-wide: %v", err)
	}
	if len(listed.Data.Repositories) != 0 {
		t.Fatalf(
			"archived organization's repositories leaked into the default list: %#v",
			listed.Data.Repositories,
		)
	}

	included, err := service.List(context.Background(), ListRequest{
		WorkspaceRoot:   root,
		IncludeArchived: true,
	})
	if err != nil {
		t.Fatalf("list repositories including archived: %v", err)
	}
	if len(included.Data.Repositories) != 1 {
		t.Fatalf(
			"--include-archived did not reach the archived organization: %#v",
			included.Data.Repositories,
		)
	}
}

// initializeEmptyRepositoryTestWorkspace builds a workspace with NO
// organization, unlike initializeRepositoryTestWorkspace which seeds "amazon".
func initializeEmptyRepositoryTestWorkspace(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "AWS")
	service, err := foundation.NewService(foundation.Options{})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	if _, err := service.Initialize(
		context.Background(),
		foundation.InitializeRequest{
			Root:  root,
			Name:  "AWS",
			Apply: true,
		},
	); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	assertNoOrganizations(t, layout.StatePath())
	return root
}

func addOrganization(t *testing.T, root, slug, name string) string {
	t.Helper()

	service, err := organization.NewService(organization.Options{})
	if err != nil {
		t.Fatalf("new organization service: %v", err)
	}
	result, err := service.Initialize(
		context.Background(),
		organization.InitializeRequest{
			WorkspaceRoot: root,
			Slug:          slug,
			Name:          name,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize organization %s: %v", slug, err)
	}
	return result.Data.OrganizationID
}

func archiveOrganization(t *testing.T, root, selector string) {
	t.Helper()

	service, err := organization.NewService(organization.Options{})
	if err != nil {
		t.Fatalf("new organization service: %v", err)
	}
	if _, err := service.Archive(
		context.Background(),
		organization.ArchiveRequest{
			WorkspaceRoot: root,
			Selector:      selector,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("archive organization %s: %v", selector, err)
	}
}

// assertNoOrganizations is a guard for the empty-workspace fixture: if a future
// change makes workspace initialization seed an organization, the empty-list
// test would silently stop testing the empty case.
func assertNoOrganizations(t *testing.T, statePath string) {
	t.Helper()

	state, err := controlplane.OpenReadOnly(context.Background(), statePath)
	if err != nil {
		t.Fatalf("open control plane read-only: %v", err)
	}
	defer func() {
		_ = state.Close()
	}()
	organizations, err := state.Organizations(context.Background())
	if err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	if len(organizations) != 0 {
		t.Fatalf(
			"fixture workspace already has %d organization(s)",
			len(organizations),
		)
	}
}

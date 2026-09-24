package repository

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

func TestRepositoryListShowAndHealthAreDeterministicAndReadOnly(
	t *testing.T,
) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	organizationLayout, err := organization.NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	primaryLayout := repositoryTestLayout(t, root)
	zetaLayout, err := NewLayout(
		organizationLayout,
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
			primaryLayout.CloneDir(): repositoryInventory(
				primaryLayout.CloneDir(),
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
		name   string
		remote string
	}{
		{
			name:   "zeta",
			remote: "https://github.com/valter-silva-au/zeta.git",
		},
		{
			name:   "ai-dev-brain",
			remote: "https://github.com/valter-silva-au/ai-dev-brain.git",
		},
	} {
		if _, err := service.Add(
			context.Background(),
			AddRequest{
				WorkspaceRoot: root,
				Organization:  "amazon",
				Mode:          AddModeCloneNew,
				Remote:        seed.remote,
				ActorType:     "human",
				Tool:          "adb-cli",
				Apply:         true,
			},
		); err != nil {
			t.Fatalf("add %s repository: %v", seed.name, err)
		}
	}

	before := snapshotRepositoryWorkspace(t, root)
	listed, err := service.List(
		context.Background(),
		ListRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
		},
	)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(listed.Data.Repositories) != 2 {
		t.Fatalf("repositories = %#v", listed.Data.Repositories)
	}
	if listed.Data.Repositories[0].Name != "ai-dev-brain" ||
		listed.Data.Repositories[1].Name != "zeta" {
		t.Fatalf("repository order = %#v", listed.Data.Repositories)
	}
	want := listed.Data.Repositories[0]
	selectors := []struct {
		name     string
		selector string
	}{
		{name: "immutable ID", selector: want.ID},
		{
			name:     "canonical identity",
			selector: "github.com/valter-silva-au/ai-dev-brain",
		},
	}
	for _, test := range selectors {
		t.Run(test.name, func(t *testing.T) {
			shown, err := service.Show(
				context.Background(),
				ShowRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      test.selector,
				},
			)
			if err != nil {
				t.Fatalf("show repository by %s: %v", test.name, err)
			}
			if !reflect.DeepEqual(shown.Data.Repository, want) {
				t.Fatalf(
					"show by %s\n got: %#v\nwant: %#v",
					test.name,
					shown.Data.Repository,
					want,
				)
			}
		})
	}
	health, err := service.Health(
		context.Background(),
		HealthRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      want.ID,
		},
	)
	if err != nil {
		t.Fatalf("repository health: %v", err)
	}
	if health.Outcome != capability.OutcomeHealthy ||
		health.Data.State != HealthClean {
		t.Fatalf("repository health = %#v", health)
	}
	if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatalf("repository reads mutated workspace")
	}
}

func TestRepositoryHealthClassifiesUnsafeAndUnavailableStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inventory Inventory
		err       error
		want      HealthState
		wantErr   bool
	}{
		{
			name:      "missing clone",
			inventory: Inventory{},
			want:      HealthMissingClone,
		},
		{
			name: "missing remote",
			inventory: Inventory{
				IsRepository: true,
			},
			want: HealthMissingRemote,
		},
		{
			name: "dirty",
			inventory: Inventory{
				IsRepository: true,
				Dirty:        true,
				Remotes: []RemoteState{{
					Name: "origin",
				}},
			},
			want: HealthDirty,
		},
		{
			name: "ahead",
			inventory: Inventory{
				IsRepository: true,
				Ahead:        2,
				Remotes: []RemoteState{{
					Name: "origin",
				}},
			},
			want: HealthAhead,
		},
		{
			name: "behind",
			inventory: Inventory{
				IsRepository: true,
				Behind:       3,
				Remotes: []RemoteState{{
					Name: "origin",
				}},
			},
			want: HealthBehind,
		},
		{
			name: "diverged",
			inventory: Inventory{
				IsRepository: true,
				Ahead:        1,
				Behind:       1,
				Diverged:     true,
				Remotes: []RemoteState{{
					Name: "origin",
				}},
			},
			want: HealthDiverged,
		},
		{
			name: "authentication",
			err:  ErrAuthentication,
			want: HealthAuthenticationFailure,
		},
		{
			name:    "inventory failure",
			err:     errors.New("inventory unavailable"),
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, service, git, clonePath := managedRepositoryForHealth(t)
			git.inventories[clonePath] = test.inventory
			if test.err != nil {
				git.inventoryErrs = map[string]error{clonePath: test.err}
			}
			result, err := service.Health(
				context.Background(),
				HealthRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
				},
			)
			if test.wantErr {
				if !errors.Is(err, test.err) {
					t.Fatalf("health error = %v, want %v", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("health returned transport error: %v", err)
			}
			if result.Data.State != test.want {
				t.Fatalf("state = %q, want %q", result.Data.State, test.want)
			}
			if result.Outcome != capability.OutcomeAttention {
				t.Fatalf("outcome = %q, want attention", result.Outcome)
			}
		})
	}
}

func TestRepositoryOperationsRejectMissingSelectorWithoutMutation(
	t *testing.T,
) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	service := newTestRepositoryService(t, &fakeGit{}, nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "show",
			run: func() error {
				_, err := service.Show(
					context.Background(),
					ShowRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
					},
				)
				return err
			},
		},
		{
			name: "health",
			run: func() error {
				_, err := service.Health(
					context.Background(),
					HealthRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
					},
				)
				return err
			},
		},
		{
			name: "fetch",
			run: func() error {
				_, err := service.Fetch(
					context.Background(),
					FetchRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
					},
				)
				return err
			},
		},
		{
			name: "update",
			run: func() error {
				_, err := service.Update(
					context.Background(),
					UpdateRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
					},
				)
				return err
			},
		},
	}

	before := snapshotRepositoryWorkspace(t, root)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("missing repository selector was accepted")
			}
		})
	}
	if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatalf("missing-selector failures mutated workspace")
	}
}

func TestRepositoryFetchAndUpdateAreExplicitAndFastForwardOnly(t *testing.T) {
	t.Parallel()

	root, service, git, clonePath := managedRepositoryForHealth(t)
	git.inventories[clonePath] = Inventory{
		IsRepository:  true,
		Branch:        "main",
		DefaultBranch: "main",
		Behind:        2,
		Remotes: []RemoteState{{
			Name:     "origin",
			FetchURL: "https://github.com/valter-silva-au/ai-dev-brain.git",
		}},
	}

	planned, err := service.Fetch(
		context.Background(),
		FetchRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			ActorType:     "human",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan fetch: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned ||
		len(git.fetchCalls) != 0 {
		t.Fatalf("fetch plan = %#v, calls=%#v", planned, git.fetchCalls)
	}

	updated, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("fast-forward update: %v", err)
	}
	if updated.Outcome != capability.OutcomeApplied {
		t.Fatalf("update outcome = %q, want applied", updated.Outcome)
	}
	if len(git.fetchCalls) != 1 ||
		len(git.fastForwardCalls) != 1 ||
		git.fastForwardCalls[0].Branch != "main" {
		t.Fatalf(
			"fetch=%#v fast-forward=%#v",
			git.fetchCalls,
			git.fastForwardCalls,
		)
	}

	git.fetchCalls = nil
	git.fastForwardCalls = nil
	git.inventories[clonePath] = Inventory{
		IsRepository: true,
		Dirty:        true,
		Behind:       2,
		Remotes: []RemoteState{{
			Name: "origin",
		}},
	}
	blocked, err := service.Update(
		context.Background(),
		UpdateRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("dirty update returned transport error: %v", err)
	}
	if blocked.Outcome != capability.OutcomeConflict ||
		len(git.fetchCalls) != 0 ||
		len(git.fastForwardCalls) != 0 {
		t.Fatalf("dirty update = %#v, git=%#v", blocked, git)
	}
}

func TestRepositoryArchiveIsReversibleAndMovePreservesIdentity(t *testing.T) {
	t.Parallel()

	root, service, git, clonePath := managedRepositoryForHealth(t)
	archived, err := service.Archive(
		context.Background(),
		ArchiveRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("archive repository: %v", err)
	}
	if archived.Data.Repository.Status != StatusArchived {
		t.Fatalf("archived repository = %#v", archived.Data.Repository)
	}
	restored, err := service.Archive(
		context.Background(),
		ArchiveRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      archived.Data.Repository.ID,
			Restore:       true,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil || restored.Data.Repository.Status != StatusActive {
		t.Fatalf("restore repository = %#v, %v", restored, err)
	}

	newRemote := "https://github.com/valter-silva-au/adb.git"
	newLayout := movedRepositoryTestLayout(t, root)
	git.inventories[clonePath] = repositoryInventory(
		clonePath,
		"origin",
		newRemote,
	)
	moved, err := service.Move(
		context.Background(),
		MoveRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      restored.Data.Repository.ID,
			NewRemote:     newRemote,
			UpdateRemote:  true,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("move repository: %v", err)
	}
	if moved.Data.Repository.ID != restored.Data.Repository.ID ||
		moved.Data.Repository.Path != newLayout.Root() ||
		moved.Data.Repository.Name != "adb" {
		t.Fatalf("moved repository = %#v", moved.Data.Repository)
	}
	if len(git.setRemoteCalls) != 1 {
		t.Fatalf("set remote calls = %#v", git.setRemoteCalls)
	}
	shown, err := service.Show(
		context.Background(),
		ShowRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
		},
	)
	if err != nil || shown.Data.Repository.ID != restored.Data.Repository.ID {
		t.Fatalf("show moved repository by old alias = %#v, %v", shown, err)
	}
}

func managedRepositoryForHealth(
	t *testing.T,
) (string, *Service, *fakeGit, string) {
	t.Helper()

	root, _ := initializeRepositoryTestWorkspace(t)
	layout := repositoryTestLayout(t, root)
	git := &fakeGit{
		inventories: map[string]Inventory{
			layout.CloneDir(): repositoryInventory(
				layout.CloneDir(),
				"origin",
				"https://github.com/valter-silva-au/ai-dev-brain.git",
			),
		},
	}
	service := newTestRepositoryService(t, git, nil)
	if _, err := service.Add(
		context.Background(),
		AddRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Mode:          AddModeCloneNew,
			Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	); err != nil {
		t.Fatalf("add managed repository: %v", err)
	}
	return root, service, git, layout.CloneDir()
}

func movedRepositoryTestLayout(t *testing.T, root string) Layout {
	t.Helper()

	organizationLayout, err := organization.NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	layout, err := NewLayout(
		organizationLayout,
		"repos",
		"github.com",
		"valter-silva-au",
		"adb",
	)
	if err != nil {
		t.Fatalf("new moved repository layout: %v", err)
	}
	return layout
}

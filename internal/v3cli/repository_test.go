package v3cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

func TestRepositoryCommandSurfacePlansWithoutLegacyApp(t *testing.T) {
	service := &fakeRepository{
		addResult: capability.Result[repository.MutationData]{
			Capability:  repository.AddDescriptor.Capability,
			Version:     repository.AddDescriptor.Version,
			Outcome:     capability.OutcomePlanned,
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}
	loadCount := 0
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		Repository:   service,
		LoadLegacyApp: func() error {
			loadCount++
			return nil
		},
	})
	repo, _, err := root.Find([]string{"repo"})
	if err != nil {
		t.Fatalf("find repo command: %v", err)
	}
	for _, path := range [][]string{
		{"add"},
		{"adopt"},
		{"list"},
		{"show"},
		{"health"},
		{"fetch"},
		{"update"},
		{"move"},
		{"archive"},
		{"worktree", "list"},
		{"worktree", "repair"},
		{"worktree", "prune"},
	} {
		command, _, err := repo.Find(path)
		if err != nil || command == nil || command.Name() != path[len(path)-1] {
			t.Fatalf("repository command %v unavailable: %v", path, err)
		}
	}
	execute(
		t,
		root,
		"repo",
		"add",
		"https://github.com/owner/repo.git",
		"--workspace",
		t.TempDir(),
		"--org",
		"amazon",
		"--format",
		"json",
	)
	if len(service.addRequests) != 1 || service.addRequests[0].Apply {
		t.Fatalf("repository add requests = %#v", service.addRequests)
	}
	if loadCount != 0 {
		t.Fatalf("repository command loaded legacy app %d time(s)", loadCount)
	}
}

// TestRepositoryListOrgIsOptional pins the workspace-wide inventory. Every
// other repo subcommand addresses one repository and needs its organization,
// but `list` answers "what is in this workspace?" — a question that should not
// require already knowing the answer's first component.
func TestRepositoryListOrgIsOptional(t *testing.T) {
	workspace := t.TempDir()
	service := &fakeRepository{
		listResult: capability.Result[repository.ListData]{
			Capability:  repository.ListDescriptor.Capability,
			Version:     repository.ListDescriptor.Version,
			Outcome:     capability.OutcomeHealthy,
			Data:        repository.ListData{Repositories: []repository.Data{}},
			Effects:     []capability.Effect{},
			Warnings:    []capability.Notice{},
			NextActions: []capability.Action{},
			Recovery:    capability.Recovery{Guidance: []string{}},
		},
	}

	for name, args := range map[string][]string{
		"without org": {"repo", "list", "--workspace", workspace},
		"with org": {
			"repo", "list", "--workspace", workspace, "--org", "amazon",
		},
	} {
		t.Run(name, func(t *testing.T) {
			service.listRequests = nil
			execute(t, NewRoot(RootOptions{
				Foundation:   &fakeFoundation{},
				Organization: &fakeOrganization{},
				Repository:   service,
				LoadLegacyApp: func() error {
					t.Fatal("repo list loaded the legacy app")
					return nil
				},
			}), append(args, "--format", "json")...)

			if len(service.listRequests) != 1 {
				t.Fatalf("list requests = %#v", service.listRequests)
			}
			request := service.listRequests[0]
			if request.WorkspaceRoot != workspace {
				t.Fatalf("list workspace = %q, want %q", request.WorkspaceRoot, workspace)
			}
			wantOrg := ""
			if name == "with org" {
				wantOrg = "amazon"
			}
			if request.Organization != wantOrg {
				t.Fatalf(
					"list organization = %q, want %q",
					request.Organization,
					wantOrg,
				)
			}
		})
	}
}

// TestRepositoryNonListCommandsStillRequireOrg keeps the relaxation scoped to
// `list`. Making --org optional everywhere would turn a forgotten flag into an
// ambiguous selector lookup across organizations.
func TestRepositoryNonListCommandsStillRequireOrg(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		Repository:   &fakeRepository{},
		LoadLegacyApp: func() error {
			return nil
		},
	})
	repo, _, err := root.Find([]string{"repo"})
	if err != nil {
		t.Fatalf("find repo command: %v", err)
	}

	for _, path := range [][]string{
		{"add"},
		{"adopt"},
		{"show"},
		{"health"},
		{"fetch"},
		{"update"},
		{"move"},
		{"archive"},
		{"worktree", "list"},
		{"worktree", "repair"},
		{"worktree", "prune"},
	} {
		command, _, err := repo.Find(path)
		if err != nil || command == nil {
			t.Fatalf("repository command %v unavailable: %v", path, err)
		}
		annotations := command.Flags().Lookup("org").Annotations
		if len(annotations[cobra.BashCompOneRequiredFlag]) == 0 {
			t.Fatalf("repo %v must still require --org", path)
		}
	}

	list, _, err := repo.Find([]string{"list"})
	if err != nil || list == nil {
		t.Fatalf("find repo list: %v", err)
	}
	if len(list.Flags().Lookup("org").Annotations[cobra.BashCompOneRequiredFlag]) != 0 {
		t.Fatal("repo list must not require --org")
	}
}

type fakeRepository struct {
	addResult            capability.Result[repository.MutationData]
	addRequests          []repository.AddRequest
	listRequests         []repository.ListRequest
	adoptResult          capability.Result[repository.MutationData]
	listResult           capability.Result[repository.ListData]
	showResult           capability.Result[repository.ShowData]
	healthResult         capability.Result[repository.HealthData]
	fetchResult          capability.Result[repository.MutationData]
	updateResult         capability.Result[repository.MutationData]
	moveResult           capability.Result[repository.MutationData]
	archiveResult        capability.Result[repository.MutationData]
	worktreeListResult   capability.Result[repository.WorktreeListData]
	worktreeRepairResult capability.Result[repository.WorktreeMutationData]
	worktreePruneResult  capability.Result[repository.WorktreeMutationData]
}

func (service *fakeRepository) Add(
	_ context.Context,
	request repository.AddRequest,
) (capability.Result[repository.MutationData], error) {
	service.addRequests = append(service.addRequests, request)
	return service.addResult, nil
}

func (service *fakeRepository) Adopt(
	context.Context,
	repository.AdoptRequest,
) (capability.Result[repository.MutationData], error) {
	return service.adoptResult, nil
}

func (service *fakeRepository) List(
	_ context.Context,
	request repository.ListRequest,
) (capability.Result[repository.ListData], error) {
	service.listRequests = append(service.listRequests, request)
	return service.listResult, nil
}

func (service *fakeRepository) Show(
	context.Context,
	repository.ShowRequest,
) (capability.Result[repository.ShowData], error) {
	return service.showResult, nil
}

func (service *fakeRepository) Health(
	context.Context,
	repository.HealthRequest,
) (capability.Result[repository.HealthData], error) {
	return service.healthResult, nil
}

func (service *fakeRepository) Fetch(
	context.Context,
	repository.FetchRequest,
) (capability.Result[repository.MutationData], error) {
	return service.fetchResult, nil
}

func (service *fakeRepository) Update(
	context.Context,
	repository.UpdateRequest,
) (capability.Result[repository.MutationData], error) {
	return service.updateResult, nil
}

func (service *fakeRepository) Move(
	context.Context,
	repository.MoveRequest,
) (capability.Result[repository.MutationData], error) {
	return service.moveResult, nil
}

func (service *fakeRepository) Archive(
	context.Context,
	repository.ArchiveRequest,
) (capability.Result[repository.MutationData], error) {
	return service.archiveResult, nil
}

func (service *fakeRepository) WorktreeList(
	context.Context,
	repository.WorktreeListRequest,
) (capability.Result[repository.WorktreeListData], error) {
	return service.worktreeListResult, nil
}

func (service *fakeRepository) WorktreeRepair(
	context.Context,
	repository.WorktreeRepairRequest,
) (capability.Result[repository.WorktreeMutationData], error) {
	return service.worktreeRepairResult, nil
}

func (service *fakeRepository) WorktreePrune(
	context.Context,
	repository.WorktreePruneRequest,
) (capability.Result[repository.WorktreeMutationData], error) {
	return service.worktreePruneResult, nil
}

// TestReposIsFoldedIntoTheV3RepoCommand pins the last Q4 move: the legacy
// `adb repos` (clones on disk under <workspace>/repos) folds into the v3 `adb
// repo` noun.
//
// `repos list` cannot keep the name `list`, because the v3 `repo list` already
// means something different — registered v3 repositories from the control plane,
// not clones on disk. So it becomes `inventory`, and the collision is the reason
// rather than a stylistic preference.
//
// Both children are legacy (they read `cli.App`), so like `org create` they must
// carry NO v3 annotation or the root's PersistentPreRunE would skip loading it.
func TestReposIsFoldedIntoTheV3RepoCommand(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation:   &fakeFoundation{},
		Organization: &fakeOrganization{},
		Repository:   &fakeRepository{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	for _, name := range []string{"pull", "inventory"} {
		cmd, remaining, err := root.Find([]string{"repo", name})
		if err != nil || len(remaining) > 0 || cmd == nil || cmd.Name() != name {
			t.Fatalf("`adb repo %s` is not registered: %v", name, err)
		}
		if cmd.Hidden {
			t.Fatalf("`adb repo %s` is the blessed spelling and must be visible", name)
		}
		if cmd.Annotations[v3Annotation] == "true" {
			t.Fatalf("`adb repo %s` must not be v3-annotated: it needs the legacy App", name)
		}
	}

	// The v3 `repo list` must still be its own thing.
	list, _, err := root.Find([]string{"repo", "list"})
	if err != nil || list == nil || list.Name() != "list" {
		t.Fatalf("v3 `adb repo list` disappeared: %v", err)
	}
	if list.Annotations[v3Annotation] != "true" {
		t.Fatal("v3 `adb repo list` lost its annotation")
	}
}

package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestAddCloneNewPlansThenAppliesAndIsIdempotent(t *testing.T) {
	t.Parallel()

	root, organizationID := initializeRepositoryTestWorkspace(t)
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
	request := AddRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Mode:          AddModeCloneNew,
		Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
		ActorType:     "human",
		Tool:          "adb-cli",
	}

	before := snapshotRepositoryWorkspace(t, root)
	planned, err := service.Add(context.Background(), request)
	if err != nil {
		t.Fatalf("plan repository add: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan outcome = %q, want planned", planned.Outcome)
	}
	if len(git.cloneCalls) != 0 || len(git.initCalls) != 0 {
		t.Fatalf("repository plan executed git: %#v", git)
	}
	if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatalf("repository add plan mutated workspace")
	}

	request.Apply = true
	applied, err := service.Add(context.Background(), request)
	if err != nil {
		t.Fatalf("apply repository add: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.Repository.ID == "" ||
		applied.Data.Repository.OrganizationID != organizationID ||
		applied.Data.Repository.Path != layout.Root() ||
		applied.Data.Repository.ClonePath != layout.CloneDir() {
		t.Fatalf("applied repository = %#v", applied.Data.Repository)
	}
	if len(git.cloneCalls) != 1 {
		t.Fatalf("clone calls = %#v", git.cloneCalls)
	}
	if git.cloneCalls[0].Remote != request.Remote ||
		git.cloneCalls[0].Destination != layout.CloneDir() {
		t.Fatalf("clone call = %#v", git.cloneCalls[0])
	}
	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read repository manifest: %v", err)
	}
	if manifest.CanonicalClone.External ||
		manifest.CanonicalClone.Path != manifest.Roles.Clone {
		t.Fatalf("canonical clone = %#v", manifest.CanonicalClone)
	}
	assertRepositoryProjection(t, root, organizationID, manifest.ID, layout)

	repeated, err := service.Add(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat repository add: %v", err)
	}
	if repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat outcome = %q, want unchanged", repeated.Outcome)
	}
	if len(git.cloneCalls) != 1 {
		t.Fatalf("repeat add cloned again: %#v", git.cloneCalls)
	}
}

func TestAddInitializeLocalCreatesGitRepositoryAndCanonicalRemote(
	t *testing.T,
) {
	t.Parallel()

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
	result, err := service.Add(
		context.Background(),
		AddRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Mode:          AddModeInitializeLocal,
			Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize local repository: %v", err)
	}
	if result.Outcome != capability.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", result.Outcome)
	}
	if !reflect.DeepEqual(git.initCalls, []string{layout.CloneDir()}) {
		t.Fatalf("git init calls = %#v", git.initCalls)
	}
	if !reflect.DeepEqual(git.addRemoteCalls, []addRemoteCall{{
		Path:   layout.CloneDir(),
		Name:   "origin",
		Remote: "https://github.com/valter-silva-au/ai-dev-brain.git",
	}}) {
		t.Fatalf("git remote calls = %#v", git.addRemoteCalls)
	}
}

func TestAdoptExistingRegistersExternalCanonicalCloneWithoutMovingIt(
	t *testing.T,
) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	source := filepath.Join(t.TempDir(), "existing")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create adoption source: %v", err)
	}
	authored := filepath.Join(source, "README.md")
	if err := os.WriteFile(authored, []byte("authored\n"), 0o644); err != nil {
		t.Fatalf("write adoption source: %v", err)
	}
	git := &fakeGit{
		inventories: map[string]Inventory{
			source: repositoryInventory(
				source,
				"origin",
				"git@github.com:valter-silva-au/ai-dev-brain.git",
			),
		},
	}
	service := newTestRepositoryService(t, git, nil)
	request := AdoptRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Path:          source,
		RemoteName:    "origin",
		ActorType:     "human",
		Tool:          "adb-cli",
	}

	before := snapshotRepositoryWorkspace(t, root)
	planned, err := service.Adopt(context.Background(), request)
	if err != nil {
		t.Fatalf("plan repository adoption: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("plan outcome = %q, want planned", planned.Outcome)
	}
	if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatalf("repository adoption plan mutated workspace")
	}

	request.Apply = true
	applied, err := service.Adopt(context.Background(), request)
	if err != nil {
		t.Fatalf("apply repository adoption: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("adoption outcome = %q, want applied", applied.Outcome)
	}
	if content, err := os.ReadFile(authored); err != nil ||
		string(content) != "authored\n" {
		t.Fatalf("adoption changed authored clone: %q, %v", content, err)
	}
	if len(git.cloneCalls) != 0 ||
		len(git.initCalls) != 0 ||
		len(git.addRemoteCalls) != 0 {
		t.Fatalf("adoption performed mutating git operation: %#v", git)
	}
	layout := repositoryTestLayout(t, root)
	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read adopted repository manifest: %v", err)
	}
	if !manifest.CanonicalClone.External ||
		manifest.CanonicalClone.Path != source {
		t.Fatalf("adopted canonical clone = %#v", manifest.CanonicalClone)
	}
	if _, err := os.Stat(layout.CloneDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adoption created implicit canonical clone directory: %v", err)
	}
}

func TestRepositoryAddRejectsIdentityPathAndRemoteCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, root string, layout Layout)
		remote  string
	}{
		{
			name: "unmanaged path",
			prepare: func(t *testing.T, _ string, layout Layout) {
				t.Helper()
				if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
					t.Fatalf("create unmanaged repository path: %v", err)
				}
			},
			remote: "https://github.com/valter-silva-au/ai-dev-brain.git",
		},
		{
			name:    "remote identity mismatch",
			prepare: func(*testing.T, string, Layout) {},
			remote:  "https://github.com/other/repository.git",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, _ := initializeRepositoryTestWorkspace(t)
			layout := repositoryTestLayout(t, root)
			test.prepare(t, root, layout)
			service := newTestRepositoryService(t, &fakeGit{}, nil)
			before := snapshotRepositoryWorkspace(t, root)
			result, err := service.Add(
				context.Background(),
				AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeCloneNew,
					Host:          "github.com",
					Owner:         "valter-silva-au",
					Name:          "ai-dev-brain",
					Remote:        test.remote,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				},
			)
			if err != nil {
				t.Fatalf("collision returned transport error: %v", err)
			}
			if result.Outcome != capability.OutcomeConflict {
				t.Fatalf("outcome = %q, want conflict", result.Outcome)
			}
			if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
				after,
				before,
			) {
				t.Fatalf("collision mutated workspace")
			}
		})
	}
}

func TestRepositoryAddFailureAfterPlanLeavesInspectableJournal(
	t *testing.T,
) {
	t.Parallel()

	root, _ := initializeRepositoryTestWorkspace(t)
	injected := errors.New("injected after-plan failure")
	git := &fakeGit{}
	service := newTestRepositoryService(t, git, func() error {
		return injected
	})
	result, err := service.Add(
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
	)
	if !errors.Is(err, injected) {
		t.Fatalf("add error = %v, want injected failure", err)
	}
	if result.Outcome != capability.OutcomeFailed ||
		!result.Recovery.Required ||
		result.Data.OperationID == "" {
		t.Fatalf("failed result = %#v", result)
	}
	if len(git.cloneCalls) != 0 {
		t.Fatalf("failure after plan executed git: %#v", git.cloneCalls)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	journals, err := journal.NewStore(
		workspaceLayout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := journals.Inspect(result.Data.OperationID)
	if err != nil {
		t.Fatalf("inspect failed repository operation: %v", err)
	}
	if state.Status != journal.StatusApplying {
		t.Fatalf("journal status = %q, want applying", state.Status)
	}
}

type cloneCall struct {
	Dir         string
	Remote      string
	Destination string
}

type addRemoteCall struct {
	Path   string
	Name   string
	Remote string
}

type fastForwardCall struct {
	Path   string
	Remote string
	Branch string
}

type setRemoteCall struct {
	Path   string
	Name   string
	Remote string
}

type addWorktreeCall struct {
	RepositoryPath string
	WorktreePath   string
	Branch         string
}

type removeWorktreeCall struct {
	RepositoryPath string
	WorktreePath   string
}

type fakeGit struct {
	inventories         map[string]Inventory
	inventoryErrs       map[string]error
	inventoryCalls      []string
	cloneCalls          []cloneCall
	initCalls           []string
	addRemoteCalls      []addRemoteCall
	setRemoteCalls      []setRemoteCall
	fetchCalls          []string
	fastForwardCalls    []fastForwardCall
	worktrees           map[string][]GitWorktree
	worktreeErrs        map[string]error
	worktreeCalls       []string
	addWorktreeCalls    []addWorktreeCall
	removeWorktreeCalls []removeWorktreeCall
	removeWorktreeErr   error
	afterClone          func(cloneCall) error
	afterInit           func(string) error
	afterFetch          func(string) error
}

func (git *fakeGit) Inventory(
	_ context.Context,
	path string,
	_ string,
) (Inventory, error) {
	git.inventoryCalls = append(git.inventoryCalls, path)
	if err := git.inventoryErrs[path]; err != nil {
		return Inventory{}, err
	}
	return git.inventories[path], nil
}

func (git *fakeGit) Clone(
	_ context.Context,
	dir string,
	remote string,
	destination string,
) error {
	git.cloneCalls = append(git.cloneCalls, cloneCall{
		Dir:         dir,
		Remote:      remote,
		Destination: destination,
	})
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	if git.afterClone != nil {
		return git.afterClone(git.cloneCalls[len(git.cloneCalls)-1])
	}
	return nil
}

func (git *fakeGit) Init(_ context.Context, path string) error {
	git.initCalls = append(git.initCalls, path)
	if git.afterInit != nil {
		return git.afterInit(path)
	}
	return nil
}

func (git *fakeGit) AddRemote(
	_ context.Context,
	path string,
	name string,
	remote string,
) error {
	git.addRemoteCalls = append(git.addRemoteCalls, addRemoteCall{
		Path:   path,
		Name:   name,
		Remote: remote,
	})
	return nil
}

func (git *fakeGit) Fetch(
	_ context.Context,
	path string,
	remote string,
) error {
	git.fetchCalls = append(git.fetchCalls, path+"\x00"+remote)
	if git.afterFetch != nil {
		return git.afterFetch(path)
	}
	return nil
}

func (git *fakeGit) SetRemoteURL(
	_ context.Context,
	path string,
	name string,
	remote string,
) error {
	git.setRemoteCalls = append(git.setRemoteCalls, setRemoteCall{
		Path:   path,
		Name:   name,
		Remote: remote,
	})
	return nil
}

func (git *fakeGit) FastForward(
	_ context.Context,
	path string,
	remote string,
	branch string,
) error {
	git.fastForwardCalls = append(git.fastForwardCalls, fastForwardCall{
		Path:   path,
		Remote: remote,
		Branch: branch,
	})
	return nil
}

func (git *fakeGit) Worktrees(
	_ context.Context,
	path string,
) ([]GitWorktree, error) {
	git.worktreeCalls = append(git.worktreeCalls, path)
	if err := git.worktreeErrs[path]; err != nil {
		return nil, err
	}
	return append([]GitWorktree(nil), git.worktrees[path]...), nil
}

func (git *fakeGit) AddWorktree(
	_ context.Context,
	repositoryPath string,
	worktreePath string,
	branch string,
) error {
	git.addWorktreeCalls = append(git.addWorktreeCalls, addWorktreeCall{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		Branch:         branch,
	})
	return os.MkdirAll(worktreePath, 0o755)
}

func (git *fakeGit) RemoveWorktree(
	_ context.Context,
	repositoryPath string,
	worktreePath string,
) error {
	git.removeWorktreeCalls = append(
		git.removeWorktreeCalls,
		removeWorktreeCall{
			RepositoryPath: repositoryPath,
			WorktreePath:   worktreePath,
		},
	)
	return git.removeWorktreeErr
}

func initializeRepositoryTestWorkspace(t *testing.T) (string, string) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "AWS")
	foundationService, err := foundation.NewService(foundation.Options{})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	if _, err := foundationService.Initialize(
		context.Background(),
		foundation.InitializeRequest{
			Root:  root,
			Name:  "AWS",
			Apply: true,
		},
	); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	organizationService, err := organization.NewService(organization.Options{})
	if err != nil {
		t.Fatalf("new organization service: %v", err)
	}
	result, err := organizationService.Initialize(
		context.Background(),
		organization.InitializeRequest{
			WorkspaceRoot: root,
			Slug:          "amazon",
			Name:          "Amazon",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("initialize organization: %v", err)
	}
	return root, result.Data.OrganizationID
}

func repositoryTestLayout(t *testing.T, root string) Layout {
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
		"ai-dev-brain",
	)
	if err != nil {
		t.Fatalf("new repository layout: %v", err)
	}
	return layout
}

func repositoryInventory(
	path string,
	remoteName string,
	remoteURL string,
) Inventory {
	normalized, err := NormalizeRemoteURL(remoteURL)
	if err != nil {
		panic(err)
	}
	return Inventory{
		Path:          path,
		Root:          path,
		GitDir:        filepath.Join(path, ".git"),
		CommonDir:     filepath.Join(path, ".git"),
		IsRepository:  true,
		Branch:        "main",
		DefaultBranch: "main",
		Remotes: []RemoteState{{
			Name:     remoteName,
			FetchURL: normalized.Normalized,
			PushURL:  normalized.Normalized,
		}},
	}
}

func newTestRepositoryService(
	t *testing.T,
	git Git,
	afterPlan func() error,
) *Service {
	t.Helper()

	index := 0
	service, err := NewService(Options{
		Clock: func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				12,
				0,
				index,
				0,
				time.UTC,
			)
		},
		IDGenerator: func() string {
			index++
			return fmt.Sprintf("repository-test-%02d", index)
		},
		Git:       git,
		AfterPlan: afterPlan,
	})
	if err != nil {
		t.Fatalf("new repository service: %v", err)
	}
	return service
}

func snapshotRepositoryWorkspace(
	t *testing.T,
	root string,
) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			snapshot[relative] = fmt.Sprintf(
				"size=%d sha256=%x",
				len(content),
				sha256.Sum256(content),
			)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("snapshot repository workspace: %v", err)
	}
	return snapshot
}

func assertRepositoryProjection(
	t *testing.T,
	root string,
	organizationID string,
	repositoryID string,
	layout Layout,
) {
	t.Helper()

	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	state, err := controlplane.OpenReadOnly(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	defer func() {
		_ = state.Close()
	}()
	projection, err := state.Repository(
		context.Background(),
		organizationID,
		repositoryID,
	)
	if err != nil {
		t.Fatalf("read repository projection: %v", err)
	}
	if projection.Path != layout.Root() ||
		projection.Host != layout.Host() ||
		projection.Owner != layout.Owner() ||
		projection.Name != layout.Name() {
		t.Fatalf("repository projection = %#v", projection)
	}
}

var _ Git = (*fakeGit)(nil)

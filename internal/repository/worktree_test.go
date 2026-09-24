package repository

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/controlplane"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestGitWorktreePorcelainParsingAndArgvOperations(t *testing.T) {
	t.Parallel()

	repositoryPath := filepath.Join(t.TempDir(), "repo")
	worktreePath := filepath.Join(t.TempDir(), "ADB-39")
	runner := &fakeRunner{responses: map[string]RunResult{
		commandKey(
			repositoryPath,
			"worktree",
			"list",
			"--porcelain",
		): {
			Stdout: "worktree " + repositoryPath + "\n" +
				"HEAD abc\n" +
				"branch refs/heads/main\n\n" +
				"worktree " + worktreePath + "\n" +
				"HEAD def\n" +
				"branch refs/heads/feat/ADB-39-v3\n" +
				"locked agent\n\n",
		},
	}}
	client := NewClient(runner)
	worktrees, err := client.Worktrees(context.Background(), repositoryPath)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	want := []GitWorktree{
		{
			Path:   repositoryPath,
			HEAD:   "abc",
			Branch: "main",
		},
		{
			Path:   worktreePath,
			HEAD:   "def",
			Branch: "feat/ADB-39-v3",
			Locked: true,
		},
	}
	if !reflect.DeepEqual(worktrees, want) {
		t.Fatalf("worktrees\n got: %#v\nwant: %#v", worktrees, want)
	}
	if err := client.AddWorktree(
		context.Background(),
		repositoryPath,
		worktreePath,
		"feat/ADB-39-v3",
	); err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if err := client.RemoveWorktree(
		context.Background(),
		repositoryPath,
		worktreePath,
	); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if got := runner.invocations[len(runner.invocations)-2:]; !reflect.DeepEqual(
		got,
		[]Invocation{
			{
				Dir: repositoryPath,
				Args: []string{
					"worktree",
					"add",
					"--",
					worktreePath,
					"feat/ADB-39-v3",
				},
			},
			{
				Dir:  repositoryPath,
				Args: []string{"worktree", "remove", "--", worktreePath},
			},
		},
	) {
		t.Fatalf("worktree invocations = %#v", got)
	}
}

func TestWorktreeListRepairAndPruneAreSafeAndExact(t *testing.T) {
	t.Parallel()

	root, service, git, clonePath := managedRepositoryForHealth(t)
	layout := repositoryTestLayout(t, root)
	registration := WorktreeRegistration{
		TicketKey: "ADB-39",
		Name:      "default",
		Path:      "work/ADB-39",
		Branch:    "feat/ADB-39-v3",
		Active:    false,
	}
	seedWorktreeRegistration(t, root, layout, registration)
	worktreePath := filepath.Join(layout.Root(), registration.Path)
	git.worktrees = map[string][]GitWorktree{
		clonePath: {{
			Path:   clonePath,
			Branch: "main",
		}},
	}

	listed, err := service.WorktreeList(
		context.Background(),
		WorktreeListRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
		},
	)
	if err != nil {
		t.Fatalf("list repository worktrees: %v", err)
	}
	if len(listed.Data.Worktrees) != 1 ||
		!listed.Data.Worktrees[0].Missing ||
		listed.Data.Worktrees[0].Path != worktreePath {
		t.Fatalf("listed worktrees = %#v", listed.Data.Worktrees)
	}

	planned, err := service.WorktreeRepair(
		context.Background(),
		WorktreeRepairRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			TicketKey:     "ADB-39",
			Name:          "default",
			ActorType:     "human",
			Tool:          "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("plan worktree repair: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned ||
		len(git.addWorktreeCalls) != 0 {
		t.Fatalf("repair plan = %#v, calls=%#v", planned, git.addWorktreeCalls)
	}
	repaired, err := service.WorktreeRepair(
		context.Background(),
		WorktreeRepairRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			TicketKey:     "ADB-39",
			Name:          "default",
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("apply worktree repair: %v", err)
	}
	if repaired.Outcome != capability.OutcomeApplied ||
		!reflect.DeepEqual(git.addWorktreeCalls, []addWorktreeCall{{
			RepositoryPath: clonePath,
			WorktreePath:   worktreePath,
			Branch:         registration.Branch,
		}}) {
		t.Fatalf("repair = %#v, calls=%#v", repaired, git.addWorktreeCalls)
	}

	git.worktrees[clonePath] = []GitWorktree{
		{Path: clonePath, Branch: "main"},
		{Path: worktreePath, Branch: registration.Branch},
	}
	git.inventories[worktreePath] = Inventory{
		IsRepository: true,
		Branch:       registration.Branch,
	}
	pruned, err := service.WorktreePrune(
		context.Background(),
		WorktreePruneRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			Path:          worktreePath,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil {
		t.Fatalf("prune worktree: %v", err)
	}
	if pruned.Outcome != capability.OutcomeApplied ||
		!reflect.DeepEqual(git.removeWorktreeCalls, []removeWorktreeCall{{
			RepositoryPath: clonePath,
			WorktreePath:   worktreePath,
		}}) {
		t.Fatalf("prune = %#v, calls=%#v", pruned, git.removeWorktreeCalls)
	}
	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read pruned manifest: %v", err)
	}
	if len(manifest.Worktrees) != 0 {
		t.Fatalf("pruned registration remains: %#v", manifest.Worktrees)
	}
}

func TestWorktreePruneRefusesActiveDirtyAheadAndUnknownTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		active  bool
		dirty   bool
		ahead   int
		unknown bool
	}{
		{name: "active", active: true},
		{name: "dirty", dirty: true},
		{name: "ahead", ahead: 1},
		{name: "unknown", unknown: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, service, git, clonePath := managedRepositoryForHealth(t)
			layout := repositoryTestLayout(t, root)
			registration := WorktreeRegistration{
				TicketKey: "ADB-39",
				Name:      "default",
				Path:      "work/ADB-39",
				Branch:    "feat/ADB-39-v3",
				Active:    test.active,
			}
			seedWorktreeRegistration(t, root, layout, registration)
			path := filepath.Join(layout.Root(), registration.Path)
			target := path
			if test.unknown {
				target = filepath.Join(layout.WorktreesDir(), "unknown")
			} else if !test.active {
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatalf("create registered worktree target: %v", err)
				}
			}
			git.worktrees = map[string][]GitWorktree{
				clonePath: {
					{Path: clonePath, Branch: "main"},
					{Path: target, Branch: registration.Branch},
				},
			}
			git.inventories[target] = Inventory{
				IsRepository: true,
				Dirty:        test.dirty,
				Ahead:        test.ahead,
			}
			result, err := service.WorktreePrune(
				context.Background(),
				WorktreePruneRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
					Path:          target,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				},
			)
			if err != nil {
				t.Fatalf("prune refusal returned transport error: %v", err)
			}
			if result.Outcome != capability.OutcomeConflict ||
				len(git.removeWorktreeCalls) != 0 {
				t.Fatalf("prune refusal = %#v, calls=%#v", result, git.removeWorktreeCalls)
			}
		})
	}
}

func seedWorktreeRegistration(
	t *testing.T,
	root string,
	layout Layout,
	registration WorktreeRegistration,
) {
	t.Helper()

	manifest, err := ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read repository manifest: %v", err)
	}
	manifest.Worktrees = append(manifest.Worktrees, registration)
	if err := manifest.Validate(layout); err != nil {
		t.Fatalf("validate worktree registration: %v", err)
	}
	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode worktree registration: %v", err)
	}
	if err := os.WriteFile(
		layout.ManifestPath(),
		encoded.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write worktree registration: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	state, err := controlplane.Open(
		context.Background(),
		workspaceLayout.StatePath(),
	)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	defer func() {
		_ = state.Close()
	}()
	if err := state.ObserveRepository(
		context.Background(),
		repositoryProjection(
			layout,
			manifest,
			encoded.Bytes(),
			manifest.UpdatedAt,
		),
	); err != nil {
		t.Fatalf("project worktree registration: %v", err)
	}
	if journal.Digest(encoded.Bytes()) == "" {
		t.Fatal("empty manifest digest")
	}
}

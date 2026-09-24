package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

func TestRepositoryInitialRedirectedRoleBlocksAddAndAdoptBeforeGitOrJournal(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *Service, string, string) (capability.Outcome, error)
	}{
		{
			name: "add clone-new",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				_ string,
			) (capability.Outcome, error) {
				result, err := service.Add(ctx, AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeCloneNew,
					Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name: "add initialize-local",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				_ string,
			) (capability.Outcome, error) {
				result, err := service.Add(ctx, AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeInitializeLocal,
					Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name: "adopt external clone",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				source string,
			) (capability.Outcome, error) {
				result, err := service.Adopt(ctx, AdoptRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Path:          source,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, _ := initializeRepositoryTestWorkspace(t)
			source := filepath.Join(t.TempDir(), "source")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatalf("create adoption source: %v", err)
			}
			git := &fakeGit{inventories: map[string]Inventory{
				source: repositoryInventory(
					source,
					"origin",
					"https://github.com/valter-silva-au/ai-dev-brain.git",
				),
			}}
			service := newTestRepositoryService(t, git, nil)
			external := redirectRepositoriesRole(t, root)
			beforeExternal := snapshotRepositoryWorkspace(t, external)
			beforeJournal := snapshotRepositoryEvents(t, root)

			outcome, err := test.run(
				context.Background(),
				service,
				root,
				source,
			)
			if err == nil {
				t.Fatal("redirected repositories role was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("redirected operation outcome = %q", outcome)
			}
			assertNoRepositoryGitCalls(t, git)
			assertRepositoryBoundaryUnchanged(
				t,
				root,
				external,
				beforeExternal,
				beforeJournal,
			)
		})
	}
}

func TestRepositoryInitialRedirectedRoleBlocksManagedOperationsBeforeGitOrJournal(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(*testing.T, string, *fakeGit, string, Layout) string
		run     func(context.Context, *Service, string, string) (capability.Outcome, error)
	}{
		{
			name: "show",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.Show(ctx, ShowRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      selector,
				})
				return result.Outcome, err
			},
		},
		{
			name: "list",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				_ string,
			) (capability.Outcome, error) {
				result, err := service.List(ctx, ListRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
				})
				return result.Outcome, err
			},
		},
		{
			name: "archive replacement",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.Archive(ctx, ArchiveRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      selector,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name: "move",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.Move(ctx, MoveRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      selector,
					NewRemote:     "https://github.com/valter-silva-au/adb.git",
					UpdateRemote:  true,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name: "fetch",
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.Fetch(ctx, FetchRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      selector,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name: "update",
			prepare: func(
				_ *testing.T,
				_ string,
				git *fakeGit,
				clonePath string,
				_ Layout,
			) string {
				inventory := git.inventories[clonePath]
				inventory.Behind = 1
				git.inventories[clonePath] = inventory
				return ""
			},
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.Update(ctx, UpdateRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      selector,
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				return result.Outcome, err
			},
		},
		{
			name:    "worktree repair",
			prepare: prepareBoundaryWorktree,
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				result, err := service.WorktreeRepair(
					ctx,
					WorktreeRepairRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
						Selector:      selector,
						TicketKey:     "ADB-39",
						Name:          "default",
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         true,
					},
				)
				return result.Outcome, err
			},
		},
		{
			name:    "worktree prune",
			prepare: prepareBoundaryWorktree,
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				selector string,
			) (capability.Outcome, error) {
				layout := repositoryTestLayout(t, root)
				result, err := service.WorktreePrune(
					ctx,
					WorktreePruneRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
						Selector:      selector,
						Path: filepath.Join(
							layout.Root(),
							"work",
							"ADB-39",
						),
						ActorType: "human",
						Tool:      "adb-cli",
						Apply:     true,
					},
				)
				return result.Outcome, err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, service, git, clonePath := managedRepositoryForHealth(t)
			layout := repositoryTestLayout(t, root)
			if test.prepare != nil {
				test.prepare(t, root, git, clonePath, layout)
			}
			resetRepositoryGitCalls(git)
			external := redirectRepositoriesRole(t, root)
			beforeExternal := snapshotRepositoryWorkspace(t, external)
			beforeJournal := snapshotRepositoryEvents(t, root)

			outcome, err := test.run(
				context.Background(),
				service,
				root,
				"github.com/valter-silva-au/ai-dev-brain",
			)
			if err == nil {
				t.Fatal("redirected repositories role was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("redirected operation outcome = %q", outcome)
			}
			assertNoRepositoryGitCalls(t, git)
			assertRepositoryBoundaryUnchanged(
				t,
				root,
				external,
				beforeExternal,
				beforeJournal,
			)
		})
	}
}

func TestRepositoryInitialExactCloneRedirectBlocksInventoryAndWorktrees(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *Service, string) error
	}{
		{
			name: "health inventory",
			run: func(ctx context.Context, service *Service, root string) error {
				_, err := service.Health(ctx, HealthRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
				})
				return err
			},
		},
		{
			name: "worktree list",
			run: func(ctx context.Context, service *Service, root string) error {
				_, err := service.WorktreeList(ctx, WorktreeListRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
				})
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, service, git, clonePath := managedRepositoryForHealth(t)
			external := redirectExactRepositoryPath(t, clonePath)
			beforeExternal := snapshotRepositoryWorkspace(t, external)
			resetRepositoryGitCalls(git)

			if err := test.run(context.Background(), service, root); err == nil {
				t.Fatal("redirected exact clone was accepted")
			}
			assertNoRepositoryGitCalls(t, git)
			if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"external clone changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
		})
	}
}

func TestWorktreeRepairRequiresContainedInternalCloneBeforeWorktrees(
	t *testing.T,
) {
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
	external := filepath.Join(t.TempDir(), "missing-internal-clone")
	if err := os.Rename(clonePath, external); err != nil {
		t.Fatalf("move internal clone outside workspace: %v", err)
	}
	resetRepositoryGitCalls(git)

	_, err := service.WorktreeRepair(
		context.Background(),
		WorktreeRepairRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      "github.com/valter-silva-au/ai-dev-brain",
			TicketKey:     registration.TicketKey,
			Name:          registration.Name,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err == nil {
		t.Fatal("worktree repair accepted a missing internal clone")
	}
	if len(git.worktreeCalls) != 0 {
		t.Fatalf("worktree calls = %#v", git.worktreeCalls)
	}
}

func TestRepositoryPostPlanRoleChangeBlocksGitAndAppliedSteps(t *testing.T) {
	t.Parallel()

	type setupResult struct {
		root         string
		git          *fakeGit
		run          func(context.Context, *Service) (capability.Outcome, string, error)
		assertNoCall func(*testing.T, *fakeGit)
	}
	tests := []struct {
		name  string
		setup func(*testing.T) setupResult
	}{
		{
			name: "add clone-new",
			setup: func(t *testing.T) setupResult {
				root, _ := initializeRepositoryTestWorkspace(t)
				git := &fakeGit{}
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Add(ctx, AddRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Mode:          AddModeCloneNew,
							Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.cloneCalls) != 0 ||
							len(git.inventoryCalls) != 0 {
							t.Fatalf("post-plan clone calls = %#v", git)
						}
					},
				}
			},
		},
		{
			name: "add initialize-local",
			setup: func(t *testing.T) setupResult {
				root, _ := initializeRepositoryTestWorkspace(t)
				git := &fakeGit{}
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Add(ctx, AddRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Mode:          AddModeInitializeLocal,
							Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.initCalls) != 0 ||
							len(git.addRemoteCalls) != 0 ||
							len(git.inventoryCalls) != 0 {
							t.Fatalf("post-plan initialize calls = %#v", git)
						}
					},
				}
			},
		},
		{
			name: "move before set-remote",
			setup: func(t *testing.T) setupResult {
				root, _, git, _ := managedRepositoryForHealth(t)
				resetRepositoryGitCalls(git)
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Move(ctx, MoveRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							NewRemote:     "https://github.com/valter-silva-au/adb.git",
							UpdateRemote:  true,
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.setRemoteCalls) != 0 {
							t.Fatalf("post-plan set-remote calls = %#v", git.setRemoteCalls)
						}
					},
				}
			},
		},
		{
			name: "fetch",
			setup: func(t *testing.T) setupResult {
				root, _, git, _ := managedRepositoryForHealth(t)
				resetRepositoryGitCalls(git)
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Fetch(ctx, FetchRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.fetchCalls) != 0 {
							t.Fatalf("post-plan fetch calls = %#v", git.fetchCalls)
						}
					},
				}
			},
		},
		{
			name: "update",
			setup: func(t *testing.T) setupResult {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				inventory := git.inventories[clonePath]
				inventory.Behind = 1
				git.inventories[clonePath] = inventory
				resetRepositoryGitCalls(git)
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Update(ctx, UpdateRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.fetchCalls) != 0 ||
							len(git.fastForwardCalls) != 0 {
							t.Fatalf("post-plan update calls = %#v", git)
						}
					},
				}
			},
		},
		{
			name: "worktree repair",
			setup: func(t *testing.T) setupResult {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				layout := repositoryTestLayout(t, root)
				prepareBoundaryWorktree(t, root, git, clonePath, layout)
				git.worktrees[clonePath] = []GitWorktree{{
					Path:   clonePath,
					Branch: "main",
				}}
				resetRepositoryGitCalls(git)
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.WorktreeRepair(
							ctx,
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
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.addWorktreeCalls) != 0 {
							t.Fatalf(
								"post-plan repair calls = %#v",
								git.addWorktreeCalls,
							)
						}
					},
				}
			},
		},
		{
			name: "worktree prune",
			setup: func(t *testing.T) setupResult {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				layout := repositoryTestLayout(t, root)
				worktreePath := prepareBoundaryWorktree(
					t,
					root,
					git,
					clonePath,
					layout,
				)
				resetRepositoryGitCalls(git)
				return setupResult{
					root: root,
					git:  git,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.WorktreePrune(
							ctx,
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
						return result.Outcome, result.Data.OperationID, err
					},
					assertNoCall: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.removeWorktreeCalls) != 0 {
							t.Fatalf(
								"post-plan prune calls = %#v",
								git.removeWorktreeCalls,
							)
						}
					},
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := test.setup(t)
			var external string
			var beforeExternal map[string]string
			service := newPostPlanRepositoryService(
				t,
				fixture.git,
				func() error {
					external = redirectRepositoriesRole(t, fixture.root)
					beforeExternal = snapshotRepositoryWorkspace(t, external)
					return nil
				},
			)

			outcome, operationID, err := fixture.run(
				context.Background(),
				service,
			)
			if err == nil {
				t.Fatal("post-plan repositories role change was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("post-plan operation outcome = %q", outcome)
			}
			fixture.assertNoCall(t, fixture.git)
			if external == "" {
				t.Fatalf(
					"operation failed before after-plan hook: outcome=%q err=%v",
					outcome,
					err,
				)
			}
			if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"post-plan operation changed external role\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			assertRepositoryJournalHasNoAppliedStep(t, fixture.root, operationID)
		})
	}
}

func TestRepositoryPostPlanExactCloneRedirectBlocksGit(t *testing.T) {
	t.Parallel()

	type fixture struct {
		root       string
		git        *fakeGit
		exactPath  string
		run        func(context.Context, *Service) (capability.Outcome, string, error)
		assertNone func(*testing.T, *fakeGit)
	}
	tests := []struct {
		name  string
		setup func(*testing.T) fixture
	}{
		{
			name: "clone-new destination",
			setup: func(t *testing.T) fixture {
				root, _ := initializeRepositoryTestWorkspace(t)
				layout := repositoryTestLayout(t, root)
				git := &fakeGit{}
				return fixture{
					root:      root,
					git:       git,
					exactPath: layout.CloneDir(),
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Add(ctx, AddRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Mode:          AddModeCloneNew,
							Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNone: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.cloneCalls) != 0 {
							t.Fatalf("clone calls = %#v", git.cloneCalls)
						}
					},
				}
			},
		},
		{
			name: "move source clone",
			setup: func(t *testing.T) fixture {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				resetRepositoryGitCalls(git)
				return fixture{
					root:      root,
					git:       git,
					exactPath: clonePath,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Move(ctx, MoveRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							NewRemote:     "https://github.com/valter-silva-au/adb.git",
							UpdateRemote:  true,
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNone: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.setRemoteCalls) != 0 {
							t.Fatalf("set-remote calls = %#v", git.setRemoteCalls)
						}
					},
				}
			},
		},
		{
			name: "fetch source clone",
			setup: func(t *testing.T) fixture {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				resetRepositoryGitCalls(git)
				return fixture{
					root:      root,
					git:       git,
					exactPath: clonePath,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Fetch(ctx, FetchRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNone: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.fetchCalls) != 0 {
							t.Fatalf("fetch calls = %#v", git.fetchCalls)
						}
					},
				}
			},
		},
		{
			name: "update source clone",
			setup: func(t *testing.T) fixture {
				root, _, git, clonePath := managedRepositoryForHealth(t)
				inventory := git.inventories[clonePath]
				inventory.Behind = 1
				git.inventories[clonePath] = inventory
				resetRepositoryGitCalls(git)
				return fixture{
					root:      root,
					git:       git,
					exactPath: clonePath,
					run: func(
						ctx context.Context,
						service *Service,
					) (capability.Outcome, string, error) {
						result, err := service.Update(ctx, UpdateRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Selector:      "github.com/valter-silva-au/ai-dev-brain",
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						})
						return result.Outcome, result.Data.OperationID, err
					},
					assertNone: func(t *testing.T, git *fakeGit) {
						t.Helper()
						if len(git.fetchCalls) != 0 ||
							len(git.fastForwardCalls) != 0 {
							t.Fatalf("update calls = %#v", git)
						}
					},
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := test.setup(t)
			var external string
			var beforeExternal map[string]string
			service := newPostPlanRepositoryService(
				t,
				fixture.git,
				func() error {
					external = redirectExactRepositoryPath(t, fixture.exactPath)
					beforeExternal = snapshotRepositoryWorkspace(t, external)
					return nil
				},
			)
			outcome, operationID, err := fixture.run(
				context.Background(),
				service,
			)
			if err == nil {
				t.Fatal("exact clone redirection was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("outcome = %q, want failed", outcome)
			}
			fixture.assertNone(t, fixture.git)
			if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"external clone changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			assertRepositoryJournalHasNoAppliedStep(t, fixture.root, operationID)
		})
	}
}

func TestRepositoryPostPlanExactWorktreeRedirectBlocksGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		redirect    func(Layout, string) string
		run         func(context.Context, *Service, string, string) (capability.Outcome, string, error)
		assertNone  func(*testing.T, *fakeGit)
		worktreeNow bool
	}{
		{
			name: "repair redirected parent",
			redirect: func(layout Layout, _ string) string {
				return layout.WorktreesDir()
			},
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				_ string,
			) (capability.Outcome, string, error) {
				result, err := service.WorktreeRepair(
					ctx,
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
				return result.Outcome, result.Data.OperationID, err
			},
			assertNone: func(t *testing.T, git *fakeGit) {
				t.Helper()
				if len(git.addWorktreeCalls) != 0 {
					t.Fatalf("add-worktree calls = %#v", git.addWorktreeCalls)
				}
			},
		},
		{
			name: "prune redirected target",
			redirect: func(_ Layout, worktreePath string) string {
				return worktreePath
			},
			run: func(
				ctx context.Context,
				service *Service,
				root string,
				worktreePath string,
			) (capability.Outcome, string, error) {
				result, err := service.WorktreePrune(
					ctx,
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
				return result.Outcome, result.Data.OperationID, err
			},
			assertNone: func(t *testing.T, git *fakeGit) {
				t.Helper()
				if len(git.removeWorktreeCalls) != 0 {
					t.Fatalf(
						"remove-worktree calls = %#v",
						git.removeWorktreeCalls,
					)
				}
			},
			worktreeNow: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, _, git, clonePath := managedRepositoryForHealth(t)
			layout := repositoryTestLayout(t, root)
			worktreePath := prepareBoundaryWorktree(
				t,
				root,
				git,
				clonePath,
				layout,
			)
			if !test.worktreeNow {
				git.worktrees[clonePath] = []GitWorktree{{
					Path:   clonePath,
					Branch: "main",
				}}
			} else if err := os.MkdirAll(worktreePath, 0o755); err != nil {
				t.Fatalf("create registered worktree target: %v", err)
			}
			resetRepositoryGitCalls(git)
			exactPath := test.redirect(layout, worktreePath)
			var external string
			var beforeExternal map[string]string
			service := newPostPlanRepositoryService(
				t,
				git,
				func() error {
					external = redirectExactRepositoryPath(t, exactPath)
					beforeExternal = snapshotRepositoryWorkspace(t, external)
					return nil
				},
			)
			outcome, operationID, err := test.run(
				context.Background(),
				service,
				root,
				worktreePath,
			)
			if err == nil {
				t.Fatal("exact worktree redirection was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("outcome = %q, want failed", outcome)
			}
			if operationID == "" {
				t.Fatalf("operation failed before journaling: %v", err)
			}
			test.assertNone(t, git)
			if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"external worktree path changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			assertRepositoryJournalNotCommitted(t, root, operationID)
		})
	}
}

func TestRepositoryRevalidatesExactPathsBetweenGitCalls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		run        func(*testing.T, *fakeGit) (string, string, capability.Outcome, string, error)
		assertCall func(*testing.T, *fakeGit)
	}{
		{
			name: "clone before verify inventory",
			run: func(
				t *testing.T,
				git *fakeGit,
			) (string, string, capability.Outcome, string, error) {
				root, _ := initializeRepositoryTestWorkspace(t)
				layout := repositoryTestLayout(t, root)
				var external string
				git.afterClone = func(call cloneCall) error {
					external = redirectExactRepositoryPath(t, call.Destination)
					return nil
				}
				service := newPostPlanRepositoryService(t, git, nil)
				result, err := service.Add(context.Background(), AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeCloneNew,
					Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				if external == "" || layout.CloneDir() == "" {
					t.Fatal("clone hook was not reached")
				}
				return root, external, result.Outcome, result.Data.OperationID, err
			},
			assertCall: func(t *testing.T, git *fakeGit) {
				t.Helper()
				if len(git.cloneCalls) != 1 ||
					len(git.inventoryCalls) != 0 {
					t.Fatalf("clone/inventory calls = %#v", git)
				}
			},
		},
		{
			name: "init before add-remote",
			run: func(
				t *testing.T,
				git *fakeGit,
			) (string, string, capability.Outcome, string, error) {
				root, _ := initializeRepositoryTestWorkspace(t)
				var external string
				git.afterInit = func(path string) error {
					external = redirectExactRepositoryPath(t, path)
					return nil
				}
				service := newPostPlanRepositoryService(t, git, nil)
				result, err := service.Add(context.Background(), AddRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Mode:          AddModeInitializeLocal,
					Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				if external == "" {
					t.Fatal("init hook was not reached")
				}
				return root, external, result.Outcome, result.Data.OperationID, err
			},
			assertCall: func(t *testing.T, git *fakeGit) {
				t.Helper()
				if len(git.initCalls) != 1 ||
					len(git.addRemoteCalls) != 0 {
					t.Fatalf("init/add-remote calls = %#v", git)
				}
			},
		},
		{
			name: "fetch before fast-forward",
			run: func(
				t *testing.T,
				git *fakeGit,
			) (string, string, capability.Outcome, string, error) {
				root, _, seededGit, clonePath := managedRepositoryForHealth(t)
				*git = *seededGit
				inventory := git.inventories[clonePath]
				inventory.Behind = 1
				git.inventories[clonePath] = inventory
				resetRepositoryGitCalls(git)
				var external string
				git.afterFetch = func(path string) error {
					external = redirectExactRepositoryPath(t, path)
					return nil
				}
				service := newPostPlanRepositoryService(t, git, nil)
				result, err := service.Update(context.Background(), UpdateRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
					ActorType:     "human",
					Tool:          "adb-cli",
					Apply:         true,
				})
				if external == "" {
					t.Fatal("fetch hook was not reached")
				}
				return root, external, result.Outcome, result.Data.OperationID, err
			},
			assertCall: func(t *testing.T, git *fakeGit) {
				t.Helper()
				if len(git.fetchCalls) != 1 ||
					len(git.fastForwardCalls) != 0 {
					t.Fatalf("fetch/fast-forward calls = %#v", git)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			git := &fakeGit{}
			root, external, outcome, operationID, err := test.run(t, git)
			if err == nil {
				t.Fatal("between-call path redirection was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("outcome = %q, want failed", outcome)
			}
			test.assertCall(t, git)
			if marker, markerErr := os.ReadFile(
				filepath.Join(external, "boundary-marker"),
			); markerErr != nil || string(marker) != "outside\n" {
				t.Fatalf("external path changed: %q, %v", marker, markerErr)
			}
			assertRepositoryJournalNotCommitted(t, root, operationID)
		})
	}
}

func TestRepositoryMoveRejectsDestinationSymlinksBeforePlanning(t *testing.T) {
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

				root, service, git, _ := managedRepositoryForHealth(t)
				destination := movedRepositoryTestLayout(t, root)
				if err := os.MkdirAll(
					filepath.Dir(destination.Root()),
					0o755,
				); err != nil {
					t.Fatalf("create move destination parent: %v", err)
				}
				external := filepath.Join(t.TempDir(), "external-destination")
				var beforeExternal map[string]string
				if link.live {
					if err := os.MkdirAll(external, 0o755); err != nil {
						t.Fatalf("create live destination target: %v", err)
					}
					if err := os.WriteFile(
						filepath.Join(external, "boundary-marker"),
						[]byte("outside\n"),
						0o644,
					); err != nil {
						t.Fatalf("write live destination marker: %v", err)
					}
					beforeExternal = snapshotRepositoryWorkspace(t, external)
				}
				if err := os.Symlink(external, destination.Root()); err != nil {
					t.Fatalf("create destination symlink: %v", err)
				}
				beforeJournal := snapshotRepositoryEvents(t, root)
				resetRepositoryGitCalls(git)

				result, err := service.Move(
					context.Background(),
					MoveRequest{
						WorkspaceRoot: root,
						Organization:  "amazon",
						Selector:      "github.com/valter-silva-au/ai-dev-brain",
						NewRemote:     "https://github.com/valter-silva-au/adb.git",
						UpdateRemote:  true,
						ActorType:     "human",
						Tool:          "adb-cli",
						Apply:         apply,
					},
				)
				if err == nil {
					t.Fatalf("destination symlink was accepted: %#v", result)
				}
				if result.Outcome == capability.OutcomePlanned ||
					result.Outcome == capability.OutcomeApplied {
					t.Fatalf("destination symlink outcome = %q", result.Outcome)
				}
				assertNoRepositoryGitCalls(t, git)
				if after := snapshotRepositoryEvents(t, root); !reflect.DeepEqual(
					after,
					beforeJournal,
				) {
					t.Fatalf(
						"destination symlink changed journal\n got: %#v\nwant: %#v",
						after,
						beforeJournal,
					)
				}
				if link.live {
					if after := snapshotRepositoryWorkspace(
						t,
						external,
					); !reflect.DeepEqual(after, beforeExternal) {
						t.Fatalf(
							"live destination target changed\n got: %#v\nwant: %#v",
							after,
							beforeExternal,
						)
					}
					return
				}
				if _, statErr := os.Lstat(external); !errors.Is(
					statErr,
					os.ErrNotExist,
				) {
					t.Fatalf("dangling destination target changed: %v", statErr)
				}
			})
		}
	}
}

func TestWorktreePruneRemoveFailureIsStructuredAndJournaled(t *testing.T) {
	t.Parallel()

	root, _, git, clonePath := managedRepositoryForHealth(t)
	layout := repositoryTestLayout(t, root)
	worktreePath := prepareBoundaryWorktree(
		t,
		root,
		git,
		clonePath,
		layout,
	)
	injected := errors.New("injected remove worktree failure")
	git.removeWorktreeErr = injected
	resetRepositoryGitCalls(git)
	service := newPostPlanRepositoryService(t, git, nil)

	result, err := service.WorktreePrune(
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
	if !errors.Is(err, injected) {
		t.Fatalf("prune error = %v, want injected failure", err)
	}
	if len(git.removeWorktreeCalls) != 1 {
		t.Fatalf("remove worktree calls = %#v", git.removeWorktreeCalls)
	}
	assertStructuredWorktreePruneFailure(t, root, result)
}

func TestWorktreePruneLaterFailuresAreStructuredAndJournaled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		afterPlan func(*testing.T, string, Layout) func() error
	}{
		{
			name: "manifest write",
			afterPlan: func(
				t *testing.T,
				_ string,
				layout Layout,
			) func() error {
				return func() error {
					redirectRepositoryControlDir(t, layout, nil)
					return nil
				}
			},
		},
		{
			name: "projection",
			afterPlan: func(
				t *testing.T,
				root string,
				_ Layout,
			) func() error {
				workspaceLayout, err := workspace.NewLayout(root)
				if err != nil {
					t.Fatalf("new workspace layout: %v", err)
				}
				return func() error {
					statePath := workspaceLayout.StatePath()
					if err := os.Rename(
						statePath,
						statePath+".before-prune",
					); err != nil {
						t.Fatalf("move control plane state: %v", err)
					}
					if err := os.Mkdir(statePath, 0o755); err != nil {
						t.Fatalf("block control plane state path: %v", err)
					}
					return nil
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, _, git, clonePath := managedRepositoryForHealth(t)
			layout := repositoryTestLayout(t, root)
			worktreePath := prepareBoundaryWorktree(
				t,
				root,
				git,
				clonePath,
				layout,
			)
			resetRepositoryGitCalls(git)
			service := newPostPlanRepositoryService(
				t,
				git,
				test.afterPlan(t, root, layout),
			)

			result, err := service.WorktreePrune(
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
			if err == nil {
				t.Fatal("prune post-plan failure was accepted")
			}
			if len(git.removeWorktreeCalls) != 1 {
				t.Fatalf("remove worktree calls = %#v", git.removeWorktreeCalls)
			}
			assertStructuredWorktreePruneFailure(t, root, result)
		})
	}
}

func assertStructuredWorktreePruneFailure(
	t *testing.T,
	root string,
	result capability.Result[WorktreeMutationData],
) {
	t.Helper()

	if result.Capability != WorktreePruneDescriptor.Capability ||
		result.Outcome != capability.OutcomeFailed ||
		result.Data.OperationID == "" ||
		!result.Recovery.Required {
		t.Fatalf("structured prune failure = %#v", result)
	}
	if len(result.Effects) != 2 {
		t.Fatalf("prune failure effects = %#v", result.Effects)
	}
	for _, effect := range result.Effects {
		if effect.Status != capability.EffectPlanned {
			t.Fatalf("prune failure effect = %#v", effect)
		}
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	store, err := journal.NewStore(
		workspaceLayout.EventsDir(),
		time.Now,
		func() string { return "unused" },
	)
	if err != nil {
		t.Fatalf("open journal store: %v", err)
	}
	state, err := store.InspectRecorded(result.Data.OperationID)
	if err != nil {
		t.Fatalf("inspect prune journal: %v", err)
	}
	if state.Status != journal.StatusFailed {
		t.Fatalf("prune journal status = %q, want failed", state.Status)
	}
}

func redirectExactRepositoryPath(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create exact-path parent: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external")
	if err := os.Rename(path, external); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("move exact managed path outside workspace: %v", err)
		}
		if err := os.MkdirAll(external, 0o755); err != nil {
			t.Fatalf("create external exact path: %v", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(external, "boundary-marker"),
		[]byte("outside\n"),
		0o644,
	); err != nil {
		t.Fatalf("write exact-path boundary marker: %v", err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatalf("redirect exact managed path: %v", err)
	}
	return external
}

func TestRepositoryMatchingContentCannotBypassRootedPublication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(
			*testing.T,
			*Service,
			func(func() error),
		) (string, *string, func() (capability.Outcome, string, error))
	}{
		{
			name: "create target already matches",
			setup: func(
				t *testing.T,
				service *Service,
				setAfterPlan func(func() error),
			) (string, *string, func() (capability.Outcome, string, error)) {
				root, _ := initializeRepositoryTestWorkspace(t)
				layout := repositoryTestLayout(t, root)
				source := filepath.Join(t.TempDir(), "source")
				if err := os.MkdirAll(source, 0o755); err != nil {
					t.Fatalf("create adoption source: %v", err)
				}
				git := service.git.(*fakeGit)
				git.inventories = map[string]Inventory{
					source: repositoryInventory(
						source,
						"origin",
						"https://github.com/valter-silva-au/ai-dev-brain.git",
					),
				}
				external := new(string)
				setAfterPlan(func() error {
					*external = redirectRepositoryControlDir(
						t,
						layout,
						map[string][]byte{
							"config.yaml": []byte(defaultConfig),
						},
					)
					return nil
				})
				return root, external, func() (capability.Outcome, string, error) {
					result, err := service.Adopt(
						context.Background(),
						AdoptRequest{
							WorkspaceRoot: root,
							Organization:  "amazon",
							Path:          source,
							ActorType:     "human",
							Tool:          "adb-cli",
							Apply:         true,
						},
					)
					return result.Outcome, result.Data.OperationID, err
				}
			},
		},
		{
			name: "replacement target matches before",
			setup: func(
				t *testing.T,
				service *Service,
				setAfterPlan func(func() error),
			) (string, *string, func() (capability.Outcome, string, error)) {
				root, _, git, _ := managedRepositoryForHealth(t)
				service.git = git
				layout := repositoryTestLayout(t, root)
				external := new(string)
				setAfterPlan(func() error {
					*external = redirectRepositoryControlDir(t, layout, nil)
					return nil
				})
				return root, external, func() (capability.Outcome, string, error) {
					result, err := service.Archive(
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
					return result.Outcome, result.Data.OperationID, err
				}
			},
		},
		{
			name: "replacement target matches after",
			setup: func(
				t *testing.T,
				service *Service,
				setAfterPlan func(func() error),
			) (string, *string, func() (capability.Outcome, string, error)) {
				root, _, git, _ := managedRepositoryForHealth(t)
				service.git = git
				layout := repositoryTestLayout(t, root)
				manifest, err := ReadManifest(layout.ManifestPath())
				if err != nil {
					t.Fatalf("read repository manifest: %v", err)
				}
				archivedAt := time.Date(
					2026,
					time.September,
					10,
					13,
					0,
					1,
					0,
					time.UTC,
				)
				manifest.Status = StatusArchived
				manifest.ArchivedAt = &archivedAt
				manifest.UpdatedAt = archivedAt
				manifest.LastMutation = mutationProvenance(
					"repository-boundary-01",
					"human",
					"",
					"adb-cli",
				)
				content, err := encodeManifest(manifest)
				if err != nil {
					t.Fatalf("encode expected archived manifest: %v", err)
				}
				external := new(string)
				setAfterPlan(func() error {
					*external = redirectRepositoryControlDir(
						t,
						layout,
						map[string][]byte{"manifest.yaml": content},
					)
					return nil
				})
				return root, external, func() (capability.Outcome, string, error) {
					result, err := service.Archive(
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
					return result.Outcome, result.Data.OperationID, err
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			git := &fakeGit{}
			var afterPlan func() error
			var external *string
			var beforeExternal map[string]string
			service := newPostPlanRepositoryService(
				t,
				git,
				func() error {
					if err := afterPlan(); err != nil {
						return err
					}
					beforeExternal = snapshotRepositoryWorkspace(t, *external)
					return nil
				},
			)
			var root string
			var run func() (capability.Outcome, string, error)
			root, external, run = test.setup(
				t,
				service,
				func(callback func() error) {
					afterPlan = callback
				},
			)
			outcome, operationID, err := run()
			if *external == "" {
				t.Fatal("matching-content after-plan hook was not reached")
			}
			if err == nil {
				t.Fatal("matching redirected target was accepted")
			}
			if outcome == capability.OutcomeApplied {
				t.Fatalf("matching redirected outcome = %q", outcome)
			}
			if after := snapshotRepositoryWorkspace(t, *external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"matching redirected target changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
			assertRepositoryJournalHasNoAppliedStep(t, root, operationID)
		})
	}
}

func TestRepositoryRedirectedManagedContainerIsRejectedBeforeManifestReadOrGit(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *Service, string) error
	}{
		{
			name: "show",
			run: func(ctx context.Context, service *Service, root string) error {
				_, err := service.Show(ctx, ShowRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
				})
				return err
			},
		},
		{
			name: "list",
			run: func(ctx context.Context, service *Service, root string) error {
				_, err := service.List(ctx, ListRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
				})
				return err
			},
		},
		{
			name: "health",
			run: func(ctx context.Context, service *Service, root string) error {
				_, err := service.Health(ctx, HealthRequest{
					WorkspaceRoot: root,
					Organization:  "amazon",
					Selector:      "github.com/valter-silva-au/ai-dev-brain",
				})
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root, service, git, _ := managedRepositoryForHealth(t)
			layout := repositoryTestLayout(t, root)
			external := filepath.Join(t.TempDir(), "external-repository")
			if err := os.Rename(layout.Root(), external); err != nil {
				t.Fatalf("move managed repository outside workspace: %v", err)
			}
			if err := os.Symlink(external, layout.Root()); err != nil {
				t.Fatalf("redirect managed repository container: %v", err)
			}
			beforeExternal := snapshotRepositoryWorkspace(t, external)
			resetRepositoryGitCalls(git)

			if err := test.run(context.Background(), service, root); err == nil {
				t.Fatal("redirected managed repository container was accepted")
			}
			assertNoRepositoryGitCalls(t, git)
			if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
				after,
				beforeExternal,
			) {
				t.Fatalf(
					"redirected repository changed\n got: %#v\nwant: %#v",
					after,
					beforeExternal,
				)
			}
		})
	}
}

func TestRepositoryLifecycleUsesContainedNonDefaultWorkspaceAndRepositoryRoles(
	t *testing.T,
) {
	t.Parallel()

	root, organizationID, organizationLayout := initializeRepositoryWorkspaceWithRoles(
		t,
		"managed-organizations",
		"managed-repositories",
	)
	layout, err := NewLayout(
		organizationLayout,
		"managed-repositories",
		"github.com",
		"valter-silva-au",
		"ai-dev-brain",
	)
	if err != nil {
		t.Fatalf("new custom repository layout: %v", err)
	}
	defaultRepositories := organizationLayout.RepositoriesDir()
	if _, err := os.Stat(defaultRepositories); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default repositories role exists before lifecycle: %v", err)
	}
	git := &fakeGit{inventories: map[string]Inventory{
		layout.CloneDir(): repositoryInventory(
			layout.CloneDir(),
			"origin",
			"https://github.com/valter-silva-au/ai-dev-brain.git",
		),
	}}
	service := newTestRepositoryService(t, git, nil)
	addRequest := AddRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Mode:          AddModeCloneNew,
		Remote:        "https://github.com/valter-silva-au/ai-dev-brain.git",
		ActorType:     "human",
		Tool:          "adb-cli",
	}

	before := snapshotRepositoryWorkspace(t, root)
	planned, err := service.Add(context.Background(), addRequest)
	if err != nil {
		t.Fatalf("plan custom-role add: %v", err)
	}
	if planned.Outcome != capability.OutcomePlanned {
		t.Fatalf("custom-role plan outcome = %q", planned.Outcome)
	}
	if after := snapshotRepositoryWorkspace(t, root); !reflect.DeepEqual(
		after,
		before,
	) {
		t.Fatal("custom-role add preview mutated workspace")
	}

	addRequest.Apply = true
	added, err := service.Add(context.Background(), addRequest)
	if err != nil {
		t.Fatalf("apply custom-role add: %v", err)
	}
	if added.Outcome != capability.OutcomeApplied ||
		added.Data.Repository.OrganizationID != organizationID ||
		added.Data.Repository.Path != layout.Root() {
		t.Fatalf("custom-role add = %#v", added)
	}
	repeated, err := service.Add(context.Background(), addRequest)
	if err != nil || repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat custom-role add = %#v, %v", repeated, err)
	}
	shown, err := service.Show(context.Background(), ShowRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
	})
	if err != nil || shown.Data.Repository.Path != layout.Root() {
		t.Fatalf("show custom-role repository = %#v, %v", shown, err)
	}
	listed, err := service.List(context.Background(), ListRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
	})
	if err != nil || len(listed.Data.Repositories) != 1 ||
		listed.Data.Repositories[0].ID != added.Data.Repository.ID {
		t.Fatalf("list custom-role repositories = %#v, %v", listed, err)
	}
	healthy, err := service.Health(context.Background(), HealthRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
	})
	if err != nil || healthy.Data.State != HealthClean {
		t.Fatalf("health custom-role repository = %#v, %v", healthy, err)
	}

	inventory := git.inventories[layout.CloneDir()]
	inventory.Behind = 1
	git.inventories[layout.CloneDir()] = inventory
	updated, err := service.Update(context.Background(), UpdateRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	})
	if err != nil || updated.Outcome != capability.OutcomeApplied {
		t.Fatalf("update custom-role repository = %#v, %v", updated, err)
	}
	inventory.Behind = 0
	git.inventories[layout.CloneDir()] = inventory

	if _, err := service.Fetch(context.Background(), FetchRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}); err != nil {
		t.Fatalf("fetch custom-role repository: %v", err)
	}
	archived, err := service.Archive(context.Background(), ArchiveRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	})
	if err != nil || archived.Data.Repository.Status != StatusArchived {
		t.Fatalf("archive custom-role repository = %#v, %v", archived, err)
	}
	if _, err := service.Archive(context.Background(), ArchiveRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
		Restore:       true,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}); err != nil {
		t.Fatalf("restore custom-role repository: %v", err)
	}

	movedLayout, err := NewLayout(
		organizationLayout,
		"managed-repositories",
		"github.com",
		"valter-silva-au",
		"adb",
	)
	if err != nil {
		t.Fatalf("new moved custom-role layout: %v", err)
	}
	moved, err := service.Move(context.Background(), MoveRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      added.Data.Repository.ID,
		NewRemote:     "https://github.com/valter-silva-au/adb.git",
		UpdateRemote:  true,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	})
	if err != nil || moved.Data.Repository.Path != movedLayout.Root() {
		t.Fatalf("move custom-role repository = %#v, %v", moved, err)
	}

	registration := WorktreeRegistration{
		TicketKey: "ADB-39",
		Name:      "default",
		Path:      "work/ADB-39",
		Branch:    "feat/ADB-39-v3",
		Active:    false,
	}
	seedWorktreeRegistration(t, root, movedLayout, registration)
	worktreePath := filepath.Join(movedLayout.Root(), registration.Path)
	git.worktrees = map[string][]GitWorktree{
		moved.Data.Repository.ClonePath: {{
			Path:   moved.Data.Repository.ClonePath,
			Branch: "main",
		}},
	}
	repaired, err := service.WorktreeRepair(
		context.Background(),
		WorktreeRepairRequest{
			WorkspaceRoot: root,
			Organization:  "amazon",
			Selector:      moved.Data.Repository.ID,
			TicketKey:     registration.TicketKey,
			Name:          registration.Name,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil || repaired.Outcome != capability.OutcomeApplied {
		t.Fatalf("repair custom-role worktree = %#v, %v", repaired, err)
	}
	git.worktrees[moved.Data.Repository.ClonePath] = []GitWorktree{
		{Path: moved.Data.Repository.ClonePath, Branch: "main"},
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
			Selector:      moved.Data.Repository.ID,
			Path:          worktreePath,
			ActorType:     "human",
			Tool:          "adb-cli",
			Apply:         true,
		},
	)
	if err != nil || pruned.Outcome != capability.OutcomeApplied {
		t.Fatalf("prune custom-role worktree = %#v, %v", pruned, err)
	}

	source := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create external adoption source: %v", err)
	}
	authored := filepath.Join(source, "README.md")
	if err := os.WriteFile(authored, []byte("authored\n"), 0o644); err != nil {
		t.Fatalf("write external adoption source: %v", err)
	}
	git.inventories[source] = repositoryInventory(
		source,
		"origin",
		"https://github.com/valter-silva-au/external.git",
	)
	adopted, err := service.Adopt(context.Background(), AdoptRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Path:          source,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	})
	if err != nil || !adopted.Data.Repository.ExternalClone {
		t.Fatalf("adopt custom-role repository = %#v, %v", adopted, err)
	}
	if _, err := service.Health(context.Background(), HealthRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      adopted.Data.Repository.ID,
	}); err != nil {
		t.Fatalf("health external canonical clone: %v", err)
	}
	if _, err := service.Fetch(context.Background(), FetchRequest{
		WorkspaceRoot: root,
		Organization:  "amazon",
		Selector:      adopted.Data.Repository.ID,
		ActorType:     "human",
		Tool:          "adb-cli",
		Apply:         true,
	}); err != nil {
		t.Fatalf("fetch external canonical clone: %v", err)
	}
	if content, err := os.ReadFile(authored); err != nil ||
		string(content) != "authored\n" {
		t.Fatalf("external adoption source changed: %q, %v", content, err)
	}
	if _, err := os.Stat(defaultRepositories); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default repositories role was touched: %v", err)
	}
}

func initializeRepositoryWorkspaceWithRoles(
	t *testing.T,
	organizationsRole string,
	repositoriesRole string,
) (string, string, organization.Layout) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "AWS")
	foundationService, err := foundation.NewService(foundation.Options{})
	if err != nil {
		t.Fatalf("new foundation service: %v", err)
	}
	if _, err := foundationService.Initialize(
		context.Background(),
		foundation.InitializeRequest{Root: root, Name: "AWS", Apply: true},
	); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	workspaceLayout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	workspaceManifest, err := workspace.ReadManifest(
		workspaceLayout.ManifestPath(),
	)
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	workspaceManifest.Roles.Organizations = organizationsRole
	var workspaceContent bytes.Buffer
	if err := workspace.EncodeManifest(&workspaceContent, workspaceManifest); err != nil {
		t.Fatalf("encode workspace manifest: %v", err)
	}
	if err := os.WriteFile(
		workspaceLayout.ManifestPath(),
		workspaceContent.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write workspace manifest: %v", err)
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
	organizationLayout, err := organization.NewLayoutForRole(
		root,
		organizationsRole,
		"amazon",
	)
	if err != nil {
		t.Fatalf("new custom organization layout: %v", err)
	}
	organizationManifest, err := organization.ReadManifest(
		organizationLayout.ManifestPath(),
	)
	if err != nil {
		t.Fatalf("read organization manifest: %v", err)
	}
	organizationManifest.Roles.Repositories = repositoriesRole
	var organizationContent bytes.Buffer
	if err := organization.EncodeManifest(
		&organizationContent,
		organizationManifest,
	); err != nil {
		t.Fatalf("encode organization manifest: %v", err)
	}
	if err := os.WriteFile(
		organizationLayout.ManifestPath(),
		organizationContent.Bytes(),
		0o644,
	); err != nil {
		t.Fatalf("write organization manifest: %v", err)
	}
	return root, result.Data.OrganizationID, organizationLayout
}

func redirectRepositoryControlDir(
	t *testing.T,
	layout Layout,
	replacements map[string][]byte,
) string {
	t.Helper()

	if err := os.MkdirAll(layout.Root(), 0o755); err != nil {
		t.Fatalf("create repository root: %v", err)
	}
	external := filepath.Join(
		filepath.Dir(layout.RepositoriesRoot()),
		fmt.Sprintf("external-control-%d", time.Now().UnixNano()),
	)
	if err := os.Rename(layout.ControlDir(), external); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("move repository control dir: %v", err)
		}
		if err := os.MkdirAll(external, 0o755); err != nil {
			t.Fatalf("create external control dir: %v", err)
		}
	}
	for name, content := range replacements {
		if err := os.WriteFile(
			filepath.Join(external, name),
			content,
			0o644,
		); err != nil {
			t.Fatalf("write external %s: %v", name, err)
		}
	}
	if err := os.Symlink(external, layout.ControlDir()); err != nil {
		t.Fatalf("redirect repository control dir: %v", err)
	}
	return external
}

func newPostPlanRepositoryService(
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
				13,
				0,
				index,
				0,
				time.UTC,
			)
		},
		IDGenerator: func() string {
			index++
			return fmt.Sprintf("repository-boundary-%02d", index)
		},
		Git:       git,
		AfterPlan: afterPlan,
	})
	if err != nil {
		t.Fatalf("new post-plan repository service: %v", err)
	}
	return service
}

func prepareBoundaryWorktree(
	t *testing.T,
	root string,
	git *fakeGit,
	clonePath string,
	layout Layout,
) string {
	t.Helper()

	registration := WorktreeRegistration{
		TicketKey: "ADB-39",
		Name:      "default",
		Path:      "work/ADB-39",
		Branch:    "feat/ADB-39-v3",
		Active:    false,
	}
	seedWorktreeRegistration(t, root, layout, registration)
	worktreePath := filepath.Join(layout.Root(), registration.Path)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create registered worktree path: %v", err)
	}
	git.worktrees = map[string][]GitWorktree{
		clonePath: {
			{Path: clonePath, Branch: "main"},
			{Path: worktreePath, Branch: registration.Branch},
		},
	}
	git.inventories[worktreePath] = Inventory{
		IsRepository: true,
		Branch:       registration.Branch,
	}
	return worktreePath
}

func redirectRepositoriesRole(t *testing.T, root string) string {
	t.Helper()

	organizationLayout, err := organization.NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	manifest, err := organization.ReadManifest(
		organizationLayout.ManifestPath(),
	)
	if err != nil {
		t.Fatalf("read organization manifest: %v", err)
	}
	role, err := organizationLayout.ResolveRole(manifest.Roles.Repositories)
	if err != nil {
		t.Fatalf("resolve repositories role: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external-repositories")
	if err := os.Rename(role, external); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("move repositories role outside workspace: %v", err)
		}
		if err := os.MkdirAll(external, 0o755); err != nil {
			t.Fatalf("create external repositories role: %v", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(external, "boundary-marker"),
		[]byte("outside\n"),
		0o644,
	); err != nil {
		t.Fatalf("write external boundary marker: %v", err)
	}
	if err := os.Symlink(external, role); err != nil {
		t.Fatalf("redirect repositories role: %v", err)
	}
	return external
}

func snapshotRepositoryEvents(t *testing.T, root string) map[string]string {
	t.Helper()

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	return snapshotRepositoryWorkspace(t, layout.EventsDir())
}

func assertRepositoryBoundaryUnchanged(
	t *testing.T,
	root string,
	external string,
	beforeExternal map[string]string,
	beforeJournal map[string]string,
) {
	t.Helper()

	if after := snapshotRepositoryWorkspace(t, external); !reflect.DeepEqual(
		after,
		beforeExternal,
	) {
		t.Fatalf(
			"external repositories changed\n got: %#v\nwant: %#v",
			after,
			beforeExternal,
		)
	}
	if after := snapshotRepositoryEvents(t, root); !reflect.DeepEqual(
		after,
		beforeJournal,
	) {
		t.Fatalf(
			"repository journal changed\n got: %#v\nwant: %#v",
			after,
			beforeJournal,
		)
	}
}

func resetRepositoryGitCalls(git *fakeGit) {
	git.inventoryCalls = nil
	git.cloneCalls = nil
	git.initCalls = nil
	git.addRemoteCalls = nil
	git.setRemoteCalls = nil
	git.fetchCalls = nil
	git.fastForwardCalls = nil
	git.worktreeCalls = nil
	git.addWorktreeCalls = nil
	git.removeWorktreeCalls = nil
}

func assertNoRepositoryGitCalls(t *testing.T, git *fakeGit) {
	t.Helper()

	if len(git.inventoryCalls) != 0 ||
		len(git.cloneCalls) != 0 ||
		len(git.initCalls) != 0 ||
		len(git.addRemoteCalls) != 0 ||
		len(git.setRemoteCalls) != 0 ||
		len(git.fetchCalls) != 0 ||
		len(git.fastForwardCalls) != 0 ||
		len(git.worktreeCalls) != 0 ||
		len(git.addWorktreeCalls) != 0 ||
		len(git.removeWorktreeCalls) != 0 {
		t.Fatalf("redirected role executed git: %#v", git)
	}
}

func assertRepositoryJournalHasNoAppliedStep(
	t *testing.T,
	root string,
	operationID string,
) {
	t.Helper()

	if operationID == "" {
		t.Fatal("repository operation id is empty")
	}
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	operationDir := filepath.Join(layout.EventsDir(), operationID)
	err = filepath.WalkDir(
		operationDir,
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
			if strings.Contains(string(content), string(journal.PhaseStepApplied)) {
				t.Fatalf("journal contains applied step: %s", path)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspect repository journal: %v", err)
	}
}

func assertRepositoryJournalNotCommitted(
	t *testing.T,
	root string,
	operationID string,
) {
	t.Helper()

	if operationID == "" {
		t.Fatal("repository operation id is empty")
	}
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new workspace layout: %v", err)
	}
	operationDir := filepath.Join(layout.EventsDir(), operationID)
	err = filepath.WalkDir(
		operationDir,
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
			if strings.Contains(string(content), string(journal.PhaseCommitted)) {
				t.Fatalf("journal contains committed event: %s", path)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("inspect repository journal: %v", err)
	}
}

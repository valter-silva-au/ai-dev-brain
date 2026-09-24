package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ===== The ambient-input contract (AppOptions) ==============================
//
// Every test in this package that builds an App uses NewAppIsolated, NOT NewApp.
// A t.TempDir() basePath is NOT enough on its own: it pins only the REPO config
// tier (<basePath>/.taskrc), while three inputs stay ambient —
//
//   - the GLOBAL config tier defaults to $HOME/.taskconfig (NewViperConfigManager
//     fills an empty path in from os.UserHomeDir);
//   - the active org tier comes from $ADB_ORG.
//
// So NewApp(t.TempDir()) READS the developer's config (hook gates,
// task_id_prefix, an absolute hooks.memory.db_path).
//
// This used to be fixed per-test with t.Setenv("HOME", …). NewAppIsolated
// replaces that for two reasons: it cannot be forgotten by the next author (the
// constructor is the isolation), and t.Setenv forbids t.Parallel outright, so the
// env approach forced every hermetic test to be serial.
//
// The two helpers below build the hostile environment the contract test proves
// NewAppIsolated ignores. Their t.Setenv calls are the one legitimate remaining
// use in this file: they ESTABLISH the ambient state under test rather than
// working around it.

// Org ids, one per input that can select the middle tier, so a resolved
// MergedConfig.Org NAMES which input won rather than merely differing.
const (
	hostileOrgID   = "hostile-org"   // only $ADB_ORG names this
	workspaceOrgID = "workspace-org" // only the workspace's own .taskrc names this
	pinnedOrgID    = "pinned-org"    // only AppOptions.Org names this
)

// hostileGlobalConfig is a global tier that agrees with nothing a test wants: a
// task_id_prefix no fixture uses and hooks.memory switched on. Written into a
// fake $HOME it stands in for the developer's real ~/.taskconfig, which on this
// repo's own workstation does enable an Ollama-backed memory hook.
const hostileGlobalConfig = "task_id_prefix: HOSTILE\nhooks:\n  memory:\n    enabled: true\n"

// hostileEnv points $HOME at a temp dir carrying hostileGlobalConfig and exports
// $ADB_ORG, then returns that home. This is the environment an un-isolated App
// inherits.
func hostileEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".taskconfig"), []byte(hostileGlobalConfig), 0o644); err != nil {
		t.Fatalf("seed hostile global config: %v", err)
	}
	t.Setenv("HOME", home)            // os.UserHomeDir on unix
	t.Setenv("USERPROFILE", home)     // os.UserHomeDir on windows
	t.Setenv("ADB_ORG", hostileOrgID) // the ambient org tier
	return home
}

// hostileWorkspace seeds a workspace whose own data DISAGREES with the ambient
// environment: .taskrc selects workspaceOrgID, and a config.yaml exists for every
// org any case can resolve.
//
// All three org dirs are needed because GetOrgConfig returns (nil, nil) for an
// org with no config.yaml — so if only one existed, a wrong winner would show up
// as "no org tier at all" and the test could not tell which input had won.
func hostileWorkspace(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, ".taskrc"),
		[]byte("repo_name: contract\norg: "+workspaceOrgID+"\n"), 0o644); err != nil {
		t.Fatalf("seed .taskrc: %v", err)
	}
	for _, org := range []string{hostileOrgID, workspaceOrgID, pinnedOrgID} {
		dir := filepath.Join(base, "orgs", org)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("seed org dir %s: %v", org, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
			[]byte("org_id: "+org+"\n"), 0o644); err != nil {
			t.Fatalf("seed org config %s: %v", org, err)
		}
	}
	return base
}

// writeGlobalTier writes a global-tier .taskconfig declaring prefix at path.
func writeGlobalTier(t *testing.T, path, prefix string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("task_id_prefix: "+prefix+"\n"), 0o644); err != nil {
		t.Fatalf("write global tier %s: %v", path, err)
	}
}

// TestAppConstructors_AmbientInputContract pins what each constructor resolves
// from the environment, against a $HOME and $ADB_ORG that are actively hostile.
//
// The observables are one per ambient input: the global tier's task_id_prefix
// plus its hooks.memory flag (the GLOBAL config path), and the resolved
// MergedConfig.Org id (the ORG tier).
//
// A third observable — where a terminal-state write landed — was dropped with the
// VS Code integration. It was the assertion that caught the original bug (two
// tests silently accreting entries into a real home), so if a future ambient
// $HOME-resolving path is added, give it an observable here too.
func TestAppConstructors_AmbientInputContract(t *testing.T) {
	// Not parallel, and cannot be: t.Setenv is what establishes the hostile
	// environment, and the Go testing package forbids combining the two.
	cases := []struct {
		name string
		// seed runs before the App is built, for a case that needs an extra file
		// (a pinned or basePath-local global tier).
		seed func(t *testing.T, base string)
		// build constructs the App under test.
		build func(t *testing.T, base string) (*App, error)
		// wantPrefix is MergedConfig.Global.TaskIDPrefix — "TASK" is the
		// DefaultGlobalConfig value, i.e. "no global file was read".
		wantPrefix string
		// wantOrg is MergedConfig.Org.OrgID — it names the winning input.
		wantOrg string
		// The two bools sit last to keep the struct packed; each is read
		// alongside a field above it.
		//
		// wantMemory is ResolvedHooks().Memory.Enabled; true only when the hostile
		// global tier was merged.
		wantMemory bool
	}{
		{
			// The documented behaviour of the CLI's constructor. Asserted, not
			// merely tolerated: NewApp is what cmd/adb uses, so "it reads $HOME
			// and $ADB_ORG" is a contract, and a future change that quietly made
			// NewApp hermetic would break the CLI while looking like a cleanup.
			name:  "NewApp inherits the ambient environment",
			build: func(t *testing.T, base string) (*App, error) { return NewApp(base) },
			// $ADB_ORG outranks .taskrc's `org:` for an un-isolated App.
			wantPrefix: "HOSTILE", wantMemory: true, wantOrg: hostileOrgID,
		},
		{
			// The constructor every other test in this package uses.
			name:  "NewAppIsolated ignores a hostile $HOME and $ADB_ORG",
			build: func(t *testing.T, base string) (*App, error) { return NewAppIsolated(base) },
			// No <base>/.taskconfig exists, so the global tier is defaults —
			// and .taskrc's `org:` still selects the middle tier, because that
			// is workspace DATA, not ambient state.
			wantPrefix: "TASK", wantMemory: false, wantOrg: workspaceOrgID,
		},
		{
			// Isolation relocates the global tier rather than removing it, so a
			// test can still exercise Repo > Org > Global by writing the file.
			name: "NewAppIsolated reads a global .taskconfig under basePath",
			seed: func(t *testing.T, base string) {
				writeGlobalTier(t, filepath.Join(base, ".taskconfig"), "WSGLOBAL")
			},
			build:      func(t *testing.T, base string) (*App, error) { return NewAppIsolated(base) },
			wantPrefix: "WSGLOBAL", wantMemory: false, wantOrg: workspaceOrgID,
		},
		{
			name: "NewAppWithOptions pins GlobalConfigPath over the isolated default",
			seed: func(t *testing.T, base string) {
				writeGlobalTier(t, filepath.Join(base, "pinned.taskconfig"), "PINNED")
			},
			build: func(t *testing.T, base string) (*App, error) {
				return NewAppWithOptions(base, AppOptions{
					Isolated:         true,
					GlobalConfigPath: filepath.Join(base, "pinned.taskconfig"),
				})
			},
			wantPrefix: "PINNED", wantMemory: false, wantOrg: workspaceOrgID,
		},
		{
			// Org pins the tier outright: unlike Isolated, it suppresses the
			// workspace's own `org:` field as well as $ADB_ORG.
			name: "NewAppWithOptions Org suppresses both .taskrc and $ADB_ORG",
			build: func(t *testing.T, base string) (*App, error) {
				return NewAppWithOptions(base, AppOptions{Isolated: true, Org: pinnedOrgID})
			},
			wantPrefix: "TASK", wantMemory: false, wantOrg: pinnedOrgID,
		},
		{
			// Every field pinned and Isolated UNSET: the explicit paths alone are
			// enough to keep the App out of $HOME, so Isolated is a shorthand for
			// the common case rather than the only way to be hermetic.
			name: "NewAppWithOptions pins every field without Isolated",
			seed: func(t *testing.T, base string) {
				writeGlobalTier(t, filepath.Join(base, "pinned.taskconfig"), "PINNED")
			},
			build: func(t *testing.T, base string) (*App, error) {
				return NewAppWithOptions(base, AppOptions{
					GlobalConfigPath: filepath.Join(base, "pinned.taskconfig"),
					Org:              pinnedOrgID,
				})
			},
			wantPrefix: "PINNED", wantMemory: false, wantOrg: pinnedOrgID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostileEnv(t)
			base := hostileWorkspace(t)
			if tc.seed != nil {
				tc.seed(t, base)
			}

			app, err := tc.build(t, base)
			if err != nil {
				t.Fatalf("build App: %v", err)
			}
			t.Cleanup(func() { _ = app.Cleanup() })

			// --- the GLOBAL config tier -----------------------------------
			if got := app.MergedConfig.Global.TaskIDPrefix; got != tc.wantPrefix {
				t.Errorf("Global.TaskIDPrefix = %q, want %q — the wrong global tier was read",
					got, tc.wantPrefix)
			}
			if got := app.MergedConfig.ResolvedHooks().Memory.Enabled; got != tc.wantMemory {
				t.Errorf("ResolvedHooks().Memory.Enabled = %v, want %v", got, tc.wantMemory)
			}

			// --- the ORG tier ---------------------------------------------
			if app.MergedConfig.Org == nil {
				t.Fatalf("no org tier resolved; want %q", tc.wantOrg)
			}
			if got := app.MergedConfig.Org.OrgID; got != tc.wantOrg {
				t.Errorf("Org.OrgID = %q, want %q — the wrong input selected the org tier",
					got, tc.wantOrg)
			}

		})
	}
}

// TestResolveWorktreesDir covers threading RepoConfig.worktree_base_path into
// worktree creation instead of the hardcoded "<base>/work" (#206, F2c).
func TestResolveWorktreesDir(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	abs := filepath.Join(t.TempDir(), "elsewhere")
	cases := []struct {
		name string
		repo *models.RepoConfig
		want string
	}{
		{"nil repo falls back to work", nil, filepath.Join(base, "work")},
		{"empty base path falls back to work", &models.RepoConfig{}, filepath.Join(base, "work")},
		{"relative path is joined under base", &models.RepoConfig{WorktreeBasePath: "trees"}, filepath.Join(base, "trees")},
		{"absolute path is used verbatim", &models.RepoConfig{WorktreeBasePath: abs}, abs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveWorktreesDir(base, tc.repo); got != tc.want {
				t.Errorf("resolveWorktreesDir = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveWorktreeBaseBranch covers threading RepoConfig.base_branch into
// worktree creation instead of the hardcoded "main", so a repo whose default
// branch is master/develop lands on the right base (#206, F2c).
func TestResolveWorktreeBaseBranch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		repo *models.RepoConfig
		want string
	}{
		{"nil repo falls back to main", nil, "main"},
		{"empty base branch falls back to main", &models.RepoConfig{}, "main"},
		{"configured master is honoured", &models.RepoConfig{BaseBranch: "master"}, "master"},
		{"configured develop is honoured", &models.RepoConfig{BaseBranch: "develop"}, "develop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveWorktreeBaseBranch(tc.repo); got != tc.want {
				t.Errorf("resolveWorktreeBaseBranch = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewApp(t *testing.T) {
	t.Parallel()
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Test creating app
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	// Verify app structure
	if app == nil {
		t.Fatal("NewApp() returned nil app")
	}

	// Verify base path
	if app.BasePath != tmpDir {
		t.Errorf("BasePath = %v, want %v", app.BasePath, tmpDir)
	}

	// Verify configuration subsystem
	if app.ConfigManager == nil {
		t.Error("ConfigManager is nil")
	}
	if app.MergedConfig == nil {
		t.Error("MergedConfig is nil")
	}

	// Verify storage subsystem
	if app.BacklogManager == nil {
		t.Error("BacklogManager is nil")
	}
	if app.ContextManager == nil {
		t.Error("ContextManager is nil")
	}
	if app.SessionStoreManager == nil {
		t.Error("SessionStoreManager is nil")
	}

	// Verify core services
	if app.TaskIDGenerator == nil {
		t.Error("TaskIDGenerator is nil")
	}
	if app.TemplateManager == nil {
		t.Error("TemplateManager is nil")
	}
	if app.TaskManager == nil {
		t.Error("TaskManager is nil")
	}
	if app.AIContextGenerator == nil {
		t.Error("AIContextGenerator is nil (rich generator not wired)")
	}

	// Verify integration subsystem
	if app.GitWorktreeManager == nil {
		t.Error("GitWorktreeManager is nil")
	}

	// Verify observability subsystem
	if app.EventLog == nil {
		t.Error("EventLog is nil")
	}
	if app.MetricsCalculator == nil {
		t.Error("MetricsCalculator is nil")
	}
	if app.AlertEvaluator == nil {
		t.Error("AlertEvaluator is nil")
	}
}

// TestApp_AIContextGenerator_ProducesRichContext verifies the AIContextGenerator
// wired in NewApp is the rich multi-section generator: calling Generate() writes
// a multi-section CLAUDE.md and maintains .context_state.yaml (the T3 wiring —
// the generator was previously dead code, never constructed).
func TestApp_AIContextGenerator_ProducesRichContext(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	if app.AIContextGenerator == nil {
		t.Fatal("AIContextGenerator is nil")
	}

	// Seed an active task so the Active Tasks section has content.
	task := models.NewTask("TASK-00001", "Wire the rich generator", models.TaskTypeFeat)
	task.Status = models.TaskStatusInProgress
	if err := app.BacklogManager.AddTask(*task); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	if err := app.AIContextGenerator.Generate(); err != nil {
		t.Fatalf("AIContextGenerator.Generate() error = %v", err)
	}

	// Multi-section body (this is what distinguishes the rich generator from the
	// trivial one, which only emits Overview + Current Backlog). It lands in the
	// agent-agnostic AGENTS.md; CLAUDE.md is a pointer to it (TASK-00039).
	agentsPath := filepath.Join(tmpDir, core.CanonicalInstructionFile)
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", core.CanonicalInstructionFile, err)
	}
	content := string(data)
	for _, section := range []string{
		"## What's Changed",
		"## Active Tasks",
		"## Critical Decisions",
		"## Captured Sessions",
		"## Stakeholders & Contacts",
	} {
		if !strings.Contains(content, section) {
			t.Errorf("rich AGENTS.md missing section %q", section)
		}
	}
	if !strings.Contains(content, "TASK-00001") {
		t.Error("rich AGENTS.md should list the active task TASK-00001")
	}

	// context_state.yaml must have been written (the section-hash mechanism).
	// It now lives under .adb/ (#186/#189), not at the workspace root.
	statePath := filepath.Join(tmpDir, ".adb", "context_state.yaml")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error(".adb/context_state.yaml was not written by the rich generator")
	}
}

func TestNewApp_EmptyBasePath(t *testing.T) {
	// NewApp now ensures a .adb/ state dir at init (issue #186). With an empty
	// base path that resolves to ".", so run in a throwaway cwd to keep the
	// eager MkdirAll from polluting the package directory during `go test`.
	//
	// No t.Parallel here: t.Chdir mutates process-wide state, and the testing
	// package forbids the combination outright.
	t.Chdir(t.TempDir())

	// Test with empty base path (should use ".")
	app, err := NewAppIsolated("")
	if err != nil {
		t.Fatalf("NewApp(\"\") error = %v", err)
	}

	if app.BasePath != "." {
		t.Errorf("BasePath = %v, want %v", app.BasePath, ".")
	}
}

// TestApp_StatePath pins the state-path convention (#186, ticket #187): every
// adb-owned state file lives at <BasePath>/.adb/<name>. This is the single seam
// the whole .adb/ consolidation routes through, so a table test guards it
// against silent regression. Mirrors the path table tests in internal/core.
func TestApp_StatePath(t *testing.T) {
	t.Parallel()
	app := &App{BasePath: filepath.Join("/ws", "root")}

	// Representative names spanning the relocated set: the scheduler triad, the
	// counters, the event logs, and the SQLite memory store.
	cases := []string{
		"scheduler.log",
		"scheduler.pid",
		"scheduler_state.yaml",
		"task_counter",
		"session_counter",
		"context_state.yaml",
		"events.jsonl",
		"governance.jsonl",
		"memory.sqlite",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			want := filepath.Join(app.BasePath, ".adb", name)
			if got := app.StatePath(name); got != want {
				t.Errorf("StatePath(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// TestNewApp_CreatesStateDir verifies the expand-step guarantee: App init
// creates the .adb/ directory when absent, so a freshly initialised workspace
// has somewhere for state to land without a migration.
func TestNewApp_CreatesStateDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	if _, err := NewAppIsolated(tmp); err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(tmp, ".adb"))
	if err != nil {
		t.Fatalf(".adb/ was not created at App init: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".adb exists but is not a directory")
	}
}

// TestNewApp_PreservesExistingStateDir guards user story 2/19: creating the
// .adb/ dir must never clobber what is already there (a live workspace's
// .adb/claude-user.md and, post-migration, all its state).
func TestNewApp_PreservesExistingStateDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	adbDir := filepath.Join(tmp, ".adb")
	if err := os.MkdirAll(adbDir, 0o755); err != nil {
		t.Fatalf("seed .adb/: %v", err)
	}
	marker := filepath.Join(adbDir, "claude-user.md")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	if _, err := NewAppIsolated(tmp); err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("existing .adb/ content lost: %v", err)
	}
	if string(got) != "keep me" {
		t.Errorf("existing .adb/ content overwritten: got %q", got)
	}
}

// TestEdgeWriterAdapter_AddEdgeFromIngestedNode guards #174: an edge whose `from`
// is an ingested node (not a task or initiative) must be landable — otherwise a
// well-formed edge proposal from an ingested node can never be accepted (it
// re-queues forever). Before the fix AddEdge only resolved tasks + initiatives.
func TestEdgeWriterAdapter_AddEdgeFromIngestedNode(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	nodes := storage.NewFileNodeStore(tmp)
	// A node minted by the ingestion pipeline.
	if err := nodes.Put(models.IngestedNode{ID: "stakeholder:acme", Type: "stakeholder", Title: "Acme"}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	adapter := &edgeWriterAdapter{
		backlog: storage.NewFileBacklogManager(tmp),
		stage:   storage.NewFileStageStore(tmp),
		nodes:   nodes,
	}

	link := models.Link{Type: models.EdgeRelatesTo, Target: "system:widget"}
	if err := adapter.AddEdge("stakeholder:acme", link); err != nil {
		t.Fatalf("AddEdge from an ingested node should succeed, got: %v", err)
	}

	// The link landed on the node.
	got, found, err := nodes.Get("stakeholder:acme")
	if err != nil || !found {
		t.Fatalf("node lookup after AddEdge: found=%v err=%v", found, err)
	}
	if len(got.Links) != 1 || got.Links[0] != link {
		t.Errorf("node links = %+v, want exactly the added link", got.Links)
	}

	// Idempotent: a second identical AddEdge does not duplicate.
	if err := adapter.AddEdge("stakeholder:acme", link); err != nil {
		t.Fatalf("second AddEdge: %v", err)
	}
	got, _, _ = nodes.Get("stakeholder:acme")
	if len(got.Links) != 1 {
		t.Errorf("duplicate edge added: links = %+v", got.Links)
	}

	// A truly-unknown from still errors (no task, initiative, or node).
	if err := adapter.AddEdge("ghost:nope", link); err == nil {
		t.Error("AddEdge from an unknown entity should error")
	}
}

func TestApp_Integration_ConfigurationLoading(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a test .taskrc file
	taskrcContent := `task_id_prefix: "TEST"
base_branch: "develop"
reviewers:
  - "reviewer1"
  - "reviewer2"
`
	taskrcPath := filepath.Join(tmpDir, ".taskrc")
	if err := os.WriteFile(taskrcPath, []byte(taskrcContent), 0o644); err != nil {
		t.Fatalf("Failed to write .taskrc: %v", err)
	}

	// Initialize app
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	// Verify configuration was loaded
	if app.MergedConfig == nil {
		t.Fatal("MergedConfig is nil")
	}

	// Check that repo config was merged
	if app.MergedConfig.Repo == nil {
		t.Fatal("MergedConfig.Repo is nil")
	}

	if app.MergedConfig.Repo.BaseBranch != "develop" {
		t.Errorf("MergedConfig.Repo.BaseBranch = %v, want %v", app.MergedConfig.Repo.BaseBranch, "develop")
	}

	if len(app.MergedConfig.Repo.Reviewers) != 2 {
		t.Errorf("MergedConfig.Repo.Reviewers length = %v, want %v", len(app.MergedConfig.Repo.Reviewers), 2)
	}
}

func TestApp_Integration_Adapters(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	t.Run("BacklogStoreAdapter", func(t *testing.T) {
		// Test that we can add and retrieve tasks through the adapter
		task := models.NewTask("TEST-001", "Test Task", models.TaskTypeFeat)

		err := app.BacklogManager.AddTask(*task)
		if err != nil {
			t.Fatalf("BacklogManager.AddTask() error = %v", err)
		}

		retrieved, err := app.BacklogManager.GetTask("TEST-001")
		if err != nil {
			t.Fatalf("BacklogManager.GetTask() error = %v", err)
		}

		if retrieved.ID != "TEST-001" {
			t.Errorf("Retrieved task ID = %v, want %v", retrieved.ID, "TEST-001")
		}
	})

	t.Run("ContextStoreAdapter", func(t *testing.T) {
		// Create task directory first
		taskDir := filepath.Join(tmpDir, "tickets", "TEST-002")
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			t.Fatalf("Failed to create task directory: %v", err)
		}

		// Test writing and reading context
		content := "Test context content"
		err := app.ContextManager.WriteContext("TEST-002", content)
		if err != nil {
			t.Fatalf("ContextManager.WriteContext() error = %v", err)
		}

		retrieved, err := app.ContextManager.ReadContext("TEST-002")
		if err != nil {
			t.Fatalf("ContextManager.ReadContext() error = %v", err)
		}

		if retrieved != content {
			t.Errorf("Retrieved context = %v, want %v", retrieved, content)
		}
	})

	t.Run("EventLoggerAdapter", func(t *testing.T) {
		// Test that event logging works
		app.EventLog.Log("test.event", map[string]interface{}{
			"test_key": "test_value",
		})

		// Read events back
		events, err := app.EventLog.ReadAll()
		if err != nil {
			t.Fatalf("EventLog.ReadAll() error = %v", err)
		}

		// Should have at least one event
		if len(events) == 0 {
			t.Error("No events logged")
		}
	})
}

func TestApp_Cleanup(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	// Test cleanup
	err = app.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}
}

func TestAdapters_BacklogStore(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	// Create adapter directly to test all methods
	adapter := &backlogStoreAdapter{manager: app.BacklogManager}

	task := models.NewTask("ADAPT-001", "Adapter Test", models.TaskTypeFeat)

	// Test AddTask
	if err := adapter.AddTask(*task); err != nil {
		t.Fatalf("AddTask() error = %v", err)
	}

	// Test GetTask
	got, err := adapter.GetTask("ADAPT-001")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.ID != "ADAPT-001" {
		t.Errorf("GetTask().ID = %v, want ADAPT-001", got.ID)
	}

	// Test UpdateTask
	task.Title = "Updated Title"
	if err := adapter.UpdateTask(*task); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	// Test Load
	backlog, err := adapter.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(backlog.Tasks) != 1 {
		t.Errorf("Load() tasks = %d, want 1", len(backlog.Tasks))
	}

	// Test Save
	if err := adapter.Save(backlog); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Test RemoveTask
	if err := adapter.RemoveTask("ADAPT-001"); err != nil {
		t.Fatalf("RemoveTask() error = %v", err)
	}
}

func TestAdapters_ContextStore(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &contextStoreAdapter{manager: app.ContextManager}

	// Create task directory
	taskDir := filepath.Join(tmpDir, "tickets", "CTX-001")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	// Test WriteContext / ReadContext
	if err := adapter.WriteContext("CTX-001", "test context"); err != nil {
		t.Fatalf("WriteContext() error = %v", err)
	}
	ctx, err := adapter.ReadContext("CTX-001")
	if err != nil {
		t.Fatalf("ReadContext() error = %v", err)
	}
	if ctx != "test context" {
		t.Errorf("ReadContext() = %v, want 'test context'", ctx)
	}

	// Test AppendContext
	if err := adapter.AppendContext("CTX-001", "\nappended"); err != nil {
		t.Fatalf("AppendContext() error = %v", err)
	}

	// Test WriteNotes / ReadNotes
	if err := adapter.WriteNotes("CTX-001", "test notes"); err != nil {
		t.Fatalf("WriteNotes() error = %v", err)
	}
	notes, err := adapter.ReadNotes("CTX-001")
	if err != nil {
		t.Fatalf("ReadNotes() error = %v", err)
	}
	if notes != "test notes" {
		t.Errorf("ReadNotes() = %v, want 'test notes'", notes)
	}
}

func TestAdapters_WorktreeCreator(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &worktreeCreatorAdapter{manager: app.GitWorktreeManager, basePath: tmpDir}

	// Test with empty repoPath defaults to basePath
	// This will fail (no git repo at tmpDir) but exercises the code path
	err = adapter.CreateWorktree("TEST-001", "task/TEST-001", filepath.Join(tmpDir, "work", "TEST-001"), "")
	if err == nil {
		t.Error("CreateWorktree() with non-git basePath should fail")
	}

	// Test with explicit repoPath
	err = adapter.CreateWorktree("TEST-002", "task/TEST-002", filepath.Join(tmpDir, "work", "TEST-002"), "/nonexistent/repo")
	if err == nil {
		t.Error("CreateWorktree() with nonexistent repo should fail")
	}
}

func TestAdapters_WorktreeRemover(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &worktreeRemoverAdapter{manager: app.GitWorktreeManager}

	// Test with nonexistent path (force skips the dirty guard; the existence
	// check still fails first).
	err = adapter.RemoveWorktree("/nonexistent/worktree", true)
	if err == nil {
		t.Error("RemoveWorktree() with nonexistent path should fail")
	}
}

func TestAdapters_EventLogger(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &eventLoggerAdapter{log: app.EventLog}

	// Test logging (should not panic)
	adapter.Log("test.event", map[string]interface{}{
		"key": "value",
	})

	// Verify event was written
	events, err := app.EventLog.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(events) == 0 {
		t.Error("No events after Log()")
	}
}

func TestAdapters_SessionCapturer(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &sessionCapturerAdapter{manager: app.SessionStoreManager}

	// Test CaptureSession (currently returns error in adapter)
	err = adapter.CaptureSession("TASK-001", "S-001", map[string]interface{}{})
	if err == nil {
		t.Error("CaptureSession() should return error (not fully implemented)")
	}
}

func TestApp_GetSessionStore(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	store := app.GetSessionStore()
	if store == nil {
		t.Error("GetSessionStore() returned nil")
	}
}

func TestApp_Integration_NoCircularImports(t *testing.T) {
	t.Parallel()
	// This test verifies that the app can be constructed without circular import issues
	// If the test compiles and runs, it means there are no circular imports
	tmpDir := t.TempDir()

	app, err := NewAppIsolated(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	if app == nil {
		t.Fatal("NewApp() returned nil")
	}

	// Verify all major components are initialized
	components := map[string]interface{}{
		"ConfigManager":      app.ConfigManager,
		"BacklogManager":     app.BacklogManager,
		"ContextManager":     app.ContextManager,
		"TaskIDGenerator":    app.TaskIDGenerator,
		"TemplateManager":    app.TemplateManager,
		"TaskManager":        app.TaskManager,
		"GitWorktreeManager": app.GitWorktreeManager,
		"EventLog":           app.EventLog,
	}

	for name, component := range components {
		if component == nil {
			t.Errorf("Component %s is nil", name)
		}
	}
}

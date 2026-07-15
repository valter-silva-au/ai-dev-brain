package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestRollupSerenaEvents covers the pure #203 rollup: only
// serena.effectiveness_recorded events count; the average excludes score-0
// (unset) records; Recent is newest-first.
func TestRollupSerenaEvents(t *testing.T) {
	events := []observability.Event{
		{Type: observability.EventTaskCreated, Data: map[string]interface{}{"task_id": "x"}},
		{Type: observability.EventSerenaEffectivenessRecorded, Data: map[string]interface{}{"verdict": "helped", "score": float64(5), "used_for": "find_symbol"}},
		{Type: observability.EventSerenaEffectivenessRecorded, Data: map[string]interface{}{"verdict": "helped", "score": float64(3)}},
		{Type: observability.EventSerenaEffectivenessRecorded, Data: map[string]interface{}{"verdict": "unused", "score": float64(0)}},
	}
	r := rollupSerenaEvents(events)
	if r.Total != 3 {
		t.Errorf("Total = %d, want 3", r.Total)
	}
	if r.ByVerdict["helped"] != 2 || r.ByVerdict["unused"] != 1 {
		t.Errorf("ByVerdict = %v, want helped:2 unused:1", r.ByVerdict)
	}
	if r.AverageScore != 4.0 { // (5+3)/2 — score-0 excluded
		t.Errorf("AverageScore = %v, want 4.0", r.AverageScore)
	}
	if len(r.Recent) != 3 || r.Recent[0].Verdict != "unused" {
		t.Errorf("Recent should be newest-first with 3 entries, got %+v", r.Recent)
	}
}

// TestSerenaTelemetryAdapter_RecordThenReport verifies Record emits exactly one
// event per call and Report rolls the log back up (#203).
func TestSerenaTelemetryAdapter_RecordThenReport(t *testing.T) {
	log := observability.NewEventLog(filepath.Join(t.TempDir(), ".events.jsonl"))
	a := &serenaTelemetryAdapter{log: log}

	if err := a.Record(models.SerenaRecord{Verdict: "helped", Score: 4, UsedFor: "x", TaskID: "TASK-1"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := a.Record(models.SerenaRecord{Verdict: "unused", Score: 0}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Exactly one event per Record.
	events, err := log.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	serenaCount := 0
	for _, e := range events {
		if e.Type == observability.EventSerenaEffectivenessRecorded {
			serenaCount++
		}
	}
	if serenaCount != 2 {
		t.Errorf("expected 2 serena events, got %d", serenaCount)
	}

	r, err := a.Report()
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if r.Total != 2 || r.ByVerdict["helped"] != 1 || r.ByVerdict["unused"] != 1 {
		t.Errorf("rollup = %+v, want total 2, helped:1 unused:1", r)
	}
	if r.AverageScore != 4.0 { // only the score-4 record counts
		t.Errorf("AverageScore = %v, want 4.0", r.AverageScore)
	}
}

// TestResolveWorktreesDir covers threading RepoConfig.worktree_base_path into
// worktree creation instead of the hardcoded "<base>/work" (#206, F2c).
func TestResolveWorktreesDir(t *testing.T) {
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
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Test creating app
	app, err := NewApp(tmpDir)
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
	if app.TerminalStateWriter == nil {
		t.Error("TerminalStateWriter is nil")
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
	tmpDir := t.TempDir()

	app, err := NewApp(tmpDir)
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

	// Multi-section CLAUDE.md (this is what distinguishes the rich generator
	// from the trivial one, which only emits Overview + Current Backlog).
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
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
			t.Errorf("rich CLAUDE.md missing section %q", section)
		}
	}
	if !strings.Contains(content, "TASK-00001") {
		t.Error("rich CLAUDE.md should list the active task TASK-00001")
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
	t.Chdir(t.TempDir())

	// Test with empty base path (should use ".")
	app, err := NewApp("")
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
	tmp := t.TempDir()

	if _, err := NewApp(tmp); err != nil {
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
	tmp := t.TempDir()
	adbDir := filepath.Join(tmp, ".adb")
	if err := os.MkdirAll(adbDir, 0o755); err != nil {
		t.Fatalf("seed .adb/: %v", err)
	}
	marker := filepath.Join(adbDir, "claude-user.md")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	if _, err := NewApp(tmp); err != nil {
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
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()

	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()

	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
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

func TestAdapters_TerminalStateUpdater(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	adapter := &terminalStateUpdaterAdapter{writer: app.TerminalStateWriter}

	// Test with status in state map
	err = adapter.WriteTerminalState(tmpDir, "TASK-001", map[string]interface{}{
		"status": "in_progress",
	})
	if err != nil {
		t.Fatalf("WriteTerminalState() error = %v", err)
	}

	// Test without status key (uses default)
	err = adapter.WriteTerminalState(tmpDir, "TASK-002", map[string]interface{}{
		"other_key": "value",
	})
	if err != nil {
		t.Fatalf("WriteTerminalState() with default status error = %v", err)
	}
}

func TestApp_GetSessionStore(t *testing.T) {
	tmpDir := t.TempDir()
	app, err := NewApp(tmpDir)
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	store := app.GetSessionStore()
	if store == nil {
		t.Error("GetSessionStore() returned nil")
	}
}

func TestApp_Integration_NoCircularImports(t *testing.T) {
	// This test verifies that the app can be constructed without circular import issues
	// If the test compiles and runs, it means there are no circular imports
	tmpDir := t.TempDir()

	app, err := NewApp(tmpDir)
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

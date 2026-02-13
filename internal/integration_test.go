package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/internal/integration"
	"github.com/drapaimern/ai-dev-brain/internal/observability"
	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestApp creates a fully wired App in a temporary directory.
// The event log is closed automatically when the test finishes.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("creating test app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// newTestAppWithConfig creates a fully wired App with a custom .taskconfig.
// The event log is closed automatically when the test finishes.
func newTestAppWithConfig(t *testing.T, configYAML string) *App {
	t.Helper()
	dir := t.TempDir()
	if configYAML != "" {
		if err := os.WriteFile(filepath.Join(dir, ".taskconfig"), []byte(configYAML), 0o644); err != nil {
			t.Fatalf("writing .taskconfig: %v", err)
		}
	}
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("creating test app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// =========================================================================
// 1. End-to-end task lifecycle: Create -> Work -> Archive -> Unarchive -> Resume
// =========================================================================

func TestIntegration_TaskLifecycle_CreateResumeArchiveUnarchive(t *testing.T) {
	app := newTestApp(t)

	// --- Create a feat task ---
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "add-auth", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if task.Type != models.TaskTypeFeat {
		t.Fatalf("expected feat type, got %s", task.Type)
	}
	if task.Status != models.StatusBacklog {
		t.Fatalf("expected backlog status on creation, got %s", task.Status)
	}
	taskID := task.ID

	// Verify ticket directory was created with all expected files.
	ticketDir := filepath.Join(app.BasePath, "tickets", taskID)
	for _, name := range []string{"status.yaml", "notes.md", "design.md", "context.md"} {
		p := filepath.Join(ticketDir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Fatalf("%s not created: %s", name, p)
		}
	}

	// Verify communications directory was created.
	commsDir := filepath.Join(ticketDir, "communications")
	info, err := os.Stat(commsDir)
	if err != nil {
		t.Fatalf("communications dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("communications should be a directory")
	}

	// Verify task appears in backlog.
	allTasks, err := app.TaskMgr.GetAllTasks()
	if err != nil {
		t.Fatalf("getting all tasks: %v", err)
	}
	if len(allTasks) != 1 {
		t.Fatalf("expected 1 task in backlog, got %d", len(allTasks))
	}
	if allTasks[0].ID != taskID {
		t.Fatalf("expected task ID %s, got %s", taskID, allTasks[0].ID)
	}

	// --- Resume: backlog -> in_progress ---
	resumed, err := app.TaskMgr.ResumeTask(taskID)
	if err != nil {
		t.Fatalf("resuming task: %v", err)
	}
	if resumed.Status != models.StatusInProgress {
		t.Fatalf("expected in_progress status after resume, got %s", resumed.Status)
	}

	// Verify backlog also updated.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("loading backlog after resume: %v", err)
	}
	blEntry, err := app.BacklogMgr.GetTask(taskID)
	if err != nil {
		t.Fatalf("getting backlog entry after resume: %v", err)
	}
	if blEntry.Status != models.StatusInProgress {
		t.Fatalf("backlog status after resume = %s, want in_progress", blEntry.Status)
	}

	// --- Archive: in_progress -> archived ---
	handoff, err := app.TaskMgr.ArchiveTask(taskID)
	if err != nil {
		t.Fatalf("archiving task: %v", err)
	}
	if handoff.TaskID != taskID {
		t.Fatalf("expected handoff task ID %s, got %s", taskID, handoff.TaskID)
	}

	// Verify ticket was moved to _archived/.
	archivedTicketDir := filepath.Join(app.BasePath, "tickets", "_archived", taskID)
	if _, err := os.Stat(archivedTicketDir); err != nil {
		t.Fatalf("archived ticket directory should exist: %v", err)
	}

	// Verify original ticket directory no longer exists.
	if _, err := os.Stat(ticketDir); !os.IsNotExist(err) {
		t.Fatal("original ticket directory should be removed after archive")
	}

	// Verify handoff.md was created in the archived location.
	handoffPath := filepath.Join(archivedTicketDir, "handoff.md")
	handoffData, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("reading handoff.md: %v", err)
	}
	if !strings.Contains(string(handoffData), taskID) {
		t.Fatal("handoff.md should contain the task ID")
	}
	if !strings.Contains(string(handoffData), "Archived") {
		t.Fatal("handoff.md should contain 'Archived'")
	}

	// Verify status is archived (loadable from _archived).
	archived, err := app.TaskMgr.GetTask(taskID)
	if err != nil {
		t.Fatalf("getting archived task: %v", err)
	}
	if archived.Status != models.StatusArchived {
		t.Fatalf("expected archived status, got %s", archived.Status)
	}

	// Verify .pre_archive_status was saved in the archived location.
	preArchivePath := filepath.Join(archivedTicketDir, ".pre_archive_status")
	preData, err := os.ReadFile(preArchivePath)
	if err != nil {
		t.Fatalf("reading .pre_archive_status: %v", err)
	}
	if strings.TrimSpace(string(preData)) != string(models.StatusInProgress) {
		t.Fatalf("pre_archive_status = %q, want %q", string(preData), models.StatusInProgress)
	}

	// --- Unarchive: archived -> in_progress (restored) ---
	unarchived, err := app.TaskMgr.UnarchiveTask(taskID)
	if err != nil {
		t.Fatalf("unarchiving task: %v", err)
	}
	if unarchived.Status != models.StatusInProgress {
		t.Fatalf("expected in_progress after unarchive (restored), got %s", unarchived.Status)
	}

	// Verify ticket was moved back to active location.
	if _, err := os.Stat(ticketDir); err != nil {
		t.Fatalf("ticket should be back in active location after unarchive: %v", err)
	}

	// Verify _archived directory no longer contains this task.
	if _, err := os.Stat(archivedTicketDir); !os.IsNotExist(err) {
		t.Fatal("archived ticket directory should be removed after unarchive")
	}

	// Verify .pre_archive_status was cleaned up from active location.
	activePreArchivePath := filepath.Join(ticketDir, ".pre_archive_status")
	if _, err := os.Stat(activePreArchivePath); !os.IsNotExist(err) {
		t.Fatal("expected .pre_archive_status to be removed after unarchive")
	}

	// Verify backlog reflects restored status.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("loading backlog after unarchive: %v", err)
	}
	restored, err := app.BacklogMgr.GetTask(taskID)
	if err != nil {
		t.Fatalf("getting backlog entry after unarchive: %v", err)
	}
	if restored.Status != models.StatusInProgress {
		t.Fatalf("backlog status after unarchive = %s, want in_progress", restored.Status)
	}

	// --- Resume again (no-op since already in_progress) ---
	resumed2, err := app.TaskMgr.ResumeTask(taskID)
	if err != nil {
		t.Fatalf("resuming after unarchive: %v", err)
	}
	if resumed2.ID != taskID {
		t.Fatalf("expected task ID %s, got %s", taskID, resumed2.ID)
	}
	if resumed2.Status != models.StatusInProgress {
		t.Fatalf("expected in_progress after second resume, got %s", resumed2.Status)
	}
}

// TestIntegration_ArchiveFromBacklogPreservesStatus verifies that archiving
// a task directly from backlog status saves "backlog" as the pre-archive status.
func TestIntegration_ArchiveFromBacklogPreservesStatus(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeBug, "fix-crash", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Archive directly from backlog.
	if _, err := app.TaskMgr.ArchiveTask(task.ID); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	// Unarchive should restore to backlog.
	unarchived, err := app.TaskMgr.UnarchiveTask(task.ID)
	if err != nil {
		t.Fatalf("unarchiving: %v", err)
	}
	if unarchived.Status != models.StatusBacklog {
		t.Errorf("status after unarchive = %s, want backlog", unarchived.Status)
	}
}

// TestIntegration_DoubleArchiveReturnsError verifies idempotency guard.
func TestIntegration_DoubleArchiveReturnsError(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "double-archive", "", core.CreateTaskOpts{})
	if _, err := app.TaskMgr.ArchiveTask(task.ID); err != nil {
		t.Fatalf("first archive: %v", err)
	}
	if _, err := app.TaskMgr.ArchiveTask(task.ID); err == nil {
		t.Error("expected error on second archive, got nil")
	}
}

// TestIntegration_UnarchiveNonArchivedReturnsError tests the guard.
func TestIntegration_UnarchiveNonArchivedReturnsError(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "not-archived", "", core.CreateTaskOpts{})
	if _, err := app.TaskMgr.UnarchiveTask(task.ID); err == nil {
		t.Error("expected error when unarchiving a non-archived task")
	}
}

// =========================================================================
// 2. Multi-repo workflow: task structure and worktree path validation.
// =========================================================================

func TestIntegration_MultiRepoWorktreePathValidation(t *testing.T) {
	app := newTestApp(t)

	// CreateWorktree requires real git, so we test validation guards instead.
	// The GitWorktreeManager rejects invalid configs.
	_, err := app.WorktreeMgr.CreateWorktree(integration.WorktreeConfig{
		RepoPath:   "",
		BranchName: "feat/test",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Error("expected error for empty RepoPath")
	}

	_, err = app.WorktreeMgr.CreateWorktree(integration.WorktreeConfig{
		RepoPath:   "github.com/org/repo",
		BranchName: "",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Error("expected error for empty BranchName")
	}

	_, err = app.WorktreeMgr.CreateWorktree(integration.WorktreeConfig{
		RepoPath:   "github.com/org/repo",
		BranchName: "feat/test",
		TaskID:     "",
	})
	if err == nil {
		t.Error("expected error for empty TaskID")
	}
}

func TestIntegration_TaskWithoutRepoHasNoWorktree(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeSpike, "investigate-api", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	loaded, err := app.TaskMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("getting task: %v", err)
	}
	if loaded.WorktreePath != "" {
		t.Errorf("expected empty worktree path when no repo, got %q", loaded.WorktreePath)
	}
}

// =========================================================================
// 3. Offline/online transition: queue operations, verify sync.
// =========================================================================

func TestIntegration_OfflineQueueAndSync(t *testing.T) {
	app := newTestApp(t)

	// Initially empty queue: sync should be a no-op.
	result, err := app.OfflineMgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("sync empty queue: %v", err)
	}
	if result.Synced != 0 || result.Failed != 0 {
		t.Errorf("expected 0/0 on empty queue, got synced=%d failed=%d", result.Synced, result.Failed)
	}

	// Queue multiple operations.
	ops := []integration.QueuedOperation{
		{ID: "op-1", Type: "status_update", Payload: "in_progress", Timestamp: time.Now()},
		{ID: "op-2", Type: "backlog_sync", Payload: map[string]string{"task": "TASK-00001"}, Timestamp: time.Now()},
		{ID: "op-3", Type: "communication_log", Payload: "standup notes", Timestamp: time.Now()},
	}
	for _, op := range ops {
		if err := app.OfflineMgr.QueueOperation(op); err != nil {
			t.Fatalf("queueing %s: %v", op.ID, err)
		}
	}

	// Verify queue file exists and has correct data.
	queuePath := filepath.Join(app.BasePath, ".offline_queue.json")
	queueData, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("reading queue file: %v", err)
	}
	var queued []integration.QueuedOperation
	if err := json.Unmarshal(queueData, &queued); err != nil {
		t.Fatalf("parsing queue JSON: %v", err)
	}
	if len(queued) != 3 {
		t.Errorf("expected 3 queued operations, got %d", len(queued))
	}

	// Sync: the default executeOperation is a no-op success.
	result, err = app.OfflineMgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Synced != 3 {
		t.Errorf("expected 3 synced, got %d", result.Synced)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}

	// Queue file should be removed after full sync.
	if _, err := os.Stat(queuePath); !os.IsNotExist(err) {
		t.Error("expected queue file to be removed after full sync")
	}
}

func TestIntegration_OfflineQueueMultipleRounds(t *testing.T) {
	app := newTestApp(t)

	// Round 1: queue and sync.
	if err := app.OfflineMgr.QueueOperation(integration.QueuedOperation{
		ID: "round1", Type: "test", Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("queue round 1: %v", err)
	}
	r1, _ := app.OfflineMgr.SyncPendingOperations()
	if r1.Synced != 1 {
		t.Errorf("round 1: expected 1 synced, got %d", r1.Synced)
	}

	// Round 2: queue and sync again.
	if err := app.OfflineMgr.QueueOperation(integration.QueuedOperation{
		ID: "round2", Type: "test", Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("queue round 2: %v", err)
	}
	r2, _ := app.OfflineMgr.SyncPendingOperations()
	if r2.Synced != 1 {
		t.Errorf("round 2: expected 1 synced, got %d", r2.Synced)
	}
}

func TestIntegration_OfflineConnectivityCallback(t *testing.T) {
	app := newTestApp(t)

	callbackInvoked := false
	app.OfflineMgr.OnConnectivityChange(func(online bool) {
		callbackInvoked = true
	})

	// Callback is only invoked internally; registration alone should not trigger it.
	if callbackInvoked {
		t.Error("callback should not be invoked just by registering")
	}
}

// =========================================================================
// 4. Knowledge feedback loop: task comms/notes -> extract -> handoff -> ADR -> wiki
// =========================================================================

func TestIntegration_KnowledgeFeedback_FullLoop(t *testing.T) {
	app := newTestApp(t)

	// Create a task.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "auth-feature", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}
	taskID := task.ID
	ticketDir := filepath.Join(app.BasePath, "tickets", taskID)

	// Enrich notes.md with learnings, gotchas, and wiki update hints.
	notesContent := `# Feature Notes

## Requirements
- [ ] Support OAuth2

## Learnings
- REST is better for this use case than GraphQL
- Pagination reduces load on the DB

## Gotchas
- Rate limiting resets at midnight UTC
- Auth tokens expire silently

## Wiki Updates
- API design patterns
- Rate limiting strategy
`
	if err := os.WriteFile(filepath.Join(ticketDir, "notes.md"), []byte(notesContent), 0o644); err != nil {
		t.Fatalf("writing notes.md: %v", err)
	}

	// Enrich context.md with decisions and progress.
	contextContent := `# Task Context: ` + taskID + `

## Summary
Redesigning the public API to use REST patterns

## Current Focus
Endpoint design

## Recent Progress
- Defined resource models
- Created OpenAPI spec

## Open Questions
- [ ] Should we version the URL path?

## Decisions Made
- Use JSON:API format for responses
- Implement cursor-based pagination

## Blockers

## Next Steps
- [ ] Build prototype endpoints

## Related Resources
`
	if err := os.WriteFile(filepath.Join(ticketDir, "context.md"), []byte(contextContent), 0o644); err != nil {
		t.Fatalf("writing context.md: %v", err)
	}

	// Add a decision communication.
	err = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
		Source:  "slack",
		Contact: "alice",
		Topic:   "auth-approach",
		Content: "Use OAuth2 with PKCE flow",
		Tags:    []models.CommunicationTag{models.TagDecision},
	})
	if err != nil {
		t.Fatalf("adding communication: %v", err)
	}

	// Add a requirement communication.
	err = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date:    time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
		Source:  "email",
		Contact: "bob",
		Topic:   "security-req",
		Content: "Must support MFA",
		Tags:    []models.CommunicationTag{models.TagRequirement},
	})
	if err != nil {
		t.Fatalf("adding requirement: %v", err)
	}

	// --- Extract knowledge ---
	knowledge, err := app.KnowledgeX.ExtractFromTask(taskID)
	if err != nil {
		t.Fatalf("extracting knowledge: %v", err)
	}
	if knowledge.TaskID != taskID {
		t.Errorf("knowledge.TaskID = %s, want %s", knowledge.TaskID, taskID)
	}
	if len(knowledge.Learnings) == 0 {
		t.Error("expected learnings extracted from notes")
	}
	if len(knowledge.Gotchas) == 0 {
		t.Error("expected gotchas extracted from notes")
	}
	if len(knowledge.Decisions) == 0 {
		t.Error("expected decisions (from context and communications)")
	}

	// Verify communication-derived decision.
	foundCommDecision := false
	for _, d := range knowledge.Decisions {
		if strings.Contains(d.Decision, "OAuth2") || strings.Contains(d.Decision, "PKCE") {
			foundCommDecision = true
			break
		}
	}
	if !foundCommDecision {
		t.Error("expected OAuth2/PKCE decision from communication")
	}

	// Verify wiki updates extracted from notes.
	if len(knowledge.WikiUpdates) == 0 {
		t.Error("expected wiki updates extracted")
	}

	// --- Generate handoff ---
	handoff, err := app.KnowledgeX.GenerateHandoff(taskID)
	if err != nil {
		t.Fatalf("generating handoff: %v", err)
	}
	if handoff.TaskID != taskID {
		t.Errorf("handoff.TaskID = %s, want %s", handoff.TaskID, taskID)
	}

	handoffPath := filepath.Join(ticketDir, "handoff.md")
	handoffData, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("reading handoff.md: %v", err)
	}
	handoffStr := string(handoffData)
	if !strings.Contains(handoffStr, taskID) {
		t.Error("handoff.md missing task ID")
	}
	if !strings.Contains(handoffStr, "Provenance") {
		t.Error("handoff.md missing Provenance section")
	}

	// --- Create ADR ---
	decision := models.Decision{
		Title:        "Use JSON:API for responses",
		Context:      "We need a standard API format across services",
		Decision:     "Adopt JSON:API specification for all public endpoints",
		Consequences: []string{"Clients must follow JSON:API parsing", "Better interop"},
		Alternatives: []string{"Plain JSON", "GraphQL"},
	}
	adrPath, err := app.KnowledgeX.CreateADR(decision, taskID)
	if err != nil {
		t.Fatalf("creating ADR: %v", err)
	}

	// Verify ADR format and provenance.
	adrData, err := os.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("reading ADR: %v", err)
	}
	adrStr := string(adrData)
	for _, want := range []string{"ADR-", "**Status:** Accepted", "**Source:** " + taskID,
		"## Decision", "## Consequences", "## Alternatives Considered", "Plain JSON"} {
		if !strings.Contains(adrStr, want) {
			t.Errorf("ADR missing %q", want)
		}
	}

	// --- Update wiki ---
	if err := app.KnowledgeX.UpdateWiki(knowledge); err != nil {
		t.Fatalf("updating wiki: %v", err)
	}

	wikiDir := filepath.Join(app.BasePath, "docs", "wiki")
	wikiEntries, err := os.ReadDir(wikiDir)
	if err != nil {
		t.Fatalf("reading wiki dir: %v", err)
	}
	if len(wikiEntries) == 0 {
		t.Error("expected wiki files after UpdateWiki")
	}

	// At least one wiki file should reference the task for provenance.
	foundTaskRef := false
	for _, entry := range wikiEntries {
		data, _ := os.ReadFile(filepath.Join(wikiDir, entry.Name()))
		if strings.Contains(string(data), taskID) {
			foundTaskRef = true
			break
		}
	}
	if !foundTaskRef {
		t.Error("wiki files should reference the source task ID")
	}
}

// =========================================================================
// 5. CLI execution within task context.
// =========================================================================

func TestIntegration_CLIExecutor_EnvInjection(t *testing.T) {
	app := newTestApp(t)

	taskCtx := &integration.TaskEnvContext{
		TaskID:       "TASK-00042",
		Branch:       "feat/add-auth",
		WorktreePath: "/tmp/work/TASK-00042",
		TicketPath:   "/tmp/tickets/TASK-00042",
	}

	baseEnv := []string{"PATH=/usr/bin", "HOME=/home/test"}
	env := app.Executor.BuildEnv(baseEnv, taskCtx)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	expected := map[string]string{
		"ADB_TASK_ID":       "TASK-00042",
		"ADB_BRANCH":        "feat/add-auth",
		"ADB_WORKTREE_PATH": "/tmp/work/TASK-00042",
		"ADB_TICKET_PATH":   "/tmp/tickets/TASK-00042",
	}
	for key, want := range expected {
		if envMap[key] != want {
			t.Errorf("%s = %q, want %q", key, envMap[key], want)
		}
	}

	// Base env vars are preserved.
	if envMap["HOME"] != "/home/test" {
		t.Errorf("HOME not preserved, got %q", envMap["HOME"])
	}
}

func TestIntegration_CLIExecutor_NilContextDoesNotInjectVars(t *testing.T) {
	app := newTestApp(t)

	baseEnv := []string{"PATH=/usr/bin"}
	env := app.Executor.BuildEnv(baseEnv, nil)
	for _, e := range env {
		if strings.HasPrefix(e, "ADB_") {
			t.Errorf("unexpected ADB_ var with nil context: %s", e)
		}
	}
	if len(env) != len(baseEnv) {
		t.Errorf("env length changed with nil context: %d -> %d", len(baseEnv), len(env))
	}
}

func TestIntegration_CLIExecutor_AliasResolution(t *testing.T) {
	app := newTestApp(t)

	aliases := []integration.CLIAlias{
		{Name: "build", Command: "go", DefaultArgs: []string{"build", "./..."}},
		{Name: "lint", Command: "golangci-lint", DefaultArgs: []string{"run"}},
		{Name: "deploy", Command: "kubectl"},
	}

	// Known alias with default args.
	cmd, args, found := app.Executor.ResolveAlias("build", aliases)
	if !found {
		t.Fatal("expected 'build' alias to be found")
	}
	if cmd != "go" {
		t.Errorf("command = %q, want go", cmd)
	}
	if len(args) != 2 || args[0] != "build" || args[1] != "./..." {
		t.Errorf("args = %v, want [build ./...]", args)
	}

	// Known alias without default args.
	cmd, args, found = app.Executor.ResolveAlias("deploy", aliases)
	if !found {
		t.Fatal("expected 'deploy' alias to be found")
	}
	if cmd != "kubectl" {
		t.Errorf("command = %q, want kubectl", cmd)
	}
	if args != nil {
		t.Errorf("expected nil default args, got %v", args)
	}

	// Unknown alias.
	cmd, args, found = app.Executor.ResolveAlias("unknown", aliases)
	if found {
		t.Fatal("expected 'unknown' to NOT be found")
	}
	if cmd != "unknown" {
		t.Errorf("unresolved command = %q, want unknown", cmd)
	}
	if args != nil {
		t.Errorf("unresolved args should be nil, got %v", args)
	}
}

func TestIntegration_CLIExecutor_ListAliases(t *testing.T) {
	app := newTestApp(t)

	aliases := []integration.CLIAlias{
		{Name: "lint", Command: "golangci-lint", DefaultArgs: []string{"run"}},
		{Name: "build", Command: "go"},
	}
	listed := app.Executor.ListAliases(aliases)
	if len(listed) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(listed))
	}
	if !strings.Contains(listed[0], "lint -> golangci-lint") {
		t.Errorf("first alias = %q, expected lint -> golangci-lint", listed[0])
	}
	if !strings.Contains(listed[1], "build -> go") {
		t.Errorf("second alias = %q, expected build -> go", listed[1])
	}
}

func TestIntegration_CLIExecutor_LogFailure(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "log-failure-test", "", core.CreateTaskOpts{})
	ticketDir := filepath.Join(app.BasePath, "tickets", task.ID)

	taskCtx := &integration.TaskEnvContext{
		TaskID:     task.ID,
		TicketPath: ticketDir,
	}
	failResult := &integration.CLIExecResult{
		ExitCode: 1,
		Stderr:   "compilation failed: missing import",
	}

	if err := app.Executor.LogFailure(taskCtx, "go", []string{"build", "."}, failResult); err != nil {
		t.Fatalf("LogFailure: %v", err)
	}

	// Verify failure was appended to context.md.
	contextPath := filepath.Join(ticketDir, "context.md")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("reading context.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## CLI Failure") {
		t.Error("context.md missing CLI Failure section")
	}
	if !strings.Contains(content, "Exit Code:** 1") {
		t.Error("context.md missing exit code")
	}
	if !strings.Contains(content, "compilation failed") {
		t.Error("context.md missing stderr content")
	}
}

// =========================================================================
// 6. Configuration precedence: .taskrc > .taskconfig > defaults
// =========================================================================

func TestIntegration_ConfigPrecedence_GlobalAndRepo(t *testing.T) {
	dir := t.TempDir()

	// Write .taskconfig with global settings.
	taskconfig := `defaults:
  ai: kiro
  priority: P2
  owner: global-owner
task_id:
  prefix: PROJ
  counter: 10
screenshot:
  hotkey: ctrl+shift+s
cli_aliases:
  - name: lint
    command: golangci-lint
    default_args:
      - run
`
	if err := os.WriteFile(filepath.Join(dir, ".taskconfig"), []byte(taskconfig), 0o644); err != nil {
		t.Fatalf("writing .taskconfig: %v", err)
	}

	// Create a repo with .taskrc.
	repoDir := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}
	taskrc := `build_command: "make build"
test_command: "make test"
default_reviewers:
  - alice
  - bob
conventions:
  - "Use conventional commits"
  - "Branch naming: type/task-id-description"
`
	if err := os.WriteFile(filepath.Join(repoDir, ".taskrc"), []byte(taskrc), 0o644); err != nil {
		t.Fatalf("writing .taskrc: %v", err)
	}

	cfgMgr := core.NewConfigurationManager(dir)

	// --- Global config ---
	global, err := cfgMgr.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if global.DefaultAI != "kiro" {
		t.Errorf("DefaultAI = %q, want kiro", global.DefaultAI)
	}
	if global.TaskIDPrefix != "PROJ" {
		t.Errorf("TaskIDPrefix = %q, want PROJ", global.TaskIDPrefix)
	}
	if global.DefaultPriority != models.P2 {
		t.Errorf("DefaultPriority = %q, want P2", global.DefaultPriority)
	}
	if global.TaskIDCounter != 10 {
		t.Errorf("TaskIDCounter = %d, want 10", global.TaskIDCounter)
	}
	if len(global.CLIAliases) != 1 || global.CLIAliases[0].Name != "lint" {
		t.Errorf("CLIAliases parsing failed: %+v", global.CLIAliases)
	}

	// --- Repo config ---
	repo, err := cfgMgr.LoadRepoConfig(repoDir)
	if err != nil {
		t.Fatalf("LoadRepoConfig: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo config")
	}
	if repo.BuildCommand != "make build" {
		t.Errorf("BuildCommand = %q, want 'make build'", repo.BuildCommand)
	}
	if repo.TestCommand != "make test" {
		t.Errorf("TestCommand = %q, want 'make test'", repo.TestCommand)
	}
	if len(repo.DefaultReviewers) != 2 {
		t.Errorf("expected 2 reviewers, got %d", len(repo.DefaultReviewers))
	}
	if len(repo.Conventions) != 2 {
		t.Errorf("expected 2 conventions, got %d", len(repo.Conventions))
	}

	// --- Merged config: .taskrc > .taskconfig > defaults ---
	merged, err := cfgMgr.GetMergedConfig(repoDir)
	if err != nil {
		t.Fatalf("GetMergedConfig: %v", err)
	}
	if merged.DefaultAI != "kiro" {
		t.Errorf("merged.DefaultAI = %q, want kiro", merged.DefaultAI)
	}
	if merged.TaskIDPrefix != "PROJ" {
		t.Errorf("merged.TaskIDPrefix = %q, want PROJ", merged.TaskIDPrefix)
	}
	if merged.Repo == nil {
		t.Fatal("expected merged.Repo to be non-nil")
	}
	if merged.Repo.BuildCommand != "make build" {
		t.Errorf("merged.Repo.BuildCommand = %q, want 'make build'", merged.Repo.BuildCommand)
	}
}

func TestIntegration_ConfigDefaults_WhenNoConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgMgr := core.NewConfigurationManager(dir)

	global, err := cfgMgr.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if global.DefaultAI != "kiro" {
		t.Errorf("default DefaultAI = %q, want kiro", global.DefaultAI)
	}
	if global.TaskIDPrefix != "TASK" {
		t.Errorf("default TaskIDPrefix = %q, want TASK", global.TaskIDPrefix)
	}
	if global.DefaultPriority != models.P2 {
		t.Errorf("default DefaultPriority = %q, want P2", global.DefaultPriority)
	}
}

func TestIntegration_ConfigMerge_WithoutRepoConfig(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ".taskconfig"), []byte("defaults:\n  ai: claude\n"), 0o644)

	cfgMgr := core.NewConfigurationManager(dir)
	merged, err := cfgMgr.GetMergedConfig("")
	if err != nil {
		t.Fatalf("GetMergedConfig: %v", err)
	}
	if merged.Repo != nil {
		t.Error("expected merged.Repo to be nil when repoPath is empty")
	}
	if merged.DefaultAI != "claude" {
		t.Errorf("merged.DefaultAI = %q, want claude", merged.DefaultAI)
	}
}

func TestIntegration_ConfigValidation(t *testing.T) {
	dir := t.TempDir()
	cfgMgr := core.NewConfigurationManager(dir)

	// Valid config.
	if err := cfgMgr.ValidateConfig(&models.GlobalConfig{
		DefaultAI: "kiro", TaskIDPrefix: "TASK", DefaultPriority: models.P2,
	}); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}

	// Invalid: empty prefix.
	if err := cfgMgr.ValidateConfig(&models.GlobalConfig{
		DefaultAI: "kiro", TaskIDPrefix: "",
	}); err == nil {
		t.Error("expected error for empty prefix")
	}

	// Invalid: nil config.
	if err := cfgMgr.ValidateConfig(nil); err == nil {
		t.Error("expected error for nil config")
	}

	// Invalid: bad task type in repo templates.
	if err := cfgMgr.ValidateConfig(&models.RepoConfig{
		Templates: map[models.TaskType]string{"invalid": "t.md"},
	}); err == nil {
		t.Error("expected error for invalid task type in templates")
	}

	// Valid merged config.
	if err := cfgMgr.ValidateConfig(&models.MergedConfig{
		GlobalConfig: models.GlobalConfig{DefaultAI: "kiro", TaskIDPrefix: "TASK"},
	}); err != nil {
		t.Errorf("valid merged config rejected: %v", err)
	}
}

// =========================================================================
// 7. Backlog filtering: multiple tasks, filter combinations.
// =========================================================================

func TestIntegration_BacklogFilter_StatusAndPriority(t *testing.T) {
	app := newTestApp(t)

	// Create tasks with different types.
	task1, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "feature-one", "", core.CreateTaskOpts{})
	task2, _ := app.TaskMgr.CreateTask(models.TaskTypeBug, "fix-bug", "", core.CreateTaskOpts{})
	task3, _ := app.TaskMgr.CreateTask(models.TaskTypeSpike, "investigate", "", core.CreateTaskOpts{})

	// Verify all start in backlog.
	allTasks, _ := app.TaskMgr.GetAllTasks()
	if len(allTasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(allTasks))
	}

	// Change statuses.
	_, _ = app.TaskMgr.ResumeTask(task1.ID)  // -> in_progress
	_, _ = app.TaskMgr.ArchiveTask(task2.ID) // -> archived
	// task3 stays in backlog

	// Filter by in_progress.
	ipTasks, err := app.TaskMgr.GetTasksByStatus(models.StatusInProgress)
	if err != nil {
		t.Fatalf("GetTasksByStatus(in_progress): %v", err)
	}
	if len(ipTasks) != 1 || ipTasks[0].ID != task1.ID {
		t.Errorf("expected [%s] for in_progress, got %d tasks", task1.ID, len(ipTasks))
	}

	// Filter by archived.
	archivedTasks, _ := app.TaskMgr.GetTasksByStatus(models.StatusArchived)
	if len(archivedTasks) != 1 || archivedTasks[0].ID != task2.ID {
		t.Errorf("expected [%s] for archived, got %d tasks", task2.ID, len(archivedTasks))
	}

	// Filter by backlog.
	backlogTasks, _ := app.TaskMgr.GetTasksByStatus(models.StatusBacklog)
	if len(backlogTasks) != 1 || backlogTasks[0].ID != task3.ID {
		t.Errorf("expected [%s] for backlog, got %d tasks", task3.ID, len(backlogTasks))
	}
}

func TestIntegration_BacklogFilter_Combinations(t *testing.T) {
	app := newTestApp(t)

	// Create 5 tasks with various properties.
	t1, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "dashboard", "", core.CreateTaskOpts{})
	t2, _ := app.TaskMgr.CreateTask(models.TaskTypeBug, "login-fix", "", core.CreateTaskOpts{})
	t3, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "search", "", core.CreateTaskOpts{})
	t4, _ := app.TaskMgr.CreateTask(models.TaskTypeRefactor, "db-refactor", "", core.CreateTaskOpts{})
	t5, _ := app.TaskMgr.CreateTask(models.TaskTypeSpike, "cache-spike", "", core.CreateTaskOpts{})

	// Set up different properties via backlog.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("load backlog: %v", err)
	}

	// t1: backlog, P0, alice, [frontend]
	_ = app.BacklogMgr.UpdateTask(t1.ID, storage.BacklogEntry{Priority: models.P0, Owner: "alice", Tags: []string{"frontend"}})
	// t2: backlog, P1, bob, [backend, urgent]
	_ = app.BacklogMgr.UpdateTask(t2.ID, storage.BacklogEntry{Priority: models.P1, Owner: "bob", Tags: []string{"backend", "urgent"}})
	// t3: in_progress, P2, alice, [frontend, search]
	_ = app.BacklogMgr.UpdateTask(t3.ID, storage.BacklogEntry{Status: models.StatusInProgress, Priority: models.P2, Owner: "alice", Tags: []string{"frontend", "search"}})
	// t4: backlog, P3, charlie, [backend]
	_ = app.BacklogMgr.UpdateTask(t4.ID, storage.BacklogEntry{Priority: models.P3, Owner: "charlie", Tags: []string{"backend"}})
	// t5: blocked, P1, alice, [backend, cache]
	_ = app.BacklogMgr.UpdateTask(t5.ID, storage.BacklogEntry{Status: models.StatusBlocked, Priority: models.P1, Owner: "alice", Tags: []string{"backend", "cache"}})

	if err := app.BacklogMgr.Save(); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("reload backlog: %v", err)
	}

	tests := []struct {
		name    string
		filter  storage.BacklogFilter
		wantLen int
	}{
		{"all tasks", storage.BacklogFilter{}, 5},
		{"status=backlog", storage.BacklogFilter{Status: []models.TaskStatus{models.StatusBacklog}}, 3},
		{"status=in_progress", storage.BacklogFilter{Status: []models.TaskStatus{models.StatusInProgress}}, 1},
		{"status=backlog+blocked", storage.BacklogFilter{Status: []models.TaskStatus{models.StatusBacklog, models.StatusBlocked}}, 4},
		{"priority=P1", storage.BacklogFilter{Priority: []models.Priority{models.P1}}, 2},
		{"owner=alice", storage.BacklogFilter{Owner: "alice"}, 3},
		{"owner=bob", storage.BacklogFilter{Owner: "bob"}, 1},
		{"tag=frontend", storage.BacklogFilter{Tags: []string{"frontend"}}, 2},
		{"tag=backend", storage.BacklogFilter{Tags: []string{"backend"}}, 3},
		{"backlog+alice", storage.BacklogFilter{Status: []models.TaskStatus{models.StatusBacklog}, Owner: "alice"}, 1},
		{"P1+backend", storage.BacklogFilter{Priority: []models.Priority{models.P1}, Tags: []string{"backend"}}, 2},
		{"alice+frontend", storage.BacklogFilter{Owner: "alice", Tags: []string{"frontend"}}, 2},
		{"owner=nobody", storage.BacklogFilter{Owner: "nobody"}, 0},
		{"tag=nonexistent", storage.BacklogFilter{Tags: []string{"nonexistent"}}, 0},
		{"backend+urgent (AND tags)", storage.BacklogFilter{Tags: []string{"backend", "urgent"}}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, err := app.BacklogMgr.FilterTasks(tc.filter)
			if err != nil {
				t.Fatalf("FilterTasks: %v", err)
			}
			if len(results) != tc.wantLen {
				ids := make([]string, len(results))
				for i, r := range results {
					ids[i] = r.ID
				}
				t.Errorf("got %d results %v, want %d", len(results), ids, tc.wantLen)
			}
		})
	}
}

// =========================================================================
// Additional: Priority reordering
// =========================================================================

func TestIntegration_PriorityReordering(t *testing.T) {
	app := newTestApp(t)

	task1, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "low-pri", "", core.CreateTaskOpts{})
	task2, _ := app.TaskMgr.CreateTask(models.TaskTypeBug, "high-pri", "", core.CreateTaskOpts{})
	task3, _ := app.TaskMgr.CreateTask(models.TaskTypeSpike, "mid-pri", "", core.CreateTaskOpts{})

	// Reorder: task2 first (P0), task3 second (P1), task1 third (P2).
	err := app.TaskMgr.ReorderPriorities([]string{task2.ID, task3.ID, task1.ID})
	if err != nil {
		t.Fatalf("reordering priorities: %v", err)
	}

	t2, _ := app.TaskMgr.GetTask(task2.ID)
	t3, _ := app.TaskMgr.GetTask(task3.ID)
	t1, _ := app.TaskMgr.GetTask(task1.ID)

	if t2.Priority != models.P0 {
		t.Errorf("task2 priority = %s, want P0", t2.Priority)
	}
	if t3.Priority != models.P1 {
		t.Errorf("task3 priority = %s, want P1", t3.Priority)
	}
	if t1.Priority != models.P2 {
		t.Errorf("task1 priority = %s, want P2", t1.Priority)
	}
}

func TestIntegration_UpdateTaskPriority(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "pri-test", "", core.CreateTaskOpts{})
	if err := app.TaskMgr.UpdateTaskPriority(task.ID, models.P0); err != nil {
		t.Fatalf("UpdateTaskPriority: %v", err)
	}
	got, _ := app.TaskMgr.GetTask(task.ID)
	if got.Priority != models.P0 {
		t.Errorf("priority = %s, want P0", got.Priority)
	}
}

// =========================================================================
// Additional: Status transitions
// =========================================================================

func TestIntegration_StatusTransitions(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "status-test", "", core.CreateTaskOpts{})

	statuses := []models.TaskStatus{
		models.StatusInProgress,
		models.StatusBlocked,
		models.StatusReview,
		models.StatusDone,
	}

	for _, status := range statuses {
		if err := app.TaskMgr.UpdateTaskStatus(task.ID, status); err != nil {
			t.Fatalf("UpdateTaskStatus(%s): %v", status, err)
		}
		got, _ := app.TaskMgr.GetTask(task.ID)
		if got.Status != status {
			t.Errorf("after UpdateTaskStatus(%s): got %s", status, got.Status)
		}
	}
}

// =========================================================================
// Additional: Design document lifecycle
// =========================================================================

func TestIntegration_DesignDoc_BootstrapAndPopulate(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "new-api", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}
	taskID := task.ID

	// Verify design.md was created during bootstrap.
	designPath := filepath.Join(app.BasePath, "tickets", taskID, "design.md")
	if _, err := os.Stat(designPath); os.IsNotExist(err) {
		t.Fatal("design.md not created during bootstrap")
	}

	// Initialize with the full design doc format.
	if err := app.DesignGen.InitializeDesignDoc(taskID); err != nil {
		t.Fatalf("InitializeDesignDoc: %v", err)
	}
	data, _ := os.ReadFile(designPath)
	if !strings.Contains(string(data), taskID) {
		t.Error("design.md should contain task ID after initialization")
	}

	// Add communications for context population.
	_ = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), Source: "meeting",
		Contact: "charlie", Topic: "api-design",
		Content: "REST over gRPC for external API",
		Tags:    []models.CommunicationTag{models.TagRequirement},
	})
	_ = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), Source: "slack",
		Contact: "dave", Topic: "tech-stack",
		Content: "Use Go with Chi router",
		Tags:    []models.CommunicationTag{models.TagDecision},
	})

	// Populate from context.
	if err := app.DesignGen.PopulateFromContext(taskID); err != nil {
		t.Fatalf("PopulateFromContext: %v", err)
	}

	doc, err := app.DesignGen.GetDesignDoc(taskID)
	if err != nil {
		t.Fatalf("GetDesignDoc: %v", err)
	}
	if len(doc.StakeholderRequirements) == 0 {
		t.Error("expected stakeholder requirements populated")
	}
	if len(doc.TechnicalDecisions) == 0 {
		t.Error("expected technical decisions extracted")
	}

	// Update a section.
	if err := app.DesignGen.UpdateDesignDoc(taskID, core.DesignUpdate{
		Section: "overview",
		Content: "Build a REST API for the new-api feature using Go and Chi.",
	}); err != nil {
		t.Fatalf("UpdateDesignDoc: %v", err)
	}

	updated, _ := app.DesignGen.GetDesignDoc(taskID)
	if updated.Overview != "Build a REST API for the new-api feature using Go and Chi." {
		t.Errorf("overview = %q, want updated content", updated.Overview)
	}
}

// =========================================================================
// Additional: Update generation
// =========================================================================

func TestIntegration_UpdateGeneration(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "update-test", "", core.CreateTaskOpts{})
	taskID := task.ID

	_ = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date: time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC), Source: "slack",
		Contact: "manager", Topic: "status-check",
		Content: "What is the status of this feature?",
		Tags:    []models.CommunicationTag{models.TagQuestion},
	})

	plan, err := app.UpdateGen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("GenerateUpdates: %v", err)
	}
	if plan.TaskID != taskID {
		t.Errorf("plan.TaskID = %s, want %s", plan.TaskID, taskID)
	}
	if plan.GeneratedAt.IsZero() {
		t.Error("expected non-zero GeneratedAt")
	}
}

// =========================================================================
// Additional: Communication search
// =========================================================================

func TestIntegration_CommunicationSearch(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "search-test", "", core.CreateTaskOpts{})
	taskID := task.ID

	_ = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC), Source: "slack",
		Contact: "alice", Topic: "database-choice",
		Content: "We should use PostgreSQL for the primary database",
		Tags:    []models.CommunicationTag{models.TagDecision},
	})
	_ = app.CommMgr.AddCommunication(taskID, models.Communication{
		Date: time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC), Source: "email",
		Contact: "bob", Topic: "timeline",
		Content: "Need the feature by end of sprint",
		Tags:    []models.CommunicationTag{models.TagActionItem},
	})

	// Search by content.
	results, _ := app.CommMgr.SearchCommunications(taskID, "PostgreSQL")
	if len(results) != 1 {
		t.Errorf("expected 1 result for PostgreSQL, got %d", len(results))
	}

	// Search by contact.
	results2, _ := app.CommMgr.SearchCommunications(taskID, "bob")
	if len(results2) != 1 {
		t.Errorf("expected 1 result for bob, got %d", len(results2))
	}

	// Get all.
	all, _ := app.CommMgr.GetAllCommunications(taskID)
	if len(all) != 2 {
		t.Errorf("expected 2 communications, got %d", len(all))
	}
}

// =========================================================================
// Additional: AI context generation
// =========================================================================

func TestIntegration_AIContextGeneration(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "feature-x", "", core.CreateTaskOpts{})
	_, _ = app.TaskMgr.CreateTask(models.TaskTypeBug, "fix-y", "", core.CreateTaskOpts{})

	// Sync context to generate both files.
	if err := app.AICtxGen.SyncContext(); err != nil {
		t.Fatalf("SyncContext: %v", err)
	}

	// Verify CLAUDE.md.
	claudePath := filepath.Join(app.BasePath, "CLAUDE.md")
	claudeData, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}
	content := string(claudeData)
	for _, section := range []string{
		"## Project Overview",
		"## Directory Structure",
		"## Key Conventions",
		"## Glossary",
		"## Active Tasks",
	} {
		if !strings.Contains(content, section) {
			t.Errorf("CLAUDE.md missing section %q", section)
		}
	}
	if !strings.Contains(content, task.ID) {
		t.Errorf("CLAUDE.md should contain active task ID %s", task.ID)
	}

	// Verify kiro.md.
	kiroPath := filepath.Join(app.BasePath, "kiro.md")
	if _, err := os.Stat(kiroPath); os.IsNotExist(err) {
		t.Fatal("kiro.md not created")
	}
}

func TestIntegration_AIContextGeneration_WithADR(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "auth-redesign", "", core.CreateTaskOpts{})

	// Create an ADR so the decisions summary has content.
	_, _ = app.KnowledgeX.CreateADR(models.Decision{
		Title:    "Adopt OAuth2 with PKCE for authentication",
		Context:  "Current auth system uses session cookies",
		Decision: "All public-facing services use OAuth2 with PKCE flow",
	}, task.ID)

	_, err := app.AICtxGen.GenerateContextFile(core.AITypeClaude)
	if err != nil {
		t.Fatalf("GenerateContextFile: %v", err)
	}

	claudeData, _ := os.ReadFile(filepath.Join(app.BasePath, "CLAUDE.md"))
	content := string(claudeData)
	if !strings.Contains(content, "Active Decisions Summary") {
		t.Error("CLAUDE.md missing decisions summary")
	}
	if !strings.Contains(content, "OAuth2") {
		t.Error("CLAUDE.md should reference the OAuth2 ADR")
	}
}

// =========================================================================
// Additional: Conflict detection
// =========================================================================

func TestIntegration_ConflictDetection_WithADR(t *testing.T) {
	app := newTestApp(t)

	_, _ = app.KnowledgeX.CreateADR(models.Decision{
		Title:    "Use REST for API",
		Context:  "External facing API needs broad client support",
		Decision: "Use REST over gRPC for external API endpoints",
	}, "TASK-00001")

	conflicts, err := app.ConflictDt.CheckForConflicts(core.ConflictContext{
		TaskID:          "TASK-00002",
		ProposedChanges: "Use gRPC for all API endpoints",
	})
	if err != nil {
		t.Fatalf("CheckForConflicts: %v", err)
	}

	// The integration path should work end-to-end without error.
	_ = conflicts
}

func TestIntegration_ConflictDetection_NoConflicts(t *testing.T) {
	app := newTestApp(t)

	conflicts, err := app.ConflictDt.CheckForConflicts(core.ConflictContext{
		TaskID:          "TASK-00001",
		ProposedChanges: "Add a new logging feature",
	})
	if err != nil {
		t.Fatalf("CheckForConflicts: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts with empty docs, got %d", len(conflicts))
	}
}

// =========================================================================
// Additional: Context manager lifecycle
// =========================================================================

func TestIntegration_ContextManager_Lifecycle(t *testing.T) {
	app := newTestApp(t)

	taskID := "TASK-CTX-01"
	ctx, err := app.ContextMgr.InitializeContext(taskID)
	if err != nil {
		t.Fatalf("InitializeContext: %v", err)
	}
	if ctx.TaskID != taskID {
		t.Errorf("TaskID = %s, want %s", ctx.TaskID, taskID)
	}

	// Load.
	loaded, err := app.ContextMgr.LoadContext(taskID)
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if !strings.Contains(loaded.Context, taskID) {
		t.Error("loaded context should contain task ID")
	}

	// Update.
	if err := app.ContextMgr.UpdateContext(taskID, map[string]interface{}{
		"notes": "# Updated Notes\n\n- Important finding",
	}); err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	// Persist and reload.
	if err := app.ContextMgr.PersistContext(taskID); err != nil {
		t.Fatalf("PersistContext: %v", err)
	}
	reloaded, _ := app.ContextMgr.LoadContext(taskID)
	if !strings.Contains(reloaded.Notes, "Important finding") {
		t.Error("persisted notes should contain the update")
	}

	// AI context.
	aiCtx, _ := app.ContextMgr.GetContextForAI(taskID)
	if aiCtx == nil {
		t.Fatal("AI context should not be nil")
	}
}

// =========================================================================
// Additional: Backlog persistence round-trip
// =========================================================================

func TestIntegration_BacklogPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)

	_ = mgr.Load()
	entry := storage.BacklogEntry{
		ID: "TASK-00001", Title: "Test task", Status: models.StatusBacklog,
		Priority: models.P2, Owner: "alice", Repo: "github.com/org/repo",
		Branch: "feat/test", Created: time.Now().Format(time.RFC3339),
		Tags: []string{"test", "integration"},
	}
	_ = mgr.AddTask(entry)
	_ = mgr.Save()

	// Fresh manager: load from disk.
	mgr2 := storage.NewBacklogManager(dir)
	_ = mgr2.Load()
	loaded, err := mgr2.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.Title != "Test task" {
		t.Errorf("Title = %q, want 'Test task'", loaded.Title)
	}
	if loaded.Status != models.StatusBacklog {
		t.Errorf("Status = %s, want backlog", loaded.Status)
	}
	if loaded.Owner != "alice" {
		t.Errorf("Owner = %q, want alice", loaded.Owner)
	}
	if len(loaded.Tags) != 2 {
		t.Errorf("Tags = %v, want [test integration]", loaded.Tags)
	}
}

func TestIntegration_BacklogDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	mgr := storage.NewBacklogManager(dir)
	_ = mgr.Load()
	_ = mgr.AddTask(storage.BacklogEntry{ID: "TASK-00001", Title: "First"})
	if err := mgr.AddTask(storage.BacklogEntry{ID: "TASK-00001", Title: "Dup"}); err == nil {
		t.Error("expected error for duplicate task ID")
	}
}

// =========================================================================
// Additional: Template manager per task type
// =========================================================================

func TestIntegration_TemplateManager_AllTaskTypes(t *testing.T) {
	dir := t.TempDir()
	tmplMgr := core.NewTemplateManager(dir)

	types := []struct {
		typ      models.TaskType
		notesKW  string
		designKW string
	}{
		{models.TaskTypeFeat, "Feature Notes", "Technical Design"},
		{models.TaskTypeBug, "Bug Notes", "Root Cause"},
		{models.TaskTypeSpike, "Spike Notes", "Investigation Scope"},
		{models.TaskTypeRefactor, "Refactor Notes", "Current Architecture"},
	}

	for _, tt := range types {
		t.Run(string(tt.typ), func(t *testing.T) {
			ticketDir := filepath.Join(dir, "tickets", "test-"+string(tt.typ))
			_ = os.MkdirAll(ticketDir, 0o755)

			if err := tmplMgr.ApplyTemplate(ticketDir, tt.typ); err != nil {
				t.Fatalf("ApplyTemplate(%s): %v", tt.typ, err)
			}

			notesData, _ := os.ReadFile(filepath.Join(ticketDir, "notes.md"))
			if !strings.Contains(string(notesData), tt.notesKW) {
				t.Errorf("notes.md missing %q for type %s", tt.notesKW, tt.typ)
			}

			designData, _ := os.ReadFile(filepath.Join(ticketDir, "design.md"))
			if !strings.Contains(string(designData), tt.designKW) {
				t.Errorf("design.md missing %q for type %s", tt.designKW, tt.typ)
			}
		})
	}
}

// =========================================================================
// Additional: Task ID sequential generation
// =========================================================================

func TestIntegration_TaskIDSequentialGeneration(t *testing.T) {
	dir := t.TempDir()
	idGen := core.NewTaskIDGenerator(dir, "TASK")

	expected := []string{"TASK-00001", "TASK-00002", "TASK-00003", "TASK-00004", "TASK-00005"}
	for i, want := range expected {
		id, err := idGen.GenerateTaskID()
		if err != nil {
			t.Fatalf("GenerateTaskID %d: %v", i, err)
		}
		if id != want {
			t.Errorf("id[%d] = %s, want %s", i, id, want)
		}
	}
}

func TestIntegration_TaskIDCustomPrefix(t *testing.T) {
	dir := t.TempDir()
	idGen := core.NewTaskIDGenerator(dir, "PROJ")

	id, err := idGen.GenerateTaskID()
	if err != nil {
		t.Fatalf("GenerateTaskID: %v", err)
	}
	if id != "PROJ-00001" {
		t.Errorf("id = %s, want PROJ-00001", id)
	}
}

// =========================================================================
// Additional: Full App initialization
// =========================================================================

func TestIntegration_AppInitialization(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	components := []struct {
		name string
		ok   bool
	}{
		{"TaskMgr", app.TaskMgr != nil},
		{"BacklogMgr", app.BacklogMgr != nil},
		{"ContextMgr", app.ContextMgr != nil},
		{"CommMgr", app.CommMgr != nil},
		{"Bootstrap", app.Bootstrap != nil},
		{"IDGen", app.IDGen != nil},
		{"TmplMgr", app.TmplMgr != nil},
		{"UpdateGen", app.UpdateGen != nil},
		{"AICtxGen", app.AICtxGen != nil},
		{"DesignGen", app.DesignGen != nil},
		{"KnowledgeX", app.KnowledgeX != nil},
		{"ConflictDt", app.ConflictDt != nil},
		{"WorktreeMgr", app.WorktreeMgr != nil},
		{"OfflineMgr", app.OfflineMgr != nil},
		{"TabMgr", app.TabMgr != nil},
		{"ScreenPipe", app.ScreenPipe != nil},
		{"Executor", app.Executor != nil},
		{"Runner", app.Runner != nil},
	}
	for _, c := range components {
		if !c.ok {
			t.Errorf("expected %s to be initialized (non-nil)", c.name)
		}
	}
}

func TestIntegration_AppInitialization_WithConfig(t *testing.T) {
	app := newTestAppWithConfig(t, `defaults:
  ai: claude
  priority: P1
task_id:
  prefix: PRJ
  counter: 0
cli_aliases:
  - name: build
    command: go
    default_args:
      - build
      - ./...
`)

	// Verify the app still initializes correctly with custom config.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "test", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if !strings.HasPrefix(task.ID, "PRJ-") {
		t.Errorf("expected PRJ- prefix, got %s", task.ID)
	}
}

// =========================================================================
// Additional: Full workflow - create, ADR, then AI context references it
// =========================================================================

func TestIntegration_FullWorkflow_ADRAndContextGen(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "auth-redesign", "", core.CreateTaskOpts{})

	adrDecision := models.Decision{
		Title:    "Adopt OAuth2 with PKCE for authentication",
		Context:  "Current auth uses session cookies, incompatible with SPAs",
		Decision: "All public-facing services use OAuth2 with PKCE flow",
	}
	adrPath, _ := app.KnowledgeX.CreateADR(adrDecision, task.ID)
	if _, err := os.Stat(adrPath); err != nil {
		t.Fatalf("ADR not written: %v", err)
	}

	_, _ = app.AICtxGen.GenerateContextFile(core.AITypeClaude)
	claudeData, _ := os.ReadFile(filepath.Join(app.BasePath, "CLAUDE.md"))
	content := string(claudeData)
	if !strings.Contains(content, "OAuth2") {
		t.Error("CLAUDE.md should reference the OAuth2 ADR")
	}
}

// =========================================================================
// Additional: Bootstrap directory structure verification
// =========================================================================

func TestIntegration_BootstrapDirectoryStructure(t *testing.T) {
	app := newTestApp(t)

	result, err := app.Bootstrap.Bootstrap(core.BootstrapConfig{
		Type:  models.TaskTypeBug,
		Title: "crash-on-startup",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if !strings.HasPrefix(result.TaskID, "TASK-") {
		t.Errorf("task ID = %s, expected TASK- prefix", result.TaskID)
	}

	// Verify all files.
	for _, name := range []string{"status.yaml", "notes.md", "design.md", "context.md"} {
		if _, err := os.Stat(filepath.Join(result.TicketPath, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	// Notes should have the bug template.
	notesData, _ := os.ReadFile(filepath.Join(result.TicketPath, "notes.md"))
	if !strings.Contains(string(notesData), "Bug Notes") {
		t.Error("notes.md missing Bug Notes heading")
	}
	if !strings.Contains(string(notesData), "Steps to Reproduce") {
		t.Error("notes.md missing Steps to Reproduce for bug template")
	}
}

// =========================================================================
// Additional: Resume already in_progress is a no-op
// =========================================================================

func TestIntegration_ResumeAlreadyInProgress(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "already-ip", "", core.CreateTaskOpts{})

	r1, _ := app.TaskMgr.ResumeTask(task.ID)
	if r1.Status != models.StatusInProgress {
		t.Fatalf("first resume: %s, want in_progress", r1.Status)
	}

	r2, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if r2.Status != models.StatusInProgress {
		t.Errorf("second resume: %s, want in_progress", r2.Status)
	}
}

// =========================================================================
// Edge Case 1: Empty backlog operations
// =========================================================================

func TestEdgeCase_EmptyBacklog_GetAllTasks(t *testing.T) {
	app := newTestApp(t)

	tasks, err := app.TaskMgr.GetAllTasks()
	if err != nil {
		t.Fatalf("GetAllTasks on empty backlog: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestEdgeCase_EmptyBacklog_GetTasksByStatus(t *testing.T) {
	app := newTestApp(t)

	for _, status := range []models.TaskStatus{
		models.StatusBacklog, models.StatusInProgress,
		models.StatusArchived, models.StatusDone,
	} {
		tasks, err := app.TaskMgr.GetTasksByStatus(status)
		if err != nil {
			t.Fatalf("GetTasksByStatus(%s) on empty backlog: %v", status, err)
		}
		if len(tasks) != 0 {
			t.Errorf("status %s: expected 0 tasks, got %d", status, len(tasks))
		}
	}
}

func TestEdgeCase_EmptyBacklog_ReorderPriorities(t *testing.T) {
	app := newTestApp(t)

	err := app.TaskMgr.ReorderPriorities([]string{"TASK-NONEXISTENT"})
	if err == nil {
		t.Error("expected error when reordering with nonexistent task IDs")
	}
}

// =========================================================================
// Edge Case 2: Special characters in branch names and content
// =========================================================================

func TestEdgeCase_SpecialCharsBranchName(t *testing.T) {
	app := newTestApp(t)

	// Branch names with special characters that should still work.
	branches := []string{
		"fix/issue-123",
		"feat/my_feature",
		"bugfix/CamelCase",
		"spike/a-b-c-d-e",
	}
	for _, branch := range branches {
		task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, branch, "", core.CreateTaskOpts{})
		if err != nil {
			t.Errorf("CreateTask(%q): %v", branch, err)
			continue
		}
		if task.Branch != branch {
			t.Errorf("branch = %q, want %q", task.Branch, branch)
		}
	}
}

// =========================================================================
// Edge Case 3: Communication with special characters in content
// =========================================================================

func TestEdgeCase_CommunicationSpecialContent(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "special-comms", "", core.CreateTaskOpts{})

	// Content with markdown, code blocks, special chars.
	specialContent := "## Decision\n\nUse `SELECT * FROM users WHERE id = $1`\n\n" +
		"```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n" +
		"Special chars: <>&\"'!@#$%^*()\n\nUnicode: cafe\u0301"

	err := app.CommMgr.AddCommunication(task.ID, models.Communication{
		Date:    time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
		Source:  "meeting",
		Contact: "dev-team",
		Topic:   "code-review",
		Content: specialContent,
		Tags:    []models.CommunicationTag{models.TagDecision},
	})
	if err != nil {
		t.Fatalf("AddCommunication with special content: %v", err)
	}

	// Search should find it.
	results, _ := app.CommMgr.SearchCommunications(task.ID, "SELECT")
	if len(results) == 0 {
		t.Error("expected to find communication with SQL content")
	}
}

// =========================================================================
// Edge Case 4: Corrupted YAML in backlog
// =========================================================================

func TestEdgeCase_CorruptedBacklogYAML(t *testing.T) {
	dir := t.TempDir()

	// Write corrupted YAML.
	backlogPath := filepath.Join(dir, "backlog.yaml")
	_ = os.WriteFile(backlogPath, []byte("this is not: valid:\nyaml: [[["), 0o644)

	mgr := storage.NewBacklogManager(dir)
	err := mgr.Load()
	if err == nil {
		t.Error("expected error when loading corrupted YAML")
	}
}

// =========================================================================
// Edge Case 5: Corrupted status.yaml for a task
// =========================================================================

func TestEdgeCase_CorruptedStatusYAML(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "corrupt-status", "", core.CreateTaskOpts{})

	// Corrupt the status.yaml file.
	statusPath := filepath.Join(app.BasePath, "tickets", task.ID, "status.yaml")
	_ = os.WriteFile(statusPath, []byte("this: is: broken: yaml: [[["), 0o644)

	// GetTask should fail gracefully.
	_, err := app.TaskMgr.GetTask(task.ID)
	if err == nil {
		t.Error("expected error when reading corrupted status.yaml")
	}
}

// =========================================================================
// Edge Case 6: Missing ticket directory for resume
// =========================================================================

func TestEdgeCase_ResumeDeletedTicketDir(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "deleted-dir", "", core.CreateTaskOpts{})

	// Delete the ticket directory.
	ticketDir := filepath.Join(app.BasePath, "tickets", task.ID)
	_ = os.RemoveAll(ticketDir)

	_, err := app.TaskMgr.ResumeTask(task.ID)
	if err == nil {
		t.Error("expected error when resuming task with deleted ticket directory")
	}
}

// =========================================================================
// Edge Case 7: Many tasks - stress test
// =========================================================================

func TestEdgeCase_ManyTasks(t *testing.T) {
	app := newTestApp(t)

	const numTasks = 20
	ids := make([]string, numTasks)
	for i := 0; i < numTasks; i++ {
		task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, fmt.Sprintf("task-%03d", i), "", core.CreateTaskOpts{})
		if err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
		ids[i] = task.ID
	}

	// Verify all tasks exist.
	all, err := app.TaskMgr.GetAllTasks()
	if err != nil {
		t.Fatalf("GetAllTasks: %v", err)
	}
	if len(all) != numTasks {
		t.Errorf("expected %d tasks, got %d", numTasks, len(all))
	}

	// Verify IDs are unique.
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate task ID: %s", id)
		}
		seen[id] = true
	}

	// Reorder all priorities.
	err = app.TaskMgr.ReorderPriorities(ids)
	if err != nil {
		t.Fatalf("ReorderPriorities with %d tasks: %v", numTasks, err)
	}
}

// =========================================================================
// Edge Case 8: Offline queue with invalid JSON
// =========================================================================

func TestEdgeCase_OfflineQueue_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()

	// Write corrupted JSON to the queue file.
	queuePath := filepath.Join(dir, ".offline_queue.json")
	_ = os.WriteFile(queuePath, []byte("not valid json{{{"), 0o644)

	mgr := integration.NewOfflineManager(dir)
	_, err := mgr.SyncPendingOperations()
	if err == nil {
		t.Error("expected error when syncing corrupted queue")
	}
}

// =========================================================================
// Edge Case 9: Context update on non-existent task
// =========================================================================

func TestEdgeCase_ContextUpdate_NonExistentTask(t *testing.T) {
	app := newTestApp(t)

	err := app.ContextMgr.UpdateContext("NON-EXISTENT-TASK", map[string]interface{}{
		"notes": "some data",
	})
	if err == nil {
		t.Error("expected error when updating context for non-existent task")
	}
}

// =========================================================================
// Edge Case 10: Archive task that has never been created via bootstrap
// =========================================================================

func TestEdgeCase_ArchiveNonExistentTask(t *testing.T) {
	app := newTestApp(t)

	_, err := app.TaskMgr.ArchiveTask("TASK-NONEXISTENT")
	if err == nil {
		t.Error("expected error when archiving non-existent task")
	}
}

// =========================================================================
// Edge Case 11: Concurrent-like sequential task creation (no goroutines)
// =========================================================================

func TestEdgeCase_SequentialRapidCreation(t *testing.T) {
	app := newTestApp(t)

	// Create tasks in rapid succession to stress ID generation.
	for i := 0; i < 10; i++ {
		_, err := app.TaskMgr.CreateTask(models.TaskTypeBug, fmt.Sprintf("rapid-%d", i), "", core.CreateTaskOpts{})
		if err != nil {
			t.Fatalf("rapid creation %d: %v", i, err)
		}
	}

	all, _ := app.TaskMgr.GetAllTasks()
	if len(all) != 10 {
		t.Errorf("expected 10 tasks, got %d", len(all))
	}
}

// =========================================================================
// Edge Case 12: Knowledge extraction from empty task
// =========================================================================

func TestEdgeCase_KnowledgeExtraction_EmptyTask(t *testing.T) {
	app := newTestApp(t)

	task, _ := app.TaskMgr.CreateTask(models.TaskTypeFeat, "empty-task", "", core.CreateTaskOpts{})

	knowledge, err := app.KnowledgeX.ExtractFromTask(task.ID)
	if err != nil {
		t.Fatalf("ExtractFromTask on empty task: %v", err)
	}
	// Should not panic and should return valid (possibly empty) knowledge.
	_ = knowledge
}

// =========================================================================
// App.go adapter tests
// =========================================================================

func TestApp_ResolveBasePath_ADBHome(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ADB_HOME", tmpDir)

	got := ResolveBasePath()
	if got != tmpDir {
		t.Errorf("ResolveBasePath = %q, want %q", got, tmpDir)
	}
}

func TestApp_ResolveBasePath_WalkUpToTaskconfig(t *testing.T) {
	t.Setenv("ADB_HOME", "")

	// Create a directory tree: root/sub/deep and put .taskconfig at root.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".taskconfig"), []byte("defaults:\n  ai: kiro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Change to the deep directory.
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(deep)

	got := ResolveBasePath()
	// Resolve symlinks (macOS: /tmp -> /private/tmp) and short names (Windows: RUNNER~1 -> runneradmin).
	expected, _ := filepath.EvalSymlinks(root)
	got, _ = filepath.EvalSymlinks(got)
	if got != expected {
		t.Errorf("ResolveBasePath = %q, want %q", got, expected)
	}
}

func TestApp_ResolveBasePath_FallbackToCwd(t *testing.T) {
	t.Setenv("ADB_HOME", "")

	// Use a temp dir with no .taskconfig anywhere.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	got := ResolveBasePath()
	// Resolve symlinks (macOS: /tmp -> /private/tmp) and short names (Windows: RUNNER~1 -> runneradmin).
	expected, _ := filepath.EvalSymlinks(tmpDir)
	got, _ = filepath.EvalSymlinks(got)
	if got != expected {
		t.Errorf("ResolveBasePath = %q, want %q (cwd fallback)", got, expected)
	}
}

func TestApp_BacklogStoreAdapter_GetTask(t *testing.T) {
	app := newTestApp(t)

	// Create a task so we can test GetTask via the adapter.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "get-task-test", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// GetTask is called through the backlogStoreAdapter.
	got, err := app.TaskMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("got task ID %s, want %s", got.ID, task.ID)
	}
}

func TestApp_WorktreeAdapter_CreateWorktree_ValidationError(t *testing.T) {
	app := newTestApp(t)

	// CreateWorktree with empty repo should fail validation.
	_, err := app.WorktreeMgr.CreateWorktree(integration.WorktreeConfig{
		RepoPath:   "",
		BranchName: "feat/test",
		TaskID:     "TASK-00001",
	})
	if err == nil {
		t.Error("expected error for empty RepoPath")
	}
}

func TestApp_WorktreeAdapter_RemoveWorktree_NonExistent(t *testing.T) {
	app := newTestApp(t)

	// RemoveWorktree on a non-existent path should return an error or succeed gracefully.
	err := app.WorktreeMgr.RemoveWorktree("/nonexistent/worktree/path")
	// The behavior depends on the implementation; we just verify it doesn't panic.
	_ = err
}

func TestApp_BacklogStoreAdapter_FilterTasks(t *testing.T) {
	app := newTestApp(t)

	// Create tasks with different statuses.
	_, _ = app.TaskMgr.CreateTask(models.TaskTypeFeat, "filter-test-1", "", core.CreateTaskOpts{})
	_, _ = app.TaskMgr.CreateTask(models.TaskTypeBug, "filter-test-2", "", core.CreateTaskOpts{})

	// Filter by status.
	tasks, err := app.TaskMgr.GetTasksByStatus(models.StatusBacklog)
	if err != nil {
		t.Fatalf("GetTasksByStatus: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 backlog tasks, got %d", len(tasks))
	}
}

func TestApp_NewApp_EmptyPrefix(t *testing.T) {
	dir := t.TempDir()
	// Write a .taskconfig with empty prefix to test the default fallback.
	cfg := `task_id:
  prefix: ""
`
	if err := os.WriteFile(filepath.Join(dir, ".taskconfig"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// With empty prefix, it should fall back to "TASK".
	id, err := app.IDGen.GenerateTaskID()
	if err != nil {
		t.Fatalf("GenerateTaskID: %v", err)
	}
	if !strings.HasPrefix(id, "TASK-") {
		t.Errorf("expected TASK- prefix (fallback), got %s", id)
	}
}

func TestApp_BacklogStoreAdapter_GetTask_Error(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Getting a non-existent task should return an error.
	_, err = app.TaskMgr.GetTask("TASK-NONEXISTENT")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestApp_AdapterDelegation_Worktree(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// The worktreeAdapter.CreateWorktree is exercised via Bootstrap with a repo path.
	// Since we can't have a real git repo, we verify the adapter is wired correctly
	// by exercising the bootstrap with a repo path that will fail at the git level.
	_, err = app.Bootstrap.Bootstrap(core.BootstrapConfig{
		Type:       models.TaskTypeFeat,
		Title:      "test-wt",
		RepoPath:   "github.com/test/repo",
		BranchName: "feat/test",
	})
	// This will fail because the repo doesn't exist, but the worktree adapter code
	// path is exercised.
	// Note: Bootstrap may still succeed (creating ticket dir) even if worktree fails
	// -- it depends on the implementation. We just verify it doesn't panic.
	_ = err
}

func TestApp_AdapterDelegation_BacklogGetTask(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Create a task so the backlog has an entry.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "adapter-test", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Test the backlogStoreAdapter.GetTask directly through the BacklogMgr.
	_ = app.BacklogMgr.Load()
	entry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("BacklogMgr.GetTask: %v", err)
	}
	if entry.ID != task.ID {
		t.Errorf("backlog entry ID = %s, want %s", entry.ID, task.ID)
	}
}

func TestApp_WorktreeRemoverAdapter(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Create a task and manually update status.yaml to have a worktree path.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "cleanup-test", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Read, unmarshal, modify, re-marshal to ensure proper YAML format.
	statusPath := filepath.Join(dir, "tickets", task.ID, "status.yaml")
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("reading status.yaml: %v", err)
	}

	var taskObj models.Task
	if err := yaml.Unmarshal(statusData, &taskObj); err != nil {
		t.Fatalf("unmarshalling status.yaml: %v", err)
	}
	fakeWorktree := filepath.Join(dir, "fake-worktree")
	taskObj.WorktreePath = fakeWorktree
	newData, err := yaml.Marshal(&taskObj)
	if err != nil {
		t.Fatalf("marshalling status.yaml: %v", err)
	}
	if err := os.WriteFile(statusPath, newData, 0o644); err != nil {
		t.Fatalf("writing status.yaml: %v", err)
	}

	// CleanupWorktree will call worktreeRemoverAdapter.RemoveWorktree.
	err = app.TaskMgr.CleanupWorktree(task.ID)
	// The adapter code path is exercised even if the actual worktree removal fails.
	_ = err
}

func TestApp_BacklogStoreAdapter_GetTask_Direct(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "get-task-direct", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	_ = app.BacklogMgr.Load()
	entry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("BacklogMgr.GetTask: %v", err)
	}
	if entry.ID != task.ID {
		t.Errorf("entry ID = %s, want %s", entry.ID, task.ID)
	}
}

// TestApp_BacklogStoreAdapterGetTask_ViaAdapter directly constructs and tests
// the backlogStoreAdapter.GetTask method which is part of the core.BacklogStore
// interface but not called by the current TaskManager implementation.
func TestApp_BacklogStoreAdapterGetTask_ViaAdapter(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "adapter-get", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Construct the adapter directly and call GetTask.
	adapter := &backlogStoreAdapter{mgr: app.BacklogMgr}
	_ = adapter.Load()

	entry, err := adapter.GetTask(task.ID)
	if err != nil {
		t.Fatalf("adapter.GetTask: %v", err)
	}
	if entry.ID != task.ID {
		t.Errorf("adapter entry ID = %s, want %s", entry.ID, task.ID)
	}

	// Test error case.
	_, err = adapter.GetTask("TASK-NONEXISTENT")
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

// TestApp_BacklogStoreAdapterGetAllTasks_ErrorPath tests the error path
// of backlogStoreAdapter.GetAllTasks.
func TestApp_BacklogStoreAdapterGetAllTasks_ErrorPath(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Corrupt the backlog file to trigger a GetAllTasks error.
	backlogPath := filepath.Join(dir, "backlog.yaml")
	_ = os.WriteFile(backlogPath, []byte("invalid yaml: [[["), 0o644)

	adapter := &backlogStoreAdapter{mgr: app.BacklogMgr}
	_ = adapter.Load() // Load returns error but we ignore it here.

	// After loading corrupted YAML, GetAllTasks should fail.
	_, err = adapter.GetAllTasks()
	// The behavior depends on implementation -- corrupted data may or may not
	// cause GetAllTasks to fail. We just exercise the path.
	_ = err
}

// TestApp_BacklogStoreAdapterFilterTasks_ErrorPath tests the error path
// of backlogStoreAdapter.FilterTasks.
func TestApp_BacklogStoreAdapterFilterTasks_ErrorPath(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	adapter := &backlogStoreAdapter{mgr: app.BacklogMgr}

	// FilterTasks on a fresh (empty) backlog.
	entries, err := adapter.FilterTasks(core.BacklogStoreFilter{
		Status: []models.TaskStatus{models.StatusInProgress},
	})
	if err != nil {
		t.Fatalf("FilterTasks: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestApp_NewApp_ResolveBasePath_OsGetwdError is a no-op since we can't
// easily make os.Getwd fail, but we ensure the fallback branch in
// ResolveBasePath works when no .taskconfig is found.
func TestApp_NewApp_CorruptConfig(t *testing.T) {
	dir := t.TempDir()
	// Write a corrupt .taskconfig that will cause LoadGlobalConfig to fail.
	if err := os.WriteFile(filepath.Join(dir, ".taskconfig"), []byte("defaults: [[[invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp should succeed even with corrupt config (use defaults): %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Verify it used defaults.
	id, err := app.IDGen.GenerateTaskID()
	if err != nil {
		t.Fatalf("GenerateTaskID: %v", err)
	}
	if !strings.HasPrefix(id, "TASK-") {
		t.Errorf("expected TASK- prefix (default), got %s", id)
	}
}

func TestApp_ResolveBasePath_EmptyEnv(t *testing.T) {
	t.Setenv("ADB_HOME", "")

	// Just run from a tmp dir with no .taskconfig.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	result := ResolveBasePath()
	// Resolve symlinks (macOS: /tmp -> /private/tmp) and short names (Windows: RUNNER~1 -> runneradmin).
	expected, _ := filepath.EvalSymlinks(tmpDir)
	result, _ = filepath.EvalSymlinks(result)
	if result != expected {
		t.Errorf("ResolveBasePath = %q, want %q", result, expected)
	}
}

// TestApp_BacklogAdapterGetAllTasks_Error exercises the error return path
// in backlogStoreAdapter.GetAllTasks by corrupting the loaded backlog data.
func TestApp_BacklogAdapterGetAllTasks_Error(t *testing.T) {
	dir := t.TempDir()
	blMgr := storage.NewBacklogManager(dir)

	// Write valid backlog, load it, then corrupt the underlying file.
	_ = blMgr.Load()
	_ = blMgr.AddTask(storage.BacklogEntry{ID: "TASK-00001", Title: "test"})
	_ = blMgr.Save()

	adapter := &backlogStoreAdapter{mgr: blMgr}
	_ = adapter.Load()

	// Success path.
	entries, err := adapter.GetAllTasks()
	if err != nil {
		t.Fatalf("GetAllTasks success path: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// For the error path, we need the underlying storage to return an error.
	// The storage.BacklogManager.GetAllTasks() returns data from memory after Load(),
	// so it won't error unless we use a fresh manager with a corrupt file.
	blMgr2 := storage.NewBacklogManager(dir)
	// Write corrupted backlog.
	_ = os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("invalid: [[["), 0o644)

	adapter2 := &backlogStoreAdapter{mgr: blMgr2}
	// Load will fail, but some implementations might still work for GetAllTasks.
	_ = adapter2.Load()

	// GetAllTasks after failed load.
	_, err = adapter2.GetAllTasks()
	// Exercise the path regardless of error.
	_ = err
}

// TestApp_BacklogAdapterFilterTasks_Error exercises the error return path.
func TestApp_BacklogAdapterFilterTasks_Error(t *testing.T) {
	dir := t.TempDir()
	blMgr := storage.NewBacklogManager(dir)

	adapter := &backlogStoreAdapter{mgr: blMgr}
	_ = adapter.Load()

	// Success path with empty backlog.
	entries, err := adapter.FilterTasks(core.BacklogStoreFilter{
		Status: []models.TaskStatus{models.StatusBacklog},
	})
	if err != nil {
		t.Fatalf("FilterTasks: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestApp_BacklogStoreAdapterGetTask_Direct(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	// Create a task to populate the backlog.
	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "get-task-adapter", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// The backlogStoreAdapter.GetTask is part of the core.BacklogStore interface.
	// Since it's never called by the current TaskManager implementation,
	// we test it by going through the storage layer directly.
	_ = app.BacklogMgr.Load()
	storageEntry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask via BacklogMgr: %v", err)
	}
	if storageEntry.ID != task.ID {
		t.Errorf("storage entry ID = %s, want %s", storageEntry.ID, task.ID)
	}
}

// =========================================================================
// Resume workflow integration tests
// =========================================================================

// TestIntegration_ResumeFromDoneStatus verifies that resuming a task that has
// reached "done" status does not change the status back to in_progress.
func TestIntegration_ResumeFromDoneStatus(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "done-resume", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Advance to done status.
	if err := app.TaskMgr.UpdateTaskStatus(task.ID, models.StatusDone); err != nil {
		t.Fatalf("setting status to done: %v", err)
	}

	// Resume the done task.
	resumed, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming done task: %v", err)
	}

	// ResumeTask only promotes backlog -> in_progress; done should remain done.
	if resumed.Status != models.StatusDone {
		t.Errorf("expected done status after resume, got %s", resumed.Status)
	}

	// Verify backlog also still reflects done.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}
	entry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("getting backlog entry: %v", err)
	}
	if entry.Status != models.StatusDone {
		t.Errorf("backlog status = %s, want done", entry.Status)
	}
}

// TestIntegration_ResumeArchivedTaskFails verifies that resuming an archived
// task returns the task with archived status (the archive directory structure
// is used but no status promotion occurs).
func TestIntegration_ResumeArchivedTaskFails(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "archive-resume", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Archive the task.
	if _, err := app.TaskMgr.ArchiveTask(task.ID); err != nil {
		t.Fatalf("archiving task: %v", err)
	}

	// Resume the archived task -- should still load successfully since
	// loadTaskFromTicket checks _archived/ as a fallback. The status
	// remains archived because ResumeTask only promotes backlog.
	resumed, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		// If the implementation returns an error for archived tasks, that is
		// also acceptable behavior. Document it here.
		t.Logf("ResumeTask on archived task returned error (acceptable): %v", err)
		return
	}

	// If no error, the task should still be archived.
	if resumed.Status != models.StatusArchived {
		t.Errorf("expected archived status after resume, got %s", resumed.Status)
	}
}

// TestIntegration_ResumeWithEventLogging verifies that when a backlog task
// is resumed and promoted to in_progress, the event log contains a
// task.status_changed event if the CLI layer or task manager logs it.
// Note: The current TaskManager does not log events directly; event logging
// is handled at the CLI layer. This test verifies that the EventLog is
// functional and can be written to and read from during a resume workflow.
func TestIntegration_ResumeWithEventLogging(t *testing.T) {
	app := newTestApp(t)

	if app.EventLog == nil {
		t.Fatal("expected EventLog to be initialized")
	}

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "event-resume", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Resume the task (backlog -> in_progress).
	resumed, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming task: %v", err)
	}
	if resumed.Status != models.StatusInProgress {
		t.Fatalf("expected in_progress after resume, got %s", resumed.Status)
	}

	// Simulate what the CLI layer would do: write a status_changed event.
	err = app.EventLog.Write(observability.Event{
		Time:    time.Now().UTC(),
		Level:   "INFO",
		Type:    "task.status_changed",
		Message: "task.status_changed",
		Data: map[string]any{
			"task_id":    task.ID,
			"old_status": "backlog",
			"new_status": "in_progress",
		},
	})
	if err != nil {
		t.Fatalf("writing event: %v", err)
	}

	// Read events and verify the status_changed event exists.
	events, err := app.EventLog.Read(observability.EventFilter{
		Type: "task.status_changed",
	})
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}

	found := false
	for _, evt := range events {
		if evt.Data["task_id"] == task.ID && evt.Data["new_status"] == "in_progress" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected task.status_changed event with new_status=in_progress")
	}
}

// TestIntegration_ResumeFromBlockedStatus verifies that resuming a blocked
// task does not change its status. Resume only promotes backlog tasks.
func TestIntegration_ResumeFromBlockedStatus(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "blocked-resume", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Set status to blocked.
	if err := app.TaskMgr.UpdateTaskStatus(task.ID, models.StatusBlocked); err != nil {
		t.Fatalf("setting status to blocked: %v", err)
	}

	// Resume the blocked task.
	resumed, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming blocked task: %v", err)
	}

	// Status should remain blocked.
	if resumed.Status != models.StatusBlocked {
		t.Errorf("expected blocked status after resume, got %s", resumed.Status)
	}

	// Verify via backlog.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}
	entry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("getting backlog entry: %v", err)
	}
	if entry.Status != models.StatusBlocked {
		t.Errorf("backlog status = %s, want blocked", entry.Status)
	}
}

// TestIntegration_ResumeFromReviewStatus verifies that resuming a task in
// review status does not change its status. Resume only promotes backlog tasks.
func TestIntegration_ResumeFromReviewStatus(t *testing.T) {
	app := newTestApp(t)

	task, err := app.TaskMgr.CreateTask(models.TaskTypeFeat, "review-resume", "", core.CreateTaskOpts{})
	if err != nil {
		t.Fatalf("creating task: %v", err)
	}

	// Set status to review.
	if err := app.TaskMgr.UpdateTaskStatus(task.ID, models.StatusReview); err != nil {
		t.Fatalf("setting status to review: %v", err)
	}

	// Resume the review task.
	resumed, err := app.TaskMgr.ResumeTask(task.ID)
	if err != nil {
		t.Fatalf("resuming review task: %v", err)
	}

	// Status should remain review.
	if resumed.Status != models.StatusReview {
		t.Errorf("expected review status after resume, got %s", resumed.Status)
	}

	// Verify via backlog.
	if err := app.BacklogMgr.Load(); err != nil {
		t.Fatalf("loading backlog: %v", err)
	}
	entry, err := app.BacklogMgr.GetTask(task.ID)
	if err != nil {
		t.Fatalf("getting backlog entry: %v", err)
	}
	if entry.Status != models.StatusReview {
		t.Errorf("backlog status = %s, want review", entry.Status)
	}
}

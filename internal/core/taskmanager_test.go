package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Mock implementations

type MockBacklogStore struct {
	tasks   map[string]*models.Task
	loadErr error
	saveErr error
}

func NewMockBacklogStore() *MockBacklogStore {
	return &MockBacklogStore{
		tasks: make(map[string]*models.Task),
	}
}

func (m *MockBacklogStore) Load() (*models.Backlog, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	backlog := models.NewBacklog()
	for _, task := range m.tasks {
		backlog.AddTask(*task)
	}
	return backlog, nil
}

func (m *MockBacklogStore) Save(backlog *models.Backlog) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.tasks = make(map[string]*models.Task)
	for i := range backlog.Tasks {
		taskCopy := backlog.Tasks[i]
		m.tasks[taskCopy.ID] = &taskCopy
	}
	return nil
}

func (m *MockBacklogStore) AddTask(task models.Task) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if _, exists := m.tasks[task.ID]; exists {
		return fmt.Errorf("task with ID %s already exists", task.ID)
	}
	taskCopy := task
	m.tasks[task.ID] = &taskCopy
	return nil
}

func (m *MockBacklogStore) UpdateTask(task models.Task) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if _, exists := m.tasks[task.ID]; !exists {
		return fmt.Errorf("task with ID %s not found", task.ID)
	}
	taskCopy := task
	m.tasks[task.ID] = &taskCopy
	return nil
}

func (m *MockBacklogStore) GetTask(id string) (*models.Task, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	task, exists := m.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task with ID %s not found", id)
	}
	taskCopy := *task
	return &taskCopy, nil
}

func (m *MockBacklogStore) RemoveTask(id string) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if _, exists := m.tasks[id]; !exists {
		return fmt.Errorf("task with ID %s not found", id)
	}
	delete(m.tasks, id)
	return nil
}

type MockContextStore struct {
	contexts map[string]string
	notes    map[string]string
}

func NewMockContextStore() *MockContextStore {
	return &MockContextStore{
		contexts: make(map[string]string),
		notes:    make(map[string]string),
	}
}

func (m *MockContextStore) ReadContext(taskID string) (string, error) {
	return m.contexts[taskID], nil
}

func (m *MockContextStore) WriteContext(taskID string, content string) error {
	m.contexts[taskID] = content
	return nil
}

func (m *MockContextStore) AppendContext(taskID string, section string) error {
	m.contexts[taskID] += section
	return nil
}

func (m *MockContextStore) ReadNotes(taskID string) (string, error) {
	return m.notes[taskID], nil
}

func (m *MockContextStore) WriteNotes(taskID string, content string) error {
	m.notes[taskID] = content
	return nil
}

type MockWorktreeCreator struct {
	worktrees        map[string]string // taskID -> worktreePath
	createdBranch    map[string]string // taskID -> branch passed to CreateWorktree
	existingBranches map[string]bool   // branch -> already exists (the #208 collision guard)
	createErr        error
	shouldFail       bool
}

func NewMockWorktreeCreator() *MockWorktreeCreator {
	return &MockWorktreeCreator{
		worktrees:        make(map[string]string),
		createdBranch:    make(map[string]string),
		existingBranches: make(map[string]bool),
	}
}

func (m *MockWorktreeCreator) CreateWorktree(taskID, branchName, worktreePath, repoPath string) error {
	if m.shouldFail || m.createErr != nil {
		if m.createErr != nil {
			return m.createErr
		}
		return fmt.Errorf("worktree creation failed")
	}
	m.worktrees[taskID] = worktreePath
	m.createdBranch[taskID] = branchName
	return nil
}

func (m *MockWorktreeCreator) BranchExists(repoPath, branch string) (bool, error) {
	return m.existingBranches[branch], nil
}

// NormalizeRepoPath returns the repo path unchanged. Real callers go through
// integration.DefaultGitWorktreeManager.NormalizeRepoPath; for unit tests we
// just need a callable that doesn't error so the TaskManager.Create path
// keeps working when --repo is set.
func (m *MockWorktreeCreator) NormalizeRepoPath(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repoPath cannot be empty")
	}
	return repoPath, nil
}

type MockWorktreeRemover struct {
	removed         []string
	removedForce    []bool
	removedBranches []string // "repoPath:branch"
	removeErr       error
	removeBranchErr error
}

func NewMockWorktreeRemover() *MockWorktreeRemover {
	return &MockWorktreeRemover{
		removed: []string{},
	}
}

func (m *MockWorktreeRemover) RemoveWorktree(worktreePath string, force bool) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, worktreePath)
	m.removedForce = append(m.removedForce, force)
	return nil
}

func (m *MockWorktreeRemover) RemoveBranch(repoPath, branch string) error {
	if m.removeBranchErr != nil {
		return m.removeBranchErr
	}
	m.removedBranches = append(m.removedBranches, repoPath+":"+branch)
	return nil
}

type MockSerenaProvisioner struct {
	provisioned []string // worktreePaths passed to Provision
	err         error
}

func (m *MockSerenaProvisioner) Provision(worktreePath, projectName string) error {
	m.provisioned = append(m.provisioned, worktreePath)
	return m.err
}

type MockEventLogger struct {
	events []map[string]interface{}
}

func NewMockEventLogger() *MockEventLogger {
	return &MockEventLogger{
		events: []map[string]interface{}{},
	}
}

func (m *MockEventLogger) Log(eventType string, data map[string]interface{}) {
	// Mirror the production observability.Event{Type, Data} shape: the event
	// type and the payload live in SEPARATE namespaces. (An earlier version
	// flattened data into the same map as "type", which made it impossible to
	// carry a data key literally named "type" — e.g. task.created's task type —
	// and quietly masked the metrics under-count fixed in #148.)
	m.events = append(m.events, map[string]interface{}{
		"type": eventType,
		"data": data,
	})
}

// eventData reads a payload field from a recorded MockEventLogger event,
// returning nil when the field (or the data map) is absent. Keeps the event
// type ("type") and its payload ("data") in distinct namespaces, like the
// real observability.Event.
func eventData(e map[string]interface{}, key string) interface{} {
	if d, ok := e["data"].(map[string]interface{}); ok {
		return d[key]
	}
	return nil
}

// assertHasEvent fails the test unless some recorded event has the given type.
func assertHasEvent(t *testing.T, events []map[string]interface{}, eventType string) {
	t.Helper()
	for _, e := range events {
		if e["type"] == eventType {
			return
		}
	}
	t.Errorf("expected an event of type %q, got %v", eventType, events)
}

// assertEventWithPath fails unless some event of the given type carries the
// expected data.path (the worktree.created / worktree.removed contract, #206).
func assertEventWithPath(t *testing.T, events []map[string]interface{}, eventType, wantPath string) {
	t.Helper()
	for _, e := range events {
		if e["type"] == eventType {
			if got := eventData(e, "path"); got != wantPath {
				t.Errorf("%s event: path = %v, want %q", eventType, got, wantPath)
			}
			return
		}
	}
	t.Errorf("expected an event of type %q, got %v", eventType, events)
}

type MockSessionCapturer struct {
	sessions map[string]map[string]interface{}
}

func NewMockSessionCapturer() *MockSessionCapturer {
	return &MockSessionCapturer{
		sessions: make(map[string]map[string]interface{}),
	}
}

func (m *MockSessionCapturer) CaptureSession(taskID, sessionID string, data map[string]interface{}) error {
	key := fmt.Sprintf("%s:%s", taskID, sessionID)
	m.sessions[key] = data
	return nil
}

type MockTerminalStateUpdater struct {
	states map[string]map[string]interface{}
}

func NewMockTerminalStateUpdater() *MockTerminalStateUpdater {
	return &MockTerminalStateUpdater{
		states: make(map[string]map[string]interface{}),
	}
}

func (m *MockTerminalStateUpdater) WriteTerminalState(worktreePath string, taskID string, state map[string]interface{}) error {
	key := fmt.Sprintf("%s:%s", worktreePath, taskID)
	m.states[key] = state
	return nil
}

type MockTaskIDGenerator struct {
	counter int
	prefix  string
}

func NewMockTaskIDGenerator(prefix string) *MockTaskIDGenerator {
	return &MockTaskIDGenerator{
		counter: 0,
		prefix:  prefix,
	}
}

func (m *MockTaskIDGenerator) GenerateTaskID() (string, error) {
	m.counter++
	return fmt.Sprintf("%s-%05d", m.prefix, m.counter), nil
}

type MockTemplateManager struct{}

func NewMockTemplateManager() *MockTemplateManager {
	return &MockTemplateManager{}
}

func (m *MockTemplateManager) Render(templateType TemplateType, data interface{}) (string, error) {
	bytes, err := m.RenderBytes(templateType, data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (m *MockTemplateManager) RenderBytes(templateType TemplateType, data interface{}) ([]byte, error) {
	// Simple mock implementation
	switch templateType {
	case TemplateTypeHandoff:
		return []byte("# Handoff Document\n\nTask archived."), nil
	case TemplateTypeStatus:
		return []byte("status: pending"), nil
	case TemplateTypeContext:
		return []byte("# Context\n\nTask context."), nil
	case TemplateTypeNotes:
		return []byte("# Notes\n\nTask notes."), nil
	case TemplateTypeDesign:
		return []byte("# Design\n\nTask design."), nil
	case TemplateTypeTaskContext:
		return []byte("# Task Context\n\nFor Claude."), nil
	default:
		return nil, fmt.Errorf("unknown template type: %s", templateType)
	}
}

// Helper function to create a test TaskManager
func createTestTaskManager(t *testing.T) (*TaskManager, *MockBacklogStore, *MockEventLogger, *MockWorktreeCreator, *MockWorktreeRemover, string) {
	t.Helper()

	// Create temp directories
	tempDir := t.TempDir()
	ticketsDir := filepath.Join(tempDir, "tickets")
	archivedDir := filepath.Join(tempDir, "_archived")
	worktreesDir := filepath.Join(tempDir, "worktrees")

	// Create mocks
	backlogStore := NewMockBacklogStore()
	contextStore := NewMockContextStore()
	worktreeCreator := NewMockWorktreeCreator()
	worktreeRemover := NewMockWorktreeRemover()
	eventLogger := NewMockEventLogger()
	sessionCapturer := NewMockSessionCapturer()
	terminalStateUpdater := NewMockTerminalStateUpdater()
	taskIDGenerator := NewMockTaskIDGenerator("TASK")
	templateManager := NewMockTemplateManager()

	tm := NewTaskManager(
		backlogStore,
		contextStore,
		worktreeCreator,
		worktreeRemover,
		eventLogger,
		sessionCapturer,
		terminalStateUpdater,
		taskIDGenerator,
		templateManager,
		ticketsDir,
		archivedDir,
		worktreesDir,
	)

	return tm, backlogStore, eventLogger, worktreeCreator, worktreeRemover, tempDir
}

func TestTaskManager_Create(t *testing.T) {
	tm, backlogStore, eventLogger, worktreeCreator, _, tempDir := createTestTaskManager(t)

	opts := CreateTaskOpts{
		Title:              "Test Task",
		Description:        "Test description",
		AcceptanceCriteria: []string{"AC1", "AC2"},
		TaskType:           models.TaskTypeFeat,
		Priority:           models.PriorityP1,
		Owner:              "test-owner",
		Tags:               []string{"tag1", "tag2"},
		Repo:               "test-repo",
	}

	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify task was created
	if task.ID != "TASK-00001" {
		t.Errorf("Expected task ID TASK-00001, got %s", task.ID)
	}
	if task.Title != opts.Title {
		t.Errorf("Expected title %s, got %s", opts.Title, task.Title)
	}
	if task.Priority != opts.Priority {
		t.Errorf("Expected priority %s, got %s", opts.Priority, task.Priority)
	}
	if task.Owner != opts.Owner {
		t.Errorf("Expected owner %s, got %s", opts.Owner, task.Owner)
	}
	if task.Status != models.TaskStatusBacklog {
		t.Errorf("Expected status backlog, got %s", task.Status)
	}

	// Verify task in backlog
	storedTask, err := backlogStore.GetTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task from backlog: %v", err)
	}
	if storedTask.ID != task.ID {
		t.Errorf("Task ID mismatch in backlog")
	}

	// Verify worktree was created
	if _, exists := worktreeCreator.worktrees[task.ID]; !exists {
		t.Errorf("Worktree was not created for task %s", task.ID)
	}

	// Verify events were logged: task.created + worktree.created (the latter
	// so the "Worktrees Active" metric balances against worktree.removed, #206).
	if len(eventLogger.events) != 2 {
		t.Errorf("Expected 2 events (task.created + worktree.created), got %d", len(eventLogger.events))
	} else {
		event := eventLogger.events[0]
		if event["type"] != "task.created" {
			t.Errorf("Expected event type task.created, got %s", event["type"])
		}
		if got := eventData(event, "task_id"); got != task.ID {
			t.Errorf("Expected task_id %s in event, got %v", task.ID, got)
		}
		// The task.created payload is a contract (docs/claude/subsystems.md):
		// task_id, title, type, status, priority. The metrics calculator reads
		// data["type"] and data["status"] to build TasksByType/TasksByStatus, so
		// the emit MUST carry both — see internal/observability/metrics.go (#148).
		if got := eventData(event, "type"); got != string(opts.TaskType) {
			t.Errorf("Expected data.type %q in event, got %v", opts.TaskType, got)
		}
		if got := eventData(event, "status"); got != string(models.TaskStatusBacklog) {
			t.Errorf("Expected data.status %q in event, got %v", models.TaskStatusBacklog, got)
		}

		// worktree.created carries task_id + the worktree path (#206).
		wt := eventLogger.events[1]
		if wt["type"] != "worktree.created" {
			t.Errorf("Expected second event worktree.created, got %s", wt["type"])
		}
		if got := eventData(wt, "path"); got != task.WorktreePath {
			t.Errorf("Expected worktree.created path %q, got %v", task.WorktreePath, got)
		}
	}

	// Verify the task directory was created at the nested correlation
	// path. Repo "test-repo" passes NormalizeRepoPath through unchanged
	// (it's not absolute and not URL-shaped) and Title "Test Task"
	// slugifies to "test-task", so the on-disk leaf is
	// tickets/test-repo/TASK-00001-test-task.
	expectedTaskDir := filepath.Join(tempDir, "tickets", "test-repo", task.ID+"-test-task")
	if task.TicketPath != expectedTaskDir {
		t.Errorf("TicketPath = %q, want %q", task.TicketPath, expectedTaskDir)
	}
	if _, err := os.Stat(expectedTaskDir); os.IsNotExist(err) {
		t.Errorf("Task directory was not created: %s", expectedTaskDir)
	}

	// Verify the worktree path mirrors the nested layout, and the branch
	// is the Conventional <conv-type>/<slug> form (feat/test-task), NOT
	// task/TASK-00001.
	expectedWorktreePath := filepath.Join(tempDir, "worktrees", "test-repo", task.ID+"-test-task")
	if task.WorktreePath != expectedWorktreePath {
		t.Errorf("WorktreePath = %q, want %q", task.WorktreePath, expectedWorktreePath)
	}
	if task.Branch != "feat/test-task" {
		t.Errorf("Branch = %q, want %q", task.Branch, "feat/test-task")
	}
	if task.Slug != "test-task" {
		t.Errorf("Slug = %q, want %q", task.Slug, "test-task")
	}
}

func TestTaskManager_Resume(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, _ := createTestTaskManager(t)

	// Create a task first
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events from creation
	eventLogger.events = []map[string]interface{}{}

	// Resume the task
	resumedTask, err := tm.Resume(task.ID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	// Verify status changed to in_progress
	if resumedTask.Status != models.TaskStatusInProgress {
		t.Errorf("Expected status in_progress, got %s", resumedTask.Status)
	}

	// Verify backlog was updated
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.Status != models.TaskStatusInProgress {
		t.Errorf("Backlog not updated with new status")
	}

	// Verify event was logged
	if len(eventLogger.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventLogger.events))
	} else {
		event := eventLogger.events[0]
		if event["type"] != "task.status_changed" {
			t.Errorf("Expected event type task.status_changed, got %s", event["type"])
		}
	}
}

// TestTaskManager_Resume_RefreshesWorktreeTaskContext verifies that resuming a
// task with a worktree re-renders the worktree Tier-0 file
// (.claude/rules/task-context.md) via the shared generateTaskContext path and
// logs a config.task_context_synced event (per the ticket-bootstrap-context
// ADR, part B). This is the T3 resume-refresh behavior.
func TestTaskManager_Resume_RefreshesWorktreeTaskContext(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, tempDir := createTestTaskManager(t)

	// Seed an in-progress task that already has a worktree on disk. Point the
	// worktree at a real temp directory so generateTaskContext can create
	// .claude/rules/ under it.
	worktreePath := filepath.Join(tempDir, "wt", "TASK-00001")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}
	task := models.NewTask("TASK-00001", "Resume Refresh Task", models.TaskTypeFeat)
	task.Status = models.TaskStatusInProgress
	task.WorktreePath = worktreePath
	task.Branch = "feat/resume-refresh-task"
	task.TicketPath = filepath.Join(tempDir, "tickets", "_local", "TASK-00001-resume-refresh-task")
	if err := backlogStore.AddTask(*task); err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	eventLogger.events = []map[string]interface{}{}

	if _, err := tm.Resume(task.ID); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	// The worktree Tier-0 file must have been (re)written.
	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")
	if _, err := os.Stat(taskContextPath); os.IsNotExist(err) {
		t.Fatalf("worktree task-context.md was not rendered at %s", taskContextPath)
	}

	// A config.task_context_synced event must have been logged (the status
	// wasn't changed here since the task was already in_progress, so this is
	// the only event we expect).
	found := false
	for _, e := range eventLogger.events {
		if e["type"] == "config.task_context_synced" {
			found = true
			if got := eventData(e, "trigger"); got != "resume" {
				t.Errorf("expected trigger=resume, got %v", got)
			}
		}
	}
	if !found {
		t.Errorf("expected a config.task_context_synced event, got events: %v", eventLogger.events)
	}
}

// TestTaskManager_Resume_NoWorktree_NoRefresh verifies that resuming a
// repo-less task (empty WorktreePath) does NOT attempt a worktree refresh and
// emits no config.task_context_synced event.
func TestTaskManager_Resume_NoWorktree_NoRefresh(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, _ := createTestTaskManager(t)

	task := models.NewTask("TASK-00001", "No Worktree Task", models.TaskTypeSpike)
	task.Status = models.TaskStatusInProgress // no WorktreePath
	if err := backlogStore.AddTask(*task); err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	eventLogger.events = []map[string]interface{}{}

	if _, err := tm.Resume(task.ID); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	for _, e := range eventLogger.events {
		if e["type"] == "config.task_context_synced" {
			t.Errorf("did not expect a config.task_context_synced event for a repo-less task")
		}
	}
}

func TestTaskManager_Resume_ArchivedTask(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)

	// Create a task and set it as archived
	task := models.NewTask("TASK-00001", "Test Task", models.TaskTypeFeat)
	task.Status = models.TaskStatusArchived
	_ = backlogStore.AddTask(*task)

	// Try to resume archived task
	_, err := tm.Resume(task.ID)
	if err == nil {
		t.Errorf("Expected error when resuming archived task, got nil")
	}
}

func TestTaskManager_Archive(t *testing.T) {
	tm, backlogStore, eventLogger, _, worktreeRemover, tempDir := createTestTaskManager(t)

	// Create a task with repo so worktree is created
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/test/repo",
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events from creation
	eventLogger.events = []map[string]interface{}{}

	// Archive the task
	err = tm.Archive(task.ID, ArchiveOptions{})
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	// Verify task status is archived
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.Status != models.TaskStatusArchived {
		t.Errorf("Expected status archived, got %s", storedTask.Status)
	}

	// Verify task directory was moved to _archived under the nested
	// path: _archived/<repo-subpath>/TASK-id-slug.
	archivedDir := filepath.Join(tempDir, "_archived", "github.com/test/repo", task.ID+"-test-task")
	if _, err := os.Stat(archivedDir); os.IsNotExist(err) {
		t.Errorf("Archived task directory not found: %s", archivedDir)
	}

	// Verify handoff.md was created
	handoffPath := filepath.Join(archivedDir, "handoff.md")
	if _, err := os.Stat(handoffPath); os.IsNotExist(err) {
		t.Errorf("handoff.md not found: %s", handoffPath)
	}

	// Verify worktree was removed
	if len(worktreeRemover.removed) != 1 {
		t.Errorf("Expected 1 worktree removal, got %d", len(worktreeRemover.removed))
	}

	// Archive emits both worktree.removed (with the pre-clear path, #206) and
	// task.archived.
	if len(eventLogger.events) != 2 {
		t.Errorf("Expected 2 events (worktree.removed + task.archived), got %d", len(eventLogger.events))
	}
	assertEventWithPath(t, eventLogger.events, "worktree.removed", task.WorktreePath)
	assertHasEvent(t, eventLogger.events, "task.archived")
}

// TestTaskManager_Archive_Idempotent guards #159: re-archiving an already-archived
// task must be refused, not re-run — a second archive re-nested the ticket dir
// under a second _archived/ segment (tickets/_archived/_archived/…) and left a
// TicketPath a later Unarchive would mis-restore.
func TestTaskManager_Archive_Idempotent(t *testing.T) {
	tm, backlogStore, _, _, _, tempDir := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/test/repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := tm.Archive(task.ID, ArchiveOptions{}); err != nil {
		t.Fatalf("first Archive failed: %v", err)
	}

	// Second archive must be refused.
	if err := tm.Archive(task.ID, ArchiveOptions{}); err == nil {
		t.Fatal("re-archiving an already-archived task should error")
	} else if !strings.Contains(err.Error(), "already archived") {
		t.Errorf("re-archive failed for the wrong reason: %v", err)
	}

	// No double-_archived directory must exist, and the single archived dir stays.
	doubleNested := filepath.Join(tempDir, "_archived", "_archived")
	if _, err := os.Stat(doubleNested); !os.IsNotExist(err) {
		t.Errorf("re-archive created a double-nested %s", doubleNested)
	}
	archivedDir := filepath.Join(tempDir, "_archived", "github.com/test/repo", task.ID+"-test-task")
	if _, err := os.Stat(archivedDir); os.IsNotExist(err) {
		t.Errorf("archived task dir should still be at %s", archivedDir)
	}

	// The stored TicketPath must still point at the single-archived dir, so a
	// subsequent Unarchive restores it correctly under tickets/ (not _archived/).
	stored, _ := backlogStore.GetTask(task.ID)
	if stored.TicketPath != archivedDir {
		t.Errorf("TicketPath = %q after refused re-archive, want %q", stored.TicketPath, archivedDir)
	}
	if err := tm.Unarchive(task.ID); err != nil {
		t.Fatalf("Unarchive after refused re-archive failed: %v", err)
	}
	restored, _ := backlogStore.GetTask(task.ID)
	wantRestored := filepath.Join(tempDir, "tickets", "github.com/test/repo", task.ID+"-test-task")
	if restored.TicketPath != wantRestored {
		t.Errorf("Unarchive restored to %q, want %q (must be under tickets/, not _archived/)", restored.TicketPath, wantRestored)
	}
	if strings.Contains(restored.TicketPath, "_archived") {
		t.Errorf("restored ticket path still under _archived: %q", restored.TicketPath)
	}
}

func TestTaskManager_Unarchive(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, tempDir := createTestTaskManager(t)

	// Create and archive a task
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = tm.Archive(task.ID, ArchiveOptions{})
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	// Clear events
	eventLogger.events = []map[string]interface{}{}

	// Unarchive the task
	err = tm.Unarchive(task.ID)
	if err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	// Verify task status is backlog
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.Status != models.TaskStatusBacklog {
		t.Errorf("Expected status backlog, got %s", storedTask.Status)
	}

	// Verify task directory was moved back to tickets at the nested
	// path. Created without --repo so it lives under _local/.
	activeDir := filepath.Join(tempDir, "tickets", "_local", task.ID+"-test-task")
	if _, err := os.Stat(activeDir); os.IsNotExist(err) {
		t.Errorf("Active task directory not found: %s", activeDir)
	}

	// Verify event was logged
	if len(eventLogger.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventLogger.events))
	} else {
		event := eventLogger.events[0]
		if event["type"] != "task.unarchived" {
			t.Errorf("Expected event type task.unarchived, got %s", event["type"])
		}
	}
}

func TestTaskManager_UpdateStatus(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, _ := createTestTaskManager(t)

	// Create a task
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events
	eventLogger.events = []map[string]interface{}{}

	// Update status
	err = tm.UpdateStatus(task.ID, models.TaskStatusReview)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify status was updated
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.Status != models.TaskStatusReview {
		t.Errorf("Expected status review, got %s", storedTask.Status)
	}

	// Verify event was logged
	if len(eventLogger.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventLogger.events))
	} else {
		event := eventLogger.events[0]
		if event["type"] != "task.status_changed" {
			t.Errorf("Expected event type task.status_changed, got %s", event["type"])
		}
	}
}

func TestTaskManager_StartAll(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)

	// Three backlog tasks (no repo => no worktree).
	for i := 0; i < 3; i++ {
		if _, err := tm.Create(CreateTaskOpts{Title: "task", TaskType: models.TaskTypeFeat}); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}
	// One already in review — must be left alone by StartAll.
	review, err := tm.Create(CreateTaskOpts{Title: "review", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := tm.UpdateStatus(review.ID, models.TaskStatusReview); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	results, err := tm.StartAll()
	if err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 eligible backlog tasks, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("task %s errored: %v", r.TaskID, r.Err)
		}
		if r.NewStatus != models.TaskStatusInProgress {
			t.Errorf("task %s: expected in_progress, got %s", r.TaskID, r.NewStatus)
		}
	}

	// The review task stayed in review.
	stored, _ := backlogStore.GetTask(review.ID)
	if stored.Status != models.TaskStatusReview {
		t.Errorf("review task was disturbed: status=%s", stored.Status)
	}

	// Idempotent: a second run finds nothing in backlog.
	results2, err := tm.StartAll()
	if err != nil {
		t.Fatalf("second StartAll failed: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("expected StartAll to be idempotent, got %d results", len(results2))
	}
}

func TestTaskManager_CloseAll(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)

	// One in_progress, one blocked, one review => all active => all close.
	active := []models.TaskStatus{models.TaskStatusInProgress, models.TaskStatusBlocked, models.TaskStatusReview}
	for _, st := range active {
		task, err := tm.Create(CreateTaskOpts{Title: "active", TaskType: models.TaskTypeFeat})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := tm.UpdateStatus(task.ID, st); err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}
	}
	// One left in backlog — CloseAll must NOT touch it.
	backlogTask, err := tm.Create(CreateTaskOpts{Title: "backlog", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	results, err := tm.CloseAll()
	if err != nil {
		t.Fatalf("CloseAll failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 active tasks closed, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("task %s errored: %v", r.TaskID, r.Err)
		}
		if r.NewStatus != models.TaskStatusDone {
			t.Errorf("task %s: expected done, got %s", r.TaskID, r.NewStatus)
		}
	}

	// Backlog task is still backlog.
	stored, _ := backlogStore.GetTask(backlogTask.ID)
	if stored.Status != models.TaskStatusBacklog {
		t.Errorf("backlog task was disturbed: status=%s", stored.Status)
	}
}

func TestTaskManager_UpdatePriority(t *testing.T) {
	tm, backlogStore, eventLogger, _, _, _ := createTestTaskManager(t)

	// Create a task
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Priority: models.PriorityP2,
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events
	eventLogger.events = []map[string]interface{}{}

	// Update priority
	err = tm.UpdatePriority(task.ID, models.PriorityP0)
	if err != nil {
		t.Fatalf("UpdatePriority failed: %v", err)
	}

	// Verify priority was updated
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.Priority != models.PriorityP0 {
		t.Errorf("Expected priority P0, got %s", storedTask.Priority)
	}

	// Verify event was logged
	if len(eventLogger.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventLogger.events))
	} else {
		event := eventLogger.events[0]
		if event["type"] != "task.priority_changed" {
			t.Errorf("Expected event type task.priority_changed, got %s", event["type"])
		}
	}
}

func TestTaskManager_Cleanup(t *testing.T) {
	tm, backlogStore, eventLogger, _, worktreeRemover, _ := createTestTaskManager(t)

	// Create a task with repo so worktree is created
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/test/repo",
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events
	eventLogger.events = []map[string]interface{}{}

	// Cleanup
	err = tm.Cleanup(task.ID, false)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify worktree was removed
	if len(worktreeRemover.removed) != 1 {
		t.Errorf("Expected 1 worktree removal, got %d", len(worktreeRemover.removed))
	}

	// Verify task worktree path was cleared
	storedTask, _ := backlogStore.GetTask(task.ID)
	if storedTask.WorktreePath != "" {
		t.Errorf("Expected empty worktree path, got %s", storedTask.WorktreePath)
	}

	// Verify a worktree.removed event carrying the removed path was logged (#206).
	if len(eventLogger.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventLogger.events))
	}
	assertEventWithPath(t, eventLogger.events, "worktree.removed", task.WorktreePath)
}

// TestTaskManager_Archive_KeepWorktree verifies --keep-worktree genuinely keeps
// the worktree: no removal is attempted and the task retains its worktree path
// while still being archived (#207).
func TestTaskManager_Archive_KeepWorktree(t *testing.T) {
	tm, backlogStore, _, _, worktreeRemover, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := tm.Archive(task.ID, ArchiveOptions{KeepWorktree: true}); err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	if len(worktreeRemover.removed) != 0 {
		t.Errorf("expected no worktree removal with KeepWorktree, got %v", worktreeRemover.removed)
	}
	stored, _ := backlogStore.GetTask(task.ID)
	if stored.Status != models.TaskStatusArchived {
		t.Errorf("expected status archived, got %s", stored.Status)
	}
	if stored.WorktreePath == "" {
		t.Errorf("expected worktree path preserved with KeepWorktree, got empty")
	}
}

// TestTaskManager_Archive_PruneBranch verifies opt-in branch cleanup: the
// task's local branch is deleted after its worktree is removed (#207).
func TestTaskManager_Archive_PruneBranch(t *testing.T) {
	tm, _, _, _, worktreeRemover, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := tm.Archive(task.ID, ArchiveOptions{PruneBranch: true}); err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	if len(worktreeRemover.removed) != 1 {
		t.Errorf("expected 1 worktree removal, got %d", len(worktreeRemover.removed))
	}
	want := task.Repo + ":" + task.Branch
	if len(worktreeRemover.removedBranches) != 1 || worktreeRemover.removedBranches[0] != want {
		t.Errorf("expected branch prune %q, got %v", want, worktreeRemover.removedBranches)
	}
}

// TestTaskManager_Cleanup_ForcePropagates verifies the --force flag threads down
// to the worktree remover so the dirty/unpushed guard can be overridden (#207).
func TestTaskManager_Cleanup_ForcePropagates(t *testing.T) {
	tm, _, _, _, worktreeRemover, _ := createTestTaskManager(t)

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := tm.Cleanup(task.ID, true); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if len(worktreeRemover.removedForce) != 1 || !worktreeRemover.removedForce[0] {
		t.Errorf("expected force=true propagated to RemoveWorktree, got %v", worktreeRemover.removedForce)
	}
}

func TestTaskManager_Delete(t *testing.T) {
	tm, backlogStore, eventLogger, _, worktreeRemover, tempDir := createTestTaskManager(t)

	// Create a task with repo so worktree is created
	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/test/repo",
	}
	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Clear events
	eventLogger.events = []map[string]interface{}{}

	// Delete
	err = tm.Delete(task.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify worktree was removed
	if len(worktreeRemover.removed) != 1 {
		t.Errorf("Expected 1 worktree removal, got %d", len(worktreeRemover.removed))
	}

	// Verify task directory was removed (nested path).
	taskDir := filepath.Join(tempDir, "tickets", "github.com/test/repo", task.ID+"-test-task")
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Errorf("Task directory still exists: %s", taskDir)
	}

	// Verify task was removed from backlog
	_, err = backlogStore.GetTask(task.ID)
	if err == nil {
		t.Errorf("Task still exists in backlog")
	}

	// Delete emits both worktree.removed (with the pre-clear path, #206) and
	// task.deleted.
	if len(eventLogger.events) != 2 {
		t.Errorf("Expected 2 events (worktree.removed + task.deleted), got %d", len(eventLogger.events))
	}
	assertEventWithPath(t, eventLogger.events, "worktree.removed", task.WorktreePath)
	assertHasEvent(t, eventLogger.events, "task.deleted")
}

func TestTaskManager_Create_WithoutRepo_NoWorktree(t *testing.T) {
	tm, backlogStore, _, worktreeCreator, _, tempDir := createTestTaskManager(t)

	// Create a task without specifying a repo
	opts := CreateTaskOpts{
		Title:    "Idea Task",
		TaskType: models.TaskTypeSpike,
	}

	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify task was created in backlog
	storedTask, err := backlogStore.GetTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task from backlog: %v", err)
	}
	if storedTask.ID != task.ID {
		t.Errorf("Task ID mismatch in backlog")
	}

	// Verify NO worktree was created
	if len(worktreeCreator.worktrees) != 0 {
		t.Errorf("Expected no worktrees, got %d", len(worktreeCreator.worktrees))
	}

	// Verify task has no worktree path or branch
	if task.WorktreePath != "" {
		t.Errorf("Expected empty worktree path, got %s", task.WorktreePath)
	}
	if task.Branch != "" {
		t.Errorf("Expected empty branch, got %s", task.Branch)
	}

	// Verify ticket directory was still created at the nested
	// _local/TASK-id-slug path (repo-less tasks live under _local/).
	taskDir := filepath.Join(tempDir, "tickets", "_local", task.ID+"-idea-task")
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		t.Errorf("Task directory was not created: %s", taskDir)
	}
}

// TestTaskManager_Create_BranchCollisionDisambiguates verifies that a derived
// <type>/<slug> that already exists is disambiguated by appending the task id,
// and that the disambiguated branch is what's recorded + passed to git (#208).
func TestTaskManager_Create_BranchCollisionDisambiguates(t *testing.T) {
	tm, _, _, worktreeCreator, _, _ := createTestTaskManager(t)
	worktreeCreator.existingBranches["feat/test-task"] = true

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	want := "feat/test-task-" + strings.ToLower(task.ID)
	if task.Branch != want {
		t.Errorf("expected disambiguated branch %q, got %q", want, task.Branch)
	}
	if got := worktreeCreator.createdBranch[task.ID]; got != want {
		t.Errorf("CreateWorktree got branch %q, want disambiguated %q", got, want)
	}
}

// TestTaskManager_Create_BranchCollisionUnresolvable verifies a clear error
// (not a raw git failure) when both the derived branch and its disambiguated
// form are already taken (#208).
func TestTaskManager_Create_BranchCollisionUnresolvable(t *testing.T) {
	tm, _, _, worktreeCreator, _, _ := createTestTaskManager(t)
	worktreeCreator.existingBranches["feat/test-task"] = true
	worktreeCreator.existingBranches["feat/test-task-task-00001"] = true

	_, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err == nil {
		t.Fatal("expected Create to fail when both branch names are taken")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected a clear collision error, got: %v", err)
	}
}

// TestTaskManager_Create_ProvisionsSerena verifies the provisioner fires on the
// worktree-bootstrap seam for a repo-backed code task (#202).
func TestTaskManager_Create_ProvisionsSerena(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	prov := &MockSerenaProvisioner{}
	tm.SetSerenaProvisioner(prov)

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(prov.provisioned) != 1 || prov.provisioned[0] != task.WorktreePath {
		t.Errorf("expected Serena provisioned for %q, got %v", task.WorktreePath, prov.provisioned)
	}
}

// TestTaskManager_Create_SerenaFailOpen verifies a provisioning error never
// blocks task/worktree creation (#202, fail-open).
func TestTaskManager_Create_SerenaFailOpen(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)
	prov := &MockSerenaProvisioner{err: fmt.Errorf("serena boom")}
	tm.SetSerenaProvisioner(prov)

	task, err := tm.Create(CreateTaskOpts{Title: "Test Task", TaskType: models.TaskTypeFeat, Repo: "github.com/test/repo"})
	if err != nil {
		t.Fatalf("Create must not fail when Serena provisioning errors (fail-open): %v", err)
	}
	if len(prov.provisioned) != 1 {
		t.Errorf("expected provisioning attempted once, got %d", len(prov.provisioned))
	}
	if _, gerr := backlogStore.GetTask(task.ID); gerr != nil {
		t.Errorf("task should exist despite provisioning error: %v", gerr)
	}
}

// TestTaskManager_Create_WorkType_NoSerenaProvision verifies work-type tasks
// (no worktree) never trigger provisioning (#202).
func TestTaskManager_Create_WorkType_NoSerenaProvision(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)
	prov := &MockSerenaProvisioner{}
	tm.SetSerenaProvisioner(prov)

	if _, err := tm.Create(CreateTaskOpts{Title: "Do a thing", TaskType: models.TaskTypeWork, Repo: "github.com/test/repo"}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(prov.provisioned) != 0 {
		t.Errorf("work-type task must not provision Serena, got %v", prov.provisioned)
	}
}

// TestTaskManager_Start verifies the singular start promotes a backlog task to
// in_progress and is a no-op for a task already beyond backlog (#210).
func TestTaskManager_Start(t *testing.T) {
	tm, backlogStore, _, _, _, _ := createTestTaskManager(t)
	task, err := tm.Create(CreateTaskOpts{Title: "t", TaskType: models.TaskTypeFeat})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := tm.Start(task.ID); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	got, _ := backlogStore.GetTask(task.ID)
	if got.Status != models.TaskStatusInProgress {
		t.Errorf("after Start, status = %s, want in_progress", got.Status)
	}

	// Idempotent: a second Start is a no-op (no error, no change).
	if err := tm.Start(task.ID); err != nil {
		t.Errorf("second Start should be a no-op, got %v", err)
	}
	got2, _ := backlogStore.GetTask(task.ID)
	if got2.Status != models.TaskStatusInProgress {
		t.Errorf("second Start changed status to %s", got2.Status)
	}
}

// TestTaskManager_Create_IssueLinkedBranch verifies an issue-linked ticket gets
// an ADR-0002-aware branch encoding the issue number (#210).
func TestTaskManager_Create_IssueLinkedBranch(t *testing.T) {
	tm, _, _, worktreeCreator, _, _ := createTestTaskManager(t)
	task, err := tm.Create(CreateTaskOpts{
		Title:       "My Slug",
		TaskType:    models.TaskTypeFeat,
		Repo:        "github.com/test/repo",
		RemoteIssue: 210,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if task.Branch != "feat/210-my-slug" {
		t.Errorf("branch = %q, want feat/210-my-slug", task.Branch)
	}
	if task.RemoteIssue != 210 {
		t.Errorf("task.RemoteIssue = %d, want 210", task.RemoteIssue)
	}
	if got := worktreeCreator.createdBranch[task.ID]; got != "feat/210-my-slug" {
		t.Errorf("CreateWorktree got branch %q, want feat/210-my-slug", got)
	}
}

func TestTaskManager_Create_WithDefaults(t *testing.T) {
	tm, _, _, _, _, _ := createTestTaskManager(t)

	// Create with minimal options
	opts := CreateTaskOpts{
		Title: "Minimal Task",
	}

	task, err := tm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify defaults were applied
	if task.Priority != models.PriorityP2 {
		t.Errorf("Expected default priority P2, got %s", task.Priority)
	}
	if task.Type != models.TaskTypeFeat {
		t.Errorf("Expected default type feat, got %s", task.Type)
	}
	if task.Status != models.TaskStatusBacklog {
		t.Errorf("Expected default status backlog, got %s", task.Status)
	}
}

func TestTaskManager_Create_RollbackOnWorktreeFailure(t *testing.T) {
	tm, backlogStore, _, worktreeCreator, _, tempDir := createTestTaskManager(t)

	// Set worktree creator to fail
	worktreeCreator.shouldFail = true

	opts := CreateTaskOpts{
		Title:    "Test Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/test/repo",
	}

	_, err := tm.Create(opts)
	if err == nil {
		t.Fatalf("Expected error on worktree creation failure")
	}

	// Verify task was not added to backlog
	_, err = backlogStore.GetTask("TASK-00001")
	if err == nil {
		t.Errorf("Task should not exist in backlog after rollback")
	}

	// Verify the nested task directory was cleaned up (rollback path).
	taskDir := filepath.Join(tempDir, "tickets", "github.com/test/repo", "TASK-00001-test-task")
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Errorf("Task directory should have been cleaned up: %s", taskDir)
	}
}

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestSessionStoreManager_GenerateID(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	id1, err := store.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != "S-00001" {
		t.Errorf("expected S-00001, got %s", id1)
	}

	id2, err := store.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "S-00002" {
		t.Errorf("expected S-00002, got %s", id2)
	}
}

func TestSessionStoreManager_GenerateIDPersistence(t *testing.T) {
	dir := t.TempDir()
	store1 := NewSessionStoreManager(dir)

	id1, err := store1.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != "S-00001" {
		t.Errorf("expected S-00001, got %s", id1)
	}

	// New store instance should continue from same counter.
	store2 := NewSessionStoreManager(dir)
	id2, err := store2.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "S-00002" {
		t.Errorf("expected S-00002, got %s", id2)
	}
}

func testSession(id string) models.CapturedSession {
	return models.CapturedSession{
		ID:          id,
		SessionID:   "claude-uuid-" + id,
		TaskID:      "TASK-00001",
		ProjectPath: "/home/user/project",
		GitBranch:   "feat/test",
		StartedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		EndedAt:     time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),
		Duration:    "1h0m",
		TurnCount:   5,
		Summary:     "Test session summary",
	}
}

func testTurns() []models.SessionTurn {
	return []models.SessionTurn{
		{Index: 0, Role: "user", Content: "Hello", Digest: "Hello"},
		{Index: 1, Role: "assistant", Content: "Hi there", Digest: "Hi there", ToolsUsed: []string{"Read"}},
	}
}

func TestSessionStoreManager_AddAndGetSession(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	session := testSession("S-00001")
	turns := testTurns()

	id, err := store.AddSession(session, turns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "S-00001" {
		t.Errorf("expected S-00001, got %s", id)
	}

	got, err := store.GetSession("S-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "Test session summary" {
		t.Errorf("unexpected summary: %s", got.Summary)
	}
	if got.TaskID != "TASK-00001" {
		t.Errorf("unexpected task ID: %s", got.TaskID)
	}
}

func TestSessionStoreManager_AddSessionEmptyID(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	session := testSession("")
	session.ID = ""
	if _, err := store.AddSession(session, nil); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestSessionStoreManager_AddSessionDuplicate(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	session := testSession("S-00001")
	if _, err := store.AddSession(session, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := store.AddSession(session, nil); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestSessionStoreManager_GetSessionNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)

	got, err := store.GetSession("S-99999")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if got != nil {
		t.Errorf("expected nil session, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestSessionStoreManager_GetSessionTurns(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	session := testSession("S-00001")
	turns := testTurns()
	store.AddSession(session, turns)

	got, err := store.GetSessionTurns("S-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("expected first turn role 'user', got %s", got[0].Role)
	}
	if got[1].Role != "assistant" {
		t.Errorf("expected second turn role 'assistant', got %s", got[1].Role)
	}
	if len(got[1].ToolsUsed) != 1 || got[1].ToolsUsed[0] != "Read" {
		t.Errorf("expected tools_used [Read], got %v", got[1].ToolsUsed)
	}
}

func TestSessionStoreManager_GetSessionTurnsNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	_, err := store.GetSessionTurns("S-99999")
	if err == nil {
		t.Fatal("expected error for nonexistent session turns")
	}
}

func TestSessionStoreManager_ListSessionsNoFilter(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)

	s2 := testSession("S-00002")
	s2.TaskID = "TASK-00002"
	store.AddSession(s2, nil)

	sessions, err := store.ListSessions(models.SessionFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionStoreManager_ListSessionsFilterByTask(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)

	s2 := testSession("S-00002")
	s2.TaskID = "TASK-00002"
	store.AddSession(s2, nil)

	sessions, err := store.ListSessions(models.SessionFilter{TaskID: "TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "S-00001" {
		t.Errorf("expected S-00001, got %s", sessions[0].ID)
	}
}

func TestSessionStoreManager_ListSessionsFilterByProject(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)

	s2 := testSession("S-00002")
	s2.ProjectPath = "/other/project"
	store.AddSession(s2, nil)

	sessions, err := store.ListSessions(models.SessionFilter{ProjectPath: "/home/user/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestSessionStoreManager_ListSessionsFilterBySince(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	s1 := testSession("S-00001")
	s1.EndedAt = time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	store.AddSession(s1, nil)

	s2 := testSession("S-00002")
	s2.EndedAt = time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)
	store.AddSession(s2, nil)

	since := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	sessions, err := store.ListSessions(models.SessionFilter{Since: &since})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session since 2025-01-15, got %d", len(sessions))
	}
	if sessions[0].ID != "S-00002" {
		t.Errorf("expected S-00002, got %s", sessions[0].ID)
	}
}

func TestSessionStoreManager_ListSessionsFilterByMinTurns(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	s1 := testSession("S-00001")
	s1.TurnCount = 2
	store.AddSession(s1, nil)

	s2 := testSession("S-00002")
	s2.TurnCount = 10
	store.AddSession(s2, nil)

	sessions, err := store.ListSessions(models.SessionFilter{MinTurns: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session with >= 5 turns, got %d", len(sessions))
	}
	if sessions[0].ID != "S-00002" {
		t.Errorf("expected S-00002, got %s", sessions[0].ID)
	}
}

func TestSessionStoreManager_ListSessionsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	sessions, err := store.ListSessions(models.SessionFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions for empty store, got %d", len(sessions))
	}
}

func TestSessionStoreManager_GetLatestSessionForTask(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	s1 := testSession("S-00001")
	s1.TaskID = "TASK-00001"
	s1.EndedAt = time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	store.AddSession(s1, nil)

	s2 := testSession("S-00002")
	s2.TaskID = "TASK-00001"
	s2.EndedAt = time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)
	store.AddSession(s2, nil)

	s3 := testSession("S-00003")
	s3.TaskID = "TASK-00002"
	s3.EndedAt = time.Date(2025, 1, 25, 0, 0, 0, 0, time.UTC)
	store.AddSession(s3, nil)

	latest, err := store.GetLatestSessionForTask("TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil latest session")
	}
	if latest.ID != "S-00002" {
		t.Errorf("expected S-00002, got %s", latest.ID)
	}
}

func TestSessionStoreManager_GetLatestSessionForTaskNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)

	latest, err := store.GetLatestSessionForTask("TASK-99999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest != nil {
		t.Errorf("expected nil for task with no sessions, got %+v", latest)
	}
}

func TestSessionStoreManager_GetRecentSessions(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	s1 := testSession("S-00001")
	s1.EndedAt = time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	store.AddSession(s1, nil)

	s2 := testSession("S-00002")
	s2.EndedAt = time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)
	store.AddSession(s2, nil)

	s3 := testSession("S-00003")
	s3.EndedAt = time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	store.AddSession(s3, nil)

	recent, err := store.GetRecentSessions(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent sessions, got %d", len(recent))
	}
	// Should be sorted newest first.
	if recent[0].ID != "S-00002" {
		t.Errorf("expected newest first (S-00002), got %s", recent[0].ID)
	}
	if recent[1].ID != "S-00003" {
		t.Errorf("expected second newest (S-00003), got %s", recent[1].ID)
	}
}

func TestSessionStoreManager_GetRecentSessionsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	recent, err := store.GetRecentSessions(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recent != nil {
		t.Errorf("expected nil for empty store, got %d", len(recent))
	}
}

func TestSessionStoreManager_GetRecentSessionsNoLimit(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	store.AddSession(testSession("S-00001"), nil)
	store.AddSession(testSession("S-00002"), nil)

	recent, err := store.GetRecentSessions(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected all sessions with limit 0, got %d", len(recent))
	}
}

func TestSessionStoreManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	store := NewSessionStoreManager(dir)
	session := testSession("S-00001")
	turns := testTurns()
	store.AddSession(session, turns)

	if err := store.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify index file exists.
	indexPath := filepath.Join(dir, "sessions", "index.yaml")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("expected index.yaml to exist")
	}

	// Load into a new store.
	store2 := NewSessionStoreManager(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}

	sessions, _ := store2.ListSessions(models.SessionFilter{})
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after load, got %d", len(sessions))
	}
	if sessions[0].Summary != "Test session summary" {
		t.Errorf("unexpected summary after load: %s", sessions[0].Summary)
	}

	// Verify turns survive round-trip.
	loadedTurns, err := store2.GetSessionTurns("S-00001")
	if err != nil {
		t.Fatalf("unexpected error loading turns: %v", err)
	}
	if len(loadedTurns) != 2 {
		t.Errorf("expected 2 turns after load, got %d", len(loadedTurns))
	}
}

func TestSessionStoreManager_LoadEmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	if err := store.Load(); err != nil {
		t.Fatalf("unexpected error loading from empty dir: %v", err)
	}

	sessions, _ := store.ListSessions(models.SessionFilter{})
	if sessions != nil {
		t.Errorf("expected nil sessions, got %d", len(sessions))
	}
}

func TestSessionStoreManager_LoadMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	malformed := []byte("{{invalid yaml [[[")
	if err := os.WriteFile(filepath.Join(sessionsDir, "index.yaml"), malformed, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewSessionStoreManager(dir)
	err := store.Load()
	if err == nil {
		t.Fatal("expected error loading malformed YAML")
	}
	if !strings.Contains(err.Error(), "loading session index") {
		t.Errorf("expected error about loading session index, got: %v", err)
	}
}

func TestSessionStoreManager_WritesSessionFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStoreManager(dir)

	session := testSession("S-00001")
	store.AddSession(session, testTurns())

	// Check per-session directory and files.
	sessionDir := filepath.Join(dir, "sessions", "S-00001")
	for _, file := range []string{"session.yaml", "turns.yaml", "summary.md"} {
		path := filepath.Join(sessionDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", file)
		}
	}

	// Verify summary.md content.
	summaryData, err := os.ReadFile(filepath.Join(sessionDir, "summary.md"))
	if err != nil {
		t.Fatalf("reading summary.md: %v", err)
	}
	if !strings.Contains(string(summaryData), "Session S-00001") {
		t.Errorf("summary.md should contain session ID, got: %s", string(summaryData))
	}
}

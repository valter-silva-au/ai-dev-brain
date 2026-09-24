package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestNewFileSessionStoreManager(t *testing.T) {
	tempDir := t.TempDir()

	fssm := NewFileSessionStoreManager(tempDir)
	if fssm == nil {
		t.Fatal("NewFileSessionStoreManager returned nil")
	}
	if fssm.baseDir != tempDir {
		t.Errorf("Expected baseDir %s, got %s", tempDir, fssm.baseDir)
	}
}

func TestFileSessionStoreManager_GetNextSessionID(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Get first session ID
	id1, err := fssm.GetNextSessionID()
	if err != nil {
		t.Fatalf("GetNextSessionID() failed: %v", err)
	}
	if id1 != "S-00001" {
		t.Errorf("Expected first ID to be S-00001, got %s", id1)
	}

	// Get second session ID
	id2, err := fssm.GetNextSessionID()
	if err != nil {
		t.Fatalf("GetNextSessionID() failed: %v", err)
	}
	if id2 != "S-00002" {
		t.Errorf("Expected second ID to be S-00002, got %s", id2)
	}

	// Get third session ID
	id3, err := fssm.GetNextSessionID()
	if err != nil {
		t.Fatalf("GetNextSessionID() failed: %v", err)
	}
	if id3 != "S-00003" {
		t.Errorf("Expected third ID to be S-00003, got %s", id3)
	}
}

func TestFileSessionStoreManager_SaveSession(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	session := models.NewCapturedSession("S-00001")
	session.TaskID = "TASK-001"
	session.Summary = "Test session summary"

	// Add some turns
	turn1 := models.SessionTurn{
		Index:     1,
		Role:      "user",
		Timestamp: time.Now(),
		Content:   "Hello",
	}
	session.AddTurn(turn1)

	turn2 := models.SessionTurn{
		Index:     2,
		Role:      "assistant",
		Timestamp: time.Now(),
		Content:   "Hi there!",
	}
	session.AddTurn(turn2)

	session.Finalize()

	// Save session
	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Verify session directory was created
	sessionDir := filepath.Join(tempDir, "S-00001")
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Fatal("Session directory was not created")
	}

	// Verify session.yaml exists
	sessionPath := filepath.Join(sessionDir, "session.yaml")
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("session.yaml was not created")
	}

	// Verify turns.yaml exists
	turnsPath := filepath.Join(sessionDir, "turns.yaml")
	if _, err := os.Stat(turnsPath); os.IsNotExist(err) {
		t.Error("turns.yaml was not created")
	}

	// Verify summary.md exists
	summaryPath := filepath.Join(sessionDir, "summary.md")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		t.Error("summary.md was not created")
	}

	// Verify index was updated
	indexPath := filepath.Join(tempDir, "index.yaml")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.yaml was not created")
	}
}

func TestFileSessionStoreManager_SaveSession_NilCheck(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	err := fssm.SaveSession(nil)
	if err == nil {
		t.Error("SaveSession() should fail with nil session")
	}
}

func TestFileSessionStoreManager_SaveSession_NoID(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	session := models.NewCapturedSession("")
	err := fssm.SaveSession(session)
	if err == nil {
		t.Error("SaveSession() should fail without session ID")
	}
}

func TestFileSessionStoreManager_GetSession(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create and save a session
	originalSession := models.NewCapturedSession("S-00001")
	originalSession.TaskID = "TASK-001"
	originalSession.Summary = "Test session"
	originalSession.Tags = []string{"test", "example"}
	originalSession.ToolsUsed = []string{"Read", "Write"}

	turn := models.SessionTurn{
		Index:     1,
		Role:      "user",
		Timestamp: time.Now(),
		Content:   "Test turn",
	}
	originalSession.AddTurn(turn)
	originalSession.Finalize()

	err := fssm.SaveSession(originalSession)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Retrieve the session
	retrievedSession, err := fssm.GetSession("S-00001")
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	// Verify fields
	if retrievedSession.ID != originalSession.ID {
		t.Errorf("Expected ID %s, got %s", originalSession.ID, retrievedSession.ID)
	}
	if retrievedSession.TaskID != originalSession.TaskID {
		t.Errorf("Expected TaskID %s, got %s", originalSession.TaskID, retrievedSession.TaskID)
	}
	if retrievedSession.Summary != originalSession.Summary {
		t.Errorf("Expected Summary %s, got %s", originalSession.Summary, retrievedSession.Summary)
	}
	if len(retrievedSession.Tags) != len(originalSession.Tags) {
		t.Errorf("Expected %d tags, got %d", len(originalSession.Tags), len(retrievedSession.Tags))
	}
	if len(retrievedSession.ToolsUsed) != len(originalSession.ToolsUsed) {
		t.Errorf("Expected %d tools, got %d", len(originalSession.ToolsUsed), len(retrievedSession.ToolsUsed))
	}
	if len(retrievedSession.Turns) != len(originalSession.Turns) {
		t.Errorf("Expected %d turns, got %d", len(originalSession.Turns), len(retrievedSession.Turns))
	}
}

func TestFileSessionStoreManager_GetSession_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	_, err := fssm.GetSession("S-99999")
	if err == nil {
		t.Error("GetSession() should fail for non-existent session")
	}
}

func TestFileSessionStoreManager_ListSessions(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create and save multiple sessions
	sessions := []string{"S-00001", "S-00002", "S-00003"}
	for _, sessionID := range sessions {
		session := models.NewCapturedSession(sessionID)
		session.TaskID = "TASK-001"
		session.Finalize()

		err := fssm.SaveSession(session)
		if err != nil {
			t.Fatalf("SaveSession() failed for %s: %v", sessionID, err)
		}
	}

	// List sessions
	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 sessions, found %d", len(entries))
	}

	// Verify session IDs are present
	foundIDs := make(map[string]bool)
	for _, entry := range entries {
		foundIDs[entry.ID] = true
	}

	for _, sessionID := range sessions {
		if !foundIDs[sessionID] {
			t.Errorf("Session %s not found in list", sessionID)
		}
	}
}

func TestFileSessionStoreManager_ListSessions_Empty(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 sessions for empty store, found %d", len(entries))
	}
}

func TestFileSessionStoreManager_FilterSessions_ByTaskID(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create sessions with different task IDs
	session1 := models.NewCapturedSession("S-00001")
	session1.TaskID = "TASK-001"
	session1.Finalize()

	session2 := models.NewCapturedSession("S-00002")
	session2.TaskID = "TASK-002"
	session2.Finalize()

	session3 := models.NewCapturedSession("S-00003")
	session3.TaskID = "TASK-001"
	session3.Finalize()

	fssm.SaveSession(session1)
	fssm.SaveSession(session2)
	fssm.SaveSession(session3)

	// Filter by TASK-001
	filter := &models.SessionFilter{
		TaskID: "TASK-001",
	}

	filtered, err := fssm.FilterSessions(filter)
	if err != nil {
		t.Fatalf("FilterSessions() failed: %v", err)
	}

	if len(filtered) != 2 {
		t.Errorf("Expected 2 sessions for TASK-001, found %d", len(filtered))
	}

	for _, session := range filtered {
		if session.TaskID != "TASK-001" {
			t.Errorf("Expected TaskID TASK-001, got %s", session.TaskID)
		}
	}
}

func TestFileSessionStoreManager_FilterSessions_ByTags(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create sessions with different tags
	session1 := models.NewCapturedSession("S-00001")
	session1.Tags = []string{"debug", "error"}
	session1.Finalize()

	session2 := models.NewCapturedSession("S-00002")
	session2.Tags = []string{"feature", "implementation"}
	session2.Finalize()

	session3 := models.NewCapturedSession("S-00003")
	session3.Tags = []string{"debug", "investigation"}
	session3.Finalize()

	fssm.SaveSession(session1)
	fssm.SaveSession(session2)
	fssm.SaveSession(session3)

	// Filter by "debug" tag
	filter := &models.SessionFilter{
		Tags: []string{"debug"},
	}

	filtered, err := fssm.FilterSessions(filter)
	if err != nil {
		t.Fatalf("FilterSessions() failed: %v", err)
	}

	if len(filtered) != 2 {
		t.Errorf("Expected 2 sessions with 'debug' tag, found %d", len(filtered))
	}
}

func TestFileSessionStoreManager_FilterSessions_ByDuration(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create sessions with different durations
	session1 := models.NewCapturedSession("S-00001")
	session1.StartTime = time.Now()
	session1.EndTime = session1.StartTime.Add(100 * time.Second)
	session1.Duration = 100 // 100 seconds

	session2 := models.NewCapturedSession("S-00002")
	session2.StartTime = time.Now()
	session2.EndTime = session2.StartTime.Add(500 * time.Second)
	session2.Duration = 500 // 500 seconds

	session3 := models.NewCapturedSession("S-00003")
	session3.StartTime = time.Now()
	session3.EndTime = session3.StartTime.Add(1000 * time.Second)
	session3.Duration = 1000 // 1000 seconds

	fssm.SaveSession(session1)
	fssm.SaveSession(session2)
	fssm.SaveSession(session3)

	// Filter by duration (200-800 seconds)
	filter := &models.SessionFilter{
		MinDuration: 200,
		MaxDuration: 800,
	}

	filtered, err := fssm.FilterSessions(filter)
	if err != nil {
		t.Fatalf("FilterSessions() failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 session in duration range, found %d", len(filtered))
	}

	if filtered[0].Duration != 500 {
		t.Errorf("Expected duration 500, got %d", filtered[0].Duration)
	}
}

func TestFileSessionStoreManager_FilterSessions_NilFilter(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create some sessions
	session1 := models.NewCapturedSession("S-00001")
	session1.Finalize()
	session2 := models.NewCapturedSession("S-00002")
	session2.Finalize()

	fssm.SaveSession(session1)
	fssm.SaveSession(session2)

	// Filter with nil filter (should return all)
	filtered, err := fssm.FilterSessions(nil)
	if err != nil {
		t.Fatalf("FilterSessions() with nil filter failed: %v", err)
	}

	if len(filtered) != 2 {
		t.Errorf("Expected 2 sessions with nil filter, found %d", len(filtered))
	}
}

func TestFileSessionStoreManager_DeleteSession(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create and save a session
	session := models.NewCapturedSession("S-00001")
	session.TaskID = "TASK-001"
	session.Finalize()

	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Verify session exists
	_, err = fssm.GetSession("S-00001")
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	// Delete session
	err = fssm.DeleteSession("S-00001")
	if err != nil {
		t.Fatalf("DeleteSession() failed: %v", err)
	}

	// Verify session directory was removed
	sessionDir := filepath.Join(tempDir, "S-00001")
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Session directory was not removed")
	}

	// Verify session is not in index
	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	for _, entry := range entries {
		if entry.ID == "S-00001" {
			t.Error("Deleted session still in index")
		}
	}
}

func TestFileSessionStoreManager_UpdateSession(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create and save initial session
	session := models.NewCapturedSession("S-00001")
	session.TaskID = "TASK-001"
	session.Summary = "Initial summary"
	session.Finalize()

	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Update session
	session.Summary = "Updated summary"
	session.Tags = []string{"updated"}

	err = fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() (update) failed: %v", err)
	}

	// Retrieve and verify
	retrievedSession, err := fssm.GetSession("S-00001")
	if err != nil {
		t.Fatalf("GetSession() failed: %v", err)
	}

	if retrievedSession.Summary != "Updated summary" {
		t.Errorf("Expected updated summary, got %s", retrievedSession.Summary)
	}

	// Verify index still has only one entry
	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 session in index after update, found %d", len(entries))
	}
}

func TestFileSessionStoreManager_SessionWithoutSummary(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create session without summary
	session := models.NewCapturedSession("S-00001")
	session.TaskID = "TASK-001"
	// No summary set
	session.Finalize()

	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Verify summary.md was not created
	summaryPath := filepath.Join(tempDir, "S-00001", "summary.md")
	if _, err := os.Stat(summaryPath); !os.IsNotExist(err) {
		t.Error("summary.md should not be created when summary is empty")
	}
}

func TestFileSessionStoreManager_MultipleSessions(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create multiple sessions
	numSessions := 5
	for i := 1; i <= numSessions; i++ {
		sessionID, err := fssm.GetNextSessionID()
		if err != nil {
			t.Fatalf("GetNextSessionID() failed: %v", err)
		}

		session := models.NewCapturedSession(sessionID)
		session.TaskID = "TASK-001"
		session.Finalize()

		err = fssm.SaveSession(session)
		if err != nil {
			t.Fatalf("SaveSession() failed for %s: %v", sessionID, err)
		}
	}

	// Verify all sessions are in index
	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	if len(entries) != numSessions {
		t.Errorf("Expected %d sessions, found %d", numSessions, len(entries))
	}

	// Verify next session ID is correct
	nextID, err := fssm.GetNextSessionID()
	if err != nil {
		t.Fatalf("GetNextSessionID() failed: %v", err)
	}

	expectedNextID := "S-00006"
	if nextID != expectedNextID {
		t.Errorf("Expected next ID %s, got %s", expectedNextID, nextID)
	}
}

func TestFileSessionStoreManager_DirectoryPermissions(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	session := models.NewCapturedSession("S-00001")
	session.Finalize()

	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Verify session directory is present + owner-accessible.
	sessionDir := filepath.Join(tempDir, "S-00001")
	assertOwnerAccessibleDir(t, sessionDir)
}

func TestFileSessionStoreManager_FilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	session := models.NewCapturedSession("S-00001")
	session.Summary = "Test"
	session.Finalize()

	err := fssm.SaveSession(session)
	if err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	// Verify each session file is present + owner-read/writable.
	for _, filename := range []string{"session.yaml", "turns.yaml", "summary.md"} {
		assertOwnerReadWritableFile(t, filepath.Join(tempDir, "S-00001", filename))
	}

	// Verify index.yaml is present + owner-read/writable.
	assertOwnerReadWritableFile(t, filepath.Join(tempDir, "index.yaml"))
}

func TestFileSessionStoreManager_SessionIndexSorting(t *testing.T) {
	tempDir := t.TempDir()
	fssm := NewFileSessionStoreManager(tempDir)

	// Create sessions with different start times
	times := []time.Time{
		time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC),
	}

	for i, startTime := range times {
		session := models.NewCapturedSession("S-0000" + string(rune('1'+i)))
		session.StartTime = startTime
		session.Finalize()

		err := fssm.SaveSession(session)
		if err != nil {
			t.Fatalf("SaveSession() failed: %v", err)
		}
	}

	// List sessions
	entries, err := fssm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() failed: %v", err)
	}

	// Verify sorting (newest first)
	if entries[0].StartTime < entries[1].StartTime {
		t.Error("Sessions not sorted correctly (newest first)")
	}
	if entries[1].StartTime < entries[2].StartTime {
		t.Error("Sessions not sorted correctly (newest first)")
	}
}

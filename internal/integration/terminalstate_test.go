package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewTerminalStateWriter(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "test_state.json")

	writer := NewTerminalStateWriter(stateFile)
	if writer == nil {
		t.Fatal("NewTerminalStateWriter() returned nil")
	}

	// Test with empty path (should use default)
	writer = NewTerminalStateWriter("")
	if writer == nil {
		t.Fatal("NewTerminalStateWriter() with empty path returned nil")
	}
}

func TestWriteState(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	state := TerminalState{
		WorktreePath: "/work/task-001",
		TaskID:       "TASK-001",
		Status:       "active",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	err := writer.WriteState(state)
	if err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("State file was not created")
	}

	// Verify content
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	var states map[string]TerminalState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("Failed to parse state file: %v", err)
	}

	savedState, exists := states["/work/task-001"]
	if !exists {
		t.Fatal("State was not saved")
	}

	if savedState.TaskID != "TASK-001" {
		t.Errorf("Expected TaskID 'TASK-001', got: %s", savedState.TaskID)
	}
	if savedState.Status != "active" {
		t.Errorf("Expected Status 'active', got: %s", savedState.Status)
	}
}

func TestReadState(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Write a state
	originalState := TerminalState{
		WorktreePath: "/work/task-002",
		TaskID:       "TASK-002",
		Status:       "pending",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	if err := writer.WriteState(originalState); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	// Read it back
	readState, err := writer.ReadState("/work/task-002")
	if err != nil {
		t.Fatalf("ReadState() failed: %v", err)
	}

	if readState.TaskID != originalState.TaskID {
		t.Errorf("Expected TaskID %s, got: %s", originalState.TaskID, readState.TaskID)
	}
	if readState.Status != originalState.Status {
		t.Errorf("Expected Status %s, got: %s", originalState.Status, readState.Status)
	}
}

func TestReadStateNotFound(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	_, err := writer.ReadState("/nonexistent/path")
	if err == nil {
		t.Error("ReadState() should return error for non-existent worktree")
	}
	if !strings.Contains(err.Error(), "no state found") {
		t.Errorf("Expected error to contain 'no state found', got: %v", err)
	}
}

func TestDeleteState(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Write a state
	state := TerminalState{
		WorktreePath: "/work/task-003",
		TaskID:       "TASK-003",
		Status:       "active",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	if err := writer.WriteState(state); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	// Verify it exists
	_, err := writer.ReadState("/work/task-003")
	if err != nil {
		t.Fatalf("State should exist before deletion: %v", err)
	}

	// Delete it
	if err := writer.DeleteState("/work/task-003"); err != nil {
		t.Fatalf("DeleteState() failed: %v", err)
	}

	// Verify it's gone
	_, err = writer.ReadState("/work/task-003")
	if err == nil {
		t.Error("State should not exist after deletion")
	}
}

func TestListStates(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Write multiple states
	states := []TerminalState{
		{
			WorktreePath: "/work/task-004",
			TaskID:       "TASK-004",
			Status:       "active",
			LastUpdated:  time.Now().Format(time.RFC3339),
		},
		{
			WorktreePath: "/work/task-005",
			TaskID:       "TASK-005",
			Status:       "pending",
			LastUpdated:  time.Now().Format(time.RFC3339),
		},
		{
			WorktreePath: "/work/task-006",
			TaskID:       "TASK-006",
			Status:       "blocked",
			LastUpdated:  time.Now().Format(time.RFC3339),
		},
	}

	for _, state := range states {
		if err := writer.WriteState(state); err != nil {
			t.Fatalf("WriteState() failed: %v", err)
		}
	}

	// List all states
	allStates, err := writer.ListStates()
	if err != nil {
		t.Fatalf("ListStates() failed: %v", err)
	}

	if len(allStates) != 3 {
		t.Errorf("Expected 3 states, got: %d", len(allStates))
	}

	// Verify all states are present
	taskIDs := make(map[string]bool)
	for _, state := range allStates {
		taskIDs[state.TaskID] = true
	}

	for _, originalState := range states {
		if !taskIDs[originalState.TaskID] {
			t.Errorf("TaskID %s not found in listed states", originalState.TaskID)
		}
	}
}

func TestListStatesEmpty(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	states, err := writer.ListStates()
	if err != nil {
		t.Fatalf("ListStates() on empty state failed: %v", err)
	}

	if len(states) != 0 {
		t.Errorf("Expected 0 states, got: %d", len(states))
	}
}

func TestCleanStaleStates(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Create a real worktree directory
	realWorktree := filepath.Join(tempDir, "real-worktree")
	if err := os.MkdirAll(realWorktree, 0o755); err != nil {
		t.Fatalf("Failed to create real worktree: %v", err)
	}

	// Write states - one with real path, one with fake path
	states := []TerminalState{
		{
			WorktreePath: realWorktree,
			TaskID:       "TASK-007",
			Status:       "active",
			LastUpdated:  time.Now().Format(time.RFC3339),
		},
		{
			WorktreePath: "/nonexistent/worktree",
			TaskID:       "TASK-008",
			Status:       "stale",
			LastUpdated:  time.Now().Format(time.RFC3339),
		},
	}

	for _, state := range states {
		if err := writer.WriteState(state); err != nil {
			t.Fatalf("WriteState() failed: %v", err)
		}
	}

	// Clean stale states
	if err := writer.CleanStaleStates(); err != nil {
		t.Fatalf("CleanStaleStates() failed: %v", err)
	}

	// List states - should only have the real worktree
	remainingStates, err := writer.ListStates()
	if err != nil {
		t.Fatalf("ListStates() failed: %v", err)
	}

	if len(remainingStates) != 1 {
		t.Errorf("Expected 1 remaining state, got: %d", len(remainingStates))
	}

	if len(remainingStates) > 0 && remainingStates[0].TaskID != "TASK-007" {
		t.Errorf("Expected remaining state to be TASK-007, got: %s", remainingStates[0].TaskID)
	}
}

func TestUpdateExistingState(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Write initial state
	state := TerminalState{
		WorktreePath: "/work/task-009",
		TaskID:       "TASK-009",
		Status:       "pending",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	if err := writer.WriteState(state); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	// Update the state
	state.Status = "active"
	state.LastUpdated = time.Now().Add(time.Hour).Format(time.RFC3339)

	if err := writer.WriteState(state); err != nil {
		t.Fatalf("WriteState() update failed: %v", err)
	}

	// Read it back
	readState, err := writer.ReadState("/work/task-009")
	if err != nil {
		t.Fatalf("ReadState() failed: %v", err)
	}

	if readState.Status != "active" {
		t.Errorf("Expected Status 'active', got: %s", readState.Status)
	}

	// Verify only one state exists
	allStates, err := writer.ListStates()
	if err != nil {
		t.Fatalf("ListStates() failed: %v", err)
	}

	if len(allStates) != 1 {
		t.Errorf("Expected 1 state, got: %d", len(allStates))
	}
}

func TestCorruptJSONHandling(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")

	// Write corrupt JSON
	if err := os.WriteFile(stateFile, []byte("{ corrupt json }"), 0o644); err != nil {
		t.Fatalf("Failed to write corrupt JSON: %v", err)
	}

	writer := NewTerminalStateWriter(stateFile)

	// Should handle corrupt JSON by resetting
	state := TerminalState{
		WorktreePath: "/work/task-010",
		TaskID:       "TASK-010",
		Status:       "active",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	err := writer.WriteState(state)
	if err != nil {
		t.Fatalf("WriteState() should handle corrupt JSON: %v", err)
	}

	// Should be able to read the new state
	readState, err := writer.ReadState("/work/task-010")
	if err != nil {
		t.Fatalf("ReadState() after corrupt JSON failed: %v", err)
	}

	if readState.TaskID != "TASK-010" {
		t.Errorf("Expected TaskID 'TASK-010', got: %s", readState.TaskID)
	}
}

func TestThreadSafety(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Write states concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			state := TerminalState{
				WorktreePath: filepath.Join("/work", "task", string(rune('a'+index))),
				TaskID:       "TASK-" + string(rune('A'+index)),
				Status:       "active",
				LastUpdated:  time.Now().Format(time.RFC3339),
			}

			if err := writer.WriteState(state); err != nil {
				t.Errorf("Concurrent WriteState() failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all states were written
	states, err := writer.ListStates()
	if err != nil {
		t.Fatalf("ListStates() after concurrent writes failed: %v", err)
	}

	if len(states) != numGoroutines {
		t.Errorf("Expected %d states, got: %d", numGoroutines, len(states))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Write initial state
	initialState := TerminalState{
		WorktreePath: "/work/concurrent",
		TaskID:       "TASK-CONCURRENT",
		Status:       "pending",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}
	if err := writer.WriteState(initialState); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	var wg sync.WaitGroup
	numReaders := 5
	numWriters := 5

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := writer.ReadState("/work/concurrent")
			if err != nil {
				t.Errorf("Concurrent ReadState() failed: %v", err)
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			state := TerminalState{
				WorktreePath: "/work/concurrent",
				TaskID:       "TASK-CONCURRENT",
				Status:       "active",
				LastUpdated:  time.Now().Format(time.RFC3339),
			}
			if err := writer.WriteState(state); err != nil {
				t.Errorf("Concurrent WriteState() failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

func TestStateFileInSubdirectory(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "subdir", "nested", "state.json")
	writer := NewTerminalStateWriter(stateFile)

	state := TerminalState{
		WorktreePath: "/work/task-011",
		TaskID:       "TASK-011",
		Status:       "active",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	// Should create parent directories
	err := writer.WriteState(state)
	if err != nil {
		t.Fatalf("WriteState() with nested path failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("State file was not created in nested directory")
	}
}

func TestJSONFormatting(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	state := TerminalState{
		WorktreePath: "/work/task-012",
		TaskID:       "TASK-012",
		Status:       "active",
		LastUpdated:  time.Now().Format(time.RFC3339),
	}

	if err := writer.WriteState(state); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	// Read raw file content
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	// Should be indented JSON
	content := string(data)
	if !strings.Contains(content, "\n") {
		t.Error("JSON should be formatted with newlines")
	}
	if !strings.Contains(content, "  ") {
		t.Error("JSON should be indented")
	}

	// Should be valid JSON
	var states map[string]TerminalState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Errorf("State file is not valid JSON: %v", err)
	}
}

func TestDeleteNonExistentState(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	writer := NewTerminalStateWriter(stateFile)

	// Delete non-existent state should not error
	err := writer.DeleteState("/nonexistent/path")
	if err != nil {
		t.Errorf("DeleteState() for non-existent state failed: %v", err)
	}
}

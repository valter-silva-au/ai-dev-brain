package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewApp_TaskCounterWritesUnderADB is the end-to-end guarantee for #189: a
// fresh workspace's task-ID counter is created under .adb/, not at the root.
// Generating an ID exercises the writer wired in NewApp (via App.StatePath).
func TestNewApp_TaskCounterWritesUnderADB(t *testing.T) {
	base := t.TempDir()
	app, err := NewApp(base)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	if _, err := app.TaskIDGenerator.GenerateTaskID(); err != nil {
		t.Fatalf("GenerateTaskID: %v", err)
	}

	// The counter must land under .adb/, and the legacy root path must not exist.
	if _, err := os.Stat(filepath.Join(base, ".adb", "task_counter")); err != nil {
		t.Errorf(".adb/task_counter was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, ".task_counter")); !os.IsNotExist(err) {
		t.Errorf("legacy .task_counter present at root (err=%v); expected absent", err)
	}
}

// TestApp_InternalStatePaths_UnderADB pins that the internal (non-external-
// contract) state files this ticket routes resolve under .adb/ with their
// dot-free basenames. events.jsonl / governance.jsonl are intentionally NOT
// asserted here — they move in #190.
func TestApp_InternalStatePaths_UnderADB(t *testing.T) {
	app := &App{BasePath: filepath.Join("/ws", "root")}
	for _, name := range []string{
		"task_counter",
		"memory.sqlite",
		"scheduler.pid",
		"scheduler.log",
		"scheduler_state.yaml",
		"automation_cursor",
		"session_changes",
		"evidence_reads",
		"context_state.yaml",
	} {
		want := filepath.Join(app.BasePath, ".adb", name)
		if got := app.StatePath(name); got != want {
			t.Errorf("StatePath(%q) = %q, want %q", name, got, want)
		}
	}
}

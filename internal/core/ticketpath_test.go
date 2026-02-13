package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTicketDir_ActivePath(t *testing.T) {
	dir := t.TempDir()

	taskID := "TASK-00001"
	activeDir := filepath.Join(dir, "tickets", taskID)
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveTicketDir(dir, taskID)
	if got != activeDir {
		t.Errorf("resolveTicketDir() = %s, want %s", got, activeDir)
	}
}

func TestResolveTicketDir_ArchivedPath(t *testing.T) {
	dir := t.TempDir()

	taskID := "TASK-00002"
	archivedDir := filepath.Join(dir, "tickets", ArchivedDir, taskID)
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveTicketDir(dir, taskID)
	if got != archivedDir {
		t.Errorf("resolveTicketDir() = %s, want %s", got, archivedDir)
	}
}

func TestResolveTicketDir_NeitherExists(t *testing.T) {
	dir := t.TempDir()

	taskID := "TASK-00003"
	expected := filepath.Join(dir, "tickets", taskID)

	got := resolveTicketDir(dir, taskID)
	if got != expected {
		t.Errorf("resolveTicketDir() = %s, want %s (default active path)", got, expected)
	}
}

func TestResolveTicketDir_ActivePreferred(t *testing.T) {
	dir := t.TempDir()

	taskID := "TASK-00004"
	activeDir := filepath.Join(dir, "tickets", taskID)
	archivedDir := filepath.Join(dir, "tickets", ArchivedDir, taskID)
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Active path should be preferred when both exist.
	got := resolveTicketDir(dir, taskID)
	if got != activeDir {
		t.Errorf("resolveTicketDir() = %s, want %s (active preferred)", got, activeDir)
	}
}

func TestActiveTicketDir(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00005"
	expected := filepath.Join(dir, "tickets", taskID)
	got := activeTicketDir(dir, taskID)
	if got != expected {
		t.Errorf("activeTicketDir() = %s, want %s", got, expected)
	}
}

func TestArchivedTicketDir(t *testing.T) {
	dir := t.TempDir()
	taskID := "TASK-00006"
	expected := filepath.Join(dir, "tickets", ArchivedDir, taskID)
	got := archivedTicketDir(dir, taskID)
	if got != expected {
		t.Errorf("archivedTicketDir() = %s, want %s", got, expected)
	}
}

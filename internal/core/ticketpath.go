package core

import (
	"os"
	"path/filepath"
)

// ArchivedDir is the subdirectory under tickets/ where archived task folders
// are moved to keep the VS Code file explorer clean.
const ArchivedDir = "_archived"

// ResolveTicketDir returns the directory path for a task's ticket folder.
// It checks the active path (tickets/{taskID}) first, then the archived
// path (tickets/_archived/{taskID}). If neither exists, it returns the
// active path as the default (for new tasks).
//
// This function handles nested path-based task IDs (e.g. "github.com/org/repo/feature")
// as well as legacy flat IDs (e.g. "TASK-00042").
func ResolveTicketDir(basePath, taskID string) string {
	return resolveTicketDir(basePath, taskID)
}

// resolveTicketDir is the internal implementation of ResolveTicketDir.
func resolveTicketDir(basePath, taskID string) string {
	active := filepath.Join(basePath, "tickets", taskID)
	if _, err := os.Stat(active); err == nil {
		return active
	}
	archived := filepath.Join(basePath, "tickets", ArchivedDir, taskID)
	if _, err := os.Stat(archived); err == nil {
		return archived
	}
	return active
}

// activeTicketDir returns the path where active (non-archived) tickets live.
// Use this when you know the ticket should be in the active location (e.g.,
// after unarchiving or for new tasks).
func activeTicketDir(basePath, taskID string) string {
	return filepath.Join(basePath, "tickets", taskID)
}

// archivedTicketDir returns the path where archived tickets are stored.
func archivedTicketDir(basePath, taskID string) string {
	return filepath.Join(basePath, "tickets", ArchivedDir, taskID)
}

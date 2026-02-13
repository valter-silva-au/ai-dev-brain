package storage

import (
	"os"
	"path/filepath"
)

// archivedDir is the subdirectory under tickets/ where archived task folders
// are stored.
const archivedDir = "_archived"

// resolveTicketDir returns the directory path for a task's ticket folder.
// It checks the active path (tickets/{taskID}) first, then the archived
// path (tickets/_archived/{taskID}). If neither exists, it returns the
// active path as the default.
func resolveTicketDir(basePath, taskID string) string {
	active := filepath.Join(basePath, "tickets", taskID)
	if _, err := os.Stat(active); err == nil {
		return active
	}
	archived := filepath.Join(basePath, "tickets", archivedDir, taskID)
	if _, err := os.Stat(archived); err == nil {
		return archived
	}
	return active
}

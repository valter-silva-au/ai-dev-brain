package core

import (
	"github.com/valter-silva-au/ai-dev-brain/internal/ticketpath"
)

// ArchivedDir is the subdirectory under tickets/ where archived task folders
// are moved to keep the VS Code file explorer clean.
const ArchivedDir = ticketpath.ArchivedDir

// ResolveTicketDir returns the directory path for a task's ticket folder.
// It delegates to the shared ticketpath package to avoid code duplication.
func ResolveTicketDir(basePath, taskID string) string {
	return ticketpath.ResolveTicketDir(basePath, taskID)
}

// resolveTicketDir is the internal implementation that delegates to the shared package.
func resolveTicketDir(basePath, taskID string) string {
	return ticketpath.ResolveTicketDir(basePath, taskID)
}

// activeTicketDir returns the path where active (non-archived) tickets live.
func activeTicketDir(basePath, taskID string) string {
	return ticketpath.ActiveTicketDir(basePath, taskID)
}

// archivedTicketDir returns the path where archived tickets are stored.
func archivedTicketDir(basePath, taskID string) string {
	return ticketpath.ArchivedTicketDir(basePath, taskID)
}

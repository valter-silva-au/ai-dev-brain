package storage

import (
	"github.com/valter-silva-au/ai-dev-brain/internal/ticketpath"
)

// resolveTicketDir returns the directory path for a task's ticket folder.
// It delegates to the shared ticketpath package to avoid code duplication.
func resolveTicketDir(basePath, taskID string) string {
	return ticketpath.ResolveTicketDir(basePath, taskID)
}

package core

import (
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// CommunicationStore provides access to task communications.
// This interface is defined locally in core to avoid importing storage.
type CommunicationStore interface {
	GetAllCommunications(taskID string) ([]models.Communication, error)
}

// AIContextProvider provides AI context data for a task.
// This interface is defined locally in core to avoid importing storage.
type AIContextProvider interface {
	GetContextForAI(taskID string) (*AIContext, error)
}

// AIContext represents the context data structure returned by AIContextProvider.
// This mirrors storage.AIContext but is defined here to avoid the import.
type AIContext struct {
	Summary        string
	RecentActivity []string
	Blockers       []string
	OpenQuestions  []string
}

// TaskContextLoader provides basic context loading for tasks.
// This interface is defined locally in core to avoid importing storage.
type TaskContextLoader interface {
	LoadContext(taskID string) (*TaskContext, error)
}

// TaskContext represents the persistent context for a task's AI session.
// This mirrors storage.TaskContext but is defined here to avoid the import.
type TaskContext struct {
	TaskID         string
	Notes          string
	Context        string
	Communications []models.Communication
	LastUpdated    time.Time
}

package core

import "github.com/valter-silva-au/ai-dev-brain/pkg/models"

// SessionCapturer is the subset of the session store that core and CLI
// services need. Defining it here avoids importing the storage package.
type SessionCapturer interface {
	CaptureSession(session models.CapturedSession, turns []models.SessionTurn) (string, error)
	GetSession(sessionID string) (*models.CapturedSession, error)
	ListSessions(filter models.SessionFilter) ([]models.CapturedSession, error)
	GetSessionTurns(sessionID string) ([]models.SessionTurn, error)
	GetLatestSessionForTask(taskID string) (*models.CapturedSession, error)
	GetRecentSessions(limit int) ([]models.CapturedSession, error)
	GenerateID() (string, error)
	Save() error
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// SessionIndex represents the index of all captured sessions
type SessionIndex struct {
	Sessions      []SessionIndexEntry `yaml:"sessions"`
	NextSessionID int                 `yaml:"next_session_id"`
}

// SessionIndexEntry represents a single entry in the session index
type SessionIndexEntry struct {
	ID        string `yaml:"id"`
	TaskID    string `yaml:"task_id,omitempty"`
	StartTime string `yaml:"start_time"`
	EndTime   string `yaml:"end_time,omitempty"`
	Summary   string `yaml:"summary,omitempty"`
	Directory string `yaml:"directory"`
}

// SessionStoreManager defines the interface for managing captured sessions
type SessionStoreManager interface {
	SaveSession(session *models.CapturedSession) error
	GetSession(sessionID string) (*models.CapturedSession, error)
	ListSessions() ([]SessionIndexEntry, error)
	FilterSessions(filter *models.SessionFilter) ([]*models.CapturedSession, error)
	DeleteSession(sessionID string) error
	GetNextSessionID() (string, error)
}

// FileSessionStoreManager implements SessionStoreManager with file-based storage
// It manages workspace-level session storage with a YAML index
type FileSessionStoreManager struct {
	baseDir   string
	indexPath string
	mu        sync.RWMutex // Provides concurrent safety
}

// NewFileSessionStoreManager creates a new file-based session store manager
// baseDir is the root directory for session storage (e.g., "sessions")
func NewFileSessionStoreManager(baseDir string) *FileSessionStoreManager {
	return &FileSessionStoreManager{
		baseDir:   baseDir,
		indexPath: filepath.Join(baseDir, "index.yaml"),
	}
}

// ensureBaseDir creates the base directory if it doesn't exist
func (fssm *FileSessionStoreManager) ensureBaseDir() error {
	if err := os.MkdirAll(fssm.baseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return nil
}

// loadIndexUnsafe loads the session index without acquiring locks
func (fssm *FileSessionStoreManager) loadIndexUnsafe() (*SessionIndex, error) {
	// Check if index file exists
	if _, err := os.Stat(fssm.indexPath); os.IsNotExist(err) {
		// Return empty index
		return &SessionIndex{
			Sessions:      []SessionIndexEntry{},
			NextSessionID: 1,
		}, nil
	}

	// Read the file
	data, err := os.ReadFile(fssm.indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index file: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		return &SessionIndex{
			Sessions:      []SessionIndexEntry{},
			NextSessionID: 1,
		}, nil
	}

	// Parse YAML
	var index SessionIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index YAML: %w", err)
	}

	// Ensure Sessions slice is not nil
	if index.Sessions == nil {
		index.Sessions = []SessionIndexEntry{}
	}

	// Ensure NextSessionID is at least 1
	if index.NextSessionID < 1 {
		index.NextSessionID = 1
	}

	return &index, nil
}

// saveIndexUnsafe saves the session index without acquiring locks
func (fssm *FileSessionStoreManager) saveIndexUnsafe(index *SessionIndex) error {
	// Ensure directory exists
	if err := fssm.ensureBaseDir(); err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(index)
	if err != nil {
		return fmt.Errorf("failed to marshal index to YAML: %w", err)
	}

	// Write to file
	if err := os.WriteFile(fssm.indexPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// getSessionDir returns the directory path for a session
func (fssm *FileSessionStoreManager) getSessionDir(sessionID string) string {
	return filepath.Join(fssm.baseDir, sessionID)
}

// GetNextSessionID generates the next session ID
func (fssm *FileSessionStoreManager) GetNextSessionID() (string, error) {
	fssm.mu.Lock()
	defer fssm.mu.Unlock()

	index, err := fssm.loadIndexUnsafe()
	if err != nil {
		return "", err
	}

	sessionID := fmt.Sprintf("S-%05d", index.NextSessionID)
	index.NextSessionID++

	if err := fssm.saveIndexUnsafe(index); err != nil {
		return "", err
	}

	return sessionID, nil
}

// SaveSession saves a captured session with its metadata, turns, and summary
func (fssm *FileSessionStoreManager) SaveSession(session *models.CapturedSession) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.ID == "" {
		return fmt.Errorf("session must have an ID")
	}

	fssm.mu.Lock()
	defer fssm.mu.Unlock()

	// Create session directory
	sessionDir := fssm.getSessionDir(session.ID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Save session metadata (session.yaml)
	sessionPath := filepath.Join(sessionDir, "session.yaml")
	sessionData, err := yaml.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session metadata: %w", err)
	}
	if err := os.WriteFile(sessionPath, sessionData, 0o644); err != nil {
		return fmt.Errorf("failed to write session.yaml: %w", err)
	}

	// Save turns (turns.yaml)
	turnsPath := filepath.Join(sessionDir, "turns.yaml")
	turnsData, err := yaml.Marshal(session.Turns)
	if err != nil {
		return fmt.Errorf("failed to marshal turns: %w", err)
	}
	if err := os.WriteFile(turnsPath, turnsData, 0o644); err != nil {
		return fmt.Errorf("failed to write turns.yaml: %w", err)
	}

	// Save summary (summary.md)
	if session.Summary != "" {
		summaryPath := filepath.Join(sessionDir, "summary.md")
		summaryContent := fmt.Sprintf("# Session %s Summary\n\n%s\n", session.ID, session.Summary)
		if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
			return fmt.Errorf("failed to write summary.md: %w", err)
		}
	}

	// Update index
	index, err := fssm.loadIndexUnsafe()
	if err != nil {
		return err
	}

	// Check if session already exists in index
	found := false
	for i, entry := range index.Sessions {
		if entry.ID == session.ID {
			// Update existing entry
			index.Sessions[i] = SessionIndexEntry{
				ID:        session.ID,
				TaskID:    session.TaskID,
				StartTime: session.StartTime.Format("2006-01-02T15:04:05Z"),
				EndTime:   session.EndTime.Format("2006-01-02T15:04:05Z"),
				Summary:   session.Summary,
				Directory: session.ID,
			}
			found = true
			break
		}
	}

	// Add new entry if not found
	if !found {
		index.Sessions = append(index.Sessions, SessionIndexEntry{
			ID:        session.ID,
			TaskID:    session.TaskID,
			StartTime: session.StartTime.Format("2006-01-02T15:04:05Z"),
			EndTime:   session.EndTime.Format("2006-01-02T15:04:05Z"),
			Summary:   session.Summary,
			Directory: session.ID,
		})
	}

	// Sort sessions by start time (newest first)
	sort.Slice(index.Sessions, func(i, j int) bool {
		return index.Sessions[i].StartTime > index.Sessions[j].StartTime
	})

	// Save index
	if err := fssm.saveIndexUnsafe(index); err != nil {
		return err
	}

	return nil
}

// GetSession retrieves a session by ID
func (fssm *FileSessionStoreManager) GetSession(sessionID string) (*models.CapturedSession, error) {
	fssm.mu.RLock()
	defer fssm.mu.RUnlock()

	sessionDir := fssm.getSessionDir(sessionID)

	// Check if session directory exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Read session.yaml
	sessionPath := filepath.Join(sessionDir, "session.yaml")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session.yaml: %w", err)
	}

	// Parse session metadata
	var session models.CapturedSession
	if err := yaml.Unmarshal(sessionData, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session metadata: %w", err)
	}

	return &session, nil
}

// ListSessions returns all session index entries
func (fssm *FileSessionStoreManager) ListSessions() ([]SessionIndexEntry, error) {
	fssm.mu.RLock()
	defer fssm.mu.RUnlock()

	index, err := fssm.loadIndexUnsafe()
	if err != nil {
		return nil, err
	}

	return index.Sessions, nil
}

// FilterSessions returns sessions matching the filter criteria
func (fssm *FileSessionStoreManager) FilterSessions(filter *models.SessionFilter) ([]*models.CapturedSession, error) {
	if filter == nil {
		// Return all sessions if no filter provided
		entries, err := fssm.ListSessions()
		if err != nil {
			return nil, err
		}

		sessions := make([]*models.CapturedSession, 0, len(entries))
		for _, entry := range entries {
			session, err := fssm.GetSession(entry.ID)
			if err != nil {
				continue
			}
			sessions = append(sessions, session)
		}
		return sessions, nil
	}

	entries, err := fssm.ListSessions()
	if err != nil {
		return nil, err
	}

	sessions := make([]*models.CapturedSession, 0)
	for _, entry := range entries {
		session, err := fssm.GetSession(entry.ID)
		if err != nil {
			continue
		}

		// Apply filter
		if filter.Matches(session) {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// DeleteSession removes a session and its files
func (fssm *FileSessionStoreManager) DeleteSession(sessionID string) error {
	fssm.mu.Lock()
	defer fssm.mu.Unlock()

	// Remove session directory
	sessionDir := fssm.getSessionDir(sessionID)
	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("failed to remove session directory: %w", err)
	}

	// Update index
	index, err := fssm.loadIndexUnsafe()
	if err != nil {
		return err
	}

	// Remove session from index
	for i, entry := range index.Sessions {
		if entry.ID == sessionID {
			index.Sessions = append(index.Sessions[:i], index.Sessions[i+1:]...)
			break
		}
	}

	// Save index
	if err := fssm.saveIndexUnsafe(index); err != nil {
		return err
	}

	return nil
}

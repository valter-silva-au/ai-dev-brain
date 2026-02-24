package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// SessionStoreManager defines the interface for managing the workspace-level
// session store under sessions/.
type SessionStoreManager interface {
	AddSession(session models.CapturedSession, turns []models.SessionTurn) (string, error)
	GetSession(id string) (*models.CapturedSession, error)
	ListSessions(filter models.SessionFilter) ([]models.CapturedSession, error)
	GetSessionTurns(sessionID string) ([]models.SessionTurn, error)
	GetLatestSessionForTask(taskID string) (*models.CapturedSession, error)
	GetRecentSessions(limit int) ([]models.CapturedSession, error)
	GenerateID() (string, error)
	Load() error
	Save() error
}

type fileSessionStore struct {
	basePath string
	index    models.SessionIndex
}

// NewSessionStoreManager creates a SessionStoreManager backed by YAML files
// under sessions/ in the given base directory.
func NewSessionStoreManager(basePath string) SessionStoreManager {
	return &fileSessionStore{
		basePath: basePath,
		index: models.SessionIndex{
			Version:  "1.0",
			Sessions: nil,
		},
	}
}

func (s *fileSessionStore) sessionsDir() string {
	return filepath.Join(s.basePath, "sessions")
}

func (s *fileSessionStore) indexPath() string {
	return filepath.Join(s.sessionsDir(), "index.yaml")
}

func (s *fileSessionStore) counterPath() string {
	return filepath.Join(s.sessionsDir(), ".session_counter")
}

func (s *fileSessionStore) sessionDir(id string) string {
	return filepath.Join(s.sessionsDir(), id)
}

// GenerateID reads and increments the session counter file, returning the
// next sequential ID in S-XXXXX format.
func (s *fileSessionStore) GenerateID() (string, error) {
	counterFile := s.counterPath()

	// Ensure sessions directory exists before writing counter.
	if err := os.MkdirAll(s.sessionsDir(), 0o755); err != nil {
		return "", fmt.Errorf("generating session ID: creating directory: %w", err)
	}

	// Acquire exclusive lock on counter file.
	unlock, err := s.lockCounter()
	if err != nil {
		return "", fmt.Errorf("generating session ID: acquiring lock: %w", err)
	}
	defer unlock()

	counter := 0
	data, err := os.ReadFile(counterFile)
	if err == nil {
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			counter, err = strconv.Atoi(trimmed)
			if err != nil {
				return "", fmt.Errorf("generating session ID: parsing counter: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("generating session ID: reading counter: %w", err)
	}

	counter++
	id := fmt.Sprintf("S-%05d", counter)

	if err := os.WriteFile(counterFile, []byte(strconv.Itoa(counter)), 0o600); err != nil {
		return "", fmt.Errorf("generating session ID: writing counter: %w", err)
	}
	return id, nil
}

// lockCounter acquires an exclusive lock on the session counter file.
func (s *fileSessionStore) lockCounter() (unlock func() error, err error) {
	f, err := os.OpenFile(s.counterPath(), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening counter lock file: %w", err)
	}

	// syscall.Flock is Unix-specific. On Windows, this will compile but may not work.
	// For production Windows support, use a different locking mechanism.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring counter lock: %w", err)
	}

	return func() error {
		defer f.Close()
		return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}, nil
}

// AddSession stores a captured session and its turns. The session must have
// an ID already assigned (via GenerateID).
func (s *fileSessionStore) AddSession(session models.CapturedSession, turns []models.SessionTurn) (string, error) {
	if session.ID == "" {
		return "", fmt.Errorf("adding session: ID must not be empty")
	}

	for _, existing := range s.index.Sessions {
		if existing.ID == session.ID {
			return "", fmt.Errorf("adding session: %s already exists", session.ID)
		}
	}

	// Create per-session directory.
	dir := s.sessionDir(session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("adding session: creating directory: %w", err)
	}

	// Write session metadata.
	if err := s.saveYAML(filepath.Join(dir, "session.yaml"), &session); err != nil {
		return "", fmt.Errorf("adding session: writing metadata: %w", err)
	}

	// Write turns.
	turnsWrapper := struct {
		Turns []models.SessionTurn `yaml:"turns"`
	}{Turns: turns}
	if err := s.saveYAML(filepath.Join(dir, "turns.yaml"), &turnsWrapper); err != nil {
		return "", fmt.Errorf("adding session: writing turns: %w", err)
	}

	// Write summary as markdown.
	summaryContent := fmt.Sprintf("# Session %s\n\n%s\n", session.ID, session.Summary)
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summaryContent), 0o644); err != nil {
		return "", fmt.Errorf("adding session: writing summary: %w", err)
	}

	// Add to in-memory index.
	s.index.Sessions = append(s.index.Sessions, session)

	return session.ID, nil
}

// GetSession returns the full metadata for a session by ID.
func (s *fileSessionStore) GetSession(id string) (*models.CapturedSession, error) {
	for _, session := range s.index.Sessions {
		if session.ID == id {
			return &session, nil
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

// ListSessions returns sessions matching the given filter criteria.
func (s *fileSessionStore) ListSessions(filter models.SessionFilter) ([]models.CapturedSession, error) {
	var result []models.CapturedSession

	for _, session := range s.index.Sessions {
		if filter.TaskID != "" && session.TaskID != filter.TaskID {
			continue
		}
		if filter.ProjectPath != "" && session.ProjectPath != filter.ProjectPath {
			continue
		}
		if filter.Since != nil && session.EndedAt.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && session.EndedAt.After(*filter.Until) {
			continue
		}
		if filter.MinTurns > 0 && session.TurnCount < filter.MinTurns {
			continue
		}
		result = append(result, session)
	}

	return result, nil
}

// GetSessionTurns loads turns from disk for the given session ID.
func (s *fileSessionStore) GetSessionTurns(sessionID string) ([]models.SessionTurn, error) {
	// Verify session exists in index.
	found := false
	for _, session := range s.index.Sessions {
		if session.ID == sessionID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	turnsPath := filepath.Join(s.sessionDir(sessionID), "turns.yaml")
	data, err := os.ReadFile(turnsPath)
	if err != nil {
		return nil, fmt.Errorf("reading session turns: %w", err)
	}

	var turnsWrapper struct {
		Turns []models.SessionTurn `yaml:"turns"`
	}
	if err := yaml.Unmarshal(data, &turnsWrapper); err != nil {
		return nil, fmt.Errorf("parsing session turns: %w", err)
	}

	return turnsWrapper.Turns, nil
}

// GetLatestSessionForTask returns the most recent session associated with the
// given task ID, or nil if no sessions are found.
func (s *fileSessionStore) GetLatestSessionForTask(taskID string) (*models.CapturedSession, error) {
	var latest *models.CapturedSession

	for i := range s.index.Sessions {
		session := &s.index.Sessions[i]
		if session.TaskID != taskID {
			continue
		}
		if latest == nil || session.EndedAt.After(latest.EndedAt) {
			latest = session
		}
	}

	if latest == nil {
		return nil, nil
	}

	// Return a copy.
	cp := *latest
	return &cp, nil
}

// GetRecentSessions returns the most recent sessions, ordered newest first,
// limited to the given count.
func (s *fileSessionStore) GetRecentSessions(limit int) ([]models.CapturedSession, error) {
	if len(s.index.Sessions) == 0 {
		return nil, nil
	}

	// Make a copy and sort by EndedAt descending.
	sorted := make([]models.CapturedSession, len(s.index.Sessions))
	copy(sorted, s.index.Sessions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].EndedAt.After(sorted[j].EndedAt)
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted, nil
}

// Load reads the session index from disk. Missing files are treated as empty.
func (s *fileSessionStore) Load() error {
	if err := s.loadYAML(s.indexPath(), &s.index); err != nil {
		return fmt.Errorf("loading session index: %w", err)
	}
	if s.index.Version == "" {
		s.index.Version = "1.0"
	}
	return nil
}

// Save persists the session index to disk.
func (s *fileSessionStore) Save() error {
	dir := s.sessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("saving session store: creating directory: %w", err)
	}

	if err := s.saveYAML(s.indexPath(), &s.index); err != nil {
		return fmt.Errorf("saving session index: %w", err)
	}
	return nil
}

func (s *fileSessionStore) loadYAML(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Missing files are initialized to zero values.
		}
		return err
	}
	return yaml.Unmarshal(data, target)
}

func (s *fileSessionStore) saveYAML(path string, source interface{}) error {
	data, err := yaml.Marshal(source)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

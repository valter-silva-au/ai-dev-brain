package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ContextManager defines the interface for managing task-specific context and notes
type ContextManager interface {
	ReadContext(taskID string) (string, error)
	WriteContext(taskID string, content string) error
	AppendContext(taskID string, section string) error
	ReadNotes(taskID string) (string, error)
	WriteNotes(taskID string, content string) error
}

// FileContextManager implements ContextManager with file-based storage
// It operates on per-task directories: tickets/TASK-XXXXX/context.md and notes.md
type FileContextManager struct {
	baseDir string
	mu      sync.RWMutex // Provides concurrent safety
}

// NewFileContextManager creates a new file-based context manager
// baseDir is the root directory for all task directories (e.g., "tickets")
func NewFileContextManager(baseDir string) *FileContextManager {
	return &FileContextManager{
		baseDir: baseDir,
	}
}

// getTaskDir returns the directory path for a given task ID
func (fcm *FileContextManager) getTaskDir(taskID string) string {
	return filepath.Join(fcm.baseDir, taskID)
}

// getContextPath returns the full path to the context.md file for a task
func (fcm *FileContextManager) getContextPath(taskID string) string {
	return filepath.Join(fcm.getTaskDir(taskID), "context.md")
}

// getNotesPath returns the full path to the notes.md file for a task
func (fcm *FileContextManager) getNotesPath(taskID string) string {
	return filepath.Join(fcm.getTaskDir(taskID), "notes.md")
}

// ensureTaskDir creates the task directory if it doesn't exist
func (fcm *FileContextManager) ensureTaskDir(taskID string) error {
	taskDir := fcm.getTaskDir(taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return fmt.Errorf("failed to create task directory: %w", err)
	}
	return nil
}

// ReadContext reads the context.md file for a task
// Returns an empty string if the file doesn't exist
func (fcm *FileContextManager) ReadContext(taskID string) (string, error) {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	contextPath := fcm.getContextPath(taskID)

	// Check if file exists
	if _, err := os.Stat(contextPath); os.IsNotExist(err) {
		return "", nil
	}

	// Read the file
	data, err := os.ReadFile(contextPath)
	if err != nil {
		return "", fmt.Errorf("failed to read context file: %w", err)
	}

	return string(data), nil
}

// WriteContext writes content to the context.md file for a task
// This overwrites any existing content
func (fcm *FileContextManager) WriteContext(taskID string, content string) error {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	// Ensure task directory exists
	if err := fcm.ensureTaskDir(taskID); err != nil {
		return err
	}

	contextPath := fcm.getContextPath(taskID)

	// Write to file with proper permissions
	if err := os.WriteFile(contextPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}

	return nil
}

// AppendContext appends a section to the context.md file for a task
// This is useful for hook-driven incremental updates
func (fcm *FileContextManager) AppendContext(taskID string, section string) error {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	// Ensure task directory exists
	if err := fcm.ensureTaskDir(taskID); err != nil {
		return err
	}

	contextPath := fcm.getContextPath(taskID)

	// Open file for appending, create if doesn't exist
	file, err := os.OpenFile(contextPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open context file for appending: %w", err)
	}
	defer file.Close()

	// Append section
	if _, err := file.WriteString(section); err != nil {
		return fmt.Errorf("failed to append to context file: %w", err)
	}

	return nil
}

// ReadNotes reads the notes.md file for a task
// Returns an empty string if the file doesn't exist
func (fcm *FileContextManager) ReadNotes(taskID string) (string, error) {
	fcm.mu.RLock()
	defer fcm.mu.RUnlock()

	notesPath := fcm.getNotesPath(taskID)

	// Check if file exists
	if _, err := os.Stat(notesPath); os.IsNotExist(err) {
		return "", nil
	}

	// Read the file
	data, err := os.ReadFile(notesPath)
	if err != nil {
		return "", fmt.Errorf("failed to read notes file: %w", err)
	}

	return string(data), nil
}

// WriteNotes writes content to the notes.md file for a task
// This overwrites any existing content
func (fcm *FileContextManager) WriteNotes(taskID string, content string) error {
	fcm.mu.Lock()
	defer fcm.mu.Unlock()

	// Ensure task directory exists
	if err := fcm.ensureTaskDir(taskID); err != nil {
		return err
	}

	notesPath := fcm.getNotesPath(taskID)

	// Write to file with proper permissions
	if err := os.WriteFile(notesPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write notes file: %w", err)
	}

	return nil
}

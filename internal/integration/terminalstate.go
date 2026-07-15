package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TerminalState represents the state of a terminal for a specific worktree
type TerminalState struct {
	WorktreePath string `json:"worktree_path"`
	TaskID       string `json:"task_id"`
	Status       string `json:"status"`       // e.g., "active", "pending", "blocked"
	LastUpdated  string `json:"last_updated"` // ISO 8601 timestamp
}

// TerminalStateWriter manages .adb_terminal_state.json for VS Code tab styling
type TerminalStateWriter interface {
	// WriteState writes terminal state to .adb_terminal_state.json
	WriteState(state TerminalState) error

	// ReadState reads terminal state from .adb_terminal_state.json
	ReadState(worktreePath string) (*TerminalState, error)

	// DeleteState removes a terminal state entry
	DeleteState(worktreePath string) error

	// ListStates lists all terminal states
	ListStates() ([]TerminalState, error)

	// CleanStaleStates removes entries for non-existent worktrees
	CleanStaleStates() error
}

// DefaultTerminalStateWriter implements TerminalStateWriter
type DefaultTerminalStateWriter struct {
	stateFile string
	mu        sync.Mutex
}

// NewTerminalStateWriter creates a new TerminalStateWriter
// stateFile: path to .adb_terminal_state.json (defaults to ~/.adb_terminal_state.json)
func NewTerminalStateWriter(stateFile string) TerminalStateWriter {
	if stateFile == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			stateFile = ".adb_terminal_state.json"
		} else {
			stateFile = filepath.Join(homeDir, ".adb_terminal_state.json")
		}
	}
	return &DefaultTerminalStateWriter{
		stateFile: stateFile,
	}
}

// WriteState writes terminal state to .adb_terminal_state.json
func (w *DefaultTerminalStateWriter) WriteState(state TerminalState) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Read existing states
	states, err := w.readStatesUnsafe()
	if err != nil {
		// If file doesn't exist or is corrupt, start with empty map
		states = make(map[string]TerminalState)
	}

	// Update or add state
	states[state.WorktreePath] = state

	// Write back to file
	return w.writeStatesUnsafe(states)
}

// ReadState reads terminal state from .adb_terminal_state.json
func (w *DefaultTerminalStateWriter) ReadState(worktreePath string) (*TerminalState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	states, err := w.readStatesUnsafe()
	if err != nil {
		return nil, err
	}

	state, exists := states[worktreePath]
	if !exists {
		return nil, fmt.Errorf("no state found for worktree: %s", worktreePath)
	}

	return &state, nil
}

// DeleteState removes a terminal state entry
func (w *DefaultTerminalStateWriter) DeleteState(worktreePath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	states, err := w.readStatesUnsafe()
	if err != nil {
		return err
	}

	delete(states, worktreePath)

	return w.writeStatesUnsafe(states)
}

// ListStates lists all terminal states
func (w *DefaultTerminalStateWriter) ListStates() ([]TerminalState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	states, err := w.readStatesUnsafe()
	if err != nil {
		return nil, err
	}

	result := make([]TerminalState, 0, len(states))
	for _, state := range states {
		result = append(result, state)
	}

	return result, nil
}

// CleanStaleStates removes entries for non-existent worktrees
func (w *DefaultTerminalStateWriter) CleanStaleStates() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	states, err := w.readStatesUnsafe()
	if err != nil {
		return err
	}

	// Check each worktree path
	for path := range states {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			delete(states, path)
		}
	}

	return w.writeStatesUnsafe(states)
}

// readStatesUnsafe reads states without locking (caller must hold lock)
func (w *DefaultTerminalStateWriter) readStatesUnsafe() (map[string]TerminalState, error) {
	// Check if file exists
	if _, err := os.Stat(w.stateFile); os.IsNotExist(err) {
		return make(map[string]TerminalState), nil
	}

	// Read file
	data, err := os.ReadFile(w.stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Parse JSON
	var states map[string]TerminalState
	if err := json.Unmarshal(data, &states); err != nil {
		// Corrupt JSON - reset to empty map
		return make(map[string]TerminalState), nil
	}

	return states, nil
}

// writeStatesUnsafe writes states without locking (caller must hold lock)
func (w *DefaultTerminalStateWriter) writeStatesUnsafe(states map[string]TerminalState) error {
	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal states: %w", err)
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(w.stateFile)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(w.stateFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

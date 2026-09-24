package hooks

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
)

// ChangeTracker tracks changes to files during a session
type ChangeTracker struct {
	basePath    string // workspace root; statedir.Ensure targets its .adb/
	sessionFile string
}

// NewChangeTracker creates a new change tracker. The session-changes file lives
// under the workspace's .adb/ state dir (#186), via the shared statedir seam.
func NewChangeTracker(basePath string) *ChangeTracker {
	return &ChangeTracker{
		basePath:    basePath,
		sessionFile: statedir.Path(basePath, statedir.FileSessionChanges),
	}
}

// TrackChange appends a change record to the session changes file
func (ct *ChangeTracker) TrackChange(filePath, operation string) error {
	// Ensure the .adb/ parent exists (#186) — the file now lives under it.
	if err := statedir.Ensure(ct.basePath); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	f, err := os.OpenFile(ct.sessionFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open session changes file: %w", err)
	}
	defer f.Close()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s|%s|%s\n", timestamp, operation, filePath)

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to write change record: %w", err)
	}

	return nil
}

// GetChanges returns all tracked changes
func (ct *ChangeTracker) GetChanges() ([]Change, error) {
	f, err := os.Open(ct.sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Change{}, nil
		}
		return nil, fmt.Errorf("failed to open session changes file: %w", err)
	}
	defer f.Close()

	var changes []Change
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "|", 3)
		if len(parts) == 3 {
			changes = append(changes, Change{
				Timestamp: parts[0],
				Operation: parts[1],
				FilePath:  parts[2],
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session changes: %w", err)
	}

	return changes, nil
}

// Clear removes all tracked changes
func (ct *ChangeTracker) Clear() error {
	if err := os.Remove(ct.sessionFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear session changes: %w", err)
	}
	return nil
}

// Change represents a single tracked change
type Change struct {
	Timestamp string
	Operation string
	FilePath  string
}

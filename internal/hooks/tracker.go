package hooks

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

const sessionChangesFile = ".adb_session_changes"

// ChangeTracker manages the append-only session change log.
type ChangeTracker struct {
	filePath string
}

// NewChangeTracker creates a tracker that stores changes in basePath/.adb_session_changes.
func NewChangeTracker(basePath string) *ChangeTracker {
	return &ChangeTracker{
		filePath: filepath.Join(basePath, sessionChangesFile),
	}
}

// Append adds a change entry to the session change log.
// Format per line: timestamp|tool|filepath
func (t *ChangeTracker) Append(entry models.SessionChangeEntry) error {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UTC().Unix()
	}
	f, err := os.OpenFile(t.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening change tracker: %w", err)
	}
	defer f.Close()

	line := fmt.Sprintf("%d|%s|%s\n", entry.Timestamp, entry.Tool, entry.FilePath)
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("writing change entry: %w", err)
	}
	return nil
}

// Read returns all change entries from the session change log.
func (t *ChangeTracker) Read() ([]models.SessionChangeEntry, error) {
	f, err := os.Open(t.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening change tracker: %w", err)
	}
	defer f.Close()

	var entries []models.SessionChangeEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue // Skip malformed lines.
		}
		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, models.SessionChangeEntry{
			Timestamp: ts,
			Tool:      parts[1],
			FilePath:  parts[2],
		})
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("reading change tracker: %w", err)
	}
	return entries, nil
}

// Cleanup removes the session change log file.
func (t *ChangeTracker) Cleanup() error {
	if err := os.Remove(t.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning up change tracker: %w", err)
	}
	return nil
}

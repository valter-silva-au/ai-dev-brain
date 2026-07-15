package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
)

// TaskIDGenerator defines the interface for generating task IDs
type TaskIDGenerator interface {
	GenerateTaskID() (string, error)
}

// FileTaskIDGenerator implements TaskIDGenerator with file-based counter and flock locking
type FileTaskIDGenerator struct {
	counterFile string
	prefix      string
	mu          sync.Mutex // in-process locking (for multi-threaded access within same process)
}

// NewFileTaskIDGenerator creates a new file-based task ID generator
// counterFile: path to the counter file (e.g., ".task_counter")
// prefix: prefix for task IDs (e.g., "TASK")
func NewFileTaskIDGenerator(counterFile, prefix string) *FileTaskIDGenerator {
	return &FileTaskIDGenerator{
		counterFile: counterFile,
		prefix:      prefix,
	}
}

// GenerateTaskID generates a new sequential task ID with format {prefix}-{counter:05d}
// Uses file-based counter with flock locking for cross-process safety
func (g *FileTaskIDGenerator) GenerateTaskID() (string, error) {
	// In-process lock (for multi-threaded access within same process)
	g.mu.Lock()
	defer g.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(g.counterFile)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Open or create the counter file
	file, err := os.OpenFile(g.counterFile, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to open counter file: %w", err)
	}
	defer file.Close()

	// Acquire exclusive lock - blocks until lock is available
	// This provides cross-process safety. Implementation is platform-specific
	// (flock on Unix, LockFileEx on Windows) — see internal/lockfile.
	unlock, err := lockfile.Lock(file)
	if err != nil {
		return "", fmt.Errorf("failed to acquire file lock: %w", err)
	}
	defer unlock()

	// Read current counter value
	counter, err := g.readCounter(file)
	if err != nil {
		return "", fmt.Errorf("failed to read counter: %w", err)
	}

	// Increment counter
	counter++

	// Write updated counter back to file
	if err := g.writeCounter(file, counter); err != nil {
		return "", fmt.Errorf("failed to write counter: %w", err)
	}

	// Format task ID
	taskID := fmt.Sprintf("%s-%05d", g.prefix, counter)
	return taskID, nil
}

// readCounter reads the counter value from the file
func (g *FileTaskIDGenerator) readCounter(file *os.File) (int, error) {
	// Get file size
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}

	// If file is empty, return 0
	if info.Size() == 0 {
		return 0, nil
	}

	// Read file content
	content := make([]byte, info.Size())
	if _, err := file.ReadAt(content, 0); err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse counter value
	counterStr := strings.TrimSpace(string(content))
	if counterStr == "" {
		return 0, nil
	}

	counter, err := strconv.Atoi(counterStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse counter value: %w", err)
	}

	return counter, nil
}

// writeCounter writes the counter value to the file
func (g *FileTaskIDGenerator) writeCounter(file *os.File, counter int) error {
	// Truncate file to ensure clean write
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate file: %w", err)
	}

	// Seek to beginning
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek file: %w", err)
	}

	// Write counter value
	counterStr := fmt.Sprintf("%d\n", counter)
	if _, err := file.WriteString(counterStr); err != nil {
		return fmt.Errorf("failed to write counter: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

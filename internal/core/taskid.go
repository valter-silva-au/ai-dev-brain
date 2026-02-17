package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TaskIDGenerator defines the interface for generating unique, sequential task IDs.
type TaskIDGenerator interface {
	GenerateTaskID() (string, error)
}

// fileTaskIDGenerator implements TaskIDGenerator by persisting a counter
// in a .task_counter file on disk.
type fileTaskIDGenerator struct {
	basePath string
	prefix   string
	padWidth int
}

// NewTaskIDGenerator creates a new TaskIDGenerator that stores its counter
// in a .task_counter file within basePath. padWidth controls the zero-padding
// width of the numeric portion. Use 0 for no padding (e.g., TASK-1).
func NewTaskIDGenerator(basePath string, prefix string, padWidth int) TaskIDGenerator {
	return &fileTaskIDGenerator{
		basePath: basePath,
		prefix:   prefix,
		padWidth: padWidth,
	}
}

// GenerateTaskID reads the current counter from the .task_counter file,
// increments it, writes it back, and returns the formatted task ID.
// If the counter file does not exist, the counter starts from 1.
// Format: {prefix}-{counter:05d} (e.g., TASK-00001).
func (g *fileTaskIDGenerator) GenerateTaskID() (string, error) {
	counterPath := filepath.Join(g.basePath, ".task_counter")

	counter := 0
	data, err := os.ReadFile(counterPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading task counter file: %w", err)
	}
	if err == nil {
		trimmed := strings.TrimSpace(string(data))
		counter, err = strconv.Atoi(trimmed)
		if err != nil {
			return "", fmt.Errorf("parsing task counter %q: %w", trimmed, err)
		}
	}

	counter++

	if err := os.MkdirAll(g.basePath, 0o750); err != nil {
		return "", fmt.Errorf("creating base path for task counter: %w", err)
	}

	if err := os.WriteFile(counterPath, []byte(strconv.Itoa(counter)), 0o600); err != nil {
		return "", fmt.Errorf("writing task counter file: %w", err)
	}

	if g.padWidth > 0 {
		return fmt.Sprintf("%s-%0*d", g.prefix, g.padWidth, counter), nil
	}
	return fmt.Sprintf("%s-%d", g.prefix, counter), nil
}

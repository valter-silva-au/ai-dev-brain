package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

// NormalizeTaskID converts backslashes to forward slashes and strips trailing
// slashes from a task ID. This ensures consistent lookup on Windows where users
// may pass path-based task IDs with OS-native separators.
func NormalizeTaskID(taskID string) string {
	normalized := filepath.ToSlash(taskID)
	normalized = strings.TrimRight(normalized, "/")
	return normalized
}

// legacyTaskIDPattern matches traditional TASK-XXXXX style IDs.
var legacyTaskIDPattern = regexp.MustCompile(`^[A-Z0-9]+-\d+$`)

// unsafePathSegment matches characters not allowed in path-based task ID segments.
var unsafePathSegment = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// BuildPathTaskID joins a prefix and a sanitized description with a "/" separator
// to form a path-based task ID (e.g. "finance/new-feature").
func BuildPathTaskID(prefix, description string) string {
	desc := sanitizePathSegment(description)
	if prefix == "" {
		return desc
	}
	return strings.TrimRight(prefix, "/") + "/" + desc
}

// IsLegacyTaskID returns true if the ID matches the traditional TASK-XXXXX format.
func IsLegacyTaskID(taskID string) bool {
	return legacyTaskIDPattern.MatchString(taskID)
}

// ValidatePathTaskID checks that a path-based task ID contains no ".." segments,
// no leading/trailing slashes, and that each segment is a valid directory name.
func ValidatePathTaskID(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID must not be empty")
	}
	if strings.HasPrefix(taskID, "/") || strings.HasSuffix(taskID, "/") {
		return fmt.Errorf("task ID %q must not start or end with /", taskID)
	}
	segments := strings.Split(taskID, "/")
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("task ID %q contains empty segment", taskID)
		}
		if seg == ".." || seg == "." {
			return fmt.Errorf("task ID %q contains invalid segment %q", taskID, seg)
		}
	}
	return nil
}

// NormalizeRepoToPrefix strips the "repos/" prefix from a repo path and returns
// the remaining platform/org/repo portion as a task ID prefix. Returns empty
// string if the repo path is not under basePath (e.g. an absolute path from a
// different workspace).
func NormalizeRepoToPrefix(repoPath, basePath string) string {
	cleaned := filepath.ToSlash(repoPath)

	// Clean relative path components (./, ../) before further processing.
	// Use path.Clean (not filepath.Clean) since we've already converted to
	// forward slashes.
	cleaned = path.Clean(cleaned)

	// Strip basePath prefix if present.
	if basePath != "" {
		base := filepath.ToSlash(basePath)
		base = strings.TrimRight(base, "/")
		cleaned = strings.TrimPrefix(cleaned, base+"/")
	}

	// Strip "repos/" prefix.
	cleaned = strings.TrimPrefix(cleaned, "repos/")

	// Strip trailing slashes.
	cleaned = strings.TrimRight(cleaned, "/")

	// If the result is still an absolute path (e.g. the repo is not under
	// basePath), it cannot be used as a task ID prefix. This happens when
	// detectGitRoot() returns an absolute OS path for a repo outside the
	// adb workspace. Check for Unix-style leading slash, Windows drive
	// letters (C:), and filepath.IsAbs for platform-native detection.
	if strings.HasPrefix(cleaned, "/") || filepath.IsAbs(cleaned) || (len(cleaned) >= 2 && cleaned[1] == ':') {
		return ""
	}

	return cleaned
}

// sanitizePathSegment lowercases, replaces unsafe characters with dashes,
// collapses consecutive dashes, and trims leading/trailing dashes.
func sanitizePathSegment(s string) string {
	s = strings.ToLower(s)
	s = unsafePathSegment.ReplaceAllString(s, "-")
	s = collapseDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// PrefixFromTaskID extracts the prefix portion of a path-based task ID
// (everything except the last segment). Returns empty string for legacy IDs
// or single-segment IDs.
func PrefixFromTaskID(taskID string) string {
	if IsLegacyTaskID(taskID) {
		return ""
	}
	idx := strings.LastIndex(taskID, "/")
	if idx < 0 {
		return ""
	}
	return taskID[:idx]
}

// RepoFromTaskID extracts the short repo name from a path-based task ID.
// For "github.com/org/repo/feature", returns "repo".
// For legacy IDs, returns empty string.
func RepoFromTaskID(taskID string) string {
	if IsLegacyTaskID(taskID) {
		return ""
	}
	parts := strings.Split(taskID, "/")
	if len(parts) < 2 {
		return ""
	}
	// The repo name is the second-to-last segment.
	return parts[len(parts)-2]
}

// DescriptionFromTaskID extracts the last segment (description) from a task ID.
func DescriptionFromTaskID(taskID string) string {
	idx := strings.LastIndex(taskID, "/")
	if idx < 0 {
		return taskID
	}
	return taskID[idx+1:]
}

package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// AppendToContext appends a markdown section to the task's context.md file.
// This is an append-only operation that does not rewrite existing content.
func AppendToContext(ticketPath string, section string) error {
	contextPath := filepath.Join(ticketPath, "context.md")

	f, err := os.OpenFile(contextPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening context.md: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + section + "\n"); err != nil {
		return fmt.Errorf("appending to context.md: %w", err)
	}
	return nil
}

// UpdateStatusTimestamp updates the "updated" field in the task's status.yaml.
func UpdateStatusTimestamp(ticketPath string) error {
	statusPath := filepath.Join(ticketPath, "status.yaml")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No status.yaml, nothing to update.
		}
		return fmt.Errorf("reading status.yaml: %w", err)
	}

	// Parse as generic map to preserve all fields.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing status.yaml: %w", err)
	}

	raw["updated"] = time.Now().UTC().Format(time.RFC3339)

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}

	if err := os.WriteFile(statusPath, out, 0o644); err != nil {
		return fmt.Errorf("writing status.yaml: %w", err)
	}
	return nil
}

// GroupChangesByDirectory groups change entries by their parent directory.
func GroupChangesByDirectory(entries []models.SessionChangeEntry) map[string][]string {
	grouped := make(map[string][]string)
	seen := make(map[string]bool)

	for _, e := range entries {
		dir := filepath.Dir(e.FilePath)
		base := filepath.Base(e.FilePath)
		key := dir + "/" + base
		if seen[key] {
			continue
		}
		seen[key] = true
		grouped[dir] = append(grouped[dir], base)
	}

	// Sort filenames within each directory.
	for dir := range grouped {
		sort.Strings(grouped[dir])
	}
	return grouped
}

// FormatSessionSummary creates a markdown section from grouped changes.
func FormatSessionSummary(grouped map[string][]string) string {
	if len(grouped) == 0 {
		return ""
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Session %s\n", now))

	// Sort directories for deterministic output.
	dirs := make([]string, 0, len(grouped))
	for dir := range grouped {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		files := grouped[dir]
		sb.WriteString(fmt.Sprintf("- Modified: %s/ (%s)\n", dir, strings.Join(files, ", ")))
	}
	return sb.String()
}

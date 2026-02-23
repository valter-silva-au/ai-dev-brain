package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestAppendToContext(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.md")

	// Create initial context.
	initial := "# Task Context\n\n## Summary\nExisting content.\n"
	if err := os.WriteFile(contextPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("writing initial context: %v", err)
	}

	// Append section.
	section := "### Session 2025-01-15\n- Modified: internal/core/ (foo.go)"
	if err := AppendToContext(dir, section); err != nil {
		t.Fatalf("AppendToContext failed: %v", err)
	}

	// Verify.
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("reading context: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Existing content.") {
		t.Error("existing content should be preserved")
	}
	if !strings.Contains(content, "### Session 2025-01-15") {
		t.Error("appended section should be present")
	}
}

func TestAppendToContext_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	section := "### New Section\n- Some content"
	if err := AppendToContext(dir, section); err != nil {
		t.Fatalf("AppendToContext should create file: %v", err)
	}

	contextPath := filepath.Join(dir, "context.md")
	if _, err := os.Stat(contextPath); err != nil {
		t.Fatalf("context.md should be created: %v", err)
	}
}

func TestUpdateStatusTimestamp(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	// Create initial status.yaml.
	initial := "id: TASK-00001\nstatus: in_progress\nupdated: \"2025-01-01T00:00:00Z\"\n"
	if err := os.WriteFile(statusPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("writing initial status: %v", err)
	}

	if err := UpdateStatusTimestamp(dir); err != nil {
		t.Fatalf("UpdateStatusTimestamp failed: %v", err)
	}

	// Verify timestamp was updated.
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("reading status: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "2025-01-01T00:00:00Z") {
		t.Error("timestamp should have been updated from the original value")
	}
	if !strings.Contains(content, "updated:") {
		t.Error("updated field should still be present")
	}
}

func TestUpdateStatusTimestamp_NoFile(t *testing.T) {
	dir := t.TempDir()
	// Should not error when status.yaml doesn't exist.
	if err := UpdateStatusTimestamp(dir); err != nil {
		t.Fatalf("UpdateStatusTimestamp on missing file should not error: %v", err)
	}
}

func TestGroupChangesByDirectory(t *testing.T) {
	entries := []models.SessionChangeEntry{
		{FilePath: "internal/core/foo.go"},
		{FilePath: "internal/core/bar.go"},
		{FilePath: "internal/cli/hook.go"},
		{FilePath: "internal/core/foo.go"}, // duplicate
		{FilePath: "pkg/models/task.go"},
	}

	grouped := GroupChangesByDirectory(entries)

	if len(grouped) != 3 {
		t.Fatalf("expected 3 directories, got %d", len(grouped))
	}
	if len(grouped["internal/core"]) != 2 {
		t.Errorf("internal/core should have 2 files, got %d", len(grouped["internal/core"]))
	}
	if len(grouped["internal/cli"]) != 1 {
		t.Errorf("internal/cli should have 1 file, got %d", len(grouped["internal/cli"]))
	}
	if len(grouped["pkg/models"]) != 1 {
		t.Errorf("pkg/models should have 1 file, got %d", len(grouped["pkg/models"]))
	}
}

func TestGroupChangesByDirectory_DeduplicatesFiles(t *testing.T) {
	entries := []models.SessionChangeEntry{
		{FilePath: "a/b.go"},
		{FilePath: "a/b.go"},
		{FilePath: "a/b.go"},
	}

	grouped := GroupChangesByDirectory(entries)
	if len(grouped["a"]) != 1 {
		t.Errorf("duplicate files should be deduplicated, got %d", len(grouped["a"]))
	}
}

func TestFormatSessionSummary(t *testing.T) {
	grouped := map[string][]string{
		"internal/core": {"foo.go", "bar.go"},
		"internal/cli":  {"hook.go"},
	}

	summary := FormatSessionSummary(grouped)

	if summary == "" {
		t.Fatal("summary should not be empty")
	}
	if !strings.Contains(summary, "### Session") {
		t.Error("summary should have session header")
	}
	if !strings.Contains(summary, "internal/cli/") {
		t.Error("summary should contain directory paths")
	}
	if !strings.Contains(summary, "foo.go, bar.go") {
		t.Error("summary should contain file names")
	}
}

func TestFormatSessionSummary_Empty(t *testing.T) {
	grouped := map[string][]string{}
	summary := FormatSessionSummary(grouped)
	if summary != "" {
		t.Errorf("empty grouped should produce empty summary, got %q", summary)
	}
}

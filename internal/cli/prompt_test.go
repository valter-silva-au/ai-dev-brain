package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTypeFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"[feat] add-new-feature", "feat"},
		{"[bug] fix-crash", "bug"},
		{"[spike] research-api", "spike"},
		{"[refactor] cleanup-code", "refactor"},
		{"no brackets here", "?"},
		{"", "?"},
		{"[", "?"},
		{"[]", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := parseTypeFromTitle(tt.title)
			if got != tt.want {
				t.Errorf("parseTypeFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"in_progress", "*"},
		{"blocked", "!"},
		{"review", "?"},
		{"done", "+"},
		{"backlog", "."},
		{"unknown", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := statusIcon(tt.status)
			if got != tt.want {
				t.Errorf("statusIcon(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestPriorityColor(t *testing.T) {
	tests := []struct {
		priority string
		want     string
	}{
		{"P0", "1;37;41"},
		{"P1", "1;31"},
		{"P2", "1;36"},
		{"P3", "0;37"},
		{"p0", "1;37;41"}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := priorityColor(tt.priority)
			if got != tt.want {
				t.Errorf("priorityColor(%q) = %q, want %q", tt.priority, got, tt.want)
			}
		})
	}
}

func TestParseStatusFile(t *testing.T) {
	// Create a temporary status.yaml
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	content := `task_id: PRIS-00022
title: [spike] domain-dns-cloudflare
status: in_progress
created_at: 2026-03-15T19:52:07+08:00
updated_at: 2026-03-15T19:52:07+08:00
priority: P0
`
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := parseStatusFile(statusPath)

	if s.TaskID != "PRIS-00022" {
		t.Errorf("TaskID = %q, want %q", s.TaskID, "PRIS-00022")
	}
	if s.Title != "[spike] domain-dns-cloudflare" {
		t.Errorf("Title = %q, want %q", s.Title, "[spike] domain-dns-cloudflare")
	}
	if s.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", s.Status, "in_progress")
	}
	if s.Priority != "P0" {
		t.Errorf("Priority = %q, want %q", s.Priority, "P0")
	}
}

func TestParseStatusFile_NoPriority(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	content := `task_id: TASK-00001
title: [feat] some-feature
status: backlog
created_at: 2026-03-15T19:52:07+08:00
`
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := parseStatusFile(statusPath)

	if s.Priority != "" {
		t.Errorf("Priority = %q, want empty", s.Priority)
	}
}

func TestParseStatusFile_Missing(t *testing.T) {
	s := parseStatusFile("/nonexistent/path/status.yaml")

	if s.TaskID != "" {
		t.Errorf("Expected empty TaskID for missing file, got %q", s.TaskID)
	}
}

func TestFormatTaskPrompt(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	content := `task_id: PRIS-00022
title: [spike] domain-dns-cloudflare
status: in_progress
priority: P0
`
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := formatTaskPrompt("PRIS-00022", statusPath)

	// Should contain task ID, type, priority, and status icon
	expected := `\[\033[1;37;41m\][PRIS-00022 spike P0 *]\[\033[0m\]`
	if result != expected {
		t.Errorf("formatTaskPrompt = %q, want %q", result, expected)
	}
}

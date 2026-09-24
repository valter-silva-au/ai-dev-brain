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

// TASK-00049: the generator emitter-quotes frontmatter values that need it
// (a plain YAML scalar cannot hold ": "). parseStatusFile must reverse that
// quoting losslessly while tolerating legacy unquoted files.

func TestUnquoteYAMLScalar(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{`"double quoted"`, "double quoted"},
		{`"escaped \" inside"`, `escaped " inside`},
		{`"backslash \\"`, `backslash \`},
		{`'single quoted'`, "single quoted"},
		{`'no escapes \n here'`, `no escapes \n here`},
		{`"not an edge`, `"not an edge`},
		{`"`, `"`},
		{``, ``},
		{`[feat] unquoted prefix`, `[feat] unquoted prefix`},
	}
	for _, tt := range tests {
		if got := unquoteYAMLScalar(tt.in); got != tt.want {
			t.Errorf("unquoteYAMLScalar(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseStatusFile_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	// What the generator emits for a colon-bearing, [type]-prefixed title
	// after the TASK-00049 emitter-quoting fix.
	content := `task_id: TASK-00049
title: "[feat] Learn pipeline: channel-catalog ingestion"
status: in_progress
priority: P1
`
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := parseStatusFile(statusPath)
	if s.Title != "[feat] Learn pipeline: channel-catalog ingestion" {
		t.Errorf("Title = %q, want unquoted value", s.Title)
	}
	if got := parseTypeFromTitle(s.Title); got != "feat" {
		t.Errorf("parseTypeFromTitle(quoted title) = %q, want feat", got)
	}
	if s.Status != "in_progress" || s.Priority != "P1" {
		t.Errorf("Status/Priority = %q/%q, want in_progress/P1", s.Status, s.Priority)
	}
}

func TestParseStatusFile_LegacyUnquotedStillWorks(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	content := "task_id: PRIS-00022\ntitle: [spike] domain-dns-cloudflare\nstatus: in_progress\npriority: P0\n"
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := parseStatusFile(statusPath)
	if s.Title != "[spike] domain-dns-cloudflare" || s.TaskID != "PRIS-00022" {
		t.Errorf("legacy parse broken: %#v", s)
	}
}

func TestFormatTaskPrompt_QuotedTitle(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.yaml")

	content := "task_id: TASK-00049\ntitle: \"[feat] Whatever: the colon strikes back\"\nstatus: in_progress\npriority: P1\n"
	if err := os.WriteFile(statusPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := formatTaskPrompt("TASK-00049", statusPath)
	expected := `\[\033[1;31m\][TASK-00049 feat P1 *]\[\033[0m\]`
	if result != expected {
		t.Errorf("formatTaskPrompt = %q, want %q", result, expected)
	}
}

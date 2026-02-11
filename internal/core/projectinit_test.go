package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_CreatesFullStructure(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	result, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "my-project",
		AI:       "claude",
		Prefix:   "TASK",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify all directories exist.
	dirs := []string{
		"tickets", "work", "tools",
		".claude", ".claude/rules",
		".claude/skills/commit", ".claude/skills/task-status",
		".claude/skills/push", ".claude/skills/pr", ".claude/skills/review",
		".claude/skills/sync", ".claude/skills/changelog",
		".claude/agents",
		"docs", "docs/wiki", "docs/decisions", "docs/runbooks",
	}
	for _, dir := range dirs {
		info, err := os.Stat(filepath.Join(base, dir))
		if err != nil {
			t.Errorf("directory %s not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}

	// Verify all files exist.
	files := []string{
		".taskconfig",
		".task_counter",
		".gitignore",
		"backlog.yaml",
		"CLAUDE.md",
		"tickets/README.md",
		"work/README.md",
		"tools/README.md",
		"docs/README.md",
		".claude/settings.json",
		".claude/rules/workspace.md",
		".claude/skills/commit/SKILL.md",
		".claude/skills/task-status/SKILL.md",
		".claude/skills/push/SKILL.md",
		".claude/skills/pr/SKILL.md",
		".claude/skills/review/SKILL.md",
		".claude/skills/sync/SKILL.md",
		".claude/skills/changelog/SKILL.md",
		".claude/agents/code-reviewer.md",
		"docs/stakeholders.md",
		"docs/contacts.md",
		"docs/glossary.md",
		"docs/decisions/ADR-TEMPLATE.md",
	}
	for _, f := range files {
		info, err := os.Stat(filepath.Join(base, f))
		if err != nil {
			t.Errorf("file %s not created: %v", f, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("%s is a directory, expected file", f)
		}
	}

	// Verify git repository was initialized.
	gitDir := filepath.Join(base, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		t.Errorf(".git directory not created: %v", err)
	} else if !info.IsDir() {
		t.Error(".git should be a directory")
	}

	// Most items should be in Created. The basePath itself is created by
	// t.TempDir() so it will be in the Skipped list.
	if len(result.Created) == 0 {
		t.Error("expected items in Created list")
	}
}

func TestInit_SkipsExistingFiles(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	// Pre-create .taskconfig with custom content.
	customContent := "# my custom config\ndo_not_overwrite: true\n"
	if err := os.WriteFile(filepath.Join(base, ".taskconfig"), []byte(customContent), 0o600); err != nil {
		t.Fatalf("failed to write custom .taskconfig: %v", err)
	}

	result, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "test",
		AI:       "claude",
		Prefix:   "TASK",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// .taskconfig should be preserved.
	data, err := os.ReadFile(filepath.Join(base, ".taskconfig"))
	if err != nil {
		t.Fatalf("failed to read .taskconfig: %v", err)
	}
	if string(data) != customContent {
		t.Errorf(".taskconfig was overwritten: got %q, want %q", string(data), customContent)
	}

	// .taskconfig path should be in Skipped.
	found := false
	for _, p := range result.Skipped {
		if strings.HasSuffix(p, ".taskconfig") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .taskconfig in Skipped list")
	}
}

func TestInit_SkipsExistingDirectories(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	// Pre-create tickets/ with a file inside.
	ticketsDir := filepath.Join(base, "tickets")
	if err := os.MkdirAll(ticketsDir, 0o750); err != nil {
		t.Fatalf("failed to create tickets dir: %v", err)
	}
	markerFile := filepath.Join(ticketsDir, "marker.txt")
	if err := os.WriteFile(markerFile, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("failed to write marker file: %v", err)
	}

	_, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "test",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Marker file should still be there.
	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("marker file was removed: %v", err)
	}
	if string(data) != "keep me" {
		t.Errorf("marker file content changed: got %q", string(data))
	}
}

func TestInit_Idempotent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	cfg := InitConfig{
		BasePath: base,
		Name:     "test-project",
		AI:       "kiro",
		Prefix:   "TSK",
	}

	// First run.
	result1, err := pi.Init(cfg)
	if err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	if len(result1.Created) == 0 {
		t.Error("first run should create items")
	}

	// Second run.
	result2, err := pi.Init(cfg)
	if err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	if len(result2.Created) != 0 {
		t.Errorf("second run should create nothing, but created %d items", len(result2.Created))
	}
	if len(result2.Skipped) == 0 {
		t.Error("second run should skip all items")
	}
}

func TestInit_DefaultValues(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "my-project-dir")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	pi := NewProjectInitializer()
	_, err := pi.Init(InitConfig{
		BasePath: sub,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Check .taskconfig uses defaults.
	data, err := os.ReadFile(filepath.Join(sub, ".taskconfig"))
	if err != nil {
		t.Fatalf("failed to read .taskconfig: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "ai: claude") {
		t.Error(".taskconfig should default AI to claude")
	}
	if !strings.Contains(content, `prefix: "TASK"`) {
		t.Error(".taskconfig should default prefix to TASK")
	}
}

func TestInit_CustomValues(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "custom-project",
		AI:       "kiro",
		Prefix:   "PRJ",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, ".taskconfig"))
	if err != nil {
		t.Fatalf("failed to read .taskconfig: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "ai: kiro") {
		t.Errorf(".taskconfig should contain ai: kiro, got:\n%s", content)
	}
	if !strings.Contains(content, `prefix: "PRJ"`) {
		t.Errorf(".taskconfig should contain prefix: PRJ, got:\n%s", content)
	}
}

func TestInit_TaskconfigIsValidYAML(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
		AI:       "claude",
		Prefix:   "TASK",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, ".taskconfig"))
	if err != nil {
		t.Fatalf("failed to read .taskconfig: %v", err)
	}

	content := string(data)
	// Verify it has the expected structure.
	expectedKeys := []string{"version:", "defaults:", "task_id:", "screenshot:", "offline_mode:"}
	for _, key := range expectedKeys {
		if !strings.Contains(content, key) {
			t.Errorf(".taskconfig missing key %q", key)
		}
	}
}

func TestInit_BacklogYamlContent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, "backlog.yaml"))
	if err != nil {
		t.Fatalf("failed to read backlog.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `version: "1.0"`) {
		t.Error("backlog.yaml should contain version 1.0")
	}
	if !strings.Contains(content, "tasks: {}") {
		t.Error("backlog.yaml should contain empty tasks map")
	}
}

func TestInit_TaskCounterContent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, ".task_counter"))
	if err != nil {
		t.Fatalf("failed to read .task_counter: %v", err)
	}
	if string(data) != "0" {
		t.Errorf(".task_counter should be '0', got %q", string(data))
	}
}

func TestInit_ResultSummary(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	result, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "test",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Should have created all items.
	// Directories: basePath(skipped) + tickets + work + tools + .claude +
	//              .claude/rules + .claude/skills/commit + .claude/skills/task-status +
	//              .claude/skills/push + .claude/skills/pr + .claude/skills/review +
	//              .claude/skills/sync + .claude/skills/changelog +
	//              .claude/agents + docs + docs/wiki + docs/decisions + docs/runbooks = 18
	// Files: .taskconfig + .task_counter + .gitignore + backlog.yaml + CLAUDE.md +
	//        4 READMEs + settings.json + workspace.md + 7 SKILLs + agent +
	//        stakeholders.md + contacts.md + glossary.md + ADR-TEMPLATE.md = 23
	// Git: .git = 1
	totalExpected := 42
	totalGot := len(result.Created) + len(result.Skipped)
	if totalGot != totalExpected {
		t.Errorf("expected %d total items, got %d (created=%d, skipped=%d)",
			totalExpected, totalGot, len(result.Created), len(result.Skipped))
	}
}

func TestInit_ClaudeMdContent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "my-cool-project",
		Prefix:   "MCP",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}
	content := string(data)

	// CLAUDE.md should contain the project name.
	if !strings.Contains(content, "my-cool-project") {
		t.Error("CLAUDE.md should contain project name")
	}
	// CLAUDE.md should contain the task prefix.
	if !strings.Contains(content, "MCP-XXXXX") {
		t.Error("CLAUDE.md should contain task prefix in ID format")
	}
	// Should reference adb commands.
	if !strings.Contains(content, "adb feat") {
		t.Error("CLAUDE.md should reference adb commands")
	}
}

func TestInit_ClaudeCodeConfiguration(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
		Name:     "test",
		Prefix:   "TST",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify .claude/settings.json has permissions.
	data, err := os.ReadFile(filepath.Join(base, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}
	if !strings.Contains(string(data), "permissions") {
		t.Error("settings.json should contain permissions")
	}

	// Verify .claude/rules/workspace.md references the prefix.
	data, err = os.ReadFile(filepath.Join(base, ".claude", "rules", "workspace.md"))
	if err != nil {
		t.Fatalf("failed to read workspace.md: %v", err)
	}
	if !strings.Contains(string(data), "TST-XXXXX") {
		t.Error("workspace.md should contain task prefix in ID format")
	}

	// Verify skills exist with expected content.
	data, err = os.ReadFile(filepath.Join(base, ".claude", "skills", "commit", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read commit SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "name: commit") {
		t.Error("commit skill should have name frontmatter")
	}

	data, err = os.ReadFile(filepath.Join(base, ".claude", "skills", "task-status", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read task-status SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "name: task-status") {
		t.Error("task-status skill should have name frontmatter")
	}

	// Verify new workflow skills exist with expected content.
	newSkills := []struct {
		name string
		dir  string
	}{
		{"push", "push"},
		{"pr", "pr"},
		{"review", "review"},
		{"sync", "sync"},
		{"changelog", "changelog"},
	}
	for _, s := range newSkills {
		data, err = os.ReadFile(filepath.Join(base, ".claude", "skills", s.dir, "SKILL.md"))
		if err != nil {
			t.Fatalf("failed to read %s SKILL.md: %v", s.name, err)
		}
		if !strings.Contains(string(data), "name: "+s.name) {
			t.Errorf("%s skill should have name frontmatter", s.name)
		}
	}

	// Verify agent exists.
	data, err = os.ReadFile(filepath.Join(base, ".claude", "agents", "code-reviewer.md"))
	if err != nil {
		t.Fatalf("failed to read code-reviewer.md: %v", err)
	}
	if !strings.Contains(string(data), "name: code-reviewer") {
		t.Error("code-reviewer agent should have name frontmatter")
	}
}

func TestInit_DirectoryReadmes(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tests := []struct {
		path     string
		contains string
	}{
		{"tickets/README.md", "# Tickets"},
		{"work/README.md", "# Work"},
		{"tools/README.md", "# Tools"},
		{"docs/README.md", "# Documentation"},
	}
	for _, tt := range tests {
		data, err := os.ReadFile(filepath.Join(base, tt.path))
		if err != nil {
			t.Errorf("file %s not created: %v", tt.path, err)
			continue
		}
		if !strings.Contains(string(data), tt.contains) {
			t.Errorf("file %s missing expected content %q", tt.path, tt.contains)
		}
	}
}

func TestInit_DocsSubstructure(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify docs/ template files have expected content.
	tests := []struct {
		path     string
		contains string
	}{
		{"docs/stakeholders.md", "# Stakeholders"},
		{"docs/contacts.md", "# Contacts"},
		{"docs/glossary.md", "# Glossary"},
		{"docs/decisions/ADR-TEMPLATE.md", "ADR-XXXX"},
	}
	for _, tt := range tests {
		data, err := os.ReadFile(filepath.Join(base, tt.path))
		if err != nil {
			t.Errorf("file %s not created: %v", tt.path, err)
			continue
		}
		if !strings.Contains(string(data), tt.contains) {
			t.Errorf("file %s missing expected content %q", tt.path, tt.contains)
		}
	}
}

func TestInit_GitignoreContent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	_, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	content := string(data)

	expected := []string{"work/", "repos/", ".task_counter"}
	for _, pattern := range expected {
		if !strings.Contains(content, pattern) {
			t.Errorf(".gitignore should contain %q", pattern)
		}
	}
}

func TestInit_GitInit(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	result, err := pi.Init(InitConfig{
		BasePath: base,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// .git should exist.
	gitDir := filepath.Join(base, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		t.Fatalf(".git directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".git should be a directory")
	}

	// .git should be in Created list.
	found := false
	for _, p := range result.Created {
		if strings.HasSuffix(p, ".git") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .git in Created list")
	}
}

func TestInit_GitInitIdempotent(t *testing.T) {
	base := t.TempDir()
	pi := NewProjectInitializer()

	cfg := InitConfig{BasePath: base}

	// First run creates .git.
	_, err := pi.Init(cfg)
	if err != nil {
		t.Fatalf("first Init failed: %v", err)
	}

	// Second run should skip .git.
	result2, err := pi.Init(cfg)
	if err != nil {
		t.Fatalf("second Init failed: %v", err)
	}

	found := false
	for _, p := range result2.Skipped {
		if strings.HasSuffix(p, ".git") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .git in Skipped list on second run")
	}
}

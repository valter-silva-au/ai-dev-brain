package core

import (
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// aiTypeGenerator draws a random AI type string.
func aiTypeGenerator() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"claude", "kiro", "copilot", "gemini"})
}

// prefixGenerator draws a random task ID prefix.
func prefixGenerator() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Z]{2,6}`)
}

// projectNameGenerator draws a random project name.
func projectNameGenerator() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9\-]{2,19}`)
}

// expectedDirs lists all directories that Init must create relative to basePath.
var expectedDirs = []string{
	"tickets",
	"work",
	"tools",
	".claude",
	".claude/rules",
	".claude/skills/commit",
	".claude/skills/task-status",
	".claude/skills/push",
	".claude/skills/pr",
	".claude/skills/review",
	".claude/skills/sync",
	".claude/skills/changelog",
	".claude/agents",
	"docs",
	"docs/wiki",
	"docs/decisions",
	"docs/runbooks",
}

// expectedFiles lists all files that Init must create relative to basePath.
var expectedFiles = []string{
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

// Feature: ai-dev-brain, Property: Init Idempotency
// For any valid InitConfig, running Init twice on the same directory SHALL:
// - Succeed both times without error
// - Return all items as "skipped" on the second run
func TestProperty_InitIdempotency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ai := aiTypeGenerator().Draw(rt, "ai")
		prefix := prefixGenerator().Draw(rt, "prefix")
		name := projectNameGenerator().Draw(rt, "name")

		dir, err := os.MkdirTemp("", "init-prop-idem-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		pi := NewProjectInitializer()
		cfg := InitConfig{
			BasePath: dir,
			Name:     name,
			AI:       ai,
			Prefix:   prefix,
		}

		// First run must succeed and create items.
		result1, err := pi.Init(cfg)
		if err != nil {
			t.Fatalf("first Init failed: %v", err)
		}
		if len(result1.Created) == 0 {
			t.Fatal("first run must create at least one item")
		}

		// Second run must succeed and skip everything.
		result2, err := pi.Init(cfg)
		if err != nil {
			t.Fatalf("second Init failed: %v", err)
		}
		if len(result2.Created) != 0 {
			t.Fatalf("second run must create nothing, but created %d items", len(result2.Created))
		}
		if len(result2.Skipped) == 0 {
			t.Fatal("second run must skip at least one item")
		}
	})
}

// Feature: ai-dev-brain, Property: Init Structure Completeness
// For any valid InitConfig, after Init, all expected directories and files
// SHALL exist.
func TestProperty_InitStructureCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ai := aiTypeGenerator().Draw(rt, "ai")
		prefix := prefixGenerator().Draw(rt, "prefix")
		name := projectNameGenerator().Draw(rt, "name")

		dir, err := os.MkdirTemp("", "init-prop-complete-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		pi := NewProjectInitializer()
		_, err = pi.Init(InitConfig{
			BasePath: dir,
			Name:     name,
			AI:       ai,
			Prefix:   prefix,
		})
		if err != nil {
			t.Fatalf("Init failed: %v", err)
		}

		// All expected directories must exist.
		for _, d := range expectedDirs {
			info, err := os.Stat(filepath.Join(dir, d))
			if err != nil {
				t.Fatalf("directory %s must exist: %v", d, err)
			}
			if !info.IsDir() {
				t.Fatalf("%s must be a directory", d)
			}
		}

		// All expected files must exist.
		for _, f := range expectedFiles {
			info, err := os.Stat(filepath.Join(dir, f))
			if err != nil {
				t.Fatalf("file %s must exist: %v", f, err)
			}
			if info.IsDir() {
				t.Fatalf("%s must be a file, not a directory", f)
			}
		}

		// Git repository must be initialized.
		gitDir := filepath.Join(dir, ".git")
		info, err := os.Stat(gitDir)
		if err != nil {
			t.Fatalf(".git directory must exist: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf(".git must be a directory")
		}
	})
}

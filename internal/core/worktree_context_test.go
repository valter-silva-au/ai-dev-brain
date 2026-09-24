package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// The worktree's Tier-0 task context was written to a single hardcoded
// Claude-Code path, `<worktree>/.claude/rules/task-context.md`. An agent that is
// not Claude Code — Codex, Aider, Gemini CLI — started in that worktree got
// nothing at all, which is the gap TASK-00039 Q5 exists to close.
//
// Valter's chosen layout mirrors the workspace's canonical+pointer shape, but
// puts the canonical file in adb's OWN per-worktree namespace rather than at the
// repo root:
//
//	<worktree>/.adb/task-context.md          canonical, agent-agnostic
//	<worktree>/.claude/rules/task-context.md pointer
//	<worktree>/AGENTS.md                     pointer, only when absent
//
// A bare AGENTS.md at a repo root would collide with the growing number of repos
// that ship their own, and adb's skip-if-not-adb-generated rule would then leave
// the agent silently context-less.

func taskContextFixture(t *testing.T) (worktree string, tm TemplateManager, config BootstrapConfig) {
	t.Helper()
	worktree = t.TempDir()
	manager, err := NewEmbedTemplateManager(templates.FS)
	if err != nil {
		t.Fatalf("create template manager: %v", err)
	}
	tm = manager
	config = BootstrapConfig{
		TaskID:      "TASK-00042",
		Title:       "wire the widget",
		Description: "worktree context layout",
		Status:      "in_progress",
		Branch:      "feat/wire-the-widget",
		TicketPath:  "/ws/tickets/_local/TASK-00042-wire-the-widget",
	}
	config.WorktreePath = worktree
	return worktree, tm, config
}

// The canonical file is agent-agnostic and lives under .adb/, matching the #186
// convention for everything else adb keeps per-workspace.
func TestGenerateTaskContext_WritesCanonicalUnderADB(t *testing.T) {
	worktree, tm, config := taskContextFixture(t)

	if err := generateTaskContext(worktree, tm, config); err != nil {
		t.Fatalf("generateTaskContext: %v", err)
	}

	canonical := filepath.Join(worktree, ".adb", "task-context.md")
	body, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("canonical task context not written to %s: %v", canonical, err)
	}
	if !strings.Contains(string(body), config.TaskID) {
		t.Errorf("canonical file does not name the task:\n%s", body)
	}
}

// Every harness pointer must resolve to the canonical file, and must do so
// through the @import syntax — a markdown link alone looks right and supplies no
// context, which is the same trap the workspace pointers documented.
func TestGenerateTaskContext_WritesHarnessPointers(t *testing.T) {
	worktree, tm, config := taskContextFixture(t)

	if err := generateTaskContext(worktree, tm, config); err != nil {
		t.Fatalf("generateTaskContext: %v", err)
	}

	for _, pointer := range []string{
		filepath.Join(".claude", "rules", "task-context.md"),
		"AGENTS.md",
	} {
		path := filepath.Join(worktree, pointer)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("pointer %s not written: %v", pointer, err)
			continue
		}
		if !strings.Contains(string(body), "@") {
			t.Errorf("pointer %s carries no @import, so it supplies no context:\n%s", pointer, body)
		}
		if !strings.Contains(string(body), "task-context.md") {
			t.Errorf("pointer %s does not reference the canonical file:\n%s", pointer, body)
		}
	}
}

// A repo that ships its own AGENTS.md must keep it byte for byte. This is the
// case that ruled out putting the canonical file at the repo root: the safe
// behaviour there (skip) would mean the agent gets no task context at all,
// whereas skipping a POINTER only costs that one harness's convenience — the
// canonical file under .adb/ is still there.
func TestGenerateTaskContext_NeverClobbersARepoOwnAgentsFile(t *testing.T) {
	worktree, tm, config := taskContextFixture(t)

	own := []byte("# The repo's own agent instructions\n\nDo not lose me.\n")
	agentsPath := filepath.Join(worktree, "AGENTS.md")
	if err := os.WriteFile(agentsPath, own, 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}

	if err := generateTaskContext(worktree, tm, config); err != nil {
		t.Fatalf("generateTaskContext: %v", err)
	}

	after, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(after) != string(own) {
		t.Errorf("the repo's own AGENTS.md was modified:\nwant %q\ngot  %q", own, after)
	}

	// And the canonical context still landed, so the agent is not context-less.
	if _, err := os.Stat(filepath.Join(worktree, ".adb", "task-context.md")); err != nil {
		t.Errorf("canonical task context missing after the AGENTS.md skip: %v", err)
	}
}

// Regeneration happens on every create and resume, so it has to be idempotent —
// a second run must not append, duplicate, or churn the files.
func TestGenerateTaskContext_IsIdempotent(t *testing.T) {
	worktree, tm, config := taskContextFixture(t)

	if err := generateTaskContext(worktree, tm, config); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := readWorktreeContextFiles(t, worktree)

	if err := generateTaskContext(worktree, tm, config); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second := readWorktreeContextFiles(t, worktree)

	for name, want := range first {
		if got := second[name]; got != want {
			t.Errorf("%s changed on regeneration:\nfirst  %q\nsecond %q", name, want, got)
		}
	}
}

func readWorktreeContextFiles(t *testing.T, worktree string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range []string{
		filepath.Join(".adb", "task-context.md"),
		filepath.Join(".claude", "rules", "task-context.md"),
		"AGENTS.md",
	} {
		body, err := os.ReadFile(filepath.Join(worktree, rel))
		if err != nil {
			continue
		}
		out[rel] = string(body)
	}
	return out
}

// The end-to-end property, and the one that actually mattered: rendering the
// task context into a linked worktree must ALSO leave the clone excluding those
// files, so the worktree is not instantly dirty.
func TestGenerateTaskContext_ExcludesItsOwnOutput(t *testing.T) {
	root := t.TempDir()
	repoGit := filepath.Join(root, "repo", ".git")
	worktreeGitDir := filepath.Join(repoGit, "worktrees", "TASK-00042")
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	worktree := filepath.Join(root, "work", "TASK-00042")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+worktreeGitDir+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	manager, err := NewEmbedTemplateManager(templates.FS)
	if err != nil {
		t.Fatalf("template manager: %v", err)
	}
	config := BootstrapConfig{TaskID: "TASK-00042", Title: "excluded", Status: "in_progress"}
	config.WorktreePath = worktree
	if err := generateTaskContext(worktree, manager, config); err != nil {
		t.Fatalf("generateTaskContext: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(repoGit, "info", "exclude"))
	if err != nil {
		t.Fatalf("generateTaskContext did not write the clone's exclude: %v", err)
	}
	if !strings.Contains(string(body), "/AGENTS.md") {
		t.Errorf("exclude does not cover adb's own generated files:\n%s", body)
	}
}

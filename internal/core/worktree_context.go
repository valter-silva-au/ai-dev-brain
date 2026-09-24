package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// The per-worktree Tier-0 task context, in the agent-agnostic layout decided in
// TASK-00039 (Q5).
//
// Before this, the context went to exactly one hardcoded path,
// `<worktree>/.claude/rules/task-context.md`. Any agent that is not Claude Code —
// Codex, Aider, Gemini CLI, Amazon Q — started in that worktree got no task
// context at all, which made "adb is agent-agnostic" untrue at the one place it
// matters most: the directory the agent actually runs in.
//
// The layout mirrors the workspace's canonical+pointer shape (see
// agentinstructions.go), with one deliberate difference: the canonical file lives
// under `.adb/`, adb's own per-worktree namespace, NOT at the repo root.
//
//	<worktree>/.adb/task-context.md           the context itself
//	<worktree>/.claude/rules/task-context.md  pointer
//	<worktree>/AGENTS.md                      pointer, only when absent
//
// Putting the canonical file at the repo root was the obvious reading of Q5 and
// is the wrong call, because a worktree is a checkout of somebody else's repo. A
// growing number of repos ship their own AGENTS.md; adb's
// skip-if-not-adb-generated rule would then leave that file correctly untouched
// and the agent silently context-less. Under this layout a skipped AGENTS.md
// costs one harness's convenience, and the context is still on disk where the
// pointer — and any agent told to read `.adb/task-context.md` — can find it.
const (
	// WorktreeContextDir is adb's per-worktree namespace, matching the #186
	// convention for per-workspace state.
	WorktreeContextDir = ".adb"
	// WorktreeContextFile is the canonical, agent-agnostic task context.
	WorktreeContextFile = "task-context.md"
)

// worktreeContextPointers are the per-harness pointer files adb publishes beside
// the canonical context. Each is a path relative to the worktree root.
//
// `AGENTS.md` is here as a POINTER rather than as the canonical file: that is
// what makes a repo's own AGENTS.md safe (it is skipped, and nothing is lost).
func worktreeContextPointers() []string {
	return []string{
		filepath.Join(".claude", "rules", "task-context.md"),
		"AGENTS.md",
	}
}

// generatedWorktreePaths are the paths adb writes into a worktree, as gitignore
// patterns anchored to the worktree root.
//
// This list is the reason excludeGeneratedWorktreeFiles exists — see its doc.
func generatedWorktreePaths() []string {
	return []string{
		"/" + WorktreeContextDir + "/",
		"/.claude/rules/task-context.md",
		"/AGENTS.md",
	}
}

// worktreeContextPointerBody is the content of a per-harness pointer file.
//
// The @import line is load-bearing, not decorative: Claude Code only actually
// reads a referenced file through its @ syntax, so a pointer carrying only a
// markdown link would look correct in review and supply nothing at runtime. The
// markdown link follows for harnesses (and humans) that do not implement imports.
func worktreeContextPointerBody() string {
	canonical := WorktreeContextDir + "/" + WorktreeContextFile
	return fmt.Sprintf(`# Task context

%s

The context for the task this worktree belongs to lives in
[%s](./%s) — one agent-agnostic file, so every coding agent
started here reads the same thing.

@%s
`, generatedMarker, canonical, canonical, canonical)
}

// generateTaskContext renders the Tier-0 task context into a worktree: the
// canonical file under .adb/, plus a pointer per harness.
//
// It is on the create/resume hot path, so it stays cheap and its callers treat a
// failure as non-fatal (a stale or missing context file must not fail a task
// create). A pointer that cannot be published because the user owns that filename
// is reported by publishInstructionFile as skipped and is deliberately NOT an
// error — the canonical file is what carries the context.
func generateTaskContext(worktreePath string, tm TemplateManager, config BootstrapConfig) error {
	if worktreePath == "" {
		return fmt.Errorf("worktree path is required")
	}

	contextDir := filepath.Join(worktreePath, WorktreeContextDir)
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", WorktreeContextDir, err)
	}

	templateData := taskContextData(config, nowStamp())
	canonical := filepath.Join(contextDir, WorktreeContextFile)
	if err := renderTemplateToFile(tm, TemplateTypeTaskContext, templateData, canonical); err != nil {
		return err
	}

	// Pointers are best-effort by design: a repo that owns one of these
	// filenames keeps it, and the canonical file above is unaffected.
	body := worktreeContextPointerBody()
	for _, rel := range worktreeContextPointers() {
		path := filepath.Join(worktreePath, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", rel, err)
		}
		if _, err := publishInstructionFile(worktreePath, rel, body, false); err != nil {
			return fmt.Errorf("failed to publish %s: %w", rel, err)
		}
	}

	// Teach the clone to ignore what we just wrote, so the worktree is not
	// instantly dirty. This reuses the SAME helper the Serena provisioner uses —
	// it had already solved this problem for `.serena/`, and a second independent
	// appender to one git exclude file is how the two spellings drift.
	//
	// Non-fatal on purpose: failing to write an exclude is cosmetic, while failing
	// the render would break `task create`. A repo-less task's ticket directory has
	// no .git at all, which is the common case for the error being ignored here.
	for _, pattern := range generatedWorktreePaths() {
		if err := excludeFromWorktreeVCS(worktreePath, pattern); err != nil {
			// Every remaining pattern targets the SAME exclude file, so the first
			// failure (no .git, an unreadable common dir) is the verdict for all of
			// them — retrying the rest would only repeat it.
			break
		}
	}

	//nolint:nilerr // deliberate: a failed VCS exclude is cosmetic, and must not fail `task create` after the context rendered fine
	return nil
}

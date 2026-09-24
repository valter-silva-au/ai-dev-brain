package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContextGenerator generates context files for AI agents
type ContextGenerator interface {
	// GenerateContext regenerates CLAUDE.md from backlog and task data
	GenerateContext() error

	// GenerateTaskContext generates task-specific context
	GenerateTaskContext(taskID string, hookMode bool) error

	// GenerateRepoContext generates repository context
	GenerateRepoContext() error

	// GenerateClaudeUserContext generates Claude user context
	GenerateClaudeUserContext(dryRun bool, mcp bool) error

	// GenerateAll regenerates all context files
	GenerateAll() error
}

// DefaultContextGenerator implements ContextGenerator
type DefaultContextGenerator struct {
	backlogPath string
	ticketsDir  string
	repoRoot    string
	templateMgr TemplateManager
}

// NewContextGenerator creates a new context generator
func NewContextGenerator(backlogPath, ticketsDir, repoRoot string, templateMgr TemplateManager) ContextGenerator {
	return &DefaultContextGenerator{
		backlogPath: backlogPath,
		ticketsDir:  ticketsDir,
		repoRoot:    repoRoot,
		templateMgr: templateMgr,
	}
}

// GenerateContext regenerates CLAUDE.md from backlog and task data
func (cg *DefaultContextGenerator) GenerateContext() error {
	// Read backlog
	backlogData, err := os.ReadFile(cg.backlogPath)
	if err != nil {
		return fmt.Errorf("failed to read backlog: %w", err)
	}

	// Create the instruction body
	content := "# AI Dev Brain Context\n\n"
	content += "## Overview\n\n"
	content += "This workspace is managed by AI Dev Brain (adb), a task management system for AI-assisted development.\n\n"
	content += "## Current Backlog\n\n"
	content += "```yaml\n"
	content += string(backlogData)
	content += "\n```\n\n"
	content += "## Workspace Structure\n\n"
	content += "- `tickets/` - Task-specific context and notes\n"
	content += "- `work/` - Git worktrees for task isolation\n"
	content += "- `backlog.yaml` - Task backlog\n"
	content += "- `.adb/events.jsonl` - Event log for observability\n"

	// Publish through the shared instruction writer so the thin generator lands
	// the same canonical AGENTS.md + pointer layout as the default one. Without
	// this, `--thin` would quietly reintroduce a Claude-only workspace.
	if _, err := WriteInstructionFiles(
		cg.repoRoot, content, DefaultInstructionPointers(), false,
	); err != nil {
		return fmt.Errorf("failed to write instruction files: %w", err)
	}

	return nil
}

// GenerateTaskContext generates task-specific context
func (cg *DefaultContextGenerator) GenerateTaskContext(taskID string, hookMode bool) error {
	// Resolve the id to a ticket dir on disk; do NOT join it onto ticketsDir.
	//
	// Joining assumed the FLAT layout, which tickets have not used since the
	// correlation layout landed (L100 §5) — a real ticket is at
	// tickets/<platform>/<org>/<repo>/TASK-id-slug/ or tickets/_local/TASK-id-slug/.
	// So for every non-legacy ticket this reported success while writing to
	// tickets/TASK-id/context.md, a directory os.MkdirAll below had just invented,
	// and left the real ticket's context.md stale. Worse, the phantom dir's base
	// name is exactly the id, so ResolveTicketDir matches it too — and it is
	// SHALLOWER than the real one, which is the tie-break, so it could win from
	// then on.
	//
	// Resolving is also what makes gosec's G703 on the write below unreachable
	// here: ResolveTicketDir walks ticketsDir and matches base names, so a taskID
	// containing ".." or an absolute path matches nothing and is rejected. That
	// guard previously lived only in internal/cli (taskIsKnown), while
	// GenerateTaskContext is on the exported ContextGenerator interface.
	taskDir, err := ResolveTicketDir(cg.ticketsDir, taskID)
	if err != nil {
		return fmt.Errorf("failed to resolve ticket directory for %s: %w", taskID, err)
	}
	contextPath := filepath.Join(taskDir, "context.md")

	// Read existing context if it exists
	existingContext := ""
	if data, err := os.ReadFile(contextPath); err == nil {
		existingContext = string(data)
	}

	// In hook mode, just append a timestamp
	if hookMode {
		timestamp := fmt.Sprintf("\n\n## Updated: %s\n\n", getTimestamp())
		content := existingContext + timestamp
		// gosec G703 still sees taskID as tainted here: its analysis cannot follow
		// the constraint through ResolveTicketDir's directory walk. contextPath is
		// built from a path that walk RETURNED, so it is inside ticketsDir by
		// construction and a "../" id matches nothing. Pinned by
		// TestGenerateTaskContext_RefusesToTraverse.
		//nolint:gosec // contextPath comes from ResolveTicketDir's walk, not from taskID
		return os.WriteFile(contextPath, []byte(content), 0o644)
	}

	// Generate full context using template
	data := map[string]interface{}{
		"TaskID":          taskID,
		"ExistingContext": existingContext,
	}

	content, err := cg.templateMgr.Render("task-context.md", data)
	if err != nil {
		return fmt.Errorf("failed to render task context template: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return fmt.Errorf("failed to create task directory: %w", err)
	}

	if err := os.WriteFile(contextPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write task context: %w", err)
	}

	return nil
}

// workTreeDir is the workspace directory holding per-ticket git worktrees. The
// repo-context walk prunes it: it holds thousands of files that belong to other
// repositories, not to this workspace.
const workTreeDir = "work"

// isUnderWorkTree reports whether a workspace-relative path lies INSIDE the
// work/ worktree tree. sep is the separator the path is written with — the
// caller passes filepath.Separator, because relPath comes from filepath.Rel.
//
// That separator is a parameter, not read from filepath inside, for two reasons.
//
// It is what makes the Windows behaviour testable on a Unix host, and this check
// is where a Windows-only bug lived: it used to be inlined at the call site as
// strings.HasPrefix(relPath, "work/") against a filepath.Rel result, so on
// Windows it was handed `work\github.com\…`, the prefix never matched, and the
// prune silently did nothing — `adb context repos` would walk and document every
// ticket worktree into .adb/repo-context.md. A test that can only construct
// "work/x" on the host it runs on cannot observe that.
//
// And it keeps the rule honest per-platform rather than treating both slashes as
// separators everywhere: on Unix a backslash is a legal character in a file name,
// so a directory literally named `work\x` at the workspace root is ONE component
// and must not be pruned.
//
// `work` itself returns false, deliberately: the walk documents the `- work/` row
// and prunes at each direct child instead. That is the pre-existing, Unix-visible
// output of `adb context repos`, which the separator fix must not change.
func isUnderWorkTree(relPath string, sep rune) bool {
	if sep != '/' {
		relPath = strings.ReplaceAll(relPath, string(sep), "/")
	}
	return strings.HasPrefix(relPath, workTreeDir+"/")
}

// GenerateRepoContext generates repository context
func (cg *DefaultContextGenerator) GenerateRepoContext() error {
	repoContextPath := filepath.Join(cg.repoRoot, ".adb", "repo-context.md")

	// Gather repository information
	content := "# Repository Context\n\n"
	content += "## Structure\n\n"

	// Walk the repository and document structure
	err := filepath.Walk(cg.repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// The relative path drives BOTH the skip rule and the depth check, so an
		// unchecked error here is not cosmetic: an empty relPath defeats the
		// `work/` skip (so every ticket worktree gets walked and documented) and
		// emits a bare `- /` row. Unreachable for a path filepath.Walk produced
		// under repoRoot, which is why it is reported rather than papered over —
		// the walk above is strict about its errors too.
		relPath, relErr := filepath.Rel(cg.repoRoot, path)
		if relErr != nil {
			return fmt.Errorf("failed to resolve %s relative to the workspace: %w", path, relErr)
		}

		// The workspace root, which filepath.Walk always visits first. It is
		// neither skippable nor documentable, and getting either half wrong was a
		// separate bug — so it is handled once, up front, before the skip rule.
		//
		// It must not PRUNE. The skip test below used to read the base name off
		// `path`, which at the root is the workspace directory's own name; a
		// workspace at ~/.adb-workspace therefore matched the hidden-directory
		// rule on the very first callback, returned SkipDir, and ended the walk
		// before visiting a single child. `.adb/repo-context.md` came out holding
		// nothing but the two header lines, and `adb context repos` reported
		// success over it: a total, silent failure. A dot in the workspace's own
		// name says nothing about its contents — hidden-ness is a property of a
		// component INSIDE the workspace.
		//
		// And it must not be a ROW. relPath is "." here, which has zero
		// separators and so passed the depth check, and filepath.ToSlash(".") is
		// "." — emitting a literal `- ./` line. The workspace root is not a
		// directory inside the workspace, so it is not part of its structure; the
		// row also named the very file it was being written into.
		//
		// relPath == "." identifies the root unambiguously (filepath.Rel returns
		// exactly that for path == repoRoot) and is already computed, so this
		// needs no extra syscall and no comparison of two path spellings.
		if relPath == "." {
			return nil
		}

		// Skip hidden directories and work directories.
		//
		// The hidden test reads the base name off relPath rather than path,
		// keeping the whole callback on the relative path the comment above
		// promises. For every non-root entry the two are the same string — path is
		// repoRoot + separator + relPath, so both take the same trailing
		// component — which is what makes this switch behaviour-neutral now that
		// the root returns above; verified over hidden, spaced, and
		// backslash-containing names. Reading it off relPath is also the spelling
		// that CANNOT reacquire the root bug: relPath is scoped to the workspace,
		// so nothing outside it can ever be inspected here again.
		if strings.HasPrefix(filepath.Base(relPath), ".") ||
			isUnderWorkTree(relPath, filepath.Separator) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only document directories at the first two levels. The bound admits 0
		// or 1 separators — `tickets` (level 1) and `tickets/_local` (level 2) —
		// and with the root returning above those are the only depths that reach
		// it, so the comment is now exact. It used to admit a third, `.` (also
		// zero separators), which is what made "first two levels" false; that
		// resolved for free when the root stopped being a row, and the bound is
		// deliberately UNCHANGED — tightening it would change which directories
		// appear in every existing workspace's generated repo-context.md.
		//
		// The depth count is already separator-correct on both platforms —
		// os.PathSeparator is whatever filepath.Rel just wrote — and counting `/`
		// in the slash form below would give the identical number, so it is left
		// alone.
		if info.IsDir() && strings.Count(relPath, string(os.PathSeparator)) < 2 {
			// Render with forward slashes so the generated markdown does not
			// change shape with the host OS (a no-op on Unix, where ToSlash is
			// the identity). A reader of .adb/repo-context.md — human or agent —
			// should not have to infer which platform wrote it.
			content += fmt.Sprintf("- `%s/`\n", filepath.ToSlash(relPath))
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk repository: %w", err)
	}

	// Ensure directory exists
	adbDir := filepath.Join(cg.repoRoot, ".adb")
	if err := os.MkdirAll(adbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .adb directory: %w", err)
	}

	if err := os.WriteFile(repoContextPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write repo context: %w", err)
	}

	return nil
}

// GenerateClaudeUserContext generates Claude user context
func (cg *DefaultContextGenerator) GenerateClaudeUserContext(dryRun bool, mcp bool) error {
	content := "# Claude User Context\n\n"
	content += "Generated by AI Dev Brain\n\n"

	if mcp {
		content += "## MCP Integration\n\n"
		content += "This workspace uses Model Context Protocol for enhanced AI integration.\n\n"
	}

	if dryRun {
		fmt.Println("DRY RUN: Would write Claude user context:")
		fmt.Println(content)
		return nil
	}

	userContextPath := filepath.Join(cg.repoRoot, ".adb", "claude-user.md")

	// Ensure directory exists
	adbDir := filepath.Join(cg.repoRoot, ".adb")
	if err := os.MkdirAll(adbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .adb directory: %w", err)
	}

	if err := os.WriteFile(userContextPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write Claude user context: %w", err)
	}

	return nil
}

// GenerateAll regenerates all context files
func (cg *DefaultContextGenerator) GenerateAll() error {
	if err := cg.GenerateContext(); err != nil {
		return fmt.Errorf("failed to generate CLAUDE.md: %w", err)
	}

	if err := cg.GenerateRepoContext(); err != nil {
		return fmt.Errorf("failed to generate repo context: %w", err)
	}

	if err := cg.GenerateClaudeUserContext(false, false); err != nil {
		return fmt.Errorf("failed to generate Claude user context: %w", err)
	}

	return nil
}

// getTimestamp returns the current timestamp in ISO 8601 format
func getTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

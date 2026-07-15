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

	// Create CLAUDE.md content
	content := "# AI Dev Brain - Claude Context\n\n"
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
	content += "- `.events.jsonl` - Event log for observability\n"

	// Write CLAUDE.md
	claudePath := filepath.Join(cg.repoRoot, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	return nil
}

// GenerateTaskContext generates task-specific context
func (cg *DefaultContextGenerator) GenerateTaskContext(taskID string, hookMode bool) error {
	taskDir := filepath.Join(cg.ticketsDir, taskID)
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

		// Skip hidden directories and work directories
		relPath, _ := filepath.Rel(cg.repoRoot, path)
		if strings.HasPrefix(filepath.Base(path), ".") || strings.HasPrefix(relPath, "work/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only document directories at first two levels
		if info.IsDir() && strings.Count(relPath, string(os.PathSeparator)) < 2 {
			content += fmt.Sprintf("- `%s/`\n", relPath)
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

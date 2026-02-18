package core

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// WorktreeCreator is the subset of GitWorktreeManager that the bootstrap
// system needs. Defining it here avoids importing the integration package.
type WorktreeCreator interface {
	CreateWorktree(config WorktreeCreateConfig) (string, error)
}

// WorktreeCreateConfig mirrors integration.WorktreeConfig so that callers
// can pass the required fields without importing integration.
type WorktreeCreateConfig struct {
	RepoPath   string
	BranchName string
	TaskID     string
	BaseBranch string
}

// BootstrapConfig holds the parameters for bootstrapping a new task.
type BootstrapConfig struct {
	Type       models.TaskType
	Title      string
	BranchName string
	RepoPath   string
	Template   string
	Priority   models.Priority
	Owner      string
	Tags       []string
	Source     string
	// TaskID, if non-empty, is used instead of generating a new one.
	// This allows callers to pre-generate the ID for branch formatting.
	TaskID string
	// Prefix is passed through for informational purposes (the pre-computed
	// TaskID already contains the prefix). Not used in bootstrap logic.
	Prefix string
}

// BootstrapResult holds the outputs of a successful bootstrap operation.
type BootstrapResult struct {
	TaskID       string
	TicketPath   string
	WorktreePath string
	ContextPath  string
}

// BootstrapSystem defines the interface for initializing new tasks with
// their full directory structure, templates, and worktree.
type BootstrapSystem interface {
	Bootstrap(config BootstrapConfig) (*BootstrapResult, error)
	ApplyTemplate(taskID string, templateType models.TaskType) error
	GenerateTaskID() (string, error)
}

// bootstrapSystem implements BootstrapSystem by coordinating the
// TaskIDGenerator, WorktreeCreator, and TemplateManager.
type bootstrapSystem struct {
	basePath    string
	idGen       TaskIDGenerator
	worktreeMgr WorktreeCreator
	tmplMgr     TemplateManager
}

// NewBootstrapSystem creates a new BootstrapSystem.
// worktreeMgr may be nil if worktree creation is not needed.
func NewBootstrapSystem(basePath string, idGen TaskIDGenerator, worktreeMgr WorktreeCreator, tmplMgr TemplateManager) BootstrapSystem {
	return &bootstrapSystem{
		basePath:    basePath,
		idGen:       idGen,
		worktreeMgr: worktreeMgr,
		tmplMgr:     tmplMgr,
	}
}

// Bootstrap creates a new task with a unique ID, directory structure,
// template files, status.yaml, context.md, and optionally a git worktree.
func (bs *bootstrapSystem) Bootstrap(config BootstrapConfig) (*BootstrapResult, error) {
	var taskID string
	if config.TaskID != "" {
		taskID = config.TaskID
	} else {
		var err error
		taskID, err = bs.idGen.GenerateTaskID()
		if err != nil {
			return nil, fmt.Errorf("generating task ID: %w", err)
		}
	}

	ticketPath := filepath.Join(bs.basePath, "tickets", taskID)

	// Create the ticket directory structure.
	dirs := []string{
		ticketPath,
		filepath.Join(ticketPath, "communications"),
		filepath.Join(ticketPath, "sessions"),
		filepath.Join(ticketPath, "knowledge"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Apply type-specific template (writes notes.md and design.md).
	if err := bs.tmplMgr.ApplyTemplate(ticketPath, config.Type); err != nil {
		return nil, fmt.Errorf("applying template for %s: %w", taskID, err)
	}

	// Write context.md with initial scaffold.
	contextPath := filepath.Join(ticketPath, "context.md")
	contextContent := fmt.Sprintf(`# Task Context: %s

## Summary

## Current Focus

## Recent Progress

## Open Questions

## Decisions Made

## Blockers

## Next Steps

## Related Resources
`, taskID)
	if err := os.WriteFile(contextPath, []byte(contextContent), 0o600); err != nil {
		return nil, fmt.Errorf("writing context.md for %s: %w", taskID, err)
	}

	// Write status.yaml.
	now := time.Now().UTC()
	task := models.Task{
		ID:       taskID,
		Title:    config.Title,
		Type:     config.Type,
		Status:   models.StatusBacklog,
		Priority: config.Priority,
		Owner:    config.Owner,
		Repo:     config.RepoPath,
		Branch:   config.BranchName,
		Created:  now,
		Updated:  now,
		Tags:     config.Tags,
		Source:   config.Source,
	}

	result := &BootstrapResult{
		TaskID:      taskID,
		TicketPath:  ticketPath,
		ContextPath: contextPath,
	}

	// Create worktree if a repo path and branch are provided.
	if config.RepoPath != "" && config.BranchName != "" && bs.worktreeMgr != nil {
		wtPath, err := bs.worktreeMgr.CreateWorktree(WorktreeCreateConfig{
			RepoPath:   config.RepoPath,
			BranchName: config.BranchName,
			TaskID:     taskID,
		})
		if err != nil {
			return nil, fmt.Errorf("creating worktree for %s: %w", taskID, err)
		}
		task.WorktreePath = wtPath
		result.WorktreePath = wtPath
	}

	// Generate .claude/rules/task-context.md in the worktree.
	if result.WorktreePath != "" {
		// Non-fatal: log but don't fail bootstrap.
		_ = bs.generateTaskContext(result.WorktreePath, taskID, config)
	}

	task.TicketPath = ticketPath

	statusPath := filepath.Join(ticketPath, "status.yaml")
	statusData, err := yaml.Marshal(&task)
	if err != nil {
		return nil, fmt.Errorf("marshalling status.yaml for %s: %w", taskID, err)
	}
	if err := os.WriteFile(statusPath, statusData, 0o600); err != nil {
		return nil, fmt.Errorf("writing status.yaml for %s: %w", taskID, err)
	}

	return result, nil
}

// ApplyTemplate delegates to the TemplateManager, applying the type-specific
// template to the task's ticket folder. Checks both active and archived locations.
func (bs *bootstrapSystem) ApplyTemplate(taskID string, templateType models.TaskType) error {
	ticketPath := resolveTicketDir(bs.basePath, taskID)
	return bs.tmplMgr.ApplyTemplate(ticketPath, templateType)
}

// GenerateTaskID delegates to the TaskIDGenerator.
func (bs *bootstrapSystem) GenerateTaskID() (string, error) {
	return bs.idGen.GenerateTaskID()
}

// generateTaskContext writes a .claude/rules/task-context.md file inside the
// worktree so AI assistants have immediate task awareness.
func (bs *bootstrapSystem) generateTaskContext(worktreePath, taskID string, config BootstrapConfig) error {
	rulesDir := filepath.Join(worktreePath, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return fmt.Errorf("creating rules directory: %w", err)
	}

	content := fmt.Sprintf(`# Task Context: %s

This worktree is for task %s (%s).

- **Type**: %s
- **Branch**: %s
- **Status**: backlog (newly created)
- **Ticket**: tickets/%s/

## Key Files
- tickets/%s/context.md -- Running context (update as you work)
- tickets/%s/notes.md -- Requirements and acceptance criteria
- tickets/%s/design.md -- Technical design document
- tickets/%s/sessions/ -- Session summaries (save progress between sessions)
- tickets/%s/knowledge/ -- Extracted decisions and facts

## Instructions
- Update context.md with progress, decisions, and blockers as you work
- Save session summaries to sessions/ when ending a work session
- Record key decisions in knowledge/decisions.yaml
`, taskID, taskID, config.Title, config.Type, config.BranchName,
		taskID, taskID, taskID, taskID, taskID, taskID)

	contextPath := filepath.Join(rulesDir, "task-context.md")
	return os.WriteFile(contextPath, []byte(content), 0o644)
}

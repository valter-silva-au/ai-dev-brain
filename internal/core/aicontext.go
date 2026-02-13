package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// AIType represents the target AI assistant.
type AIType string

const (
	AITypeClaude AIType = "claude"
	AITypeKiro   AIType = "kiro"
)

// ContextSection represents a specific section in the AI context file.
type ContextSection string

const (
	SectionOverview          ContextSection = "overview"
	SectionStructure         ContextSection = "structure"
	SectionConventions       ContextSection = "conventions"
	SectionGlossary          ContextSection = "glossary"
	SectionDecisions         ContextSection = "decisions"
	SectionActiveTasks       ContextSection = "active_tasks"
	SectionCriticalDecisions ContextSection = "critical_decisions"
	SectionRecentSessions    ContextSection = "recent_sessions"
	SectionContacts          ContextSection = "contacts"
)

// AIContextFile represents a generated AI context file.
type AIContextFile struct {
	AIType      AIType
	GeneratedAt time.Time
	Sections    AIContextSections
}

// AIContextSections holds the content for each section of the AI context file.
type AIContextSections struct {
	Overview            string
	DirectoryStructure  string
	Conventions         string
	Glossary            string
	DecisionsSummary    string
	ActiveTaskSummaries string
	CriticalDecisions   string
	RecentSessions      string
	StakeholderLinks    string
	ContactLinks        string
}

// AIContextGenerator generates and maintains root-level AI context files.
type AIContextGenerator interface {
	GenerateContextFile(aiType AIType) (string, error)
	RegenerateSection(section ContextSection) error
	SyncContext() error
	AssembleProjectOverview() (string, error)
	AssembleDirectoryStructure() (string, error)
	AssembleConventions() (string, error)
	AssembleGlossary() (string, error)
	AssembleActiveTaskSummaries() (string, error)
	AssembleDecisionsSummary() (string, error)
}

type aiContextGenerator struct {
	basePath   string
	backlogMgr storage.BacklogManager
	sections   AIContextSections
}

// NewAIContextGenerator creates a new AIContextGenerator.
func NewAIContextGenerator(basePath string, backlogMgr storage.BacklogManager) AIContextGenerator {
	return &aiContextGenerator{
		basePath:   basePath,
		backlogMgr: backlogMgr,
	}
}

func (g *aiContextGenerator) filenameForAI(aiType AIType) string {
	switch aiType {
	case AITypeClaude:
		return "CLAUDE.md"
	case AITypeKiro:
		return "kiro.md"
	default:
		return "CLAUDE.md"
	}
}

func (g *aiContextGenerator) GenerateContextFile(aiType AIType) (string, error) {
	if err := g.assembleAll(); err != nil {
		return "", fmt.Errorf("generating context file: %w", err)
	}

	content := g.renderContextFile()
	path := filepath.Join(g.basePath, g.filenameForAI(aiType))

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("generating context file: writing %s: %w", path, err)
	}

	return path, nil
}

func (g *aiContextGenerator) RegenerateSection(section ContextSection) error {
	var err error
	switch section {
	case SectionOverview:
		g.sections.Overview, err = g.AssembleProjectOverview()
	case SectionStructure:
		g.sections.DirectoryStructure, err = g.AssembleDirectoryStructure()
	case SectionConventions:
		g.sections.Conventions, err = g.AssembleConventions()
	case SectionGlossary:
		g.sections.Glossary, err = g.AssembleGlossary()
	case SectionDecisions:
		g.sections.DecisionsSummary, err = g.AssembleDecisionsSummary()
	case SectionActiveTasks:
		g.sections.ActiveTaskSummaries, err = g.AssembleActiveTaskSummaries()
	case SectionCriticalDecisions:
		g.sections.CriticalDecisions, err = g.assembleCriticalDecisions()
	case SectionRecentSessions:
		g.sections.RecentSessions, err = g.assembleRecentSessions()
	case SectionContacts:
		g.sections.ContactLinks = g.assembleContacts()
		g.sections.StakeholderLinks = g.assembleStakeholders()
	default:
		return fmt.Errorf("unknown section: %s", section)
	}
	return err
}

func (g *aiContextGenerator) SyncContext() error {
	if err := g.assembleAll(); err != nil {
		return fmt.Errorf("syncing context: %w", err)
	}

	// Generate both CLAUDE.md and kiro.md.
	for _, aiType := range []AIType{AITypeClaude, AITypeKiro} {
		content := g.renderContextFile()
		path := filepath.Join(g.basePath, g.filenameForAI(aiType))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("syncing context: writing %s: %w", path, err)
		}
	}

	return nil
}

func (g *aiContextGenerator) assembleAll() error {
	var err error

	g.sections.Overview, err = g.AssembleProjectOverview()
	if err != nil {
		return err
	}
	g.sections.DirectoryStructure, err = g.AssembleDirectoryStructure()
	if err != nil {
		return err
	}
	g.sections.Conventions, err = g.AssembleConventions()
	if err != nil {
		return err
	}
	g.sections.Glossary, err = g.AssembleGlossary()
	if err != nil {
		return err
	}
	g.sections.DecisionsSummary, err = g.AssembleDecisionsSummary()
	if err != nil {
		return err
	}
	g.sections.ActiveTaskSummaries, err = g.AssembleActiveTaskSummaries()
	if err != nil {
		return err
	}
	g.sections.CriticalDecisions, err = g.assembleCriticalDecisions()
	if err != nil {
		return err
	}
	g.sections.RecentSessions, err = g.assembleRecentSessions()
	if err != nil {
		return err
	}
	g.sections.StakeholderLinks = g.assembleStakeholders()
	g.sections.ContactLinks = g.assembleContacts()

	return nil
}

func (g *aiContextGenerator) AssembleProjectOverview() (string, error) {
	return "AI Dev Brain is a developer productivity system that wraps AI coding assistants with persistent context management. This monorepo contains task management, documentation, and multi-repository worktrees.", nil
}

func (g *aiContextGenerator) AssembleDirectoryStructure() (string, error) {
	var sb strings.Builder
	sb.WriteString("- `docs/` - Organizational knowledge (wiki, ADRs, runbooks, contacts)\n")
	sb.WriteString("- `tickets/` - Task folders with context, communications, and design docs\n")
	sb.WriteString("- `tickets/_archived/` - Archived task folders (moved here on archive, restored on unarchive)\n")
	sb.WriteString("- `repos/` - Git worktrees organized by platform/org/repo\n")
	sb.WriteString("- `backlog.yaml` - Central task registry")
	return sb.String(), nil
}

func (g *aiContextGenerator) AssembleConventions() (string, error) {
	var sb strings.Builder

	// Try to read conventions from wiki files.
	conventionsPath := filepath.Join(g.basePath, "docs", "wiki")
	if entries, err := os.ReadDir(conventionsPath); err == nil {
		for _, entry := range entries {
			if strings.Contains(strings.ToLower(entry.Name()), "convention") {
				data, err := os.ReadFile(filepath.Join(conventionsPath, entry.Name())) //nolint:gosec // G304: reading convention files from managed wiki directory
				if err == nil {
					sb.WriteString(string(data))
					sb.WriteString("\n")
				}
			}
		}
	}

	if sb.Len() == 0 {
		sb.WriteString("- Branch naming: `{type}/{task-id}-{description}`\n")
		sb.WriteString("- Commit format: Conventional Commits with task ID reference\n")
		sb.WriteString("- PR template: Include task ID, summary, and testing notes")
	}

	return sb.String(), nil
}

func (g *aiContextGenerator) AssembleGlossary() (string, error) {
	glossaryPath := filepath.Join(g.basePath, "docs", "glossary.md")
	data, err := os.ReadFile(glossaryPath) //nolint:gosec // G304: reading glossary from managed docs directory
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("reading glossary: %w", err)
		}
		// Return default glossary.
		var sb strings.Builder
		sb.WriteString("- **Task**: Unit of work with TASK-XXXXX ID\n")
		sb.WriteString("- **Worktree**: Isolated git working directory for a task\n")
		sb.WriteString("- **ADR**: Architecture Decision Record")
		return sb.String(), nil
	}
	return strings.TrimSpace(string(data)), nil
}

func (g *aiContextGenerator) AssembleActiveTaskSummaries() (string, error) {
	if err := g.backlogMgr.Load(); err != nil {
		return "", fmt.Errorf("assembling active tasks: %w", err)
	}

	// Filter for active tasks (not archived, not done).
	activeStatuses := []models.TaskStatus{
		models.StatusInProgress, models.StatusBlocked,
		models.StatusReview, models.StatusBacklog,
	}
	tasks, err := g.backlogMgr.FilterTasks(storage.BacklogFilter{
		Status: activeStatuses,
	})
	if err != nil {
		return "", fmt.Errorf("assembling active tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No active tasks.", nil
	}

	var sb strings.Builder
	sb.WriteString("| Task ID | Title | Status | Branch |\n")
	sb.WriteString("|---------|-------|--------|--------|\n")
	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			task.ID, task.Title, task.Status, task.Branch))
	}
	return sb.String(), nil
}

func (g *aiContextGenerator) AssembleDecisionsSummary() (string, error) {
	decisionsDir := filepath.Join(g.basePath, "docs", "decisions")
	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No decisions recorded yet.", nil
		}
		return "", fmt.Errorf("assembling decisions: %w", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(decisionsDir, entry.Name())) //nolint:gosec // G304: reading ADR files from managed decisions directory
		if err != nil {
			continue
		}
		content := string(data)
		// Only include accepted ADRs.
		if !strings.Contains(content, "**Status:** Accepted") {
			continue
		}
		// Extract title from first line.
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) > 0 {
			title := strings.TrimPrefix(lines[0], "# ")
			// Extract source task.
			source := ""
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "**Source:**") {
					source = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "**Source:**"))
					break
				}
			}
			if source != "" {
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", title, source))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", title))
			}
		}
	}

	if sb.Len() == 0 {
		return "No decisions recorded yet.", nil
	}
	return sb.String(), nil
}

func (g *aiContextGenerator) assembleStakeholders() string {
	path := filepath.Join(g.basePath, "docs", "stakeholders.md")
	if _, err := os.Stat(path); err == nil {
		return "See [docs/stakeholders.md](docs/stakeholders.md) for outcome owners."
	}
	return "No stakeholders file found."
}

func (g *aiContextGenerator) assembleContacts() string {
	path := filepath.Join(g.basePath, "docs", "contacts.md")
	if _, err := os.Stat(path); err == nil {
		return "See [docs/contacts.md](docs/contacts.md) for subject matter experts."
	}
	return "No contacts file found."
}

// assembleCriticalDecisions reads knowledge/decisions.yaml from active task
// tickets and returns a formatted list of recent decisions.
func (g *aiContextGenerator) assembleCriticalDecisions() (string, error) {
	if err := g.backlogMgr.Load(); err != nil {
		return "", fmt.Errorf("assembling critical decisions: %w", err)
	}

	activeStatuses := []models.TaskStatus{
		models.StatusInProgress, models.StatusBlocked,
		models.StatusReview, models.StatusBacklog,
	}
	tasks, err := g.backlogMgr.FilterTasks(storage.BacklogFilter{
		Status: activeStatuses,
	})
	if err != nil {
		return "", fmt.Errorf("assembling critical decisions: %w", err)
	}

	var sb strings.Builder
	for _, task := range tasks {
		decPath := filepath.Join(g.basePath, "tickets", task.ID, "knowledge", "decisions.yaml")
		data, err := os.ReadFile(decPath) //nolint:gosec // G304: reading decisions from managed ticket directory
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n", task.ID))
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	if sb.Len() == 0 {
		return "No critical decisions recorded in active tasks.", nil
	}
	return sb.String(), nil
}

// assembleRecentSessions reads the latest session files from active task
// tickets and returns a formatted summary.
func (g *aiContextGenerator) assembleRecentSessions() (string, error) {
	if err := g.backlogMgr.Load(); err != nil {
		return "", fmt.Errorf("assembling recent sessions: %w", err)
	}

	activeStatuses := []models.TaskStatus{
		models.StatusInProgress, models.StatusBlocked,
		models.StatusReview,
	}
	tasks, err := g.backlogMgr.FilterTasks(storage.BacklogFilter{
		Status: activeStatuses,
	})
	if err != nil {
		return "", fmt.Errorf("assembling recent sessions: %w", err)
	}

	var sb strings.Builder
	for _, task := range tasks {
		sessionsDir := filepath.Join(g.basePath, "tickets", task.ID, "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil || len(entries) == 0 {
			continue
		}

		// Get the last session file (entries are sorted by name, which is timestamp-based).
		lastEntry := entries[len(entries)-1]
		if lastEntry.IsDir() || !strings.HasSuffix(lastEntry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessionsDir, lastEntry.Name())) //nolint:gosec // G304: reading session files from managed ticket directory
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s (latest: %s)\n\n", task.ID, lastEntry.Name()))
		// Include only the first 20 lines to keep context concise.
		lines := strings.SplitN(content, "\n", 21)
		if len(lines) > 20 {
			sb.WriteString(strings.Join(lines[:20], "\n"))
			sb.WriteString("\n...(truncated)\n")
		} else {
			sb.WriteString(content)
		}
		sb.WriteString("\n\n")
	}

	if sb.Len() == 0 {
		return "No recent sessions recorded.", nil
	}
	return sb.String(), nil
}

func (g *aiContextGenerator) renderContextFile() string {
	now := time.Now().Format(time.RFC3339)

	var sb strings.Builder
	sb.WriteString("# AI Dev Brain Context\n\n")
	sb.WriteString(fmt.Sprintf("> Auto-generated context file for AI coding assistants. Last updated: %s\n\n", now))

	sb.WriteString("## Project Overview\n\n")
	sb.WriteString(g.sections.Overview)
	sb.WriteString("\n\n")

	sb.WriteString("## Directory Structure\n\n")
	sb.WriteString(g.sections.DirectoryStructure)
	sb.WriteString("\n\n")

	sb.WriteString("## Key Conventions\n\n")
	sb.WriteString(g.sections.Conventions)
	sb.WriteString("\n\n")

	sb.WriteString("## Glossary\n\n")
	sb.WriteString(g.sections.Glossary)
	sb.WriteString("\n\n")

	sb.WriteString("## Active Decisions Summary\n\n")
	sb.WriteString(g.sections.DecisionsSummary)
	sb.WriteString("\n\n")

	sb.WriteString("## Active Tasks\n\n")
	sb.WriteString(g.sections.ActiveTaskSummaries)
	sb.WriteString("\n\n")

	sb.WriteString("## Critical Decisions\n\n")
	sb.WriteString(g.sections.CriticalDecisions)
	sb.WriteString("\n\n")

	sb.WriteString("## Recent Sessions\n\n")
	sb.WriteString(g.sections.RecentSessions)
	sb.WriteString("\n\n")

	sb.WriteString("## Key Contacts\n\n")
	sb.WriteString(g.sections.StakeholderLinks)
	sb.WriteString("\n")
	sb.WriteString(g.sections.ContactLinks)
	sb.WriteString("\n\n")

	sb.WriteString("---\n")
	sb.WriteString("*Run `adb sync-context` to regenerate this file.*\n")

	return sb.String()
}

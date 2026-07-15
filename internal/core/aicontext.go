package core

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// AIContextGenerator generates comprehensive CLAUDE.md context file
type AIContextGenerator interface {
	Generate() error
}

// DefaultAIContextGenerator implements AIContextGenerator
type DefaultAIContextGenerator struct {
	repoRoot       string
	backlogManager storage.BacklogManager
}

// NewAIContextGenerator creates a new AI context generator
func NewAIContextGenerator(repoRoot string, backlogManager storage.BacklogManager) AIContextGenerator {
	return &DefaultAIContextGenerator{
		repoRoot:       repoRoot,
		backlogManager: backlogManager,
	}
}

// ContextState tracks section hashes for change detection
type ContextState struct {
	LastGenerated time.Time         `yaml:"last_generated"`
	SectionHashes map[string]string `yaml:"section_hashes"`
}

// Generate creates the comprehensive CLAUDE.md file
func (g *DefaultAIContextGenerator) Generate() error {
	// Load previous state
	prevState, err := g.loadContextState()
	if err != nil {
		// If state doesn't exist, create empty one
		prevState = &ContextState{
			SectionHashes: make(map[string]string),
		}
	}

	// Generate all sections
	sections := make(map[string]string)
	currentHashes := make(map[string]string)

	sections["overview"] = g.generateOverview()
	currentHashes["overview"] = g.hashContent(sections["overview"])

	sections["directory"] = g.generateDirectoryStructure()
	currentHashes["directory"] = g.hashContent(sections["directory"])

	sections["conventions"] = g.generateConventions()
	currentHashes["conventions"] = g.hashContent(sections["conventions"])

	sections["glossary"] = g.generateGlossary()
	currentHashes["glossary"] = g.hashContent(sections["glossary"])

	sections["decisions"] = g.generateDecisionsSummary()
	currentHashes["decisions"] = g.hashContent(sections["decisions"])

	sections["active_tasks"] = g.generateActiveTasks()
	currentHashes["active_tasks"] = g.hashContent(sections["active_tasks"])

	sections["critical_decisions"] = g.generateCriticalDecisions()
	currentHashes["critical_decisions"] = g.hashContent(sections["critical_decisions"])

	sections["recent_sessions"] = g.generateRecentSessions()
	currentHashes["recent_sessions"] = g.hashContent(sections["recent_sessions"])

	sections["captured_sessions"] = g.generateCapturedSessions()
	currentHashes["captured_sessions"] = g.hashContent(sections["captured_sessions"])

	sections["stakeholders"] = g.generateStakeholders()
	currentHashes["stakeholders"] = g.hashContent(sections["stakeholders"])

	// Generate "What's Changed" section by comparing hashes
	sections["whats_changed"] = g.generateWhatsChanged(prevState.SectionHashes, currentHashes)

	// Assemble CLAUDE.md
	content := g.assembleCLAUDEmd(sections)

	// Write CLAUDE.md
	claudePath := filepath.Join(g.repoRoot, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	// Save new state
	newState := &ContextState{
		LastGenerated: time.Now().UTC(),
		SectionHashes: currentHashes,
	}
	if err := g.saveContextState(newState); err != nil {
		return fmt.Errorf("failed to save context state: %w", err)
	}

	return nil
}

func (g *DefaultAIContextGenerator) generateOverview() string {
	return `# AI Dev Brain - Claude Context

This workspace is managed by **AI Dev Brain (adb)**, a task management system for AI-assisted development.

## What is AI Dev Brain?

AI Dev Brain is a comprehensive task management and workflow orchestration system designed specifically for AI-assisted software development. It provides:

- **Task Management**: Structured backlog with status tracking, priorities, and dependencies
- **Context Generation**: Automatic generation of context files for AI agents
- **Git Integration**: Per-task worktrees for isolated development
- **Knowledge Capture**: Structured capture of decisions, learnings, and gotchas
- **Session Management**: Recording and replay of development sessions
- **Hook System**: Extensible hooks for workflow automation
- **Team Coordination**: Multi-team routing and notification support

## How to Use This Context

This document provides comprehensive context about the current state of the project. It is automatically generated and updated by the adb system. Sections include:

- **Directory Structure**: Overview of the repository layout
- **Conventions**: Coding and process conventions from documentation
- **Glossary**: Key terms and definitions
- **Decisions**: Summary of architectural and design decisions
- **Active Tasks**: Currently in-progress work
- **Critical Decisions**: Recent decisions from active task contexts
- **Recent Sessions**: Latest development session notes
- **What's Changed**: Summary of changes since last context generation
- **Stakeholders**: Key contacts and stakeholders
`
}

func (g *DefaultAIContextGenerator) generateDirectoryStructure() string {
	return `
## Directory Structure

` + "```" + `
.
├── cmd/                    # Command-line interface
│   └── adb/               # Main adb executable
├── internal/              # Internal packages (not importable)
│   ├── core/             # Core business logic
│   ├── storage/          # Storage implementations
│   └── cli/              # CLI command handlers
├── pkg/                   # Public packages (importable)
│   └── models/           # Domain models and types
├── docs/                  # Documentation
│   ├── wiki/             # Wiki pages (conventions, guides)
│   ├── decisions/        # Architectural decision records (ADRs)
│   ├── glossary.md       # Terminology glossary
│   ├── stakeholders.md   # Stakeholder information
│   └── contacts.md       # Contact information
├── templates/             # Template files
├── tickets/               # Per-task context and artifacts
│   └── TASK-XXX/         # Task-specific directory
│       ├── context.md    # Task context
│       ├── notes.md      # Developer notes
│       ├── sessions/     # Session recordings
│       └── knowledge/    # Captured knowledge
│           └── decisions.yaml
├── sessions/              # Captured session store
├── work/                  # Git worktrees (task isolation)
├── backlog.yaml          # Task backlog
├── .context_state.yaml   # Context generation state
└── CLAUDE.md             # This file
` + "```" + `
`
}

func (g *DefaultAIContextGenerator) generateConventions() string {
	var sb strings.Builder
	sb.WriteString("\n## Conventions\n\n")

	// Look for convention files in docs/wiki
	conventionFiles, err := filepath.Glob(filepath.Join(g.repoRoot, "docs", "wiki", "*convention*.md"))
	if err != nil || len(conventionFiles) == 0 {
		sb.WriteString("_No convention documents found._\n")
		return sb.String()
	}

	for _, file := range conventionFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		basename := filepath.Base(file)
		sb.WriteString(fmt.Sprintf("### %s\n\n", basename))
		sb.WriteString(string(content))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateGlossary() string {
	var sb strings.Builder
	sb.WriteString("\n## Glossary\n\n")

	glossaryPath := filepath.Join(g.repoRoot, "docs", "glossary.md")
	content, err := os.ReadFile(glossaryPath)
	if err != nil {
		sb.WriteString("_No glossary found._\n")
		return sb.String()
	}

	sb.WriteString(string(content))
	sb.WriteString("\n")
	return sb.String()
}

func (g *DefaultAIContextGenerator) generateDecisionsSummary() string {
	var sb strings.Builder
	sb.WriteString("\n## Architectural Decisions\n\n")

	decisionsDir := filepath.Join(g.repoRoot, "docs", "decisions")
	decisionFiles, err := filepath.Glob(filepath.Join(decisionsDir, "*.md"))
	if err != nil || len(decisionFiles) == 0 {
		sb.WriteString("_No decision documents found._\n")
		return sb.String()
	}

	// Sort by filename (reverse order to get newest first)
	sort.Slice(decisionFiles, func(i, j int) bool {
		return decisionFiles[i] > decisionFiles[j]
	})

	for _, file := range decisionFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		// Only include accepted ADRs
		contentStr := string(content)
		if !strings.Contains(strings.ToLower(contentStr), "status: accepted") &&
			!strings.Contains(strings.ToLower(contentStr), "status:accepted") {
			continue
		}

		basename := filepath.Base(file)
		sb.WriteString(fmt.Sprintf("### %s\n\n", basename))

		// Extract title and status if present
		lines := strings.Split(contentStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") {
				sb.WriteString(line)
				sb.WriteString("\n\n")
				break
			}
		}
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateActiveTasks() string {
	var sb strings.Builder
	sb.WriteString("\n## Active Tasks\n\n")

	backlog, err := g.backlogManager.Load()
	if err != nil {
		sb.WriteString("_Failed to load backlog._\n")
		return sb.String()
	}

	// Filter for active tasks
	activeTasks := []models.Task{}
	for _, task := range backlog.Tasks {
		if task.IsActive() {
			activeTasks = append(activeTasks, task)
		}
	}

	if len(activeTasks) == 0 {
		sb.WriteString("_No active tasks._\n")
		return sb.String()
	}

	// Sort by priority and status
	sort.Slice(activeTasks, func(i, j int) bool {
		if activeTasks[i].Priority != activeTasks[j].Priority {
			return activeTasks[i].Priority < activeTasks[j].Priority
		}
		return activeTasks[i].Status < activeTasks[j].Status
	})

	for _, task := range activeTasks {
		sb.WriteString(fmt.Sprintf("### %s: %s\n\n", task.ID, task.Title))
		sb.WriteString(fmt.Sprintf("- **Status**: %s\n", task.Status))
		sb.WriteString(fmt.Sprintf("- **Priority**: %s\n", task.Priority))
		sb.WriteString(fmt.Sprintf("- **Type**: %s\n", task.Type))
		if task.Owner != "" {
			sb.WriteString(fmt.Sprintf("- **Owner**: %s\n", task.Owner))
		}
		if len(task.BlockedBy) > 0 {
			sb.WriteString(fmt.Sprintf("- **Blocked By**: %s\n", strings.Join(task.BlockedBy, ", ")))
		}
		if len(task.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("- **Tags**: %s\n", strings.Join(task.Tags, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateCriticalDecisions() string {
	var sb strings.Builder
	sb.WriteString("\n## Critical Decisions (from Active Tasks)\n\n")

	backlog, err := g.backlogManager.Load()
	if err != nil {
		sb.WriteString("_Failed to load backlog._\n")
		return sb.String()
	}

	foundDecisions := false

	// Get active tasks
	for _, task := range backlog.Tasks {
		if !task.IsActive() {
			continue
		}

		// Load decisions for this task
		decisionsPath := filepath.Join(g.repoRoot, "tickets", task.ID, "knowledge", "decisions.yaml")
		decisions, err := g.loadDecisions(decisionsPath)
		if err != nil {
			continue
		}

		if len(decisions) > 0 {
			foundDecisions = true
			sb.WriteString(fmt.Sprintf("### %s: %s\n\n", task.ID, task.Title))

			for _, dec := range decisions {
				sb.WriteString(fmt.Sprintf("#### %s (%s)\n\n", dec.Title, dec.Status))
				sb.WriteString(fmt.Sprintf("%s\n\n", dec.Description))
				if dec.Rationale != "" {
					sb.WriteString(fmt.Sprintf("**Rationale**: %s\n\n", dec.Rationale))
				}
			}
		}
	}

	if !foundDecisions {
		sb.WriteString("_No critical decisions found in active tasks._\n")
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateRecentSessions() string {
	var sb strings.Builder
	sb.WriteString("\n## Recent Sessions\n\n")

	// Find all session files
	ticketsDir := filepath.Join(g.repoRoot, "tickets")
	sessionFiles := []string{}

	// Walk through tickets directory
	err := filepath.Walk(ticketsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on error
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") && strings.Contains(path, "sessions") {
			sessionFiles = append(sessionFiles, path)
		}
		return nil
	})

	if err != nil || len(sessionFiles) == 0 {
		sb.WriteString("_No recent sessions found._\n")
		return sb.String()
	}

	// Sort by modification time (newest first)
	sort.Slice(sessionFiles, func(i, j int) bool {
		iInfo, _ := os.Stat(sessionFiles[i])
		jInfo, _ := os.Stat(sessionFiles[j])
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	// Take up to 5 most recent sessions
	limit := 5
	if len(sessionFiles) < limit {
		limit = len(sessionFiles)
	}

	for i := 0; i < limit; i++ {
		file := sessionFiles[i]
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		// Truncate to 20 lines
		lines := strings.Split(string(content), "\n")
		if len(lines) > 20 {
			lines = lines[:20]
		}

		relPath, _ := filepath.Rel(g.repoRoot, file)
		// Emit forward slashes so generated markdown is portable — a
		// Windows-generated CLAUDE.md committed to git shouldn't
		// surface `tickets\TASK-001\sessions\session.md` when all other
		// consumers (wiki, diffs, LLMs) expect `/` separators.
		sb.WriteString(fmt.Sprintf("### %s\n\n", filepath.ToSlash(relPath)))
		sb.WriteString(strings.Join(lines, "\n"))
		if len(strings.Split(string(content), "\n")) > 20 {
			sb.WriteString("\n\n_[truncated]_\n")
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateCapturedSessions() string {
	var sb strings.Builder
	sb.WriteString("\n## Captured Sessions\n\n")

	sessionsDir := filepath.Join(g.repoRoot, "sessions")
	sessionFiles, err := filepath.Glob(filepath.Join(sessionsDir, "*.yaml"))
	if err != nil || len(sessionFiles) == 0 {
		sb.WriteString("_No captured sessions found._\n")
		return sb.String()
	}

	// Sort by filename (reverse order for newest first)
	sort.Slice(sessionFiles, func(i, j int) bool {
		return sessionFiles[i] > sessionFiles[j]
	})

	// List up to 10 most recent sessions
	limit := 10
	if len(sessionFiles) < limit {
		limit = len(sessionFiles)
	}

	for i := 0; i < limit; i++ {
		basename := filepath.Base(sessionFiles[i])
		sb.WriteString(fmt.Sprintf("- %s\n", basename))
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateStakeholders() string {
	var sb strings.Builder
	sb.WriteString("\n## Stakeholders & Contacts\n\n")

	// Read stakeholders
	stakeholdersPath := filepath.Join(g.repoRoot, "docs", "stakeholders.md")
	if content, err := os.ReadFile(stakeholdersPath); err == nil {
		sb.WriteString("### Stakeholders\n\n")
		sb.WriteString(string(content))
		sb.WriteString("\n\n")
	}

	// Read contacts
	contactsPath := filepath.Join(g.repoRoot, "docs", "contacts.md")
	if content, err := os.ReadFile(contactsPath); err == nil {
		sb.WriteString("### Contacts\n\n")
		sb.WriteString(string(content))
		sb.WriteString("\n")
	}

	if sb.Len() == len("\n## Stakeholders & Contacts\n\n") {
		sb.WriteString("_No stakeholder or contact information found._\n")
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) generateWhatsChanged(prevHashes, currentHashes map[string]string) string {
	var sb strings.Builder
	sb.WriteString("\n## What's Changed\n\n")

	changes := []string{}

	// Check each section for changes
	sectionNames := map[string]string{
		"overview":           "Project Overview",
		"directory":          "Directory Structure",
		"conventions":        "Conventions",
		"glossary":           "Glossary",
		"decisions":          "Architectural Decisions",
		"active_tasks":       "Active Tasks",
		"critical_decisions": "Critical Decisions",
		"recent_sessions":    "Recent Sessions",
		"captured_sessions":  "Captured Sessions",
		"stakeholders":       "Stakeholders & Contacts",
	}

	for key, name := range sectionNames {
		prevHash, hadPrev := prevHashes[key]
		currHash, hasCurr := currentHashes[key]

		if !hadPrev && hasCurr {
			changes = append(changes, fmt.Sprintf("- **%s**: New section added", name))
		} else if hadPrev && hasCurr && prevHash != currHash {
			changes = append(changes, fmt.Sprintf("- **%s**: Updated", name))
		}
	}

	if len(changes) == 0 {
		sb.WriteString("_No changes detected since last generation._\n")
	} else {
		sb.WriteString("Changes detected since last context generation:\n\n")
		for _, change := range changes {
			sb.WriteString(change)
			sb.WriteString("\n")
		}
	}

	// Add task-specific changes
	backlog, err := g.backlogManager.Load()
	if err == nil {
		activeCount := 0
		for _, task := range backlog.Tasks {
			if task.IsActive() {
				activeCount++
			}
		}
		sb.WriteString(fmt.Sprintf("\n**Summary**: %d active task(s)\n", activeCount))
	}

	return sb.String()
}

func (g *DefaultAIContextGenerator) assembleCLAUDEmd(sections map[string]string) string {
	var sb strings.Builder

	// Assemble in order
	sb.WriteString(sections["overview"])
	sb.WriteString(sections["whats_changed"])
	sb.WriteString(sections["directory"])
	sb.WriteString(sections["conventions"])
	sb.WriteString(sections["glossary"])
	sb.WriteString(sections["decisions"])
	sb.WriteString(sections["active_tasks"])
	sb.WriteString(sections["critical_decisions"])
	sb.WriteString(sections["recent_sessions"])
	sb.WriteString(sections["captured_sessions"])
	sb.WriteString(sections["stakeholders"])

	// Footer
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("_Generated by AI Dev Brain on %s_\n", time.Now().UTC().Format(time.RFC3339)))

	return sb.String()
}

func (g *DefaultAIContextGenerator) hashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (g *DefaultAIContextGenerator) loadContextState() (*ContextState, error) {
	statePath := statedir.Path(g.repoRoot, statedir.FileContextState)
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	var state ContextState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.SectionHashes == nil {
		state.SectionHashes = make(map[string]string)
	}

	return &state, nil
}

func (g *DefaultAIContextGenerator) saveContextState(state *ContextState) error {
	statePath := statedir.Path(g.repoRoot, statedir.FileContextState)
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	// Ensure the .adb/ state dir exists (#186) — this generator can be
	// constructed and run without going through NewApp's ensureStateDir.
	if err := statedir.Ensure(g.repoRoot); err != nil {
		return err
	}
	return os.WriteFile(statePath, data, 0o644)
}

func (g *DefaultAIContextGenerator) loadDecisions(path string) ([]models.Decision, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var decisions struct {
		Decisions []models.Decision `yaml:"decisions"`
	}

	if err := yaml.Unmarshal(data, &decisions); err != nil {
		return nil, err
	}

	return decisions.Decisions, nil
}

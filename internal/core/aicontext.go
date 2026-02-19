package core

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
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
	KnowledgeSummary    string
	StakeholderLinks    string
	ContactLinks        string
}

// ContextState captures a snapshot of the assembled context for change detection.
type ContextState struct {
	SyncedAt       time.Time         `yaml:"synced_at"`
	ActiveTaskIDs  []string          `yaml:"active_task_ids"`
	KnowledgeCount int               `yaml:"knowledge_count"`
	DecisionCount  int               `yaml:"decision_count"`
	SessionCount   int               `yaml:"session_count"`
	ADRTitles      []string          `yaml:"adr_titles"`
	SectionHashes  map[string]string `yaml:"section_hashes"`
}

// ContextChange describes a single difference between two context states.
type ContextChange struct {
	Section     string
	Description string
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
	basePath       string
	backlogMgr     storage.BacklogManager
	knowledgeMgr   KnowledgeManager
	sessionCapture SessionCapturer // may be nil
	sections       AIContextSections
}

// NewAIContextGenerator creates a new AIContextGenerator.
// knowledgeMgr may be nil if knowledge features are not available.
// sessionCapture may be nil if session capture features are not available.
func NewAIContextGenerator(basePath string, backlogMgr storage.BacklogManager, knowledgeMgr KnowledgeManager, sessionCapture SessionCapturer) AIContextGenerator {
	return &aiContextGenerator{
		basePath:       basePath,
		backlogMgr:     backlogMgr,
		knowledgeMgr:   knowledgeMgr,
		sessionCapture: sessionCapture,
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

	// Load previous state for evolution tracking.
	prevState, _ := g.loadState()

	// Compute current state from assembled sections.
	currState := g.computeCurrentState()

	// Diff states to detect changes.
	changes := g.diffStates(prevState, currState)

	// Render the "What's Changed" section.
	var prevSyncedAt time.Time
	if prevState != nil {
		prevSyncedAt = prevState.SyncedAt
	}
	whatsChanged := g.renderWhatsChanged(changes, prevSyncedAt)

	// Generate both CLAUDE.md and kiro.md with the changes section injected.
	for _, aiType := range []AIType{AITypeClaude, AITypeKiro} {
		content := g.renderContextFileWithChanges(whatsChanged)
		path := filepath.Join(g.basePath, g.filenameForAI(aiType))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("syncing context: writing %s: %w", path, err)
		}
	}

	// Append changelog entry.
	syncedAt := time.Now().UTC()
	_ = g.appendChangelog(changes, syncedAt)

	// Save current state for next diff.
	currState.SyncedAt = syncedAt
	if err := g.saveState(currState); err != nil {
		return fmt.Errorf("syncing context: saving state: %w", err)
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
	g.sections.KnowledgeSummary = g.assembleKnowledgeSummary()
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

	// Append captured sessions if available.
	if g.sessionCapture != nil {
		captured, err := g.sessionCapture.GetRecentSessions(5)
		if err == nil {
			for _, s := range captured {
				label := s.ProjectPath
				if s.TaskID != "" {
					label = s.TaskID
				}
				sb.WriteString(fmt.Sprintf("### %s (captured: %s, %d turns)\n\n",
					label,
					s.StartedAt.Format("2006-01-02T15:04:05Z"),
					s.TurnCount,
				))
				if s.Summary != "" {
					sb.WriteString(s.Summary)
				} else {
					sb.WriteString(fmt.Sprintf("Session in `%s`", s.ProjectPath))
					if s.Duration != "" {
						sb.WriteString(fmt.Sprintf(", duration: %s", s.Duration))
					}
				}
				sb.WriteString("\n\n")
			}
		}
	}

	if sb.Len() == 0 {
		return "No recent sessions recorded.", nil
	}
	return sb.String(), nil
}

// assembleKnowledgeSummary queries the KnowledgeManager for accumulated
// knowledge and returns a markdown summary. Returns empty string if the
// knowledge manager is not available or the store is empty.
func (g *aiContextGenerator) assembleKnowledgeSummary() string {
	if g.knowledgeMgr == nil {
		return ""
	}
	summary, err := g.knowledgeMgr.AssembleKnowledgeSummary(20)
	if err != nil {
		return ""
	}
	return summary
}

func (g *aiContextGenerator) renderContextFile() string {
	return g.renderContextFileWithChanges("")
}

// renderContextFileWithChanges renders the full context file, optionally
// injecting a "What's Changed" section after the header and before "## Project Overview".
func (g *aiContextGenerator) renderContextFileWithChanges(whatsChanged string) string {
	now := time.Now().Format(time.RFC3339)

	var sb strings.Builder
	sb.WriteString("# AI Dev Brain Context\n\n")
	sb.WriteString(fmt.Sprintf("> Auto-generated context file for AI coding assistants. Last updated: %s\n\n", now))

	if whatsChanged != "" {
		sb.WriteString(whatsChanged)
		sb.WriteString("\n\n")
	}

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

	if g.sections.KnowledgeSummary != "" {
		sb.WriteString("## Accumulated Knowledge\n\n")
		sb.WriteString(g.sections.KnowledgeSummary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Key Contacts\n\n")
	sb.WriteString(g.sections.StakeholderLinks)
	sb.WriteString("\n")
	sb.WriteString(g.sections.ContactLinks)
	sb.WriteString("\n\n")

	sb.WriteString("---\n")
	sb.WriteString("*Run `adb sync-context` to regenerate this file.*\n")

	return sb.String()
}

// loadState reads the context state from .context_state.yaml.
// Returns nil if the file does not exist (first sync).
func (g *aiContextGenerator) loadState() (*ContextState, error) {
	path := filepath.Join(g.basePath, ".context_state.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // G304: reading managed state file
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading context state: %w", err)
	}

	var state ContextState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("loading context state: parsing YAML: %w", err)
	}
	return &state, nil
}

// saveState writes the context state to .context_state.yaml atomically.
func (g *aiContextGenerator) saveState(state *ContextState) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("saving context state: marshaling YAML: %w", err)
	}

	path := filepath.Join(g.basePath, ".context_state.yaml")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("saving context state: writing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("saving context state: renaming: %w", err)
	}
	return nil
}

// computeCurrentState derives a ContextState from the currently assembled sections.
func (g *aiContextGenerator) computeCurrentState() *ContextState {
	state := &ContextState{
		SectionHashes: make(map[string]string),
	}

	// Count active tasks by parsing the assembled table.
	var taskIDs []string
	for _, line := range strings.Split(g.sections.ActiveTaskSummaries, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "|--") || strings.HasPrefix(line, "| Task ID") {
			continue
		}
		if strings.HasPrefix(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				id := strings.TrimSpace(parts[1])
				if id != "" && id != "Task ID" {
					taskIDs = append(taskIDs, id)
				}
			}
		}
	}
	sort.Strings(taskIDs)
	state.ActiveTaskIDs = taskIDs

	// Count knowledge entries from knowledge summary lines.
	if g.sections.KnowledgeSummary != "" {
		for _, line := range strings.Split(g.sections.KnowledgeSummary, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "- **") {
				state.KnowledgeCount++
			}
		}
	}

	// Count decisions from critical decisions section (### headers).
	for _, line := range strings.Split(g.sections.CriticalDecisions, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- decision:") ||
			strings.HasPrefix(strings.TrimSpace(line), "decision:") {
			state.DecisionCount++
		}
	}

	// Count sessions from recent sessions section (### headers).
	for _, line := range strings.Split(g.sections.RecentSessions, "\n") {
		if strings.HasPrefix(line, "### ") {
			state.SessionCount++
		}
	}

	// Extract ADR titles from decisions summary.
	var adrTitles []string
	for _, line := range strings.Split(g.sections.DecisionsSummary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			title := strings.TrimPrefix(line, "- ")
			if title != "" {
				adrTitles = append(adrTitles, title)
			}
		}
	}
	sort.Strings(adrTitles)
	state.ADRTitles = adrTitles

	// Hash each section for change detection.
	state.SectionHashes["overview"] = g.hashSection(g.sections.Overview)
	state.SectionHashes["directory_structure"] = g.hashSection(g.sections.DirectoryStructure)
	state.SectionHashes["conventions"] = g.hashSection(g.sections.Conventions)
	state.SectionHashes["glossary"] = g.hashSection(g.sections.Glossary)
	state.SectionHashes["decisions_summary"] = g.hashSection(g.sections.DecisionsSummary)
	state.SectionHashes["active_tasks"] = g.hashSection(g.sections.ActiveTaskSummaries)
	state.SectionHashes["critical_decisions"] = g.hashSection(g.sections.CriticalDecisions)
	state.SectionHashes["recent_sessions"] = g.hashSection(g.sections.RecentSessions)
	state.SectionHashes["knowledge_summary"] = g.hashSection(g.sections.KnowledgeSummary)
	state.SectionHashes["stakeholders"] = g.hashSection(g.sections.StakeholderLinks)
	state.SectionHashes["contacts"] = g.hashSection(g.sections.ContactLinks)

	return state
}

// diffStates compares two context states and produces human-readable changes.
// If prev is nil, this is treated as the first sync.
func (g *aiContextGenerator) diffStates(prev, curr *ContextState) []ContextChange {
	if prev == nil {
		return nil
	}

	var changes []ContextChange

	// Compare active task lists.
	prevTasks := make(map[string]bool, len(prev.ActiveTaskIDs))
	for _, id := range prev.ActiveTaskIDs {
		prevTasks[id] = true
	}
	currTasks := make(map[string]bool, len(curr.ActiveTaskIDs))
	for _, id := range curr.ActiveTaskIDs {
		currTasks[id] = true
	}
	for _, id := range curr.ActiveTaskIDs {
		if !prevTasks[id] {
			changes = append(changes, ContextChange{
				Section:     "Active Tasks",
				Description: fmt.Sprintf("Task %s added", id),
			})
		}
	}
	for _, id := range prev.ActiveTaskIDs {
		if !currTasks[id] {
			changes = append(changes, ContextChange{
				Section:     "Active Tasks",
				Description: fmt.Sprintf("Task %s removed", id),
			})
		}
	}

	// Knowledge count delta.
	if delta := curr.KnowledgeCount - prev.KnowledgeCount; delta != 0 {
		if delta > 0 {
			changes = append(changes, ContextChange{
				Section:     "Knowledge",
				Description: fmt.Sprintf("%d new knowledge entry(s) added", delta),
			})
		} else {
			changes = append(changes, ContextChange{
				Section:     "Knowledge",
				Description: fmt.Sprintf("%d knowledge entry(s) removed", -delta),
			})
		}
	}

	// Decision count delta.
	if delta := curr.DecisionCount - prev.DecisionCount; delta != 0 {
		if delta > 0 {
			changes = append(changes, ContextChange{
				Section:     "Critical Decisions",
				Description: fmt.Sprintf("%d new decision(s) recorded", delta),
			})
		} else {
			changes = append(changes, ContextChange{
				Section:     "Critical Decisions",
				Description: fmt.Sprintf("%d decision(s) removed", -delta),
			})
		}
	}

	// Session count delta.
	if delta := curr.SessionCount - prev.SessionCount; delta != 0 {
		if delta > 0 {
			changes = append(changes, ContextChange{
				Section:     "Recent Sessions",
				Description: fmt.Sprintf("%d new session(s) recorded", delta),
			})
		} else {
			changes = append(changes, ContextChange{
				Section:     "Recent Sessions",
				Description: fmt.Sprintf("%d session(s) removed", -delta),
			})
		}
	}

	// ADR additions.
	prevADRs := make(map[string]bool, len(prev.ADRTitles))
	for _, t := range prev.ADRTitles {
		prevADRs[t] = true
	}
	for _, t := range curr.ADRTitles {
		if !prevADRs[t] {
			changes = append(changes, ContextChange{
				Section:     "Decisions Summary",
				Description: fmt.Sprintf("New ADR: %s", t),
			})
		}
	}

	// Section hash changes for remaining sections.
	sectionNames := map[string]string{
		"overview":            "Project Overview",
		"directory_structure": "Directory Structure",
		"conventions":         "Key Conventions",
		"glossary":            "Glossary",
		"stakeholders":        "Stakeholders",
		"contacts":            "Contacts",
	}
	for key, name := range sectionNames {
		prevHash := prev.SectionHashes[key]
		currHash := curr.SectionHashes[key]
		if prevHash != currHash {
			changes = append(changes, ContextChange{
				Section:     name,
				Description: fmt.Sprintf("%s section updated", name),
			})
		}
	}

	return changes
}

// renderWhatsChanged renders a markdown section summarizing what changed.
func (g *aiContextGenerator) renderWhatsChanged(changes []ContextChange, prevSyncedAt time.Time) string {
	if changes == nil {
		return "## What's Changed\n\nFirst context generation -- no previous state to compare."
	}

	if len(changes) == 0 {
		sinceStr := prevSyncedAt.Format(time.RFC3339)
		return fmt.Sprintf("## What's Changed (since %s)\n\nNo changes since last sync.", sinceStr)
	}

	sinceStr := prevSyncedAt.Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## What's Changed (since %s)\n\n", sinceStr))
	for _, c := range changes {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Section, c.Description))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// appendChangelog prepends a changelog entry to .context_changelog.md.
// If the file has more than 50 entries, the oldest are pruned.
func (g *aiContextGenerator) appendChangelog(changes []ContextChange, syncedAt time.Time) error {
	path := filepath.Join(g.basePath, ".context_changelog.md")

	// Build the new entry.
	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("## %s\n\n", syncedAt.Format(time.RFC3339)))
	if changes == nil || len(changes) == 0 {
		entry.WriteString("No changes detected.\n")
	} else {
		for _, c := range changes {
			entry.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Section, c.Description))
		}
	}

	// Read existing content.
	existing, _ := os.ReadFile(path) //nolint:gosec // G304: reading managed changelog file
	existingStr := string(existing)

	// Strip the file header if present.
	const header = "# Context Changelog\n\n"
	body := existingStr
	if strings.HasPrefix(body, header) {
		body = body[len(header):]
	}

	// Prepend the new entry.
	newContent := header + entry.String() + "\n" + body

	// Prune to 50 entries by counting "## " timestamp headers.
	entries := splitChangelogEntries(newContent)
	if len(entries.body) > 50 {
		entries.body = entries.body[:50]
	}
	newContent = entries.header
	for _, e := range entries.body {
		newContent += e
	}

	return os.WriteFile(path, []byte(newContent), 0o644)
}

type changelogParts struct {
	header string
	body   []string
}

// splitChangelogEntries splits a changelog into its header and individual entries.
func splitChangelogEntries(content string) changelogParts {
	const header = "# Context Changelog\n\n"
	result := changelogParts{header: header}

	body := content
	if strings.HasPrefix(body, header) {
		body = body[len(header):]
	}

	// Split on "## " at the start of a line.
	var current strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") && current.Len() > 0 {
			result.body = append(result.body, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		text := current.String()
		if strings.TrimSpace(text) != "" {
			result.body = append(result.body, text)
		}
	}

	return result
}

// hashSection computes an FNV-1a 32-bit hash of the content, formatted as hex.
func (g *aiContextGenerator) hashSection(content string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(content))
	return fmt.Sprintf("%08x", h.Sum32())
}

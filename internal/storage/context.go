package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TaskContext represents the persistent context for a task's AI session.
type TaskContext struct {
	TaskID         string                 `yaml:"task_id"`
	Notes          string                 `yaml:"notes"`
	Context        string                 `yaml:"context"`
	Communications []models.Communication `yaml:"communications"`
	LastUpdated    time.Time              `yaml:"last_updated"`
}

// AIContext is the summarized context assembled for AI assistant consumption.
type AIContext struct {
	Summary        string   `yaml:"summary"`
	RecentActivity []string `yaml:"recent_activity"`
	OpenQuestions  []string `yaml:"open_questions"`
	Decisions      []string `yaml:"decisions"`
	Blockers       []string `yaml:"blockers"`
}

// ContextManager manages persistent context for AI session continuity.
type ContextManager interface {
	InitializeContext(taskID string) (*TaskContext, error)
	LoadContext(taskID string) (*TaskContext, error)
	UpdateContext(taskID string, updates map[string]interface{}) error
	PersistContext(taskID string) error
	GetContextForAI(taskID string) (*AIContext, error)
}

type fileContextManager struct {
	basePath string
	contexts map[string]*TaskContext
}

// NewContextManager creates a new ContextManager that stores context
// in the tickets/ directory under the given base path.
func NewContextManager(basePath string) ContextManager {
	return &fileContextManager{
		basePath: basePath,
		contexts: make(map[string]*TaskContext),
	}
}

func (m *fileContextManager) ticketDir(taskID string) string {
	return resolveTicketDir(m.basePath, taskID)
}

func (m *fileContextManager) contextPath(taskID string) string {
	return filepath.Join(m.ticketDir(taskID), "context.md")
}

func (m *fileContextManager) notesPath(taskID string) string {
	return filepath.Join(m.ticketDir(taskID), "notes.md")
}

func (m *fileContextManager) commsDir(taskID string) string {
	return filepath.Join(m.ticketDir(taskID), "communications")
}

const contextTemplate = `# Task Context: %s

## Summary
[AI-maintained summary of current task state]

## Current Focus
[What we're working on right now]

## Recent Progress
- [Chronological list of completed items]

## Open Questions
- [ ] [Questions needing answers]

## Decisions Made
- [Key decisions with rationale]

## Blockers
- [Current blockers and who can help]

## Next Steps
- [ ] [Planned next actions]

## Related Resources
- [Links to relevant docs, PRs, etc.]
`

const notesTemplate = `# Notes: %s

`

func (m *fileContextManager) InitializeContext(taskID string) (*TaskContext, error) {
	dir := m.ticketDir(taskID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("initializing context for %s: creating ticket dir: %w", taskID, err)
	}
	if err := os.MkdirAll(m.commsDir(taskID), 0o750); err != nil {
		return nil, fmt.Errorf("initializing context for %s: creating communications dir: %w", taskID, err)
	}

	contextContent := fmt.Sprintf(contextTemplate, taskID)
	if err := os.WriteFile(m.contextPath(taskID), []byte(contextContent), 0o600); err != nil {
		return nil, fmt.Errorf("initializing context for %s: writing context.md: %w", taskID, err)
	}

	notesContent := fmt.Sprintf(notesTemplate, taskID)
	if err := os.WriteFile(m.notesPath(taskID), []byte(notesContent), 0o600); err != nil {
		return nil, fmt.Errorf("initializing context for %s: writing notes.md: %w", taskID, err)
	}

	ctx := &TaskContext{
		TaskID:      taskID,
		Notes:       notesContent,
		Context:     contextContent,
		LastUpdated: time.Now(),
	}
	m.contexts[taskID] = ctx
	return ctx, nil
}

func (m *fileContextManager) LoadContext(taskID string) (*TaskContext, error) {
	contextData, err := os.ReadFile(m.contextPath(taskID))
	if err != nil {
		return nil, fmt.Errorf("loading context for %s: reading context.md: %w", taskID, err)
	}

	notesData, err := os.ReadFile(m.notesPath(taskID))
	if err != nil {
		return nil, fmt.Errorf("loading context for %s: reading notes.md: %w", taskID, err)
	}

	// Load communications from the communications directory.
	comms, err := m.loadCommunications(taskID)
	if err != nil {
		return nil, fmt.Errorf("loading context for %s: loading communications: %w", taskID, err)
	}

	ctx := &TaskContext{
		TaskID:         taskID,
		Notes:          string(notesData),
		Context:        string(contextData),
		Communications: comms,
		LastUpdated:    time.Now(),
	}
	m.contexts[taskID] = ctx
	return ctx, nil
}

func (m *fileContextManager) loadCommunications(taskID string) ([]models.Communication, error) {
	dir := m.commsDir(taskID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var comms []models.Communication
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // G304: reading communication files from managed directory
		if err != nil {
			continue
		}
		comm := parseCommunicationFile(string(data))
		comms = append(comms, comm)
	}
	return comms, nil
}

func parseCommunicationFile(content string) models.Communication {
	comm := models.Communication{}
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "**Date:**") {
			dateStr := strings.TrimSpace(strings.TrimPrefix(line, "**Date:**"))
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				comm.Date = t
			}
		} else if strings.HasPrefix(line, "**Source:**") {
			comm.Source = strings.TrimSpace(strings.TrimPrefix(line, "**Source:**"))
		} else if strings.HasPrefix(line, "**Contact:**") {
			comm.Contact = strings.TrimSpace(strings.TrimPrefix(line, "**Contact:**"))
		} else if strings.HasPrefix(line, "**Topic:**") {
			comm.Topic = strings.TrimSpace(strings.TrimPrefix(line, "**Topic:**"))
		}
	}

	// Extract content between ## Content and ## Tags.
	if idx := strings.Index(content, "## Content"); idx >= 0 {
		rest := content[idx+len("## Content"):]
		if endIdx := strings.Index(rest, "## Tags"); endIdx >= 0 {
			comm.Content = strings.TrimSpace(rest[:endIdx])
		} else {
			comm.Content = strings.TrimSpace(rest)
		}
	}

	// Extract tags.
	if idx := strings.Index(content, "## Tags"); idx >= 0 {
		rest := content[idx+len("## Tags"):]
		if endIdx := strings.Index(rest, "## "); endIdx >= 0 {
			rest = rest[:endIdx]
		}
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				tag := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if tag != "" {
					comm.Tags = append(comm.Tags, models.CommunicationTag(tag))
				}
			}
		}
	}

	return comm
}

func (m *fileContextManager) UpdateContext(taskID string, updates map[string]interface{}) error {
	ctx, ok := m.contexts[taskID]
	if !ok {
		loaded, err := m.LoadContext(taskID)
		if err != nil {
			return fmt.Errorf("updating context for %s: %w", taskID, err)
		}
		ctx = loaded
	}

	if notes, ok := updates["notes"]; ok {
		if s, ok := notes.(string); ok {
			ctx.Notes = s
		}
	}
	if context, ok := updates["context"]; ok {
		if s, ok := context.(string); ok {
			ctx.Context = s
		}
	}

	ctx.LastUpdated = time.Now()
	m.contexts[taskID] = ctx
	return nil
}

func (m *fileContextManager) PersistContext(taskID string) error {
	ctx, ok := m.contexts[taskID]
	if !ok {
		return fmt.Errorf("persisting context for %s: no context loaded", taskID)
	}

	if err := os.MkdirAll(m.ticketDir(taskID), 0o750); err != nil {
		return fmt.Errorf("persisting context for %s: creating directory: %w", taskID, err)
	}

	if err := os.WriteFile(m.contextPath(taskID), []byte(ctx.Context), 0o600); err != nil {
		return fmt.Errorf("persisting context for %s: writing context.md: %w", taskID, err)
	}
	if err := os.WriteFile(m.notesPath(taskID), []byte(ctx.Notes), 0o600); err != nil {
		return fmt.Errorf("persisting context for %s: writing notes.md: %w", taskID, err)
	}

	return nil
}

func (m *fileContextManager) GetContextForAI(taskID string) (*AIContext, error) {
	ctx, ok := m.contexts[taskID]
	if !ok {
		loaded, err := m.LoadContext(taskID)
		if err != nil {
			return nil, fmt.Errorf("getting AI context for %s: %w", taskID, err)
		}
		ctx = loaded
	}

	ai := &AIContext{}
	ai.Summary = extractSection(ctx.Context, "## Summary")
	ai.RecentActivity = extractListSection(ctx.Context, "## Recent Progress")
	ai.OpenQuestions = extractListSection(ctx.Context, "## Open Questions")
	ai.Decisions = extractListSection(ctx.Context, "## Decisions Made")
	ai.Blockers = extractListSection(ctx.Context, "## Blockers")

	return ai, nil
}

// extractSection returns the text content between a section header and the next section header.
func extractSection(content, header string) string {
	idx := strings.Index(content, header)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(header):]
	if endIdx := strings.Index(rest, "\n## "); endIdx >= 0 {
		return strings.TrimSpace(rest[:endIdx])
	}
	return strings.TrimSpace(rest)
}

// extractListSection returns the list items from a markdown section.
func extractListSection(content, header string) []string {
	section := extractSection(content, header)
	if section == "" {
		return nil
	}
	var items []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(line, "- ")
			// Strip checkbox prefix if present.
			item = strings.TrimPrefix(item, "[ ] ")
			item = strings.TrimPrefix(item, "[x] ")
			item = strings.TrimSpace(item)
			if item != "" {
				items = append(items, item)
			}
		}
	}
	return items
}

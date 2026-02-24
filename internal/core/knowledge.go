package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// KnowledgeExtractor extracts and feeds knowledge back to documentation.
type KnowledgeExtractor interface {
	ExtractFromTask(taskID string) (*models.ExtractedKnowledge, error)
	GenerateHandoff(taskID string) (*models.HandoffDocument, error)
	UpdateWiki(knowledge *models.ExtractedKnowledge) error
	CreateADR(decision models.Decision, taskID string) (string, error)
}

type knowledgeExtractor struct {
	basePath string
	ctxMgr   TaskContextLoader
	commMgr  CommunicationStore
}

// NewKnowledgeExtractor creates a KnowledgeExtractor that reads task context
// and communications and produces knowledge artifacts.
func NewKnowledgeExtractor(basePath string, ctxMgr TaskContextLoader, commMgr CommunicationStore) KnowledgeExtractor {
	return &knowledgeExtractor{
		basePath: basePath,
		ctxMgr:   ctxMgr,
		commMgr:  commMgr,
	}
}

func (ke *knowledgeExtractor) ExtractFromTask(taskID string) (*models.ExtractedKnowledge, error) {
	ctx, err := ke.ctxMgr.LoadContext(taskID)
	if err != nil {
		return nil, fmt.Errorf("extracting knowledge from %s: loading context: %w", taskID, err)
	}

	comms, err := ke.commMgr.GetAllCommunications(taskID)
	if err != nil {
		return nil, fmt.Errorf("extracting knowledge from %s: loading communications: %w", taskID, err)
	}

	knowledge := &models.ExtractedKnowledge{
		TaskID: taskID,
	}

	// Extract learnings from notes and context.
	knowledge.Learnings = extractListItems(ctx.Notes, "## Learnings")
	if len(knowledge.Learnings) == 0 {
		knowledge.Learnings = extractListItems(ctx.Notes, "## Key Learnings")
	}

	// Extract gotchas from notes.
	knowledge.Gotchas = extractListItems(ctx.Notes, "## Gotchas")

	// Extract decisions from context and communications.
	contextDecisions := extractListItems(ctx.Context, "## Decisions Made")
	for _, d := range contextDecisions {
		knowledge.Decisions = append(knowledge.Decisions, models.Decision{
			Title:    d,
			Decision: d,
		})
	}

	// Extract decisions tagged in communications.
	for _, comm := range comms {
		for _, tag := range comm.Tags {
			if tag == models.TagDecision {
				knowledge.Decisions = append(knowledge.Decisions, models.Decision{
					Title:    comm.Topic,
					Context:  fmt.Sprintf("From %s communication with %s on %s", comm.Source, comm.Contact, comm.Date.Format("2006-01-02")),
					Decision: comm.Content,
				})
				break
			}
		}
	}

	// Extract knowledge from design.md if it exists.
	designDocPath := filepath.Join(resolveTicketDir(ke.basePath, taskID), "design.md")
	if designData, err := os.ReadFile(designDocPath); err == nil {
		designContent := string(designData)

		// Extract technical decisions from the design doc table.
		designDecisions := extractDesignDocDecisions(designContent)
		for _, dd := range designDecisions {
			knowledge.Decisions = append(knowledge.Decisions, models.Decision{
				Title:    dd,
				Decision: dd,
			})
		}

		// Extract overview as a learning.
		overview := extractSectionText(designContent, "## Overview")
		if overview != "" && !isPlaceholder(overview) {
			knowledge.Learnings = append(knowledge.Learnings, overview)
		}

		// Extract component descriptions as learnings.
		components := extractComponentLearnings(designContent)
		knowledge.Learnings = append(knowledge.Learnings, components...)
	}

	// Extract wiki updates from notes.
	wikiItems := extractListItems(ctx.Notes, "## Wiki Updates")
	for _, item := range wikiItems {
		knowledge.WikiUpdates = append(knowledge.WikiUpdates, models.WikiUpdate{
			Topic:  item,
			TaskID: taskID,
		})
	}

	// Extract runbook updates from notes.
	runbookItems := extractListItems(ctx.Notes, "## Runbook Updates")
	for _, item := range runbookItems {
		knowledge.RunbookUpdates = append(knowledge.RunbookUpdates, models.RunbookUpdate{
			Section: item,
			TaskID:  taskID,
		})
	}

	return knowledge, nil
}

func (ke *knowledgeExtractor) GenerateHandoff(taskID string) (*models.HandoffDocument, error) {
	ctx, err := ke.ctxMgr.LoadContext(taskID)
	if err != nil {
		return nil, fmt.Errorf("generating handoff for %s: loading context: %w", taskID, err)
	}

	knowledge, err := ke.ExtractFromTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("generating handoff for %s: extracting knowledge: %w", taskID, err)
	}

	handoff := &models.HandoffDocument{
		TaskID:      taskID,
		GeneratedAt: time.Now(),
	}

	// Extract summary from context.
	handoff.Summary = extractSectionText(ctx.Context, "## Summary")

	// Extract completed work from context.
	handoff.CompletedWork = extractListItems(ctx.Context, "## Recent Progress")

	// Extract open items from context.
	handoff.OpenItems = extractListItems(ctx.Context, "## Next Steps")

	// Learnings from knowledge extraction.
	handoff.Learnings = knowledge.Learnings

	// Write handoff.md.
	handoffPath := filepath.Join(resolveTicketDir(ke.basePath, taskID), "handoff.md")
	content := formatHandoff(handoff, knowledge)
	if err := os.WriteFile(handoffPath, []byte(content), 0o600); err != nil {
		return nil, fmt.Errorf("generating handoff for %s: writing handoff.md: %w", taskID, err)
	}

	return handoff, nil
}

func (ke *knowledgeExtractor) UpdateWiki(knowledge *models.ExtractedKnowledge) error {
	wikiDir := filepath.Join(ke.basePath, "docs", "wiki")
	if err := os.MkdirAll(wikiDir, 0o750); err != nil {
		return fmt.Errorf("updating wiki: creating directory: %w", err)
	}

	for _, update := range knowledge.WikiUpdates {
		path := update.WikiPath
		if path == "" {
			// Default to a topic-based path.
			safeTopic := sanitizeForPath(update.Topic)
			path = filepath.Join(wikiDir, safeTopic+".md")
		} else if !filepath.IsAbs(path) {
			path = filepath.Join(ke.basePath, path)
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("updating wiki: creating directory for %s: %w", path, err)
		}

		content := fmt.Sprintf("# %s\n\n%s\n\n---\n*Learned from %s*\n",
			update.Topic, update.Content, knowledge.TaskID)

		// Append if the file already exists.
		if existing, err := os.ReadFile(path); err == nil {
			attribution := fmt.Sprintf("\n\n## Update from %s\n\n%s\n\n---\n*Learned from %s*\n",
				knowledge.TaskID, update.Content, knowledge.TaskID)
			content = string(existing) + attribution
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("updating wiki for topic %q: %w", update.Topic, err)
		}
	}

	return nil
}

func (ke *knowledgeExtractor) CreateADR(decision models.Decision, taskID string) (string, error) {
	decisionsDir := filepath.Join(ke.basePath, "docs", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o750); err != nil {
		return "", fmt.Errorf("creating ADR: creating directory: %w", err)
	}

	// Determine the next ADR number.
	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		return "", fmt.Errorf("creating ADR: reading decisions dir: %w", err)
	}
	nextNum := len(entries) + 1
	adrID := fmt.Sprintf("ADR-%04d", nextNum)

	safeTitle := sanitizeForPath(decision.Title)
	filename := fmt.Sprintf("%s-%s.md", adrID, safeTitle)
	path := filepath.Join(decisionsDir, filename)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", adrID, decision.Title))
	sb.WriteString("**Status:** Accepted\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Source:** %s\n\n", taskID))
	sb.WriteString("## Context\n\n")
	sb.WriteString(decision.Context)
	sb.WriteString("\n\n## Decision\n\n")
	sb.WriteString(decision.Decision)
	sb.WriteString("\n\n## Consequences\n\n")
	for _, c := range decision.Consequences {
		sb.WriteString(fmt.Sprintf("- %s\n", c))
	}
	sb.WriteString("\n## Alternatives Considered\n\n")
	for _, a := range decision.Alternatives {
		sb.WriteString(fmt.Sprintf("- %s\n", a))
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return "", fmt.Errorf("creating ADR: writing file: %w", err)
	}

	return path, nil
}

func formatHandoff(handoff *models.HandoffDocument, knowledge *models.ExtractedKnowledge) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Handoff: %s\n\n", handoff.TaskID))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n", handoff.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString("**Status:** Archived\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString(handoff.Summary)
	sb.WriteString("\n\n")

	sb.WriteString("## Completed Work\n\n")
	for _, item := range handoff.CompletedWork {
		sb.WriteString(fmt.Sprintf("- %s\n", item))
	}
	sb.WriteString("\n")

	sb.WriteString("## Open Items\n\n")
	for _, item := range handoff.OpenItems {
		sb.WriteString(fmt.Sprintf("- [ ] %s\n", item))
	}
	sb.WriteString("\n")

	sb.WriteString("## Key Learnings\n\n")
	for _, item := range handoff.Learnings {
		sb.WriteString(fmt.Sprintf("- %s\n", item))
	}
	sb.WriteString("\n")

	sb.WriteString("## Decisions Made\n\n")
	for _, d := range knowledge.Decisions {
		sb.WriteString(fmt.Sprintf("- %s\n", d.Title))
	}
	sb.WriteString("\n")

	sb.WriteString("## Gotchas\n\n")
	for _, g := range knowledge.Gotchas {
		sb.WriteString(fmt.Sprintf("- %s\n", g))
	}
	sb.WriteString("\n")

	sb.WriteString("## Related Documentation\n\n")
	for _, doc := range handoff.RelatedDocs {
		sb.WriteString(fmt.Sprintf("- %s\n", doc))
	}
	sb.WriteString("\n")

	sb.WriteString("## Provenance\n\n")
	sb.WriteString(fmt.Sprintf("This handoff was generated from %s communications and notes.\n", handoff.TaskID))

	return sb.String()
}

// extractSectionText returns the text content of a markdown section.
func extractSectionText(content, header string) string {
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

// extractListItems extracts list items from a markdown section.
func extractListItems(content, header string) []string {
	section := extractSectionText(content, header)
	if section == "" {
		return nil
	}
	var items []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(line, "- ")
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

// extractDesignDocDecisions extracts decision text from the Technical Decisions
// markdown table in a design document.
func extractDesignDocDecisions(content string) []string {
	section := extractSectionText(content, "## Technical Decisions")
	if section == "" {
		return nil
	}
	var decisions []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "| Decision") || strings.HasPrefix(line, "|---") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		decision := strings.TrimSpace(parts[1])
		if decision != "" {
			decisions = append(decisions, decision)
		}
	}
	return decisions
}

// extractComponentLearnings extracts component name+purpose as learnings from
// the Components section of a design document.
func extractComponentLearnings(content string) []string {
	section := extractSectionText(content, "## Components")
	if section == "" {
		return nil
	}
	var learnings []string
	lines := strings.Split(section, "\n")
	var currentName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "### ") {
			currentName = strings.TrimPrefix(line, "### ")
		} else if strings.HasPrefix(line, "- **Purpose:**") && currentName != "" {
			purpose := strings.TrimSpace(strings.TrimPrefix(line, "- **Purpose:**"))
			if purpose != "" && !isPlaceholder(purpose) {
				learnings = append(learnings, fmt.Sprintf("%s: %s", currentName, purpose))
			}
		}
	}
	return learnings
}

// isPlaceholder returns true if the text is a template placeholder.
func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")
}

func sanitizeForPath(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	// Collapse multiple hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

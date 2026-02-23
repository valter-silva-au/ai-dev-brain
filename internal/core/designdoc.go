package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TaskDesignDocument represents a task-level technical design document.
type TaskDesignDocument struct {
	TaskID                  string
	Title                   string
	Overview                string
	Architecture            string // Mermaid diagram
	Components              []ComponentDescription
	TechnicalDecisions      []TechnicalDecision
	RelatedADRs             []string
	StakeholderRequirements []string
	LastUpdated             time.Time
}

// ComponentDescription describes a single component in the design document.
type ComponentDescription struct {
	Name         string
	Purpose      string
	Interfaces   []string
	Dependencies []string
}

// TechnicalDecision captures a technical decision extracted from communications
// or manually added to the design document.
type TechnicalDecision struct {
	Decision     string
	Rationale    string
	Source       string
	Date         time.Time
	ADRCandidate bool
}

// DesignUpdate represents an update to a specific section of the design document.
type DesignUpdate struct {
	Section string // "overview", "architecture", "components", "decisions"
	Content string
}

// TaskDesignDocGenerator manages task-level technical design documents.
type TaskDesignDocGenerator interface {
	InitializeDesignDoc(taskID string) error
	PopulateFromContext(taskID string) error
	UpdateDesignDoc(taskID string, update DesignUpdate) error
	ExtractFromCommunications(taskID string) ([]TechnicalDecision, error)
	GenerateArchitectureDiagram(taskID string) (string, error)
	GetDesignDoc(taskID string) (*TaskDesignDocument, error)
}

type taskDesignDocGenerator struct {
	basePath string
	commMgr  storage.CommunicationManager
}

// NewTaskDesignDocGenerator creates a new TaskDesignDocGenerator.
func NewTaskDesignDocGenerator(basePath string, commMgr storage.CommunicationManager) TaskDesignDocGenerator {
	return &taskDesignDocGenerator{
		basePath: basePath,
		commMgr:  commMgr,
	}
}

func (g *taskDesignDocGenerator) designDocPath(taskID string) string {
	return filepath.Join(resolveTicketDir(g.basePath, taskID), "design.md")
}

const designDocTemplate = `# Technical Design: %s

**Task:** %s
**Created:** %s
**Last Updated:** %s

## Overview

[Brief description of what this task accomplishes technically]

## Architecture

` + "```mermaid\ngraph TB\n    A[Component A] --> B[Component B]\n    B --> C[Component C]\n```" + `

## Components

## Technical Decisions

| Decision | Rationale | Source | Date |
|----------|-----------|--------|------|

## Related ADRs

## Stakeholder Requirements

## Open Technical Questions

- [ ] [Question]

---
*This document is maintained by AI Dev Brain and updated as work progresses.*
`

// InitializeDesignDoc creates a new design.md file for the given task.
func (g *taskDesignDocGenerator) InitializeDesignDoc(taskID string) error {
	dir := filepath.Dir(g.designDocPath(taskID))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("initializing design doc for %s: creating directory: %w", taskID, err)
	}

	now := time.Now()
	content := fmt.Sprintf(designDocTemplate,
		taskID,
		taskID,
		now.Format("2006-01-02"),
		now.Format(time.RFC3339),
	)

	if err := os.WriteFile(g.designDocPath(taskID), []byte(content), 0o600); err != nil {
		return fmt.Errorf("initializing design doc for %s: writing file: %w", taskID, err)
	}

	return nil
}

// PopulateFromContext pulls wiki, ADR, and requirement context into the design doc.
// It reads existing communications for requirements and decisions, scans docs/decisions
// for related ADRs, and updates the design document accordingly.
func (g *taskDesignDocGenerator) PopulateFromContext(taskID string) error {
	doc, err := g.GetDesignDoc(taskID)
	if err != nil {
		return fmt.Errorf("populating design doc for %s: %w", taskID, err)
	}

	// Extract technical decisions from communications.
	decisions, err := g.ExtractFromCommunications(taskID)
	if err != nil {
		return fmt.Errorf("populating design doc for %s: extracting decisions: %w", taskID, err)
	}
	doc.TechnicalDecisions = append(doc.TechnicalDecisions, decisions...)

	// Extract stakeholder requirements from communications.
	comms, err := g.commMgr.GetAllCommunications(taskID)
	if err != nil {
		return fmt.Errorf("populating design doc for %s: loading communications: %w", taskID, err)
	}
	for _, comm := range comms {
		for _, tag := range comm.Tags {
			if tag == models.TagRequirement {
				req := fmt.Sprintf("[From %s via %s] %s", comm.Contact, comm.Source, comm.Content)
				doc.StakeholderRequirements = append(doc.StakeholderRequirements, req)
				break
			}
		}
	}

	// Scan for related ADRs that reference this task.
	adrs, err := g.findRelatedADRs(taskID)
	if err == nil {
		doc.RelatedADRs = append(doc.RelatedADRs, adrs...)
	}

	doc.LastUpdated = time.Now()
	return g.writeDesignDoc(doc)
}

// UpdateDesignDoc updates a specific section of the design document.
func (g *taskDesignDocGenerator) UpdateDesignDoc(taskID string, update DesignUpdate) error {
	doc, err := g.GetDesignDoc(taskID)
	if err != nil {
		return fmt.Errorf("updating design doc for %s: %w", taskID, err)
	}

	switch update.Section {
	case "overview":
		doc.Overview = update.Content
	case "architecture":
		doc.Architecture = update.Content
	case "components":
		doc.Components = append(doc.Components, ComponentDescription{
			Name:    update.Content,
			Purpose: "[To be filled]",
		})
	case "decisions":
		doc.TechnicalDecisions = append(doc.TechnicalDecisions, TechnicalDecision{
			Decision: update.Content,
			Date:     time.Now(),
		})
	default:
		return fmt.Errorf("updating design doc for %s: unknown section %q", taskID, update.Section)
	}

	doc.LastUpdated = time.Now()
	return g.writeDesignDoc(doc)
}

// ExtractFromCommunications extracts technical decisions from task communications.
func (g *taskDesignDocGenerator) ExtractFromCommunications(taskID string) ([]TechnicalDecision, error) {
	comms, err := g.commMgr.GetAllCommunications(taskID)
	if err != nil {
		return nil, fmt.Errorf("extracting decisions from communications for %s: %w", taskID, err)
	}

	var decisions []TechnicalDecision
	for _, comm := range comms {
		for _, tag := range comm.Tags {
			if tag == models.TagDecision {
				decisions = append(decisions, TechnicalDecision{
					Decision:     comm.Content,
					Rationale:    comm.Topic,
					Source:       fmt.Sprintf("%s-%s", comm.Source, comm.Contact),
					Date:         comm.Date,
					ADRCandidate: true,
				})
				break
			}
		}
	}

	return decisions, nil
}

// GenerateArchitectureDiagram generates a Mermaid diagram from the design document's
// component list. Returns the Mermaid source string.
func (g *taskDesignDocGenerator) GenerateArchitectureDiagram(taskID string) (string, error) {
	doc, err := g.GetDesignDoc(taskID)
	if err != nil {
		return "", fmt.Errorf("generating diagram for %s: %w", taskID, err)
	}

	if len(doc.Components) == 0 {
		return "graph TB\n    Note[No components defined yet]", nil
	}

	var sb strings.Builder
	sb.WriteString("graph TB\n")

	// Build a dependency graph from the components.
	for _, comp := range doc.Components {
		sb.WriteString(fmt.Sprintf("    %s[%s]\n", sanitizeForMermaid(comp.Name), comp.Name))
	}

	for _, comp := range doc.Components {
		for _, dep := range comp.Dependencies {
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", sanitizeForMermaid(comp.Name), sanitizeForMermaid(dep)))
		}
	}

	return sb.String(), nil
}

// GetDesignDoc reads and parses the design.md file for the given task.
func (g *taskDesignDocGenerator) GetDesignDoc(taskID string) (*TaskDesignDocument, error) {
	data, err := os.ReadFile(g.designDocPath(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("design doc for %s not found: %w", taskID, err)
		}
		return nil, fmt.Errorf("reading design doc for %s: %w", taskID, err)
	}

	return parseDesignDoc(taskID, string(data)), nil
}

// parseDesignDoc parses a design.md markdown file into a TaskDesignDocument.
func parseDesignDoc(taskID, content string) *TaskDesignDocument {
	doc := &TaskDesignDocument{
		TaskID: taskID,
	}

	// Extract title from header metadata.
	doc.Title = extractMetadataField(content, "**Task:**")

	// Extract last updated.
	lastUpdatedStr := extractMetadataField(content, "**Last Updated:**")
	if t, err := time.Parse(time.RFC3339, lastUpdatedStr); err == nil {
		doc.LastUpdated = t
	}

	// Extract overview.
	doc.Overview = extractDesignSection(content, "## Overview")

	// Extract architecture (Mermaid block).
	doc.Architecture = extractMermaidBlock(content)

	// Extract components.
	doc.Components = extractComponents(content)

	// Extract technical decisions from table.
	doc.TechnicalDecisions = extractDecisionTable(content)

	// Extract related ADRs.
	doc.RelatedADRs = extractListFromSection(content, "## Related ADRs")

	// Extract stakeholder requirements.
	doc.StakeholderRequirements = extractListFromSection(content, "## Stakeholder Requirements")

	return doc
}

func extractMetadataField(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func extractDesignSection(content, header string) string {
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

func extractMermaidBlock(content string) string {
	start := strings.Index(content, "```mermaid")
	if start < 0 {
		return ""
	}
	rest := content[start+len("```mermaid"):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func extractComponents(content string) []ComponentDescription {
	section := extractDesignSection(content, "## Components")
	if section == "" {
		return nil
	}

	var components []ComponentDescription
	lines := strings.Split(section, "\n")
	var current *ComponentDescription

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "### ") {
			if current != nil {
				components = append(components, *current)
			}
			current = &ComponentDescription{
				Name: strings.TrimPrefix(line, "### "),
			}
		} else if current != nil {
			if strings.HasPrefix(line, "- **Purpose:**") {
				current.Purpose = strings.TrimSpace(strings.TrimPrefix(line, "- **Purpose:**"))
			} else if strings.HasPrefix(line, "- **Interfaces:**") {
				ifaces := strings.TrimSpace(strings.TrimPrefix(line, "- **Interfaces:**"))
				if ifaces != "" {
					current.Interfaces = strings.Split(ifaces, ", ")
				}
			} else if strings.HasPrefix(line, "- **Dependencies:**") {
				deps := strings.TrimSpace(strings.TrimPrefix(line, "- **Dependencies:**"))
				if deps != "" {
					current.Dependencies = strings.Split(deps, ", ")
				}
			}
		}
	}

	if current != nil {
		components = append(components, *current)
	}

	return components
}

func extractDecisionTable(content string) []TechnicalDecision {
	section := extractDesignSection(content, "## Technical Decisions")
	if section == "" {
		return nil
	}

	var decisions []TechnicalDecision
	lines := strings.Split(section, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "| Decision") || strings.HasPrefix(line, "|---") {
			continue
		}
		parts := strings.Split(line, "|")
		// Expected: empty, decision, rationale, source, date, empty
		if len(parts) < 5 {
			continue
		}
		decision := strings.TrimSpace(parts[1])
		rationale := strings.TrimSpace(parts[2])
		source := strings.TrimSpace(parts[3])
		dateStr := strings.TrimSpace(parts[4])

		if decision == "" {
			continue
		}

		td := TechnicalDecision{
			Decision:  decision,
			Rationale: rationale,
			Source:    source,
		}
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			td.Date = t
		}
		decisions = append(decisions, td)
	}

	return decisions
}

func extractListFromSection(content, header string) []string {
	section := extractDesignSection(content, header)
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

func (g *taskDesignDocGenerator) writeDesignDoc(doc *TaskDesignDocument) error {
	content := formatDesignDoc(doc)
	if err := os.WriteFile(g.designDocPath(doc.TaskID), []byte(content), 0o600); err != nil {
		return fmt.Errorf("writing design doc for %s: %w", doc.TaskID, err)
	}
	return nil
}

func formatDesignDoc(doc *TaskDesignDocument) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Technical Design: %s\n\n", doc.TaskID))
	sb.WriteString(fmt.Sprintf("**Task:** %s\n", doc.Title))
	sb.WriteString(fmt.Sprintf("**Last Updated:** %s\n\n", doc.LastUpdated.Format(time.RFC3339)))

	sb.WriteString("## Overview\n\n")
	if doc.Overview != "" {
		sb.WriteString(doc.Overview)
	} else {
		sb.WriteString("[Brief description of what this task accomplishes technically]")
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Architecture\n\n")
	if doc.Architecture != "" {
		sb.WriteString("```mermaid\n")
		sb.WriteString(doc.Architecture)
		sb.WriteString("\n```")
	} else {
		sb.WriteString("```mermaid\ngraph TB\n    A[Component A] --> B[Component B]\n```")
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Components\n\n")
	if len(doc.Components) > 0 {
		for _, comp := range doc.Components {
			sb.WriteString(fmt.Sprintf("### %s\n", comp.Name))
			sb.WriteString(fmt.Sprintf("- **Purpose:** %s\n", comp.Purpose))
			if len(comp.Interfaces) > 0 {
				sb.WriteString(fmt.Sprintf("- **Interfaces:** %s\n", strings.Join(comp.Interfaces, ", ")))
			}
			if len(comp.Dependencies) > 0 {
				sb.WriteString(fmt.Sprintf("- **Dependencies:** %s\n", strings.Join(comp.Dependencies, ", ")))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Technical Decisions\n\n")
	sb.WriteString("| Decision | Rationale | Source | Date |\n")
	sb.WriteString("|----------|-----------|--------|------|\n")
	for _, d := range doc.TechnicalDecisions {
		dateStr := ""
		if !d.Date.IsZero() {
			dateStr = d.Date.Format("2006-01-02")
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", d.Decision, d.Rationale, d.Source, dateStr))
	}
	sb.WriteString("\n")

	sb.WriteString("## Related ADRs\n\n")
	for _, adr := range doc.RelatedADRs {
		sb.WriteString(fmt.Sprintf("- %s\n", adr))
	}
	sb.WriteString("\n")

	sb.WriteString("## Stakeholder Requirements\n\n")
	for _, req := range doc.StakeholderRequirements {
		sb.WriteString(fmt.Sprintf("- %s\n", req))
	}
	sb.WriteString("\n")

	sb.WriteString("---\n*This document is maintained by AI Dev Brain and updated as work progresses.*\n")

	return sb.String()
}

// findRelatedADRs scans docs/decisions/ for ADRs that reference the given task ID.
func (g *taskDesignDocGenerator) findRelatedADRs(taskID string) ([]string, error) {
	decisionsDir := filepath.Join(g.basePath, "docs", "decisions")
	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning ADRs: %w", err)
	}

	var related []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(decisionsDir, entry.Name())) //nolint:gosec // G304: reading ADR files from managed decisions directory
		if err != nil {
			continue
		}
		if strings.Contains(string(data), taskID) {
			relPath := filepath.Join("../../docs/decisions", entry.Name())
			// Extract title from first line if possible.
			firstLine := strings.SplitN(string(data), "\n", 2)[0]
			title := strings.TrimPrefix(firstLine, "# ")
			related = append(related, fmt.Sprintf("[%s](%s)", title, relPath))
		}
	}

	return related, nil
}

// sanitizeForMermaid converts a component name to a valid Mermaid node ID.
func sanitizeForMermaid(name string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, name)
	// Collapse multiple underscores.
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}
	return strings.Trim(result, "_")
}

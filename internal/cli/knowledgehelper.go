package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const knowledgeSectionHeader = "## Accumulated Project Knowledge"

// Enriched section headers inserted into task-context.md on resume.
const (
	requirementsSectionHeader = "## Requirements (from notes.md)"
	decisionsSectionHeader    = "## Task Decisions"
	latestSessionHeader       = "## Latest Session"
)

// taskDecision represents a single entry in knowledge/decisions.yaml.
type taskDecision struct {
	Decision  string `yaml:"decision"`
	Rationale string `yaml:"rationale"`
	Date      string `yaml:"date"`
	Status    string `yaml:"status"`
}

// appendKnowledgeToTaskContext appends a knowledge summary section to the
// task-context.md file in the worktree. This is non-fatal: any errors are
// silently ignored so that task creation/resume is never blocked.
func appendKnowledgeToTaskContext(worktreePath string) {
	if KnowledgeMgr == nil {
		return
	}

	summary, err := KnowledgeMgr.AssembleKnowledgeSummary(10)
	if err != nil || summary == "" {
		return
	}

	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")

	existing, err := os.ReadFile(taskContextPath)
	if err != nil {
		return
	}

	content := string(existing)

	// Remove any existing knowledge section before appending fresh content.
	content = removeKnowledgeSection(content)

	section := "\n" + knowledgeSectionHeader + "\n\n" + summary
	content = strings.TrimRight(content, "\n") + "\n" + section

	_ = os.WriteFile(taskContextPath, []byte(content), 0o644)
}

// removeKnowledgeSection strips the "## Accumulated Project Knowledge" section
// and everything after it from the content. This allows refreshing the section
// on resume without duplicating it.
func removeKnowledgeSection(content string) string {
	idx := strings.Index(content, knowledgeSectionHeader)
	if idx < 0 {
		return content
	}
	return strings.TrimRight(content[:idx], "\n") + "\n"
}

// readTruncatedFile reads a file and returns the first maxLines lines.
// Returns empty string if the file does not exist or cannot be read.
func readTruncatedFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, fmt.Sprintf("\n... (truncated at %d lines)", maxLines))
	}
	return strings.Join(lines, "\n")
}

// readDecisions parses a knowledge/decisions.yaml file and returns a
// markdown-formatted list. Returns empty string if the file does not exist
// or contains no decisions.
func readDecisions(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var decisions []taskDecision
	if err := yaml.Unmarshal(data, &decisions); err != nil {
		return ""
	}
	if len(decisions) == 0 {
		return ""
	}

	var b strings.Builder
	for _, d := range decisions {
		b.WriteString(fmt.Sprintf("- **%s**", d.Decision))
		if d.Rationale != "" {
			b.WriteString(fmt.Sprintf(" -- %s", d.Rationale))
		}
		if d.Date != "" {
			b.WriteString(fmt.Sprintf(" (%s)", d.Date))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// readLatestSession finds the most recent .md file in sessionsDir and returns
// its first maxLines lines. Returns empty string if no sessions exist.
func readLatestSession(sessionsDir string, maxLines int) string {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}

	var mdFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mdFiles = append(mdFiles, e.Name())
		}
	}
	if len(mdFiles) == 0 {
		return ""
	}

	sort.Strings(mdFiles)
	latest := mdFiles[len(mdFiles)-1]
	return readTruncatedFile(filepath.Join(sessionsDir, latest), maxLines)
}

// removeSectionByHeader removes a markdown ## section and its content up to
// the next ## heading (or end of string). Returns the content with the section
// removed.
func removeSectionByHeader(content, header string) string {
	idx := strings.Index(content, header)
	if idx < 0 {
		return content
	}

	// Find the next ## heading after this section.
	rest := content[idx+len(header):]
	nextIdx := strings.Index(rest, "\n## ")
	if nextIdx < 0 {
		// Section extends to end of content.
		return strings.TrimRight(content[:idx], "\n") + "\n"
	}
	// Keep the newline before the next section header.
	after := rest[nextIdx:]
	return strings.TrimRight(content[:idx], "\n") + "\n" + after
}

// removeEnrichedSections strips all three enriched sections from content,
// making enrichTaskContext idempotent on re-resume.
func removeEnrichedSections(content string) string {
	content = removeSectionByHeader(content, requirementsSectionHeader)
	content = removeSectionByHeader(content, decisionsSectionHeader)
	content = removeSectionByHeader(content, latestSessionHeader)
	return content
}

// enrichTaskContext inlines task notes, decisions, and the latest session
// summary into the task-context.md file in the worktree. This gives AI agents
// immediate access to key task context without having to read separate files.
// This is non-fatal: any errors are silently ignored so that resume is never
// blocked. The function is idempotent: enriched sections are removed before
// being re-added.
func enrichTaskContext(worktreePath, ticketPath string) {
	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")

	existing, err := os.ReadFile(taskContextPath)
	if err != nil {
		return
	}

	content := string(existing)

	// Remove any previously enriched sections (idempotent on re-resume).
	content = removeEnrichedSections(content)

	// Build enriched sections.
	var sections []string

	if notes := readTruncatedFile(filepath.Join(ticketPath, "notes.md"), 50); notes != "" {
		sections = append(sections, requirementsSectionHeader+"\n\n"+notes)
	}

	if decisions := readDecisions(filepath.Join(ticketPath, "knowledge", "decisions.yaml")); decisions != "" {
		sections = append(sections, decisionsSectionHeader+"\n\n"+decisions)
	}

	if session := readLatestSession(filepath.Join(ticketPath, "sessions"), 30); session != "" {
		sections = append(sections, latestSessionHeader+"\n\n"+session)
	}

	if len(sections) == 0 {
		return
	}

	enrichedBlock := "\n" + strings.Join(sections, "\n\n") + "\n"

	// Insert enriched sections before the knowledge section if present,
	// otherwise append at the end.
	if idx := strings.Index(content, knowledgeSectionHeader); idx >= 0 {
		content = strings.TrimRight(content[:idx], "\n") + "\n" + enrichedBlock + "\n" + content[idx:]
	} else {
		content = strings.TrimRight(content, "\n") + "\n" + enrichedBlock
	}

	_ = os.WriteFile(taskContextPath, []byte(content), 0o644)
}

// refreshTaskContextMetadata reads task-context.md in a worktree and updates
// the Status line to match the provided status. This is non-fatal: errors are
// silently ignored so that resume is never blocked.
func refreshTaskContextMetadata(worktreePath, status string) {
	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")

	data, err := os.ReadFile(taskContextPath)
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	updated := false
	for i, line := range lines {
		if strings.HasPrefix(line, "- **Status**: ") {
			lines[i] = "- **Status**: " + status
			updated = true
			break
		}
	}

	if !updated {
		return
	}

	_ = os.WriteFile(taskContextPath, []byte(strings.Join(lines, "\n")), 0o644)
}

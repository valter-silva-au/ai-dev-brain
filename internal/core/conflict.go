package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConflictType categorises the kind of conflict detected.
type ConflictType string

const (
	ConflictADRViolation            ConflictType = "adr_violation"
	ConflictPreviousDecision        ConflictType = "previous_decision"
	ConflictStakeholderRequirement  ConflictType = "stakeholder_requirement"
)

// Severity indicates how urgent a conflict is.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// ConflictContext provides the information needed to check for conflicts.
type ConflictContext struct {
	TaskID          string
	ProposedChanges string
	AffectedFiles   []string
}

// Conflict represents a detected conflict between proposed changes and
// existing decisions, ADRs, or stakeholder requirements.
type Conflict struct {
	Type           ConflictType
	Source         string
	Description    string
	Recommendation string
	Severity       Severity
}

// ConflictDetector defines the interface for checking proposed changes
// against existing ADRs, decisions, and requirements.
type ConflictDetector interface {
	CheckForConflicts(ctx ConflictContext) ([]Conflict, error)
}

// conflictDetector implements ConflictDetector by scanning docs/decisions/
// and docs/wiki/ for existing ADRs and requirements, then comparing them
// against the proposed changes.
type conflictDetector struct {
	basePath string
}

// NewConflictDetector creates a new ConflictDetector rooted at basePath.
func NewConflictDetector(basePath string) ConflictDetector {
	return &conflictDetector{basePath: basePath}
}

// CheckForConflicts scans existing ADRs, previous task decisions, and
// stakeholder requirements for potential conflicts with the proposed changes.
func (cd *conflictDetector) CheckForConflicts(ctx ConflictContext) ([]Conflict, error) {
	var conflicts []Conflict

	adrConflicts, err := cd.checkADRConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking ADR conflicts: %w", err)
	}
	conflicts = append(conflicts, adrConflicts...)

	decisionConflicts, err := cd.checkPreviousDecisions(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking previous decisions: %w", err)
	}
	conflicts = append(conflicts, decisionConflicts...)

	reqConflicts, err := cd.checkStakeholderRequirements(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking stakeholder requirements: %w", err)
	}
	conflicts = append(conflicts, reqConflicts...)

	return conflicts, nil
}

// checkADRConflicts reads ADR files from docs/decisions/ and checks
// whether any accepted ADR decisions conflict with the proposed changes.
func (cd *conflictDetector) checkADRConflicts(ctx ConflictContext) ([]Conflict, error) {
	decisionsDir := filepath.Join(cd.basePath, "docs", "decisions")
	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading decisions directory: %w", err)
	}

	var conflicts []Conflict
	proposedLower := strings.ToLower(ctx.ProposedChanges)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(decisionsDir, entry.Name())
		content, err := os.ReadFile(filePath) //nolint:gosec // G304: reading ADR files from managed decisions directory
		if err != nil {
			continue
		}

		adrContent := string(content)

		// Only check accepted ADRs.
		if !strings.Contains(adrContent, "**Status:** Accepted") {
			continue
		}

		decision := extractSection(adrContent, "## Decision")
		if decision == "" {
			continue
		}

		decisionLower := strings.ToLower(decision)

		// Check for keyword overlap between the proposed changes and the
		// ADR decision section. Keywords are significant words (>= 4 chars)
		// from the ADR decision.
		keywords := extractKeywords(decisionLower)
		matchCount := 0
		for _, kw := range keywords {
			if strings.Contains(proposedLower, kw) {
				matchCount++
			}
		}

		// If there's meaningful keyword overlap, flag as a potential conflict.
		if matchCount >= 2 {
			title := extractTitle(adrContent)
			conflicts = append(conflicts, Conflict{
				Type:           ConflictADRViolation,
				Source:         entry.Name(),
				Description:    fmt.Sprintf("Proposed changes may conflict with ADR %q: %s", title, truncate(decision, 200)),
				Recommendation: fmt.Sprintf("Review %s and verify the proposed changes align with the accepted decision.", entry.Name()),
				Severity:       SeverityHigh,
			})
		}
	}

	return conflicts, nil
}

// checkPreviousDecisions reads design.md files from other tasks' ticket
// folders and checks for conflicts with their recorded decisions.
func (cd *conflictDetector) checkPreviousDecisions(ctx ConflictContext) ([]Conflict, error) {
	ticketsDir := filepath.Join(cd.basePath, "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tickets directory: %w", err)
	}

	var conflicts []Conflict
	proposedLower := strings.ToLower(ctx.ProposedChanges)

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ctx.TaskID {
			continue
		}

		designPath := filepath.Join(ticketsDir, entry.Name(), "design.md")
		content, err := os.ReadFile(designPath) //nolint:gosec // G304: reading design docs from managed tickets directory
		if err != nil {
			continue
		}

		decisions := extractSection(string(content), "## Decisions")
		if decisions == "" {
			continue
		}

		decisionsLower := strings.ToLower(decisions)
		keywords := extractKeywords(decisionsLower)
		matchCount := 0
		for _, kw := range keywords {
			if strings.Contains(proposedLower, kw) {
				matchCount++
			}
		}

		if matchCount >= 2 {
			conflicts = append(conflicts, Conflict{
				Type:           ConflictPreviousDecision,
				Source:         entry.Name(),
				Description:    fmt.Sprintf("Proposed changes may conflict with decisions in task %s: %s", entry.Name(), truncate(decisions, 200)),
				Recommendation: fmt.Sprintf("Review the decisions recorded in %s/design.md before proceeding.", entry.Name()),
				Severity:       SeverityMedium,
			})
		}
	}

	return conflicts, nil
}

// checkStakeholderRequirements reads requirements files from docs/wiki/
// and checks whether proposed changes conflict with documented requirements.
func (cd *conflictDetector) checkStakeholderRequirements(ctx ConflictContext) ([]Conflict, error) {
	wikiDir := filepath.Join(cd.basePath, "docs", "wiki")
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading wiki directory: %w", err)
	}

	var conflicts []Conflict
	proposedLower := strings.ToLower(ctx.ProposedChanges)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(wikiDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		contentStr := string(content)
		contentLower := strings.ToLower(contentStr)

		// Look for requirement-like content.
		keywords := extractKeywords(contentLower)
		matchCount := 0
		for _, kw := range keywords {
			if strings.Contains(proposedLower, kw) {
				matchCount++
			}
		}

		if matchCount >= 2 {
			title := extractTitle(contentStr)
			if title == "" {
				title = entry.Name()
			}
			conflicts = append(conflicts, Conflict{
				Type:           ConflictStakeholderRequirement,
				Source:         entry.Name(),
				Description:    fmt.Sprintf("Proposed changes may conflict with stakeholder requirement %q in %s", title, entry.Name()),
				Recommendation: fmt.Sprintf("Review %s and confirm the proposed changes satisfy the documented requirements.", entry.Name()),
				Severity:       SeverityMedium,
			})
		}
	}

	return conflicts, nil
}

// extractSection extracts the content of a markdown section identified by
// its heading. Content goes from the heading to the next heading of the
// same or higher level, or end of file.
func extractSection(content, heading string) string {
	idx := strings.Index(content, heading)
	if idx < 0 {
		return ""
	}

	start := idx + len(heading)
	rest := content[start:]

	// Find the next heading of the same or higher level.
	level := strings.Count(heading, "#")
	lines := strings.Split(rest, "\n")
	var section []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && trimmed[0] == '#' {
			headingLevel := 0
			for _, ch := range trimmed {
				if ch == '#' {
					headingLevel++
				} else {
					break
				}
			}
			if headingLevel > 0 && headingLevel <= level {
				break
			}
		}
		section = append(section, line)
	}

	return strings.TrimSpace(strings.Join(section, "\n"))
}

// extractTitle pulls the first markdown heading from the content.
func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimPrefix(trimmed, "# ")
		}
	}
	return ""
}

// extractKeywords returns significant words (>= 4 characters, lowercased)
// from the input text, excluding common stop words.
func extractKeywords(text string) []string {
	words := strings.Fields(text)
	seen := make(map[string]struct{})
	var keywords []string

	for _, w := range words {
		// Strip markdown and punctuation.
		w = strings.Trim(w, "[](){}|*_`#-:;,.\"/!?")
		w = strings.ToLower(w)
		if len(w) < 4 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		keywords = append(keywords, w)
	}

	return keywords
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stopWords contains common English words that should be ignored during
// keyword matching to reduce false positives.
var stopWords = map[string]bool{
	"that": true, "this": true, "with": true, "from": true,
	"will": true, "have": true, "been": true, "were": true,
	"they": true, "them": true, "then": true, "than": true,
	"what": true, "when": true, "where": true, "which": true,
	"would": true, "could": true, "should": true, "shall": true,
	"about": true, "after": true, "before": true, "between": true,
	"into": true, "through": true, "during": true, "each": true,
	"also": true, "some": true, "other": true, "more": true,
	"there": true, "their": true, "these": true, "those": true,
	"being": true, "does": true, "done": true, "make": true,
	"made": true, "just": true, "only": true, "such": true,
	"like": true, "over": true, "under": true, "because": true,
}

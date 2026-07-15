package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// Conflict represents a conflict between proposed changes and existing decisions
type Conflict struct {
	Type        string // "decision", "adr", "constraint"
	Severity    string // "low", "medium", "high", "critical"
	Description string
	Source      string // task ID or ADR ID that contains the conflicting decision
	Suggestion  string
}

// ConflictDetector checks proposed changes against existing ADRs and decisions
type ConflictDetector struct {
	basePath           string
	knowledgeExtractor *KnowledgeExtractor
}

// NewConflictDetector creates a new conflict detector
func NewConflictDetector(basePath string) *ConflictDetector {
	return &ConflictDetector{
		basePath:           basePath,
		knowledgeExtractor: NewKnowledgeExtractor(basePath),
	}
}

// CheckProposedChanges checks if proposed changes conflict with existing decisions
func (cd *ConflictDetector) CheckProposedChanges(changes []string) ([]Conflict, error) {
	var conflicts []Conflict

	// Load all existing decisions from all tasks
	allDecisions, err := cd.loadAllDecisions()
	if err != nil {
		return nil, fmt.Errorf("failed to load decisions: %w", err)
	}

	// Load ADRs if they exist
	adrDecisions, err := cd.loadADRs()
	if err != nil {
		// ADRs are optional, so we don't fail if they don't exist
		adrDecisions = []ADR{}
	}

	// Check each change against decisions
	for _, change := range changes {
		// Check against task decisions
		for taskID, knowledge := range allDecisions {
			for _, decision := range knowledge.Decisions {
				if !decision.IsActive() {
					continue
				}

				// Simple keyword matching for conflict detection
				if cd.hasConflict(change, decision) {
					conflicts = append(conflicts, Conflict{
						Type:        "decision",
						Severity:    cd.determineSeverity(decision),
						Description: fmt.Sprintf("Proposed change may conflict with decision: %s", decision.Title),
						Source:      taskID,
						Suggestion:  decision.Rationale,
					})
				}
			}
		}

		// Check against ADRs
		for _, adr := range adrDecisions {
			if adr.Status != "accepted" && adr.Status != "proposed" {
				continue
			}

			if cd.hasADRConflict(change, adr) {
				conflicts = append(conflicts, Conflict{
					Type:        "adr",
					Severity:    "high",
					Description: fmt.Sprintf("Proposed change may conflict with ADR: %s", adr.Title),
					Source:      adr.ID,
					Suggestion:  adr.Context,
				})
			}
		}
	}

	return conflicts, nil
}

// CheckTaskChanges checks if changes in a task conflict with existing decisions
func (cd *ConflictDetector) CheckTaskChanges(taskID string, changes []string) ([]Conflict, error) {
	// Load current task knowledge
	currentKnowledge, err := cd.knowledgeExtractor.LoadKnowledge(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to load task knowledge: %w", err)
	}

	// Exclude current task from conflict checking
	allDecisions, err := cd.loadAllDecisions()
	if err != nil {
		return nil, fmt.Errorf("failed to load decisions: %w", err)
	}

	delete(allDecisions, taskID)

	var conflicts []Conflict

	// Check each change
	for _, change := range changes {
		for otherTaskID, knowledge := range allDecisions {
			for _, decision := range knowledge.Decisions {
				if !decision.IsActive() {
					continue
				}

				if cd.hasConflict(change, decision) {
					conflicts = append(conflicts, Conflict{
						Type:        "decision",
						Severity:    cd.determineSeverity(decision),
						Description: fmt.Sprintf("Change conflicts with decision from %s: %s", otherTaskID, decision.Title),
						Source:      otherTaskID,
						Suggestion:  decision.Rationale,
					})
				}
			}
		}
	}

	// Also check against current task's own decisions
	for _, decision := range currentKnowledge.Decisions {
		if !decision.IsActive() {
			continue
		}

		for _, change := range changes {
			if cd.hasConflict(change, decision) {
				conflicts = append(conflicts, Conflict{
					Type:        "decision",
					Severity:    "low",
					Description: fmt.Sprintf("Change may conflict with own decision: %s", decision.Title),
					Source:      taskID,
					Suggestion:  "Review consistency with task's own decisions",
				})
			}
		}
	}

	return conflicts, nil
}

// Common words to skip in conflict detection
var commonWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "up": true, "about": true, "into": true,
	"through": true, "during": true, "before": true, "after": true, "above": true,
	"below": true, "between": true, "under": true, "again": true, "further": true,
	"then": true, "once": true, "here": true, "there": true, "when": true, "where": true,
	"why": true, "how": true, "all": true, "both": true, "each": true, "few": true,
	"more": true, "most": true, "other": true, "some": true, "such": true, "no": true,
	"nor": true, "not": true, "only": true, "own": true, "same": true, "so": true,
	"than": true, "too": true, "very": true, "can": true, "will": true, "just": true,
	"should": true, "now": true, "decision": true, "description": true,
}

// hasConflict checks if a change conflicts with a decision using simple keyword matching
func (cd *ConflictDetector) hasConflict(change string, decision models.Decision) bool {
	changeLower := strings.ToLower(change)

	// Extract words from decision title and description
	titleWords := strings.Fields(strings.ToLower(decision.Title))
	descWords := strings.Fields(strings.ToLower(decision.Description))

	// Combine all words
	allWords := append(titleWords, descWords...)

	// Add tags as keywords (tags are more significant)
	for _, tag := range decision.Tags {
		allWords = append(allWords, strings.ToLower(tag))
	}

	// Check if any significant word appears in the change
	for _, word := range allWords {
		// Skip very short words
		if len(word) <= 2 {
			continue
		}

		// Remove punctuation from word
		word = strings.Trim(word, ".,;:!?\"'")

		// Skip common words
		if commonWords[word] {
			continue
		}

		if word != "" && strings.Contains(changeLower, word) {
			return true
		}
	}

	return false
}

// hasADRConflict checks if a change conflicts with an ADR
func (cd *ConflictDetector) hasADRConflict(change string, adr ADR) bool {
	changeLower := strings.ToLower(change)

	// Extract words from ADR title and context
	titleWords := strings.Fields(strings.ToLower(adr.Title))
	contextWords := strings.Fields(strings.ToLower(adr.Context))

	// Combine all words
	allWords := append(titleWords, contextWords...)

	// Add tags as keywords
	for _, tag := range adr.Tags {
		allWords = append(allWords, strings.ToLower(tag))
	}

	// Check if any significant word appears in the change
	for _, word := range allWords {
		// Skip very short words
		if len(word) <= 2 {
			continue
		}

		// Remove punctuation from word
		word = strings.Trim(word, ".,;:!?\"'")

		// Skip common words
		if commonWords[word] {
			continue
		}

		if word != "" && strings.Contains(changeLower, word) {
			return true
		}
	}

	return false
}

// determineSeverity determines the severity of a conflict based on decision properties
func (cd *ConflictDetector) determineSeverity(decision models.Decision) string {
	// If decision is accepted, it's more severe
	if decision.Status == "accepted" {
		return "high"
	}

	// If decision has consequences listed, it's more important
	if len(decision.Consequences) > 0 {
		return "medium"
	}

	return "low"
}

// loadAllDecisions loads all decisions from all tasks
func (cd *ConflictDetector) loadAllDecisions() (map[string]*models.ExtractedKnowledge, error) {
	taskIDs, err := cd.knowledgeExtractor.ListAllKnowledge()
	if err != nil {
		return nil, err
	}

	allDecisions := make(map[string]*models.ExtractedKnowledge)
	for _, taskID := range taskIDs {
		knowledge, err := cd.knowledgeExtractor.LoadKnowledge(taskID)
		if err != nil {
			// Skip tasks with invalid knowledge files
			continue
		}

		allDecisions[taskID] = knowledge
	}

	return allDecisions, nil
}

// ADR represents an Architecture Decision Record
type ADR struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Status  string   `yaml:"status"`
	Context string   `yaml:"context"`
	Tags    []string `yaml:"tags,omitempty"`
}

// loadADRs loads ADRs from the docs/adr directory
func (cd *ConflictDetector) loadADRs() ([]ADR, error) {
	adrDir := filepath.Join(cd.basePath, "docs", "adr")

	// Check if ADR directory exists
	if _, err := os.Stat(adrDir); os.IsNotExist(err) {
		return []ADR{}, nil
	}

	entries, err := os.ReadDir(adrDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read ADR directory: %w", err)
	}

	var adrs []ADR
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .yaml and .yml files
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		adrPath := filepath.Join(adrDir, entry.Name())
		data, err := os.ReadFile(adrPath)
		if err != nil {
			continue
		}

		var adr ADR
		if err := yaml.Unmarshal(data, &adr); err != nil {
			continue
		}

		adrs = append(adrs, adr)
	}

	return adrs, nil
}

// GetConflictSummary returns a summary of conflicts grouped by severity
func (cd *ConflictDetector) GetConflictSummary(conflicts []Conflict) map[string][]Conflict {
	summary := make(map[string][]Conflict)
	summary["critical"] = []Conflict{}
	summary["high"] = []Conflict{}
	summary["medium"] = []Conflict{}
	summary["low"] = []Conflict{}

	for _, conflict := range conflicts {
		summary[conflict.Severity] = append(summary[conflict.Severity], conflict)
	}

	return summary
}

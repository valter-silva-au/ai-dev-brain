package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

// TestProperty_ConflictDetectionConsistency verifies conflict detection consistency
func TestProperty_ConflictDetectionConsistency(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		tempDir := filepath.Join(baseDir, suffix)
		cd := NewConflictDetector(tempDir)

		// Create knowledge directory matching the path expected by ListAllKnowledge:
		// basePath/tickets/TASK-XXXXX/knowledge/decisions.yaml
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		knowledgeDir := filepath.Join(tempDir, "tickets", taskID, "knowledge")
		if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
			t.Fatalf("Failed to create knowledge dir: %v", err)
		}

		// Add a decision
		keyword := rapid.StringMatching(`^[a-z]{3,10}$`).Draw(t, "keyword")

		knowledge := &models.ExtractedKnowledge{
			Decisions: []models.Decision{
				{
					Title:       "Use " + keyword,
					Description: "We decided to use " + keyword,
					Status:      "accepted",
					Tags:        []string{keyword},
				},
			},
		}

		knowledgePath := filepath.Join(knowledgeDir, "decisions.yaml")
		data, _ := yaml.Marshal(knowledge)
		if err := os.WriteFile(knowledgePath, data, 0o644); err != nil {
			t.Fatalf("Failed to write knowledge: %v", err)
		}

		// Check for conflicts with same keyword
		changes := []string{"We will use " + keyword + " for this"}
		conflicts, err := cd.CheckProposedChanges(changes)
		if err != nil {
			t.Fatalf("CheckProposedChanges failed: %v", err)
		}

		// Should detect conflict due to keyword match
		if len(conflicts) == 0 {
			t.Fatal("Should have detected conflict with matching keyword")
		}
	})
}

// TestProperty_ConflictDetectionNoFalsePositives verifies no false positives with unrelated changes
func TestProperty_ConflictDetectionNoFalsePositives(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		tempDir := filepath.Join(baseDir, suffix)
		cd := NewConflictDetector(tempDir)

		// Create knowledge directory matching the expected path:
		// basePath/tickets/TASK-XXXXX/knowledge/decisions.yaml
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		knowledgeDir := filepath.Join(tempDir, "tickets", taskID, "knowledge")
		if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
			t.Fatalf("Failed to create knowledge dir: %v", err)
		}

		// Add a decision with specific keyword
		knowledge := &models.ExtractedKnowledge{
			Decisions: []models.Decision{
				{
					Title:       "Use PostgreSQL",
					Description: "We decided to use PostgreSQL for database",
					Status:      "accepted",
					Tags:        []string{"database", "postgresql"},
				},
			},
		}

		knowledgePath := filepath.Join(knowledgeDir, "decisions.yaml")
		data, _ := yaml.Marshal(knowledge)
		if err := os.WriteFile(knowledgePath, data, 0o644); err != nil {
			t.Fatalf("Failed to write knowledge: %v", err)
		}

		// Check for conflicts with completely unrelated change
		unrelatedChanges := []string{"Update frontend styling", "Fix button alignment"}
		conflicts, err := cd.CheckProposedChanges(unrelatedChanges)
		if err != nil {
			t.Fatalf("CheckProposedChanges failed: %v", err)
		}

		// Should not detect false conflicts
		if len(conflicts) > 0 {
			t.Fatalf("Should not detect conflicts with unrelated changes, got %d", len(conflicts))
		}
	})
}

// TestProperty_ConflictSeverityClassification verifies severity classification
func TestProperty_ConflictSeverityClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cd := &ConflictDetector{}

		validSeverities := []string{"low", "medium", "high", "critical"}

		// Test with accepted decision
		decision := models.Decision{
			Status: "accepted",
		}
		severity := cd.determineSeverity(decision)
		if severity != "high" {
			t.Fatalf("Accepted decision should have high severity, got %s", severity)
		}

		// Test with consequences
		decision = models.Decision{
			Status:       "proposed",
			Consequences: []string{"consequence1"},
		}
		severity = cd.determineSeverity(decision)
		if severity != "medium" {
			t.Fatalf("Decision with consequences should have medium severity, got %s", severity)
		}

		// Test default
		decision = models.Decision{
			Status: "proposed",
		}
		severity = cd.determineSeverity(decision)

		// Verify severity is valid
		valid := false
		for _, s := range validSeverities {
			if severity == s {
				valid = true
				break
			}
		}
		if !valid {
			t.Fatalf("Invalid severity: %s", severity)
		}
	})
}

// TestProperty_ConflictSummaryGrouping verifies conflict grouping by severity
func TestProperty_ConflictSummaryGrouping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cd := &ConflictDetector{}

		numConflicts := rapid.IntRange(1, 20).Draw(t, "numConflicts")
		conflicts := make([]Conflict, numConflicts)

		severities := []string{"low", "medium", "high", "critical"}
		for i := 0; i < numConflicts; i++ {
			conflicts[i] = Conflict{
				Type:        "decision",
				Severity:    rapid.SampledFrom(severities).Draw(t, "severity"),
				Description: "test conflict",
			}
		}

		summary := cd.GetConflictSummary(conflicts)

		// Verify all severity levels are present in summary
		if _, ok := summary["low"]; !ok {
			t.Fatal("Summary missing 'low' severity level")
		}
		if _, ok := summary["medium"]; !ok {
			t.Fatal("Summary missing 'medium' severity level")
		}
		if _, ok := summary["high"]; !ok {
			t.Fatal("Summary missing 'high' severity level")
		}
		if _, ok := summary["critical"]; !ok {
			t.Fatal("Summary missing 'critical' severity level")
		}

		// Verify total count matches
		totalInSummary := len(summary["low"]) + len(summary["medium"]) + len(summary["high"]) + len(summary["critical"])
		if totalInSummary != numConflicts {
			t.Fatalf("Expected %d total conflicts in summary, got %d", numConflicts, totalInSummary)
		}
	})
}

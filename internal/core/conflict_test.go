package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

func TestNewConflictDetector(t *testing.T) {
	tmpDir := t.TempDir()

	detector := NewConflictDetector(tmpDir)
	if detector == nil {
		t.Fatal("NewConflictDetector() returned nil")
	}

	if detector.basePath != tmpDir {
		t.Errorf("basePath = %v, want %v", detector.basePath, tmpDir)
	}

	if detector.knowledgeExtractor == nil {
		t.Error("knowledgeExtractor should not be nil")
	}
}

func TestConflictDetector_CheckProposedChanges(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	// Setup: Create a task with decisions
	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(filepath.Join(taskDir, "knowledge"), 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	knowledge := models.NewExtractedKnowledge(taskID)
	decision := models.Decision{
		ID:          "DEC-001",
		Title:       "Use PostgreSQL",
		Description: "Decision to use PostgreSQL for database",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
		Tags:        []string{"database", "postgresql"},
	}
	knowledge.AddDecision(decision)

	extractor := NewKnowledgeExtractor(tmpDir)
	if err := extractor.SaveKnowledge(taskID, knowledge); err != nil {
		t.Fatalf("Failed to save knowledge: %v", err)
	}

	tests := []struct {
		name          string
		changes       []string
		wantConflicts bool
		minConflicts  int
	}{
		{
			name:          "No conflicts",
			changes:       []string{"update frontend code", "fix bug in UI"},
			wantConflicts: false,
			minConflicts:  0,
		},
		{
			name:          "Conflict with decision",
			changes:       []string{"switch to MySQL database", "update postgresql connection"},
			wantConflicts: true,
			minConflicts:  1,
		},
		{
			name:          "Conflict with tag",
			changes:       []string{"refactor database layer", "add new postgresql feature"},
			wantConflicts: true,
			minConflicts:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts, err := detector.CheckProposedChanges(tt.changes)
			if err != nil {
				t.Fatalf("CheckProposedChanges() error = %v", err)
			}

			hasConflicts := len(conflicts) > 0
			if hasConflicts != tt.wantConflicts {
				t.Errorf("CheckProposedChanges() hasConflicts = %v, want %v", hasConflicts, tt.wantConflicts)
			}

			if tt.wantConflicts && len(conflicts) < tt.minConflicts {
				t.Errorf("Expected at least %d conflicts, got %d", tt.minConflicts, len(conflicts))
			}
		})
	}
}

func TestConflictDetector_CheckTaskChanges(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	// Setup: Create two tasks with decisions
	task1ID := "TASK-001"
	task1Dir := filepath.Join(tmpDir, "tickets", task1ID)
	if err := os.MkdirAll(filepath.Join(task1Dir, "knowledge"), 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	knowledge1 := models.NewExtractedKnowledge(task1ID)
	decision1 := models.Decision{
		ID:          "DEC-001",
		Title:       "Use REST API",
		Description: "Use REST for API communication",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
	}
	knowledge1.AddDecision(decision1)

	extractor := NewKnowledgeExtractor(tmpDir)
	if err := extractor.SaveKnowledge(task1ID, knowledge1); err != nil {
		t.Fatalf("Failed to save knowledge: %v", err)
	}

	// Create second task
	task2ID := "TASK-002"
	task2Dir := filepath.Join(tmpDir, "tickets", task2ID)
	if err := os.MkdirAll(filepath.Join(task2Dir, "knowledge"), 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	knowledge2 := models.NewExtractedKnowledge(task2ID)
	decision2 := models.Decision{
		ID:          "DEC-002",
		Title:       "GraphQL Schema",
		Description: "Define GraphQL schema",
		Status:      "proposed",
		DecidedAt:   time.Now().UTC(),
	}
	knowledge2.AddDecision(decision2)

	if err := extractor.SaveKnowledge(task2ID, knowledge2); err != nil {
		t.Fatalf("Failed to save knowledge: %v", err)
	}

	tests := []struct {
		name          string
		taskID        string
		changes       []string
		wantConflicts bool
	}{
		{
			name:          "No conflicts",
			taskID:        task2ID,
			changes:       []string{"update documentation", "add tests"},
			wantConflicts: false,
		},
		{
			name:          "Conflict with other task decision",
			taskID:        task2ID,
			changes:       []string{"switch from REST to GraphQL", "update REST endpoints"},
			wantConflicts: true,
		},
		{
			name:          "Conflict with own decision",
			taskID:        task2ID,
			changes:       []string{"remove graphql completely", "add graphql mutation"},
			wantConflicts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts, err := detector.CheckTaskChanges(tt.taskID, tt.changes)
			if err != nil {
				t.Fatalf("CheckTaskChanges() error = %v", err)
			}

			hasConflicts := len(conflicts) > 0
			if hasConflicts != tt.wantConflicts {
				t.Errorf("CheckTaskChanges() hasConflicts = %v, want %v", hasConflicts, tt.wantConflicts)
			}
		})
	}
}

func TestConflictDetector_CheckProposedChanges_WithADRs(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	// Setup: Create ADR directory and file
	adrDir := filepath.Join(tmpDir, "docs", "adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("Failed to create ADR dir: %v", err)
	}

	adr := ADR{
		ID:      "ADR-001",
		Title:   "Use Microservices Architecture",
		Status:  "accepted",
		Context: "We decided to use microservices for better scalability",
		Tags:    []string{"architecture", "microservices"},
	}

	adrData, err := yaml.Marshal(adr)
	if err != nil {
		t.Fatalf("Failed to marshal ADR: %v", err)
	}

	adrPath := filepath.Join(adrDir, "001-microservices.yaml")
	if err := os.WriteFile(adrPath, adrData, 0o644); err != nil {
		t.Fatalf("Failed to write ADR: %v", err)
	}

	tests := []struct {
		name          string
		changes       []string
		wantConflicts bool
	}{
		{
			name:          "No conflict with ADR",
			changes:       []string{"update frontend", "add new feature"},
			wantConflicts: false,
		},
		{
			name:          "Conflict with ADR",
			changes:       []string{"migrate to monolithic architecture", "remove microservices"},
			wantConflicts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts, err := detector.CheckProposedChanges(tt.changes)
			if err != nil {
				t.Fatalf("CheckProposedChanges() error = %v", err)
			}

			hasConflicts := len(conflicts) > 0
			if hasConflicts != tt.wantConflicts {
				t.Errorf("CheckProposedChanges() hasConflicts = %v, want %v (conflicts: %+v)", hasConflicts, tt.wantConflicts, conflicts)
			}

			// If we expect conflicts, verify at least one is from an ADR
			if tt.wantConflicts {
				hasADRConflict := false
				for _, conflict := range conflicts {
					if conflict.Type == "adr" {
						hasADRConflict = true
						break
					}
				}
				if !hasADRConflict {
					t.Error("Expected at least one ADR conflict")
				}
			}
		})
	}
}

func TestConflictDetector_DetermineSeverity(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	tests := []struct {
		name         string
		decision     models.Decision
		wantSeverity string
	}{
		{
			name: "Accepted decision - high severity",
			decision: models.Decision{
				Status: "accepted",
			},
			wantSeverity: "high",
		},
		{
			name: "Proposed decision with consequences - medium severity",
			decision: models.Decision{
				Status:       "proposed",
				Consequences: []string{"Impact on performance"},
			},
			wantSeverity: "medium",
		},
		{
			name: "Proposed decision without consequences - low severity",
			decision: models.Decision{
				Status:       "proposed",
				Consequences: []string{},
			},
			wantSeverity: "low",
		},
		{
			name: "Rejected decision - low severity",
			decision: models.Decision{
				Status: "rejected",
			},
			wantSeverity: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity := detector.determineSeverity(tt.decision)
			if severity != tt.wantSeverity {
				t.Errorf("determineSeverity() = %v, want %v", severity, tt.wantSeverity)
			}
		})
	}
}

func TestConflictDetector_GetConflictSummary(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	conflicts := []Conflict{
		{Type: "decision", Severity: "critical", Description: "Critical conflict"},
		{Type: "decision", Severity: "high", Description: "High conflict 1"},
		{Type: "adr", Severity: "high", Description: "High conflict 2"},
		{Type: "decision", Severity: "medium", Description: "Medium conflict"},
		{Type: "decision", Severity: "low", Description: "Low conflict 1"},
		{Type: "decision", Severity: "low", Description: "Low conflict 2"},
	}

	summary := detector.GetConflictSummary(conflicts)

	if len(summary["critical"]) != 1 {
		t.Errorf("Expected 1 critical conflict, got %d", len(summary["critical"]))
	}

	if len(summary["high"]) != 2 {
		t.Errorf("Expected 2 high conflicts, got %d", len(summary["high"]))
	}

	if len(summary["medium"]) != 1 {
		t.Errorf("Expected 1 medium conflict, got %d", len(summary["medium"]))
	}

	if len(summary["low"]) != 2 {
		t.Errorf("Expected 2 low conflicts, got %d", len(summary["low"]))
	}
}

func TestConflictDetector_HasConflict(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	decision := models.Decision{
		Title:       "Use PostgreSQL",
		Description: "Database decision",
		Tags:        []string{"database", "sql"},
	}

	tests := []struct {
		name         string
		change       string
		wantConflict bool
	}{
		{
			name:         "Exact match in change",
			change:       "Update PostgreSQL configuration",
			wantConflict: true,
		},
		{
			name:         "Partial match (case insensitive)",
			change:       "Fix postgresql connection issue",
			wantConflict: true,
		},
		{
			name:         "Match with tag",
			change:       "Refactor database layer",
			wantConflict: true,
		},
		{
			name:         "No match",
			change:       "Update frontend components",
			wantConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasConflict := detector.hasConflict(tt.change, decision)
			if hasConflict != tt.wantConflict {
				t.Errorf("hasConflict() = %v, want %v", hasConflict, tt.wantConflict)
			}
		})
	}
}

func TestConflictDetector_HasADRConflict(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	adr := ADR{
		Title:   "Use REST API",
		Context: "We chose REST over GraphQL for simplicity",
	}

	tests := []struct {
		name         string
		change       string
		wantConflict bool
	}{
		{
			name:         "Match with title",
			change:       "Migrate REST API to gRPC",
			wantConflict: true,
		},
		{
			name:         "Match with context",
			change:       "Implement GraphQL endpoint",
			wantConflict: true,
		},
		{
			name:         "No match",
			change:       "Update documentation",
			wantConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasConflict := detector.hasADRConflict(tt.change, adr)
			if hasConflict != tt.wantConflict {
				t.Errorf("hasADRConflict() = %v, want %v", hasConflict, tt.wantConflict)
			}
		})
	}
}

func TestConflictDetector_LoadADRs(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	t.Run("No ADR directory", func(t *testing.T) {
		adrs, err := detector.loadADRs()
		if err != nil {
			t.Errorf("loadADRs() error = %v, want nil", err)
		}

		if len(adrs) != 0 {
			t.Errorf("Expected empty ADR list, got %d", len(adrs))
		}
	})

	t.Run("Load ADRs from directory", func(t *testing.T) {
		adrDir := filepath.Join(tmpDir, "docs", "adr")
		if err := os.MkdirAll(adrDir, 0o755); err != nil {
			t.Fatalf("Failed to create ADR dir: %v", err)
		}

		// Create multiple ADRs
		adrs := []ADR{
			{ID: "ADR-001", Title: "First ADR", Status: "accepted"},
			{ID: "ADR-002", Title: "Second ADR", Status: "proposed"},
		}

		for i, adr := range adrs {
			data, err := yaml.Marshal(adr)
			if err != nil {
				t.Fatalf("Failed to marshal ADR: %v", err)
			}

			path := filepath.Join(adrDir, fmt.Sprintf("%03d-adr.yaml", i+1))
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatalf("Failed to write ADR: %v", err)
			}
		}

		// Create a non-YAML file (should be ignored)
		txtPath := filepath.Join(adrDir, "readme.txt")
		if err := os.WriteFile(txtPath, []byte("text"), 0o644); err != nil {
			t.Fatalf("Failed to write text file: %v", err)
		}

		loaded, err := detector.loadADRs()
		if err != nil {
			t.Fatalf("loadADRs() error = %v", err)
		}

		if len(loaded) != 2 {
			t.Errorf("Expected 2 ADRs, got %d", len(loaded))
		}
	})
}

func TestConflictDetector_LoadAllDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	// Create multiple tasks with decisions
	taskIDs := []string{"TASK-001", "TASK-002"}
	for _, taskID := range taskIDs {
		taskDir := filepath.Join(tmpDir, "tickets", taskID)
		if err := os.MkdirAll(filepath.Join(taskDir, "knowledge"), 0o755); err != nil {
			t.Fatalf("Failed to create task dir: %v", err)
		}

		knowledge := models.NewExtractedKnowledge(taskID)
		decision := models.Decision{
			ID:        fmt.Sprintf("DEC-%s", taskID),
			Title:     fmt.Sprintf("Decision for %s", taskID),
			Status:    "accepted",
			DecidedAt: time.Now().UTC(),
		}
		knowledge.AddDecision(decision)

		extractor := NewKnowledgeExtractor(tmpDir)
		if err := extractor.SaveKnowledge(taskID, knowledge); err != nil {
			t.Fatalf("Failed to save knowledge: %v", err)
		}
	}

	allDecisions, err := detector.loadAllDecisions()
	if err != nil {
		t.Fatalf("loadAllDecisions() error = %v", err)
	}

	if len(allDecisions) != 2 {
		t.Errorf("Expected 2 tasks with decisions, got %d", len(allDecisions))
	}

	for _, taskID := range taskIDs {
		if _, ok := allDecisions[taskID]; !ok {
			t.Errorf("Expected task %s in decisions map", taskID)
		}
	}
}

func TestConflictDetector_SkipInactiveDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	detector := NewConflictDetector(tmpDir)

	// Create task with multiple decisions including inactive ones
	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(filepath.Join(taskDir, "knowledge"), 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	knowledge := models.NewExtractedKnowledge(taskID)

	// Active decision
	activeDecision := models.Decision{
		ID:          "DEC-001",
		Title:       "Active Decision",
		Description: "active decision description",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
	}
	knowledge.AddDecision(activeDecision)

	// Inactive decision (rejected)
	inactiveDecision := models.Decision{
		ID:          "DEC-002",
		Title:       "Rejected Decision",
		Description: "rejected decision description",
		Status:      "rejected",
		DecidedAt:   time.Now().UTC(),
	}
	knowledge.AddDecision(inactiveDecision)

	extractor := NewKnowledgeExtractor(tmpDir)
	if err := extractor.SaveKnowledge(taskID, knowledge); err != nil {
		t.Fatalf("Failed to save knowledge: %v", err)
	}

	// Check for conflicts - should only conflict with active decision
	changes := []string{"update active decision"}

	conflicts, err := detector.CheckProposedChanges(changes)
	if err != nil {
		t.Fatalf("CheckProposedChanges() error = %v", err)
	}

	// Should find conflict with active decision
	if len(conflicts) == 0 {
		t.Error("Expected conflict with active decision")
	}

	// Verify no conflicts with rejected decision
	changes2 := []string{"update rejected decision"}
	conflicts2, err := detector.CheckProposedChanges(changes2)
	if err != nil {
		t.Fatalf("CheckProposedChanges() error = %v", err)
	}

	// Should not find conflict since decision is rejected
	if len(conflicts2) > 0 {
		t.Error("Should not conflict with rejected decision")
	}
}

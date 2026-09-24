package models

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewDecision(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		title       string
		description string
	}{
		{"creates decision", "D-001", "Use PostgreSQL", "Database choice"},
		{"creates another decision", "D-002", "Use REST API", "API style"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := NewDecision(tt.id, tt.title, tt.description)

			if decision.ID != tt.id {
				t.Errorf("ID = %v, want %v", decision.ID, tt.id)
			}
			if decision.Title != tt.title {
				t.Errorf("Title = %v, want %v", decision.Title, tt.title)
			}
			if decision.Description != tt.description {
				t.Errorf("Description = %v, want %v", decision.Description, tt.description)
			}
			if decision.Status != "proposed" {
				t.Errorf("Status = %v, want proposed", decision.Status)
			}
			if decision.DecidedAt.IsZero() {
				t.Error("DecidedAt should be set")
			}
			if decision.DecidedAt.Location() != time.UTC {
				t.Error("DecidedAt should be in UTC")
			}
		})
	}
}

func TestDecision_IsAccepted(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"accepted is accepted", "accepted", true},
		{"proposed is not accepted", "proposed", false},
		{"rejected is not accepted", "rejected", false},
		{"deprecated is not accepted", "deprecated", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := &Decision{Status: tt.status}
			if got := decision.IsAccepted(); got != tt.want {
				t.Errorf("IsAccepted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecision_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"proposed is active", "proposed", true},
		{"accepted is active", "accepted", true},
		{"rejected is not active", "rejected", false},
		{"deprecated is not active", "deprecated", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := &Decision{Status: tt.status}
			if got := decision.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewExtractedKnowledge(t *testing.T) {
	taskID := "TASK-001"
	knowledge := NewExtractedKnowledge(taskID)

	if knowledge.TaskID != taskID {
		t.Errorf("TaskID = %v, want %v", knowledge.TaskID, taskID)
	}
	if knowledge.ExtractedAt.IsZero() {
		t.Error("ExtractedAt should be set")
	}
	if knowledge.ExtractedAt.Location() != time.UTC {
		t.Error("ExtractedAt should be in UTC")
	}
	if knowledge.Decisions == nil {
		t.Error("Decisions should be initialized")
	}
	if knowledge.Learnings == nil {
		t.Error("Learnings should be initialized")
	}
	if knowledge.Gotchas == nil {
		t.Error("Gotchas should be initialized")
	}
}

func TestExtractedKnowledge_AddDecision(t *testing.T) {
	knowledge := NewExtractedKnowledge("TASK-001")
	decision := Decision{
		ID:          "D-001",
		Title:       "Test",
		Description: "Test decision",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
	}

	knowledge.AddDecision(decision)

	if len(knowledge.Decisions) != 1 {
		t.Errorf("Decisions length = %v, want 1", len(knowledge.Decisions))
	}
	if knowledge.Decisions[0].ID != decision.ID {
		t.Errorf("Decision ID = %v, want %v", knowledge.Decisions[0].ID, decision.ID)
	}
}

func TestExtractedKnowledge_AddLearning(t *testing.T) {
	knowledge := NewExtractedKnowledge("TASK-001")
	learning := Learning{
		Title:       "Test Learning",
		Description: "Learned something",
		Category:    "technical",
		Timestamp:   time.Now().UTC(),
	}

	knowledge.AddLearning(learning)

	if len(knowledge.Learnings) != 1 {
		t.Errorf("Learnings length = %v, want 1", len(knowledge.Learnings))
	}
	if knowledge.Learnings[0].Title != learning.Title {
		t.Errorf("Learning Title = %v, want %v", knowledge.Learnings[0].Title, learning.Title)
	}
}

func TestExtractedKnowledge_AddGotcha(t *testing.T) {
	knowledge := NewExtractedKnowledge("TASK-001")
	gotcha := Gotcha{
		Title:       "Test Gotcha",
		Description: "Watch out for this",
		Solution:    "Do this instead",
		Severity:    "high",
		Timestamp:   time.Now().UTC(),
	}

	knowledge.AddGotcha(gotcha)

	if len(knowledge.Gotchas) != 1 {
		t.Errorf("Gotchas length = %v, want 1", len(knowledge.Gotchas))
	}
	if knowledge.Gotchas[0].Title != gotcha.Title {
		t.Errorf("Gotcha Title = %v, want %v", knowledge.Gotchas[0].Title, gotcha.Title)
	}
}

func TestNewHandoffDocument(t *testing.T) {
	taskID := "TASK-001"
	taskTitle := "Test Task"
	handoff := NewHandoffDocument(taskID, taskTitle)

	if handoff.TaskID != taskID {
		t.Errorf("TaskID = %v, want %v", handoff.TaskID, taskID)
	}
	if handoff.TaskTitle != taskTitle {
		t.Errorf("TaskTitle = %v, want %v", handoff.TaskTitle, taskTitle)
	}
	if handoff.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should be set")
	}
	if handoff.GeneratedAt.Location() != time.UTC {
		t.Error("GeneratedAt should be in UTC")
	}

	// Verify all slices are initialized
	if handoff.Objectives == nil {
		t.Error("Objectives should be initialized")
	}
	if handoff.Completed == nil {
		t.Error("Completed should be initialized")
	}
	if handoff.NotCompleted == nil {
		t.Error("NotCompleted should be initialized")
	}
	if handoff.Decisions == nil {
		t.Error("Decisions should be initialized")
	}
	if handoff.KeyLearnings == nil {
		t.Error("KeyLearnings should be initialized")
	}
	if handoff.Gotchas == nil {
		t.Error("Gotchas should be initialized")
	}
}

func TestDecision_YAMLSerialization(t *testing.T) {
	decision := &Decision{
		ID:           "D-001",
		Title:        "Use PostgreSQL",
		Description:  "Database choice for the project",
		Context:      "Need a relational database",
		Rationale:    "Strong ACID guarantees",
		Alternatives: []string{"MySQL", "MongoDB"},
		Consequences: []string{"Need to manage migrations"},
		Status:       "accepted",
		DecidedBy:    "team-lead",
		DecidedAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Tags:         []string{"database", "backend"},
		RelatedTo:    []string{"TASK-001"},
	}

	data, err := yaml.Marshal(decision)
	if err != nil {
		t.Fatalf("Failed to marshal decision: %v", err)
	}

	var decoded Decision
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal decision: %v", err)
	}

	if decoded.ID != decision.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, decision.ID)
	}
	if decoded.Title != decision.Title {
		t.Errorf("Title = %v, want %v", decoded.Title, decision.Title)
	}
	if decoded.Status != decision.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, decision.Status)
	}
	if len(decoded.Alternatives) != len(decision.Alternatives) {
		t.Errorf("Alternatives length = %v, want %v", len(decoded.Alternatives), len(decision.Alternatives))
	}
}

func TestExtractedKnowledge_YAMLSerialization(t *testing.T) {
	knowledge := &ExtractedKnowledge{
		TaskID:      "TASK-001",
		ExtractedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Decisions: []Decision{
			{
				ID:        "D-001",
				Title:     "Test Decision",
				Status:    "accepted",
				DecidedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		Learnings: []Learning{
			{
				Title:       "Test Learning",
				Description: "Learned something",
				Category:    "technical",
				Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		Gotchas: []Gotcha{
			{
				Title:     "Test Gotcha",
				Severity:  "medium",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		References: []string{"doc1.md", "doc2.md"},
		Summary:    "Test summary",
	}

	data, err := yaml.Marshal(knowledge)
	if err != nil {
		t.Fatalf("Failed to marshal knowledge: %v", err)
	}

	var decoded ExtractedKnowledge
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal knowledge: %v", err)
	}

	if decoded.TaskID != knowledge.TaskID {
		t.Errorf("TaskID = %v, want %v", decoded.TaskID, knowledge.TaskID)
	}
	if len(decoded.Decisions) != len(knowledge.Decisions) {
		t.Errorf("Decisions length = %v, want %v", len(decoded.Decisions), len(knowledge.Decisions))
	}
	if len(decoded.Learnings) != len(knowledge.Learnings) {
		t.Errorf("Learnings length = %v, want %v", len(decoded.Learnings), len(knowledge.Learnings))
	}
	if len(decoded.Gotchas) != len(knowledge.Gotchas) {
		t.Errorf("Gotchas length = %v, want %v", len(decoded.Gotchas), len(knowledge.Gotchas))
	}
}

func TestHandoffDocument_YAMLSerialization(t *testing.T) {
	handoff := &HandoffDocument{
		TaskID:       "TASK-001",
		TaskTitle:    "Implement feature X",
		GeneratedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:      "Completed feature X implementation",
		Objectives:   []string{"Objective 1", "Objective 2"},
		Completed:    []string{"Item 1", "Item 2"},
		NotCompleted: []string{"Item 3"},
		Decisions: []Decision{
			{
				ID:        "D-001",
				Title:     "Test",
				Status:    "accepted",
				DecidedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		KeyLearnings: []Learning{
			{
				Title:     "Learning 1",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		Gotchas: []Gotcha{
			{
				Title:     "Gotcha 1",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		OpenItems:      []string{"Open 1"},
		NextSteps:      []string{"Step 1"},
		References:     []string{"ref1.md"},
		FilesModified:  []string{"file.go"},
		TestsAdded:     []string{"test.go"},
		Dependencies:   []string{"TASK-002"},
		ArchivedReason: "Feature completed",
		Owner:          "user@example.com",
		Reviewers:      []string{"reviewer1", "reviewer2"},
	}

	data, err := yaml.Marshal(handoff)
	if err != nil {
		t.Fatalf("Failed to marshal handoff: %v", err)
	}

	var decoded HandoffDocument
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal handoff: %v", err)
	}

	if decoded.TaskID != handoff.TaskID {
		t.Errorf("TaskID = %v, want %v", decoded.TaskID, handoff.TaskID)
	}
	if decoded.TaskTitle != handoff.TaskTitle {
		t.Errorf("TaskTitle = %v, want %v", decoded.TaskTitle, handoff.TaskTitle)
	}
	if decoded.Summary != handoff.Summary {
		t.Errorf("Summary = %v, want %v", decoded.Summary, handoff.Summary)
	}
	if len(decoded.Objectives) != len(handoff.Objectives) {
		t.Errorf("Objectives length = %v, want %v", len(decoded.Objectives), len(handoff.Objectives))
	}
}

func TestLearning_YAMLSerialization(t *testing.T) {
	learning := Learning{
		Title:       "Test Learning",
		Description: "Description",
		Category:    "technical",
		Tags:        []string{"go", "testing"},
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := yaml.Marshal(learning)
	if err != nil {
		t.Fatalf("Failed to marshal learning: %v", err)
	}

	var decoded Learning
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal learning: %v", err)
	}

	if decoded.Title != learning.Title {
		t.Errorf("Title = %v, want %v", decoded.Title, learning.Title)
	}
	if decoded.Category != learning.Category {
		t.Errorf("Category = %v, want %v", decoded.Category, learning.Category)
	}
}

func TestGotcha_YAMLSerialization(t *testing.T) {
	gotcha := Gotcha{
		Title:       "Test Gotcha",
		Description: "Description",
		Solution:    "Solution",
		Prevention:  "Prevention",
		Severity:    "high",
		Tags:        []string{"security"},
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := yaml.Marshal(gotcha)
	if err != nil {
		t.Fatalf("Failed to marshal gotcha: %v", err)
	}

	var decoded Gotcha
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal gotcha: %v", err)
	}

	if decoded.Title != gotcha.Title {
		t.Errorf("Title = %v, want %v", decoded.Title, gotcha.Title)
	}
	if decoded.Severity != gotcha.Severity {
		t.Errorf("Severity = %v, want %v", decoded.Severity, gotcha.Severity)
	}
	if decoded.Solution != gotcha.Solution {
		t.Errorf("Solution = %v, want %v", decoded.Solution, gotcha.Solution)
	}
}

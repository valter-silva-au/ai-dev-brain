package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestNewKnowledgeExtractor(t *testing.T) {
	tmpDir := t.TempDir()

	extractor := NewKnowledgeExtractor(tmpDir)
	if extractor == nil {
		t.Fatal("NewKnowledgeExtractor() returned nil")
	}

	if extractor.basePath != tmpDir {
		t.Errorf("basePath = %v, want %v", extractor.basePath, tmpDir)
	}
}

func TestKnowledgeExtractor_ExtractFromTask(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	tests := []struct {
		name         string
		taskID       string
		setupFunc    func(string)
		wantErr      bool
		checkSummary bool
		checkRefs    bool
	}{
		{
			name:    "Extract from task with context.md",
			taskID:  "TASK-001",
			wantErr: false,
			setupFunc: func(taskID string) {
				taskDir := filepath.Join(tmpDir, "tickets", taskID)
				os.MkdirAll(taskDir, 0o755)
				os.WriteFile(filepath.Join(taskDir, "context.md"), []byte("Task context"), 0o644)
			},
			checkSummary: true,
		},
		{
			name:    "Extract from task with notes.md",
			taskID:  "TASK-002",
			wantErr: false,
			setupFunc: func(taskID string) {
				taskDir := filepath.Join(tmpDir, "tickets", taskID)
				os.MkdirAll(taskDir, 0o755)
				os.WriteFile(filepath.Join(taskDir, "notes.md"), []byte("Task notes"), 0o644)
			},
			checkRefs: true,
		},
		{
			name:      "Extract from non-existent task",
			taskID:    "TASK-999",
			wantErr:   true,
			setupFunc: func(taskID string) {},
		},
		{
			name:    "Extract from task with both files",
			taskID:  "TASK-003",
			wantErr: false,
			setupFunc: func(taskID string) {
				taskDir := filepath.Join(tmpDir, "tickets", taskID)
				os.MkdirAll(taskDir, 0o755)
				os.WriteFile(filepath.Join(taskDir, "context.md"), []byte("Context"), 0o644)
				os.WriteFile(filepath.Join(taskDir, "notes.md"), []byte("Notes"), 0o644)
			},
			checkSummary: true,
			checkRefs:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFunc(tt.taskID)

			knowledge, err := extractor.ExtractFromTask(tt.taskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFromTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if knowledge.TaskID != tt.taskID {
				t.Errorf("TaskID = %v, want %v", knowledge.TaskID, tt.taskID)
			}

			if tt.checkSummary && knowledge.Summary == "" {
				t.Error("Expected non-empty summary")
			}

			if tt.checkRefs && len(knowledge.References) == 0 {
				t.Error("Expected references to be populated")
			}
		})
	}
}

func TestKnowledgeExtractor_SaveAndLoadKnowledge(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	// Create knowledge
	knowledge := models.NewExtractedKnowledge(taskID)
	knowledge.Summary = "Test summary"

	decision := models.Decision{
		ID:          "DEC-001",
		Title:       "Use PostgreSQL",
		Description: "Decision to use PostgreSQL database",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
	}
	knowledge.AddDecision(decision)

	// Save knowledge
	err := extractor.SaveKnowledge(taskID, knowledge)
	if err != nil {
		t.Fatalf("SaveKnowledge() error = %v", err)
	}

	// Check file was created
	decisionsPath := filepath.Join(taskDir, "knowledge", "decisions.yaml")
	if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
		t.Fatalf("decisions.yaml was not created")
	}

	// Load knowledge back
	loaded, err := extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if loaded.TaskID != taskID {
		t.Errorf("TaskID = %v, want %v", loaded.TaskID, taskID)
	}

	if loaded.Summary != "Test summary" {
		t.Errorf("Summary = %v, want %v", loaded.Summary, "Test summary")
	}

	if len(loaded.Decisions) != 1 {
		t.Fatalf("Expected 1 decision, got %d", len(loaded.Decisions))
	}

	if loaded.Decisions[0].ID != "DEC-001" {
		t.Errorf("Decision ID = %v, want %v", loaded.Decisions[0].ID, "DEC-001")
	}
}

func TestKnowledgeExtractor_LoadKnowledge_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	// Load from non-existent task should return empty knowledge
	knowledge, err := extractor.LoadKnowledge("TASK-999")
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v, want nil", err)
	}

	if knowledge.TaskID != "TASK-999" {
		t.Errorf("TaskID = %v, want %v", knowledge.TaskID, "TASK-999")
	}

	if len(knowledge.Decisions) != 0 {
		t.Errorf("Expected empty decisions, got %d", len(knowledge.Decisions))
	}
}

func TestKnowledgeExtractor_AddDecision(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	decision := models.Decision{
		ID:          "DEC-001",
		Title:       "Test Decision",
		Description: "A test decision",
		Status:      "proposed",
		DecidedAt:   time.Now().UTC(),
	}

	err := extractor.AddDecision(taskID, decision)
	if err != nil {
		t.Fatalf("AddDecision() error = %v", err)
	}

	// Load and verify
	knowledge, err := extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if len(knowledge.Decisions) != 1 {
		t.Fatalf("Expected 1 decision, got %d", len(knowledge.Decisions))
	}

	if knowledge.Decisions[0].Title != "Test Decision" {
		t.Errorf("Decision title = %v, want %v", knowledge.Decisions[0].Title, "Test Decision")
	}

	// Add another decision
	decision2 := models.Decision{
		ID:          "DEC-002",
		Title:       "Second Decision",
		Description: "Another decision",
		Status:      "accepted",
		DecidedAt:   time.Now().UTC(),
	}

	err = extractor.AddDecision(taskID, decision2)
	if err != nil {
		t.Fatalf("AddDecision() error = %v", err)
	}

	// Load and verify both
	knowledge, err = extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if len(knowledge.Decisions) != 2 {
		t.Fatalf("Expected 2 decisions, got %d", len(knowledge.Decisions))
	}
}

func TestKnowledgeExtractor_AddLearning(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	learning := models.Learning{
		Title:       "Test Learning",
		Description: "A test learning",
		Category:    "technical",
		Timestamp:   time.Now().UTC(),
	}

	err := extractor.AddLearning(taskID, learning)
	if err != nil {
		t.Fatalf("AddLearning() error = %v", err)
	}

	// Load and verify
	knowledge, err := extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if len(knowledge.Learnings) != 1 {
		t.Fatalf("Expected 1 learning, got %d", len(knowledge.Learnings))
	}

	if knowledge.Learnings[0].Title != "Test Learning" {
		t.Errorf("Learning title = %v, want %v", knowledge.Learnings[0].Title, "Test Learning")
	}
}

func TestKnowledgeExtractor_AddGotcha(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	gotcha := models.Gotcha{
		Title:       "Test Gotcha",
		Description: "A test gotcha",
		Severity:    "medium",
		Timestamp:   time.Now().UTC(),
	}

	err := extractor.AddGotcha(taskID, gotcha)
	if err != nil {
		t.Fatalf("AddGotcha() error = %v", err)
	}

	// Load and verify
	knowledge, err := extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if len(knowledge.Gotchas) != 1 {
		t.Fatalf("Expected 1 gotcha, got %d", len(knowledge.Gotchas))
	}

	if knowledge.Gotchas[0].Title != "Test Gotcha" {
		t.Errorf("Gotcha title = %v, want %v", knowledge.Gotchas[0].Title, "Test Gotcha")
	}
}

func TestKnowledgeExtractor_ExtractAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	taskID := "TASK-001"
	taskDir := filepath.Join(tmpDir, "tickets", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	// Create context file
	contextPath := filepath.Join(taskDir, "context.md")
	if err := os.WriteFile(contextPath, []byte("Task context"), 0o644); err != nil {
		t.Fatalf("Failed to write context: %v", err)
	}

	// Extract and save
	err := extractor.ExtractAndSave(taskID)
	if err != nil {
		t.Fatalf("ExtractAndSave() error = %v", err)
	}

	// Verify file was created
	decisionsPath := filepath.Join(taskDir, "knowledge", "decisions.yaml")
	if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
		t.Fatalf("decisions.yaml was not created")
	}

	// Load and verify
	knowledge, err := extractor.LoadKnowledge(taskID)
	if err != nil {
		t.Fatalf("LoadKnowledge() error = %v", err)
	}

	if knowledge.TaskID != taskID {
		t.Errorf("TaskID = %v, want %v", knowledge.TaskID, taskID)
	}
}

func TestKnowledgeExtractor_ListAllKnowledge(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	// Create multiple tasks with knowledge
	taskIDs := []string{"TASK-001", "TASK-002", "TASK-003"}
	for _, taskID := range taskIDs {
		taskDir := filepath.Join(tmpDir, "tickets", taskID)
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			t.Fatalf("Failed to create task dir: %v", err)
		}

		knowledge := models.NewExtractedKnowledge(taskID)
		if err := extractor.SaveKnowledge(taskID, knowledge); err != nil {
			t.Fatalf("Failed to save knowledge: %v", err)
		}
	}

	// Create a task without knowledge
	taskDir := filepath.Join(tmpDir, "tickets", "TASK-004")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("Failed to create task dir: %v", err)
	}

	// List all knowledge
	list, err := extractor.ListAllKnowledge()
	if err != nil {
		t.Fatalf("ListAllKnowledge() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 tasks with knowledge, got %d", len(list))
	}

	// Verify all expected task IDs are in the list
	found := make(map[string]bool)
	for _, id := range list {
		found[id] = true
	}

	for _, expectedID := range taskIDs {
		if !found[expectedID] {
			t.Errorf("Expected task %s in list, but not found", expectedID)
		}
	}

	// TASK-004 should not be in the list
	if found["TASK-004"] {
		t.Error("TASK-004 should not be in the list")
	}
}

func TestKnowledgeExtractor_ListAllKnowledge_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewKnowledgeExtractor(tmpDir)

	// List without any tasks
	list, err := extractor.ListAllKnowledge()
	if err != nil {
		t.Fatalf("ListAllKnowledge() error = %v", err)
	}

	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}
}

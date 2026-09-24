package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// KnowledgeExtractor extracts learnings, decisions, and gotchas from completed tasks
type KnowledgeExtractor struct {
	basePath string
}

// NewKnowledgeExtractor creates a new knowledge extractor
func NewKnowledgeExtractor(basePath string) *KnowledgeExtractor {
	return &KnowledgeExtractor{
		basePath: basePath,
	}
}

// ExtractFromTask extracts knowledge from a completed task
// It analyzes the task artifacts (context.md, notes.md, etc.) and creates a decisions.yaml file
func (ke *KnowledgeExtractor) ExtractFromTask(taskID string) (*models.ExtractedKnowledge, error) {
	taskDir := filepath.Join(ke.basePath, "tickets", taskID)

	// Check if task directory exists
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("task directory not found: %s", taskDir)
	}

	// Create extracted knowledge
	knowledge := models.NewExtractedKnowledge(taskID)

	// Read context.md if it exists
	contextPath := filepath.Join(taskDir, "context.md")
	if contextData, err := os.ReadFile(contextPath); err == nil {
		knowledge.Summary = string(contextData)
	}

	// Read notes.md if it exists and append to references
	notesPath := filepath.Join(taskDir, "notes.md")
	if _, err := os.Stat(notesPath); err == nil {
		knowledge.References = append(knowledge.References, notesPath)
	}

	return knowledge, nil
}

// SaveKnowledge saves extracted knowledge to the task's knowledge directory
func (ke *KnowledgeExtractor) SaveKnowledge(taskID string, knowledge *models.ExtractedKnowledge) error {
	taskDir := filepath.Join(ke.basePath, "tickets", taskID)
	knowledgeDir := filepath.Join(taskDir, "knowledge")

	// Create knowledge directory if it doesn't exist
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		return fmt.Errorf("failed to create knowledge directory: %w", err)
	}

	// Save to decisions.yaml
	decisionsPath := filepath.Join(knowledgeDir, "decisions.yaml")
	data, err := yaml.Marshal(knowledge)
	if err != nil {
		return fmt.Errorf("failed to marshal knowledge: %w", err)
	}

	if err := os.WriteFile(decisionsPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write decisions.yaml: %w", err)
	}

	return nil
}

// LoadKnowledge loads extracted knowledge from a task's knowledge directory
func (ke *KnowledgeExtractor) LoadKnowledge(taskID string) (*models.ExtractedKnowledge, error) {
	taskDir := filepath.Join(ke.basePath, "tickets", taskID)
	decisionsPath := filepath.Join(taskDir, "knowledge", "decisions.yaml")

	// Check if file exists
	if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
		// Return empty knowledge if file doesn't exist
		return models.NewExtractedKnowledge(taskID), nil
	}

	// Read and unmarshal
	data, err := os.ReadFile(decisionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read decisions.yaml: %w", err)
	}

	var knowledge models.ExtractedKnowledge
	if err := yaml.Unmarshal(data, &knowledge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decisions.yaml: %w", err)
	}

	return &knowledge, nil
}

// AddDecision adds a decision to the task's knowledge base
func (ke *KnowledgeExtractor) AddDecision(taskID string, decision models.Decision) error {
	knowledge, err := ke.LoadKnowledge(taskID)
	if err != nil {
		return fmt.Errorf("failed to load knowledge: %w", err)
	}

	knowledge.AddDecision(decision)

	if err := ke.SaveKnowledge(taskID, knowledge); err != nil {
		return fmt.Errorf("failed to save knowledge: %w", err)
	}

	return nil
}

// AddLearning adds a learning to the task's knowledge base
func (ke *KnowledgeExtractor) AddLearning(taskID string, learning models.Learning) error {
	knowledge, err := ke.LoadKnowledge(taskID)
	if err != nil {
		return fmt.Errorf("failed to load knowledge: %w", err)
	}

	knowledge.AddLearning(learning)

	if err := ke.SaveKnowledge(taskID, knowledge); err != nil {
		return fmt.Errorf("failed to save knowledge: %w", err)
	}

	return nil
}

// AddGotcha adds a gotcha to the task's knowledge base
func (ke *KnowledgeExtractor) AddGotcha(taskID string, gotcha models.Gotcha) error {
	knowledge, err := ke.LoadKnowledge(taskID)
	if err != nil {
		return fmt.Errorf("failed to load knowledge: %w", err)
	}

	knowledge.AddGotcha(gotcha)

	if err := ke.SaveKnowledge(taskID, knowledge); err != nil {
		return fmt.Errorf("failed to save knowledge: %w", err)
	}

	return nil
}

// ExtractAndSave extracts knowledge from a task and saves it
func (ke *KnowledgeExtractor) ExtractAndSave(taskID string) error {
	knowledge, err := ke.ExtractFromTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to extract knowledge: %w", err)
	}

	if err := ke.SaveKnowledge(taskID, knowledge); err != nil {
		return fmt.Errorf("failed to save knowledge: %w", err)
	}

	return nil
}

// ListAllKnowledge returns a list of all task IDs that have knowledge extracted
func (ke *KnowledgeExtractor) ListAllKnowledge() ([]string, error) {
	ticketsDir := filepath.Join(ke.basePath, "tickets")

	// Check if tickets directory exists
	if _, err := os.Stat(ticketsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tickets directory: %w", err)
	}

	var taskIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if knowledge/decisions.yaml exists
		decisionsPath := filepath.Join(ticketsDir, entry.Name(), "knowledge", "decisions.yaml")
		if _, err := os.Stat(decisionsPath); err == nil {
			taskIDs = append(taskIDs, entry.Name())
		}
	}

	return taskIDs, nil
}

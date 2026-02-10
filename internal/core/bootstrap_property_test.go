package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

// allTaskTypes is used by property generators to draw a random task type.
var allTaskTypes = []models.TaskType{
	models.TaskTypeFeat,
	models.TaskTypeBug,
	models.TaskTypeSpike,
	models.TaskTypeRefactor,
}

// taskTypeGenerator draws a random TaskType value.
func taskTypeGenerator() *rapid.Generator[models.TaskType] {
	return rapid.Custom(func(t *rapid.T) models.TaskType {
		idx := rapid.IntRange(0, len(allTaskTypes)-1).Draw(t, "taskTypeIdx")
		return allTaskTypes[idx]
	})
}

// Feature: ai-dev-brain, Property 2: Task Bootstrap Structure Completeness
// For any newly bootstrapped task, the ticket folder SHALL contain:
// communications/ directory, notes.md, context.md, design.md, and status.yaml.
func TestProperty_BootstrapStructureCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskType := taskTypeGenerator().Draw(rt, "taskType")
		title := rapid.StringMatching(`[A-Za-z ]{3,50}`).Draw(rt, "title")

		dir, err := os.MkdirTemp("", "bootstrap-prop2-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(dir)

		idGen := NewTaskIDGenerator(dir, "TASK")
		tmplMgr := NewTemplateManager(dir)
		mockWt := &mockWorktreeCreator{createdPath: filepath.Join(dir, "worktree")}
		bs := NewBootstrapSystem(dir, idGen, mockWt, tmplMgr)

		result, err := bs.Bootstrap(BootstrapConfig{
			Type:       taskType,
			Title:      title,
			RepoPath:   "github.com/org/repo",
			BranchName: "feat/test",
		})
		if err != nil {
			t.Fatalf("Bootstrap failed: %v", err)
		}

		// Ticket path must exist.
		if _, err := os.Stat(result.TicketPath); err != nil {
			t.Fatalf("ticket path %s should exist: %v", result.TicketPath, err)
		}

		// communications/ directory must exist.
		commDir := filepath.Join(result.TicketPath, "communications")
		info, err := os.Stat(commDir)
		if err != nil || !info.IsDir() {
			t.Fatalf("communications/ directory must exist at %s", commDir)
		}

		// Required files must exist.
		requiredFiles := []string{"notes.md", "context.md", "design.md", "status.yaml"}
		for _, f := range requiredFiles {
			fpath := filepath.Join(result.TicketPath, f)
			if _, err := os.Stat(fpath); err != nil {
				t.Fatalf("required file %s must exist: %v", f, err)
			}
		}

		// status.yaml must be valid YAML with the correct task ID.
		statusData, err := os.ReadFile(filepath.Join(result.TicketPath, "status.yaml"))
		if err != nil {
			t.Fatalf("failed to read status.yaml: %v", err)
		}
		var task models.Task
		if err := yaml.Unmarshal(statusData, &task); err != nil {
			t.Fatalf("status.yaml must be valid YAML: %v", err)
		}
		if task.ID != result.TaskID {
			t.Fatalf("status.yaml ID %q must match result task ID %q", task.ID, result.TaskID)
		}
		if task.Title != title {
			t.Fatalf("status.yaml title %q must match input title %q", task.Title, title)
		}
		if task.Type != taskType {
			t.Fatalf("status.yaml type %q must match input type %q", task.Type, taskType)
		}
		if task.Status != models.StatusBacklog {
			t.Fatalf("status.yaml status must be backlog, got %q", task.Status)
		}

		// context.md must reference the task ID.
		contextData, err := os.ReadFile(filepath.Join(result.TicketPath, "context.md"))
		if err != nil {
			t.Fatalf("failed to read context.md: %v", err)
		}
		if !strings.Contains(string(contextData), result.TaskID) {
			t.Fatalf("context.md must reference task ID %s", result.TaskID)
		}

		// Worktree path must be set when worktree manager is provided.
		if result.WorktreePath == "" {
			t.Fatal("worktree path must be set when worktree manager is provided")
		}
	})
}

// templateContentMarkers maps each task type to content that MUST appear
// in the corresponding notes.md and design.md files.
var templateContentMarkers = map[models.TaskType]struct {
	notesMarkers  []string
	designMarkers []string
}{
	models.TaskTypeFeat: {
		notesMarkers:  []string{"Feature Notes", "Requirements", "Acceptance Criteria"},
		designMarkers: []string{"Technical Design", "Architecture", "API Changes"},
	},
	models.TaskTypeBug: {
		notesMarkers:  []string{"Bug Notes", "Steps to Reproduce", "Root Cause Analysis"},
		designMarkers: []string{"Technical Design", "Root Cause", "Fix Approach"},
	},
	models.TaskTypeSpike: {
		notesMarkers:  []string{"Spike Notes", "Research Questions", "Time-Box"},
		designMarkers: []string{"Technical Design", "Investigation Scope", "Proof of Concept"},
	},
	models.TaskTypeRefactor: {
		notesMarkers:  []string{"Refactor Notes", "Current State", "Rollback Plan"},
		designMarkers: []string{"Technical Design", "Current Architecture", "Migration Plan"},
	},
}

// Feature: ai-dev-brain, Property 3: Template Application by Type
// For any task type (feat, bug, spike, refactor), bootstrapping a task with
// that type SHALL apply the corresponding template, and the resulting files
// SHALL contain template-specific content.
func TestProperty_TemplateApplicationByType(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskType := taskTypeGenerator().Draw(rt, "taskType")
		title := rapid.StringMatching(`[A-Za-z ]{3,50}`).Draw(rt, "title")

		dir, err := os.MkdirTemp("", "bootstrap-prop3-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(dir)

		idGen := NewTaskIDGenerator(dir, "TASK")
		tmplMgr := NewTemplateManager(dir)
		bs := NewBootstrapSystem(dir, idGen, nil, tmplMgr)

		result, err := bs.Bootstrap(BootstrapConfig{
			Type:  taskType,
			Title: title,
		})
		if err != nil {
			t.Fatalf("Bootstrap failed: %v", err)
		}

		markers, ok := templateContentMarkers[taskType]
		if !ok {
			t.Fatalf("no content markers defined for task type %q", taskType)
		}

		// Verify notes.md contains type-specific content.
		notesData, err := os.ReadFile(filepath.Join(result.TicketPath, "notes.md"))
		if err != nil {
			t.Fatalf("failed to read notes.md: %v", err)
		}
		notesStr := string(notesData)
		for _, marker := range markers.notesMarkers {
			if !strings.Contains(notesStr, marker) {
				t.Fatalf("notes.md for %s must contain %q", taskType, marker)
			}
		}

		// Verify design.md contains type-specific content.
		designData, err := os.ReadFile(filepath.Join(result.TicketPath, "design.md"))
		if err != nil {
			t.Fatalf("failed to read design.md: %v", err)
		}
		designStr := string(designData)
		for _, marker := range markers.designMarkers {
			if !strings.Contains(designStr, marker) {
				t.Fatalf("design.md for %s must contain %q", taskType, marker)
			}
		}
	})
}

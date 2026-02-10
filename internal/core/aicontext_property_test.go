package core

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// Feature: ai-dev-brain, Property 20: AI Context File Content Completeness
func TestAIContextFileContentCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "aicontext-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		backlogMgr := storage.NewBacklogManager(dir)

		// Add random tasks.
		nTasks := rapid.IntRange(0, 5).Draw(t, "nTasks")
		for i := 0; i < nTasks; i++ {
			taskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(0, 99999).Draw(t, fmt.Sprintf("taskID%d", i)))
			statuses := []models.TaskStatus{
				models.StatusBacklog, models.StatusInProgress, models.StatusBlocked,
				models.StatusReview, models.StatusDone, models.StatusArchived,
			}
			status := statuses[rapid.IntRange(0, len(statuses)-1).Draw(t, fmt.Sprintf("status%d", i))]

			backlogMgr.AddTask(storage.BacklogEntry{
				ID:       taskID,
				Title:    genAlpha(t, fmt.Sprintf("title%d", i), 3, 20),
				Status:   status,
				Priority: models.P2,
				Branch:   genAlpha(t, fmt.Sprintf("branch%d", i), 3, 15),
			})
		}
		backlogMgr.Save()

		gen := NewAIContextGenerator(dir, backlogMgr)

		// Choose random AI type.
		aiTypes := []AIType{AITypeClaude, AITypeKiro}
		aiType := aiTypes[rapid.IntRange(0, 1).Draw(t, "aiType")]

		path, err := gen.GenerateContextFile(aiType)
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)

		// Verify all required sections are present.
		requiredSections := []string{
			"## Project Overview",
			"## Directory Structure",
			"## Key Conventions",
			"## Glossary",
			"## Active Decisions Summary",
			"## Active Tasks",
			"## Key Contacts",
		}
		for _, section := range requiredSections {
			if !strings.Contains(content, section) {
				t.Fatalf("generated %s file missing section: %s", aiType, section)
			}
		}

		// Verify header and footer.
		if !strings.Contains(content, "# AI Dev Brain Context") {
			t.Fatal("missing title header")
		}
		if !strings.Contains(content, "adb sync-context") {
			t.Fatal("missing sync-context footer")
		}
	})
}

// Feature: ai-dev-brain, Property 21: AI Context File Sync Consistency
func TestAIContextFileSyncConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "aicontext-sync-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		backlogMgr := storage.NewBacklogManager(dir)
		gen := NewAIContextGenerator(dir, backlogMgr)

		// Initial sync with some tasks.
		nInitialTasks := rapid.IntRange(1, 3).Draw(t, "nInitial")
		for i := 0; i < nInitialTasks; i++ {
			backlogMgr.AddTask(storage.BacklogEntry{
				ID:     fmt.Sprintf("TASK-%05d", i+1),
				Title:  genAlpha(t, fmt.Sprintf("iTitle%d", i), 3, 20),
				Status: models.StatusInProgress,
				Branch: genAlpha(t, fmt.Sprintf("iBranch%d", i), 3, 10),
			})
		}
		backlogMgr.Save()
		gen.SyncContext()

		// Add a new task.
		newTaskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(10000, 99999).Draw(t, "newTaskID"))
		newTitle := genAlpha(t, "newTitle", 3, 20)
		backlogMgr2 := storage.NewBacklogManager(dir)
		backlogMgr2.Load()
		backlogMgr2.AddTask(storage.BacklogEntry{
			ID:     newTaskID,
			Title:  newTitle,
			Status: models.StatusInProgress,
			Branch: genAlpha(t, "newBranch", 3, 10),
		})
		backlogMgr2.Save()

		// Re-sync with a fresh generator that will pick up the new backlog state.
		backlogMgr3 := storage.NewBacklogManager(dir)
		gen2 := NewAIContextGenerator(dir, backlogMgr3)
		if err := gen2.SyncContext(); err != nil {
			t.Fatal(err)
		}

		// Verify the new task appears in both files.
		for _, name := range []string{"CLAUDE.md", "kiro.md"} {
			data, err := os.ReadFile(fmt.Sprintf("%s/%s", dir, name))
			if err != nil {
				t.Fatalf("%s not found after sync: %v", name, err)
			}
			content := string(data)
			if !strings.Contains(content, newTaskID) {
				t.Fatalf("%s does not contain newly added task %s after sync", name, newTaskID)
			}
		}
	})
}

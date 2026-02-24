package core

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// fakeBacklogStore is a minimal in-memory implementation of BacklogStore for testing.
type fakeBacklogStore struct {
	tasks map[string]BacklogStoreEntry
}

func newFakeBacklogStore() *fakeBacklogStore {
	return &fakeBacklogStore{
		tasks: make(map[string]BacklogStoreEntry),
	}
}

func (f *fakeBacklogStore) AddTask(entry BacklogStoreEntry) error {
	f.tasks[entry.ID] = entry
	return nil
}

func (f *fakeBacklogStore) UpdateTask(taskID string, updates BacklogStoreEntry) error {
	f.tasks[taskID] = updates
	return nil
}

func (f *fakeBacklogStore) GetTask(taskID string) (*BacklogStoreEntry, error) {
	e, ok := f.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return &e, nil
}

func (f *fakeBacklogStore) GetAllTasks() ([]BacklogStoreEntry, error) {
	result := make([]BacklogStoreEntry, 0, len(f.tasks))
	for _, e := range f.tasks {
		result = append(result, e)
	}
	return result, nil
}

func (f *fakeBacklogStore) FilterTasks(filter BacklogStoreFilter) ([]BacklogStoreEntry, error) {
	var result []BacklogStoreEntry
	for _, e := range f.tasks {
		if len(filter.Status) > 0 {
			match := false
			for _, s := range filter.Status {
				if e.Status == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		result = append(result, e)
	}
	return result, nil
}

func (f *fakeBacklogStore) Load() error {
	return nil
}

func (f *fakeBacklogStore) Save() error {
	return nil
}

// Feature: ai-dev-brain, Property 20: AI Context File Content Completeness
func TestAIContextFileContentCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dir, err := os.MkdirTemp("", "aicontext-prop-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()

		backlogMgr := newFakeBacklogStore()

		// Add random tasks.
		nTasks := rapid.IntRange(0, 5).Draw(t, "nTasks")
		for i := 0; i < nTasks; i++ {
			taskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(0, 99999).Draw(t, fmt.Sprintf("taskID%d", i)))
			statuses := []models.TaskStatus{
				models.StatusBacklog, models.StatusInProgress, models.StatusBlocked,
				models.StatusReview, models.StatusDone, models.StatusArchived,
			}
			status := statuses[rapid.IntRange(0, len(statuses)-1).Draw(t, fmt.Sprintf("status%d", i))]

			_ = backlogMgr.AddTask(BacklogStoreEntry{
				ID:       taskID,
				Title:    genAlpha(t, fmt.Sprintf("title%d", i), 3, 20),
				Status:   status,
				Priority: models.P2,
				Branch:   genAlpha(t, fmt.Sprintf("branch%d", i), 3, 15),
			})
		}
		_ = backlogMgr.Save()

		gen := NewAIContextGenerator(dir, backlogMgr, nil, nil)

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
		defer func() { _ = os.RemoveAll(dir) }()

		backlogMgr := newFakeBacklogStore()
		gen := NewAIContextGenerator(dir, backlogMgr, nil, nil)

		// Initial sync with some tasks.
		nInitialTasks := rapid.IntRange(1, 3).Draw(t, "nInitial")
		for i := 0; i < nInitialTasks; i++ {
			_ = backlogMgr.AddTask(BacklogStoreEntry{
				ID:     fmt.Sprintf("TASK-%05d", i+1),
				Title:  genAlpha(t, fmt.Sprintf("iTitle%d", i), 3, 20),
				Status: models.StatusInProgress,
				Branch: genAlpha(t, fmt.Sprintf("iBranch%d", i), 3, 10),
			})
		}
		_ = gen.SyncContext()

		// Add a new task to the same store.
		newTaskID := fmt.Sprintf("TASK-%05d", rapid.IntRange(10000, 99999).Draw(t, "newTaskID"))
		_ = backlogMgr.AddTask(BacklogStoreEntry{
			ID:     newTaskID,
			Title:  genAlpha(t, "newTitle", 3, 20),
			Status: models.StatusInProgress,
			Branch: genAlpha(t, "newBranch", 3, 10),
		})

		// Re-sync — should pick up the newly added task.
		if err := gen.SyncContext(); err != nil {
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

package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// TestTaskManager_Unarchive_PrunesEmptyArchivedNesting pins follow-up 3: Unarchive
// moved the ticket dir back out of _archived/ but left the nesting it came from
// behind, so tickets/_archived/github.com/acme/thing/ accumulated empty
// directories that read as "there are archived tickets here".
func TestTaskManager_Unarchive_PrunesEmptyArchivedNesting(t *testing.T) {
	tm, _, _, _, _, tempDir := createTestTaskManager(t)
	archivedRoot := filepath.Join(tempDir, "_archived")

	task, err := tm.Create(CreateTaskOpts{
		Title:    "Nested Task",
		TaskType: models.TaskTypeFeat,
		Repo:     "github.com/acme/thing",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := tm.Archive(task.ID, ArchiveOptions{}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	nesting := filepath.Join(archivedRoot, "github.com", "acme", "thing")
	if _, err := os.Stat(nesting); err != nil {
		t.Fatalf("expected the archived nesting to exist after Archive: %v", err)
	}

	if err := tm.Unarchive(task.ID); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	// Every level the ticket vacated is gone…
	for _, dir := range []string{
		nesting,
		filepath.Join(archivedRoot, "github.com", "acme"),
		filepath.Join(archivedRoot, "github.com"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected empty archived nesting %s to be pruned (stat err = %v)", dir, err)
		}
	}
	// …and _archived/ itself is NOT, because it is the boundary, not a level.
	if _, err := os.Stat(archivedRoot); err != nil {
		t.Errorf("expected %s to survive: %v", archivedRoot, err)
	}
}

// TestTaskManager_Unarchive_KeepsNestingSharedWithSibling is the safety half of
// the same change, and the reason the prune uses os.Remove rather than a
// RemoveAll walking upward: one archived nesting can hold several tickets, and
// unarchiving one of them must not take the others with it.
func TestTaskManager_Unarchive_KeepsNestingSharedWithSibling(t *testing.T) {
	tm, _, _, _, _, tempDir := createTestTaskManager(t)
	archivedRoot := filepath.Join(tempDir, "_archived")

	var ids []string
	for _, title := range []string{"First Task", "Second Task"} {
		task, err := tm.Create(CreateTaskOpts{
			Title:    title,
			TaskType: models.TaskTypeFeat,
			Repo:     "github.com/acme/thing",
		})
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		if err := tm.Archive(task.ID, ArchiveOptions{}); err != nil {
			t.Fatalf("Archive %q: %v", title, err)
		}
		ids = append(ids, task.ID)
	}

	if err := tm.Unarchive(ids[0]); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	nesting := filepath.Join(archivedRoot, "github.com", "acme", "thing")
	entries, err := os.ReadDir(nesting)
	if err != nil {
		t.Fatalf("expected the shared archived nesting to survive: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected the sibling archived ticket to remain, got %d entries", len(entries))
	}
	sibling := filepath.Join(nesting, entries[0].Name())
	if _, err := os.Stat(filepath.Join(sibling, "handoff.md")); err != nil {
		t.Errorf("expected the sibling ticket's contents intact: %v", err)
	}
}

// TestTaskManager_Unarchive_FlatLayoutLeavesArchivedRootAlone guards the boundary
// from the other direction: a legacy flat ticket sits directly in _archived/, so
// the directory it vacates IS the boundary and there is nothing to prune.
func TestTaskManager_Unarchive_FlatLayoutLeavesArchivedRootAlone(t *testing.T) {
	tm, backlogStore, _, _, _, tempDir := createTestTaskManager(t)
	archivedRoot := filepath.Join(tempDir, "_archived")

	// A legacy flat archived ticket: _archived/TASK-00001, no nesting.
	flatDir := filepath.Join(archivedRoot, "TASK-00001")
	if err := os.MkdirAll(flatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	task := models.NewTask("TASK-00001", "Flat Task", models.TaskTypeFeat)
	task.Status = models.TaskStatusArchived
	task.TicketPath = flatDir
	if err := backlogStore.AddTask(*task); err != nil {
		t.Fatal(err)
	}

	if err := tm.Unarchive(task.ID); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	if _, err := os.Stat(archivedRoot); err != nil {
		t.Errorf("expected %s to survive an unarchive of a flat ticket: %v", archivedRoot, err)
	}
}

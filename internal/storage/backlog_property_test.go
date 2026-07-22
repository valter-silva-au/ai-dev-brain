package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// TestProperty_StorageCorruptedYAMLRecovery verifies handling of corrupted YAML files
func TestProperty_StorageCorruptedYAMLRecovery(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		// Write corrupted YAML
		corruptedYAML := rapid.StringN(10, 500, 1000).Draw(t, "corrupted")
		if err := os.WriteFile(filePath, []byte(corruptedYAML), 0o644); err != nil {
			t.Fatalf("Failed to write corrupted YAML: %v", err)
		}

		fbm := NewFileBacklogManager(filePath)
		_, err := fbm.Load()

		// Should fail gracefully on corrupted YAML
		if err == nil {
			// It's possible the random string was valid YAML, skip
			return
		}

		// Error should be descriptive
		if err.Error() == "" {
			t.Fatal("Error message should not be empty")
		}
	})
}

// TestProperty_StorageMissingDirectory verifies directory creation
func TestProperty_StorageMissingDirectory(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		depth := rapid.IntRange(1, 5).Draw(t, "depth")

		// Create nested path
		path := baseDir
		for i := 0; i < depth; i++ {
			path = filepath.Join(path, rapid.StringMatching(`^[a-z]+$`).Draw(t, "dir"))
		}
		filePath := filepath.Join(path, "backlog.yaml")

		fbm := NewFileBacklogManager(filePath)
		backlog := models.NewBacklog()

		// Save should create all necessary directories
		err := fbm.Save(backlog)
		if err != nil {
			t.Fatalf("Save failed to create directories: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Fatal("File was not created")
		}
	})
}

// TestProperty_StorageReadOnlyTargetIsAllOrNothing verifies how Save behaves when
// the TARGET file is read-only. saveUnsafe now writes through atomicWriteFile — a
// temp-file-plus-rename replace — rather than truncating the target in place, and
// that deliberately changes what a read-only target means, per platform. Each
// platform's outcome is asserted EXPLICITLY (not "either is acceptable"), so the
// test still proves a specific contract on both:
//
//   - On POSIX, rename(2) takes its permission from the containing DIRECTORY, not
//     the target file, so replacing a read-only file MUST SUCCEED with the new
//     content. (The pre-atomic os.WriteFile path errored here; this test used to
//     assert that stale expectation and is why it broke when the atomic replace
//     landed.) POSIX permission-error surfacing now lives in the unwritable-DIRECTORY
//     case, TestStorage_UnwritableDirectorySurfacesSaveError.
//   - On Windows, the read-only attribute blocks MoveFileEx, so the replace MUST
//     FAIL and leave the prior backlog intact — this is where Windows proves Save
//     still surfaces a permission error.
//
// In both directions the write is ALL-OR-NOTHING: Save either fully applies the new
// content or leaves the previous valid backlog untouched — never torn.
func TestProperty_StorageReadOnlyTargetIsAllOrNothing(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only test when running as root")
	}

	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		fbm := NewFileBacklogManager(filePath)

		// Seed an empty backlog, then make the file read-only.
		if err := fbm.Save(models.NewBacklog()); err != nil {
			t.Fatalf("Initial save failed: %v", err)
		}
		if err := os.Chmod(filePath, 0o444); err != nil {
			t.Fatalf("Failed to change permissions: %v", err)
		}
		defer func() { _ = os.Chmod(filePath, 0o644) }() // restore for cleanup

		// Attempt to add a task over the read-only target.
		updated := models.NewBacklog()
		task := models.NewTask("TASK-00001", "test", models.TaskTypeFeat)
		updated.AddTask(*task)
		saveErr := fbm.Save(updated)

		// The file must always remain a fully valid backlog (never torn), and the
		// per-platform outcome is asserted explicitly below.
		reloaded, err := fbm.Load()
		if err != nil {
			t.Fatalf("backlog unreadable after Save over a read-only target (torn write?): %v", err)
		}

		if runtime.GOOS == "windows" {
			// The read-only ATTRIBUTE blocks MoveFileEx: the replace MUST fail and
			// leave the prior (empty) backlog intact.
			if saveErr == nil {
				t.Fatal("Save over a read-only target must fail on Windows (the read-only attribute blocks MoveFileEx), but it succeeded")
			}
			if len(reloaded.Tasks) != 0 {
				t.Fatalf("Save failed but the backlog was mutated: %d tasks, want 0 (partial write)", len(reloaded.Tasks))
			}
			return
		}

		// POSIX: rename(2) draws permission from the DIRECTORY, not the target file,
		// so the replace MUST succeed with the new content.
		if saveErr != nil {
			t.Fatalf("Save over a read-only target must succeed on POSIX (rename permission comes from the directory), got: %v", saveErr)
		}
		if reloaded.FindTaskByID("TASK-00001") == nil {
			t.Fatal("Save reported success but the new task is missing (lost write)")
		}
	})
}

// TestStorage_UnwritableDirectorySurfacesSaveError proves Save still surfaces a
// permission error — and leaves the target intact — when the write genuinely cannot
// proceed. In the atomic-replace world a read-only TARGET no longer errors on POSIX
// (rename(2) draws its permission from the directory), so the place a permission
// error still surfaces there is an unwritable DIRECTORY: os.CreateTemp for the temp
// file fails with EACCES before any rename, so the existing target is never touched.
//
// This runs on POSIX only. Contrary to the intuition that directory-permission
// denial is cross-platform, the Windows read-only attribute on a directory does NOT
// deny creating files inside it, so chmod cannot make a directory unwritable here;
// Windows keeps its permission-error coverage through the read-only-TARGET branch of
// TestProperty_StorageReadOnlyTargetIsAllOrNothing (which fails the replace and
// asserts the prior backlog is intact). Skipped as root, which bypasses directory
// permission bits.
func TestStorage_UnwritableDirectorySurfacesSaveError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory read-only does not deny file creation; Windows permission-error coverage is the read-only-target case")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission bits")
	}

	dir := t.TempDir()
	subdir := filepath.Join(dir, "registry")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	filePath := filepath.Join(subdir, "backlog.yaml")

	fbm := NewFileBacklogManager(filePath)
	// Seed a valid backlog we can prove stays intact.
	if err := fbm.Save(models.NewBacklog()); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Deny writes to the directory: os.CreateTemp inside it must now fail.
	if err := os.Chmod(subdir, 0o555); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	defer func() { _ = os.Chmod(subdir, 0o755) }() // restore for TempDir cleanup

	updated := models.NewBacklog()
	updated.AddTask(*models.NewTask("TASK-00001", "test", models.TaskTypeFeat))
	if err := fbm.Save(updated); err == nil {
		t.Fatal("Save into an unwritable directory should return a permission error, got nil")
	}

	// The prior backlog must be intact and readable — the failed Save touched nothing.
	reloaded, err := fbm.Load()
	if err != nil {
		t.Fatalf("prior backlog unreadable after a failed Save (torn write?): %v", err)
	}
	if len(reloaded.Tasks) != 0 {
		t.Fatalf("failed Save mutated the backlog: %d tasks, want 0", len(reloaded.Tasks))
	}
}

// TestProperty_StorageConcurrentWrites verifies concurrent write safety
func TestProperty_StorageConcurrentWrites(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		fbm := NewFileBacklogManager(filePath)
		goroutines := rapid.IntRange(2, 10).Draw(t, "goroutines")

		// Generate task IDs outside of goroutines
		taskIDs := make([]string, goroutines)
		for i := 0; i < goroutines; i++ {
			taskIDs[i] = "TASK-" + strconv.FormatInt(int64(rapid.IntRange(0, 99999).Draw(t, "id")), 10)
		}

		var wg sync.WaitGroup
		errors := make([]error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(index int, taskID string) {
				defer wg.Done()
				task := models.NewTask(taskID, "test", models.TaskTypeFeat)
				errors[index] = fbm.AddTask(*task)
			}(i, taskIDs[i])
		}

		wg.Wait()

		// Verify backlog is still valid and loadable after concurrent writes
		backlog, err := fbm.Load()
		if err != nil {
			t.Fatalf("Failed to load after concurrent writes: %v", err)
		}

		// With concurrent access, the exact number of tasks is non-deterministic
		// (some writes may race). Just verify the file isn't corrupted.
		if backlog == nil {
			t.Fatal("Backlog should not be nil after concurrent writes")
		}
	})
}

// TestProperty_StorageEmptyBacklog verifies operations on empty backlog
func TestProperty_StorageEmptyBacklog(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		fbm := NewFileBacklogManager(filePath)

		// Load non-existent file should return empty backlog
		backlog, err := fbm.Load()
		if err != nil {
			t.Fatalf("Load of non-existent file failed: %v", err)
		}

		if len(backlog.Tasks) != 0 {
			t.Fatal("Empty backlog should have zero tasks")
		}

		// Operations on empty backlog
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		_, err = fbm.GetTask(taskID)
		if err == nil {
			t.Fatal("GetTask should fail on empty backlog")
		}

		err = fbm.RemoveTask(taskID)
		if err == nil {
			t.Fatal("RemoveTask should fail on empty backlog")
		}
	})
}

// TestProperty_StorageInvalidTaskID verifies handling of invalid task IDs
func TestProperty_StorageInvalidTaskID(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		fbm := NewFileBacklogManager(filePath)

		// Add a valid task with unique ID
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		validTask := models.NewTask(taskID, "valid", models.TaskTypeFeat)
		if err := fbm.AddTask(*validTask); err != nil {
			t.Fatalf("Failed to add valid task: %v", err)
		}

		// Try to operate on a non-existent task
		invalidID := rapid.StringMatching(`^INVALID-[a-z]+$`).Draw(t, "invalidID")

		_, err := fbm.GetTask(invalidID)
		if err == nil {
			t.Fatal("GetTask should fail for invalid ID")
		}

		err = fbm.RemoveTask(invalidID)
		if err == nil {
			t.Fatal("RemoveTask should fail for invalid ID")
		}
	})
}

// TestProperty_StorageAddUpdateRemoveSequence verifies operation sequences
func TestProperty_StorageAddUpdateRemoveSequence(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		filePath := filepath.Join(baseDir, suffix+"_backlog.yaml")

		fbm := NewFileBacklogManager(filePath)
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		// Use YAML-safe strings (no tabs, no leading colons/newlines that break YAML)
		title1 := rapid.StringMatching(`^[a-zA-Z0-9 _-]{1,50}$`).Draw(t, "title1")
		title2 := rapid.StringMatching(`^[a-zA-Z0-9 _-]{1,50}$`).Draw(t, "title2")

		// Add
		task := models.NewTask(taskID, title1, models.TaskTypeFeat)
		if err := fbm.AddTask(*task); err != nil {
			t.Fatalf("AddTask failed: %v", err)
		}

		// Update
		task.Title = title2
		if err := fbm.UpdateTask(*task); err != nil {
			t.Fatalf("UpdateTask failed: %v", err)
		}

		// Verify update
		retrieved, err := fbm.GetTask(taskID)
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if retrieved.Title != title2 {
			t.Fatalf("Title not updated: expected %s, got %s", title2, retrieved.Title)
		}

		// Remove
		if err := fbm.RemoveTask(taskID); err != nil {
			t.Fatalf("RemoveTask failed: %v", err)
		}

		// Verify removal
		_, err = fbm.GetTask(taskID)
		if err == nil {
			t.Fatal("Task should not exist after removal")
		}
	})
}

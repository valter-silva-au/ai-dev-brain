package models

import (
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_BacklogAddRemoveIdempotent verifies add/remove operations are consistent
func TestProperty_BacklogAddRemoveIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		backlog := NewBacklog()
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		title := rapid.String().Draw(t, "title")

		task := NewTask(taskID, title, TaskTypeFeat)

		// Add task
		backlog.AddTask(*task)

		// Verify it exists
		found := backlog.FindTaskByID(taskID)
		if found == nil {
			t.Fatal("Task not found after adding")
		}

		// Remove task
		removed := backlog.RemoveTask(taskID)
		if !removed {
			t.Fatal("Failed to remove task")
		}

		// Verify it's gone
		found = backlog.FindTaskByID(taskID)
		if found != nil {
			t.Fatal("Task still exists after removal")
		}
	})
}

// TestProperty_BacklogUpdatePreservesID verifies update doesn't change task ID
func TestProperty_BacklogUpdatePreservesID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		backlog := NewBacklog()
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		originalTitle := rapid.String().Draw(t, "originalTitle")
		newTitle := rapid.String().Draw(t, "newTitle")

		task := NewTask(taskID, originalTitle, TaskTypeFeat)
		backlog.AddTask(*task)

		// Update task
		task.Title = newTitle
		updated := backlog.UpdateTask(*task)
		if !updated {
			t.Fatal("Failed to update task")
		}

		// Verify ID is unchanged
		found := backlog.FindTaskByID(taskID)
		if found == nil {
			t.Fatal("Task not found after update")
		}
		if found.ID != taskID {
			t.Fatalf("Task ID changed: %s != %s", found.ID, taskID)
		}
		if found.Title != newTitle {
			t.Fatalf("Task title not updated: %s != %s", found.Title, newTitle)
		}
	})
}

// TestProperty_BacklogEmptyOperations verifies operations on empty backlog
func TestProperty_BacklogEmptyOperations(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		backlog := NewBacklog()
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		// Operations on empty backlog should handle gracefully
		found := backlog.FindTaskByID(taskID)
		if found != nil {
			t.Fatal("Found task in empty backlog")
		}

		removed := backlog.RemoveTask(taskID)
		if removed {
			t.Fatal("Removed task from empty backlog")
		}

		task := NewTask(taskID, "test", TaskTypeFeat)
		updated := backlog.UpdateTask(*task)
		if updated {
			t.Fatal("Updated non-existent task")
		}
	})
}

// TestProperty_BacklogMultipleOperations verifies complex operation sequences
func TestProperty_BacklogMultipleOperations(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		backlog := NewBacklog()
		numTasks := rapid.IntRange(1, 20).Draw(t, "numTasks")

		taskIDs := make([]string, numTasks)
		for i := 0; i < numTasks; i++ {
			taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
			taskIDs[i] = taskID
			task := NewTask(taskID, rapid.String().Draw(t, "title"), TaskTypeFeat)
			backlog.AddTask(*task)
		}

		// Verify all tasks are present
		if len(backlog.Tasks) != numTasks {
			t.Fatalf("Expected %d tasks, got %d", numTasks, len(backlog.Tasks))
		}

		// Remove half of the tasks
		for i := 0; i < numTasks/2; i++ {
			removed := backlog.RemoveTask(taskIDs[i])
			if !removed {
				t.Fatalf("Failed to remove task %s", taskIDs[i])
			}
		}

		// Verify correct count remains
		expectedRemaining := numTasks - numTasks/2
		if len(backlog.Tasks) != expectedRemaining {
			t.Fatalf("Expected %d remaining tasks, got %d", expectedRemaining, len(backlog.Tasks))
		}
	})
}

package models

import (
	"testing"
	"time"
)

func TestNewBacklog(t *testing.T) {
	b := NewBacklog()
	if b == nil {
		t.Fatal("NewBacklog() returned nil")
	}
	if b.Tasks == nil {
		t.Error("Tasks slice should not be nil")
	}
	if len(b.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(b.Tasks))
	}
}

func TestBacklog_AddTask(t *testing.T) {
	b := NewBacklog()
	task := NewTask("TASK-001", "Test task", TaskTypeFeat)

	b.AddTask(*task)

	if len(b.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(b.Tasks))
	}
	if b.Tasks[0].ID != "TASK-001" {
		t.Errorf("Expected task ID TASK-001, got %s", b.Tasks[0].ID)
	}
}

func TestBacklog_FindTaskByID(t *testing.T) {
	b := NewBacklog()
	task1 := NewTask("TASK-001", "Test task 1", TaskTypeFeat)
	task2 := NewTask("TASK-002", "Test task 2", TaskTypeBug)

	b.AddTask(*task1)
	b.AddTask(*task2)

	found := b.FindTaskByID("TASK-002")
	if found == nil {
		t.Fatal("Expected to find task TASK-002")
	}
	if found.ID != "TASK-002" {
		t.Errorf("Expected task ID TASK-002, got %s", found.ID)
	}
	if found.Title != "Test task 2" {
		t.Errorf("Expected title 'Test task 2', got %s", found.Title)
	}

	notFound := b.FindTaskByID("TASK-999")
	if notFound != nil {
		t.Error("Expected nil for non-existent task")
	}
}

func TestBacklog_UpdateTask(t *testing.T) {
	b := NewBacklog()
	task := NewTask("TASK-001", "Original title", TaskTypeFeat)
	b.AddTask(*task)

	// Update the task
	updatedTask := *task
	updatedTask.Title = "Updated title"
	updatedTask.Status = TaskStatusInProgress
	updatedTask.Updated = time.Now().UTC()

	success := b.UpdateTask(updatedTask)
	if !success {
		t.Error("UpdateTask should return true for existing task")
	}

	// Verify update
	found := b.FindTaskByID("TASK-001")
	if found == nil {
		t.Fatal("Task not found after update")
	}
	if found.Title != "Updated title" {
		t.Errorf("Expected updated title, got %s", found.Title)
	}
	if found.Status != TaskStatusInProgress {
		t.Errorf("Expected status in_progress, got %s", found.Status)
	}

	// Try to update non-existent task
	nonExistentTask := NewTask("TASK-999", "Non-existent", TaskTypeBug)
	success = b.UpdateTask(*nonExistentTask)
	if success {
		t.Error("UpdateTask should return false for non-existent task")
	}
}

func TestBacklog_RemoveTask(t *testing.T) {
	b := NewBacklog()
	task1 := NewTask("TASK-001", "Task 1", TaskTypeFeat)
	task2 := NewTask("TASK-002", "Task 2", TaskTypeBug)
	task3 := NewTask("TASK-003", "Task 3", TaskTypeSpike)

	b.AddTask(*task1)
	b.AddTask(*task2)
	b.AddTask(*task3)

	if len(b.Tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(b.Tasks))
	}

	// Remove middle task
	success := b.RemoveTask("TASK-002")
	if !success {
		t.Error("RemoveTask should return true for existing task")
	}
	if len(b.Tasks) != 2 {
		t.Errorf("Expected 2 tasks after removal, got %d", len(b.Tasks))
	}

	// Verify task is gone
	found := b.FindTaskByID("TASK-002")
	if found != nil {
		t.Error("Task should not be found after removal")
	}

	// Verify other tasks remain
	if b.Tasks[0].ID != "TASK-001" {
		t.Errorf("Expected first task to be TASK-001, got %s", b.Tasks[0].ID)
	}
	if b.Tasks[1].ID != "TASK-003" {
		t.Errorf("Expected second task to be TASK-003, got %s", b.Tasks[1].ID)
	}

	// Try to remove non-existent task
	success = b.RemoveTask("TASK-999")
	if success {
		t.Error("RemoveTask should return false for non-existent task")
	}
	if len(b.Tasks) != 2 {
		t.Error("Task count should not change when removing non-existent task")
	}
}

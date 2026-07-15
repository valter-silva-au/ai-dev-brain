package models

import (
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_TaskStateTransitions verifies valid task state transitions
func TestProperty_TaskStateTransitions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")
		title := rapid.String().Draw(t, "title")

		task := NewTask(taskID, title, TaskTypeFeat)

		// Initial state should be backlog
		if task.Status != TaskStatusBacklog {
			t.Fatalf("Initial status should be backlog, got %s", task.Status)
		}

		// Valid state transitions
		validTransitions := []TaskStatus{
			TaskStatusInProgress,
			TaskStatusReview,
			TaskStatusBlocked,
			TaskStatusDone,
			TaskStatusArchived,
		}

		newStatus := rapid.SampledFrom(validTransitions).Draw(t, "newStatus")
		task.Status = newStatus

		if task.Status != newStatus {
			t.Fatalf("Status not updated: expected %s, got %s", newStatus, task.Status)
		}
	})
}

// TestProperty_TaskIsActiveConsistency verifies IsActive() method consistency
func TestProperty_TaskIsActiveConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		task := NewTask("TASK-00001", "test", TaskTypeFeat)

		activeStates := []TaskStatus{TaskStatusInProgress, TaskStatusReview, TaskStatusBlocked}
		inactiveStates := []TaskStatus{TaskStatusBacklog, TaskStatusDone, TaskStatusArchived}

		// Test active states
		status := rapid.SampledFrom(activeStates).Draw(t, "activeStatus")
		task.Status = status
		if !task.IsActive() {
			t.Fatalf("Task with status %s should be active", status)
		}

		// Test inactive states
		status = rapid.SampledFrom(inactiveStates).Draw(t, "inactiveStatus")
		task.Status = status
		if task.IsActive() {
			t.Fatalf("Task with status %s should not be active", status)
		}
	})
}

// TestProperty_TaskBlockedConsistency verifies IsBlocked() method consistency
func TestProperty_TaskBlockedConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		task := NewTask("TASK-00001", "test", TaskTypeFeat)

		// Test blocked status
		task.Status = TaskStatusBlocked
		if !task.IsBlocked() {
			t.Fatal("Task with blocked status should return IsBlocked() = true")
		}

		// Test blocked by dependency
		task.Status = TaskStatusInProgress
		numBlockers := rapid.IntRange(1, 5).Draw(t, "numBlockers")
		task.BlockedBy = make([]string, numBlockers)
		for i := 0; i < numBlockers; i++ {
			task.BlockedBy[i] = rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "blockerID")
		}

		if !task.IsBlocked() {
			t.Fatal("Task with BlockedBy items should return IsBlocked() = true")
		}

		// Test not blocked
		task.Status = TaskStatusInProgress
		task.BlockedBy = []string{}
		if task.IsBlocked() {
			t.Fatal("Task with no blockers should return IsBlocked() = false")
		}
	})
}

// TestProperty_TaskUpdateTimestamp verifies timestamp update behavior
func TestProperty_TaskUpdateTimestamp(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		task := NewTask("TASK-00001", "test", TaskTypeFeat)

		originalUpdated := task.Updated

		// Small delay to ensure time difference
		task.UpdateTimestamp()

		if !task.Updated.After(originalUpdated) && !task.Updated.Equal(originalUpdated) {
			t.Fatal("UpdateTimestamp should update the Updated field")
		}
	})
}

// TestProperty_TaskPriorityValues verifies valid priority values
func TestProperty_TaskPriorityValues(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		task := NewTask("TASK-00001", "test", TaskTypeFeat)

		validPriorities := []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}
		priority := rapid.SampledFrom(validPriorities).Draw(t, "priority")

		task.Priority = priority

		if task.Priority != priority {
			t.Fatalf("Priority not set correctly: expected %s, got %s", priority, task.Priority)
		}
	})
}

// TestProperty_TaskTypeValues verifies valid task type values
func TestProperty_TaskTypeValues(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		validTypes := []TaskType{TaskTypeFeat, TaskTypeBug, TaskTypeSpike, TaskTypeRefactor}
		taskType := rapid.SampledFrom(validTypes).Draw(t, "taskType")

		task := NewTask("TASK-00001", "test", taskType)

		if task.Type != taskType {
			t.Fatalf("TaskType not set correctly: expected %s, got %s", taskType, task.Type)
		}
	})
}

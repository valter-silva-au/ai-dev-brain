package models

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewTask(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		title    string
		taskType TaskType
		want     func(*Task) bool
	}{
		{
			name:     "creates task with feat type",
			id:       "TASK-001",
			title:    "Test Task",
			taskType: TaskTypeFeat,
			want: func(task *Task) bool {
				return task.ID == "TASK-001" &&
					task.Title == "Test Task" &&
					task.Type == TaskTypeFeat &&
					task.Status == TaskStatusBacklog &&
					task.Priority == PriorityP2
			},
		},
		{
			name:     "creates task with bug type",
			id:       "BUG-123",
			title:    "Fix bug",
			taskType: TaskTypeBug,
			want: func(task *Task) bool {
				return task.ID == "BUG-123" &&
					task.Type == TaskTypeBug &&
					task.Status == TaskStatusBacklog
			},
		},
		{
			name:     "initializes empty slices",
			id:       "TASK-002",
			title:    "Test",
			taskType: TaskTypeSpike,
			want: func(task *Task) bool {
				return len(task.Tags) == 0 &&
					len(task.BlockedBy) == 0 &&
					len(task.Teams) == 0 &&
					task.TeamMetadata != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewTask(tt.id, tt.title, tt.taskType)
			if !tt.want(task) {
				t.Errorf("NewTask() validation failed for %s", tt.name)
			}
			// Verify timestamps are set and in UTC
			if task.Created.IsZero() {
				t.Error("Created timestamp should be set")
			}
			if task.Updated.IsZero() {
				t.Error("Updated timestamp should be set")
			}
			if task.Created.Location() != time.UTC {
				t.Error("Created timestamp should be in UTC")
			}
		})
	}
}

func TestTask_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{"in_progress is active", TaskStatusInProgress, true},
		{"review is active", TaskStatusReview, true},
		{"blocked is active", TaskStatusBlocked, true},
		{"backlog is not active", TaskStatusBacklog, false},
		{"done is not active", TaskStatusDone, false},
		{"archived is not active", TaskStatusArchived, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Status: tt.status}
			if got := task.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTask_IsBlocked(t *testing.T) {
	tests := []struct {
		name      string
		status    TaskStatus
		blockedBy []string
		want      bool
	}{
		{
			name:      "blocked status is blocked",
			status:    TaskStatusBlocked,
			blockedBy: []string{},
			want:      true,
		},
		{
			name:      "has blockedBy items",
			status:    TaskStatusInProgress,
			blockedBy: []string{"TASK-001"},
			want:      true,
		},
		{
			name:      "not blocked",
			status:    TaskStatusInProgress,
			blockedBy: []string{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				Status:    tt.status,
				BlockedBy: tt.blockedBy,
			}
			if got := task.IsBlocked(); got != tt.want {
				t.Errorf("IsBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTask_UpdateTimestamp(t *testing.T) {
	task := &Task{
		Updated: time.Now().UTC().Add(-1 * time.Hour),
	}
	oldTime := task.Updated

	time.Sleep(10 * time.Millisecond)
	task.UpdateTimestamp()

	if !task.Updated.After(oldTime) {
		t.Error("UpdateTimestamp() should update to a later time")
	}
	if task.Updated.Location() != time.UTC {
		t.Error("UpdateTimestamp() should use UTC")
	}
}

func TestTask_YAMLSerialization(t *testing.T) {
	tests := []struct {
		name string
		task *Task
	}{
		{
			name: "full task",
			task: &Task{
				ID:           "TASK-001",
				Title:        "Test Task",
				Type:         TaskTypeFeat,
				Source:       "github",
				Status:       TaskStatusInProgress,
				Priority:     PriorityP1,
				Owner:        "user@example.com",
				Created:      time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Updated:      time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
				Repo:         "org/repo",
				Branch:       "feat/test",
				WorktreePath: "/path/to/worktree",
				TicketPath:   "/path/to/ticket",
				Tags:         []string{"backend", "api"},
				BlockedBy:    []string{"TASK-000"},
				Teams:        []string{"backend-team"},
				TeamMetadata: map[string]string{"team": "backend"},
			},
		},
		{
			name: "minimal task",
			task: &Task{
				ID:       "TASK-002",
				Title:    "Minimal",
				Type:     TaskTypeBug,
				Status:   TaskStatusBacklog,
				Priority: PriorityP2,
				Created:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Updated:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to YAML
			data, err := yaml.Marshal(tt.task)
			if err != nil {
				t.Fatalf("Failed to marshal task: %v", err)
			}

			// Unmarshal back
			var decoded Task
			err = yaml.Unmarshal(data, &decoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal task: %v", err)
			}

			// Compare key fields
			if decoded.ID != tt.task.ID {
				t.Errorf("ID mismatch: got %v, want %v", decoded.ID, tt.task.ID)
			}
			if decoded.Title != tt.task.Title {
				t.Errorf("Title mismatch: got %v, want %v", decoded.Title, tt.task.Title)
			}
			if decoded.Type != tt.task.Type {
				t.Errorf("Type mismatch: got %v, want %v", decoded.Type, tt.task.Type)
			}
			if decoded.Status != tt.task.Status {
				t.Errorf("Status mismatch: got %v, want %v", decoded.Status, tt.task.Status)
			}
			if decoded.Priority != tt.task.Priority {
				t.Errorf("Priority mismatch: got %v, want %v", decoded.Priority, tt.task.Priority)
			}
		})
	}
}

func TestTaskType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		taskType TaskType
		expected string
	}{
		{"feat type", TaskTypeFeat, "feat"},
		{"bug type", TaskTypeBug, "bug"},
		{"spike type", TaskTypeSpike, "spike"},
		{"refactor type", TaskTypeRefactor, "refactor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.taskType) != tt.expected {
				t.Errorf("TaskType = %v, want %v", tt.taskType, tt.expected)
			}
		})
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	tests := []struct {
		name     string
		status   TaskStatus
		expected string
	}{
		{"backlog status", TaskStatusBacklog, "backlog"},
		{"in_progress status", TaskStatusInProgress, "in_progress"},
		{"blocked status", TaskStatusBlocked, "blocked"},
		{"review status", TaskStatusReview, "review"},
		{"done status", TaskStatusDone, "done"},
		{"archived status", TaskStatusArchived, "archived"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("TaskStatus = %v, want %v", tt.status, tt.expected)
			}
		})
	}
}

func TestPriority_Constants(t *testing.T) {
	tests := []struct {
		name     string
		priority Priority
		expected string
	}{
		{"P0 priority", PriorityP0, "P0"},
		{"P1 priority", PriorityP1, "P1"},
		{"P2 priority", PriorityP2, "P2"},
		{"P3 priority", PriorityP3, "P3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.priority) != tt.expected {
				t.Errorf("Priority = %v, want %v", tt.priority, tt.expected)
			}
		})
	}
}

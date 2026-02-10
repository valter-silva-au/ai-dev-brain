package models

import "time"

// TaskType represents the type of work a task involves.
type TaskType string

const (
	TaskTypeFeat     TaskType = "feat"
	TaskTypeBug      TaskType = "bug"
	TaskTypeSpike    TaskType = "spike"
	TaskTypeRefactor TaskType = "refactor"
)

// TaskStatus represents the current lifecycle state of a task.
type TaskStatus string

const (
	StatusBacklog    TaskStatus = "backlog"
	StatusInProgress TaskStatus = "in_progress"
	StatusBlocked    TaskStatus = "blocked"
	StatusReview     TaskStatus = "review"
	StatusDone       TaskStatus = "done"
	StatusArchived   TaskStatus = "archived"
)

// Priority represents the urgency level of a task.
type Priority string

const (
	P0 Priority = "P0"
	P1 Priority = "P1"
	P2 Priority = "P2"
	P3 Priority = "P3"
)

// Task represents a unit of work identified by a unique TASK-XXXXX ID,
// containing all context, communications, and artifacts related to that work item.
type Task struct {
	ID           string     `yaml:"id"`
	Title        string     `yaml:"title"`
	Type         TaskType   `yaml:"type"`
	Status       TaskStatus `yaml:"status"`
	Priority     Priority   `yaml:"priority"`
	Owner        string     `yaml:"owner"`
	Repo         string     `yaml:"repo"`
	Branch       string     `yaml:"branch"`
	WorktreePath string     `yaml:"worktree"`
	TicketPath   string     `yaml:"ticket_path"`
	Created      time.Time  `yaml:"created"`
	Updated      time.Time  `yaml:"updated"`
	Tags         []string   `yaml:"tags"`
	BlockedBy    []string   `yaml:"blocked_by"`
	Related      []string   `yaml:"related"`
	Source       string     `yaml:"source,omitempty"`
}

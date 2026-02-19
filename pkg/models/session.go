package models

import "time"

// CapturedSession represents a recorded Claude Code session with metadata
// linking it to a task and project context.
type CapturedSession struct {
	ID          string    `yaml:"id"`
	SessionID   string    `yaml:"session_id"`
	TaskID      string    `yaml:"task_id,omitempty"`
	ProjectPath string    `yaml:"project_path"`
	GitBranch   string    `yaml:"git_branch,omitempty"`
	StartedAt   time.Time `yaml:"started_at"`
	EndedAt     time.Time `yaml:"ended_at"`
	Duration    string    `yaml:"duration"`
	TurnCount   int       `yaml:"turn_count"`
	Summary     string    `yaml:"summary,omitempty"`
	Tags        []string  `yaml:"tags,omitempty"`
}

// SessionTurn represents a single user or assistant turn within a captured session.
type SessionTurn struct {
	Index     int       `yaml:"index"`
	Role      string    `yaml:"role"`
	Timestamp time.Time `yaml:"timestamp"`
	Content   string    `yaml:"content"`
	Digest    string    `yaml:"digest,omitempty"`
	ToolsUsed []string  `yaml:"tools_used,omitempty"`
}

// SessionFilter specifies criteria for querying captured sessions.
type SessionFilter struct {
	TaskID      string
	ProjectPath string
	Since       *time.Time
	Until       *time.Time
	MinTurns    int
}

// SessionCaptureConfig holds configuration for the automatic session capture feature.
type SessionCaptureConfig struct {
	Enabled         bool   `yaml:"enabled" mapstructure:"enabled"`
	LLMSummarize    bool   `yaml:"llm_summarize" mapstructure:"llm_summarize"`
	LLMProvider     string `yaml:"llm_provider" mapstructure:"llm_provider"`
	LLMModel        string `yaml:"llm_model" mapstructure:"llm_model"`
	MaxTurnsStored  int    `yaml:"max_turns_stored" mapstructure:"max_turns_stored"`
	MinTurnsCapture int    `yaml:"min_turns_capture" mapstructure:"min_turns_capture"`
}

// SessionIndex is the master index of all captured sessions.
type SessionIndex struct {
	Version  string            `yaml:"version"`
	Sessions []CapturedSession `yaml:"sessions"`
}

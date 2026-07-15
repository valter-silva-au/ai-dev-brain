package models

import "time"

// SessionTurn represents a single turn in a captured session
type SessionTurn struct {
	Index     int       `yaml:"index"`
	Role      string    `yaml:"role"` // "user" or "assistant"
	Timestamp time.Time `yaml:"timestamp"`
	Content   string    `yaml:"content"`
	ToolCalls []string  `yaml:"tool_calls,omitempty"`
	Artifacts []string  `yaml:"artifacts,omitempty"`
}

// CapturedSession represents a captured AI session
type CapturedSession struct {
	ID          string            `yaml:"id"`
	TaskID      string            `yaml:"task_id,omitempty"`
	StartTime   time.Time         `yaml:"start_time"`
	EndTime     time.Time         `yaml:"end_time,omitempty"`
	Duration    int               `yaml:"duration,omitempty"` // in seconds
	Turns       []SessionTurn     `yaml:"turns"`
	Summary     string            `yaml:"summary,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
	ToolsUsed   []string          `yaml:"tools_used,omitempty"`
	FilesEdited []string          `yaml:"files_edited,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
}

// SessionFilter defines criteria for filtering sessions
type SessionFilter struct {
	TaskID      string    `yaml:"task_id,omitempty"`
	StartDate   time.Time `yaml:"start_date,omitempty"`
	EndDate     time.Time `yaml:"end_date,omitempty"`
	Tags        []string  `yaml:"tags,omitempty"`
	ToolsUsed   []string  `yaml:"tools_used,omitempty"`
	MinDuration int       `yaml:"min_duration,omitempty"` // in seconds
	MaxDuration int       `yaml:"max_duration,omitempty"` // in seconds
}

// SessionCaptureConfig defines configuration for session capture
type SessionCaptureConfig struct {
	Enabled            bool     `yaml:"enabled"`
	AutoCapture        bool     `yaml:"auto_capture"`
	CaptureTranscripts bool     `yaml:"capture_transcripts"`
	CaptureSummaries   bool     `yaml:"capture_summaries"`
	CaptureArtifacts   bool     `yaml:"capture_artifacts"`
	StoragePath        string   `yaml:"storage_path,omitempty"`
	MaxSessionSize     int      `yaml:"max_session_size,omitempty"` // in MB
	RetentionDays      int      `yaml:"retention_days,omitempty"`
	ExcludePatterns    []string `yaml:"exclude_patterns,omitempty"`
	IncludeTools       []string `yaml:"include_tools,omitempty"`
	ExcludeTools       []string `yaml:"exclude_tools,omitempty"`
}

// NewCapturedSession creates a new CapturedSession with default values
func NewCapturedSession(id string) *CapturedSession {
	return &CapturedSession{
		ID:          id,
		StartTime:   time.Now().UTC(),
		Turns:       []SessionTurn{},
		Tags:        []string{},
		ToolsUsed:   []string{},
		FilesEdited: []string{},
		Metadata:    make(map[string]string),
	}
}

// AddTurn adds a turn to the session
func (s *CapturedSession) AddTurn(turn SessionTurn) {
	s.Turns = append(s.Turns, turn)
}

// Finalize marks the session as complete and calculates duration
func (s *CapturedSession) Finalize() {
	s.EndTime = time.Now().UTC()
	s.Duration = int(s.EndTime.Sub(s.StartTime).Seconds())
}

// DefaultSessionCaptureConfig returns a SessionCaptureConfig with sensible defaults
func DefaultSessionCaptureConfig() *SessionCaptureConfig {
	return &SessionCaptureConfig{
		Enabled:            true,
		AutoCapture:        false,
		CaptureTranscripts: true,
		CaptureSummaries:   true,
		CaptureArtifacts:   false,
		MaxSessionSize:     100, // 100 MB
		RetentionDays:      90,
		ExcludePatterns:    []string{},
		IncludeTools:       []string{},
		ExcludeTools:       []string{},
	}
}

// Matches checks if a session matches the filter criteria
func (f *SessionFilter) Matches(session *CapturedSession) bool {
	// Check TaskID
	if f.TaskID != "" && session.TaskID != f.TaskID {
		return false
	}

	// Check start date
	if !f.StartDate.IsZero() && session.StartTime.Before(f.StartDate) {
		return false
	}

	// Check end date
	if !f.EndDate.IsZero() && session.StartTime.After(f.EndDate) {
		return false
	}

	// Check tags
	if len(f.Tags) > 0 {
		tagMatch := false
		for _, filterTag := range f.Tags {
			for _, sessionTag := range session.Tags {
				if filterTag == sessionTag {
					tagMatch = true
					break
				}
			}
			if tagMatch {
				break
			}
		}
		if !tagMatch {
			return false
		}
	}

	// Check tools used
	if len(f.ToolsUsed) > 0 {
		toolMatch := false
		for _, filterTool := range f.ToolsUsed {
			for _, sessionTool := range session.ToolsUsed {
				if filterTool == sessionTool {
					toolMatch = true
					break
				}
			}
			if toolMatch {
				break
			}
		}
		if !toolMatch {
			return false
		}
	}

	// Check duration
	if f.MinDuration > 0 && session.Duration < f.MinDuration {
		return false
	}
	if f.MaxDuration > 0 && session.Duration > f.MaxDuration {
		return false
	}

	return true
}

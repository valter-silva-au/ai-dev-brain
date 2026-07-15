package models

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewCapturedSession(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{"creates session with ID", "S-001"},
		{"creates session with different ID", "SESSION-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewCapturedSession(tt.sessionID)

			if session.ID != tt.sessionID {
				t.Errorf("ID = %v, want %v", session.ID, tt.sessionID)
			}
			if session.StartTime.IsZero() {
				t.Error("StartTime should be set")
			}
			if session.StartTime.Location() != time.UTC {
				t.Error("StartTime should be in UTC")
			}
			if session.Turns == nil {
				t.Error("Turns should be initialized")
			}
			if session.Tags == nil {
				t.Error("Tags should be initialized")
			}
			if session.Metadata == nil {
				t.Error("Metadata should be initialized")
			}
		})
	}
}

func TestCapturedSession_AddTurn(t *testing.T) {
	session := NewCapturedSession("S-001")

	turns := []SessionTurn{
		{
			Index:     1,
			Role:      "user",
			Timestamp: time.Now().UTC(),
			Content:   "Hello",
		},
		{
			Index:     2,
			Role:      "assistant",
			Timestamp: time.Now().UTC(),
			Content:   "Hi there",
			ToolCalls: []string{"Read", "Write"},
		},
	}

	for _, turn := range turns {
		session.AddTurn(turn)
	}

	if len(session.Turns) != len(turns) {
		t.Errorf("Turns length = %v, want %v", len(session.Turns), len(turns))
	}

	for i, turn := range session.Turns {
		if turn.Index != turns[i].Index {
			t.Errorf("Turn[%d].Index = %v, want %v", i, turn.Index, turns[i].Index)
		}
		if turn.Role != turns[i].Role {
			t.Errorf("Turn[%d].Role = %v, want %v", i, turn.Role, turns[i].Role)
		}
	}
}

func TestCapturedSession_Finalize(t *testing.T) {
	session := NewCapturedSession("S-001")
	startTime := session.StartTime

	// Wait to ensure duration is measurable (at least 1 second)
	time.Sleep(1100 * time.Millisecond)

	session.Finalize()

	if session.EndTime.IsZero() {
		t.Error("EndTime should be set")
	}
	if session.EndTime.Location() != time.UTC {
		t.Error("EndTime should be in UTC")
	}
	if !session.EndTime.After(startTime) {
		t.Error("EndTime should be after StartTime")
	}
	if session.Duration < 1 {
		t.Errorf("Duration should be at least 1 second, got %d", session.Duration)
	}
}

func TestDefaultSessionCaptureConfig(t *testing.T) {
	config := DefaultSessionCaptureConfig()

	tests := []struct {
		name  string
		check func(*SessionCaptureConfig) bool
		desc  string
	}{
		{
			name: "enabled by default",
			check: func(c *SessionCaptureConfig) bool {
				return c.Enabled
			},
			desc: "Enabled should be true",
		},
		{
			name: "auto capture disabled",
			check: func(c *SessionCaptureConfig) bool {
				return !c.AutoCapture
			},
			desc: "AutoCapture should be false",
		},
		{
			name: "transcripts enabled",
			check: func(c *SessionCaptureConfig) bool {
				return c.CaptureTranscripts
			},
			desc: "CaptureTranscripts should be true",
		},
		{
			name: "summaries enabled",
			check: func(c *SessionCaptureConfig) bool {
				return c.CaptureSummaries
			},
			desc: "CaptureSummaries should be true",
		},
		{
			name: "artifacts disabled",
			check: func(c *SessionCaptureConfig) bool {
				return !c.CaptureArtifacts
			},
			desc: "CaptureArtifacts should be false",
		},
		{
			name: "max size is 100MB",
			check: func(c *SessionCaptureConfig) bool {
				return c.MaxSessionSize == 100
			},
			desc: "MaxSessionSize should be 100",
		},
		{
			name: "retention is 90 days",
			check: func(c *SessionCaptureConfig) bool {
				return c.RetentionDays == 90
			},
			desc: "RetentionDays should be 90",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(config) {
				t.Errorf("%s", tt.desc)
			}
		})
	}
}

func TestSessionFilter_Matches(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		filter  SessionFilter
		session *CapturedSession
		want    bool
	}{
		{
			name: "matches task ID",
			filter: SessionFilter{
				TaskID: "TASK-001",
			},
			session: &CapturedSession{
				ID:        "S-001",
				TaskID:    "TASK-001",
				StartTime: baseTime,
			},
			want: true,
		},
		{
			name: "does not match task ID",
			filter: SessionFilter{
				TaskID: "TASK-002",
			},
			session: &CapturedSession{
				ID:        "S-001",
				TaskID:    "TASK-001",
				StartTime: baseTime,
			},
			want: false,
		},
		{
			name: "matches start date",
			filter: SessionFilter{
				StartDate: baseTime.Add(-1 * time.Hour),
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
			},
			want: true,
		},
		{
			name: "before start date",
			filter: SessionFilter{
				StartDate: baseTime.Add(1 * time.Hour),
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
			},
			want: false,
		},
		{
			name: "matches end date",
			filter: SessionFilter{
				EndDate: baseTime.Add(1 * time.Hour),
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
			},
			want: true,
		},
		{
			name: "after end date",
			filter: SessionFilter{
				EndDate: baseTime.Add(-1 * time.Hour),
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
			},
			want: false,
		},
		{
			name: "matches tags",
			filter: SessionFilter{
				Tags: []string{"important"},
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Tags:      []string{"important", "bug-fix"},
			},
			want: true,
		},
		{
			name: "does not match tags",
			filter: SessionFilter{
				Tags: []string{"feature"},
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Tags:      []string{"important", "bug-fix"},
			},
			want: false,
		},
		{
			name: "matches tools used",
			filter: SessionFilter{
				ToolsUsed: []string{"Read"},
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				ToolsUsed: []string{"Read", "Write"},
			},
			want: true,
		},
		{
			name: "matches min duration",
			filter: SessionFilter{
				MinDuration: 100,
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Duration:  120,
			},
			want: true,
		},
		{
			name: "below min duration",
			filter: SessionFilter{
				MinDuration: 200,
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Duration:  120,
			},
			want: false,
		},
		{
			name: "matches max duration",
			filter: SessionFilter{
				MaxDuration: 200,
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Duration:  120,
			},
			want: true,
		},
		{
			name: "exceeds max duration",
			filter: SessionFilter{
				MaxDuration: 100,
			},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
				Duration:  120,
			},
			want: false,
		},
		{
			name:   "empty filter matches all",
			filter: SessionFilter{},
			session: &CapturedSession{
				ID:        "S-001",
				StartTime: baseTime,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Matches(tt.session)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCapturedSession_YAMLSerialization(t *testing.T) {
	session := &CapturedSession{
		ID:        "S-001",
		TaskID:    "TASK-001",
		StartTime: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC),
		Duration:  3600,
		Turns: []SessionTurn{
			{
				Index:     1,
				Role:      "user",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Content:   "Test",
			},
		},
		Summary:     "Test session",
		Tags:        []string{"test"},
		ToolsUsed:   []string{"Read"},
		FilesEdited: []string{"file.go"},
		Metadata: map[string]string{
			"version": "1.0",
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(session)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	// Unmarshal back
	var decoded CapturedSession
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	// Verify key fields
	if decoded.ID != session.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, session.ID)
	}
	if decoded.TaskID != session.TaskID {
		t.Errorf("TaskID = %v, want %v", decoded.TaskID, session.TaskID)
	}
	if decoded.Duration != session.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, session.Duration)
	}
	if len(decoded.Turns) != len(session.Turns) {
		t.Errorf("Turns length = %v, want %v", len(decoded.Turns), len(session.Turns))
	}
}

func TestSessionTurn_YAMLSerialization(t *testing.T) {
	turn := SessionTurn{
		Index:     1,
		Role:      "assistant",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Content:   "Test content",
		ToolCalls: []string{"Read", "Write"},
		Artifacts: []string{"output.txt"},
	}

	data, err := yaml.Marshal(turn)
	if err != nil {
		t.Fatalf("Failed to marshal turn: %v", err)
	}

	var decoded SessionTurn
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal turn: %v", err)
	}

	if decoded.Index != turn.Index {
		t.Errorf("Index = %v, want %v", decoded.Index, turn.Index)
	}
	if decoded.Role != turn.Role {
		t.Errorf("Role = %v, want %v", decoded.Role, turn.Role)
	}
	if decoded.Content != turn.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, turn.Content)
	}
}

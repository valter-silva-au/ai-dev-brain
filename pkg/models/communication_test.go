package models

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewCommunication(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		taskID  string
		content string
	}{
		{"creates communication", "C-001", "TASK-001", "Test content"},
		{"creates another communication", "C-002", "TASK-002", "Another content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comm := NewCommunication(tt.id, tt.taskID, tt.content)

			if comm.ID != tt.id {
				t.Errorf("ID = %v, want %v", comm.ID, tt.id)
			}
			if comm.TaskID != tt.taskID {
				t.Errorf("TaskID = %v, want %v", comm.TaskID, tt.taskID)
			}
			if comm.Content != tt.content {
				t.Errorf("Content = %v, want %v", comm.Content, tt.content)
			}
			if comm.Date.IsZero() {
				t.Error("Date should be set")
			}
			if comm.Date.Location() != time.UTC {
				t.Error("Date should be in UTC")
			}
			if comm.Tags == nil {
				t.Error("Tags should be initialized")
			}
			if comm.Metadata == nil {
				t.Error("Metadata should be initialized")
			}
		})
	}
}

func TestCommunication_AddTag(t *testing.T) {
	tests := []struct {
		name        string
		initialTags []CommunicationTag
		addTag      CommunicationTag
		expectedLen int
		shouldAdd   bool
	}{
		{
			name:        "add new tag",
			initialTags: []CommunicationTag{},
			addTag:      CommunicationTagQuestion,
			expectedLen: 1,
			shouldAdd:   true,
		},
		{
			name:        "add duplicate tag",
			initialTags: []CommunicationTag{CommunicationTagQuestion},
			addTag:      CommunicationTagQuestion,
			expectedLen: 1,
			shouldAdd:   false,
		},
		{
			name:        "add different tag",
			initialTags: []CommunicationTag{CommunicationTagQuestion},
			addTag:      CommunicationTagAnswer,
			expectedLen: 2,
			shouldAdd:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comm := &Communication{
				Tags: tt.initialTags,
			}

			comm.AddTag(tt.addTag)

			if len(comm.Tags) != tt.expectedLen {
				t.Errorf("Tags length = %v, want %v", len(comm.Tags), tt.expectedLen)
			}

			if tt.shouldAdd {
				found := false
				for _, tag := range comm.Tags {
					if tag == tt.addTag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Tag %v not found in tags", tt.addTag)
				}
			}
		})
	}
}

func TestCommunication_HasTag(t *testing.T) {
	tests := []struct {
		name string
		tags []CommunicationTag
		tag  CommunicationTag
		want bool
	}{
		{
			name: "has tag",
			tags: []CommunicationTag{CommunicationTagQuestion, CommunicationTagAnswer},
			tag:  CommunicationTagQuestion,
			want: true,
		},
		{
			name: "does not have tag",
			tags: []CommunicationTag{CommunicationTagQuestion},
			tag:  CommunicationTagAnswer,
			want: false,
		},
		{
			name: "empty tags",
			tags: []CommunicationTag{},
			tag:  CommunicationTagQuestion,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comm := &Communication{
				Tags: tt.tags,
			}

			if got := comm.HasTag(tt.tag); got != tt.want {
				t.Errorf("HasTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommunication_AddActionItem(t *testing.T) {
	comm := NewCommunication("C-001", "TASK-001", "Content")

	items := []ActionItem{
		{
			Description: "Item 1",
			Assignee:    "user1",
			Status:      "pending",
			CreatedAt:   time.Now().UTC(),
		},
		{
			Description: "Item 2",
			Assignee:    "user2",
			Status:      "done",
			CreatedAt:   time.Now().UTC(),
		},
	}

	for _, item := range items {
		comm.AddActionItem(item)
	}

	if len(comm.ActionItems) != len(items) {
		t.Errorf("ActionItems length = %v, want %v", len(comm.ActionItems), len(items))
	}

	for i, item := range comm.ActionItems {
		if item.Description != items[i].Description {
			t.Errorf("ActionItem[%d].Description = %v, want %v", i, item.Description, items[i].Description)
		}
	}
}

func TestNewActionItem(t *testing.T) {
	description := "Test action item"
	item := NewActionItem(description)

	if item.Description != description {
		t.Errorf("Description = %v, want %v", item.Description, description)
	}
	if item.Status != "pending" {
		t.Errorf("Status = %v, want pending", item.Status)
	}
	if item.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if item.CreatedAt.Location() != time.UTC {
		t.Error("CreatedAt should be in UTC")
	}
}

func TestActionItem_IsPending(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"pending is pending", "pending", true},
		{"in_progress is not pending", "in_progress", false},
		{"done is not pending", "done", false},
		{"cancelled is not pending", "cancelled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &ActionItem{Status: tt.status}
			if got := item.IsPending(); got != tt.want {
				t.Errorf("IsPending() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActionItem_IsDone(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"done is done", "done", true},
		{"pending is not done", "pending", false},
		{"in_progress is not done", "in_progress", false},
		{"cancelled is not done", "cancelled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &ActionItem{Status: tt.status}
			if got := item.IsDone(); got != tt.want {
				t.Errorf("IsDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActionItem_MarkDone(t *testing.T) {
	item := &ActionItem{
		Description: "Test",
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
	}

	item.MarkDone()

	if item.Status != "done" {
		t.Errorf("Status = %v, want done", item.Status)
	}
}

func TestCommunicationTag_Constants(t *testing.T) {
	tests := []struct {
		name     string
		tag      CommunicationTag
		expected string
	}{
		{"question tag", CommunicationTagQuestion, "question"},
		{"answer tag", CommunicationTagAnswer, "answer"},
		{"feedback tag", CommunicationTagFeedback, "feedback"},
		{"status_update tag", CommunicationTagStatusUpdate, "status_update"},
		{"blocker tag", CommunicationTagBlocker, "blocker"},
		{"decision tag", CommunicationTagDecision, "decision"},
		{"review tag", CommunicationTagReview, "review"},
		{"meeting tag", CommunicationTagMeeting, "meeting"},
		{"email tag", CommunicationTagEmail, "email"},
		{"slack tag", CommunicationTagSlack, "slack"},
		{"other tag", CommunicationTagOther, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.tag) != tt.expected {
				t.Errorf("CommunicationTag = %v, want %v", tt.tag, tt.expected)
			}
		})
	}
}

func TestCommunication_YAMLSerialization(t *testing.T) {
	comm := &Communication{
		ID:          "C-001",
		TaskID:      "TASK-001",
		Date:        time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		From:        "user@example.com",
		To:          []string{"team@example.com"},
		Subject:     "Test Subject",
		Content:     "Test content",
		Tags:        []CommunicationTag{CommunicationTagQuestion, CommunicationTagEmail},
		Channel:     "email",
		ThreadID:    "thread-123",
		References:  []string{"C-000"},
		Attachments: []string{"file.pdf"},
		ActionItems: []ActionItem{
			{
				Description: "Action 1",
				Assignee:    "user1",
				DueDate:     time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
				Status:      "pending",
				CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		Metadata: map[string]string{
			"priority": "high",
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(comm)
	if err != nil {
		t.Fatalf("Failed to marshal communication: %v", err)
	}

	// Unmarshal back
	var decoded Communication
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal communication: %v", err)
	}

	// Verify key fields
	if decoded.ID != comm.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, comm.ID)
	}
	if decoded.TaskID != comm.TaskID {
		t.Errorf("TaskID = %v, want %v", decoded.TaskID, comm.TaskID)
	}
	if decoded.Subject != comm.Subject {
		t.Errorf("Subject = %v, want %v", decoded.Subject, comm.Subject)
	}
	if decoded.Content != comm.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, comm.Content)
	}
	if decoded.Channel != comm.Channel {
		t.Errorf("Channel = %v, want %v", decoded.Channel, comm.Channel)
	}
	if len(decoded.Tags) != len(comm.Tags) {
		t.Errorf("Tags length = %v, want %v", len(decoded.Tags), len(comm.Tags))
	}
	if len(decoded.ActionItems) != len(comm.ActionItems) {
		t.Errorf("ActionItems length = %v, want %v", len(decoded.ActionItems), len(comm.ActionItems))
	}
}

func TestActionItem_YAMLSerialization(t *testing.T) {
	item := ActionItem{
		Description: "Test action",
		Assignee:    "user@example.com",
		DueDate:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Status:      "in_progress",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("Failed to marshal action item: %v", err)
	}

	var decoded ActionItem
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal action item: %v", err)
	}

	if decoded.Description != item.Description {
		t.Errorf("Description = %v, want %v", decoded.Description, item.Description)
	}
	if decoded.Assignee != item.Assignee {
		t.Errorf("Assignee = %v, want %v", decoded.Assignee, item.Assignee)
	}
	if decoded.Status != item.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, item.Status)
	}
}

func TestCommunication_MultipleTagsHandling(t *testing.T) {
	comm := NewCommunication("C-001", "TASK-001", "Content")

	tags := []CommunicationTag{
		CommunicationTagQuestion,
		CommunicationTagEmail,
		CommunicationTagBlocker,
	}

	for _, tag := range tags {
		comm.AddTag(tag)
	}

	if len(comm.Tags) != len(tags) {
		t.Errorf("Tags length = %v, want %v", len(comm.Tags), len(tags))
	}

	// Verify all tags are present
	for _, tag := range tags {
		if !comm.HasTag(tag) {
			t.Errorf("Tag %v not found", tag)
		}
	}
}

func TestCommunication_EmptyOptionalFields(t *testing.T) {
	comm := &Communication{
		ID:      "C-001",
		TaskID:  "TASK-001",
		Date:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Content: "Minimal content",
	}

	data, err := yaml.Marshal(comm)
	if err != nil {
		t.Fatalf("Failed to marshal communication: %v", err)
	}

	var decoded Communication
	err = yaml.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal communication: %v", err)
	}

	if decoded.ID != comm.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, comm.ID)
	}
	if decoded.Content != comm.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, comm.Content)
	}
}

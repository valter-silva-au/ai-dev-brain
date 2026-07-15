package models

import "time"

// CommunicationTag represents a tag for categorizing communications
type CommunicationTag string

const (
	CommunicationTagQuestion     CommunicationTag = "question"
	CommunicationTagAnswer       CommunicationTag = "answer"
	CommunicationTagFeedback     CommunicationTag = "feedback"
	CommunicationTagStatusUpdate CommunicationTag = "status_update"
	CommunicationTagBlocker      CommunicationTag = "blocker"
	CommunicationTagDecision     CommunicationTag = "decision"
	CommunicationTagReview       CommunicationTag = "review"
	CommunicationTagMeeting      CommunicationTag = "meeting"
	CommunicationTagEmail        CommunicationTag = "email"
	CommunicationTagSlack        CommunicationTag = "slack"
	CommunicationTagOther        CommunicationTag = "other"
)

// CommunicationDirection records whether a communication was received from a
// stakeholder (inbound) or sent to one (outbound). Empty means unspecified, so a
// pre-direction record marshals byte-identically.
type CommunicationDirection string

const (
	// DirectionInbound is a communication received from a stakeholder.
	DirectionInbound CommunicationDirection = "inbound"
	// DirectionOutbound is a communication sent to a stakeholder.
	DirectionOutbound CommunicationDirection = "outbound"
)

// IsValid reports whether d is one of the two canonical directions. An empty
// direction is NOT valid — callers that require one (the `adb comm` log surface)
// check this; storage tolerates an empty direction on read.
func (d CommunicationDirection) IsValid() bool {
	return d == DirectionInbound || d == DirectionOutbound
}

// Communication represents a communication with a stakeholder
type Communication struct {
	ID          string                 `yaml:"id"`
	TaskID      string                 `yaml:"task_id"`
	Direction   CommunicationDirection `yaml:"direction,omitempty"`
	Date        time.Time              `yaml:"date"`
	From        string                 `yaml:"from,omitempty"`
	To          []string               `yaml:"to,omitempty"`
	Subject     string                 `yaml:"subject,omitempty"`
	Content     string                 `yaml:"content"`
	Tags        []CommunicationTag     `yaml:"tags,omitempty"`
	Channel     string                 `yaml:"channel,omitempty"` // email, slack, teams, meeting, etc.
	ThreadID    string                 `yaml:"thread_id,omitempty"`
	References  []string               `yaml:"references,omitempty"` // IDs of related communications
	Attachments []string               `yaml:"attachments,omitempty"`
	ActionItems []ActionItem           `yaml:"action_items,omitempty"`
	Metadata    map[string]string      `yaml:"metadata,omitempty"`
}

// ActionItem represents an action item from a communication
type ActionItem struct {
	Description string    `yaml:"description"`
	Assignee    string    `yaml:"assignee,omitempty"`
	DueDate     time.Time `yaml:"due_date,omitempty"`
	Status      string    `yaml:"status"` // pending, in_progress, done, cancelled
	CreatedAt   time.Time `yaml:"created_at"`
}

// NewCommunication creates a new Communication with default values
func NewCommunication(id, taskID, content string) *Communication {
	return &Communication{
		ID:          id,
		TaskID:      taskID,
		Date:        time.Now().UTC(),
		Content:     content,
		Tags:        []CommunicationTag{},
		To:          []string{},
		References:  []string{},
		Attachments: []string{},
		ActionItems: []ActionItem{},
		Metadata:    make(map[string]string),
	}
}

// AddTag adds a tag to the communication
func (c *Communication) AddTag(tag CommunicationTag) {
	// Check if tag already exists
	for _, t := range c.Tags {
		if t == tag {
			return
		}
	}
	c.Tags = append(c.Tags, tag)
}

// AddActionItem adds an action item to the communication
func (c *Communication) AddActionItem(item ActionItem) {
	c.ActionItems = append(c.ActionItems, item)
}

// HasTag returns true if the communication has the specified tag
func (c *Communication) HasTag(tag CommunicationTag) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// NewActionItem creates a new ActionItem with default values
func NewActionItem(description string) ActionItem {
	return ActionItem{
		Description: description,
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
	}
}

// IsPending returns true if the action item is pending
func (a *ActionItem) IsPending() bool {
	return a.Status == "pending"
}

// IsDone returns true if the action item is done
func (a *ActionItem) IsDone() bool {
	return a.Status == "done"
}

// MarkDone marks the action item as done
func (a *ActionItem) MarkDone() {
	a.Status = "done"
}

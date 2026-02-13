package models

import "time"

// CommunicationTag represents a category tag applied to a communication entry.
type CommunicationTag string

const (
	TagRequirement CommunicationTag = "requirement"
	TagDecision    CommunicationTag = "decision"
	TagBlocker     CommunicationTag = "blocker"
	TagQuestion    CommunicationTag = "question"
	TagActionItem  CommunicationTag = "action_item"
)

// Communication represents a stakeholder communication entry
// stored as a chronological markdown file in a task's communications/ folder.
type Communication struct {
	Date    time.Time          `yaml:"date"`
	Source  string             `yaml:"source"`
	Contact string             `yaml:"contact"`
	Topic   string             `yaml:"topic"`
	Content string             `yaml:"content"`
	Tags    []CommunicationTag `yaml:"tags"`
}

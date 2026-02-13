package models

// ChannelType represents the kind of channel.
type ChannelType string

const (
	ChannelEmail    ChannelType = "email"
	ChannelSlack    ChannelType = "slack"
	ChannelTicket   ChannelType = "ticket"
	ChannelFile     ChannelType = "file"
	ChannelDocument ChannelType = "document"
)

// ChannelItemPriority represents urgency of a channel item.
type ChannelItemPriority string

const (
	ChannelPriorityHigh   ChannelItemPriority = "high"
	ChannelPriorityMedium ChannelItemPriority = "medium"
	ChannelPriorityLow    ChannelItemPriority = "low"
)

// ChannelItemStatus represents the processing state of a channel item.
type ChannelItemStatus string

const (
	ChannelStatusPending    ChannelItemStatus = "pending"
	ChannelStatusProcessed  ChannelItemStatus = "processed"
	ChannelStatusActionable ChannelItemStatus = "actionable"
	ChannelStatusArchived   ChannelItemStatus = "archived"
)

// ChannelItem represents an input item from any channel.
type ChannelItem struct {
	ID          string              `yaml:"id"`
	Channel     ChannelType         `yaml:"channel"`
	Source      string              `yaml:"source"` // e.g., "gmail", "slack-workspace"
	From        string              `yaml:"from"`
	Subject     string              `yaml:"subject"`
	Content     string              `yaml:"content"`
	Date        string              `yaml:"date"`
	Priority    ChannelItemPriority `yaml:"priority"`
	Status      ChannelItemStatus   `yaml:"status"`
	Tags        []string            `yaml:"tags,omitempty"`
	Metadata    map[string]string   `yaml:"metadata,omitempty"` // Channel-specific metadata
	RelatedTask string              `yaml:"related_task,omitempty"`
}

// OutputItem represents an output to be sent to a channel.
type OutputItem struct {
	ID          string            `yaml:"id"`
	Channel     ChannelType       `yaml:"channel"`
	Destination string            `yaml:"destination"` // e.g., email address, slack channel
	Subject     string            `yaml:"subject"`
	Content     string            `yaml:"content"`
	InReplyTo   string            `yaml:"in_reply_to,omitempty"` // ChannelItem ID this responds to
	SourceTask  string            `yaml:"source_task,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
}

// ChannelConfig holds configuration for a channel adapter.
type ChannelConfig struct {
	Name     string            `yaml:"name"`
	Type     ChannelType       `yaml:"type"`
	Enabled  bool              `yaml:"enabled"`
	Settings map[string]string `yaml:"settings,omitempty"`
}

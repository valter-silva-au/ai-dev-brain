package models

import "time"

// ProjectStatus represents the current lifecycle state of a project.
type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectArchived ProjectStatus = "archived"
	ProjectPaused   ProjectStatus = "paused"
)

// Project represents a project in the Hive Mind system.
type Project struct {
	Name                string        `yaml:"name"`
	RepoPath            string        `yaml:"repo_path"`
	Purpose             string        `yaml:"purpose"`
	TechStack           []string      `yaml:"tech_stack,omitempty"`
	Status              ProjectStatus `yaml:"status"`
	LastUpdated         time.Time     `yaml:"last_updated"`
	Tags                []string      `yaml:"tags,omitempty"`
	DefaultAI           string        `yaml:"default_ai"`
	RelatedProjects     []string      `yaml:"related_projects,omitempty"`
	KnowledgeEntryCount int           `yaml:"knowledge_entry_count"`
	DecisionCount       int           `yaml:"decision_count"`
	ActiveTaskCount     int           `yaml:"active_task_count"`
}

// ProjectFilter represents filter criteria for querying projects.
type ProjectFilter struct {
	Status    ProjectStatus `yaml:"status"`
	Tags      []string      `yaml:"tags,omitempty"`
	TechStack []string      `yaml:"tech_stack,omitempty"`
}

// AgentType represents the type of agent in the Hive Mind system.
type AgentType string

const (
	AgentClaudeCode AgentType = "claude-code"
	AgentOpenClaw   AgentType = "openclaw"
)

// AgentStatus represents the current operational state of an agent.
type AgentStatus string

const (
	AgentIdle    AgentStatus = "idle"
	AgentBusy    AgentStatus = "busy"
	AgentOffline AgentStatus = "offline"
)

// Agent represents an AI agent in the Hive Mind system.
type Agent struct {
	Name          string      `yaml:"name"`
	Type          AgentType   `yaml:"type"`
	Model         string      `yaml:"model"`
	Capabilities  []string    `yaml:"capabilities,omitempty"`
	Role          string      `yaml:"role"`
	Status        AgentStatus `yaml:"status"`
	LastSeen      time.Time   `yaml:"last_seen"`
	HomeProject   string      `yaml:"home_project"`
	ActiveTask    string      `yaml:"active_task"`
	SessionCount  int         `yaml:"session_count"`
	MemoryPath    string      `yaml:"memory_path"`
	WorkspacePath string      `yaml:"workspace_path"`
}

// AgentFilter represents filter criteria for querying agents.
type AgentFilter struct {
	Type         AgentType   `yaml:"type"`
	Capabilities []string    `yaml:"capabilities,omitempty"`
	Status       AgentStatus `yaml:"status"`
}

// HiveKnowledgeResult represents a knowledge entry result from the Hive Mind system.
type HiveKnowledgeResult struct {
	ID          string    `yaml:"id"`
	Project     string    `yaml:"project"`
	ProjectPath string    `yaml:"project_path"`
	LocalID     string    `yaml:"local_id"`
	Type        string    `yaml:"type"`
	Topic       string    `yaml:"topic"`
	Summary     string    `yaml:"summary"`
	Detail      string    `yaml:"detail"`
	SourceTask  string    `yaml:"source_task"`
	Date        time.Time `yaml:"date"`
	Tags        []string  `yaml:"tags,omitempty"`
}

// HiveContext represents the aggregated context from the Hive Mind system.
type HiveContext struct {
	CurrentProject    Project               `yaml:"current_project"`
	RelatedProjects   []Project             `yaml:"related_projects,omitempty"`
	RelatedKnowledge  []HiveKnowledgeResult `yaml:"related_knowledge,omitempty"`
	ActiveAgents      []Agent               `yaml:"active_agents,omitempty"`
	CrossRepoPatterns []string              `yaml:"cross_repo_patterns,omitempty"`
	Summary           string                `yaml:"summary"`
}

// HiveMessageType represents the type of message in the Hive Mind messaging system.
type HiveMessageType string

const (
	HiveMessageRequest  HiveMessageType = "request"
	HiveMessageResponse HiveMessageType = "response"
	HiveMessageNotify   HiveMessageType = "notify"
	HiveMessageQuery    HiveMessageType = "query"
)

// HiveMessageStatus represents the processing state of a message.
type HiveMessageStatus string

const (
	HiveMessagePending   HiveMessageStatus = "pending"
	HiveMessageDelivered HiveMessageStatus = "delivered"
	HiveMessageRead      HiveMessageStatus = "read"
	HiveMessageArchived  HiveMessageStatus = "archived"
)

// HiveMessage represents a message in the Hive Mind messaging system.
type HiveMessage struct {
	ID             string            `yaml:"id"`
	ConversationID string            `yaml:"conversation_id,omitempty"`
	From           string            `yaml:"from"`
	To             string            `yaml:"to"`
	Subject        string            `yaml:"subject"`
	Content        string            `yaml:"content"`
	Type           HiveMessageType   `yaml:"type"`
	Priority       string            `yaml:"priority"`
	Date           string            `yaml:"date"`
	InReplyTo      string            `yaml:"in_reply_to,omitempty"`
	Tags           []string          `yaml:"tags,omitempty"`
	Metadata       map[string]string `yaml:"metadata,omitempty"`
	Status         HiveMessageStatus `yaml:"status"`
}

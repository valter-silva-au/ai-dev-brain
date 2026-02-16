package models

// KnowledgeEntryType categorizes the kind of knowledge captured.
type KnowledgeEntryType string

const (
	KnowledgeTypeDecision     KnowledgeEntryType = "decision"
	KnowledgeTypeLearning     KnowledgeEntryType = "learning"
	KnowledgeTypePattern      KnowledgeEntryType = "pattern"
	KnowledgeTypeGotcha       KnowledgeEntryType = "gotcha"
	KnowledgeTypeRelationship KnowledgeEntryType = "relationship"
)

// KnowledgeSourceType identifies where a knowledge entry originated.
type KnowledgeSourceType string

const (
	SourceTaskArchive   KnowledgeSourceType = "task_archive"
	SourceCommunication KnowledgeSourceType = "communication"
	SourceEmail         KnowledgeSourceType = "email"
	SourceTicket        KnowledgeSourceType = "ticket"
	SourceManual        KnowledgeSourceType = "manual"
	SourceSession       KnowledgeSourceType = "session"
	SourceChannel       KnowledgeSourceType = "channel"
)

// EntityType categorizes entities in the knowledge graph.
type EntityType string

const (
	EntityPerson       EntityType = "person"
	EntitySystem       EntityType = "system"
	EntityOrganization EntityType = "organization"
	EntityProject      EntityType = "project"
)

// KnowledgeEntry represents a single piece of accumulated knowledge
// that persists across task lifecycles.
type KnowledgeEntry struct {
	ID         string              `yaml:"id"`
	Type       KnowledgeEntryType  `yaml:"type"`
	Topic      string              `yaml:"topic"`
	Summary    string              `yaml:"summary"`
	Detail     string              `yaml:"detail,omitempty"`
	SourceTask string              `yaml:"source_task,omitempty"`
	SourceType KnowledgeSourceType `yaml:"source_type"`
	Date       string              `yaml:"date"`
	Entities   []string            `yaml:"entities,omitempty"`
	Tags       []string            `yaml:"tags,omitempty"`
	Related    []string            `yaml:"related,omitempty"`
}

// KnowledgeIndex is the master index of all knowledge entries.
type KnowledgeIndex struct {
	Version string           `yaml:"version"`
	Entries []KnowledgeEntry `yaml:"entries"`
}

// Topic represents a theme that spans multiple tasks and
// groups related knowledge entries.
type Topic struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	RelatedTopics []string `yaml:"related_topics,omitempty"`
	Tasks         []string `yaml:"tasks,omitempty"`
	EntryCount    int      `yaml:"entry_count"`
}

// TopicGraph is the collection of all topics and their relationships.
type TopicGraph struct {
	Version string           `yaml:"version"`
	Topics  map[string]Topic `yaml:"topics"`
}

// Entity represents a person, system, or organization referenced
// across the knowledge base.
type Entity struct {
	Name        string     `yaml:"name"`
	Type        EntityType `yaml:"type"`
	Description string     `yaml:"description"`
	Role        string     `yaml:"role,omitempty"`
	Channels    []string   `yaml:"channels,omitempty"`
	FirstSeen   string     `yaml:"first_seen"`
	Tasks       []string   `yaml:"tasks,omitempty"`
	Knowledge   []string   `yaml:"knowledge,omitempty"`
}

// EntityRegistry is the collection of all known entities.
type EntityRegistry struct {
	Version  string            `yaml:"version"`
	Entities map[string]Entity `yaml:"entities"`
}

// TimelineEntry is a chronological knowledge event.
type TimelineEntry struct {
	Date        string `yaml:"date"`
	KnowledgeID string `yaml:"knowledge_id"`
	Event       string `yaml:"event"`
	Task        string `yaml:"task,omitempty"`
	Channel     string `yaml:"channel,omitempty"`
}

// Timeline is the chronological trail of knowledge accumulation.
type Timeline struct {
	Version string          `yaml:"version"`
	Entries []TimelineEntry `yaml:"entries"`
}

package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// KnowledgeStoreAccess is the subset of storage.KnowledgeStoreManager that the
// core package needs. Defining it here avoids importing the storage package.
type KnowledgeStoreAccess interface {
	AddEntry(entry models.KnowledgeEntry) (string, error)
	GetEntry(id string) (*models.KnowledgeEntry, error)
	GetAllEntries() ([]models.KnowledgeEntry, error)
	QueryByTopic(topic string) ([]models.KnowledgeEntry, error)
	QueryByEntity(entity string) ([]models.KnowledgeEntry, error)
	QueryByTags(tags []string) ([]models.KnowledgeEntry, error)
	Search(query string) ([]models.KnowledgeEntry, error)
	GetTopics() (*models.TopicGraph, error)
	AddTopic(topic models.Topic) error
	GetTopic(name string) (*models.Topic, error)
	GetEntities() (*models.EntityRegistry, error)
	AddEntity(entity models.Entity) error
	GetEntity(name string) (*models.Entity, error)
	GetTimeline(since time.Time) ([]models.TimelineEntry, error)
	AddTimelineEntry(entry models.TimelineEntry) error
	GenerateID() (string, error)
	Load() error
	Save() error
}

// KnowledgeManager provides business logic for the long-term knowledge
// persistence layer. It coordinates adding knowledge entries, maintaining the
// topic graph and entity registry, and querying accumulated knowledge.
type KnowledgeManager interface {
	// AddKnowledge adds a knowledge entry with automatic topic/entity/timeline updates.
	AddKnowledge(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error)

	// IngestFromExtraction feeds extracted task knowledge into the store.
	IngestFromExtraction(knowledge *models.ExtractedKnowledge) ([]string, error)

	// Query operations.
	Search(query string) ([]models.KnowledgeEntry, error)
	QueryByTopic(topic string) ([]models.KnowledgeEntry, error)
	QueryByEntity(entity string) ([]models.KnowledgeEntry, error)
	QueryByTags(tags []string) ([]models.KnowledgeEntry, error)

	// Topic operations.
	ListTopics() (*models.TopicGraph, error)
	GetTopicEntries(topic string) ([]models.KnowledgeEntry, error)

	// Timeline operations.
	GetTimeline(since time.Time) ([]models.TimelineEntry, error)

	// GetRelatedKnowledge returns knowledge entries related to a task based on
	// the task's tags, branch name, and related task IDs.
	GetRelatedKnowledge(task models.Task) ([]models.KnowledgeEntry, error)

	// AssembleKnowledgeSummary produces a markdown summary suitable for inclusion
	// in CLAUDE.md or task context files.
	AssembleKnowledgeSummary(maxEntries int) (string, error)
}

type knowledgeManager struct {
	store KnowledgeStoreAccess
}

// NewKnowledgeManager creates a KnowledgeManager backed by the given store.
func NewKnowledgeManager(store KnowledgeStoreAccess) KnowledgeManager {
	return &knowledgeManager{store: store}
}

func (km *knowledgeManager) AddKnowledge(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
	id, err := km.store.GenerateID()
	if err != nil {
		return "", fmt.Errorf("adding knowledge: %w", err)
	}

	entry := models.KnowledgeEntry{
		ID:         id,
		Type:       entryType,
		Topic:      topic,
		Summary:    summary,
		Detail:     detail,
		SourceTask: sourceTask,
		SourceType: sourceType,
		Date:       time.Now().UTC().Format("2006-01-02"),
		Entities:   entities,
		Tags:       tags,
	}

	if _, err := km.store.AddEntry(entry); err != nil {
		return "", fmt.Errorf("adding knowledge: %w", err)
	}

	// Update topic graph if a topic is specified.
	if topic != "" {
		topicObj := models.Topic{
			Name: topic,
		}
		if sourceTask != "" {
			topicObj.Tasks = []string{sourceTask}
		}
		if err := km.store.AddTopic(topicObj); err != nil {
			return "", fmt.Errorf("adding knowledge: updating topic: %w", err)
		}
	}

	// Update entity registry.
	for _, entityName := range entities {
		entity := models.Entity{
			Name:      entityName,
			Type:      models.EntitySystem, // Default; caller can update.
			FirstSeen: entry.Date,
			Knowledge: []string{id},
		}
		if sourceTask != "" {
			entity.Tasks = []string{sourceTask}
		}
		if err := km.store.AddEntity(entity); err != nil {
			return "", fmt.Errorf("adding knowledge: updating entity %q: %w", entityName, err)
		}
	}

	// Add timeline entry.
	timelineEntry := models.TimelineEntry{
		Date:        entry.Date,
		KnowledgeID: id,
		Event:       fmt.Sprintf("%s: %s", entryType, summary),
		Task:        sourceTask,
	}
	if err := km.store.AddTimelineEntry(timelineEntry); err != nil {
		return "", fmt.Errorf("adding knowledge: updating timeline: %w", err)
	}

	if err := km.store.Save(); err != nil {
		return "", fmt.Errorf("adding knowledge: saving: %w", err)
	}

	return id, nil
}

// IngestFromExtraction feeds all knowledge items from an ExtractedKnowledge
// into the knowledge store, returning the IDs of all created entries.
func (km *knowledgeManager) IngestFromExtraction(knowledge *models.ExtractedKnowledge) ([]string, error) {
	if knowledge == nil {
		return nil, nil
	}

	var ids []string

	// Ingest decisions.
	for _, d := range knowledge.Decisions {
		id, err := km.AddKnowledge(
			models.KnowledgeTypeDecision,
			"", // Topic auto-detected or left blank.
			d.Title,
			d.Decision,
			knowledge.TaskID,
			models.SourceTaskArchive,
			nil,
			nil,
		)
		if err != nil {
			return ids, fmt.Errorf("ingesting decision from %s: %w", knowledge.TaskID, err)
		}
		ids = append(ids, id)
	}

	// Ingest learnings.
	for _, l := range knowledge.Learnings {
		id, err := km.AddKnowledge(
			models.KnowledgeTypeLearning,
			"",
			l,
			"",
			knowledge.TaskID,
			models.SourceTaskArchive,
			nil,
			nil,
		)
		if err != nil {
			return ids, fmt.Errorf("ingesting learning from %s: %w", knowledge.TaskID, err)
		}
		ids = append(ids, id)
	}

	// Ingest gotchas.
	for _, g := range knowledge.Gotchas {
		id, err := km.AddKnowledge(
			models.KnowledgeTypeGotcha,
			"",
			g,
			"",
			knowledge.TaskID,
			models.SourceTaskArchive,
			nil,
			nil,
		)
		if err != nil {
			return ids, fmt.Errorf("ingesting gotcha from %s: %w", knowledge.TaskID, err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (km *knowledgeManager) Search(query string) ([]models.KnowledgeEntry, error) {
	return km.store.Search(query)
}

func (km *knowledgeManager) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	return km.store.QueryByTopic(topic)
}

func (km *knowledgeManager) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	return km.store.QueryByEntity(entity)
}

func (km *knowledgeManager) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	return km.store.QueryByTags(tags)
}

func (km *knowledgeManager) ListTopics() (*models.TopicGraph, error) {
	return km.store.GetTopics()
}

func (km *knowledgeManager) GetTopicEntries(topic string) ([]models.KnowledgeEntry, error) {
	return km.store.QueryByTopic(topic)
}

func (km *knowledgeManager) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	return km.store.GetTimeline(since)
}

// GetRelatedKnowledge searches for knowledge entries that are related to a task
// based on the task's tags, branch name, and related task IDs.
func (km *knowledgeManager) GetRelatedKnowledge(task models.Task) ([]models.KnowledgeEntry, error) {
	seen := make(map[string]struct{})
	var result []models.KnowledgeEntry

	addUnique := func(entries []models.KnowledgeEntry) {
		for _, e := range entries {
			if _, ok := seen[e.ID]; !ok {
				seen[e.ID] = struct{}{}
				result = append(result, e)
			}
		}
	}

	// Search by task tags.
	if len(task.Tags) > 0 {
		entries, err := km.store.QueryByTags(task.Tags)
		if err != nil {
			return nil, fmt.Errorf("getting related knowledge: %w", err)
		}
		addUnique(entries)
	}

	// Search by branch name keywords (split on hyphens).
	if task.Branch != "" {
		keywords := strings.Split(task.Branch, "-")
		for _, kw := range keywords {
			if len(kw) < 4 {
				continue
			}
			entries, err := km.store.Search(kw)
			if err != nil {
				return nil, fmt.Errorf("getting related knowledge: %w", err)
			}
			addUnique(entries)
		}
	}

	// Search by related task IDs.
	for _, relatedID := range task.Related {
		all, err := km.store.GetAllEntries()
		if err != nil {
			return nil, fmt.Errorf("getting related knowledge: %w", err)
		}
		for _, e := range all {
			if e.SourceTask == relatedID {
				addUnique([]models.KnowledgeEntry{e})
			}
		}
	}

	return result, nil
}

// AssembleKnowledgeSummary produces a markdown summary of accumulated knowledge
// suitable for inclusion in CLAUDE.md.
func (km *knowledgeManager) AssembleKnowledgeSummary(maxEntries int) (string, error) {
	topics, err := km.store.GetTopics()
	if err != nil {
		return "", fmt.Errorf("assembling knowledge summary: %w", err)
	}

	// Recent timeline entries.
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
	timeline, err := km.store.GetTimeline(thirtyDaysAgo)
	if err != nil {
		return "", fmt.Errorf("assembling knowledge summary: %w", err)
	}

	var sb strings.Builder

	// Topics section.
	if len(topics.Topics) > 0 {
		sb.WriteString("### Knowledge Topics\n\n")
		sb.WriteString("| Topic | Description | Tasks | Entries |\n")
		sb.WriteString("|-------|-------------|-------|---------|\n")
		for _, topic := range topics.Topics {
			taskList := strings.Join(topic.Tasks, ", ")
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d |\n",
				topic.Name, topic.Description, taskList, topic.EntryCount))
		}
		sb.WriteString("\n")
	}

	// Recent knowledge timeline.
	if len(timeline) > 0 {
		sb.WriteString("### Recent Knowledge (last 30 days)\n\n")
		limit := len(timeline)
		if maxEntries > 0 && limit > maxEntries {
			limit = maxEntries
		}
		// Show most recent first.
		for i := len(timeline) - 1; i >= len(timeline)-limit; i-- {
			entry := timeline[i]
			taskRef := ""
			if entry.Task != "" {
				taskRef = fmt.Sprintf(" (%s)", entry.Task)
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s%s\n", entry.Date, entry.Event, taskRef))
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return "", nil
	}

	return sb.String(), nil
}

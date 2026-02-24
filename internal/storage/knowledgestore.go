package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// KnowledgeStoreManager defines the interface for managing the long-term
// knowledge persistence layer under docs/knowledge/.
type KnowledgeStoreManager interface {
	// Entry operations.
	AddEntry(entry models.KnowledgeEntry) (string, error)
	GetEntry(id string) (*models.KnowledgeEntry, error)
	GetAllEntries() ([]models.KnowledgeEntry, error)
	QueryByTopic(topic string) ([]models.KnowledgeEntry, error)
	QueryByEntity(entity string) ([]models.KnowledgeEntry, error)
	QueryByTags(tags []string) ([]models.KnowledgeEntry, error)
	Search(query string) ([]models.KnowledgeEntry, error)

	// Topic operations.
	GetTopics() (*models.TopicGraph, error)
	AddTopic(topic models.Topic) error
	GetTopic(name string) (*models.Topic, error)

	// Entity operations.
	GetEntities() (*models.EntityRegistry, error)
	AddEntity(entity models.Entity) error
	GetEntity(name string) (*models.Entity, error)

	// Timeline operations.
	GetTimeline(since time.Time) ([]models.TimelineEntry, error)
	AddTimelineEntry(entry models.TimelineEntry) error

	// Persistence.
	Load() error
	Save() error

	// GenerateID returns the next sequential knowledge ID (K-XXXXX).
	GenerateID() (string, error)
}

type fileKnowledgeStore struct {
	basePath string
	index    models.KnowledgeIndex
	topics   models.TopicGraph
	entities models.EntityRegistry
	timeline models.Timeline
}

// NewKnowledgeStoreManager creates a KnowledgeStoreManager backed by YAML files
// under docs/knowledge/ in the given base directory.
func NewKnowledgeStoreManager(basePath string) KnowledgeStoreManager {
	return &fileKnowledgeStore{
		basePath: basePath,
		index: models.KnowledgeIndex{
			Version: "1.0",
			Entries: nil,
		},
		topics: models.TopicGraph{
			Version: "1.0",
			Topics:  make(map[string]models.Topic),
		},
		entities: models.EntityRegistry{
			Version:  "1.0",
			Entities: make(map[string]models.Entity),
		},
		timeline: models.Timeline{
			Version: "1.0",
			Entries: nil,
		},
	}
}

func (s *fileKnowledgeStore) knowledgeDir() string {
	return filepath.Join(s.basePath, "docs", "knowledge")
}

func (s *fileKnowledgeStore) indexPath() string {
	return filepath.Join(s.knowledgeDir(), "index.yaml")
}

func (s *fileKnowledgeStore) topicsPath() string {
	return filepath.Join(s.knowledgeDir(), "topics.yaml")
}

func (s *fileKnowledgeStore) entitiesPath() string {
	return filepath.Join(s.knowledgeDir(), "entities.yaml")
}

func (s *fileKnowledgeStore) timelinePath() string {
	return filepath.Join(s.knowledgeDir(), "timeline.yaml")
}

func (s *fileKnowledgeStore) counterPath() string {
	return filepath.Join(s.basePath, ".knowledge_counter")
}

// GenerateID reads and increments the knowledge counter file, returning the
// next sequential ID in K-XXXXX format.
func (s *fileKnowledgeStore) GenerateID() (string, error) {
	counterFile := s.counterPath()
	counter := 0

	data, err := os.ReadFile(counterFile)
	if err == nil {
		counter, err = strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return "", fmt.Errorf("generating knowledge ID: parsing counter: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("generating knowledge ID: reading counter: %w", err)
	}

	counter++
	id := fmt.Sprintf("K-%05d", counter)

	if err := os.WriteFile(counterFile, []byte(strconv.Itoa(counter)), 0o600); err != nil {
		return "", fmt.Errorf("generating knowledge ID: writing counter: %w", err)
	}
	return id, nil
}

// AddEntry adds a knowledge entry to the index. The entry must have an ID
// already assigned (via GenerateID).
func (s *fileKnowledgeStore) AddEntry(entry models.KnowledgeEntry) (string, error) {
	if entry.ID == "" {
		return "", fmt.Errorf("adding knowledge entry: ID must not be empty")
	}
	for _, existing := range s.index.Entries {
		if existing.ID == entry.ID {
			return "", fmt.Errorf("adding knowledge entry: %s already exists", entry.ID)
		}
	}
	s.index.Entries = append(s.index.Entries, entry)
	return entry.ID, nil
}

func (s *fileKnowledgeStore) GetEntry(id string) (*models.KnowledgeEntry, error) {
	for _, entry := range s.index.Entries {
		if entry.ID == id {
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("knowledge entry %s not found", id)
}

func (s *fileKnowledgeStore) GetAllEntries() ([]models.KnowledgeEntry, error) {
	result := make([]models.KnowledgeEntry, len(s.index.Entries))
	copy(result, s.index.Entries)
	return result, nil
}

func (s *fileKnowledgeStore) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	var result []models.KnowledgeEntry
	lower := strings.ToLower(topic)
	for _, entry := range s.index.Entries {
		if strings.ToLower(entry.Topic) == lower {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *fileKnowledgeStore) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	var result []models.KnowledgeEntry
	lower := strings.ToLower(entity)
	for _, entry := range s.index.Entries {
		for _, e := range entry.Entities {
			if strings.ToLower(e) == lower {
				result = append(result, entry)
				break
			}
		}
	}
	return result, nil
}

func (s *fileKnowledgeStore) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[strings.ToLower(t)] = struct{}{}
	}

	var result []models.KnowledgeEntry
	for _, entry := range s.index.Entries {
		for _, et := range entry.Tags {
			if _, ok := tagSet[strings.ToLower(et)]; ok {
				result = append(result, entry)
				break
			}
		}
	}
	return result, nil
}

// Search performs a case-insensitive keyword search across summary, detail,
// topic, tags, and entities of all knowledge entries.
func (s *fileKnowledgeStore) Search(query string) ([]models.KnowledgeEntry, error) {
	if query == "" {
		return nil, nil
	}
	lower := strings.ToLower(query)

	var result []models.KnowledgeEntry
	for _, entry := range s.index.Entries {
		if matchesSearch(entry, lower) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func matchesSearch(entry models.KnowledgeEntry, query string) bool {
	if strings.Contains(strings.ToLower(entry.Summary), query) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Detail), query) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Topic), query) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	for _, ent := range entry.Entities {
		if strings.Contains(strings.ToLower(ent), query) {
			return true
		}
	}
	return false
}

func (s *fileKnowledgeStore) GetTopics() (*models.TopicGraph, error) {
	cp := models.TopicGraph{
		Version: s.topics.Version,
		Topics:  make(map[string]models.Topic, len(s.topics.Topics)),
	}
	for k, v := range s.topics.Topics {
		cp.Topics[k] = v
	}
	return &cp, nil
}

func (s *fileKnowledgeStore) AddTopic(topic models.Topic) error {
	if topic.Name == "" {
		return fmt.Errorf("adding topic: name must not be empty")
	}
	key := strings.ToLower(topic.Name)
	if existing, ok := s.topics.Topics[key]; ok {
		// Merge: update description, merge related topics, merge tasks.
		if topic.Description != "" {
			existing.Description = topic.Description
		}
		existing.RelatedTopics = mergeStringSlices(existing.RelatedTopics, topic.RelatedTopics)
		existing.Tasks = mergeStringSlices(existing.Tasks, topic.Tasks)
		existing.EntryCount = countTopicEntries(s.index.Entries, key)
		s.topics.Topics[key] = existing
	} else {
		topic.EntryCount = countTopicEntries(s.index.Entries, key)
		s.topics.Topics[key] = topic
	}
	return nil
}

func (s *fileKnowledgeStore) GetTopic(name string) (*models.Topic, error) {
	key := strings.ToLower(name)
	topic, ok := s.topics.Topics[key]
	if !ok {
		return nil, fmt.Errorf("topic %q not found", name)
	}
	return &topic, nil
}

func (s *fileKnowledgeStore) GetEntities() (*models.EntityRegistry, error) {
	cp := models.EntityRegistry{
		Version:  s.entities.Version,
		Entities: make(map[string]models.Entity, len(s.entities.Entities)),
	}
	for k, v := range s.entities.Entities {
		cp.Entities[k] = v
	}
	return &cp, nil
}

func (s *fileKnowledgeStore) AddEntity(entity models.Entity) error {
	if entity.Name == "" {
		return fmt.Errorf("adding entity: name must not be empty")
	}
	key := strings.ToLower(entity.Name)
	if existing, ok := s.entities.Entities[key]; ok {
		// Merge: update description/role, merge tasks/knowledge/channels.
		if entity.Description != "" {
			existing.Description = entity.Description
		}
		if entity.Role != "" {
			existing.Role = entity.Role
		}
		existing.Channels = mergeStringSlices(existing.Channels, entity.Channels)
		existing.Tasks = mergeStringSlices(existing.Tasks, entity.Tasks)
		existing.Knowledge = mergeStringSlices(existing.Knowledge, entity.Knowledge)
		s.entities.Entities[key] = existing
	} else {
		s.entities.Entities[key] = entity
	}
	return nil
}

func (s *fileKnowledgeStore) GetEntity(name string) (*models.Entity, error) {
	key := strings.ToLower(name)
	entity, ok := s.entities.Entities[key]
	if !ok {
		return nil, fmt.Errorf("entity %q not found", name)
	}
	return &entity, nil
}

func (s *fileKnowledgeStore) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	var result []models.TimelineEntry
	for _, entry := range s.timeline.Entries {
		t, err := time.Parse("2006-01-02", entry.Date)
		if err != nil {
			continue
		}
		if !t.Before(since) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *fileKnowledgeStore) AddTimelineEntry(entry models.TimelineEntry) error {
	s.timeline.Entries = append(s.timeline.Entries, entry)
	// Keep sorted by date.
	sort.Slice(s.timeline.Entries, func(i, j int) bool {
		return s.timeline.Entries[i].Date < s.timeline.Entries[j].Date
	})
	return nil
}

func (s *fileKnowledgeStore) Load() error {
	if err := s.loadYAML(s.indexPath(), &s.index); err != nil {
		return fmt.Errorf("loading knowledge index: %w", err)
	}
	if s.index.Version == "" {
		s.index.Version = "1.0"
	}

	if err := s.loadYAML(s.topicsPath(), &s.topics); err != nil {
		return fmt.Errorf("loading topics: %w", err)
	}
	if s.topics.Version == "" {
		s.topics.Version = "1.0"
	}
	if s.topics.Topics == nil {
		s.topics.Topics = make(map[string]models.Topic)
	}

	if err := s.loadYAML(s.entitiesPath(), &s.entities); err != nil {
		return fmt.Errorf("loading entities: %w", err)
	}
	if s.entities.Version == "" {
		s.entities.Version = "1.0"
	}
	if s.entities.Entities == nil {
		s.entities.Entities = make(map[string]models.Entity)
	}

	if err := s.loadYAML(s.timelinePath(), &s.timeline); err != nil {
		return fmt.Errorf("loading timeline: %w", err)
	}
	if s.timeline.Version == "" {
		s.timeline.Version = "1.0"
	}
	return nil
}

func (s *fileKnowledgeStore) Save() error {
	dir := s.knowledgeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("saving knowledge store: creating directory: %w", err)
	}

	if err := s.saveYAML(s.indexPath(), &s.index); err != nil {
		return fmt.Errorf("saving knowledge index: %w", err)
	}
	if err := s.saveYAML(s.topicsPath(), &s.topics); err != nil {
		return fmt.Errorf("saving topics: %w", err)
	}
	if err := s.saveYAML(s.entitiesPath(), &s.entities); err != nil {
		return fmt.Errorf("saving entities: %w", err)
	}
	if err := s.saveYAML(s.timelinePath(), &s.timeline); err != nil {
		return fmt.Errorf("saving timeline: %w", err)
	}
	return nil
}

func (s *fileKnowledgeStore) loadYAML(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Missing files are initialized to zero values.
		}
		return err
	}
	return yaml.Unmarshal(data, target)
}

func (s *fileKnowledgeStore) saveYAML(path string, source interface{}) error {
	data, err := yaml.Marshal(source)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func mergeStringSlices(existing, additions []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		seen[s] = struct{}{}
	}
	for _, s := range additions {
		if _, ok := seen[s]; !ok {
			existing = append(existing, s)
			seen[s] = struct{}{}
		}
	}
	return existing
}

func countTopicEntries(entries []models.KnowledgeEntry, topic string) int {
	count := 0
	for _, e := range entries {
		if strings.ToLower(e.Topic) == topic {
			count++
		}
	}
	return count
}

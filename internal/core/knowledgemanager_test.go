package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// inMemoryKnowledgeStore is a test double for KnowledgeStoreAccess.
type inMemoryKnowledgeStore struct {
	entries  []models.KnowledgeEntry
	topics   map[string]models.Topic
	entities map[string]models.Entity
	timeline []models.TimelineEntry
	counter  int
}

func newInMemoryKnowledgeStore() *inMemoryKnowledgeStore {
	return &inMemoryKnowledgeStore{
		topics:   make(map[string]models.Topic),
		entities: make(map[string]models.Entity),
	}
}

func (s *inMemoryKnowledgeStore) GenerateID() (string, error) {
	s.counter++
	return fmt.Sprintf("K-%05d", s.counter), nil
}

func (s *inMemoryKnowledgeStore) AddEntry(entry models.KnowledgeEntry) (string, error) {
	s.entries = append(s.entries, entry)
	return entry.ID, nil
}

func (s *inMemoryKnowledgeStore) GetEntry(id string) (*models.KnowledgeEntry, error) {
	for _, e := range s.entries {
		if e.ID == id {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *inMemoryKnowledgeStore) GetAllEntries() ([]models.KnowledgeEntry, error) {
	return s.entries, nil
}

func (s *inMemoryKnowledgeStore) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	var result []models.KnowledgeEntry
	for _, e := range s.entries {
		if e.Topic == topic {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *inMemoryKnowledgeStore) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	var result []models.KnowledgeEntry
	for _, e := range s.entries {
		for _, ent := range e.Entities {
			if ent == entity {
				result = append(result, e)
				break
			}
		}
	}
	return result, nil
}

func (s *inMemoryKnowledgeStore) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	tagSet := make(map[string]struct{})
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}
	var result []models.KnowledgeEntry
	for _, e := range s.entries {
		for _, et := range e.Tags {
			if _, ok := tagSet[et]; ok {
				result = append(result, e)
				break
			}
		}
	}
	return result, nil
}

func (s *inMemoryKnowledgeStore) Search(query string) ([]models.KnowledgeEntry, error) {
	return s.entries, nil // Simplified for tests.
}

func (s *inMemoryKnowledgeStore) GetTopics() (*models.TopicGraph, error) {
	return &models.TopicGraph{Topics: s.topics}, nil
}

func (s *inMemoryKnowledgeStore) AddTopic(topic models.Topic) error {
	s.topics[topic.Name] = topic
	return nil
}

func (s *inMemoryKnowledgeStore) GetTopic(name string) (*models.Topic, error) {
	t, ok := s.topics[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return &t, nil
}

func (s *inMemoryKnowledgeStore) GetEntities() (*models.EntityRegistry, error) {
	return &models.EntityRegistry{Entities: s.entities}, nil
}

func (s *inMemoryKnowledgeStore) AddEntity(entity models.Entity) error {
	s.entities[entity.Name] = entity
	return nil
}

func (s *inMemoryKnowledgeStore) GetEntity(name string) (*models.Entity, error) {
	e, ok := s.entities[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return &e, nil
}

func (s *inMemoryKnowledgeStore) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	return s.timeline, nil
}

func (s *inMemoryKnowledgeStore) AddTimelineEntry(entry models.TimelineEntry) error {
	s.timeline = append(s.timeline, entry)
	return nil
}

func (s *inMemoryKnowledgeStore) Load() error { return nil }
func (s *inMemoryKnowledgeStore) Save() error { return nil }

func TestKnowledgeManager_AddKnowledge(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	id, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision,
		"auth",
		"Use JWT tokens",
		"Decided to use JWT for stateless auth",
		"TASK-00001",
		models.SourceTaskArchive,
		[]string{"jwt-lib"},
		[]string{"security"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "K-00001" {
		t.Errorf("expected K-00001, got %s", id)
	}

	// Verify entry was stored.
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(store.entries))
	}
	if store.entries[0].Summary != "Use JWT tokens" {
		t.Errorf("unexpected summary: %s", store.entries[0].Summary)
	}

	// Verify topic was created.
	if _, ok := store.topics["auth"]; !ok {
		t.Error("expected topic 'auth' to be created")
	}

	// Verify entity was created.
	if _, ok := store.entities["jwt-lib"]; !ok {
		t.Error("expected entity 'jwt-lib' to be created")
	}

	// Verify timeline entry.
	if len(store.timeline) != 1 {
		t.Fatalf("expected 1 timeline entry, got %d", len(store.timeline))
	}
}

func TestKnowledgeManager_IngestFromExtraction(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	knowledge := &models.ExtractedKnowledge{
		TaskID: "TASK-00042",
		Decisions: []models.Decision{
			{Title: "Use RS256 for JWT", Decision: "Asymmetric signing"},
			{Title: "Store tokens in httpOnly cookies", Decision: "Security best practice"},
		},
		Learnings: []string{
			"Go's crypto/rsa package handles key generation well",
		},
		Gotchas: []string{
			"Token refresh requires careful race condition handling",
		},
	}

	ids, err := mgr.IngestFromExtraction(knowledge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 4 {
		t.Errorf("expected 4 IDs (2 decisions + 1 learning + 1 gotcha), got %d", len(ids))
	}

	if len(store.entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(store.entries))
	}

	// Verify types.
	typeCount := map[models.KnowledgeEntryType]int{}
	for _, e := range store.entries {
		typeCount[e.Type]++
	}
	if typeCount[models.KnowledgeTypeDecision] != 2 {
		t.Errorf("expected 2 decisions, got %d", typeCount[models.KnowledgeTypeDecision])
	}
	if typeCount[models.KnowledgeTypeLearning] != 1 {
		t.Errorf("expected 1 learning, got %d", typeCount[models.KnowledgeTypeLearning])
	}
	if typeCount[models.KnowledgeTypeGotcha] != 1 {
		t.Errorf("expected 1 gotcha, got %d", typeCount[models.KnowledgeTypeGotcha])
	}
}

func TestKnowledgeManager_IngestFromExtractionNil(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	ids, err := mgr.IngestFromExtraction(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids != nil {
		t.Errorf("expected nil IDs for nil input, got %v", ids)
	}
}

func TestKnowledgeManager_GetRelatedKnowledge(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	// Pre-populate with some entries.
	store.entries = []models.KnowledgeEntry{
		{ID: "K-00001", Tags: []string{"security"}, Summary: "Auth decision", SourceTask: "TASK-00001"},
		{ID: "K-00002", Tags: []string{"frontend"}, Summary: "UI pattern", SourceTask: "TASK-00002"},
		{ID: "K-00003", Tags: []string{"security"}, Summary: "Encryption", SourceTask: "TASK-00003"},
	}

	task := models.Task{
		ID:   "TASK-00010",
		Tags: []string{"security"},
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 related entries (security tagged), got %d", len(results))
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	// Add topic and timeline data.
	store.topics["auth"] = models.Topic{
		Name: "auth", Description: "Authentication", EntryCount: 3, Tasks: []string{"TASK-00001"},
	}
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-15", KnowledgeID: "K-00001", Event: "decision: Use JWT", Task: "TASK-00001"},
	}

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummaryEmpty(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary for empty store, got: %s", summary)
	}
}

// errKnowledgeStore is a test double that returns errors for configurable operations.
type errKnowledgeStore struct {
	inMemoryKnowledgeStore
	addEntityErr     error
	addEntryErr      error
	generateIDErr    error
	addTopicErr      error
	addTimelineErr   error
	saveErr          error
	searchErr        error
	queryByTopicErr  error
	queryByEntityErr error
	queryByTagsErr   error
	getTopicsErr     error
	getTimelineErr   error
	getAllEntriesErr error
}

func (s *errKnowledgeStore) GenerateID() (string, error) {
	if s.generateIDErr != nil {
		return "", s.generateIDErr
	}
	return s.inMemoryKnowledgeStore.GenerateID()
}

func (s *errKnowledgeStore) AddEntry(entry models.KnowledgeEntry) (string, error) {
	if s.addEntryErr != nil {
		return "", s.addEntryErr
	}
	return s.inMemoryKnowledgeStore.AddEntry(entry)
}

func (s *errKnowledgeStore) AddEntity(entity models.Entity) error {
	if s.addEntityErr != nil {
		return s.addEntityErr
	}
	return s.inMemoryKnowledgeStore.AddEntity(entity)
}

func (s *errKnowledgeStore) AddTopic(topic models.Topic) error {
	if s.addTopicErr != nil {
		return s.addTopicErr
	}
	return s.inMemoryKnowledgeStore.AddTopic(topic)
}

func (s *errKnowledgeStore) AddTimelineEntry(entry models.TimelineEntry) error {
	if s.addTimelineErr != nil {
		return s.addTimelineErr
	}
	return s.inMemoryKnowledgeStore.AddTimelineEntry(entry)
}

func (s *errKnowledgeStore) Save() error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.inMemoryKnowledgeStore.Save()
}

func (s *errKnowledgeStore) Search(query string) ([]models.KnowledgeEntry, error) {
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return s.inMemoryKnowledgeStore.Search(query)
}

func (s *errKnowledgeStore) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	if s.queryByTopicErr != nil {
		return nil, s.queryByTopicErr
	}
	return s.inMemoryKnowledgeStore.QueryByTopic(topic)
}

func (s *errKnowledgeStore) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	if s.queryByEntityErr != nil {
		return nil, s.queryByEntityErr
	}
	return s.inMemoryKnowledgeStore.QueryByEntity(entity)
}

func (s *errKnowledgeStore) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	if s.queryByTagsErr != nil {
		return nil, s.queryByTagsErr
	}
	return s.inMemoryKnowledgeStore.QueryByTags(tags)
}

func (s *errKnowledgeStore) GetTopics() (*models.TopicGraph, error) {
	if s.getTopicsErr != nil {
		return nil, s.getTopicsErr
	}
	return s.inMemoryKnowledgeStore.GetTopics()
}

func (s *errKnowledgeStore) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	if s.getTimelineErr != nil {
		return nil, s.getTimelineErr
	}
	return s.inMemoryKnowledgeStore.GetTimeline(since)
}

func (s *errKnowledgeStore) GetAllEntries() ([]models.KnowledgeEntry, error) {
	if s.getAllEntriesErr != nil {
		return nil, s.getAllEntriesErr
	}
	return s.inMemoryKnowledgeStore.GetAllEntries()
}

func newErrKnowledgeStore() *errKnowledgeStore {
	return &errKnowledgeStore{
		inMemoryKnowledgeStore: *newInMemoryKnowledgeStore(),
	}
}

// --- Passthrough method tests ---

func TestKnowledgeManager_Search(t *testing.T) {
	t.Run("returns matching entries from store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Summary: "Auth decision"},
			{ID: "K-00002", Summary: "API pattern"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.Search("auth")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// inMemoryKnowledgeStore.Search returns all entries (simplified)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("returns empty for empty store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		mgr := NewKnowledgeManager(store)

		results, err := mgr.Search("anything")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.searchErr = fmt.Errorf("search failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.Search("query")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "search failed") {
			t.Errorf("expected error to contain 'search failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_QueryByTopic(t *testing.T) {
	t.Run("returns entries matching topic", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Topic: "auth", Summary: "Auth decision"},
			{ID: "K-00002", Topic: "database", Summary: "DB pattern"},
			{ID: "K-00003", Topic: "auth", Summary: "Auth gotcha"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByTopic("auth")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results for topic 'auth', got %d", len(results))
		}
	})

	t.Run("returns empty for unknown topic", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Topic: "auth"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByTopic("nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.queryByTopicErr = fmt.Errorf("topic query failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.QueryByTopic("auth")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "topic query failed") {
			t.Errorf("expected error to contain 'topic query failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_QueryByEntity(t *testing.T) {
	t.Run("returns entries matching entity", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Entities: []string{"jwt-lib", "redis"}, Summary: "Auth tokens"},
			{ID: "K-00002", Entities: []string{"postgres"}, Summary: "DB setup"},
			{ID: "K-00003", Entities: []string{"jwt-lib"}, Summary: "Token refresh"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByEntity("jwt-lib")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results for entity 'jwt-lib', got %d", len(results))
		}
	})

	t.Run("returns empty for unknown entity", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Entities: []string{"redis"}},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByEntity("nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.queryByEntityErr = fmt.Errorf("entity query failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.QueryByEntity("jwt-lib")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "entity query failed") {
			t.Errorf("expected error to contain 'entity query failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_QueryByTags(t *testing.T) {
	t.Run("returns entries matching any tag", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Tags: []string{"security", "backend"}, Summary: "Auth"},
			{ID: "K-00002", Tags: []string{"frontend"}, Summary: "UI"},
			{ID: "K-00003", Tags: []string{"backend"}, Summary: "API"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByTags([]string{"security"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result for tag 'security', got %d", len(results))
		}
	})

	t.Run("returns empty for no matching tags", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Tags: []string{"security"}},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.QueryByTags([]string{"nonexistent"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.queryByTagsErr = fmt.Errorf("tags query failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.QueryByTags([]string{"security"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "tags query failed") {
			t.Errorf("expected error to contain 'tags query failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_ListTopics(t *testing.T) {
	t.Run("returns topics from store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.topics["auth"] = models.Topic{Name: "auth", Description: "Authentication"}
		store.topics["db"] = models.Topic{Name: "db", Description: "Database"}
		mgr := NewKnowledgeManager(store)

		graph, err := mgr.ListTopics()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(graph.Topics) != 2 {
			t.Errorf("expected 2 topics, got %d", len(graph.Topics))
		}
		if _, ok := graph.Topics["auth"]; !ok {
			t.Error("expected topic 'auth' in result")
		}
	})

	t.Run("returns empty graph for empty store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		mgr := NewKnowledgeManager(store)

		graph, err := mgr.ListTopics()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(graph.Topics) != 0 {
			t.Errorf("expected 0 topics, got %d", len(graph.Topics))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.getTopicsErr = fmt.Errorf("topics failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.ListTopics()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "topics failed") {
			t.Errorf("expected error to contain 'topics failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_GetTopicEntries(t *testing.T) {
	t.Run("returns entries for topic via QueryByTopic", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.entries = []models.KnowledgeEntry{
			{ID: "K-00001", Topic: "auth", Summary: "JWT decision"},
			{ID: "K-00002", Topic: "db", Summary: "Index strategy"},
			{ID: "K-00003", Topic: "auth", Summary: "OAuth gotcha"},
		}
		mgr := NewKnowledgeManager(store)

		results, err := mgr.GetTopicEntries("auth")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 entries for topic 'auth', got %d", len(results))
		}
	})

	t.Run("returns empty for nonexistent topic", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		mgr := NewKnowledgeManager(store)

		results, err := mgr.GetTopicEntries("nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 entries, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.queryByTopicErr = fmt.Errorf("topic entries failed")
		mgr := NewKnowledgeManager(store)

		_, err := mgr.GetTopicEntries("auth")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "topic entries failed") {
			t.Errorf("expected error to contain 'topic entries failed', got: %v", err)
		}
	})
}

func TestKnowledgeManager_GetTimeline(t *testing.T) {
	t.Run("returns timeline entries from store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		store.timeline = []models.TimelineEntry{
			{Date: "2025-01-14", KnowledgeID: "K-00001", Event: "decision: Use JWT"},
			{Date: "2025-01-15", KnowledgeID: "K-00002", Event: "learning: Go crypto"},
		}
		mgr := NewKnowledgeManager(store)

		since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := mgr.GetTimeline(since)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 timeline entries, got %d", len(results))
		}
	})

	t.Run("returns empty for empty store", func(t *testing.T) {
		store := newInMemoryKnowledgeStore()
		mgr := NewKnowledgeManager(store)

		since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		results, err := mgr.GetTimeline(since)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 entries, got %d", len(results))
		}
	})

	t.Run("propagates store error", func(t *testing.T) {
		store := newErrKnowledgeStore()
		store.getTimelineErr = fmt.Errorf("timeline failed")
		mgr := NewKnowledgeManager(store)

		since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		_, err := mgr.GetTimeline(since)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "timeline failed") {
			t.Errorf("expected error to contain 'timeline failed', got: %v", err)
		}
	})
}

// --- Extended GetRelatedKnowledge tests ---

func TestKnowledgeManager_GetRelatedKnowledge_BranchKeywords(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.entries = []models.KnowledgeEntry{
		{ID: "K-00001", Summary: "User authentication flow"},
		{ID: "K-00002", Summary: "Database migration"},
	}
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:     "TASK-00010",
		Branch: "feat/TASK-00001-add-user-auth",
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The simplified Search returns all entries for any query. Keywords extracted
	// from branch: "feat", "TASK", "00001", "add", "user", "auth". Only keywords
	// with len >= 4 are searched: "feat", "TASK", "00001", "user", "auth".
	// Due to deduplication, we get all entries once.
	if len(results) != 2 {
		t.Errorf("expected 2 results (all entries returned by search, deduplicated), got %d", len(results))
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_ShortKeywordsSkipped(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	// No entries at all - if short keywords are searched, the Search method would
	// still return empty. We verify by confirming no results for a branch with
	// only short keywords.
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:     "TASK-00010",
		Branch: "ab-cd-ef",
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All keywords are shorter than 4 chars, so no search calls made.
	if len(results) != 0 {
		t.Errorf("expected 0 results for short-only keywords, got %d", len(results))
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_RelatedTaskIDs(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.entries = []models.KnowledgeEntry{
		{ID: "K-00001", SourceTask: "TASK-00001", Summary: "Related entry"},
		{ID: "K-00002", SourceTask: "TASK-00002", Summary: "Other entry"},
		{ID: "K-00003", SourceTask: "TASK-00001", Summary: "Another related"},
	}
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:      "TASK-00010",
		Related: []string{"TASK-00001"},
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for related TASK-00001, got %d", len(results))
	}
	for _, r := range results {
		if r.SourceTask != "TASK-00001" {
			t.Errorf("expected SourceTask TASK-00001, got %s", r.SourceTask)
		}
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_Deduplication(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	// Entry K-00001 matches by tag AND by related task ID.
	store.entries = []models.KnowledgeEntry{
		{ID: "K-00001", Tags: []string{"security"}, SourceTask: "TASK-00001", Summary: "Auth decision"},
	}
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:      "TASK-00010",
		Tags:    []string{"security"},
		Related: []string{"TASK-00001"},
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should appear only once despite matching two criteria.
	if len(results) != 1 {
		t.Errorf("expected 1 deduplicated result, got %d", len(results))
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_EmptyTask(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.entries = []models.KnowledgeEntry{
		{ID: "K-00001", Tags: []string{"security"}, Summary: "Auth decision"},
	}
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID: "TASK-00010",
		// No tags, no branch, no related.
	}

	results, err := mgr.GetRelatedKnowledge(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty task, got %d", len(results))
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_TagsError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.queryByTagsErr = fmt.Errorf("tags query failed")
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:   "TASK-00010",
		Tags: []string{"security"},
	}

	_, err := mgr.GetRelatedKnowledge(task)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting related knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_SearchError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.searchErr = fmt.Errorf("search failed")
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:     "TASK-00010",
		Branch: "feat-long-keyword",
	}

	_, err := mgr.GetRelatedKnowledge(task)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting related knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestKnowledgeManager_GetRelatedKnowledge_GetAllEntriesError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.getAllEntriesErr = fmt.Errorf("get all entries failed")
	mgr := NewKnowledgeManager(store)

	task := models.Task{
		ID:      "TASK-00010",
		Related: []string{"TASK-00001"},
	}

	_, err := mgr.GetRelatedKnowledge(task)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting related knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

// --- Extended AssembleKnowledgeSummary tests ---

func TestKnowledgeManager_AssembleKnowledgeSummary_MaxEntriesLimiting(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-10", KnowledgeID: "K-00001", Event: "decision: First", Task: "TASK-00001"},
		{Date: "2025-01-11", KnowledgeID: "K-00002", Event: "learning: Second", Task: "TASK-00002"},
		{Date: "2025-01-12", KnowledgeID: "K-00003", Event: "gotcha: Third", Task: "TASK-00003"},
		{Date: "2025-01-13", KnowledgeID: "K-00004", Event: "decision: Fourth", Task: "TASK-00004"},
		{Date: "2025-01-14", KnowledgeID: "K-00005", Event: "learning: Fifth", Task: "TASK-00005"},
	}
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 2 entries should appear (the most recent ones).
	if !strings.Contains(summary, "Fifth") {
		t.Error("expected summary to contain 'Fifth' (most recent)")
	}
	if !strings.Contains(summary, "Fourth") {
		t.Error("expected summary to contain 'Fourth' (second most recent)")
	}
	if strings.Contains(summary, "First") {
		t.Error("expected summary to NOT contain 'First' (limited by maxEntries=2)")
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_ReverseChronological(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-10", KnowledgeID: "K-00001", Event: "decision: First"},
		{Date: "2025-01-11", KnowledgeID: "K-00002", Event: "learning: Second"},
		{Date: "2025-01-12", KnowledgeID: "K-00003", Event: "gotcha: Third"},
	}
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Most recent should appear first in output.
	thirdIdx := strings.Index(summary, "Third")
	secondIdx := strings.Index(summary, "Second")
	firstIdx := strings.Index(summary, "First")
	if thirdIdx == -1 || secondIdx == -1 || firstIdx == -1 {
		t.Fatalf("expected all entries in summary, got: %s", summary)
	}
	if thirdIdx > secondIdx || secondIdx > firstIdx {
		t.Errorf("expected reverse chronological order (Third before Second before First)")
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_OnlyTopicsNoTimeline(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.topics["auth"] = models.Topic{
		Name: "auth", Description: "Authentication", EntryCount: 2, Tasks: []string{"TASK-00001"},
	}
	// No timeline entries.
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "Knowledge Topics") {
		t.Error("expected summary to contain 'Knowledge Topics' header")
	}
	if strings.Contains(summary, "Recent Knowledge") {
		t.Error("expected summary to NOT contain 'Recent Knowledge' (no timeline)")
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_OnlyTimelineNoTopics(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	// No topics.
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-15", KnowledgeID: "K-00001", Event: "decision: Use JWT", Task: "TASK-00001"},
	}
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(summary, "Knowledge Topics") {
		t.Error("expected summary to NOT contain 'Knowledge Topics' (no topics)")
	}
	if !strings.Contains(summary, "Recent Knowledge") {
		t.Error("expected summary to contain 'Recent Knowledge' header")
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_TaskRefIncluded(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	store.timeline = []models.TimelineEntry{
		{Date: "2025-01-15", KnowledgeID: "K-00001", Event: "decision: Use JWT", Task: "TASK-00001"},
		{Date: "2025-01-16", KnowledgeID: "K-00002", Event: "learning: Go patterns", Task: ""},
	}
	mgr := NewKnowledgeManager(store)

	summary, err := mgr.AssembleKnowledgeSummary(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "(TASK-00001)") {
		t.Error("expected summary to contain task reference '(TASK-00001)'")
	}
	// Entry without a task should not have parenthetical ref.
	lines := strings.Split(summary, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Go patterns") && strings.Contains(line, "(") {
			t.Error("expected no task reference for entry without task")
		}
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_GetTopicsError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.getTopicsErr = fmt.Errorf("topics failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AssembleKnowledgeSummary(10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "assembling knowledge summary") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestKnowledgeManager_AssembleKnowledgeSummary_GetTimelineError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.getTimelineErr = fmt.Errorf("timeline failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AssembleKnowledgeSummary(10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "assembling knowledge summary") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

// --- Extended AddKnowledge tests ---

func TestKnowledgeManager_AddKnowledge_MultipleEntities(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision,
		"architecture",
		"Use microservices",
		"Split monolith into services",
		"TASK-00005",
		models.SourceTaskArchive,
		[]string{"redis", "kafka", "postgres"},
		[]string{"backend"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all three entities were created.
	for _, name := range []string{"redis", "kafka", "postgres"} {
		if _, ok := store.entities[name]; !ok {
			t.Errorf("expected entity %q to be created", name)
		}
	}
}

func TestKnowledgeManager_AddKnowledge_NoTopicSkipsTopic(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeLearning,
		"", // No topic.
		"Go channels",
		"Buffered channels are useful",
		"TASK-00005",
		models.SourceTaskArchive,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.topics) != 0 {
		t.Errorf("expected no topics created when topic is empty, got %d", len(store.topics))
	}
}

func TestKnowledgeManager_AddKnowledge_NoSourceTaskOmitsTaskFromTopic(t *testing.T) {
	store := newInMemoryKnowledgeStore()
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeLearning,
		"general",
		"A general learning",
		"Detail",
		"", // No source task.
		models.SourceManual,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	topic, ok := store.topics["general"]
	if !ok {
		t.Fatal("expected topic 'general' to be created")
	}
	if len(topic.Tasks) != 0 {
		t.Errorf("expected no tasks on topic when sourceTask is empty, got %v", topic.Tasks)
	}
}

func TestKnowledgeManager_AddKnowledge_GenerateIDError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.generateIDErr = fmt.Errorf("id gen failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "auth", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "adding knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestKnowledgeManager_AddKnowledge_AddEntryError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.addEntryErr = fmt.Errorf("entry write failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "auth", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "adding knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestKnowledgeManager_AddKnowledge_AddTopicError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.addTopicErr = fmt.Errorf("topic write failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "auth", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "updating topic") {
		t.Errorf("expected error to mention 'updating topic', got: %v", err)
	}
}

func TestKnowledgeManager_AddKnowledge_AddEntityError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.addEntityErr = fmt.Errorf("entity write failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, []string{"redis"}, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "updating entity") {
		t.Errorf("expected error to mention 'updating entity', got: %v", err)
	}
}

func TestKnowledgeManager_AddKnowledge_AddTimelineError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.addTimelineErr = fmt.Errorf("timeline write failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "updating timeline") {
		t.Errorf("expected error to mention 'updating timeline', got: %v", err)
	}
}

func TestKnowledgeManager_AddKnowledge_SaveError(t *testing.T) {
	store := newErrKnowledgeStore()
	store.saveErr = fmt.Errorf("save failed")
	mgr := NewKnowledgeManager(store)

	_, err := mgr.AddKnowledge(
		models.KnowledgeTypeDecision, "", "Summary", "Detail",
		"TASK-00001", models.SourceTaskArchive, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "saving") {
		t.Errorf("expected error to mention 'saving', got: %v", err)
	}
}

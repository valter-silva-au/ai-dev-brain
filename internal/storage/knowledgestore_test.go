package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestKnowledgeStoreManager_GenerateID(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	id1, err := store.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != "K-00001" {
		t.Errorf("expected K-00001, got %s", id1)
	}

	id2, err := store.GenerateID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id2 != "K-00002" {
		t.Errorf("expected K-00002, got %s", id2)
	}
}

func TestKnowledgeStoreManager_AddAndGetEntry(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	entry := models.KnowledgeEntry{
		ID:         "K-00001",
		Type:       models.KnowledgeTypeDecision,
		Topic:      "auth",
		Summary:    "Use JWT for authentication",
		SourceTask: "TASK-00001",
		SourceType: models.SourceTaskArchive,
		Date:       "2025-01-15",
		Tags:       []string{"security"},
	}

	id, err := store.AddEntry(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "K-00001" {
		t.Errorf("expected K-00001, got %s", id)
	}

	got, err := store.GetEntry("K-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary != "Use JWT for authentication" {
		t.Errorf("unexpected summary: %s", got.Summary)
	}
}

func TestKnowledgeStoreManager_AddEntryDuplicate(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	entry := models.KnowledgeEntry{ID: "K-00001", Summary: "test"}
	if _, err := store.AddEntry(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := store.AddEntry(entry); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestKnowledgeStoreManager_AddEntryEmptyID(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	entry := models.KnowledgeEntry{Summary: "test"}
	if _, err := store.AddEntry(entry); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestKnowledgeStoreManager_QueryByTopic(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Topic: "auth", Summary: "JWT decision"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00002", Topic: "database", Summary: "Use Postgres"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00003", Topic: "auth", Summary: "OAuth flow"})

	results, err := store.QueryByTopic("auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_QueryByTopicCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Topic: "Auth", Summary: "test"})

	results, err := store.QueryByTopic("auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_QueryByEntity(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Entities: []string{"gmail-api"}, Summary: "Rate limits"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00002", Entities: []string{"slack-api"}, Summary: "Webhooks"})

	results, err := store.QueryByEntity("gmail-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_QueryByTags(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Tags: []string{"security", "backend"}, Summary: "Auth"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00002", Tags: []string{"frontend"}, Summary: "UI"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00003", Tags: []string{"security"}, Summary: "Encryption"})

	results, err := store.QueryByTags([]string{"security"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_Search(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Summary: "Use JWT for authentication", Topic: "auth"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00002", Summary: "Use Postgres for storage"})
	store.AddEntry(models.KnowledgeEntry{ID: "K-00003", Summary: "Rate limiting with Redis"})

	results, err := store.Search("jwt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Search by topic.
	results, err = store.Search("auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for topic search, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_SearchEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	results, err := store.Search("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty query, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_TopicOperations(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	topic := models.Topic{
		Name:          "authentication",
		Description:   "Auth decisions",
		RelatedTopics: []string{"security"},
		Tasks:         []string{"TASK-00001"},
	}

	if err := store.AddTopic(topic); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := store.GetTopic("authentication")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Description != "Auth decisions" {
		t.Errorf("unexpected description: %s", got.Description)
	}

	// Merge: add new tasks.
	topic2 := models.Topic{
		Name:  "authentication",
		Tasks: []string{"TASK-00002"},
	}
	if err := store.AddTopic(topic2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ = store.GetTopic("authentication")
	if len(got.Tasks) != 2 {
		t.Errorf("expected 2 tasks after merge, got %d", len(got.Tasks))
	}
}

func TestKnowledgeStoreManager_TopicEmptyName(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	if err := store.AddTopic(models.Topic{}); err == nil {
		t.Fatal("expected error for empty topic name")
	}
}

func TestKnowledgeStoreManager_EntityOperations(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	entity := models.Entity{
		Name:        "gmail-api",
		Type:        models.EntitySystem,
		Description: "Google Gmail API",
		FirstSeen:   "2025-01-15",
		Tasks:       []string{"TASK-00021"},
	}

	if err := store.AddEntity(entity); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := store.GetEntity("gmail-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Description != "Google Gmail API" {
		t.Errorf("unexpected description: %s", got.Description)
	}

	// Merge: add tasks.
	entity2 := models.Entity{
		Name:  "gmail-api",
		Tasks: []string{"TASK-00022"},
	}
	if err := store.AddEntity(entity2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ = store.GetEntity("gmail-api")
	if len(got.Tasks) != 2 {
		t.Errorf("expected 2 tasks after merge, got %d", len(got.Tasks))
	}
}

func TestKnowledgeStoreManager_TimelineOperations(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddTimelineEntry(models.TimelineEntry{
		Date: "2025-01-10", KnowledgeID: "K-00001", Event: "First decision", Task: "TASK-00001",
	})
	store.AddTimelineEntry(models.TimelineEntry{
		Date: "2025-01-20", KnowledgeID: "K-00002", Event: "Second learning", Task: "TASK-00002",
	})

	since := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	results, err := store.GetTimeline(since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result since 2025-01-15, got %d", len(results))
	}
}

func TestKnowledgeStoreManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create and populate store.
	store := NewKnowledgeStoreManager(dir)
	store.AddEntry(models.KnowledgeEntry{
		ID: "K-00001", Type: models.KnowledgeTypeDecision, Topic: "auth",
		Summary: "Use JWT", SourceType: models.SourceTaskArchive, Date: "2025-01-15",
	})
	store.AddTopic(models.Topic{Name: "auth", Description: "Authentication"})
	store.AddEntity(models.Entity{Name: "jwt", Type: models.EntitySystem, FirstSeen: "2025-01-15"})
	store.AddTimelineEntry(models.TimelineEntry{
		Date: "2025-01-15", KnowledgeID: "K-00001", Event: "Decision: Use JWT",
	})

	if err := store.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify files exist.
	for _, file := range []string{"index.yaml", "topics.yaml", "entities.yaml", "timeline.yaml"} {
		path := filepath.Join(dir, "docs", "knowledge", file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", file)
		}
	}

	// Load into a new store instance.
	store2 := NewKnowledgeStoreManager(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}

	entries, _ := store2.GetAllEntries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after load, got %d", len(entries))
	}
	if entries[0].Summary != "Use JWT" {
		t.Errorf("unexpected summary after load: %s", entries[0].Summary)
	}

	topics, _ := store2.GetTopics()
	if len(topics.Topics) != 1 {
		t.Errorf("expected 1 topic after load, got %d", len(topics.Topics))
	}

	entities, _ := store2.GetEntities()
	if len(entities.Entities) != 1 {
		t.Errorf("expected 1 entity after load, got %d", len(entities.Entities))
	}
}

func TestKnowledgeStoreManager_LoadMissingFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	// Should not error on missing files.
	if err := store.Load(); err != nil {
		t.Fatalf("unexpected error loading from empty dir: %v", err)
	}

	entries, _ := store.GetAllEntries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// --- matchesSearch edge cases ---

func TestMatchesSearch_FieldCoverage(t *testing.T) {
	entry := models.KnowledgeEntry{
		ID:       "K-00001",
		Summary:  "JWT authentication overview",
		Detail:   "Detailed explanation of token flow",
		Topic:    "security-architecture",
		Tags:     []string{"backend", "auth-module"},
		Entities: []string{"gmail-api", "oauth-provider"},
	}

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"query matches summary", "jwt", true},
		{"query matches detail", "token flow", true},
		{"query matches topic", "security-architecture", true},
		{"query matches a tag", "auth-module", true},
		{"query matches an entity", "gmail-api", true},
		{"case insensitive summary", "JWT AUTHENTICATION", true},
		{"case insensitive detail", "TOKEN FLOW", true},
		{"case insensitive topic", "SECURITY-ARCHITECTURE", true},
		{"case insensitive tag", "AUTH-MODULE", true},
		{"case insensitive entity", "GMAIL-API", true},
		{"no match returns false", "nonexistent-query", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSearch(entry, strings.ToLower(tt.query))
			if got != tt.want {
				t.Errorf("matchesSearch(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestMatchesSearch_EmptyEntry(t *testing.T) {
	entry := models.KnowledgeEntry{ID: "K-00001"}
	if matchesSearch(entry, "anything") {
		t.Error("empty entry should not match any query")
	}
}

// --- Search via the public interface ---

func TestKnowledgeStoreManager_SearchByDetail(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{
		ID:     "K-00001",
		Detail: "This involves a complex token refresh mechanism",
	})
	store.AddEntry(models.KnowledgeEntry{
		ID:      "K-00002",
		Summary: "Unrelated entry",
	})

	results, err := store.Search("token refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "K-00001" {
		t.Errorf("expected K-00001, got %s", results[0].ID)
	}
}

func TestKnowledgeStoreManager_SearchByTag(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{
		ID:   "K-00001",
		Tags: []string{"performance-tuning"},
	})

	results, err := store.Search("performance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "K-00001" {
		t.Errorf("expected K-00001, got %s", results[0].ID)
	}
}

func TestKnowledgeStoreManager_SearchByEntity(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{
		ID:       "K-00001",
		Entities: []string{"redis-cache"},
	})

	results, err := store.Search("redis")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "K-00001" {
		t.Errorf("expected K-00001, got %s", results[0].ID)
	}
}

func TestKnowledgeStoreManager_SearchEmptyWithEntries(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Summary: "something"})

	results, err := store.Search("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("empty query should return nil, got %d results", len(results))
	}
}

// --- Load/Save persistence round-trip ---

func TestKnowledgeStoreManager_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	// Populate all four data stores.
	store.AddEntry(models.KnowledgeEntry{
		ID: "K-00001", Type: models.KnowledgeTypeLearning, Topic: "testing",
		Summary: "Always use table-driven tests", Detail: "Detailed reasoning here",
		SourceTask: "TASK-00010", SourceType: models.SourceSession, Date: "2025-02-01",
		Entities: []string{"go-test"}, Tags: []string{"testing", "best-practice"},
	})
	store.AddEntry(models.KnowledgeEntry{
		ID: "K-00002", Type: models.KnowledgeTypeGotcha, Topic: "concurrency",
		Summary: "Race condition in handler", Date: "2025-02-02",
	})

	store.AddTopic(models.Topic{
		Name: "testing", Description: "Testing patterns",
		RelatedTopics: []string{"quality"}, Tasks: []string{"TASK-00010"},
	})
	store.AddTopic(models.Topic{
		Name: "concurrency", Description: "Concurrency patterns",
	})

	store.AddEntity(models.Entity{
		Name: "go-test", Type: models.EntitySystem, Description: "Go testing framework",
		FirstSeen: "2025-02-01", Tasks: []string{"TASK-00010"}, Knowledge: []string{"K-00001"},
	})

	store.AddTimelineEntry(models.TimelineEntry{
		Date: "2025-02-01", KnowledgeID: "K-00001", Event: "Learned table-driven tests", Task: "TASK-00010",
	})
	store.AddTimelineEntry(models.TimelineEntry{
		Date: "2025-02-02", KnowledgeID: "K-00002", Event: "Found race condition",
	})

	if err := store.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify all 4 YAML files exist.
	for _, file := range []string{"index.yaml", "topics.yaml", "entities.yaml", "timeline.yaml"} {
		path := filepath.Join(dir, "docs", "knowledge", file)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", file, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected %s to have content", file)
		}
	}

	// Load into a fresh store.
	store2 := NewKnowledgeStoreManager(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}

	// Verify entries.
	entries, _ := store2.GetAllEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after load, got %d", len(entries))
	}
	if entries[0].ID != "K-00001" || entries[0].Summary != "Always use table-driven tests" {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[0].Detail != "Detailed reasoning here" {
		t.Errorf("expected detail to persist, got %q", entries[0].Detail)
	}
	if len(entries[0].Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(entries[0].Tags))
	}
	if len(entries[0].Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(entries[0].Entities))
	}

	// Verify topics.
	topics, _ := store2.GetTopics()
	if len(topics.Topics) != 2 {
		t.Errorf("expected 2 topics after load, got %d", len(topics.Topics))
	}
	testingTopic, err := store2.GetTopic("testing")
	if err != nil {
		t.Fatalf("expected testing topic: %v", err)
	}
	if testingTopic.Description != "Testing patterns" {
		t.Errorf("unexpected topic description: %s", testingTopic.Description)
	}

	// Verify entities.
	entities, _ := store2.GetEntities()
	if len(entities.Entities) != 1 {
		t.Errorf("expected 1 entity after load, got %d", len(entities.Entities))
	}
	goTestEntity, err := store2.GetEntity("go-test")
	if err != nil {
		t.Fatalf("expected go-test entity: %v", err)
	}
	if goTestEntity.Description != "Go testing framework" {
		t.Errorf("unexpected entity description: %s", goTestEntity.Description)
	}
	if len(goTestEntity.Knowledge) != 1 {
		t.Errorf("expected 1 knowledge ref, got %d", len(goTestEntity.Knowledge))
	}

	// Verify timeline.
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	timeline, _ := store2.GetTimeline(since)
	if len(timeline) != 2 {
		t.Errorf("expected 2 timeline entries, got %d", len(timeline))
	}
}

func TestKnowledgeStoreManager_SaveOverwrites(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Summary: "First version"})
	if err := store.Save(); err != nil {
		t.Fatalf("first save error: %v", err)
	}

	// Reload and modify.
	store2 := NewKnowledgeStoreManager(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	store2.AddEntry(models.KnowledgeEntry{ID: "K-00002", Summary: "Second entry"})
	if err := store2.Save(); err != nil {
		t.Fatalf("second save error: %v", err)
	}

	// Load again and verify both entries.
	store3 := NewKnowledgeStoreManager(dir)
	if err := store3.Load(); err != nil {
		t.Fatalf("final load error: %v", err)
	}
	entries, _ := store3.GetAllEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after overwrite, got %d", len(entries))
	}
}

func TestKnowledgeStoreManager_LoadMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	knowledgeDir := filepath.Join(dir, "docs", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write malformed YAML to index.yaml.
	malformed := []byte("{{invalid yaml content [[[")
	if err := os.WriteFile(filepath.Join(knowledgeDir, "index.yaml"), malformed, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewKnowledgeStoreManager(dir)
	err := store.Load()
	if err == nil {
		t.Fatal("expected error loading malformed YAML")
	}
	if !strings.Contains(err.Error(), "loading knowledge index") {
		t.Errorf("expected error to mention 'loading knowledge index', got: %v", err)
	}
}

func TestKnowledgeStoreManager_LoadMalformedTopics(t *testing.T) {
	dir := t.TempDir()
	knowledgeDir := filepath.Join(dir, "docs", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Valid index but malformed topics.
	if err := os.WriteFile(filepath.Join(knowledgeDir, "index.yaml"), []byte("version: '1.0'\nentries: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowledgeDir, "topics.yaml"), []byte("{{bad yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewKnowledgeStoreManager(dir)
	err := store.Load()
	if err == nil {
		t.Fatal("expected error loading malformed topics")
	}
	if !strings.Contains(err.Error(), "loading topics") {
		t.Errorf("expected error to mention 'loading topics', got: %v", err)
	}
}

func TestKnowledgeStoreManager_LoadFromEmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	if err := store.Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All structures should be initialized with defaults.
	entries, _ := store.GetAllEntries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}

	topics, _ := store.GetTopics()
	if topics.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", topics.Version)
	}
	if topics.Topics == nil {
		t.Error("expected non-nil topics map")
	}

	entities, _ := store.GetEntities()
	if entities.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", entities.Version)
	}
	if entities.Entities == nil {
		t.Error("expected non-nil entities map")
	}
}

// --- GetEntry not found ---

func TestKnowledgeStoreManager_GetEntryNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntry(models.KnowledgeEntry{ID: "K-00001", Summary: "exists"})

	got, err := store.GetEntry("K-99999")
	if err == nil {
		t.Fatal("expected error for nonexistent entry")
	}
	if got != nil {
		t.Errorf("expected nil entry, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// --- GetTopic / GetEntity not found ---

func TestKnowledgeStoreManager_GetTopicNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddTopic(models.Topic{Name: "existing", Description: "exists"})

	got, err := store.GetTopic("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent topic")
	}
	if got != nil {
		t.Errorf("expected nil topic, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestKnowledgeStoreManager_GetEntityNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntity(models.Entity{Name: "existing", Type: models.EntitySystem, FirstSeen: "2025-01-01"})

	got, err := store.GetEntity("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent entity")
	}
	if got != nil {
		t.Errorf("expected nil entity, got %+v", got)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// --- AddEntity merge behavior ---

func TestKnowledgeStoreManager_AddEntityMerge(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	// Initial entity with multiple fields.
	initial := models.Entity{
		Name:        "slack-api",
		Type:        models.EntitySystem,
		Description: "Slack API v1",
		Role:        "messaging",
		Channels:    []string{"#general"},
		Tasks:       []string{"TASK-00001"},
		Knowledge:   []string{"K-00001"},
		FirstSeen:   "2025-01-01",
	}
	if err := store.AddEntity(initial); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Merge with new tasks, knowledge, and channels.
	merge := models.Entity{
		Name:        "slack-api",
		Description: "Slack API v2",
		Role:        "communication",
		Channels:    []string{"#general", "#dev"},
		Tasks:       []string{"TASK-00001", "TASK-00002"},
		Knowledge:   []string{"K-00001", "K-00002"},
	}
	if err := store.AddEntity(merge); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := store.GetEntity("slack-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Description and role should be updated.
	if got.Description != "Slack API v2" {
		t.Errorf("expected description 'Slack API v2', got %q", got.Description)
	}
	if got.Role != "communication" {
		t.Errorf("expected role 'communication', got %q", got.Role)
	}

	// Channels should be merged (deduplicated).
	if len(got.Channels) != 2 {
		t.Errorf("expected 2 channels after merge, got %d: %v", len(got.Channels), got.Channels)
	}

	// Tasks should be merged (deduplicated).
	if len(got.Tasks) != 2 {
		t.Errorf("expected 2 tasks after merge, got %d: %v", len(got.Tasks), got.Tasks)
	}

	// Knowledge should be merged (deduplicated).
	if len(got.Knowledge) != 2 {
		t.Errorf("expected 2 knowledge refs after merge, got %d: %v", len(got.Knowledge), got.Knowledge)
	}
}

func TestKnowledgeStoreManager_AddEntityEmptyName(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	err := store.AddEntity(models.Entity{})
	if err == nil {
		t.Fatal("expected error for empty entity name")
	}
	if !strings.Contains(err.Error(), "name must not be empty") {
		t.Errorf("expected 'name must not be empty' in error, got: %v", err)
	}
}

func TestKnowledgeStoreManager_AddEntityNew(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	entity := models.Entity{
		Name:        "new-system",
		Type:        models.EntityProject,
		Description: "A brand new system",
		Role:        "primary",
		Channels:    []string{"#new-channel"},
		Tasks:       []string{"TASK-00050"},
		Knowledge:   []string{"K-00010"},
		FirstSeen:   "2025-03-01",
	}
	if err := store.AddEntity(entity); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := store.GetEntity("new-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Name != "new-system" {
		t.Errorf("expected name 'new-system', got %q", got.Name)
	}
	if got.Type != models.EntityProject {
		t.Errorf("expected type 'project', got %q", got.Type)
	}
	if got.Description != "A brand new system" {
		t.Errorf("expected description 'A brand new system', got %q", got.Description)
	}
	if len(got.Tasks) != 1 || got.Tasks[0] != "TASK-00050" {
		t.Errorf("unexpected tasks: %v", got.Tasks)
	}
	if len(got.Knowledge) != 1 || got.Knowledge[0] != "K-00010" {
		t.Errorf("unexpected knowledge: %v", got.Knowledge)
	}
}

func TestKnowledgeStoreManager_AddEntityMergeEmptyDescription(t *testing.T) {
	dir := t.TempDir()
	store := NewKnowledgeStoreManager(dir)

	store.AddEntity(models.Entity{
		Name:        "test-entity",
		Type:        models.EntitySystem,
		Description: "Original description",
		Role:        "original role",
		FirstSeen:   "2025-01-01",
	})

	// Merge with empty description and role -- should keep originals.
	store.AddEntity(models.Entity{
		Name:  "test-entity",
		Tasks: []string{"TASK-00099"},
	})

	got, _ := store.GetEntity("test-entity")
	if got.Description != "Original description" {
		t.Errorf("expected original description preserved, got %q", got.Description)
	}
	if got.Role != "original role" {
		t.Errorf("expected original role preserved, got %q", got.Role)
	}
	if len(got.Tasks) != 1 {
		t.Errorf("expected 1 task after merge, got %d", len(got.Tasks))
	}
}

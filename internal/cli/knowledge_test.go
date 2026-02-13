package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// fakeKnowledgeMgrForCLI implements core.KnowledgeManager for testing CLI knowledge commands.
type fakeKnowledgeMgrForCLI struct {
	addKnowledgeFn         func(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error)
	searchFn               func(query string) ([]models.KnowledgeEntry, error)
	queryByTopicFn         func(topic string) ([]models.KnowledgeEntry, error)
	queryByEntityFn        func(entity string) ([]models.KnowledgeEntry, error)
	queryByTagsFn          func(tags []string) ([]models.KnowledgeEntry, error)
	listTopicsFn           func() (*models.TopicGraph, error)
	getTimelineFn          func(since time.Time) ([]models.TimelineEntry, error)
	getRelatedKnowledgeFn  func(task models.Task) ([]models.KnowledgeEntry, error)
	assembleKnowledgeSumFn func(maxEntries int) (string, error)
	ingestFromExtractionFn func(knowledge *models.ExtractedKnowledge) ([]string, error)
	getTopicEntriesFn      func(topic string) ([]models.KnowledgeEntry, error)
}

func (f *fakeKnowledgeMgrForCLI) AddKnowledge(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
	if f.addKnowledgeFn != nil {
		return f.addKnowledgeFn(entryType, topic, summary, detail, sourceTask, sourceType, entities, tags)
	}
	return "KE-001", nil
}

func (f *fakeKnowledgeMgrForCLI) IngestFromExtraction(knowledge *models.ExtractedKnowledge) ([]string, error) {
	if f.ingestFromExtractionFn != nil {
		return f.ingestFromExtractionFn(knowledge)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) Search(query string) ([]models.KnowledgeEntry, error) {
	if f.searchFn != nil {
		return f.searchFn(query)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	if f.queryByTopicFn != nil {
		return f.queryByTopicFn(topic)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	if f.queryByEntityFn != nil {
		return f.queryByEntityFn(entity)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	if f.queryByTagsFn != nil {
		return f.queryByTagsFn(tags)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) ListTopics() (*models.TopicGraph, error) {
	if f.listTopicsFn != nil {
		return f.listTopicsFn()
	}
	return &models.TopicGraph{Topics: map[string]models.Topic{}}, nil
}

func (f *fakeKnowledgeMgrForCLI) GetTopicEntries(topic string) ([]models.KnowledgeEntry, error) {
	if f.getTopicEntriesFn != nil {
		return f.getTopicEntriesFn(topic)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	if f.getTimelineFn != nil {
		return f.getTimelineFn(since)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) GetRelatedKnowledge(task models.Task) ([]models.KnowledgeEntry, error) {
	if f.getRelatedKnowledgeFn != nil {
		return f.getRelatedKnowledgeFn(task)
	}
	return nil, nil
}

func (f *fakeKnowledgeMgrForCLI) AssembleKnowledgeSummary(maxEntries int) (string, error) {
	if f.assembleKnowledgeSumFn != nil {
		return f.assembleKnowledgeSumFn(maxEntries)
	}
	return "", nil
}

// --- truncate tests ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "shorter than maxLen",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exactly maxLen",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "longer than maxLen",
			input:  "hello world this is long",
			maxLen: 10,
			want:   "hello w...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "maxLen of 3 edge case",
			input:  "abcdef",
			maxLen: 3,
			want:   "...",
		},
		{
			name:   "maxLen of 4",
			input:  "abcdef",
			maxLen: 4,
			want:   "a...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// --- runQuery tests ---

func TestRunQuery(t *testing.T) {
	sampleEntries := []models.KnowledgeEntry{
		{ID: "KE-001", Type: models.KnowledgeTypeLearning, Summary: "sample entry"},
	}

	t.Run("queryType topic calls QueryByTopic", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		var capturedTopic string
		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByTopicFn: func(topic string) ([]models.KnowledgeEntry, error) {
				capturedTopic = topic
				return sampleEntries, nil
			},
		}

		entries, err := runQuery("authentication", "topic")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedTopic != "authentication" {
			t.Errorf("expected topic %q, got %q", "authentication", capturedTopic)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("queryType entity calls QueryByEntity", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		var capturedEntity string
		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByEntityFn: func(entity string) ([]models.KnowledgeEntry, error) {
				capturedEntity = entity
				return sampleEntries, nil
			},
		}

		entries, err := runQuery("alice", "entity")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedEntity != "alice" {
			t.Errorf("expected entity %q, got %q", "alice", capturedEntity)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("queryType tag calls QueryByTags", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		var capturedTags []string
		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByTagsFn: func(tags []string) ([]models.KnowledgeEntry, error) {
				capturedTags = tags
				return sampleEntries, nil
			},
		}

		entries, err := runQuery("security", "tag")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(capturedTags) != 1 || capturedTags[0] != "security" {
			t.Errorf("expected tags [security], got %v", capturedTags)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("empty queryType calls Search", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		var capturedQuery string
		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			searchFn: func(query string) ([]models.KnowledgeEntry, error) {
				capturedQuery = query
				return sampleEntries, nil
			},
		}

		entries, err := runQuery("jwt", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedQuery != "jwt" {
			t.Errorf("expected query %q, got %q", "jwt", capturedQuery)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("unknown queryType defaults to Search", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		var searchCalled bool
		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			searchFn: func(query string) ([]models.KnowledgeEntry, error) {
				searchCalled = true
				return nil, nil
			},
		}

		_, err := runQuery("test", "unknown-type")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !searchCalled {
			t.Error("expected Search to be called for unknown query type")
		}
	})

	t.Run("error propagation from Search", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			searchFn: func(query string) ([]models.KnowledgeEntry, error) {
				return nil, fmt.Errorf("search failed")
			},
		}

		_, err := runQuery("test", "")
		if err == nil {
			t.Fatal("expected error from Search")
		}
		if !strings.Contains(err.Error(), "search failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("error propagation from QueryByTopic", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByTopicFn: func(topic string) ([]models.KnowledgeEntry, error) {
				return nil, fmt.Errorf("topic query failed")
			},
		}

		_, err := runQuery("auth", "topic")
		if err == nil {
			t.Fatal("expected error from QueryByTopic")
		}
		if !strings.Contains(err.Error(), "topic query failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("error propagation from QueryByEntity", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByEntityFn: func(entity string) ([]models.KnowledgeEntry, error) {
				return nil, fmt.Errorf("entity query failed")
			},
		}

		_, err := runQuery("alice", "entity")
		if err == nil {
			t.Fatal("expected error from QueryByEntity")
		}
		if !strings.Contains(err.Error(), "entity query failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("error propagation from QueryByTags", func(t *testing.T) {
		origMgr := KnowledgeMgr
		defer func() { KnowledgeMgr = origMgr }()

		KnowledgeMgr = &fakeKnowledgeMgrForCLI{
			queryByTagsFn: func(tags []string) ([]models.KnowledgeEntry, error) {
				return nil, fmt.Errorf("tags query failed")
			},
		}

		_, err := runQuery("security", "tag")
		if err == nil {
			t.Fatal("expected error from QueryByTags")
		}
		if !strings.Contains(err.Error(), "tags query failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// --- knowledgeQueryCmd tests ---

func TestKnowledgeQueryCmd_Registration(t *testing.T) {
	subcommands := knowledgeCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "query" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'query' subcommand to be registered under 'knowledge'")
	}
}

func TestKnowledgeQueryCmd_NilKnowledgeMgr(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()
	KnowledgeMgr = nil

	err := knowledgeQueryCmd.RunE(knowledgeQueryCmd, []string{"test"})
	if err == nil {
		t.Fatal("expected error when KnowledgeMgr is nil")
	}
	if !strings.Contains(err.Error(), "knowledge manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKnowledgeQueryCmd_WithResults(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		searchFn: func(query string) ([]models.KnowledgeEntry, error) {
			return []models.KnowledgeEntry{
				{
					ID:         "KE-001",
					Type:       models.KnowledgeTypeLearning,
					Summary:    "Use RS256 for JWT",
					Topic:      "authentication",
					SourceTask: "TASK-00010",
					SourceType: models.SourceTaskArchive,
					Tags:       []string{"security", "jwt"},
					Entities:   []string{"auth-service"},
				},
			}, nil
		},
	}

	// Reset the --type flag to empty for this test.
	knowledgeQueryCmd.Flags().Set("type", "")

	err := knowledgeQueryCmd.RunE(knowledgeQueryCmd, []string{"jwt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeQueryCmd_NoResults(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		searchFn: func(query string) ([]models.KnowledgeEntry, error) {
			return nil, nil
		},
	}

	knowledgeQueryCmd.Flags().Set("type", "")

	err := knowledgeQueryCmd.RunE(knowledgeQueryCmd, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeQueryCmd_WithTypeFlag(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	var topicCalled bool
	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		queryByTopicFn: func(topic string) ([]models.KnowledgeEntry, error) {
			topicCalled = true
			return []models.KnowledgeEntry{
				{ID: "KE-001", Type: models.KnowledgeTypeDecision, Summary: "decision entry"},
			}, nil
		},
	}

	knowledgeQueryCmd.Flags().Set("type", "topic")

	err := knowledgeQueryCmd.RunE(knowledgeQueryCmd, []string{"auth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !topicCalled {
		t.Error("expected QueryByTopic to be called when --type=topic")
	}

	// Clean up the flag.
	knowledgeQueryCmd.Flags().Set("type", "")
}

// --- knowledgeAddCmd tests ---

func TestKnowledgeAddCmd_Registration(t *testing.T) {
	subcommands := knowledgeCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "add" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'add' subcommand to be registered under 'knowledge'")
	}
}

func TestKnowledgeAddCmd_NilKnowledgeMgr(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()
	KnowledgeMgr = nil

	err := knowledgeAddCmd.RunE(knowledgeAddCmd, []string{"some summary"})
	if err == nil {
		t.Fatal("expected error when KnowledgeMgr is nil")
	}
	if !strings.Contains(err.Error(), "knowledge manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKnowledgeAddCmd_MinimalFlags(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	var capturedSummary string
	var capturedType models.KnowledgeEntryType
	var capturedSource models.KnowledgeSourceType
	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		addKnowledgeFn: func(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
			capturedSummary = summary
			capturedType = entryType
			capturedSource = sourceType
			return "KE-042", nil
		},
	}

	// Reset flags to defaults.
	knowledgeAddCmd.Flags().Set("type", "learning")
	knowledgeAddCmd.Flags().Set("topic", "")
	knowledgeAddCmd.Flags().Set("detail", "")
	knowledgeAddCmd.Flags().Set("tags", "")
	knowledgeAddCmd.Flags().Set("entities", "")
	knowledgeAddCmd.Flags().Set("task", "")

	err := knowledgeAddCmd.RunE(knowledgeAddCmd, []string{"learned something new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSummary != "learned something new" {
		t.Errorf("expected summary %q, got %q", "learned something new", capturedSummary)
	}
	if capturedType != "learning" {
		t.Errorf("expected type %q, got %q", "learning", capturedType)
	}
	if capturedSource != models.SourceManual {
		t.Errorf("expected source %q, got %q", models.SourceManual, capturedSource)
	}
}

func TestKnowledgeAddCmd_AllFlags(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	var capturedType models.KnowledgeEntryType
	var capturedTopic, capturedDetail, capturedTask string
	var capturedEntities, capturedTags []string
	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		addKnowledgeFn: func(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
			capturedType = entryType
			capturedTopic = topic
			capturedDetail = detail
			capturedTask = sourceTask
			capturedEntities = entities
			capturedTags = tags
			return "KE-100", nil
		},
	}

	knowledgeAddCmd.Flags().Set("type", "decision")
	knowledgeAddCmd.Flags().Set("topic", "authentication")
	knowledgeAddCmd.Flags().Set("detail", "We chose RS256 for token signing")
	knowledgeAddCmd.Flags().Set("tags", "security, jwt, auth")
	knowledgeAddCmd.Flags().Set("entities", "auth-service, gateway")
	knowledgeAddCmd.Flags().Set("task", "TASK-00042")

	err := knowledgeAddCmd.RunE(knowledgeAddCmd, []string{"Use RS256 for JWT tokens"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedType != "decision" {
		t.Errorf("expected type %q, got %q", "decision", capturedType)
	}
	if capturedTopic != "authentication" {
		t.Errorf("expected topic %q, got %q", "authentication", capturedTopic)
	}
	if capturedDetail != "We chose RS256 for token signing" {
		t.Errorf("expected detail %q, got %q", "We chose RS256 for token signing", capturedDetail)
	}
	if capturedTask != "TASK-00042" {
		t.Errorf("expected task %q, got %q", "TASK-00042", capturedTask)
	}

	// Verify comma-separated tags are trimmed.
	expectedTags := []string{"security", "jwt", "auth"}
	if len(capturedTags) != len(expectedTags) {
		t.Fatalf("expected %d tags, got %d: %v", len(expectedTags), len(capturedTags), capturedTags)
	}
	for i, tag := range expectedTags {
		if capturedTags[i] != tag {
			t.Errorf("tag[%d] = %q, want %q", i, capturedTags[i], tag)
		}
	}

	// Verify comma-separated entities are trimmed.
	expectedEntities := []string{"auth-service", "gateway"}
	if len(capturedEntities) != len(expectedEntities) {
		t.Fatalf("expected %d entities, got %d: %v", len(expectedEntities), len(capturedEntities), capturedEntities)
	}
	for i, entity := range expectedEntities {
		if capturedEntities[i] != entity {
			t.Errorf("entity[%d] = %q, want %q", i, capturedEntities[i], entity)
		}
	}

	// Clean up flags.
	knowledgeAddCmd.Flags().Set("type", "learning")
	knowledgeAddCmd.Flags().Set("topic", "")
	knowledgeAddCmd.Flags().Set("detail", "")
	knowledgeAddCmd.Flags().Set("tags", "")
	knowledgeAddCmd.Flags().Set("entities", "")
	knowledgeAddCmd.Flags().Set("task", "")
}

func TestKnowledgeAddCmd_ErrorFromAddKnowledge(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		addKnowledgeFn: func(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
			return "", fmt.Errorf("store write failed")
		},
	}

	knowledgeAddCmd.Flags().Set("type", "learning")
	knowledgeAddCmd.Flags().Set("topic", "")
	knowledgeAddCmd.Flags().Set("detail", "")
	knowledgeAddCmd.Flags().Set("tags", "")
	knowledgeAddCmd.Flags().Set("entities", "")
	knowledgeAddCmd.Flags().Set("task", "")

	err := knowledgeAddCmd.RunE(knowledgeAddCmd, []string{"some entry"})
	if err == nil {
		t.Fatal("expected error from AddKnowledge")
	}
	if !strings.Contains(err.Error(), "adding knowledge") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "store write failed") {
		t.Errorf("expected original error, got: %v", err)
	}
}

func TestKnowledgeAddCmd_EmptyTagsAndEntities(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	var capturedTags, capturedEntities []string
	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		addKnowledgeFn: func(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
			capturedTags = tags
			capturedEntities = entities
			return "KE-001", nil
		},
	}

	knowledgeAddCmd.Flags().Set("type", "learning")
	knowledgeAddCmd.Flags().Set("topic", "")
	knowledgeAddCmd.Flags().Set("detail", "")
	knowledgeAddCmd.Flags().Set("tags", "")
	knowledgeAddCmd.Flags().Set("entities", "")
	knowledgeAddCmd.Flags().Set("task", "")

	err := knowledgeAddCmd.RunE(knowledgeAddCmd, []string{"minimal entry"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTags != nil {
		t.Errorf("expected nil tags for empty input, got %v", capturedTags)
	}
	if capturedEntities != nil {
		t.Errorf("expected nil entities for empty input, got %v", capturedEntities)
	}
}

// --- knowledgeTopicsCmd tests ---

func TestKnowledgeTopicsCmd_Registration(t *testing.T) {
	subcommands := knowledgeCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "topics" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'topics' subcommand to be registered under 'knowledge'")
	}
}

func TestKnowledgeTopicsCmd_NilKnowledgeMgr(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()
	KnowledgeMgr = nil

	err := knowledgeTopicsCmd.RunE(knowledgeTopicsCmd, []string{})
	if err == nil {
		t.Fatal("expected error when KnowledgeMgr is nil")
	}
	if !strings.Contains(err.Error(), "knowledge manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKnowledgeTopicsCmd_NoTopics(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		listTopicsFn: func() (*models.TopicGraph, error) {
			return &models.TopicGraph{Topics: map[string]models.Topic{}}, nil
		},
	}

	err := knowledgeTopicsCmd.RunE(knowledgeTopicsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeTopicsCmd_MultipleTopics(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		listTopicsFn: func() (*models.TopicGraph, error) {
			return &models.TopicGraph{
				Topics: map[string]models.Topic{
					"auth": {
						Name:        "auth",
						Description: "Authentication and authorization patterns",
						EntryCount:  5,
						Tasks:       []string{"TASK-00010", "TASK-00020"},
					},
					"testing": {
						Name:        "testing",
						Description: "Testing strategies and best practices for the project",
						EntryCount:  3,
						Tasks:       []string{"TASK-00015"},
					},
				},
			}, nil
		},
	}

	err := knowledgeTopicsCmd.RunE(knowledgeTopicsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKnowledgeTopicsCmd_DescriptionTruncation(t *testing.T) {
	// The topics command truncates descriptions at 38 characters.
	longDesc := "This is a very long description that definitely exceeds thirty-eight characters"
	truncated := truncate(longDesc, 38)
	if len(truncated) > 38 {
		t.Errorf("truncated description length %d exceeds 38", len(truncated))
	}
	if !strings.HasSuffix(truncated, "...") {
		t.Errorf("truncated description should end with ..., got %q", truncated)
	}
}

func TestKnowledgeTopicsCmd_ListTopicsError(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		listTopicsFn: func() (*models.TopicGraph, error) {
			return nil, fmt.Errorf("file not found")
		},
	}

	err := knowledgeTopicsCmd.RunE(knowledgeTopicsCmd, []string{})
	if err == nil {
		t.Fatal("expected error from ListTopics")
	}
	if !strings.Contains(err.Error(), "listing topics") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("expected original error, got: %v", err)
	}
}

// --- knowledgeTimelineCmd tests ---

func TestKnowledgeTimelineCmd_Registration(t *testing.T) {
	subcommands := knowledgeCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "timeline" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'timeline' subcommand to be registered under 'knowledge'")
	}
}

func TestKnowledgeTimelineCmd_NilKnowledgeMgr(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()
	KnowledgeMgr = nil

	err := knowledgeTimelineCmd.RunE(knowledgeTimelineCmd, []string{})
	if err == nil {
		t.Fatal("expected error when KnowledgeMgr is nil")
	}
	if !strings.Contains(err.Error(), "knowledge manager not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKnowledgeTimelineCmd_WithEntries(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		getTimelineFn: func(since time.Time) ([]models.TimelineEntry, error) {
			return []models.TimelineEntry{
				{
					Date:  "2025-01-15",
					Event: "learning: Use RS256 for JWT",
					Task:  "TASK-00010",
				},
				{
					Date:  "2025-01-16",
					Event: "decision: Switch to PostgreSQL",
					Task:  "",
				},
			}, nil
		},
	}

	// Set the --since flag.
	knowledgeTimelineCmd.Flags().Set("since", "7d")

	err := knowledgeTimelineCmd.RunE(knowledgeTimelineCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reset flag.
	knowledgeTimelineCmd.Flags().Set("since", "30d")
}

func TestKnowledgeTimelineCmd_NoEntries(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		getTimelineFn: func(since time.Time) ([]models.TimelineEntry, error) {
			return nil, nil
		},
	}

	knowledgeTimelineCmd.Flags().Set("since", "7d")

	err := knowledgeTimelineCmd.RunE(knowledgeTimelineCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reset flag.
	knowledgeTimelineCmd.Flags().Set("since", "30d")
}

func TestKnowledgeTimelineCmd_GetTimelineError(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{
		getTimelineFn: func(since time.Time) ([]models.TimelineEntry, error) {
			return nil, fmt.Errorf("timeline read error")
		},
	}

	knowledgeTimelineCmd.Flags().Set("since", "7d")

	err := knowledgeTimelineCmd.RunE(knowledgeTimelineCmd, []string{})
	if err == nil {
		t.Fatal("expected error from GetTimeline")
	}
	if !strings.Contains(err.Error(), "getting timeline") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "timeline read error") {
		t.Errorf("expected original error, got: %v", err)
	}

	// Reset flag.
	knowledgeTimelineCmd.Flags().Set("since", "30d")
}

func TestKnowledgeTimelineCmd_InvalidSinceFlag(t *testing.T) {
	origMgr := KnowledgeMgr
	defer func() { KnowledgeMgr = origMgr }()

	KnowledgeMgr = &fakeKnowledgeMgrForCLI{}

	knowledgeTimelineCmd.Flags().Set("since", "invalid")

	err := knowledgeTimelineCmd.RunE(knowledgeTimelineCmd, []string{})
	if err == nil {
		t.Fatal("expected error for invalid --since flag")
	}

	// Reset flag.
	knowledgeTimelineCmd.Flags().Set("since", "30d")
}

// --- printEntries tests ---

func TestPrintEntries_VariousEntries(t *testing.T) {
	// printEntries only prints to stdout, so we verify it does not panic.
	entries := []models.KnowledgeEntry{
		{
			ID:         "KE-001",
			Type:       models.KnowledgeTypeLearning,
			Summary:    "Use RS256 for JWT",
			Topic:      "auth",
			SourceTask: "TASK-00010",
			SourceType: models.SourceTaskArchive,
			Tags:       []string{"security", "jwt"},
			Entities:   []string{"auth-service"},
		},
		{
			ID:      "KE-002",
			Type:    models.KnowledgeTypePattern,
			Summary: "Table-driven tests for Go",
			// No topic, source, tags, or entities.
		},
		{
			ID:      "KE-003",
			Type:    models.KnowledgeTypeGotcha,
			Summary: "Nil pointer on empty slice",
			Tags:    []string{"golang"},
			// No entities, topic, or source.
		},
	}

	// This should not panic.
	printEntries(entries)
}

func TestPrintEntries_EmptySlice(t *testing.T) {
	// Verify no panic on empty slice.
	printEntries(nil)
	printEntries([]models.KnowledgeEntry{})
}

// --- knowledge parent command registration ---

func TestKnowledgeCmd_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "knowledge" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'knowledge' command to be registered on root")
	}
}

func TestKnowledgeCmd_HasSubcommands(t *testing.T) {
	subcommands := knowledgeCmd.Commands()
	expected := map[string]bool{
		"query":    false,
		"add":      false,
		"topics":   false,
		"timeline": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expected[cmd.Name()]; ok {
			expected[cmd.Name()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected subcommand %q not found under 'knowledge'", name)
		}
	}
}

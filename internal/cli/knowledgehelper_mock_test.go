package cli

import (
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// mockKnowledgeMgrForHelper implements core.KnowledgeManager for testing
// appendKnowledgeToTaskContext. Only AssembleKnowledgeSummary is meaningful.
type mockKnowledgeMgrForHelper struct {
	summary string
	err     error
}

func (m *mockKnowledgeMgrForHelper) AddKnowledge(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
	return "", nil
}

func (m *mockKnowledgeMgrForHelper) IngestFromExtraction(knowledge *models.ExtractedKnowledge) ([]string, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) Search(query string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) ListTopics() (*models.TopicGraph, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) GetTopicEntries(topic string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) GetRelatedKnowledge(task models.Task) ([]models.KnowledgeEntry, error) {
	return nil, nil
}

func (m *mockKnowledgeMgrForHelper) AssembleKnowledgeSummary(maxEntries int) (string, error) {
	return m.summary, m.err
}

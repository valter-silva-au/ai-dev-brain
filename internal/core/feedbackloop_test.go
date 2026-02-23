package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// fakeKnowledgeManager implements KnowledgeManager for testing.
type fakeKnowledgeManager struct {
	added []fakeKnowledgeCall
	idSeq int
}

type fakeKnowledgeCall struct {
	entryType  models.KnowledgeEntryType
	topic      string
	summary    string
	detail     string
	sourceTask string
	sourceType models.KnowledgeSourceType
	entities   []string
	tags       []string
}

func (km *fakeKnowledgeManager) AddKnowledge(entryType models.KnowledgeEntryType, topic, summary, detail string, sourceTask string, sourceType models.KnowledgeSourceType, entities, tags []string) (string, error) {
	km.idSeq++
	km.added = append(km.added, fakeKnowledgeCall{
		entryType:  entryType,
		topic:      topic,
		summary:    summary,
		detail:     detail,
		sourceTask: sourceTask,
		sourceType: sourceType,
		entities:   entities,
		tags:       tags,
	})
	return fmt.Sprintf("K-%03d", km.idSeq), nil
}

func (km *fakeKnowledgeManager) IngestFromExtraction(_ *models.ExtractedKnowledge) ([]string, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) Search(_ string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) QueryByTopic(_ string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) QueryByEntity(_ string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) QueryByTags(_ []string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) ListTopics() (*models.TopicGraph, error) { return nil, nil }
func (km *fakeKnowledgeManager) GetTopicEntries(_ string) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) GetTimeline(_ time.Time) ([]models.TimelineEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) GetRelatedKnowledge(_ models.Task) ([]models.KnowledgeEntry, error) {
	return nil, nil
}
func (km *fakeKnowledgeManager) AssembleKnowledgeSummary(_ int) (string, error) { return "", nil }

// fakeEventLogger records logged events.
type fakeEventLogger struct {
	events []fakeEvent
}

type fakeEvent struct {
	eventType string
	data      map[string]any
}

func (l *fakeEventLogger) LogEvent(eventType string, data map[string]any) error {
	l.events = append(l.events, fakeEvent{eventType: eventType, data: data})
	return nil
}

// --- Tests ---

func TestFeedbackLoop_RunEmpty(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name:  "test",
		typ:   models.ChannelEmail,
		items: nil,
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")
	result, err := orch.Run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ItemsFetched != 0 {
		t.Errorf("expected 0 items fetched, got %d", result.ItemsFetched)
	}
	if result.ItemsProcessed != 0 {
		t.Errorf("expected 0 items processed, got %d", result.ItemsProcessed)
	}
	if result.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", result.Skipped)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestFeedbackLoop_ClassifyTaskReference(t *testing.T) {
	tests := []struct {
		name       string
		item       models.ChannelItem
		wantTask   string
		wantAction string
	}{
		{
			name: "task reference in subject",
			item: models.ChannelItem{
				ID:      "item-1",
				Channel: models.ChannelEmail,
				Subject: "Update on TASK-00042 auth work",
				Content: "Here is the progress update.",
			},
			wantTask:   "TASK-00042",
			wantAction: "route_to_task",
		},
		{
			name: "task reference in content",
			item: models.ChannelItem{
				ID:      "item-2",
				Channel: models.ChannelSlack,
				Subject: "Quick question",
				Content: "Regarding TASK-00015, when can we deploy?",
			},
			wantTask:   "TASK-00015",
			wantAction: "route_to_task",
		},
		{
			name: "task reference in RelatedTask field",
			item: models.ChannelItem{
				ID:          "item-3",
				Channel:     models.ChannelTicket,
				Subject:     "No pattern here",
				Content:     "No pattern here either",
				RelatedTask: "TASK-00099",
			},
			wantTask:   "TASK-00099",
			wantAction: "route_to_task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewChannelRegistry()
			km := &fakeKnowledgeManager{}
			orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

			result, err := orch.ProcessItem(tc.item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.RelatedTask != tc.wantTask {
				t.Errorf("expected related task %q, got %q", tc.wantTask, result.RelatedTask)
			}
			if result.Action != tc.wantAction {
				t.Errorf("expected action %q, got %q", tc.wantAction, result.Action)
			}
		})
	}
}

func TestFeedbackLoop_ClassifyHighPriority(t *testing.T) {
	reg := NewChannelRegistry()
	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")

	item := models.ChannelItem{
		ID:       "item-hp",
		Channel:  models.ChannelEmail,
		Subject:  "Urgent production issue",
		Content:  "Something is broken",
		Priority: models.ChannelPriorityHigh,
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != "needs_attention" {
		t.Errorf("expected action %q, got %q", "needs_attention", result.Action)
	}
	if result.RelatedTask != "" {
		t.Errorf("expected no related task, got %q", result.RelatedTask)
	}
}

func TestFeedbackLoop_KnowledgeIngestion(t *testing.T) {
	reg := NewChannelRegistry()
	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	item := models.ChannelItem{
		ID:      "item-k",
		Channel: models.ChannelSlack,
		Subject: "Auth discussion for TASK-00042",
		Content: "We decided to use JWT tokens for the new API authentication layer.",
		Tags:    []string{"auth", "api"},
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != "route_to_task" {
		t.Errorf("expected action %q, got %q", "route_to_task", result.Action)
	}
	if len(result.KnowledgeIDs) != 1 {
		t.Fatalf("expected 1 knowledge ID, got %d", len(result.KnowledgeIDs))
	}
	if result.KnowledgeIDs[0] != "K-001" {
		t.Errorf("expected knowledge ID %q, got %q", "K-001", result.KnowledgeIDs[0])
	}

	// Verify the knowledge call.
	if len(km.added) != 1 {
		t.Fatalf("expected 1 knowledge call, got %d", len(km.added))
	}
	call := km.added[0]
	if call.sourceType != models.SourceChannel {
		t.Errorf("expected source type %q, got %q", models.SourceChannel, call.sourceType)
	}
	if call.sourceTask != "TASK-00042" {
		t.Errorf("expected source task %q, got %q", "TASK-00042", call.sourceTask)
	}
	if call.topic != "auth" {
		t.Errorf("expected topic %q, got %q", "auth", call.topic)
	}
}

func TestFeedbackLoop_CreateKnowledgeFromTags(t *testing.T) {
	reg := NewChannelRegistry()
	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	item := models.ChannelItem{
		ID:      "item-tags",
		Channel: models.ChannelDocument,
		Subject: "Architecture discussion",
		Content: "Short",
		Tags:    []string{"architecture"},
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != "create_knowledge" {
		t.Errorf("expected action %q, got %q", "create_knowledge", result.Action)
	}
	if len(result.KnowledgeIDs) != 1 {
		t.Fatalf("expected 1 knowledge ID, got %d", len(result.KnowledgeIDs))
	}
}

func TestFeedbackLoop_SkipItem(t *testing.T) {
	reg := NewChannelRegistry()
	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")

	item := models.ChannelItem{
		ID:      "item-skip",
		Channel: models.ChannelEmail,
		Subject: "Hello",
		Content: "Hi there",
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != "skip" {
		t.Errorf("expected action %q, got %q", "skip", result.Action)
	}
	if len(result.KnowledgeIDs) != 0 {
		t.Errorf("expected no knowledge IDs, got %d", len(result.KnowledgeIDs))
	}
	if result.Output != nil {
		t.Errorf("expected no output, got %+v", result.Output)
	}
}

func TestFeedbackLoop_NilDependencies(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name: "email",
		typ:  models.ChannelEmail,
		items: []models.ChannelItem{
			{
				ID:      "item-nil",
				Channel: models.ChannelEmail,
				Source:  "email",
				Subject: "Re: TASK-00001 progress",
				Content: "Good work on TASK-00001.",
			},
		},
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	// nil KnowledgeManager, nil BacklogStore, nil EventLogger
	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")

	// ProcessItem should work without knowledge manager.
	result, err := orch.ProcessItem(adapter.items[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != "route_to_task" {
		t.Errorf("expected action %q, got %q", "route_to_task", result.Action)
	}
	// No knowledge IDs since KnowledgeManager is nil.
	if len(result.KnowledgeIDs) != 0 {
		t.Errorf("expected 0 knowledge IDs with nil manager, got %d", len(result.KnowledgeIDs))
	}

	// Run should also succeed.
	loopResult, err := orch.Run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error on Run: %v", err)
	}
	if loopResult.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched, got %d", loopResult.ItemsFetched)
	}
}

func TestFeedbackLoop_RunWithChannelFilter(t *testing.T) {
	reg := NewChannelRegistry()
	emailAdapter := &fakeChannelAdapter{
		name: "email",
		typ:  models.ChannelEmail,
		items: []models.ChannelItem{
			{ID: "e1", Channel: models.ChannelEmail, Subject: "Hello", Content: "Short"},
		},
	}
	slackAdapter := &fakeChannelAdapter{
		name: "slack",
		typ:  models.ChannelSlack,
		items: []models.ChannelItem{
			{ID: "s1", Channel: models.ChannelSlack, Subject: "Hey", Content: "Brief"},
		},
	}
	if err := reg.Register(emailAdapter); err != nil {
		t.Fatalf("registering email adapter: %v", err)
	}
	if err := reg.Register(slackAdapter); err != nil {
		t.Fatalf("registering slack adapter: %v", err)
	}

	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")

	result, err := orch.Run(RunOptions{ChannelFilter: "email"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ItemsFetched != 1 {
		t.Errorf("expected 1 item fetched (email only), got %d", result.ItemsFetched)
	}
}

func TestFeedbackLoop_DryRun(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name: "email",
		typ:  models.ChannelEmail,
		items: []models.ChannelItem{
			{
				ID:      "dr1",
				Channel: models.ChannelEmail,
				Source:  "email",
				Subject: "Update on TASK-00001",
				Content: "Here is a detailed update on progress for the authentication feature.",
				From:    "alice@example.com",
			},
		},
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	result, err := orch.Run(RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dry run should count the output but not actually send.
	if result.OutputsDelivered != 1 {
		t.Errorf("expected 1 output delivered (dry run), got %d", result.OutputsDelivered)
	}
	if len(adapter.sentItems) != 0 {
		t.Errorf("expected 0 sends in dry run, got %d", len(adapter.sentItems))
	}
	if len(adapter.marked) != 0 {
		t.Errorf("expected 0 mark-processed in dry run, got %d", len(adapter.marked))
	}
}

func TestFeedbackLoop_EventLogging(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name:  "test",
		typ:   models.ChannelEmail,
		items: nil,
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	logger := &fakeEventLogger{}
	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, logger, "TASK")

	_, err := orch.Run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should log cycle completion.
	found := false
	for _, e := range logger.events {
		if e.eventType == "loop.cycle_completed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected loop.cycle_completed event, not found")
	}
}

func TestFeedbackLoop_OutputGeneration(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name: "email",
		typ:  models.ChannelEmail,
		items: []models.ChannelItem{
			{
				ID:      "out1",
				Channel: models.ChannelEmail,
				Source:  "email",
				Subject: "TASK-00010 deployment plan",
				Content: "We need to deploy TASK-00010 by Friday with the new auth changes.",
				From:    "bob@example.com",
			},
		},
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	result, err := orch.Run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.OutputsDelivered != 1 {
		t.Errorf("expected 1 output delivered, got %d", result.OutputsDelivered)
	}
	if len(adapter.sentItems) != 1 {
		t.Fatalf("expected 1 sent item, got %d", len(adapter.sentItems))
	}

	output := adapter.sentItems[0]
	if output.InReplyTo != "out1" {
		t.Errorf("expected InReplyTo %q, got %q", "out1", output.InReplyTo)
	}
	if output.SourceTask != "TASK-00010" {
		t.Errorf("expected SourceTask %q, got %q", "TASK-00010", output.SourceTask)
	}
	if output.Destination != "bob@example.com" {
		t.Errorf("expected Destination %q, got %q", "bob@example.com", output.Destination)
	}
}

func TestFeedbackLoop_SubstantiveContentCreatesKnowledge(t *testing.T) {
	reg := NewChannelRegistry()
	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	// Item with no tags but substantive content (>= 50 chars)
	item := models.ChannelItem{
		ID:      "sub1",
		Channel: models.ChannelFile,
		Subject: "Meeting notes",
		Content: "This is a substantive piece of content that contains enough information to be worth recording as knowledge in the system.",
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Action != "create_knowledge" {
		t.Errorf("expected action %q, got %q", "create_knowledge", result.Action)
	}
	if len(result.KnowledgeIDs) != 1 {
		t.Errorf("expected 1 knowledge ID, got %d", len(result.KnowledgeIDs))
	}
}

func TestFeedbackLoop_HighPriorityWithTaskReference(t *testing.T) {
	reg := NewChannelRegistry()
	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "TASK")

	// High priority item that also references a task should route to task.
	item := models.ChannelItem{
		ID:       "hp-task",
		Channel:  models.ChannelEmail,
		Subject:  "Urgent: TASK-00005 is blocked",
		Content:  "We need to unblock TASK-00005 immediately.",
		Priority: models.ChannelPriorityHigh,
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Task reference takes precedence over high priority.
	if result.Action != "route_to_task" {
		t.Errorf("expected action %q, got %q", "route_to_task", result.Action)
	}
	if result.RelatedTask != "TASK-00005" {
		t.Errorf("expected related task %q, got %q", "TASK-00005", result.RelatedTask)
	}
}

func TestFeedbackLoop_FetchError(t *testing.T) {
	reg := NewChannelRegistry()
	adapter := &fakeChannelAdapter{
		name:     "broken",
		typ:      models.ChannelSlack,
		fetchErr: fmt.Errorf("connection timeout"),
	}
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("registering adapter: %v", err)
	}

	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "TASK")
	result, err := orch.Run(RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.ItemsFetched != 0 {
		t.Errorf("expected 0 items fetched, got %d", result.ItemsFetched)
	}
}

func TestFeedbackLoop_CustomPrefixMatching(t *testing.T) {
	reg := NewChannelRegistry()
	km := &fakeKnowledgeManager{}
	orch := NewFeedbackLoopOrchestrator(reg, km, nil, nil, "CCAAS")

	item := models.ChannelItem{
		ID:      "custom-prefix",
		Channel: models.ChannelEmail,
		Subject: "Update on CCAAS-00042",
		Content: "Progress on the task.",
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RelatedTask != "CCAAS-00042" {
		t.Errorf("expected related task %q, got %q", "CCAAS-00042", result.RelatedTask)
	}
	if result.Action != "route_to_task" {
		t.Errorf("expected action %q, got %q", "route_to_task", result.Action)
	}
}

func TestFeedbackLoop_NoMatchForWrongPrefix(t *testing.T) {
	reg := NewChannelRegistry()
	orch := NewFeedbackLoopOrchestrator(reg, nil, nil, nil, "CCAAS")

	item := models.ChannelItem{
		ID:      "wrong-prefix",
		Channel: models.ChannelEmail,
		Subject: "Update on TASK-00042",
		Content: "This references a different prefix.",
	}

	result, err := orch.ProcessItem(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// TASK-00042 should NOT match when prefix is CCAAS.
	if result.RelatedTask != "" {
		t.Errorf("expected no related task, got %q", result.RelatedTask)
	}
}

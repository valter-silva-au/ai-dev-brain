package core

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// taskIDPattern matches TASK-XXXXX references in text.
var taskIDPattern = regexp.MustCompile(`TASK-\d{5}`)

// RunOptions configures a feedback loop cycle.
type RunOptions struct {
	DryRun        bool
	ChannelFilter string
}

// LoopResult summarises what a feedback loop cycle accomplished.
type LoopResult struct {
	ItemsFetched     int
	ItemsProcessed   int
	OutputsDelivered int
	KnowledgeAdded   int
	Skipped          int
	Errors           []string
}

// ProcessedItem captures the result of classifying and routing a single
// channel item through the feedback loop.
type ProcessedItem struct {
	Item         models.ChannelItem
	RelatedTask  string
	KnowledgeIDs []string
	Action       string
	Output       *models.OutputItem
}

// FeedbackLoopOrchestrator coordinates the input -> classify -> route -> output
// cycle across all registered channel adapters.
type FeedbackLoopOrchestrator interface {
	// Run executes a full feedback loop cycle.
	Run(opts RunOptions) (*LoopResult, error)
	// ProcessItem classifies and routes a single channel item.
	ProcessItem(item models.ChannelItem) (*ProcessedItem, error)
}

type feedbackLoopOrchestrator struct {
	channelReg   ChannelRegistry
	knowledgeMgr KnowledgeManager
	backlogStore BacklogStore
	eventLogger  EventLogger
}

// NewFeedbackLoopOrchestrator creates a FeedbackLoopOrchestrator.
// knowledgeMgr, backlogStore, and eventLogger may be nil.
func NewFeedbackLoopOrchestrator(channelReg ChannelRegistry, knowledgeMgr KnowledgeManager, backlogStore BacklogStore, eventLogger EventLogger) FeedbackLoopOrchestrator {
	return &feedbackLoopOrchestrator{
		channelReg:   channelReg,
		knowledgeMgr: knowledgeMgr,
		backlogStore: backlogStore,
		eventLogger:  eventLogger,
	}
}

// Run executes a full feedback loop cycle: fetch items from all channels,
// classify each item, route to tasks or knowledge store, and generate outputs.
func (f *feedbackLoopOrchestrator) Run(opts RunOptions) (*LoopResult, error) {
	result := &LoopResult{}

	adapters := f.channelReg.ListAdapters()
	if opts.ChannelFilter != "" {
		var filtered []ChannelAdapter
		for _, a := range adapters {
			if a.Name() == opts.ChannelFilter {
				filtered = append(filtered, a)
			}
		}
		adapters = filtered
	}

	// Fetch items from selected adapters.
	var allItems []models.ChannelItem
	for _, adapter := range adapters {
		items, err := adapter.Fetch()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetching from %s: %s", adapter.Name(), err))
			continue
		}
		allItems = append(allItems, items...)
	}
	result.ItemsFetched = len(allItems)

	// Process each item.
	for _, item := range allItems {
		processed, err := f.ProcessItem(item)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("processing item %s: %s", item.ID, err))
			continue
		}

		if processed.Action == "skip" {
			result.Skipped++
			continue
		}

		result.ItemsProcessed++
		result.KnowledgeAdded += len(processed.KnowledgeIDs)

		// Deliver output if generated and not a dry run.
		if processed.Output != nil && !opts.DryRun {
			adapter, err := f.channelReg.GetAdapter(string(item.Channel))
			if err != nil {
				// Fall back: try item source as adapter name.
				adapter, err = f.channelReg.GetAdapter(item.Source)
			}
			if err == nil {
				if sendErr := adapter.Send(*processed.Output); sendErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("sending output for %s: %s", item.ID, sendErr))
				} else {
					result.OutputsDelivered++
				}
			}
		} else if processed.Output != nil && opts.DryRun {
			result.OutputsDelivered++
		}

		// Mark processed unless dry run.
		if !opts.DryRun {
			adapter, err := f.channelReg.GetAdapter(string(item.Channel))
			if err != nil {
				adapter, _ = f.channelReg.GetAdapter(item.Source)
			}
			if adapter != nil {
				_ = adapter.MarkProcessed(item.ID)
			}
		}
	}

	// Log cycle completion.
	f.logEvent("loop.cycle_completed", map[string]any{
		"items_fetched":     result.ItemsFetched,
		"items_processed":   result.ItemsProcessed,
		"outputs_delivered": result.OutputsDelivered,
		"knowledge_added":   result.KnowledgeAdded,
		"skipped":           result.Skipped,
		"errors":            len(result.Errors),
	})

	return result, nil
}

// ProcessItem classifies and routes a single channel item.
func (f *feedbackLoopOrchestrator) ProcessItem(item models.ChannelItem) (*ProcessedItem, error) {
	processed := &ProcessedItem{
		Item: item,
	}

	// 1. Scan content and subject for TASK-XXXXX patterns.
	processed.RelatedTask = f.findTaskReference(item)

	// 2. Classify the item and determine action.
	processed.Action = f.classify(item, processed.RelatedTask)

	// 3. Ingest knowledge for items that have identifiable content.
	if f.knowledgeMgr != nil && (processed.Action == "route_to_task" || processed.Action == "create_knowledge") {
		ids, err := f.ingestKnowledge(item, processed.RelatedTask)
		if err != nil {
			return nil, fmt.Errorf("processing item %s: ingesting knowledge: %w", item.ID, err)
		}
		processed.KnowledgeIDs = ids
	}

	// 4. Generate output for items routed to a task.
	if processed.Action == "route_to_task" && processed.RelatedTask != "" {
		processed.Output = f.generateOutput(item, processed.RelatedTask)
	}

	// Log item processing.
	f.logEvent("loop.item_processed", map[string]any{
		"item_id":      item.ID,
		"channel":      string(item.Channel),
		"action":       processed.Action,
		"related_task": processed.RelatedTask,
		"knowledge":    len(processed.KnowledgeIDs),
	})

	return processed, nil
}

// findTaskReference scans the item's subject, content, and RelatedTask field
// for TASK-XXXXX patterns.
func (f *feedbackLoopOrchestrator) findTaskReference(item models.ChannelItem) string {
	// Check the RelatedTask field first.
	if item.RelatedTask != "" {
		return item.RelatedTask
	}

	// Scan subject then content.
	if match := taskIDPattern.FindString(item.Subject); match != "" {
		return match
	}
	if match := taskIDPattern.FindString(item.Content); match != "" {
		return match
	}

	return ""
}

// classify determines the action to take for an item based on rule-based logic.
func (f *feedbackLoopOrchestrator) classify(item models.ChannelItem, relatedTask string) string {
	// If item relates to a task, route it there and ingest as knowledge.
	if relatedTask != "" {
		return "route_to_task"
	}

	// If item has high priority and no related task, it needs attention.
	if item.Priority == models.ChannelPriorityHigh {
		return "needs_attention"
	}

	// If item has tags or substantive content, create knowledge.
	if len(item.Tags) > 0 || f.hasSubstantiveContent(item) {
		return "create_knowledge"
	}

	return "skip"
}

// hasSubstantiveContent checks whether the item has enough content to be
// worth recording as knowledge (more than just a greeting or empty message).
func (f *feedbackLoopOrchestrator) hasSubstantiveContent(item models.ChannelItem) bool {
	content := strings.TrimSpace(item.Content)
	// Consider content substantive if it has at least 50 characters.
	return len(content) >= 50
}

// ingestKnowledge adds a knowledge entry for the item via the KnowledgeManager.
func (f *feedbackLoopOrchestrator) ingestKnowledge(item models.ChannelItem, relatedTask string) ([]string, error) {
	topic := ""
	if len(item.Tags) > 0 {
		topic = item.Tags[0]
	}

	summary := item.Subject
	if summary == "" {
		summary = truncate(item.Content, 120)
	}

	detail := item.Content

	id, err := f.knowledgeMgr.AddKnowledge(
		models.KnowledgeTypeLearning,
		topic,
		summary,
		detail,
		relatedTask,
		models.SourceChannel,
		nil,
		item.Tags,
	)
	if err != nil {
		return nil, err
	}

	return []string{id}, nil
}

// generateOutput creates an OutputItem acknowledging receipt of a channel item
// that was routed to a task.
func (f *feedbackLoopOrchestrator) generateOutput(item models.ChannelItem, relatedTask string) *models.OutputItem {
	return &models.OutputItem{
		ID:          fmt.Sprintf("out-%s", item.ID),
		Channel:     item.Channel,
		Destination: item.From,
		Subject:     fmt.Sprintf("Re: %s", item.Subject),
		Content:     fmt.Sprintf("Received and routed to %s.", relatedTask),
		InReplyTo:   item.ID,
		SourceTask:  relatedTask,
	}
}

// logEvent emits an event if an EventLogger is configured.
func (f *feedbackLoopOrchestrator) logEvent(eventType string, data map[string]any) {
	if f.eventLogger != nil {
		_ = f.eventLogger.LogEvent(eventType, data)
	}
}

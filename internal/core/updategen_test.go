package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/storage"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// --- Helpers ---

// setupTaskWithComms creates a task ticket directory with context.md and communication files.
func setupTaskWithComms(t *testing.T, basePath, taskID string, comms []models.Communication) {
	t.Helper()
	ctxMgr := storage.NewContextManager(basePath)
	commMgr := storage.NewCommunicationManager(basePath)

	if _, err := ctxMgr.InitializeContext(taskID); err != nil {
		t.Fatalf("initializing context: %v", err)
	}

	for _, comm := range comms {
		if err := commMgr.AddCommunication(taskID, comm); err != nil {
			t.Fatalf("adding communication: %v", err)
		}
	}
}

// --- GenerateUpdates tests ---

func TestGenerateUpdates_EmptyTaskID(t *testing.T) {
	base := t.TempDir()
	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)
	gen := NewUpdateGenerator(ctxMgr, commMgr)

	_, err := gen.GenerateUpdates("")
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestGenerateUpdates_NoComms_EmptyPlan(t *testing.T) {
	base := t.TempDir()
	taskID := "TASK-00001"
	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)

	if _, err := ctxMgr.InitializeContext(taskID); err != nil {
		t.Fatalf("initializing context: %v", err)
	}

	gen := NewUpdateGenerator(ctxMgr, commMgr)
	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TaskID != taskID {
		t.Errorf("TaskID = %q, want %q", plan.TaskID, taskID)
	}
	if len(plan.Messages) != 0 {
		t.Errorf("Messages count = %d, want 0 (no contacts)", len(plan.Messages))
	}
	if plan.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}

func TestGenerateUpdates_WithComms_GeneratesMessages(t *testing.T) {
	base := t.TempDir()
	taskID := "TASK-00042"

	comms := []models.Communication{
		{
			Date:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Source:  "Slack",
			Contact: "@alice",
			Topic:   "OAuth requirements",
			Content: "Need PKCE support",
			Tags:    []models.CommunicationTag{models.TagRequirement},
		},
		{
			Date:    time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			Source:  "Email",
			Contact: "@bob",
			Topic:   "Security review",
			Content: "Please schedule a review",
			Tags:    []models.CommunicationTag{models.TagActionItem},
		},
	}

	setupTaskWithComms(t, base, taskID, comms)

	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)
	gen := NewUpdateGenerator(ctxMgr, commMgr)

	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TaskID != taskID {
		t.Errorf("TaskID = %q, want %q", plan.TaskID, taskID)
	}

	// Should have messages for both contacts.
	if len(plan.Messages) != 2 {
		t.Fatalf("Messages count = %d, want 2", len(plan.Messages))
	}

	recipients := make(map[string]bool)
	for _, msg := range plan.Messages {
		recipients[msg.Recipient] = true
		if msg.Subject == "" {
			t.Error("message has empty subject")
		}
		if msg.Body == "" {
			t.Error("message has empty body")
		}
		if !strings.Contains(msg.Body, taskID) {
			t.Errorf("message body should reference %s", taskID)
		}
	}

	if !recipients["@alice"] {
		t.Error("missing message for @alice")
	}
	if !recipients["@bob"] {
		t.Error("missing message for @bob")
	}
}

func TestGenerateUpdates_ChannelFromSource(t *testing.T) {
	base := t.TempDir()
	taskID := "TASK-00010"

	comms := []models.Communication{
		{
			Date:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Source:  "Slack",
			Contact: "@slack-user",
			Topic:   "test",
			Content: "hello",
		},
		{
			Date:    time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
			Source:  "Email",
			Contact: "@email-user",
			Topic:   "test",
			Content: "hello",
		},
		{
			Date:    time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			Source:  "Teams",
			Contact: "@teams-user",
			Topic:   "test",
			Content: "hello",
		},
	}

	setupTaskWithComms(t, base, taskID, comms)

	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)
	gen := NewUpdateGenerator(ctxMgr, commMgr)

	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	channelMap := make(map[string]MessageChannel)
	for _, msg := range plan.Messages {
		channelMap[msg.Recipient] = msg.Channel
	}

	if channelMap["@slack-user"] != ChannelSlack {
		t.Errorf("@slack-user channel = %q, want %q", channelMap["@slack-user"], ChannelSlack)
	}
	if channelMap["@email-user"] != ChannelEmail {
		t.Errorf("@email-user channel = %q, want %q", channelMap["@email-user"], ChannelEmail)
	}
	if channelMap["@teams-user"] != ChannelTeams {
		t.Errorf("@teams-user channel = %q, want %q", channelMap["@teams-user"], ChannelTeams)
	}
}

func TestGenerateUpdates_BlockersCreateHighPriority(t *testing.T) {
	base := t.TempDir()
	taskID := "TASK-00020"

	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)

	if _, err := ctxMgr.InitializeContext(taskID); err != nil {
		t.Fatalf("initializing context: %v", err)
	}

	// Add a blocker to the context.
	contextPath := filepath.Join(base, "tickets", taskID, "context.md")
	contextContent := `# Task Context: TASK-00020

## Summary
Working on rate limiting.

## Current Focus
Rate limiter implementation.

## Recent Progress
- Started implementation

## Open Questions
- [ ] What rate limit to use?

## Decisions Made
- Use token bucket

## Blockers
- Waiting for API spec from backend team

## Next Steps
- [ ] Finish implementation
`
	if err := os.WriteFile(contextPath, []byte(contextContent), 0644); err != nil {
		t.Fatalf("writing context: %v", err)
	}

	// Add a communication.
	comm := models.Communication{
		Date:    time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Source:  "Slack",
		Contact: "@charlie",
		Topic:   "API spec",
		Content: "Need the spec for rate limiting",
		Tags:    []models.CommunicationTag{models.TagBlocker},
	}
	if err := commMgr.AddCommunication(taskID, comm); err != nil {
		t.Fatalf("adding communication: %v", err)
	}

	gen := NewUpdateGenerator(ctxMgr, commMgr)
	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Messages should be high priority due to blockers.
	for _, msg := range plan.Messages {
		if msg.Priority != MsgPriorityHigh {
			t.Errorf("message to %s has priority %q, want %q (blockers exist)", msg.Recipient, msg.Priority, MsgPriorityHigh)
		}
	}

	// Should have information requests for the blocker.
	if len(plan.InformationNeeded) == 0 {
		t.Fatal("expected information requests for blockers")
	}

	foundBlocking := false
	for _, req := range plan.InformationNeeded {
		if req.Blocking {
			foundBlocking = true
		}
	}
	if !foundBlocking {
		t.Error("expected at least one blocking information request")
	}
}

func TestGenerateUpdates_DeduplicatesContacts(t *testing.T) {
	base := t.TempDir()
	taskID := "TASK-00030"

	comms := []models.Communication{
		{
			Date:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Source:  "Slack",
			Contact: "@alice",
			Topic:   "first",
			Content: "hello",
		},
		{
			Date:    time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
			Source:  "Slack",
			Contact: "@alice",
			Topic:   "second",
			Content: "follow up",
		},
	}

	setupTaskWithComms(t, base, taskID, comms)

	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)
	gen := NewUpdateGenerator(ctxMgr, commMgr)

	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have only one message for @alice (deduplicated).
	if len(plan.Messages) != 1 {
		t.Errorf("Messages count = %d, want 1 (deduplicated)", len(plan.Messages))
	}
	if plan.Messages[0].Recipient != "@alice" {
		t.Errorf("recipient = %q, want %q", plan.Messages[0].Recipient, "@alice")
	}
}

func TestGenerateUpdates_MessagesOrderedByPriority(t *testing.T) {
	// This test verifies Property 18: messages are ordered by priority.
	base := t.TempDir()
	taskID := "TASK-00040"

	ctxMgr := storage.NewContextManager(base)
	commMgr := storage.NewCommunicationManager(base)

	if _, err := ctxMgr.InitializeContext(taskID); err != nil {
		t.Fatalf("initializing context: %v", err)
	}

	// Add a blocker to context to make messages high priority.
	contextPath := filepath.Join(base, "tickets", taskID, "context.md")
	contextContent := `# Task Context: TASK-00040

## Summary
Working on feature.

## Current Focus
Building feature.

## Recent Progress
- Started

## Open Questions

## Decisions Made

## Blockers
- Need security review

## Next Steps
- [ ] Continue
`
	if err := os.WriteFile(contextPath, []byte(contextContent), 0644); err != nil {
		t.Fatalf("writing context: %v", err)
	}

	comms := []models.Communication{
		{
			Date:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Source:  "Slack",
			Contact: "@alice",
			Topic:   "feature",
			Content: "general update",
		},
		{
			Date:    time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
			Source:  "Email",
			Contact: "@bob",
			Topic:   "security",
			Content: "blocking review needed",
			Tags:    []models.CommunicationTag{models.TagBlocker},
		},
	}

	for _, comm := range comms {
		if err := commMgr.AddCommunication(taskID, comm); err != nil {
			t.Fatalf("adding communication: %v", err)
		}
	}

	gen := NewUpdateGenerator(ctxMgr, commMgr)
	plan, err := gen.GenerateUpdates(taskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(plan.Messages))
	}

	// Verify ordering: high priority messages should come before normal/low.
	for i := 1; i < len(plan.Messages); i++ {
		if priorityRank(plan.Messages[i].Priority) < priorityRank(plan.Messages[i-1].Priority) {
			t.Errorf("messages not ordered by priority: %q (%s) before %q (%s)",
				plan.Messages[i-1].Recipient, plan.Messages[i-1].Priority,
				plan.Messages[i].Recipient, plan.Messages[i].Priority)
		}
	}
}

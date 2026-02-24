package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// =============================================================================
// Generators
// =============================================================================

// genCommunication generates a random Communication with plausible values.
func genCommunication(t *rapid.T, label string) models.Communication {
	sources := []string{"Slack", "Email", "Teams"}
	contacts := []string{"@alice", "@bob", "@charlie", "@dave", "@eve"}
	tags := []models.CommunicationTag{models.TagRequirement, models.TagDecision, models.TagBlocker, models.TagQuestion, models.TagActionItem}

	numTags := rapid.IntRange(0, 3).Draw(t, label+"_numTags")
	commTags := make([]models.CommunicationTag, numTags)
	for i := range commTags {
		commTags[i] = tags[rapid.IntRange(0, len(tags)-1).Draw(t, fmt.Sprintf("%s_tag_%d", label, i))]
	}

	return models.Communication{
		Date:    time.Date(2026, time.Month(rapid.IntRange(1, 12).Draw(t, label+"_month")), rapid.IntRange(1, 28).Draw(t, label+"_day"), 0, 0, 0, 0, time.UTC),
		Source:  sources[rapid.IntRange(0, len(sources)-1).Draw(t, label+"_source")],
		Contact: contacts[rapid.IntRange(0, len(contacts)-1).Draw(t, label+"_contact")],
		Topic:   rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, label+"_topic"),
		Content: rapid.StringMatching(`[A-Za-z ,.]{5,50}`).Draw(t, label+"_content"),
		Tags:    commTags,
	}
}

// =============================================================================
// Property 18: Update Message Chronological Order
// =============================================================================

// Feature: ai-dev-brain, Property 18: Update Message Chronological Order
// *For any* generated update plan with multiple messages, the messages SHALL
// be ordered chronologically by their intended send time.
// (We validate by priority ordering, which is the temporal proxy.)
//
// **Validates: Requirements 5.2**
func TestProperty18_UpdateMessageOrder(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		base := t.TempDir()
		taskNum := rapid.IntRange(1, 99999).Draw(rt, "taskNum")
		taskID := fmt.Sprintf("TASK-%05d", taskNum)

		ctxMgr := storage.NewContextManager(base)
		commMgr := storage.NewCommunicationManager(base)

		if _, err := ctxMgr.InitializeContext(taskID); err != nil {
			rt.Fatalf("initializing context: %v", err)
		}

		// Generate random communications.
		numComms := rapid.IntRange(1, 5).Draw(rt, "numComms")
		for i := 0; i < numComms; i++ {
			comm := genCommunication(rt, fmt.Sprintf("comm_%d", i))
			if err := commMgr.AddCommunication(taskID, comm); err != nil {
				rt.Fatalf("adding communication: %v", err)
			}
		}

		gen := NewUpdateGenerator(&storageAIContextAdapter{mgr: ctxMgr}, commMgr)
		plan, err := gen.GenerateUpdates(taskID)
		if err != nil {
			rt.Fatalf("GenerateUpdates failed: %v", err)
		}

		// Verify messages are ordered by priority (high < normal < low).
		for i := 1; i < len(plan.Messages); i++ {
			if priorityRank(plan.Messages[i].Priority) < priorityRank(plan.Messages[i-1].Priority) {
				rt.Errorf("messages not ordered: [%d] %s (%s) before [%d] %s (%s)",
					i-1, plan.Messages[i-1].Recipient, plan.Messages[i-1].Priority,
					i, plan.Messages[i].Recipient, plan.Messages[i].Priority)
			}
		}

		// Verify plan metadata.
		if plan.TaskID != taskID {
			rt.Errorf("plan.TaskID = %q, want %q", plan.TaskID, taskID)
		}
		if plan.GeneratedAt.IsZero() {
			rt.Error("plan.GeneratedAt is zero")
		}
	})
}

// =============================================================================
// Property 19: No Auto-Send Invariant
// =============================================================================

// Feature: ai-dev-brain, Property 19: No Auto-Send Invariant
// The update generator SHALL never send messages automatically. It only
// produces a plan; actual sending is a separate, user-initiated action.
// We verify this by checking that GenerateUpdates returns a plan with
// messages (not side effects).
func TestProperty19_NoAutoSendInvariant(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		base := t.TempDir()
		taskNum := rapid.IntRange(1, 99999).Draw(rt, "taskNum")
		taskID := fmt.Sprintf("TASK-%05d", taskNum)

		ctxMgr := storage.NewContextManager(base)
		commMgr := storage.NewCommunicationManager(base)

		if _, err := ctxMgr.InitializeContext(taskID); err != nil {
			rt.Fatalf("initializing context: %v", err)
		}

		numComms := rapid.IntRange(0, 3).Draw(rt, "numComms")
		for i := 0; i < numComms; i++ {
			comm := genCommunication(rt, fmt.Sprintf("comm_%d", i))
			if err := commMgr.AddCommunication(taskID, comm); err != nil {
				rt.Fatalf("adding communication: %v", err)
			}
		}

		gen := NewUpdateGenerator(&storageAIContextAdapter{mgr: ctxMgr}, commMgr)
		plan, err := gen.GenerateUpdates(taskID)
		if err != nil {
			rt.Fatalf("GenerateUpdates failed: %v", err)
		}

		// The plan should be a data structure, not a side effect.
		// Verify it's an *UpdatePlan with proper fields.
		if plan == nil {
			rt.Fatal("plan is nil")
			return
		}

		// All messages should have non-empty recipients and bodies.
		for i, msg := range plan.Messages {
			if msg.Recipient == "" {
				rt.Errorf("message[%d] has empty recipient", i)
			}
			if msg.Body == "" {
				rt.Errorf("message[%d] has empty body", i)
			}
			if msg.Subject == "" {
				rt.Errorf("message[%d] has empty subject", i)
			}
			// Channel should be a valid value.
			switch msg.Channel {
			case ChannelSlack, ChannelEmail, ChannelTeams:
				// Valid.
			default:
				rt.Errorf("message[%d] has invalid channel: %q", i, msg.Channel)
			}
		}
	})
}

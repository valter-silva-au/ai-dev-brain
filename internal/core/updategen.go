package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// MessageChannel identifies the communication platform for a message.
type MessageChannel string

const (
	ChannelEmail MessageChannel = "email"
	ChannelSlack MessageChannel = "slack"
	ChannelTeams MessageChannel = "teams"
)

// MessagePriority indicates the urgency of a planned message.
type MessagePriority string

const (
	MsgPriorityHigh   MessagePriority = "high"
	MsgPriorityNormal MessagePriority = "normal"
	MsgPriorityLow    MessagePriority = "low"
)

// UpdatePlan is the result of analyzing a task's context and communications
// to determine what updates should be sent to stakeholders.
type UpdatePlan struct {
	TaskID            string
	Messages          []PlannedMessage
	InformationNeeded []InformationRequest
	GeneratedAt       time.Time
}

// PlannedMessage describes a single message to send to a stakeholder.
type PlannedMessage struct {
	Recipient string
	Reason    string
	Channel   MessageChannel
	Subject   string
	Body      string
	Priority  MessagePriority
}

// InformationRequest describes information needed from a stakeholder
// to unblock or advance the task.
type InformationRequest struct {
	From     string
	Question string
	Context  string
	Blocking bool
}

// UpdateGenerator generates stakeholder communication update plans
// by analyzing task context, communications, and blockers.
type UpdateGenerator interface {
	GenerateUpdates(taskID string) (*UpdatePlan, error)
}

// updateGenerator implements UpdateGenerator.
type updateGenerator struct {
	contextMgr AIContextProvider
	commMgr    CommunicationStore
}

// NewUpdateGenerator creates a new UpdateGenerator backed by the given
// context and communication managers.
func NewUpdateGenerator(ctxMgr AIContextProvider, commMgr CommunicationStore) UpdateGenerator {
	return &updateGenerator{
		contextMgr: ctxMgr,
		commMgr:    commMgr,
	}
}

// GenerateUpdates analyzes the task's current state and communications to
// produce a plan of stakeholder updates and information requests.
func (g *updateGenerator) GenerateUpdates(taskID string) (*UpdatePlan, error) {
	if taskID == "" {
		return nil, fmt.Errorf("generating updates: task ID is empty")
	}

	// Load the task context.
	aiCtx, err := g.contextMgr.GetContextForAI(taskID)
	if err != nil {
		return nil, fmt.Errorf("generating updates for %s: loading context: %w", taskID, err)
	}

	// Load communications.
	comms, err := g.commMgr.GetAllCommunications(taskID)
	if err != nil {
		return nil, fmt.Errorf("generating updates for %s: loading communications: %w", taskID, err)
	}

	plan := &UpdatePlan{
		TaskID:      taskID,
		GeneratedAt: time.Now(),
	}

	// Identify unique contacts from communications.
	contacts := extractUniqueContacts(comms)

	// Generate progress update messages for each contact.
	for _, contact := range contacts {
		msg := buildProgressMessage(taskID, contact, aiCtx, comms)
		plan.Messages = append(plan.Messages, msg)
	}

	// Generate information requests from blockers and open questions.
	plan.InformationNeeded = buildInformationRequests(taskID, aiCtx, comms)

	// Sort messages chronologically by priority (high first, then normal, then low).
	sort.Slice(plan.Messages, func(i, j int) bool {
		return priorityRank(plan.Messages[i].Priority) < priorityRank(plan.Messages[j].Priority)
	})

	return plan, nil
}

// priorityRank returns a numeric rank for sorting (lower = higher priority).
func priorityRank(p MessagePriority) int {
	switch p {
	case MsgPriorityHigh:
		return 0
	case MsgPriorityNormal:
		return 1
	case MsgPriorityLow:
		return 2
	default:
		return 3
	}
}

// extractUniqueContacts returns a deduplicated, sorted list of contacts
// from the communication history.
func extractUniqueContacts(comms []models.Communication) []string {
	seen := make(map[string]bool)
	var contacts []string
	for _, comm := range comms {
		contact := strings.TrimSpace(comm.Contact)
		if contact != "" && !seen[contact] {
			seen[contact] = true
			contacts = append(contacts, contact)
		}
	}
	sort.Strings(contacts)
	return contacts
}

// buildProgressMessage creates a progress update message for a contact.
func buildProgressMessage(taskID string, contact string, aiCtx *AIContext, comms []models.Communication) PlannedMessage {
	// Determine the channel from the most recent communication with this contact.
	channel := ChannelSlack // default
	for i := len(comms) - 1; i >= 0; i-- {
		if strings.TrimSpace(comms[i].Contact) == contact {
			ch := channelFromSource(comms[i].Source)
			if ch != "" {
				channel = ch
			}
			break
		}
	}

	// Build the body from context.
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Update on %s:\n\n", taskID))

	if aiCtx.Summary != "" {
		body.WriteString(fmt.Sprintf("Summary: %s\n\n", aiCtx.Summary))
	}

	if len(aiCtx.RecentActivity) > 0 {
		body.WriteString("Recent Progress:\n")
		for _, item := range aiCtx.RecentActivity {
			body.WriteString(fmt.Sprintf("- %s\n", item))
		}
		body.WriteString("\n")
	}

	if len(aiCtx.Blockers) > 0 {
		body.WriteString("Blockers:\n")
		for _, item := range aiCtx.Blockers {
			body.WriteString(fmt.Sprintf("- %s\n", item))
		}
		body.WriteString("\n")
	}

	// Determine priority: high if there are blockers, otherwise normal.
	priority := MsgPriorityNormal
	if len(aiCtx.Blockers) > 0 {
		priority = MsgPriorityHigh
	}

	// Build a reason for contacting this person.
	reason := fmt.Sprintf("Progress update for %s", taskID)
	for _, comm := range comms {
		if strings.TrimSpace(comm.Contact) == contact {
			for _, tag := range comm.Tags {
				if tag == models.TagBlocker || tag == models.TagQuestion {
					reason = fmt.Sprintf("Follow-up on %s discussion about %s", comm.Topic, taskID)
					break
				}
			}
		}
	}

	return PlannedMessage{
		Recipient: contact,
		Reason:    reason,
		Channel:   channel,
		Subject:   fmt.Sprintf("Update: %s", taskID),
		Body:      body.String(),
		Priority:  priority,
	}
}

// channelFromSource maps a communication source string to a MessageChannel.
func channelFromSource(source string) MessageChannel {
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "slack"):
		return ChannelSlack
	case strings.Contains(s, "email"):
		return ChannelEmail
	case strings.Contains(s, "teams"):
		return ChannelTeams
	default:
		return ""
	}
}

// buildInformationRequests generates information requests from task blockers
// and open questions.
func buildInformationRequests(taskID string, aiCtx *AIContext, comms []models.Communication) []InformationRequest {
	var requests []InformationRequest

	// Convert blockers to information requests.
	for _, blocker := range aiCtx.Blockers {
		// Try to identify a contact from communications related to this blocker.
		from := findRelevantContact(blocker, comms)
		requests = append(requests, InformationRequest{
			From:     from,
			Question: blocker,
			Context:  fmt.Sprintf("Blocking issue for %s", taskID),
			Blocking: true,
		})
	}

	// Convert open questions to information requests.
	for _, question := range aiCtx.OpenQuestions {
		from := findRelevantContact(question, comms)
		requests = append(requests, InformationRequest{
			From:     from,
			Question: question,
			Context:  fmt.Sprintf("Open question for %s", taskID),
			Blocking: false,
		})
	}

	return requests
}

// findRelevantContact tries to find a contact from communications whose topic
// or content is related to the given text.
func findRelevantContact(text string, comms []models.Communication) string {
	textLower := strings.ToLower(text)
	// Search recent communications first (reverse order).
	for i := len(comms) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(comms[i].Topic), textLower) ||
			strings.Contains(strings.ToLower(comms[i].Content), textLower) {
			return comms[i].Contact
		}
	}
	// If no match, return the most recent contact if any.
	if len(comms) > 0 {
		return comms[len(comms)-1].Contact
	}
	return ""
}

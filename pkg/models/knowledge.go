package models

import "time"

// Decision represents a decision made during a task
type Decision struct {
	ID           string    `yaml:"id"`
	Title        string    `yaml:"title"`
	Description  string    `yaml:"description"`
	Context      string    `yaml:"context,omitempty"`
	Rationale    string    `yaml:"rationale,omitempty"`
	Alternatives []string  `yaml:"alternatives,omitempty"`
	Consequences []string  `yaml:"consequences,omitempty"`
	Status       string    `yaml:"status"` // proposed, accepted, rejected, deprecated
	DecidedBy    string    `yaml:"decided_by,omitempty"`
	DecidedAt    time.Time `yaml:"decided_at"`
	Tags         []string  `yaml:"tags,omitempty"`
	RelatedTo    []string  `yaml:"related_to,omitempty"` // related task IDs or decision IDs
}

// ExtractedKnowledge represents knowledge extracted from a completed task
type ExtractedKnowledge struct {
	TaskID      string     `yaml:"task_id"`
	ExtractedAt time.Time  `yaml:"extracted_at"`
	Decisions   []Decision `yaml:"decisions"`
	Learnings   []Learning `yaml:"learnings,omitempty"`
	Gotchas     []Gotcha   `yaml:"gotchas,omitempty"`
	References  []string   `yaml:"references,omitempty"`
	Summary     string     `yaml:"summary,omitempty"`
}

// Learning represents a learning or insight from a task
type Learning struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Category    string    `yaml:"category,omitempty"` // technical, process, domain, etc.
	Tags        []string  `yaml:"tags,omitempty"`
	Timestamp   time.Time `yaml:"timestamp"`
}

// Gotcha represents a gotcha or pitfall encountered during a task
type Gotcha struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Solution    string    `yaml:"solution,omitempty"`
	Prevention  string    `yaml:"prevention,omitempty"`
	Severity    string    `yaml:"severity,omitempty"` // low, medium, high, critical
	Tags        []string  `yaml:"tags,omitempty"`
	Timestamp   time.Time `yaml:"timestamp"`
}

// HandoffDocument represents a handoff document for an archived task
type HandoffDocument struct {
	TaskID         string     `yaml:"task_id"`
	TaskTitle      string     `yaml:"task_title"`
	GeneratedAt    time.Time  `yaml:"generated_at"`
	Summary        string     `yaml:"summary"`
	Objectives     []string   `yaml:"objectives,omitempty"`
	Completed      []string   `yaml:"completed,omitempty"`
	NotCompleted   []string   `yaml:"not_completed,omitempty"`
	Decisions      []Decision `yaml:"decisions,omitempty"`
	KeyLearnings   []Learning `yaml:"key_learnings,omitempty"`
	Gotchas        []Gotcha   `yaml:"gotchas,omitempty"`
	OpenItems      []string   `yaml:"open_items,omitempty"`
	NextSteps      []string   `yaml:"next_steps,omitempty"`
	References     []string   `yaml:"references,omitempty"`
	FilesModified  []string   `yaml:"files_modified,omitempty"`
	TestsAdded     []string   `yaml:"tests_added,omitempty"`
	Dependencies   []string   `yaml:"dependencies,omitempty"`
	ArchivedReason string     `yaml:"archived_reason,omitempty"`
	Owner          string     `yaml:"owner,omitempty"`
	Reviewers      []string   `yaml:"reviewers,omitempty"`
}

// NewDecision creates a new Decision with default values
func NewDecision(id, title, description string) *Decision {
	return &Decision{
		ID:           id,
		Title:        title,
		Description:  description,
		Status:       "proposed",
		DecidedAt:    time.Now().UTC(),
		Tags:         []string{},
		RelatedTo:    []string{},
		Alternatives: []string{},
		Consequences: []string{},
	}
}

// NewExtractedKnowledge creates a new ExtractedKnowledge with default values
func NewExtractedKnowledge(taskID string) *ExtractedKnowledge {
	return &ExtractedKnowledge{
		TaskID:      taskID,
		ExtractedAt: time.Now().UTC(),
		Decisions:   []Decision{},
		Learnings:   []Learning{},
		Gotchas:     []Gotcha{},
		References:  []string{},
	}
}

// NewHandoffDocument creates a new HandoffDocument with default values
func NewHandoffDocument(taskID, taskTitle string) *HandoffDocument {
	return &HandoffDocument{
		TaskID:        taskID,
		TaskTitle:     taskTitle,
		GeneratedAt:   time.Now().UTC(),
		Objectives:    []string{},
		Completed:     []string{},
		NotCompleted:  []string{},
		Decisions:     []Decision{},
		KeyLearnings:  []Learning{},
		Gotchas:       []Gotcha{},
		OpenItems:     []string{},
		NextSteps:     []string{},
		References:    []string{},
		FilesModified: []string{},
		TestsAdded:    []string{},
		Dependencies:  []string{},
		Reviewers:     []string{},
	}
}

// AddDecision adds a decision to the extracted knowledge
func (k *ExtractedKnowledge) AddDecision(decision Decision) {
	k.Decisions = append(k.Decisions, decision)
}

// AddLearning adds a learning to the extracted knowledge
func (k *ExtractedKnowledge) AddLearning(learning Learning) {
	k.Learnings = append(k.Learnings, learning)
}

// AddGotcha adds a gotcha to the extracted knowledge
func (k *ExtractedKnowledge) AddGotcha(gotcha Gotcha) {
	k.Gotchas = append(k.Gotchas, gotcha)
}

// IsAccepted returns true if the decision is accepted
func (d *Decision) IsAccepted() bool {
	return d.Status == "accepted"
}

// IsActive returns true if the decision is in an active state (proposed or accepted)
func (d *Decision) IsActive() bool {
	return d.Status == "proposed" || d.Status == "accepted"
}

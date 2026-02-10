package models

import "time"

// ExtractedKnowledge represents knowledge extracted from a completed task
// during the archival process.
type ExtractedKnowledge struct {
	TaskID         string
	Learnings      []string
	Decisions      []Decision
	Gotchas        []string
	RunbookUpdates []RunbookUpdate
	WikiUpdates    []WikiUpdate
}

// Decision represents a significant technical decision identified during a task,
// potentially warranting an Architecture Decision Record (ADR).
type Decision struct {
	Title        string
	Context      string
	Decision     string
	Consequences []string
	Alternatives []string
}

// HandoffDocument represents the auto-generated summary created when archiving a task,
// capturing learnings and decisions for future reference.
type HandoffDocument struct {
	TaskID        string
	Summary       string
	CompletedWork []string
	OpenItems     []string
	Learnings     []string
	RelatedDocs   []string
	GeneratedAt   time.Time
}

// RunbookUpdate represents an update to an operational runbook
// derived from knowledge extracted during task archival.
type RunbookUpdate struct {
	RunbookPath string
	Section     string
	Content     string
	TaskID      string
}

// WikiUpdate represents an update to a wiki page
// derived from knowledge extracted during task archival.
type WikiUpdate struct {
	WikiPath string
	Topic    string
	Content  string
	TaskID   string
}

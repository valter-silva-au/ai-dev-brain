package models

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TaskType represents the type of task
type TaskType string

const (
	TaskTypeFeat     TaskType = "feat"
	TaskTypeFix      TaskType = "fix"
	TaskTypeRefactor TaskType = "refactor"
	TaskTypeDocs     TaskType = "docs"
	TaskTypeChore    TaskType = "chore"
	TaskTypeTest     TaskType = "test"
	TaskTypePerf     TaskType = "perf"
	TaskTypeSpike    TaskType = "spike"

	// TaskTypeWork and TaskTypePrototype are the NON-CODE task types (D10). They
	// model artifact/graph deliverables and time-boxed experiments rather than
	// shipped code, and are STAGE-AGNOSTIC — the StageGate enforces stage
	// discipline, the type never does.
	//
	// TaskTypeWork is an artifact/graph deliverable (a doc, a decision, a piece of
	// research): the create path deliberately builds NO worktree and NO branch for
	// it even when --repo is given (see TaskManager.Create), and with no code
	// checkout there is nothing for code gates to act on — it is exempt by
	// construction. ConventionalType(work) has no special case (it falls through
	// to "work") because a work task never mints a branch.
	TaskTypeWork TaskType = "work"
	// TaskTypePrototype (a.k.a. validation-spike) is a time-boxed experiment. It
	// DOES get a worktree/branch when a repo is set — like spike, its branch
	// prefix maps to chore (a throwaway experiment is chore-shaped).
	TaskTypePrototype TaskType = "prototype"

	// TaskTypeBug is the legacy type retained only for backward compatibility:
	// old backlog.yaml entries created before the WS-B taxonomy overhaul may
	// still carry it, ConventionalType still maps it to "fix", and
	// `adb task migrate-types` rewrites it to TaskTypeFix. It is deliberately
	// NOT in ValidTaskTypes — the create path and MCP server reject it with a
	// "use `fix`" hint so no new bug-typed tasks are minted.
	TaskTypeBug TaskType = "bug"
)

// ValidTaskTypes is the task-type set accepted when creating a task: the eight
// Conventional-Commits CODE types (WS-B) followed by the two NON-CODE types
// `work` and `prototype` (D10). It intentionally excludes the legacy
// TaskTypeBug alias — callers that pass "bug" are steered to "fix". Order is
// display order (used to render the "must be one of …" validation hint).
var ValidTaskTypes = []TaskType{
	TaskTypeFeat, TaskTypeFix, TaskTypeRefactor, TaskTypeDocs,
	TaskTypeChore, TaskTypeTest, TaskTypePerf, TaskTypeSpike,
	TaskTypeWork, TaskTypePrototype,
}

// IsValid reports whether t is one of the canonical ValidTaskTypes. The legacy
// "bug" alias and any unknown string return false; use ConventionalType (which
// still maps bug->fix) when you need the branch prefix for a legacy entry.
func (t TaskType) IsValid() bool {
	for _, v := range ValidTaskTypes {
		if t == v {
			return true
		}
	}
	return false
}

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusBacklog    TaskStatus = "backlog"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusReview     TaskStatus = "review"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusArchived   TaskStatus = "archived"
)

// Priority represents the priority level of a task
type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

// IsValid reports whether p is one of the four canonical priorities (P0–P3).
// Registries that sort by priority (e.g. the tech-debt triage list) must reject
// anything else, or an out-of-set value like "p0" sorts into an arbitrary
// position and mis-triages the item (#158).
func (p Priority) IsValid() bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	default:
		return false
	}
}

// Task represents a task in the system
type Task struct {
	ID           string            `yaml:"id"`
	Title        string            `yaml:"title"`
	Type         TaskType          `yaml:"type"`
	Source       string            `yaml:"source,omitempty"`
	Status       TaskStatus        `yaml:"status"`
	Priority     Priority          `yaml:"priority"`
	Owner        string            `yaml:"owner,omitempty"`
	Created      time.Time         `yaml:"created"`
	Updated      time.Time         `yaml:"updated"`
	Repo         string            `yaml:"repo,omitempty"`
	Slug         string            `yaml:"slug,omitempty"`
	Branch       string            `yaml:"branch,omitempty"`
	WorktreePath string            `yaml:"worktree_path,omitempty"`
	TicketPath   string            `yaml:"ticket_path,omitempty"`
	Tags         []string          `yaml:"tags,omitempty"`
	BlockedBy    []string          `yaml:"blocked_by,omitempty"`
	Teams        []string          `yaml:"teams,omitempty"`
	TeamMetadata map[string]string `yaml:"team_metadata,omitempty"`

	// Links are the typed graph edges this ticket declares toward other
	// entities (decision D6). They are the SOURCE OF TRUTH for the graph; the
	// derived graph index (internal/core/graph.go) is a rebuildable cache
	// computed from them. `omitempty` keeps pre-graph backlog entries
	// byte-identical on marshal. `Task.BlockedBy` migrates onto a `depends_on`
	// link (issue #110); until then both coexist and are read together.
	Links []Link `yaml:"links,omitempty"`

	// Initiative optionally associates this ticket with a founder-playbook
	// Initiative (see stage.go). The association is METADATA only: it does NOT
	// change the tickets/<platform>/<org>/<repo> path layout, and the owning
	// Organization is reachable transitively via the initiative's OrgID. The
	// active initiative's Stage is surfaced in the worktree AI context. Empty
	// omits on marshal, so pre-association backlog entries stay byte-identical.
	Initiative string `yaml:"initiative,omitempty"`

	// ===== WS-E: GitHub/GitLab issue-sync linkage (per-ticket, backlog.yaml-persisted) =====
	// RemoteIssue is the remote issue NUMBER (0 = unlinked). RemoteURL is its html_url.
	// LastSynced/SyncHash form the last-writer-wins reconcile baseline: SyncHash is the
	// hash of the synced-fields snapshot at LastSynced, so a later local OR remote edit
	// is detectable against a stored per-sync baseline (not against the LOCAL Updated
	// timestamp — that would be unsound). Auth NEVER lives here; providers read the
	// host `gh`/`glab` login. All four keys omit on zero so pre-WS-E entries stay
	// byte-identical when marshalled.
	RemoteIssue int       `yaml:"remote_issue,omitempty"`
	RemoteURL   string    `yaml:"remote_url,omitempty"`
	LastSynced  time.Time `yaml:"last_synced,omitempty"`
	SyncHash    string    `yaml:"sync_hash,omitempty"`
}

// NewTask creates a new task with default values
func NewTask(id, title string, taskType TaskType) *Task {
	now := time.Now().UTC()
	return &Task{
		ID:           id,
		Title:        title,
		Type:         taskType,
		Status:       TaskStatusBacklog,
		Priority:     PriorityP2,
		Created:      now,
		Updated:      now,
		Tags:         []string{},
		BlockedBy:    []string{},
		Teams:        []string{},
		TeamMetadata: make(map[string]string),
	}
}

// IsActive returns true if the task is in an active status
func (t *Task) IsActive() bool {
	return t.Status == TaskStatusInProgress || t.Status == TaskStatusReview || t.Status == TaskStatusBlocked
}

// IsBlocked returns true if the task is blocked: its status is blocked, it has
// a legacy BlockedBy entry, OR it declares a `depends_on` edge (the generic
// edge model that BlockedBy migrates onto, issue #110). A depends_on edge is
// treated as an (unmet) dependency exactly as a BlockedBy entry was, so the
// answer is identical before and after migration. `blocks`/`relates_to`/other
// edges do NOT block the declaring task — a `blocks` edge means it blocks
// others, and blocked-ness is a property of the task's own record (as BlockedBy
// always was), not of edges declared elsewhere.
func (t *Task) IsBlocked() bool {
	if t.Status == TaskStatusBlocked || len(t.BlockedBy) > 0 {
		return true
	}
	for _, l := range t.Links {
		if l.Type == EdgeDependsOn {
			return true
		}
	}
	return false
}

// DependsOn returns the targets of this task's `depends_on` edges — the tickets
// this task is blocked by under the generic edge model. It is the edge-model
// analogue of the legacy BlockedBy field.
func (t *Task) DependsOn() []string {
	var deps []string
	for _, l := range t.Links {
		if l.Type == EdgeDependsOn {
			deps = append(deps, l.Target)
		}
	}
	return deps
}

// MigrateBlockedByToLinks folds the legacy BlockedBy dependency list onto the
// generic edge model (issue #110): each entry becomes a `depends_on` link
// (deduped against existing depends_on links so a partially-migrated backlog is
// safe) and BlockedBy is cleared. Returns true if the task changed. Idempotent:
// a task with an empty BlockedBy is left untouched and returns false, so a
// second migration pass is a no-op.
func (t *Task) MigrateBlockedByToLinks() bool {
	if len(t.BlockedBy) == 0 {
		return false
	}
	existing := make(map[string]bool)
	for _, l := range t.Links {
		if l.Type == EdgeDependsOn {
			existing[l.Target] = true
		}
	}
	for _, id := range t.BlockedBy {
		if id == "" || existing[id] {
			continue
		}
		t.Links = append(t.Links, Link{Type: EdgeDependsOn, Target: id})
		existing[id] = true
	}
	t.BlockedBy = nil
	return true
}

// UpdateTimestamp updates the Updated timestamp to the current UTC time
func (t *Task) UpdateTimestamp() {
	t.Updated = time.Now().UTC()
}

// slugReplaceRunes is the conservative whitelist used by Slugify: any rune not
// in [a-z0-9-] becomes a dash, then runs of dashes collapse to one. The form
// matches the Conventional-branch suffix the ADR mandates (e.g.
// "chore/platonic-g0-insurability-probe") and the task slug stored on the
// model.
var slugReplaceRunes = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashRun = regexp.MustCompile(`-+`)

// Slugify produces a kebab-case slug from a free-form string. It lowercases,
// replaces every non-[a-z0-9-] run with a single dash, and trims leading and
// trailing dashes. An all-non-alphanumeric input collapses to the empty string;
// callers pass the result through a fallback (typically the task ID).
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	s = slugReplaceRunes.ReplaceAllString(s, "-")
	s = slugDashRun.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// ConventionalType maps an adb TaskType to the Conventional Commits prefix used
// for branch names and PR titles. The canonical WS-B set (feat, fix, refactor,
// docs, chore, test, perf) maps 1:1 to its own prefix via the default case; the
// non-identity mappings are: legacy bug -> fix and spike -> chore (fixed by the
// adb-correlation-layout ADR), and prototype -> chore (a throwaway experiment
// is chore-shaped, like spike). The non-code `work` type has NO case — it never
// mints a branch (TaskManager.Create skips worktree/branch creation for it), so
// it falls through to "work" harmlessly. Unknown types fall through to the raw
// type string so future TaskTypes don't silently break branch creation.
func ConventionalType(t TaskType) string {
	switch t {
	case TaskTypeBug:
		return "fix"
	case TaskTypeSpike, TaskTypePrototype:
		return "chore"
	case TaskTypeFeat:
		return "feat"
	case TaskTypeRefactor:
		return "refactor"
	default:
		return string(t)
	}
}

// BranchName returns the Conventional branch name for a task: `<conv-type>/<slug>`.
//
// The branch is the conventional-type prefix joined with the kebab-case slug
// derived from the task's create-time argument. If slug is empty (defensive
// fallback only — Create now always supplies one), the task ID is used so the
// resulting branch is still unique. If taskType is empty, conv-type degrades
// to the empty type string but the slash is still emitted to keep the
// Conventional shape parseable; in practice Create always passes a real type.
func BranchName(taskType TaskType, slug, id string) string {
	if slug == "" {
		slug = strings.ToLower(id)
	}
	return ConventionalType(taskType) + "/" + slug
}

// IssueBranchName is the ADR-0002-aware branch for an issue-linked ticket: once
// a ticket carries a remote issue NUMBER, the branch encodes it as
// <conv-type>/<issue>-<slug> (e.g. feat/210-my-slug) so the branch, the issue,
// and the ticket share one identity. Falls back to BranchName when issue <= 0
// (an unlinked ticket keeps the plain <conv-type>/<slug> shape).
func IssueBranchName(taskType TaskType, slug, id string, issue int) string {
	if issue <= 0 {
		return BranchName(taskType, slug, id)
	}
	if slug == "" {
		slug = strings.ToLower(id)
	}
	return ConventionalType(taskType) + "/" + strconv.Itoa(issue) + "-" + slug
}

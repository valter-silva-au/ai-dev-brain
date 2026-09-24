package core

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fallbackStandingRules is the terse stub used when no standing-rules content
// is injected (BootstrapConfig.StandingRules empty — tests, or a composition
// root that doesn't wire a harness). Bootstrap never fails on a missing
// harness asset; the full rules live in the templates tree and are loaded by
// the composition root (internal/app.go), keeping core free of any embedded-FS
// import.

// BootstrapConfig contains configuration for bootstrapping a task
type BootstrapConfig struct {
	// TaskID is the unique task identifier (e.g., "TASK-00001")
	TaskID string
	// Title is the task title
	Title string
	// Description is the task description
	Description string
	// AcceptanceCriteria is a list of acceptance criteria
	AcceptanceCriteria []string
	// Dependencies is a list of task dependencies
	Dependencies []string
	// RelatedTasks is information about related tasks
	RelatedTasks string
	// Status is the initial task status
	Status string
	// TicketsDir is the base directory for tickets (e.g., "tickets")
	TicketsDir string
	// WorktreeDir is the worktree directory (e.g., ".") the task context is
	// written into
	WorktreeDir string
	// RepoSubPath is the platform/org/repo path under TicketsDir for the
	// nested correlation layout (e.g., "github.com/awslabs/mcp"). When set,
	// the ticket directory is created at
	// <TicketsDir>/<RepoSubPath>/<TaskID>-<Slug>. When empty, the ticket
	// directory falls back to either _local/<TaskID>-<Slug> (if Slug is
	// set) or the legacy flat <TicketsDir>/<TaskID> (if neither is set).
	RepoSubPath string
	// Slug is the kebab-case slug derived from the task's create-time branch
	// argument; persisted on the task model and used as the trailing
	// `-<Slug>` segment of the ticket leaf so the correlation between
	// `tickets/...` and `work/...` is human-readable.
	Slug string
	// TicketPath is the resolved ticket directory for the task under the
	// correlation layout (e.g. tickets/<platform>/<org>/<repo>/TASK-id-slug).
	// Rendered into the worktree task-context.md Workspace section in place of
	// the old hardcoded flat `tickets/<TaskID>/` path.
	TicketPath string
	// Branch is the Conventional branch name backing the worktree
	// (e.g. `fix/<slug>`); rendered into task-context.md when set.
	Branch string
	// WorktreePath is the worktree directory for the task; rendered into
	// task-context.md when set.
	WorktreePath string
	// Phase is the current task phase (e.g. "implementation"); rendered into
	// the Tier-0 Identity block when set. Optional.
	Phase string
	// ProgressPct is the task's completion percentage (0-100); rendered into
	// the Tier-0 Identity block when non-zero. Optional.
	ProgressPct int
	// SteerDirectives is a short digest of active steer.md directives; rendered
	// into the Tier-0 Identity block when set. When empty the template points
	// at the ticket's steer.md instead. Optional.
	SteerDirectives string
	// Siblings is a list of related-ticket pointer lines (NOT their content)
	// rendered into the Tier-0 "Related tickets" block. When empty the template
	// falls back to a static pointer. Optional.
	Siblings []string
	// LiveSessions is a compact digest of other active sessions rendered into
	// the Tier-0 "Other live sessions" block. The data source (event log) is
	// wired in a later ticket; empty renders a graceful placeholder. Optional.
	LiveSessions string
	// Initiative is the display name of the founder-playbook Initiative this
	// ticket is associated with; rendered into the Tier-0 Identity block when
	// set. Optional.
	Initiative string
	// Stage is the associated Initiative's founder-playbook stage (Idea/MVP/
	// Launch/Scale); rendered alongside Initiative so the agent knows which
	// stage it is operating in. Empty when the ticket has no initiative (or the
	// stage could not be resolved) — the template then omits the line. Optional.
	Stage string
	// StandingRules is the inlined DOs/DONTs content carried into every
	// worktree's Tier-0 task-context.md. Injected by the composition root
	// (loaded from the templates FS); empty falls back to a terse stub.
	StandingRules string
	// Gate is a compact summary of the associated Initiative's most recent
	// stage-gate (e.g. "Idea->MVP (passed)"); rendered under Stage so the agent
	// sees the gate posture (decision D9). Empty when there is no initiative or
	// no gate has run yet — the template then omits the line. Optional.
	Gate string
}

// BootstrapResult contains the paths created during bootstrapping
type BootstrapResult struct {
	// TaskDir is the path to the task directory
	TaskDir string
	// StatusFile is the path to status.yaml
	StatusFile string
	// ContextFile is the path to context.md
	ContextFile string
	// NotesFile is the path to notes.md
	NotesFile string
	// DesignFile is the path to design.md
	DesignFile string
	// SessionsDir is the path to sessions directory
	SessionsDir string
	// KnowledgeDir is the path to knowledge directory
	KnowledgeDir string
	// DecisionsFile is the path to knowledge/decisions.yaml
	DecisionsFile string
	// TaskContextFile is the path to the canonical worktree task context,
	// <worktree>/.adb/task-context.md (TASK-00039 Q5; the .claude/rules/ path is
	// now a pointer to it).
	TaskContextFile string
}

// resolveTaskDir returns the full ticket directory path for a task, applying
// the nested correlation layout when RepoSubPath/Slug are set and falling
// back to the legacy flat layout otherwise. Centralised here so callers
// (BootstrapSystem and any future resolvers) agree on the rule.
func resolveTaskDir(config BootstrapConfig) string {
	switch {
	case config.RepoSubPath != "" && config.Slug != "":
		// Nested correlation layout (the path the ADR mandates):
		// tickets/<platform>/<org>/<repo>/TASK-id-slug
		return filepath.Join(config.TicketsDir, config.RepoSubPath, config.TaskID+"-"+config.Slug)
	case config.RepoSubPath != "":
		// Repo set but no slug — still nest, but use the bare TaskID as the
		// leaf so the platform/org/repo prefix is preserved. Defensive: in
		// the live code path Create always supplies a slug.
		return filepath.Join(config.TicketsDir, config.RepoSubPath, config.TaskID)
	case config.Slug != "":
		// No repo: park under _local/ with a slug-bearing leaf so repo-less
		// tasks still get a readable name and don't collide with a future
		// `_local` repo subpath.
		return filepath.Join(config.TicketsDir, "_local", config.TaskID+"-"+config.Slug)
	default:
		// Legacy flat fallback: tickets/TASK-id. Used by older tests and by
		// repo-less callers that haven't been updated to supply a slug.
		return filepath.Join(config.TicketsDir, config.TaskID)
	}
}

// BootstrapSystem scaffolds a new task's directory structure
// It creates:
// - tickets/<RepoSubPath>/<TaskID>-<Slug>/ (or the legacy/local fallback) directory
// - status.yaml, context.md, notes.md, design.md files
// - sessions/ and knowledge/ subdirectories
// - knowledge/decisions.yaml (initially empty)
// - the worktree task context: .adb/task-context.md + a pointer per harness
func BootstrapSystem(config BootstrapConfig, tm TemplateManager) (*BootstrapResult, error) {
	if config.TaskID == "" {
		return nil, fmt.Errorf("TaskID is required")
	}
	if config.Title == "" {
		return nil, fmt.Errorf("Title is required")
	}
	if config.TicketsDir == "" {
		config.TicketsDir = "tickets"
	}
	// WorktreeDir intentionally not defaulted — empty means skip task-context.md
	if config.Status == "" {
		config.Status = "pending"
	}

	// Create result structure
	result := &BootstrapResult{}

	// Create task directory using the resolved nested-or-flat layout.
	taskDir := resolveTaskDir(config)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}
	result.TaskDir = taskDir

	// Create sessions subdirectory
	sessionsDir := filepath.Join(taskDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}
	result.SessionsDir = sessionsDir

	// Create knowledge subdirectory
	knowledgeDir := filepath.Join(taskDir, "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create knowledge directory: %w", err)
	}
	result.KnowledgeDir = knowledgeDir

	// Get current timestamp
	now := time.Now().Format(time.RFC3339)

	// Prepare template data. Layer the shared task-context fields (used by
	// both the worktree task-context.md here and generateTaskContext) with the
	// extra keys the status/context/notes/design templates need.
	//
	// NOTE (TASK-00031): the description deliberately does NOT flow into
	// notes.md — context.md is the sole carrier of the why. The notes template
	// seeds an append-only session log instead; duplicate-seed detection in the
	// validate/sync path flags legacy tickets that still carry the seed.
	templateData := taskContextData(config, now)
	templateData["Dependencies"] = config.Dependencies
	templateData["RelatedTasks"] = config.RelatedTasks
	templateData["References"] = ""
	templateData["Overview"] = ""
	templateData["Components"] = ""
	templateData["DataFlow"] = ""
	templateData["ImplementationPlan"] = ""
	templateData["TechnicalDecisions"] = ""
	templateData["OpenQuestions"] = ""

	// Create status.yaml
	statusFile := filepath.Join(taskDir, "status.yaml")
	if err := renderTemplateToFile(tm, TemplateTypeStatus, templateData, statusFile); err != nil {
		return nil, fmt.Errorf("failed to create status.yaml: %w", err)
	}
	result.StatusFile = statusFile

	// Create context.md
	contextFile := filepath.Join(taskDir, "context.md")
	if err := renderTemplateToFile(tm, TemplateTypeContext, templateData, contextFile); err != nil {
		return nil, fmt.Errorf("failed to create context.md: %w", err)
	}
	result.ContextFile = contextFile

	// Create notes.md
	notesFile := filepath.Join(taskDir, "notes.md")
	if err := renderTemplateToFile(tm, TemplateTypeNotes, templateData, notesFile); err != nil {
		return nil, fmt.Errorf("failed to create notes.md: %w", err)
	}
	result.NotesFile = notesFile

	// Create design.md
	designFile := filepath.Join(taskDir, "design.md")
	if err := renderTemplateToFile(tm, TemplateTypeDesign, templateData, designFile); err != nil {
		return nil, fmt.Errorf("failed to create design.md: %w", err)
	}
	result.DesignFile = designFile

	// Create knowledge/decisions.yaml (initially empty)
	decisionsFile := filepath.Join(knowledgeDir, "decisions.yaml")
	if err := os.WriteFile(decisionsFile, []byte("# Task Decisions\n# This file tracks key decisions made during task development\n\ndecisions: []\n"), 0o644); err != nil {
		return nil, fmt.Errorf("failed to create decisions.yaml: %w", err)
	}
	result.DecisionsFile = decisionsFile

	// Write the worktree's Tier-0 task context (only when there is a worktree).
	//
	// This deliberately delegates to generateTaskContext rather than writing the
	// file itself. Both used to render task-context.md into
	// `.claude/rules/` independently, which is how one of them could be moved to
	// the agent-agnostic layout while the other kept writing the old path — and
	// the second writer is reached by a DIFFERENT entry point, so tests of the
	// first would not have noticed. One writer, one layout (TASK-00039 Q5).
	if config.WorktreeDir != "" {
		if err := generateTaskContext(config.WorktreeDir, tm, config); err != nil {
			return nil, fmt.Errorf("failed to create task context: %w", err)
		}
		result.TaskContextFile = filepath.Join(
			config.WorktreeDir, WorktreeContextDir, WorktreeContextFile,
		)
	}

	return result, nil
}

// standingRulesOr returns the injected standing rules, or the terse stub when
// nothing was wired in (tests / unwired composition roots).
func standingRulesOr(config BootstrapConfig) string {
	if strings.TrimSpace(config.StandingRules) == "" {
		return "- Reply as the workspace owner; branch discipline; production safety; model must be the strongest configured tier or better."
	}
	return strings.TrimRight(config.StandingRules, "\n")
}

// taskContextData builds the template-data map for the worktree
// task-context.md template. It is the single source of truth shared by both
// BootstrapSystem and generateTaskContext so the two paths cannot drift and
// leave the worktree file rendering empty fields (the TASK-00085 bug). `now`
// is the RFC3339 timestamp used for CreatedAt/UpdatedAt.
func taskContextData(config BootstrapConfig, now string) map[string]interface{} {
	// Best-effort state hash over the sibling + live-digest block so a later
	// ticket can cheaply detect Tier-0 drift. Empty inputs hash to "" (not a
	// hash of nothing) to keep the footer clean until the digest is wired.
	stateHash := ""
	if len(config.Siblings) > 0 || config.LiveSessions != "" {
		sum := sha256.Sum256([]byte(strings.Join(config.Siblings, "\n") + "\x00" + config.LiveSessions))
		stateHash = fmt.Sprintf("%x", sum[:6])
	}
	return map[string]interface{}{
		"TaskID":             config.TaskID,
		"Title":              config.Title,
		"Description":        config.Description,
		"AcceptanceCriteria": config.AcceptanceCriteria,
		"Status":             config.Status,
		"CreatedAt":          now,
		"UpdatedAt":          now,
		"TicketPath":         config.TicketPath,
		"Branch":             config.Branch,
		"WorktreePath":       config.WorktreePath,
		// Tier-0 optional fields (safe zero-values; template guards each).
		"Phase":           config.Phase,
		"ProgressPct":     config.ProgressPct,
		"SteerDirectives": config.SteerDirectives,
		"Siblings":        config.Siblings,
		"LiveSessions":    config.LiveSessions,
		"RepoSubPath":     config.RepoSubPath,
		"StandingRules":   standingRulesOr(config),
		"StateHash":       stateHash,
		"Initiative":      config.Initiative,
		"Stage":           config.Stage,
		"Gate":            config.Gate,
	}
}

// nowStamp is the render timestamp for a task-context render, in RFC3339.
func nowStamp() string { return time.Now().Format(time.RFC3339) }

// renderTemplateToFile renders a template and writes it to a file. Before
// rendering it stamps the machine layer into the data map: TemplateID (the
// stable template reference, e.g. "ticket/notes") and TemplateVersion (the
// 8-hex content stamp of the resolved template) — the frontmatter drift
// detection compares against (TASK-00031). A manager that cannot report a
// version stamps the empty string rather than failing the render.
func renderTemplateToFile(tm TemplateManager, templateType TemplateType, data interface{}, filePath string) error {
	data = stampTemplateData(tm, templateType, data)
	content, err := tm.RenderBytes(templateType, data)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// stampTemplateData returns a copy of data with the machine layer (template id
// + version) added, when the data is a string-keyed map — the shape all ticket
// renders use. Non-map data passes through untouched.
func stampTemplateData(tm TemplateManager, templateType TemplateType, data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	stamped := make(map[string]interface{}, len(m)+2)
	for k, v := range m {
		stamped[k] = v
	}
	stamped["TemplateID"] = TemplateID(templateType)
	stamped["TemplateVersion"] = templateVersionFor(tm, templateType)
	return stamped
}

// templateVersionFor asks the manager for the resolved template's version
// stamp; managers that can't answer (or can't resolve this type) render "".
func templateVersionFor(tm TemplateManager, templateType TemplateType) string {
	if v, ok := tm.(TemplateVersioner); ok {
		if ver, err := v.Version(templateType); err == nil {
			return ver
		}
	}
	return ""
}

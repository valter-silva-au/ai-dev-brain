package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// BacklogStore defines the interface for managing task backlogs
type BacklogStore interface {
	Load() (*models.Backlog, error)
	Save(backlog *models.Backlog) error
	AddTask(task models.Task) error
	UpdateTask(task models.Task) error
	GetTask(id string) (*models.Task, error)
	RemoveTask(id string) error
}

// ContextStore defines the interface for managing task-specific context
type ContextStore interface {
	ReadContext(taskID string) (string, error)
	WriteContext(taskID string, content string) error
	AppendContext(taskID string, section string) error
	ReadNotes(taskID string) (string, error)
	WriteNotes(taskID string, content string) error
}

// WorktreeCreator defines the interface for creating git worktrees.
//
// CreateWorktree gets the explicit branchName and worktreePath the caller
// has chosen for the nested correlation layout — the adapter threads them
// through to git's `worktree add -b <branch> <path>`. NormalizeRepoPath
// canonicalises a user-supplied --repo argument (HTTPS/SSH/platform-form)
// to <platform>/<org>/<repo>, which the TaskManager uses both as the
// nested directory prefix on the tickets/ + work/ planes and as the value
// stored on Task.Repo.
type WorktreeCreator interface {
	CreateWorktree(taskID, branchName, worktreePath, repoPath string) error
	NormalizeRepoPath(repoPath string) (string, error)
	// BranchExists reports whether a local branch already exists in the repo,
	// so Create can disambiguate a colliding <type>/<slug> before git errors
	// out (#208). A missing/unclonced repo reports false (no collision).
	BranchExists(repoPath, branch string) (bool, error)
}

// WorktreeRemover defines the interface for removing git worktrees. force skips
// the dirty/unpushed safety guard (see integration.GitWorktreeManager); RemoveBranch
// backs the opt-in orphan-branch cleanup on archive (#207).
type WorktreeRemover interface {
	RemoveWorktree(worktreePath string, force bool) error
	RemoveBranch(repoPath, branch string) error
}

// ArchiveOptions controls how Archive tears a task down (#207).
type ArchiveOptions struct {
	// Force removes the worktree even if it has uncommitted/unpushed work.
	Force bool
	// KeepWorktree archives the ticket but leaves the worktree in place.
	KeepWorktree bool
	// PruneBranch also deletes the task's local branch after the worktree is
	// removed (ignored when KeepWorktree is set).
	PruneBranch bool
}

// EventLogger defines the interface for logging events
type EventLogger interface {
	Log(eventType string, data map[string]interface{})
}

// SessionCapturer defines the interface for capturing session data
type SessionCapturer interface {
	CaptureSession(taskID, sessionID string, data map[string]interface{}) error
}

// InitiativeResolver looks up an initiative by id. It is the seam the
// TaskManager uses to (a) validate a ticket↔initiative association and (b)
// surface the initiative's founder-playbook Stage in the worktree AI context.
// It is satisfied by core.StageManager and injected optionally via
// SetInitiativeResolver — a nil resolver simply skips validation and renders
// the AI context without a Stage line (graceful degradation).
type InitiativeResolver interface {
	GetInitiative(id string) (models.Initiative, error)
}

// NeighborResolver returns the graph edges incident to an entity id. It is the
// seam the TaskManager uses to seed the worktree task-context.md with the
// ticket's bounded 1-hop neighbourhood (decision D9 — a hybrid static seed).
// It is satisfied by core.GraphManager and injected optionally via
// SetNeighborResolver — a nil resolver simply renders the context without a
// neighbourhood, byte-identical to before (graceful degradation).
type NeighborResolver interface {
	Neighbors(id string) ([]models.GraphEdge, error)
}

// TerminalStateUpdater defines the interface for updating terminal state
type TerminalStateUpdater interface {
	WriteTerminalState(worktreePath string, taskID string, state map[string]interface{}) error
}

// CreateTaskOpts contains options for creating a new task
type CreateTaskOpts struct {
	Title              string
	Description        string
	AcceptanceCriteria []string
	TaskType           models.TaskType
	Priority           models.Priority
	Owner              string
	Tags               []string
	Prefix             string
	Repo               string
	// Initiative optionally associates the new ticket with an initiative id
	// (validated against the InitiativeResolver when one is wired). Empty means
	// no association — the existing repo-less/repo-backed behaviour is unchanged.
	Initiative string
	// RemoteIssue optionally seeds the ticket's GitHub/GitLab issue number. When
	// >0 the derived branch is ADR-0002-aware (<conv-type>/<issue>-<slug>); 0
	// (the default) keeps the plain <conv-type>/<slug> shape (#210).
	RemoteIssue int
}

// TaskManager orchestrates the task lifecycle
type TaskManager struct {
	backlogStore         BacklogStore
	contextStore         ContextStore
	worktreeCreator      WorktreeCreator
	worktreeRemover      WorktreeRemover
	eventLogger          EventLogger
	sessionCapturer      SessionCapturer
	terminalStateUpdater TerminalStateUpdater
	taskIDGenerator      TaskIDGenerator
	templateManager      TemplateManager
	initiativeResolver   InitiativeResolver
	neighborResolver     NeighborResolver
	serenaProvisioner    SerenaProvisioner
	ticketsDir           string
	archivedDir          string
	worktreesDir         string
}

// NewTaskManager creates a new task manager
func NewTaskManager(
	backlogStore BacklogStore,
	contextStore ContextStore,
	worktreeCreator WorktreeCreator,
	worktreeRemover WorktreeRemover,
	eventLogger EventLogger,
	sessionCapturer SessionCapturer,
	terminalStateUpdater TerminalStateUpdater,
	taskIDGenerator TaskIDGenerator,
	templateManager TemplateManager,
	ticketsDir string,
	archivedDir string,
	worktreesDir string,
) *TaskManager {
	return &TaskManager{
		backlogStore:         backlogStore,
		contextStore:         contextStore,
		worktreeCreator:      worktreeCreator,
		worktreeRemover:      worktreeRemover,
		eventLogger:          eventLogger,
		sessionCapturer:      sessionCapturer,
		terminalStateUpdater: terminalStateUpdater,
		taskIDGenerator:      taskIDGenerator,
		templateManager:      templateManager,
		ticketsDir:           ticketsDir,
		archivedDir:          archivedDir,
		worktreesDir:         worktreesDir,
	}
}

// SetInitiativeResolver wires the (optional) initiative resolver used to
// validate ticket↔initiative associations and to surface an initiative's Stage
// in the worktree AI context. It is a post-construction setter rather than a
// constructor parameter because the association is a cross-cutting metadata
// concern layered on top of the (already large) task-lifecycle constructor, and
// because it is genuinely optional — a nil resolver degrades gracefully.
func (tm *TaskManager) SetInitiativeResolver(r InitiativeResolver) {
	tm.initiativeResolver = r
}

// SetNeighborResolver wires the (optional) graph neighbour resolver used to seed
// the worktree task-context.md with the ticket's 1-hop neighbourhood. Like the
// initiative resolver it is a post-construction setter and genuinely optional —
// a nil resolver renders the context without a neighbourhood (unchanged output).
func (tm *TaskManager) SetNeighborResolver(r NeighborResolver) {
	tm.neighborResolver = r
}

// SetSerenaProvisioner wires the (optional) per-worktree Serena provisioner
// (#202). Post-construction setter, genuinely optional: a nil provisioner
// means no .serena/project.yml is written (unchanged behaviour).
func (tm *TaskManager) SetSerenaProvisioner(p SerenaProvisioner) {
	tm.serenaProvisioner = p
}

// provisionSerena writes a per-worktree Serena config alongside the
// task-context.md written by the worktree-bootstrap seam (#202). It is
// nil-safe and fail-open: a provisioning error is logged and never blocks
// worktree/branch/ticket creation.
func (tm *TaskManager) provisionSerena(worktreePath string) {
	if tm.serenaProvisioner == nil || worktreePath == "" {
		return
	}
	if err := tm.serenaProvisioner.Provision(worktreePath, filepath.Base(worktreePath)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to provision Serena config in %s: %v\n", worktreePath, err)
	}
}

// maxContextNeighbors bounds how many 1-hop neighbours the worktree
// task-context.md lists, so the static seed stays small (decision D9). Deeper
// traversal is on-demand via `adb graph neighbors` and the MCP graph tools.
const maxContextNeighbors = 10

// neighborhoodSiblings resolves the ticket's 1-hop graph neighbourhood into
// bounded, human-readable pointer lines for the task-context.md "Related
// tickets" block. It returns nil when no resolver is wired or the ticket has no
// edges, so a ticket with no links renders byte-identically to before (no
// regression). Over maxContextNeighbors, it lists the first N and appends a
// "… and M more" pointer to the deeper tools.
func (tm *TaskManager) neighborhoodSiblings(taskID string) []string {
	if tm.neighborResolver == nil {
		return nil
	}
	edges, err := tm.neighborResolver.Neighbors(taskID)
	if err != nil || len(edges) == 0 {
		return nil
	}
	lines := make([]string, 0, len(edges))
	for _, e := range edges {
		if len(lines) >= maxContextNeighbors {
			lines = append(lines, fmt.Sprintf("… and %d more (run `adb graph neighbors %s`)", len(edges)-maxContextNeighbors, taskID))
			break
		}
		lines = append(lines, formatNeighborLine(taskID, e))
	}
	return lines
}

// formatNeighborLine renders one incident edge relative to taskID using arrows
// that show direction: `→ <target> (<type>)` for an edge this ticket declares
// (outgoing), `← <source> (<type>)` for an edge declared toward it (incoming).
func formatNeighborLine(taskID string, e models.GraphEdge) string {
	if e.From == taskID {
		return fmt.Sprintf("→ %s (%s)", e.To, e.Type)
	}
	return fmt.Sprintf("← %s (%s)", e.From, e.Type)
}

// resolveInitiativeContext returns the initiative's display name, Stage, and a
// compact gate summary for rendering into the worktree AI context (decision D9:
// the seed carries initiative/stage/gate). It is best-effort: an empty
// initiativeID, a missing resolver, or a lookup error all yield empty strings so
// the context renders without those lines rather than failing.
func (tm *TaskManager) resolveInitiativeContext(initiativeID string) (name, stage, gate string) {
	if initiativeID == "" || tm.initiativeResolver == nil {
		return "", "", ""
	}
	init, err := tm.initiativeResolver.GetInitiative(initiativeID)
	if err != nil {
		return "", "", ""
	}
	return init.Name, string(init.Stage), gateSummary(init.Gate)
}

// gateSummary renders an initiative's most recent stage-gate as one compact
// line for the AI context, e.g. "Idea->MVP (passed)". Returns "" for a nil gate
// (a pre-gate initiative) so the context omits the line entirely.
func gateSummary(g *models.GateState) string {
	if g == nil {
		return ""
	}
	status := "blocked"
	switch {
	case g.Overridden:
		status = "overridden"
	case g.Passed:
		status = "passed"
	}
	if g.Transition == "" {
		return status
	}
	return fmt.Sprintf("%s (%s)", g.Transition, status)
}

// resolveTicketDir picks the on-disk ticket directory for a task. It prefers
// the path stored on the task model (set at creation time by the nested
// layout), and only reconstructs the legacy flat path when the model
// predates the nested layout. Used by Archive/Unarchive/Delete so they
// follow the actual ticket location instead of always assuming flat.
func (tm *TaskManager) resolveTicketDir(task *models.Task) string {
	if task.TicketPath != "" {
		return task.TicketPath
	}
	return filepath.Join(tm.ticketsDir, task.ID)
}

// Create creates a new task with full lifecycle initialization
func (tm *TaskManager) Create(opts CreateTaskOpts) (*models.Task, error) {
	// Set defaults
	if opts.Prefix == "" {
		opts.Prefix = "TASK"
	}
	if opts.Priority == "" {
		opts.Priority = models.PriorityP2
	}
	if opts.TaskType == "" {
		opts.TaskType = models.TaskTypeFeat
	}

	// Validate the optional initiative association up front (before minting a
	// task ID or touching disk) so an unknown initiative fails cleanly. When no
	// resolver is wired the association is stored as-is — the field is metadata.
	if opts.Initiative != "" && tm.initiativeResolver != nil {
		if _, err := tm.initiativeResolver.GetInitiative(opts.Initiative); err != nil {
			return nil, fmt.Errorf("initiative %q: %w", opts.Initiative, err)
		}
	}

	// Generate task ID
	taskID, err := tm.taskIDGenerator.GenerateTaskID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate task ID: %w", err)
	}

	// Derive the slug + repo subpath up front so the bootstrap and worktree
	// stages share the same correlation layout. The slug is the kebab-case
	// form of the create-time branch arg (which the CLI passes as Title);
	// when the title is all-non-alphanumeric we fall back to the lowercased
	// task ID so the on-disk leaf is never empty.
	slug := models.Slugify(opts.Title)
	if slug == "" {
		slug = strings.ToLower(taskID)
	}

	// Normalize the user-supplied --repo argument once, so the same
	// `<platform>/<org>/<repo>` value is used as the nested-path prefix
	// AND stored on Task.Repo. NormalizeRepoPath also passes through
	// already-canonical forms unchanged.
	var repoSubPath string
	if opts.Repo != "" && tm.worktreeCreator != nil {
		normalized, nerr := tm.worktreeCreator.NormalizeRepoPath(opts.Repo)
		if nerr != nil {
			return nil, fmt.Errorf("failed to normalize repo path: %w", nerr)
		}
		// Only treat the normalized value as a sub-path when it looks like
		// a platform/org/repo identifier (not an absolute or relative
		// local path). Local repos still get a worktree, but skip the
		// nesting since there's no platform-qualified prefix to nest under.
		if !filepath.IsAbs(normalized) &&
			!strings.HasPrefix(normalized, "./") &&
			!strings.HasPrefix(normalized, "../") {
			repoSubPath = normalized
		}
	}

	// Create task model
	task := models.NewTask(taskID, opts.Title, opts.TaskType)
	task.Priority = opts.Priority
	task.Owner = opts.Owner
	task.Tags = opts.Tags
	task.Repo = opts.Repo
	task.RemoteIssue = opts.RemoteIssue
	task.Slug = slug
	task.Status = models.TaskStatusBacklog
	task.Initiative = opts.Initiative

	// Add to backlog
	if err := tm.backlogStore.AddTask(*task); err != nil {
		return nil, fmt.Errorf("failed to add task to backlog: %w", err)
	}

	// Bootstrap directories and files (without task-context.md — worktree doesn't exist yet).
	// Pass RepoSubPath + Slug so BootstrapSystem produces the nested ticket path
	// `tickets/<platform>/<org>/<repo>/TASK-id-slug` (or `_local/TASK-id-slug` when
	// no repo is set).
	// Resolve the initiative's Stage (best-effort) so the worktree AI context
	// tells the agent which founder-playbook stage it is operating in.
	initiativeName, stage, gate := tm.resolveInitiativeContext(task.Initiative)

	bootstrapConfig := BootstrapConfig{
		TaskID:             taskID,
		Title:              opts.Title,
		Description:        opts.Description,
		AcceptanceCriteria: opts.AcceptanceCriteria,
		Status:             string(task.Status),
		TicketsDir:         tm.ticketsDir,
		WorktreeDir:        "", // Empty — task-context.md generated after worktree creation
		RepoSubPath:        repoSubPath,
		Slug:               slug,
		Initiative:         initiativeName,
		Stage:              stage,
		Gate:               gate,
	}

	result, err := BootstrapSystem(bootstrapConfig, tm.templateManager)
	if err != nil {
		// Rollback: remove from backlog
		_ = tm.backlogStore.RemoveTask(taskID)
		return nil, fmt.Errorf("failed to bootstrap task: %w", err)
	}

	// Update task with paths
	task.TicketPath = result.TaskDir

	// Create worktree only when a repository is specified — and NEVER for the
	// non-code `work` type (D10), which is an artifact/graph deliverable, not
	// code: it gets no worktree and no branch even when --repo is given (the
	// repo association, if any, stays as ticket-nesting metadata). With no code
	// checkout there is nothing for code gates to act on, so `work` is exempt by
	// construction. `prototype` is code-shaped and takes the normal path.
	if opts.Repo != "" && tm.worktreeCreator != nil && task.Type != models.TaskTypeWork {
		// Branch is `<conv-type>/<slug>` per the ADR (e.g.
		// `chore/platonic-g0-insurability-probe`), NOT `task/<taskID>`.
		// ADR-0002-aware: an issue-linked ticket encodes its issue number in the
		// branch (<conv-type>/<issue>-<slug>); an unlinked one keeps the plain
		// <conv-type>/<slug> shape (#210).
		branchName := models.IssueBranchName(task.Type, slug, taskID, task.RemoteIssue)

		// Branch-uniqueness guard (#208): `git worktree add -b` fails if the
		// derived <type>/<slug> already exists — two same-title tickets in one
		// repo collide. Disambiguate by appending the (globally unique) task id;
		// if even that is taken, fail with a clear message rather than letting
		// git error out with raw stderr.
		if exists, berr := tm.worktreeCreator.BranchExists(opts.Repo, branchName); berr == nil && exists {
			disambiguated := branchName + "-" + strings.ToLower(taskID)
			if taken, _ := tm.worktreeCreator.BranchExists(opts.Repo, disambiguated); taken {
				_ = os.RemoveAll(result.TaskDir)
				_ = tm.backlogStore.RemoveTask(taskID)
				return nil, fmt.Errorf("branch %q already exists in %s (and disambiguated %q is taken too); rename the task or clean up the stale branch", branchName, opts.Repo, disambiguated)
			}
			branchName = disambiguated
		}

		// Worktree path mirrors the ticket path: nested under the same
		// platform/org/repo prefix when we have one, with the leaf form
		// `TASK-id-slug` so a glob like `work/**/TASK-id-*` resolves it.
		var worktreePath string
		if repoSubPath != "" {
			worktreePath = filepath.Join(tm.worktreesDir, repoSubPath, taskID+"-"+slug)
		} else {
			// Local repo (absolute/relative path) — fall back to the legacy
			// flat layout for the worktree leaf. The branch is still
			// Conventional-typed.
			worktreePath = filepath.Join(tm.worktreesDir, taskID)
		}

		if err := tm.worktreeCreator.CreateWorktree(taskID, branchName, worktreePath, opts.Repo); err != nil {
			// Rollback: remove task dir and backlog entry
			_ = os.RemoveAll(result.TaskDir)
			_ = tm.backlogStore.RemoveTask(taskID)
			return nil, fmt.Errorf("failed to create worktree: %w", err)
		}
		task.WorktreePath = worktreePath
		task.Branch = branchName

		// Populate the correlation-layout fields on the bootstrap config so the
		// worktree task-context.md renders the real ticket/worktree/branch
		// paths instead of the stale flat `tickets/<TaskID>/` placeholder
		// (TASK-00085). These are only known after the worktree is created.
		bootstrapConfig.TicketPath = task.TicketPath
		bootstrapConfig.Branch = task.Branch
		bootstrapConfig.WorktreePath = task.WorktreePath

		// Generate task-context.md inside the worktree now that it exists
		if err := generateTaskContext(worktreePath, tm.templateManager, bootstrapConfig); err != nil {
			// Non-fatal: log but continue — worktree is usable without task-context
			fmt.Fprintf(os.Stderr, "Warning: failed to generate task-context.md in worktree: %v\n", err)
		}
		// Provision a per-worktree Serena config on the same seam (#202).
		tm.provisionSerena(worktreePath)
	}

	// Update backlog with worktree info
	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		// Rollback: remove worktree and task dir (force — it was just created,
		// there is nothing to preserve).
		if tm.worktreeRemover != nil && task.WorktreePath != "" {
			_ = tm.worktreeRemover.RemoveWorktree(task.WorktreePath, true)
		}
		_ = os.RemoveAll(result.TaskDir)
		_ = tm.backlogStore.RemoveTask(taskID)
		return nil, fmt.Errorf("failed to update task in backlog: %w", err)
	}

	// Write terminal state (only if worktree was created)
	if tm.terminalStateUpdater != nil && task.WorktreePath != "" {
		terminalState := map[string]interface{}{
			"task_id":    taskID,
			"status":     task.Status,
			"created_at": task.Created,
		}
		if err := tm.terminalStateUpdater.WriteTerminalState(task.WorktreePath, taskID, terminalState); err != nil {
			// Non-fatal: log but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to write terminal state: %v\n", err)
		}
	}

	// Log event
	if tm.eventLogger != nil {
		// Payload is a contract (docs/claude/subsystems.md): task_id, title,
		// type, status, priority. The metrics calculator reads data["type"] and
		// data["status"] to build TasksByType/TasksByStatus, so both must be
		// emitted or a freshly-created task is invisible to metrics until its
		// first status change (#148).
		tm.eventLogger.Log("task.created", map[string]interface{}{
			"task_id":  taskID,
			"title":    opts.Title,
			"type":     string(task.Type),
			"status":   string(task.Status),
			"priority": opts.Priority,
			"owner":    opts.Owner,
		})

		// Emit worktree.created so "Worktrees Active" (created − removed) can
		// never go negative — the create side was previously never emitted, so
		// the dashboard derived Active = 0 − removed < 0 (#206). Only when a
		// worktree was actually created (repo-backed, non-`work` task).
		if task.WorktreePath != "" {
			tm.eventLogger.Log("worktree.created", map[string]interface{}{
				"task_id": taskID,
				"path":    task.WorktreePath,
			})
		}
	}

	return task, nil
}

// Resume loads a task and promotes it from backlog to in_progress
func (tm *TaskManager) Resume(taskID string) (*models.Task, error) {
	// Load task from backlog
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to load task: %w", err)
	}

	// Check not archived
	if task.Status == models.TaskStatusArchived {
		return nil, fmt.Errorf("cannot resume archived task %s", taskID)
	}

	// Promote to in_progress if currently in backlog
	if task.Status == models.TaskStatusBacklog {
		task.Status = models.TaskStatusInProgress
		task.UpdateTimestamp()
		if err := tm.backlogStore.UpdateTask(*task); err != nil {
			return nil, fmt.Errorf("failed to update task status: %w", err)
		}

		// Log event
		if tm.eventLogger != nil {
			tm.eventLogger.Log("task.status_changed", map[string]interface{}{
				"task_id":    taskID,
				"old_status": models.TaskStatusBacklog,
				"new_status": models.TaskStatusInProgress,
			})
		}
	}

	// Re-render the worktree Tier-0 file so status/updated fields are fresh
	// on resume (per the ticket-bootstrap-context ADR, part B). We only touch
	// the existing worktree; we do NOT regenerate root CLAUDE.md here, and we
	// reuse the shared generateTaskContext helper rather than reimplementing
	// the render. The helper renders the same template BootstrapSystem uses;
	// Description/AcceptanceCriteria are not persisted on the task model (they
	// live in the ticket files), so they render empty here — the point of the
	// resume refresh is the live Status/UpdatedAt, not a full rebuild.
	if task.WorktreePath != "" && tm.templateManager != nil {
		initiativeName, stage, gate := tm.resolveInitiativeContext(task.Initiative)
		refreshCfg := BootstrapConfig{
			TaskID:       task.ID,
			Title:        task.Title,
			Status:       string(task.Status),
			TicketPath:   task.TicketPath,
			Branch:       task.Branch,
			WorktreePath: task.WorktreePath,
			Initiative:   initiativeName,
			Stage:        stage,
			Gate:         gate,
			// Seed the bounded 1-hop graph neighbourhood (decision D9). Empty when
			// the ticket has no links, so a no-link ticket renders unchanged.
			Siblings: tm.neighborhoodSiblings(task.ID),
		}
		if err := generateTaskContext(task.WorktreePath, tm.templateManager, refreshCfg); err != nil {
			// Non-fatal: a stale worktree Tier-0 file shouldn't block resume.
			fmt.Fprintf(os.Stderr, "Warning: failed to refresh worktree task-context.md: %v\n", err)
		} else if tm.eventLogger != nil {
			tm.eventLogger.Log("config.task_context_synced", map[string]interface{}{
				"task_id": taskID,
				"trigger": "resume",
			})
		}
		// Re-provision the per-worktree Serena config on refresh (#202).
		tm.provisionSerena(task.WorktreePath)
	}

	return task, nil
}

// Archive generates handoff.md, moves ticket to _archived/, removes the
// worktree (unless opts.KeepWorktree), optionally prunes the branch, and updates
// status. Worktree removal honours opts.Force — without it a dirty/unpushed
// worktree is refused and left in place (its work preserved), #207.
func (tm *TaskManager) Archive(taskID string, opts ArchiveOptions) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Archiving is not idempotent by construction: a second archive would
	// resolve the already-_archived ticket dir, compute its sub-path relative to
	// tickets/ as "_archived/<sub>", and re-root it under archivedDir — producing
	// tickets/_archived/_archived/<sub> and a TicketPath that a later Unarchive
	// then mis-restores (leaving an active task inside _archived/). Refuse the
	// no-op up front so the ticket dir is only ever archived once (#159).
	if task.Status == models.TaskStatusArchived {
		return fmt.Errorf("task %s is already archived", taskID)
	}

	// Generate handoff.md from template
	handoffData := map[string]interface{}{
		"TaskID":     taskID,
		"Title":      task.Title,
		"Status":     task.Status,
		"Priority":   task.Priority,
		"Owner":      task.Owner,
		"Created":    task.Created.Format(time.RFC3339),
		"Updated":    task.Updated.Format(time.RFC3339),
		"ArchivedAt": time.Now().UTC().Format(time.RFC3339),
	}

	handoffContent, err := tm.templateManager.RenderBytes(TemplateTypeHandoff, handoffData)
	if err != nil {
		return fmt.Errorf("failed to render handoff template: %w", err)
	}

	// Resolve the ticket directory from the task model (nested layout)
	// rather than reconstructing a flat path — the latter pointed at a
	// directory that doesn't exist for any task created by the nested
	// CreateTask path.
	taskDir := tm.resolveTicketDir(task)
	handoffPath := filepath.Join(taskDir, "handoff.md")
	if err := os.WriteFile(handoffPath, handoffContent, 0o644); err != nil {
		return fmt.Errorf("failed to write handoff.md: %w", err)
	}

	// Move ticket to _archived/, mirroring whatever sub-path layout it had.
	// We compute the sub-path under tm.ticketsDir from the resolved taskDir
	// and re-root it under tm.archivedDir. For legacy flat tasks this
	// degenerates to archivedDir/<taskID>; for nested tasks it preserves
	// archivedDir/<platform>/<org>/<repo>/<TaskID-slug>.
	relSub, relErr := filepath.Rel(tm.ticketsDir, taskDir)
	if relErr != nil || strings.HasPrefix(relSub, "..") {
		// Defensive: if the ticket path isn't under tm.ticketsDir for
		// some reason, fall back to the bare taskID under archivedDir.
		relSub = taskID
	}
	archivedTaskDir := filepath.Join(tm.archivedDir, relSub)
	if err := os.MkdirAll(filepath.Dir(archivedTaskDir), 0o755); err != nil {
		return fmt.Errorf("failed to create archived directory: %w", err)
	}

	if err := os.Rename(taskDir, archivedTaskDir); err != nil {
		return fmt.Errorf("failed to move task to archived: %w", err)
	}

	// Remove worktree, unless the caller asked to keep it (#207). Removal is
	// non-fatal — a dirty/unpushed worktree is refused by the remover when
	// opts.Force is false, so its work is preserved and the ticket still
	// archives; we warn actionably instead of discarding.
	keptWorktree := opts.KeepWorktree
	if !opts.KeepWorktree && tm.worktreeRemover != nil && task.WorktreePath != "" {
		removedPath := task.WorktreePath
		if err := tm.worktreeRemover.RemoveWorktree(removedPath, opts.Force); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree left in place (%v). Commit/push and `adb task cleanup %s`, or re-archive with --force.\n", err, taskID)
			keptWorktree = true
		} else {
			if tm.eventLogger != nil {
				// Emit worktree.removed from the archive path too (not just
				// cleanup) so the Active metric balances (#206).
				tm.eventLogger.Log("worktree.removed", map[string]interface{}{
					"task_id": taskID,
					"path":    removedPath,
				})
			}
			// Opt-in orphan-branch cleanup once the worktree is gone (#207).
			if opts.PruneBranch && task.Repo != "" && task.Branch != "" {
				if err := tm.worktreeRemover.RemoveBranch(task.Repo, task.Branch); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to prune branch %q: %v\n", task.Branch, err)
				}
			}
		}
	}

	// Update task status to archived. Clear the worktree path only when the
	// worktree was actually removed — a kept (or refused) worktree stays linked.
	task.Status = models.TaskStatusArchived
	task.TicketPath = archivedTaskDir
	if !keptWorktree {
		task.WorktreePath = ""
	}
	task.UpdateTimestamp()
	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Log event
	if tm.eventLogger != nil {
		tm.eventLogger.Log("task.archived", map[string]interface{}{
			"task_id":      taskID,
			"archived_at":  time.Now().UTC(),
			"archived_dir": archivedTaskDir,
		})
	}

	return nil
}

// Unarchive moves a task back from _archived/ to active tickets
func (tm *TaskManager) Unarchive(taskID string) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Check if archived
	if task.Status != models.TaskStatusArchived {
		return fmt.Errorf("task %s is not archived", taskID)
	}

	// Resolve the archived directory from the task model. Archive() set
	// task.TicketPath to the archived path, so resolveTicketDir returns it
	// directly — no need to reconstruct a flat path.
	archivedTaskDir := tm.resolveTicketDir(task)

	// Mirror the archived sub-path back under tm.ticketsDir so the ticket
	// lands in the same nested location it came from.
	relSub, relErr := filepath.Rel(tm.archivedDir, archivedTaskDir)
	if relErr != nil || strings.HasPrefix(relSub, "..") {
		// Defensive fallback for legacy archived paths that weren't under
		// tm.archivedDir for some reason.
		relSub = taskID
	}
	activeTaskDir := filepath.Join(tm.ticketsDir, relSub)
	if err := os.MkdirAll(filepath.Dir(activeTaskDir), 0o755); err != nil {
		return fmt.Errorf("failed to create active ticket parent directory: %w", err)
	}

	if err := os.Rename(archivedTaskDir, activeTaskDir); err != nil {
		return fmt.Errorf("failed to move task from archived: %w", err)
	}

	// Update task status to backlog
	task.Status = models.TaskStatusBacklog
	task.TicketPath = activeTaskDir
	task.UpdateTimestamp()
	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Log event
	if tm.eventLogger != nil {
		tm.eventLogger.Log("task.unarchived", map[string]interface{}{
			"task_id":       taskID,
			"unarchived_at": time.Now().UTC(),
		})
	}

	return nil
}

// UpdateStatus updates the status of a task
func (tm *TaskManager) UpdateStatus(taskID string, newStatus models.TaskStatus) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	oldStatus := task.Status
	task.Status = newStatus
	task.UpdateTimestamp()

	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Log event
	if tm.eventLogger != nil {
		tm.eventLogger.Log("task.status_changed", map[string]interface{}{
			"task_id":    taskID,
			"old_status": oldStatus,
			"new_status": newStatus,
		})
	}

	return nil
}

// UpdatePriority updates the priority of a task
func (tm *TaskManager) UpdatePriority(taskID string, newPriority models.Priority) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	oldPriority := task.Priority
	task.Priority = newPriority
	task.UpdateTimestamp()

	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		return fmt.Errorf("failed to update task priority: %w", err)
	}

	// Log event
	if tm.eventLogger != nil {
		tm.eventLogger.Log("task.priority_changed", map[string]interface{}{
			"task_id":      taskID,
			"old_priority": oldPriority,
			"new_priority": newPriority,
		})
	}

	return nil
}

// SetInitiative associates a task with an initiative (metadata only — the
// physical ticket/worktree path layout is untouched). It validates the
// initiative exists via the resolver (when wired), persists Task.Initiative,
// and — when the task has a worktree — regenerates .claude/rules/task-context.md
// so the initiative's Stage shows up immediately for the running agent. Passing
// an empty initiativeID clears the association.
func (tm *TaskManager) SetInitiative(taskID, initiativeID string) (*models.Task, error) {
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to load task: %w", err)
	}

	if initiativeID != "" && tm.initiativeResolver != nil {
		if _, err := tm.initiativeResolver.GetInitiative(initiativeID); err != nil {
			return nil, fmt.Errorf("initiative %q: %w", initiativeID, err)
		}
	}

	task.Initiative = initiativeID
	task.UpdateTimestamp()
	if err := tm.backlogStore.UpdateTask(*task); err != nil {
		return nil, fmt.Errorf("failed to update task initiative: %w", err)
	}

	// Refresh the worktree Tier-0 file so the Stage line reflects the new
	// association right away. Non-fatal: a stale context file must not fail the
	// association write itself.
	if task.WorktreePath != "" && tm.templateManager != nil {
		initiativeName, stage, gate := tm.resolveInitiativeContext(task.Initiative)
		refreshCfg := BootstrapConfig{
			TaskID:       task.ID,
			Title:        task.Title,
			Status:       string(task.Status),
			TicketPath:   task.TicketPath,
			Branch:       task.Branch,
			WorktreePath: task.WorktreePath,
			Initiative:   initiativeName,
			Stage:        stage,
			Gate:         gate,
			// Seed the bounded 1-hop graph neighbourhood (decision D9).
			Siblings: tm.neighborhoodSiblings(task.ID),
		}
		if err := generateTaskContext(task.WorktreePath, tm.templateManager, refreshCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to refresh worktree task-context.md: %v\n", err)
		}
		// Re-provision the per-worktree Serena config on refresh (#202).
		tm.provisionSerena(task.WorktreePath)
	}

	return task, nil
}

// Cleanup removes only the worktree for a task. When force is false the
// underlying remover refuses to discard a dirty/unpushed worktree (#207).
func (tm *TaskManager) Cleanup(taskID string, force bool) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Remove worktree if it exists
	if tm.worktreeRemover != nil && task.WorktreePath != "" {
		// Capture the path before we clear it — worktree.removed documents a
		// `path` key (docs/claude/subsystems.md) and clearing it first would
		// emit an empty string.
		removedPath := task.WorktreePath
		if err := tm.worktreeRemover.RemoveWorktree(removedPath, force); err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}

		// Update task to clear worktree path
		task.WorktreePath = ""
		task.UpdateTimestamp()
		if err := tm.backlogStore.UpdateTask(*task); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}

		// Log event
		if tm.eventLogger != nil {
			tm.eventLogger.Log("worktree.removed", map[string]interface{}{
				"task_id": taskID,
				"path":    removedPath,
			})
		}
	}

	return nil
}

// BulkResult records the outcome of a single task in a bulk operation.
type BulkResult struct {
	TaskID    string
	OldStatus models.TaskStatus
	NewStatus models.TaskStatus
	Err       error
}

// StartAll promotes every task in the backlog to in_progress.
//
// Only tasks currently in the backlog status are affected; tasks that are
// already in_progress, blocked, review, done, or archived are skipped so the
// operation is idempotent and never resurrects archived work. Each task is
// processed independently: one failure does not abort the rest. The returned
// slice contains one entry per task that was eligible (i.e. in the backlog),
// in backlog order.
// Start promotes a single backlog task to in_progress WITHOUT launching a
// session — the singular counterpart to StartAll (#210). Idempotent: a task
// that is not in backlog is left unchanged (no error), matching start-all's
// per-task semantics.
func (tm *TaskManager) Start(taskID string) error {
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}
	if task.Status != models.TaskStatusBacklog {
		return nil
	}
	return tm.UpdateStatus(taskID, models.TaskStatusInProgress)
}

func (tm *TaskManager) StartAll() ([]BulkResult, error) {
	backlog, err := tm.backlogStore.Load()
	if err != nil {
		return nil, fmt.Errorf("loading backlog: %w", err)
	}

	var results []BulkResult
	for _, task := range backlog.Tasks {
		if task.Status != models.TaskStatusBacklog {
			continue
		}
		res := BulkResult{TaskID: task.ID, OldStatus: task.Status, NewStatus: models.TaskStatusInProgress}
		if err := tm.UpdateStatus(task.ID, models.TaskStatusInProgress); err != nil {
			res.Err = err
		}
		results = append(results, res)
	}
	return results, nil
}

// CloseAll marks every active task as done.
//
// Active means in_progress, blocked, or review (see Task.IsActive). Backlog,
// done, and archived tasks are left untouched. Closing only flips the status to
// done — it does not archive or remove worktrees, so the ticket history stays
// intact and reversible via `adb task update --status`. Each task is processed
// independently; one failure does not abort the rest.
func (tm *TaskManager) CloseAll() ([]BulkResult, error) {
	backlog, err := tm.backlogStore.Load()
	if err != nil {
		return nil, fmt.Errorf("loading backlog: %w", err)
	}

	var results []BulkResult
	for _, task := range backlog.Tasks {
		if !task.IsActive() {
			continue
		}
		res := BulkResult{TaskID: task.ID, OldStatus: task.Status, NewStatus: models.TaskStatusDone}
		if err := tm.UpdateStatus(task.ID, models.TaskStatusDone); err != nil {
			res.Err = err
		}
		results = append(results, res)
	}
	return results, nil
}

// Delete performs full removal of a task (worktree, ticket directory, and backlog entry)
func (tm *TaskManager) Delete(taskID string) error {
	// Load task
	task, err := tm.backlogStore.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to load task: %w", err)
	}

	// Remove worktree (force — delete is explicitly destructive).
	if tm.worktreeRemover != nil && task.WorktreePath != "" {
		removedPath := task.WorktreePath
		if err := tm.worktreeRemover.RemoveWorktree(removedPath, true); err != nil {
			// Non-fatal: log but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree: %v\n", err)
		} else if tm.eventLogger != nil {
			tm.eventLogger.Log("worktree.removed", map[string]interface{}{
				"task_id": taskID,
				"path":    removedPath,
			})
		}
	}

	// Remove ticket directory. We try the path stored on the task first
	// (correct for both nested and legacy layouts), and ALSO check the
	// legacy flat fallback in case task.TicketPath was never set.
	primaryTaskDir := tm.resolveTicketDir(task)
	legacyTaskDir := filepath.Join(tm.ticketsDir, taskID)
	legacyArchivedDir := filepath.Join(tm.archivedDir, taskID)

	candidates := []string{primaryTaskDir}
	if legacyTaskDir != primaryTaskDir {
		candidates = append(candidates, legacyTaskDir)
	}
	candidates = append(candidates, legacyArchivedDir)

	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("failed to remove task directory %s: %w", dir, err)
			}
		}
	}

	// Remove from backlog
	if err := tm.backlogStore.RemoveTask(taskID); err != nil {
		return fmt.Errorf("failed to remove task from backlog: %w", err)
	}

	// Log event
	if tm.eventLogger != nil {
		tm.eventLogger.Log("task.deleted", map[string]interface{}{
			"task_id":    taskID,
			"deleted_at": time.Now().UTC(),
		})
	}

	return nil
}

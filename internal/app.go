package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// App is the application's dependency injection container
// It wires all subsystems together using the adapter pattern to prevent circular imports
type App struct {
	// ===== Configuration =====
	BasePath      string
	ConfigManager core.ConfigurationManager
	MergedConfig  *models.MergedConfig

	// ===== Storage =====
	BacklogManager       storage.BacklogManager
	ContextManager       storage.ContextManager
	SessionStoreManager  storage.SessionStoreManager
	CommunicationManager storage.CommunicationManager

	// ===== Core Services =====
	TaskIDGenerator    core.TaskIDGenerator
	TemplateManager    core.TemplateManager
	TaskManager        *core.TaskManager
	AIContextGenerator core.AIContextGenerator
	StageManager       core.StageManager
	GraphManager       core.GraphManager
	RuleEngine         core.RuleEngine
	IngestManager      core.IngestManager
	CatalogBuilder     core.CatalogService
	DriftChecker       core.DriftChecker
	ADRManager         core.ADRManager
	DebtManager        core.DebtManager
	SLOManager         core.SLOManager
	SecurityAuditor    core.SecurityAuditor
	CRMManager         core.CRMManager
	MetricStore        *storage.FileMetricStore

	// ===== Integration =====
	GitWorktreeManager integration.GitWorktreeManager

	// ===== Observability =====
	EventLog          *observability.EventLog
	GovernanceLog     *observability.EventLog
	MetricsCalculator *observability.MetricsCalculator
	AlertEvaluator    *observability.AlertEvaluator
}

// stateDirName is the single directory under the workspace root where adb keeps
// all of its private bookkeeping (#186). It is defined once in the leaf
// statedir package so every layer (this package, the CLI, core, hooks) shares
// one convention without an import cycle.
const stateDirName = statedir.Name

// StatePath returns the absolute path of an adb-owned state file named `name`,
// under the workspace's .adb/ state directory: <BasePath>/.adb/<name>. It is the
// single seam every internal state path routes through (#186) — a future state
// file gets .adb/ placement for free by constructing its path here (or via
// statedir.Path from a lower layer) rather than joining `.adb_<thing>` onto
// BasePath. `name` is the bare basename inside .adb/ (e.g. "scheduler.log",
// "task_counter", "events.jsonl"), with no leading dot.
func (app *App) StatePath(name string) string {
	return statedir.Path(app.BasePath, name)
}

// ensureStateDir creates the .adb/ state directory if absent so state writers
// have a home before the first write. Best-effort and non-fatal, mirroring
// NewEventLog's tolerance of an un-writable log at init: state writers already
// Ensure their own parent, so a genuine permission problem still surfaces at
// the real write rather than bricking read-only commands (e.g. `adb task
// status`). statedir.Ensure is a no-op when the directory already exists, so an
// existing .adb/ (and its claude-user.md) is never disturbed.
func (app *App) ensureStateDir() error {
	return statedir.Ensure(app.BasePath)
}

// Adapters bridge core interfaces to real implementations
// This prevents circular imports: core defines interfaces, implementations live elsewhere

// backlogStoreAdapter adapts storage.BacklogManager to core.BacklogStore
type backlogStoreAdapter struct {
	manager storage.BacklogManager
}

func (a *backlogStoreAdapter) Load() (*models.Backlog, error) {
	return a.manager.Load()
}

func (a *backlogStoreAdapter) Save(backlog *models.Backlog) error {
	return a.manager.Save(backlog)
}

func (a *backlogStoreAdapter) AddTask(task models.Task) error {
	return a.manager.AddTask(task)
}

func (a *backlogStoreAdapter) UpdateTask(task models.Task) error {
	return a.manager.UpdateTask(task)
}

func (a *backlogStoreAdapter) GetTask(id string) (*models.Task, error) {
	return a.manager.GetTask(id)
}

func (a *backlogStoreAdapter) RemoveTask(id string) error {
	return a.manager.RemoveTask(id)
}

// contextStoreAdapter adapts storage.ContextManager to core.ContextStore
type contextStoreAdapter struct {
	manager storage.ContextManager
}

func (a *contextStoreAdapter) ReadContext(taskID string) (string, error) {
	return a.manager.ReadContext(taskID)
}

func (a *contextStoreAdapter) WriteContext(taskID string, content string) error {
	return a.manager.WriteContext(taskID, content)
}

func (a *contextStoreAdapter) AppendContext(taskID string, section string) error {
	return a.manager.AppendContext(taskID, section)
}

func (a *contextStoreAdapter) ReadNotes(taskID string) (string, error) {
	return a.manager.ReadNotes(taskID)
}

func (a *contextStoreAdapter) WriteNotes(taskID string, content string) error {
	return a.manager.WriteNotes(taskID, content)
}

// worktreeCreatorAdapter adapts integration.GitWorktreeManager to core.WorktreeCreator
type worktreeCreatorAdapter struct {
	manager integration.GitWorktreeManager
	// basePath is the workspace root; baseBranch is the branch new worktrees
	// are cut from, resolved from RepoConfig.base_branch (default "main") so a
	// repo whose default branch is master/develop lands on the right base
	// instead of a hardcoded "main" (#206).
	basePath   string
	baseBranch string
}

// CreateWorktree threads the caller-supplied branch name and worktree path
// straight through to the integration layer (CreateWorktreeAt) so the
// nested correlation layout chosen by the TaskManager actually lands on
// disk. The previous implementation discarded both arguments and called
// the legacy CreateWorktree which hardcoded `task/<taskID>` and
// `basePath/work/<taskID>`, which is exactly what this fix is undoing.
func (a *worktreeCreatorAdapter) CreateWorktree(taskID, branchName, worktreePath, repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("repoPath is required for worktree creation")
	}
	baseBranch := a.baseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// If the TaskManager didn't supply explicit values (defensive — the
	// nested code path always does), fall through to the legacy default
	// so older call sites and integration tests keep working.
	if branchName == "" || worktreePath == "" {
		_, err := a.manager.CreateWorktree(taskID, repoPath, baseBranch)
		return err
	}

	_, err := a.manager.CreateWorktreeAt(taskID, repoPath, baseBranch, branchName, worktreePath)
	return err
}

// NormalizeRepoPath delegates to the underlying GitWorktreeManager so the
// TaskManager can canonicalise --repo arguments without depending on the
// integration package directly (avoiding a core->integration import cycle).
func (a *worktreeCreatorAdapter) NormalizeRepoPath(repoPath string) (string, error) {
	return a.manager.NormalizeRepoPath(repoPath)
}

func (a *worktreeCreatorAdapter) BranchExists(repoPath, branch string) (bool, error) {
	return a.manager.BranchExists(repoPath, branch)
}

// resolveWorktreesDir returns the directory under which per-task worktrees are
// created. It honours RepoConfig.worktree_base_path — absolute paths are used
// verbatim, relative ones are joined under the workspace basePath — and falls
// back to the historical "<basePath>/work" when unset (#206).
func resolveWorktreesDir(basePath string, repo *models.RepoConfig) string {
	if repo != nil && repo.WorktreeBasePath != "" {
		if filepath.IsAbs(repo.WorktreeBasePath) {
			return repo.WorktreeBasePath
		}
		return filepath.Join(basePath, repo.WorktreeBasePath)
	}
	return filepath.Join(basePath, "work")
}

// resolveWorktreeBaseBranch returns the branch new worktrees are cut from. It
// honours RepoConfig.base_branch and falls back to the historical "main" when
// unset, so a repo whose default branch is master/develop can be configured
// without patching code (#206).
func resolveWorktreeBaseBranch(repo *models.RepoConfig) string {
	if repo != nil && repo.BaseBranch != "" {
		return repo.BaseBranch
	}
	return "main"
}

// worktreeRemoverAdapter adapts integration.GitWorktreeManager to core.WorktreeRemover
type worktreeRemoverAdapter struct {
	manager integration.GitWorktreeManager
}

func (a *worktreeRemoverAdapter) RemoveWorktree(worktreePath string, force bool) error {
	return a.manager.RemoveWorktree(worktreePath, force)
}

func (a *worktreeRemoverAdapter) RemoveBranch(repoPath, branch string) error {
	return a.manager.RemoveBranch(repoPath, branch)
}

// eventLoggerAdapter adapts observability.EventLog to core.EventLogger
type eventLoggerAdapter struct {
	log *observability.EventLog
}

func (a *eventLoggerAdapter) Log(eventType string, data map[string]interface{}) {
	a.log.Log(observability.EventType(eventType), data)
}

// sessionCapturerAdapter adapts storage.SessionStoreManager to core.SessionCapturer
type sessionCapturerAdapter struct {
	manager storage.SessionStoreManager
}

func (a *sessionCapturerAdapter) CaptureSession(taskID, sessionID string, data map[string]interface{}) error {
	// This is a simplified implementation - in a real system, we'd convert the data map
	// to a proper CapturedSession struct. For now, we'll return an error if not implemented.
	return fmt.Errorf("session capture not fully implemented in adapter")
}

// stageStoreAdapter adapts storage.FileStageStore to core.StageStore. The signatures
// already line up (both speak pkg/models types), so it is a thin bridge; it exists to
// keep the wiring uniform with the other adapters and to insulate core from any future
// drift in the storage-side method set.
type stageStoreAdapter struct {
	store *storage.FileStageStore
}

func (a *stageStoreAdapter) CreateOrganization(org models.Organization) error {
	return a.store.CreateOrganization(org)
}

func (a *stageStoreAdapter) GetOrganization(id string) (models.Organization, bool, error) {
	return a.store.GetOrganization(id)
}

func (a *stageStoreAdapter) ListOrganizations() ([]models.Organization, error) {
	return a.store.ListOrganizations()
}

func (a *stageStoreAdapter) CreateInitiative(init models.Initiative) error {
	return a.store.CreateInitiative(init)
}

func (a *stageStoreAdapter) GetInitiative(id string) (models.Initiative, bool, error) {
	return a.store.GetInitiative(id)
}

func (a *stageStoreAdapter) ListInitiatives() ([]models.Initiative, error) {
	return a.store.ListInitiatives()
}

func (a *stageStoreAdapter) UpdateInitiative(init models.Initiative) error {
	return a.store.UpdateInitiative(init)
}

// graphSourceAdapter yields the graph's nodes from the entity stores that
// declare typed links: the backlog (tasks), the initiative registry, and the
// ingested-node registry (nodes landed by the D8 ingestion pipeline). It bridges
// core.GraphSource to the concrete storage layer so core stays ignorant of
// storage. The frontmatter links on each entity are the source of truth
// (decision D6); the GraphManager derives its index from what this yields.
type graphSourceAdapter struct {
	backlog storage.BacklogManager
	stage   *storage.FileStageStore
	nodes   *storage.FileNodeStore
	metrics *storage.FileMetricStore
	adrs    *storage.FileADRStore
	debt    *storage.FileDebtStore
}

func (a *graphSourceAdapter) GraphNodes() ([]core.GraphNode, error) {
	backlog, err := a.backlog.Load()
	if err != nil {
		return nil, fmt.Errorf("load backlog for graph: %w", err)
	}
	inits, err := a.stage.ListInitiatives()
	if err != nil {
		return nil, fmt.Errorf("list initiatives for graph: %w", err)
	}
	ingested, err := a.nodes.List()
	if err != nil {
		return nil, fmt.Errorf("list ingested nodes for graph: %w", err)
	}
	metrics, err := a.metrics.List()
	if err != nil {
		return nil, fmt.Errorf("list metrics for graph: %w", err)
	}
	adrs, err := a.adrs.List()
	if err != nil {
		return nil, fmt.Errorf("list ADRs for graph: %w", err)
	}
	debt, err := a.debt.List()
	if err != nil {
		return nil, fmt.Errorf("list tech debt for graph: %w", err)
	}
	nodes := make([]core.GraphNode, 0, len(backlog.Tasks)+len(inits)+len(ingested)+len(metrics)+len(adrs)+len(debt))
	for _, t := range backlog.Tasks {
		nodes = append(nodes, core.GraphNode{ID: t.ID, Links: t.Links})
	}
	for _, in := range inits {
		nodes = append(nodes, core.GraphNode{ID: in.ID, Links: in.Links})
	}
	for _, n := range ingested {
		nodes = append(nodes, core.GraphNode{ID: n.ID, Links: n.Links})
	}
	// A metric node is reachable via the graph through a part_of edge toward its
	// initiative (decision D11: metrics are provenance-carrying graph nodes).
	for _, m := range metrics {
		nodes = append(nodes, core.GraphNode{
			ID:    m.GraphID(),
			Links: []models.Link{{Type: models.EdgePartOf, Target: m.Initiative}},
		})
	}
	// An ADR is an adr:NNNN node carrying whatever typed links it declares (e.g. a
	// relates_to toward the ticket/initiative it decides for) — #128 step 16.
	for _, adr := range adrs {
		nodes = append(nodes, core.GraphNode{ID: adr.GraphID(), Links: adr.Links})
	}
	// A tech-debt item is a debt:DEBT-NNNN node carrying whatever typed links it
	// declares (e.g. a relates_to toward the ticket the debt was incurred in),
	// the same way an ADR participates as adr:NNNN. Before TASK-00039 the debt
	// registry was the one governance registry nothing read: not a graph node,
	// absent from the catalog. That asymmetry is what made it look like a second
	// task-like concept rather than a registry earning its place.
	for _, d := range debt {
		nodes = append(nodes, core.GraphNode{ID: d.GraphID(), Links: d.Links})
	}
	return nodes, nil
}

// catalogSourceAdapter bridges core.CatalogSource to the concrete entity
// registries so the CatalogBuilder can inventory every entity without core
// importing storage. It reads the same stores the graph does — the backlog
// (tickets), the org/initiative registries, the ingested-node registry, and the
// metric registry — so the catalog and the graph always agree on membership.
type catalogSourceAdapter struct {
	backlog storage.BacklogManager
	stage   *storage.FileStageStore
	nodes   *storage.FileNodeStore
	metrics *storage.FileMetricStore
	adrs    *storage.FileADRStore
	debt    *storage.FileDebtStore
}

func (a *catalogSourceAdapter) Organizations() ([]models.Organization, error) {
	return a.stage.ListOrganizations()
}

func (a *catalogSourceAdapter) Initiatives() ([]models.Initiative, error) {
	return a.stage.ListInitiatives()
}

func (a *catalogSourceAdapter) Tickets() ([]models.Task, error) {
	backlog, err := a.backlog.Load()
	if err != nil {
		return nil, fmt.Errorf("load backlog for catalog: %w", err)
	}
	return backlog.Tasks, nil
}

func (a *catalogSourceAdapter) IngestedNodes() ([]models.IngestedNode, error) {
	return a.nodes.List()
}

func (a *catalogSourceAdapter) Metrics() ([]models.Metric, error) {
	return a.metrics.List()
}

func (a *catalogSourceAdapter) ADRs() ([]models.ADR, error) {
	return a.adrs.List()
}

func (a *catalogSourceAdapter) Debt() ([]models.DebtItem, error) {
	return a.debt.List()
}

// metricSourceAdapter bridges core.MetricSource to the metric registry so a
// stage gate's numeric-threshold items can read a recorded metric's value.
type metricSourceAdapter struct {
	store *storage.FileMetricStore
}

func (a *metricSourceAdapter) Metric(initiative, name string) (float64, bool, error) {
	m, found, err := a.store.Get(initiative, name)
	if err != nil || !found {
		return 0, found, err
	}
	return m.Value, true, nil
}

// edgeWriterAdapter bridges core.EdgeWriter to the two entity stores that carry
// typed frontmatter links today: the backlog (tasks) and the initiative
// registry. A rule's edge output declares a Link on its source entity — the
// graph's source of truth (decision D6) — after which the derived index can be
// rebuilt. Adding an edge is idempotent: an identical Type+Target already present
// is a no-op, so a recurring rule never accretes duplicate edges.
type edgeWriterAdapter struct {
	backlog storage.BacklogManager
	stage   *storage.FileStageStore
	nodes   *storage.FileNodeStore
}

func (a *edgeWriterAdapter) AddEdge(from string, link models.Link) error {
	// A task first: GetTask errors when absent, so a non-nil task means "found".
	if task, err := a.backlog.GetTask(from); err == nil && task != nil {
		if linkExists(task.Links, link) {
			return nil
		}
		task.Links = append(task.Links, link)
		return a.backlog.UpdateTask(*task)
	}
	// Then an initiative.
	init, found, err := a.stage.GetInitiative(from)
	if err != nil {
		return fmt.Errorf("look up initiative %q: %w", from, err)
	}
	if found {
		if linkExists(init.Links, link) {
			return nil
		}
		init.Links = append(init.Links, link)
		return a.stage.UpdateInitiative(init)
	}
	// Finally an ingested node. Ingested nodes carry Links and participate in the
	// graph exactly like a task or initiative (graphSourceAdapter reads their
	// Links), so an edge FROM an ingested node must be landable — otherwise such
	// an edge proposal can never be accepted (it re-queues forever) (#174).
	if a.nodes != nil {
		node, nFound, nErr := a.nodes.Get(from)
		if nErr != nil {
			return fmt.Errorf("look up ingested node %q: %w", from, nErr)
		}
		if nFound {
			if linkExists(node.Links, link) {
				return nil
			}
			node.Links = append(node.Links, link)
			return a.nodes.Put(node)
		}
	}
	return fmt.Errorf("cannot add edge: no task, initiative, or ingested node %q", from)
}

// linkExists reports whether links already contains an edge with the same type
// and target (the identity that makes edge writes idempotent).
func linkExists(links []models.Link, l models.Link) bool {
	for _, existing := range links {
		if existing.Type == l.Type && existing.Target == l.Target {
			return true
		}
	}
	return false
}

// NewApp creates and wires all application subsystems in dependency order
// basePath is the root directory for the workspace (e.g., "." or "/path/to/workspace")
// AppOptions pins the inputs NewApp would otherwise resolve from the user's HOME
// directory and the process environment.
//
// WHY THIS EXISTS. `basePath` only isolates the REPO config tier
// (<basePath>/.taskrc). Two more inputs are ambient no matter what basePath says:
//
//   - the GLOBAL config tier is $HOME/.taskconfig (NewViperConfigManager fills it
//     in from os.UserHomeDir when the path is empty);
//   - the terminal-state file is $HOME/.adb_terminal_state.json;
//   - and the active org tier comes from $ADB_ORG.
//
// So `NewApp(t.TempDir())` reads — and in the terminal-state case WRITES — real
// user data. That is not hypothetical: it is what made two tests fail against a
// developer's real config (hooks enabled, an Ollama embedder) while passing in CI,
// and what accreted a pile of dead test entries into a real home directory.
//
// Pinning these here rather than via t.Setenv matters for a second reason: the
// env-var approach is incompatible with t.Parallel by construction, so it makes
// hermetic tests serial. An explicitly-constructed App has no such constraint.
//
// The zero value reproduces the historical behaviour exactly, so NewApp is
// unchanged for the CLI.
type AppOptions struct {
	// GlobalConfigPath pins the global config tier. Empty means $HOME/.taskconfig
	// unless Isolated is set.
	GlobalConfigPath string
	// Org pins the active org tier, suppressing both $ADB_ORG and the repo config's
	// `org:` field. Empty means "not pinned".
	Org string
	// Isolated resolves every unset path above to a location UNDER basePath instead
	// of $HOME, and ignores $ADB_ORG. An App built this way touches nothing outside
	// basePath, which is what a test wants.
	//
	// It does NOT suppress the repo config's `org:` field: that lives inside
	// basePath, so it is part of the workspace the caller handed over, not ambient
	// state. Isolation is about the environment and $HOME, not about ignoring the
	// workspace's own description of itself.
	Isolated bool
}

// resolve fills in the effective paths for basePath, applying the Isolated rules.
func (o AppOptions) resolve(basePath string) AppOptions {
	if !o.Isolated {
		return o
	}
	if o.GlobalConfigPath == "" {
		// The workspace root is where a real global config lives anyway (base-path
		// resolution walks up for .taskconfig), so an isolated App can still
		// exercise the global tier — a test just writes the file.
		o.GlobalConfigPath = filepath.Join(basePath, ".taskconfig")
	}
	return o
}

// configManagerOptions projects the App-level options onto the config manager's
// org-resolution knobs.
func (o AppOptions) configManagerOptions() core.ConfigManagerOptions {
	return core.ConfigManagerOptions{Org: o.Org, IgnoreOrgEnv: o.Isolated}
}

// NewApp builds an App rooted at basePath with the historical, environment-aware
// defaults: the global config tier is $HOME/.taskconfig, the terminal-state file
// is $HOME/.adb_terminal_state.json, and the active org comes from $ADB_ORG.
//
// Tests should prefer NewAppIsolated, which touches nothing outside basePath.
func NewApp(basePath string) (*App, error) {
	return NewAppWithOptions(basePath, AppOptions{})
}

// NewAppIsolated builds an App that resolves NOTHING from $HOME or the process
// environment — every path lands under basePath and there is no org tier unless
// one is written into the workspace. This is the constructor tests should use: it
// is hermetic by construction rather than by remembering to set three env vars,
// and unlike t.Setenv it does not forbid t.Parallel.
func NewAppIsolated(basePath string) (*App, error) {
	return NewAppWithOptions(basePath, AppOptions{Isolated: true})
}

// NewAppWithOptions builds an App with explicit control over the ambient inputs
// described on AppOptions.
func NewAppWithOptions(basePath string, opts AppOptions) (*App, error) {
	if basePath == "" {
		basePath = "."
	}
	opts = opts.resolve(basePath)

	app := &App{
		BasePath: basePath,
	}

	// Ensure the .adb/ state directory exists before any subsystem writes state
	// into it (#186). Best-effort: a failure here is not fatal (state writers
	// MkdirAll their own parent), so a read-only command still runs.
	if err := app.ensureStateDir(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create %s state dir: %v\n", stateDirName, err)
	}

	// One-shot migration of any legacy root-level state into .adb/ (#186), run
	// before the subsystems construct so a workspace upgraded to this version is
	// not "reset". Non-fatal: a migration failure is surfaced on stderr but does
	// not abort the command (the relocation is a convenience, not a precondition
	// — writers still MkdirAll and open their own paths).
	if err := migrateStateToADB(basePath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not migrate legacy state into %s: %v\n", stateDirName, err)
	}

	// Delete state left behind by features that no longer exist (see
	// defunctStateFiles) — the counterpart to the relocation above, in the one body
	// all three constructors go through. It reports its own warnings to stderr and
	// cannot fail the command: a read-only filesystem must not break every command
	// over a leftover cache file.
	removeDefunctState(basePath, os.Stderr)

	// ===== Configuration =====
	// Load configuration from .taskconfig and .taskrc
	// An empty globalConfigPath means ~/.taskconfig (see AppOptions).
	globalConfigPath := opts.GlobalConfigPath
	repoConfigPath := filepath.Join(basePath, ".taskrc")
	app.ConfigManager = core.NewViperConfigManagerWithOptions(
		globalConfigPath, repoConfigPath, opts.configManagerOptions())

	config, err := app.ConfigManager.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	app.MergedConfig = config

	// ===== Storage =====
	// Backlog manager - stores tasks in backlog.yaml
	backlogPath := filepath.Join(basePath, "backlog.yaml")
	app.BacklogManager = storage.NewFileBacklogManager(backlogPath)

	// Context manager - manages task-specific context and notes
	ticketsDir := filepath.Join(basePath, "tickets")
	app.ContextManager = storage.NewFileContextManager(ticketsDir)

	// Session store manager - manages captured sessions
	sessionsDir := filepath.Join(basePath, "sessions")
	app.SessionStoreManager = storage.NewFileSessionStoreManager(sessionsDir)

	// Communication manager - stakeholder correspondence stored inside a ticket's
	// communications/ dir. Previously built-but-unwired (issue #121): now on App so
	// `adb comm` can log/list communications with a direction. It is a storage-level
	// interface used directly by the thin CLI (as SessionStoreManager is; the
	// contextStoreAdapter exists only to feed core, not the CLI). A resolver honours
	// the nested correlation layout so communications land inside the real ticket
	// dir (via the task's TicketPath, else a tickets/ walk) rather than a flat
	// tickets/<id>/ path.
	commMgr := storage.NewFileCommunicationManager(ticketsDir)
	commMgr.SetTicketDirResolver(func(taskID string) (string, bool) {
		if t, err := app.BacklogManager.GetTask(taskID); err == nil && t != nil && t.TicketPath != "" {
			return t.TicketPath, true
		}
		if dir, err := core.ResolveTicketDir(ticketsDir, taskID); err == nil && dir != "" {
			return dir, true
		}
		return "", false
	})
	app.CommunicationManager = commMgr

	// ===== Core Services =====
	// Task ID generator - generates sequential task IDs
	counterFile := app.StatePath(statedir.FileTaskCounter)
	prefix := "TASK"
	if app.MergedConfig != nil && app.MergedConfig.Global != nil && app.MergedConfig.Global.TaskIDPrefix != "" {
		prefix = app.MergedConfig.Global.TaskIDPrefix
	}
	app.TaskIDGenerator = core.NewFileTaskIDGenerator(counterFile, prefix)

	// Template manager - layered resolution: workspace-provisioned templates
	// ($ADB_HOME/templates/) win over the embedded defaults; both are parsed up
	// front so a broken template fails at startup, not mid-create (TASK-00031).
	templateManager, err := core.NewLayeredTemplateManager(filepath.Join(basePath, "templates"), templates.FS)
	if err != nil {
		return nil, fmt.Errorf("failed to create template manager: %w", err)
	}
	app.TemplateManager = templateManager

	// AI context generator - the rich 11-section CLAUDE.md builder with
	// .context_state.yaml section-hashing. Previously dead code (defined in
	// internal/core/aicontext.go but never constructed); wired here so
	// `adb context build` can reach it (see the ticket-bootstrap-context
	// ADR, part B). Reads the backlog through the shared BacklogManager.
	app.AIContextGenerator = core.NewAIContextGenerator(basePath, app.BacklogManager)

	// ===== Integration =====
	// Git worktree manager - manages git worktrees for task isolation
	app.GitWorktreeManager = integration.NewGitWorktreeManager(basePath)

	// ===== Observability =====
	// Event log - append-only JSONL event logging. Lives under .adb/ (#186).
	eventLogPath := app.StatePath(statedir.FileEventsLog)
	app.EventLog = observability.NewEventLog(eventLogPath)

	// Governance event log (#137 step 19) - a stream DISTINCT from dev telemetry
	// (decision D19): stage.advanced/stage.override land here as an auditable
	// governance record separate from the high-volume task/agent events in
	// events.jsonl. Surfaced by `adb governance`. Under .adb/ (#186).
	app.GovernanceLog = observability.NewEventLog(app.StatePath(statedir.FileGovernanceLog))

	// Metrics calculator - computes metrics on-demand from event log
	app.MetricsCalculator = observability.NewMetricsCalculator(app.EventLog)

	// Alert evaluator - evaluates alert conditions against thresholds resolved from
	// the layered config (Repo .taskrc > Org orgs/<id>/config.yaml > Global
	// .taskconfig > defaults), one flat custom_settings key per threshold:
	// alert_task_blocked_too_long, alert_task_stale, alert_review_too_long,
	// alert_backlog_too_large. This used to pass nil, which made the four
	// thresholds unreachable — AlertConfig.SetThreshold existed with no caller.
	//
	// Resolution is non-fatal by construction: an unparseable value keeps the
	// default and warns on STDERR (never stdout, which would corrupt a --json
	// pipeline). Config load happens here, at App init, so returning an error for a
	// bad value would kill every command in the workspace — the defect L600 §11
	// records for a dotted custom-settings key.
	var alertLookup observability.AlertSettingLookup
	if app.MergedConfig != nil {
		alertLookup = app.MergedConfig.SettingSource
	}
	alertConfig, alertWarnings := observability.ResolveAlertConfig(alertLookup)
	observability.WriteAlertConfigWarnings(os.Stderr, alertWarnings)
	app.AlertEvaluator = observability.NewAlertEvaluator(alertConfig, app.MetricsCalculator)

	// Stage manager - owns Organization/Initiative registries + the Stage dimension
	// and the founder-playbook StageGates. Registries are workspace-level metadata
	// (orgs/index.yaml, initiatives/index.yaml) — deliberately NOT part of the
	// tickets/<platform>/<org>/<repo> path layout. Wired after the EventLog so
	// AdvanceStage can emit stage.advanced / stage.override governance events.
	stageStore := storage.NewFileStageStore(basePath)

	// Metric store (decision D11) - product/PMF metric nodes (metrics/index.yaml).
	// Built before the StageManager so a gate's numeric-threshold items (the
	// MVP→Launch Sean-Ellis ≥40% + effort bar) can read recorded values, and
	// before the graph so metric nodes participate in it.
	app.MetricStore = storage.NewFileMetricStore(basePath)

	app.StageManager = core.NewStageManager(
		&stageStoreAdapter{store: stageStore},
		core.WithEvidenceRoot(basePath),
		core.WithEventLogger(&eventLoggerAdapter{log: app.EventLog}),
		// Governance stream (D19): the same stage events also land on the distinct
		// .governance.jsonl for compliance/audit, via `adb governance`.
		core.WithGovernanceLogger(&eventLoggerAdapter{log: app.GovernanceLog}),
		// Hybrid gates (D4): a gate's judgment items read a recorded adversarial
		// verdict (the devils-advocate agent's output saved under the initiative's
		// evidence dir). Absent verdict → the item degrades to "pending" and never
		// blocks, so behaviour is unchanged until a verdict is recorded.
		core.WithVerdictSource(core.NewRecordedVerdictSource()),
		// Numeric gates (D11): metric items read recorded metric nodes for their
		// threshold. Absent metric → the item is missing and blocks (the honest
		// numeric bar the file-presence check stood in for; closes #103).
		core.WithMetricSource(&metricSourceAdapter{store: app.MetricStore}),
	)

	// Ingested-node registry (decision D8) - typed nodes landed by the ingestion
	// pipeline (ingested/nodes.yaml). Constructed before the graph so it can be a
	// GraphSource contributor: an ingested node's links participate in the graph
	// like a task's or initiative's.
	nodeStore := storage.NewFileNodeStore(basePath)

	// ADR store (#128 step 16) - architecture decision records (adr/index.yaml +
	// docs/adr/NNNN-*.md). Built before the graph so ADR nodes (adr:NNNN) join it.
	adrStore := storage.NewFileADRStore(basePath)
	app.ADRManager = core.NewADRManager(adrStore)

	// Tech-debt registry (#128 step 16) - lightweight architecture-audit items
	// (debt/index.yaml), triageable by priority.
	debtStore := storage.NewFileDebtStore(basePath)
	app.DebtManager = core.NewDebtManager(debtStore)

	// SLO registry + security auditor (#131 step 17). The auditor is a
	// deterministic security/compliance posture check over the workspace files
	// (secret-scanning config, .env hygiene, pre-commit, SLOs) plus manual
	// attestation controls; compliance framework docs scaffold via `adb
	// compliance scaffold` (stateless over the embedded FS, wired in the CLI).
	app.SLOManager = core.NewSLOManager(storage.NewFileSLOStore(basePath))
	app.SecurityAuditor = core.NewSecurityAuditor(basePath)

	// CRM registry (#135 step 18) - MEDDPICC/Bowtie sales deals (crm/index.yaml).
	// GTM template packs scaffold via `adb gtm scaffold` (stateless over the
	// embedded FS, wired in the CLI like compliance).
	app.CRMManager = core.NewCRMManager(storage.NewFileCRMStore(basePath))

	// Graph manager - owns the generic typed edge graph (decision D6). Nodes come
	// from the entity stores that declare typed links (tasks in the backlog,
	// initiatives in the registry, ingested nodes, metrics, ADRs); the derived
	// index is a rebuildable cache persisted at graph/index.yaml (FileGraphStore
	// satisfies core.GraphIndexStore structurally, so it is wired without an adapter).
	app.GraphManager = core.NewGraphManager(
		&graphSourceAdapter{
			backlog: app.BacklogManager,
			stage:   stageStore,
			nodes:   nodeStore,
			metrics: app.MetricStore,
			adrs:    adrStore,
			debt:    debtStore,
		},
		storage.NewFileGraphStore(basePath),
	)

	// Catalog builder - the Backstage-style entity catalog (#128). Reads the same
	// registries the graph does (backlog, org/initiative registries, ingested
	// nodes, metrics, ADRs, tech debt) and annotates each entity with its graph degree from
	// the GraphManager. A generated, read-only inventory surfaced by `adb catalog`.
	catalogSource := &catalogSourceAdapter{
		backlog: app.BacklogManager,
		stage:   stageStore,
		nodes:   nodeStore,
		metrics: app.MetricStore,
		adrs:    adrStore,
		debt:    debtStore,
	}
	app.CatalogBuilder = core.NewCatalogBuilder(catalogSource, app.GraphManager)

	// Drift checker - the conformance-drift check (#128). Deterministic Go logic
	// that flags entities drifting from template (the copier/cruft manifest) or
	// catalog (dangling registry references) expectations. It is surfaced by
	// `adb conformance check` and driven on a schedule by a declarative D7 rule
	// (`adb schedule add --every … --run-exec "adb conformance check"`) — the
	// first real consumer of the #119 rule engine. templateVersion comes from the
	// same embedded template set `adb init project` scaffolds from.
	app.DriftChecker = core.NewDriftChecker(
		basePath,
		catalogSource,
		core.NewFileProjectInitializer(templates.FS).TemplateVersion(),
	)

	// Shared edge writer - lands typed edges onto task/initiative frontmatter (the
	// graph's source of truth), reused by the rule engine's edge outputs and the
	// ingestion pipeline's accepted edge proposals.
	edgeWriter := &edgeWriterAdapter{backlog: app.BacklogManager, stage: stageStore, nodes: nodeStore}

	// Rule engine - owns the unified declarative rule engine (decision D7). Rules
	// are authored into automation/rules.yaml (FileRuleStore); a rule's action is
	// run by FileActionRunner (exec commands run for real, skill invocations are
	// recorded as request files) and its outputs land as artifacts
	// (FileArtifactWriter) and/or typed graph edges (edgeWriter → the entity
	// stores). Time-triggered rules become scheduler jobs; event-triggered rules
	// fire via the scheduler's automation-dispatch job (opt-in, automation.enabled)
	// or `adb schedule dispatch`.
	app.RuleEngine = core.NewRuleEngine(
		storage.NewFileRuleStore(basePath),
		app.GraphManager,
		core.NewFileActionRunner(basePath),
		edgeWriter,
		core.NewFileArtifactWriter(basePath),
	)

	// Ingest manager - the staged ingestion pipeline (decision D8): immutable
	// raw/ landing with provenance + hash/cursor dedup (FileRawStore), a
	// confidence-gated review queue (FileProposalStore), and accepted proposals
	// landing as ingested nodes (nodeStore) or typed graph edges (edgeWriter).
	app.IngestManager = core.NewIngestManager(
		storage.NewFileRawStore(basePath),
		storage.NewFileProposalStore(basePath),
		nodeStore,
		edgeWriter,
	)

	// ===== Task Manager (wires everything together) =====
	// Resolve the per-repo worktree base branch + base dir from config, so the
	// hardcoded "main"/"work" no longer win over a workspace's .taskrc (#206).
	var repoCfg *models.RepoConfig
	if app.MergedConfig != nil {
		repoCfg = app.MergedConfig.Repo
	}

	// Create adapters
	backlogStoreAdpt := &backlogStoreAdapter{manager: app.BacklogManager}
	contextStoreAdpt := &contextStoreAdapter{manager: app.ContextManager}
	worktreeCreatorAdpt := &worktreeCreatorAdapter{
		manager:    app.GitWorktreeManager,
		basePath:   basePath,
		baseBranch: resolveWorktreeBaseBranch(repoCfg),
	}
	worktreeRemoverAdpt := &worktreeRemoverAdapter{manager: app.GitWorktreeManager}
	eventLoggerAdpt := &eventLoggerAdapter{log: app.EventLog}
	sessionCapturerAdpt := &sessionCapturerAdapter{manager: app.SessionStoreManager}

	// Create task manager with all dependencies
	archivedDir := filepath.Join(basePath, "tickets", "_archived")
	worktreesDir := resolveWorktreesDir(basePath, repoCfg)

	app.TaskManager = core.NewTaskManager(
		backlogStoreAdpt,
		contextStoreAdpt,
		worktreeCreatorAdpt,
		worktreeRemoverAdpt,
		eventLoggerAdpt,
		sessionCapturerAdpt,
		app.TaskIDGenerator,
		app.TemplateManager,
		ticketsDir,
		archivedDir,
		worktreesDir,
	)
	// Standing rules — the DOs/DONTs content inlined into every worktree's
	// Tier-0 task-context.md. Loaded here from the agent-agnostic templates FS
	// and injected into the TaskManager so core carries no embedded-FS import
	// (TASK-00031 agent-agnostic split). On read failure the TaskManager falls
	// back to a terse stub.
	if b, err := fs.ReadFile(templates.FS, "rules/standing-dos-donts.md"); err == nil {
		app.TaskManager.SetStandingRules(string(b))
	}
	// Wire the StageManager as the TaskManager's initiative resolver so a
	// ticket↔initiative association can be validated and the initiative's Stage
	// surfaced in the worktree AI context. StageManager satisfies
	// core.InitiativeResolver structurally.
	app.TaskManager.SetInitiativeResolver(app.StageManager)
	// Wire the GraphManager as the neighbour resolver so the worktree
	// task-context.md is seeded with the ticket's bounded 1-hop graph
	// neighbourhood (decision D9). GraphManager satisfies core.NeighborResolver
	// structurally; nil-safe if ever unset.
	app.TaskManager.SetNeighborResolver(app.GraphManager)

	// Auto-provision a per-worktree .serena/project.yml on the worktree-bootstrap
	// seam so Serena activates each code worktree as its own project (#202).
	// Fail-open + non-clobbering; adb configures only, never installs a server.
	app.TaskManager.SetSerenaProvisioner(core.NewSerenaProvisioner())

	return app, nil
}

// GetSessionStore returns the session store manager
func (app *App) GetSessionStore() storage.SessionStoreManager {
	return app.SessionStoreManager
}

// Cleanup performs cleanup operations (optional, for graceful shutdown)
func (app *App) Cleanup() error {
	// Future: close any open resources, flush buffers, etc.
	return nil
}

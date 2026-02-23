// Package internal provides the App struct that wires all components of the
// AI Dev Brain system together and initializes the CLI layer.
package internal

import (
	"os"
	"path/filepath"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/cli"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// App holds all service dependencies for the AI Dev Brain system.
type App struct {
	BasePath string

	// Configuration
	ConfigMgr core.ConfigurationManager

	// Storage layer
	BacklogMgr storage.BacklogManager
	ContextMgr storage.ContextManager
	CommMgr    storage.CommunicationManager

	// Core services
	TaskMgr      core.TaskManager
	Bootstrap    core.BootstrapSystem
	IDGen        core.TaskIDGenerator
	TmplMgr      core.TemplateManager
	UpdateGen    core.UpdateGenerator
	AICtxGen     core.AIContextGenerator
	DesignGen    core.TaskDesignDocGenerator
	KnowledgeX   core.KnowledgeExtractor
	KnowledgeMgr core.KnowledgeManager
	ConflictDt   core.ConflictDetector
	ProjectInit  core.ProjectInitializer

	// Storage layer (knowledge)
	KnowledgeStore storage.KnowledgeStoreManager

	// Storage layer (sessions)
	SessionStore storage.SessionStoreManager

	// Channel adapters
	ChannelReg   core.ChannelRegistry
	FeedbackLoop core.FeedbackLoopOrchestrator

	// Integration services
	WorktreeMgr integration.GitWorktreeManager
	OfflineMgr  integration.OfflineManager
	TabMgr      integration.TabManager
	ScreenPipe  integration.ScreenshotPipeline
	Executor    integration.CLIExecutor
	Runner      integration.TaskfileRunner
	RepoSyncMgr *integration.RepoSyncManager

	// Observability
	EventLog    observability.EventLog
	AlertEngine observability.AlertEngine
	MetricsCalc observability.MetricsCalculator
	Notifier    observability.Notifier
}

// NewApp creates and wires all components of the AI Dev Brain system.
// basePath is the root directory where all data is stored (typically ~/.adb or
// the current directory containing .taskconfig).
func NewApp(basePath string) (*App, error) {
	app := &App{BasePath: basePath}

	// --- Configuration ---
	app.ConfigMgr = core.NewConfigurationManager(basePath)
	globalCfg, err := app.ConfigMgr.LoadGlobalConfig()
	if err != nil {
		// Use defaults if config file is missing.
		globalCfg = &models.GlobalConfig{
			DefaultAI:       "kiro",
			TaskIDPrefix:    "TASK",
			DefaultPriority: models.P2,
		}
	}

	// --- Storage layer ---
	app.BacklogMgr = storage.NewBacklogManager(basePath)
	app.ContextMgr = storage.NewContextManager(basePath)
	app.CommMgr = storage.NewCommunicationManager(basePath)

	// --- Integration services ---
	app.WorktreeMgr = integration.NewGitWorktreeManager(basePath)
	app.OfflineMgr = integration.NewOfflineManager(basePath)
	app.TabMgr = integration.NewTabManager()
	app.ScreenPipe = integration.NewScreenshotPipeline(basePath)
	app.Executor = integration.NewCLIExecutor()
	app.Runner = integration.NewTaskfileRunner(app.Executor)
	app.RepoSyncMgr = integration.NewRepoSyncManager(basePath)

	// --- Channel adapters ---
	app.ChannelReg = core.NewChannelRegistry()
	channelDir := filepath.Join(basePath, "channels")
	fileAdapter, fileAdapterErr := integration.NewFileChannelAdapter(integration.FileChannelConfig{
		Name:    "file",
		BaseDir: channelDir,
	})
	if fileAdapterErr == nil {
		_ = app.ChannelReg.Register(fileAdapter) // Non-fatal if registration fails.
	}

	// --- Observability ---
	eventLogPath := filepath.Join(basePath, ".adb_events.jsonl")
	app.EventLog, err = observability.NewJSONLEventLog(eventLogPath)
	if err != nil {
		// Non-fatal: disable observability if log can't be created.
		app.EventLog = nil
	}
	if app.EventLog != nil {
		thresholds := observability.DefaultAlertThresholds()
		if globalCfg.Notifications.Alerts.BlockedHours > 0 {
			thresholds.BlockedHours = globalCfg.Notifications.Alerts.BlockedHours
		}
		if globalCfg.Notifications.Alerts.StaleDays > 0 {
			thresholds.StaleDays = globalCfg.Notifications.Alerts.StaleDays
		}
		if globalCfg.Notifications.Alerts.ReviewDays > 0 {
			thresholds.ReviewDays = globalCfg.Notifications.Alerts.ReviewDays
		}
		if globalCfg.Notifications.Alerts.MaxBacklogSize > 0 {
			thresholds.MaxBacklogSize = globalCfg.Notifications.Alerts.MaxBacklogSize
		}
		app.AlertEngine = observability.NewAlertEngine(app.EventLog, thresholds)
		app.MetricsCalc = observability.NewMetricsCalculator(app.EventLog)
	}
	if globalCfg.Notifications.Enabled && globalCfg.Notifications.Slack.WebhookURL != "" {
		app.Notifier = observability.NewSlackNotifier(globalCfg.Notifications.Slack.WebhookURL)
	}

	// --- Core services ---
	prefix := globalCfg.TaskIDPrefix
	if prefix == "" {
		prefix = "TASK"
	}
	padWidth := globalCfg.TaskIDPadWidth
	app.IDGen = core.NewTaskIDGenerator(basePath, prefix, padWidth)
	app.TmplMgr = core.NewTemplateManager(basePath)

	// Create a worktree adapter so the core bootstrap can use it without
	// importing the integration package directly.
	wtAdapter := &worktreeAdapter{mgr: app.WorktreeMgr}
	app.Bootstrap = core.NewBootstrapSystem(basePath, app.IDGen, wtAdapter, app.TmplMgr)

	// Create adapters for the task manager's BacklogStore and ContextStore interfaces.
	blAdapter := &backlogStoreAdapter{mgr: app.BacklogMgr}
	ctxAdapter := &contextStoreAdapter{mgr: app.ContextMgr}
	wtRemoveAdapter := &worktreeRemoverAdapter{mgr: app.WorktreeMgr}
	var evtAdapter core.EventLogger
	if app.EventLog != nil {
		evtAdapter = &eventLogAdapter{log: app.EventLog}
	}
	app.TaskMgr = core.NewTaskManager(basePath, app.Bootstrap, blAdapter, ctxAdapter, wtRemoveAdapter, evtAdapter)

	app.UpdateGen = core.NewUpdateGenerator(app.ContextMgr, app.CommMgr)
	app.DesignGen = core.NewTaskDesignDocGenerator(basePath, app.CommMgr)
	app.KnowledgeX = core.NewKnowledgeExtractor(basePath, app.ContextMgr, app.CommMgr)

	// --- Knowledge store ---
	app.KnowledgeStore = storage.NewKnowledgeStoreManager(basePath)
	_ = app.KnowledgeStore.Load() // Non-fatal: empty store on first use.
	ksAdapter := &knowledgeStoreAdapter{mgr: app.KnowledgeStore}
	app.KnowledgeMgr = core.NewKnowledgeManager(ksAdapter)

	// --- Session store ---
	app.SessionStore = storage.NewSessionStoreManager(basePath)
	_ = app.SessionStore.Load() // Non-fatal: empty store on first use.
	scAdapter := &sessionCapturerAdapter{mgr: app.SessionStore}
	cli.SessionCapture = scAdapter

	// --- Feedback loop orchestrator ---
	app.FeedbackLoop = core.NewFeedbackLoopOrchestrator(
		app.ChannelReg,
		app.KnowledgeMgr,
		blAdapter,
		evtAdapter,
		prefix,
	)

	// AIContextGenerator depends on KnowledgeManager for the knowledge summary section
	// and SessionCapturer for captured session display.
	app.AICtxGen = core.NewAIContextGenerator(basePath, app.BacklogMgr, app.KnowledgeMgr, scAdapter)

	app.ConflictDt = core.NewConflictDetector(basePath)
	app.ProjectInit = core.NewProjectInitializer()

	// --- Hook engine ---
	hookCfg := globalCfg.Hooks
	// If no hooks section was configured, use sensible defaults.
	if hookCfg == (models.HookConfig{}) {
		hookCfg = models.DefaultHookConfig()
	}
	hookEngine := core.NewHookEngine(basePath, hookCfg, app.KnowledgeX, app.ConflictDt)
	cli.HookEngine = hookEngine

	// --- Wire CLI package-level variables ---
	cli.BasePath = basePath
	cli.TaskMgr = app.TaskMgr
	cli.UpdateGen = app.UpdateGen
	cli.AICtxGen = app.AICtxGen
	cli.Executor = app.Executor
	cli.Runner = app.Runner
	cli.ProjectInit = app.ProjectInit
	cli.RepoSyncMgr = app.RepoSyncMgr

	cli.ChannelReg = app.ChannelReg
	cli.KnowledgeMgr = app.KnowledgeMgr
	cli.KnowledgeX = app.KnowledgeX
	cli.FeedbackLoop = app.FeedbackLoop

	cli.EventLog = app.EventLog
	cli.AlertEngine = app.AlertEngine
	cli.MetricsCalc = app.MetricsCalc
	cli.Notifier = app.Notifier
	cli.BranchPattern = globalCfg.BranchPattern

	// Convert CLIAliasConfig to integration.CLIAlias.
	aliases := make([]integration.CLIAlias, len(globalCfg.CLIAliases))
	for i, a := range globalCfg.CLIAliases {
		aliases[i] = integration.CLIAlias{
			Name:        a.Name,
			Command:     a.Command,
			DefaultArgs: a.DefaultArgs,
		}
	}
	cli.ExecAliases = aliases

	return app, nil
}

// Close releases resources held by the App, such as the event log file handle.
// It is safe to call Close on an App whose EventLog is nil.
func (a *App) Close() error {
	if a.EventLog != nil {
		return a.EventLog.Close()
	}
	return nil
}

// resolveBasePath determines the base path for the AI Dev Brain data directory.
// It checks for ADB_HOME env var, then falls back to the current directory.
func ResolveBasePath() string {
	if home := os.Getenv("ADB_HOME"); home != "" {
		return home
	}
	// Default: look for .taskconfig in the current directory tree.
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	// Walk up to find a directory containing .taskconfig.
	for {
		if _, err := os.Stat(filepath.Join(dir, ".taskconfig")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fall back to cwd.
	cwd, _ := os.Getwd()
	return cwd
}

// --- Adapters ---

// worktreeAdapter adapts integration.GitWorktreeManager to core.WorktreeCreator.
type worktreeAdapter struct {
	mgr integration.GitWorktreeManager
}

func (a *worktreeAdapter) CreateWorktree(config core.WorktreeCreateConfig) (string, error) {
	return a.mgr.CreateWorktree(integration.WorktreeConfig{
		RepoPath:   config.RepoPath,
		BranchName: config.BranchName,
		TaskID:     config.TaskID,
		BaseBranch: config.BaseBranch,
	})
}

// worktreeRemoverAdapter adapts integration.GitWorktreeManager to core.WorktreeRemover.
type worktreeRemoverAdapter struct {
	mgr integration.GitWorktreeManager
}

func (a *worktreeRemoverAdapter) RemoveWorktree(worktreePath string) error {
	return a.mgr.RemoveWorktree(worktreePath)
}

// backlogStoreAdapter adapts storage.BacklogManager to core.BacklogStore.
type backlogStoreAdapter struct {
	mgr storage.BacklogManager
}

func (a *backlogStoreAdapter) AddTask(entry core.BacklogStoreEntry) error {
	return a.mgr.AddTask(storageEntryFromCore(entry))
}

func (a *backlogStoreAdapter) UpdateTask(taskID string, updates core.BacklogStoreEntry) error {
	return a.mgr.UpdateTask(taskID, storageEntryFromCore(updates))
}

func (a *backlogStoreAdapter) GetTask(taskID string) (*core.BacklogStoreEntry, error) {
	e, err := a.mgr.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	result := coreEntryFromStorage(*e)
	return &result, nil
}

func (a *backlogStoreAdapter) GetAllTasks() ([]core.BacklogStoreEntry, error) {
	entries, err := a.mgr.GetAllTasks()
	if err != nil {
		return nil, err
	}
	result := make([]core.BacklogStoreEntry, len(entries))
	for i, e := range entries {
		result[i] = coreEntryFromStorage(e)
	}
	return result, nil
}

func (a *backlogStoreAdapter) FilterTasks(filter core.BacklogStoreFilter) ([]core.BacklogStoreEntry, error) {
	entries, err := a.mgr.FilterTasks(storage.BacklogFilter{
		Status:   filter.Status,
		Priority: filter.Priority,
		Owner:    filter.Owner,
		Repo:     filter.Repo,
		Tags:     filter.Tags,
	})
	if err != nil {
		return nil, err
	}
	result := make([]core.BacklogStoreEntry, len(entries))
	for i, e := range entries {
		result[i] = coreEntryFromStorage(e)
	}
	return result, nil
}

func (a *backlogStoreAdapter) Load() error {
	return a.mgr.Load()
}

func (a *backlogStoreAdapter) Save() error {
	return a.mgr.Save()
}

func storageEntryFromCore(e core.BacklogStoreEntry) storage.BacklogEntry {
	return storage.BacklogEntry{
		ID:        e.ID,
		Title:     e.Title,
		Source:    e.Source,
		Status:    e.Status,
		Priority:  e.Priority,
		Owner:     e.Owner,
		Repo:      e.Repo,
		Branch:    e.Branch,
		Created:   e.Created,
		Tags:      e.Tags,
		BlockedBy: e.BlockedBy,
		Related:   e.Related,
	}
}

func coreEntryFromStorage(e storage.BacklogEntry) core.BacklogStoreEntry {
	return core.BacklogStoreEntry{
		ID:        e.ID,
		Title:     e.Title,
		Source:    e.Source,
		Status:    e.Status,
		Priority:  e.Priority,
		Owner:     e.Owner,
		Repo:      e.Repo,
		Branch:    e.Branch,
		Created:   e.Created,
		Tags:      e.Tags,
		BlockedBy: e.BlockedBy,
		Related:   e.Related,
	}
}

// contextStoreAdapter adapts storage.ContextManager to core.ContextStore.
type contextStoreAdapter struct {
	mgr storage.ContextManager
}

func (a *contextStoreAdapter) LoadContext(taskID string) (interface{}, error) {
	return a.mgr.LoadContext(taskID)
}

// eventLogAdapter adapts observability.EventLog to core.EventLogger.
type eventLogAdapter struct {
	log observability.EventLog
}

func (a *eventLogAdapter) LogEvent(eventType string, data map[string]any) error {
	return a.log.Write(observability.Event{
		Time:    time.Now().UTC(),
		Level:   "INFO",
		Type:    eventType,
		Message: eventType,
		Data:    data,
	})
}

// knowledgeStoreAdapter adapts storage.KnowledgeStoreManager to core.KnowledgeStoreAccess.
type knowledgeStoreAdapter struct {
	mgr storage.KnowledgeStoreManager
}

func (a *knowledgeStoreAdapter) AddEntry(entry models.KnowledgeEntry) (string, error) {
	return a.mgr.AddEntry(entry)
}

func (a *knowledgeStoreAdapter) GetEntry(id string) (*models.KnowledgeEntry, error) {
	return a.mgr.GetEntry(id)
}

func (a *knowledgeStoreAdapter) GetAllEntries() ([]models.KnowledgeEntry, error) {
	return a.mgr.GetAllEntries()
}

func (a *knowledgeStoreAdapter) QueryByTopic(topic string) ([]models.KnowledgeEntry, error) {
	return a.mgr.QueryByTopic(topic)
}

func (a *knowledgeStoreAdapter) QueryByEntity(entity string) ([]models.KnowledgeEntry, error) {
	return a.mgr.QueryByEntity(entity)
}

func (a *knowledgeStoreAdapter) QueryByTags(tags []string) ([]models.KnowledgeEntry, error) {
	return a.mgr.QueryByTags(tags)
}

func (a *knowledgeStoreAdapter) Search(query string) ([]models.KnowledgeEntry, error) {
	return a.mgr.Search(query)
}

func (a *knowledgeStoreAdapter) GetTopics() (*models.TopicGraph, error) {
	return a.mgr.GetTopics()
}

func (a *knowledgeStoreAdapter) AddTopic(topic models.Topic) error {
	return a.mgr.AddTopic(topic)
}

func (a *knowledgeStoreAdapter) GetTopic(name string) (*models.Topic, error) {
	return a.mgr.GetTopic(name)
}

func (a *knowledgeStoreAdapter) GetEntities() (*models.EntityRegistry, error) {
	return a.mgr.GetEntities()
}

func (a *knowledgeStoreAdapter) AddEntity(entity models.Entity) error {
	return a.mgr.AddEntity(entity)
}

func (a *knowledgeStoreAdapter) GetEntity(name string) (*models.Entity, error) {
	return a.mgr.GetEntity(name)
}

func (a *knowledgeStoreAdapter) GetTimeline(since time.Time) ([]models.TimelineEntry, error) {
	return a.mgr.GetTimeline(since)
}

func (a *knowledgeStoreAdapter) AddTimelineEntry(entry models.TimelineEntry) error {
	return a.mgr.AddTimelineEntry(entry)
}

func (a *knowledgeStoreAdapter) GenerateID() (string, error) {
	return a.mgr.GenerateID()
}

func (a *knowledgeStoreAdapter) Load() error {
	return a.mgr.Load()
}

func (a *knowledgeStoreAdapter) Save() error {
	return a.mgr.Save()
}

// sessionCapturerAdapter adapts storage.SessionStoreManager to core.SessionCapturer.
type sessionCapturerAdapter struct {
	mgr storage.SessionStoreManager
}

func (a *sessionCapturerAdapter) CaptureSession(session models.CapturedSession, turns []models.SessionTurn) (string, error) {
	return a.mgr.AddSession(session, turns)
}

func (a *sessionCapturerAdapter) GetSession(sessionID string) (*models.CapturedSession, error) {
	return a.mgr.GetSession(sessionID)
}

func (a *sessionCapturerAdapter) ListSessions(filter models.SessionFilter) ([]models.CapturedSession, error) {
	return a.mgr.ListSessions(filter)
}

func (a *sessionCapturerAdapter) GetSessionTurns(sessionID string) ([]models.SessionTurn, error) {
	return a.mgr.GetSessionTurns(sessionID)
}

func (a *sessionCapturerAdapter) GetLatestSessionForTask(taskID string) (*models.CapturedSession, error) {
	return a.mgr.GetLatestSessionForTask(taskID)
}

func (a *sessionCapturerAdapter) GetRecentSessions(limit int) ([]models.CapturedSession, error) {
	return a.mgr.GetRecentSessions(limit)
}

func (a *sessionCapturerAdapter) GenerateID() (string, error) {
	return a.mgr.GenerateID()
}

func (a *sessionCapturerAdapter) Save() error {
	return a.mgr.Save()
}

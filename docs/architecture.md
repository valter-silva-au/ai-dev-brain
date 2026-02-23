# Architecture

This document describes the internal architecture of AI Dev Brain (`adb`), covering the layered package structure, interface contracts, data flow, storage design, dependency injection, and extension points.

---

## System Overview

AI Dev Brain is organized into five layers, each with a clear responsibility boundary. The CLI layer accepts user commands. The core layer contains all business logic, including the feedback loop orchestrator, knowledge manager, channel registry, and hook engine. The integration layer communicates with external systems (git, OS tools, Taskfile, file-based channels). The storage layer persists data to the local filesystem, including the long-term knowledge store. The observability layer provides event logging, metrics derivation, and alerting. A shared `pkg/models` package defines the data structures used across layers. The `internal/hooks` support package provides stdin parsing, change tracking, and artifact helpers used by the hook engine.

```mermaid
graph TD
    subgraph "CLI Layer"
        CLI["cmd/adb/main.go<br/>internal/cli (Cobra commands)"]
    end

    subgraph "Core Layer"
        TM[TaskManager]
        BS[BootstrapSystem]
        CM[ConfigurationManager]
        KE[KnowledgeExtractor]
        CD[ConflictDetector]
        ACG[AIContextGenerator]
        UG[UpdateGenerator]
        TDG[TaskDesignDocGenerator]
        IDG[TaskIDGenerator]
        TMPL[TemplateManager]
        KM[KnowledgeManager]
        FL[FeedbackLoopOrchestrator]
        CR[ChannelRegistry]
        HEN[HookEngine]
    end

    subgraph "Observability Layer"
        EL[EventLog]
        MC[MetricsCalculator]
        AE[AlertEngine]
    end

    subgraph "Integration Layer"
        GWM[GitWorktreeManager]
        OM[OfflineManager]
        TABM[TabManager]
        SP[ScreenshotPipeline]
        EXEC[CLIExecutor]
        TFR[TaskfileRunner]
        FCA[FileChannelAdapter]
        TP[TranscriptParser]
    end

    subgraph "Storage Layer"
        BM[BacklogManager]
        CTX[ContextManager]
        COMM[CommunicationManager]
        KS[KnowledgeStoreManager]
        SSM[SessionStoreManager]
    end

    subgraph "Shared Models"
        MDL["pkg/models<br/>(Task, Config, Knowledge,<br/>Communication, Channel)"]
    end

    subgraph "Embedded Templates"
        CTPL["templates/claude<br/>(Embedded Claude templates)"]
    end

    CLI --> TM
    CLI --> CTPL
    CLI --> UG
    CLI --> ACG
    CLI --> EXEC
    CLI --> TFR
    CLI --> EL
    CLI --> MC
    CLI --> AE
    CLI --> FL
    CLI --> CR
    CLI --> KM
    CLI --> SSM

    TM --> BS
    TM --> BM
    TM --> CTX
    BS --> IDG
    BS --> TMPL
    BS --> GWM

    KE --> CTX
    KE --> COMM
    UG --> CTX
    UG --> COMM
    ACG --> BM
    ACG --> KM
    ACG --> SSM
    TDG --> COMM
    CD --> MDL

    FL --> CR
    FL --> KM
    FL --> BM
    CR --> FCA
    KM --> KS

    MC --> EL
    AE --> EL

    TFR --> EXEC

    CLI --> TP
    CLI --> HEN
    HEN --> KE
    HEN --> CD

    BM --> MDL
    CTX --> MDL
    COMM --> MDL
    SSM --> MDL

    style CLI fill:#4a90d9,color:#fff
    style TM fill:#7b68ee,color:#fff
    style BS fill:#7b68ee,color:#fff
    style CM fill:#7b68ee,color:#fff
    style KE fill:#7b68ee,color:#fff
    style CD fill:#7b68ee,color:#fff
    style ACG fill:#7b68ee,color:#fff
    style UG fill:#7b68ee,color:#fff
    style TDG fill:#7b68ee,color:#fff
    style IDG fill:#7b68ee,color:#fff
    style TMPL fill:#7b68ee,color:#fff
    style KM fill:#7b68ee,color:#fff
    style FL fill:#7b68ee,color:#fff
    style CR fill:#7b68ee,color:#fff
    style HEN fill:#7b68ee,color:#fff
    style EL fill:#e74c3c,color:#fff
    style MC fill:#e74c3c,color:#fff
    style AE fill:#e74c3c,color:#fff
    style GWM fill:#2ecc71,color:#fff
    style OM fill:#2ecc71,color:#fff
    style TABM fill:#2ecc71,color:#fff
    style SP fill:#2ecc71,color:#fff
    style EXEC fill:#2ecc71,color:#fff
    style TFR fill:#2ecc71,color:#fff
    style FCA fill:#2ecc71,color:#fff
    style TP fill:#2ecc71,color:#fff
    style BM fill:#e67e22,color:#fff
    style CTX fill:#e67e22,color:#fff
    style COMM fill:#e67e22,color:#fff
    style KS fill:#e67e22,color:#fff
    style SSM fill:#e67e22,color:#fff
    style MDL fill:#95a5a6,color:#fff
    style CTPL fill:#f1c40f,color:#000
```

---

## Component and Interface Relationships

Every major component is defined by a Go interface in its owning package. Concrete implementations are unexported (lowercase struct names), and all construction goes through `New*` factory functions. The `App` struct in `internal/app.go` is the composition root that wires everything together.

```mermaid
classDiagram
    direction LR

    class TaskManager {
        <<interface>>
        +CreateTask(taskType, branchName, repoPath) Task, error
        +ResumeTask(taskID) Task, error
        +ArchiveTask(taskID) HandoffDocument, error
        +UnarchiveTask(taskID) Task, error
        +GetTasksByStatus(status) []Task, error
        +GetAllTasks() []Task, error
        +GetTask(taskID) Task, error
        +UpdateTaskStatus(taskID, status) error
        +UpdateTaskPriority(taskID, priority) error
        +ReorderPriorities(taskIDs) error
        +CleanupWorktree(taskID) error
    }

    class BootstrapSystem {
        <<interface>>
        +Bootstrap(config) BootstrapResult, error
        +ApplyTemplate(taskID, templateType) error
        +GenerateTaskID() string, error
    }

    class ConfigurationManager {
        <<interface>>
        +LoadGlobalConfig() GlobalConfig, error
        +LoadRepoConfig(repoPath) RepoConfig, error
        +GetMergedConfig(repoPath) MergedConfig, error
        +ValidateConfig(config) error
    }

    class BacklogStore {
        <<interface>>
        +AddTask(entry) error
        +UpdateTask(taskID, updates) error
        +GetTask(taskID) BacklogStoreEntry, error
        +GetAllTasks() []BacklogStoreEntry, error
        +FilterTasks(filter) []BacklogStoreEntry, error
        +Load() error
        +Save() error
    }

    class ContextStore {
        <<interface>>
        +LoadContext(taskID) interface, error
    }

    class WorktreeCreator {
        <<interface>>
        +CreateWorktree(config) string, error
    }

    class WorktreeRemover {
        <<interface>>
        +RemoveWorktree(worktreePath) error
    }

    class TaskIDGenerator {
        <<interface>>
        +GenerateTaskID() string, error
    }

    class TemplateManager {
        <<interface>>
        +ApplyTemplate(ticketPath, templateType) error
        +GetTemplate(taskType) string, error
        +RegisterTemplate(taskType, templatePath) error
    }

    class EventLog {
        <<interface>>
        +Write(event) error
        +Read(filter) []Event, error
        +Close() error
    }

    class MetricsCalculator {
        <<interface>>
        +Calculate(since) Metrics, error
    }

    class AlertEngine {
        <<interface>>
        +Evaluate() []Alert, error
    }

    class EventLogger {
        <<interface>>
        +LogEvent(eventType, data) error
    }

    class KnowledgeStoreAccess {
        <<interface>>
        +AddEntry(entry) string, error
        +GetEntry(id) KnowledgeEntry, error
        +GetAllEntries() []KnowledgeEntry, error
        +QueryByTopic(topic) []KnowledgeEntry, error
        +QueryByEntity(entity) []KnowledgeEntry, error
        +QueryByTags(tags) []KnowledgeEntry, error
        +Search(query) []KnowledgeEntry, error
        +GetTopics() TopicGraph, error
        +AddTopic(topic) error
        +GetTopic(name) Topic, error
        +GetEntities() EntityRegistry, error
        +AddEntity(entity) error
        +GetEntity(name) Entity, error
        +GetTimeline(since) []TimelineEntry, error
        +AddTimelineEntry(entry) error
        +GenerateID() string, error
        +Load() error
        +Save() error
    }

    class KnowledgeManager {
        <<interface>>
        +AddKnowledge(...) string, error
        +IngestFromExtraction(knowledge) []string, error
        +Search(query) []KnowledgeEntry, error
        +QueryByTopic(topic) []KnowledgeEntry, error
        +QueryByEntity(entity) []KnowledgeEntry, error
        +QueryByTags(tags) []KnowledgeEntry, error
        +ListTopics() TopicGraph, error
        +GetTopicEntries(topic) []KnowledgeEntry, error
        +GetTimeline(since) []TimelineEntry, error
        +GetRelatedKnowledge(task) []KnowledgeEntry, error
        +AssembleKnowledgeSummary(maxEntries) string, error
    }

    class ChannelAdapter {
        <<interface>>
        +Name() string
        +Type() ChannelType
        +Fetch() []ChannelItem, error
        +Send(item) error
        +MarkProcessed(itemID) error
    }

    class ChannelRegistry {
        <<interface>>
        +Register(adapter) error
        +GetAdapter(name) ChannelAdapter, error
        +ListAdapters() []ChannelAdapter
        +FetchAll() []ChannelItem, error
    }

    class FeedbackLoopOrchestrator {
        <<interface>>
        +Run(opts) LoopResult, error
        +ProcessItem(item) ProcessedItem, error
    }

    class ProjectInitializer {
        <<interface>>
        +Init(config) InitResult, error
    }

    class HookEngine {
        <<interface>>
        +HandlePreToolUse(input) error
        +HandlePostToolUse(input) error
        +HandleStop(input) error
        +HandleTaskCompleted(input) error
        +HandleSessionEnd(input) error
    }

    class Notifier {
        <<interface>>
        +Notify(alerts) error
    }

    HookEngine --> KnowledgeExtractor : extracts knowledge (Phase 2)
    HookEngine --> ConflictDetector : checks ADR conflicts (Phase 3)
    TaskManager --> BootstrapSystem : delegates creation
    TaskManager --> BacklogStore : persists entries
    TaskManager --> ContextStore : loads context
    TaskManager --> WorktreeRemover : removes worktrees
    TaskManager --> EventLogger : logs events
    BootstrapSystem --> TaskIDGenerator : generates IDs
    BootstrapSystem --> WorktreeCreator : creates worktrees
    BootstrapSystem --> TemplateManager : applies templates
    KnowledgeManager --> KnowledgeStoreAccess : persists knowledge
    FeedbackLoopOrchestrator --> ChannelRegistry : fetches items
    FeedbackLoopOrchestrator --> KnowledgeManager : ingests knowledge
    FeedbackLoopOrchestrator --> BacklogStore : looks up tasks
    FeedbackLoopOrchestrator --> EventLogger : logs events
    MetricsCalculator --> EventLog : reads events
    AlertEngine --> EventLog : evaluates conditions
```

---

## Adapter Pattern and Dependency Injection

The core layer defines narrow "store" interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`, `WorktreeRemover`, `EventLogger`, `KnowledgeStoreAccess`, `SessionCapturer`) that mirror subsets of the storage, integration, and observability interfaces. This keeps the core package free of import dependencies on those packages. The `App` struct bridges the gap using adapter structs.

```mermaid
classDiagram
    direction TB

    namespace core {
        class BacklogStore {
            <<interface>>
        }
        class ContextStore {
            <<interface>>
        }
        class WorktreeCreator {
            <<interface>>
        }
        class WorktreeRemover {
            <<interface>>
        }
        class EventLogger {
            <<interface>>
        }
        class KnowledgeStoreAccess {
            <<interface>>
        }
        class SessionCapturer {
            <<interface>>
        }
    }

    namespace storage {
        class BacklogManager {
            <<interface>>
        }
        class ContextManager {
            <<interface>>
        }
        class KnowledgeStoreManager {
            <<interface>>
        }
        class SessionStoreManager {
            <<interface>>
        }
    }

    namespace integration {
        class GitWorktreeManager {
            <<interface>>
        }
    }

    namespace observability {
        class EventLog {
            <<interface>>
        }
    }

    namespace internal_app {
        class backlogStoreAdapter {
            -mgr: BacklogManager
        }
        class contextStoreAdapter {
            -mgr: ContextManager
        }
        class worktreeAdapter {
            -mgr: GitWorktreeManager
        }
        class worktreeRemoverAdapter {
            -mgr: GitWorktreeManager
        }
        class eventLogAdapter {
            -log: EventLog
        }
        class knowledgeStoreAdapter {
            -mgr: KnowledgeStoreManager
        }
        class sessionCapturerAdapter {
            -mgr: SessionStoreManager
        }
    }

    backlogStoreAdapter ..|> BacklogStore : implements
    backlogStoreAdapter --> BacklogManager : delegates to

    contextStoreAdapter ..|> ContextStore : implements
    contextStoreAdapter --> ContextManager : delegates to

    worktreeAdapter ..|> WorktreeCreator : implements
    worktreeAdapter --> GitWorktreeManager : delegates to

    worktreeRemoverAdapter ..|> WorktreeRemover : implements
    worktreeRemoverAdapter --> GitWorktreeManager : delegates to

    eventLogAdapter ..|> EventLogger : implements
    eventLogAdapter --> EventLog : delegates to

    knowledgeStoreAdapter ..|> KnowledgeStoreAccess : implements
    knowledgeStoreAdapter --> KnowledgeStoreManager : delegates to

    sessionCapturerAdapter ..|> SessionCapturer : implements
    sessionCapturerAdapter --> SessionStoreManager : delegates to
```

The `NewApp` function in `internal/app.go` performs all wiring in a fixed order:

1. **Configuration** -- `ConfigurationManager` loads `.taskconfig` with Viper.
2. **Storage** -- `BacklogManager`, `ContextManager`, `CommunicationManager`, `SessionStoreManager` are created with the base path.
3. **Integration** -- `GitWorktreeManager`, `OfflineManager`, `TabManager`, `ScreenshotPipeline`, `CLIExecutor`, `TaskfileRunner`, `RepoSyncManager` are created.
4. **Observability** -- `EventLog` (JSONL-backed) is opened at `.adb_events.jsonl`. `AlertEngine` and `MetricsCalculator` are created with the event log and configurable thresholds. If the event log cannot be created, observability is disabled gracefully (non-fatal).
5. **Core** -- Core services receive their dependencies through constructors, using adapter structs where cross-layer communication is needed. The `eventLogAdapter` bridges `observability.EventLog` to `core.EventLogger`. The `worktreeRemoverAdapter` bridges `integration.GitWorktreeManager` to `core.WorktreeRemover`. The `sessionCapturerAdapter` bridges `storage.SessionStoreManager` to `core.SessionCapturer`.
6. **Channel adapters** -- `ChannelRegistry` is created. `FileChannelAdapter` is initialized from the `channels/` directory and registered (non-fatal if registration fails).
7. **Knowledge store** -- `KnowledgeStoreManager` is created and loaded. `knowledgeStoreAdapter` bridges it to `core.KnowledgeStoreAccess`. `KnowledgeManager` and `FeedbackLoopOrchestrator` are created with their dependencies.
8. **CLI wiring** -- Package-level variables in `internal/cli` are set to the core, integration, and observability service instances (`TaskMgr`, `UpdateGen`, `AICtxGen`, `Executor`, `Runner`, `ProjectInit`, `RepoSyncMgr`, `ChannelReg`, `KnowledgeMgr`, `KnowledgeX`, `FeedbackLoop`, `EventLog`, `AlertEngine`, `MetricsCalc`, `SessionCapture`, `Notifier`).

---

## Task Creation Flow

The following sequence diagram shows what happens when a user runs `adb feat my-feature --repo github.com/org/repo`:

```mermaid
sequenceDiagram
    participant User
    participant CLI as CLI (Cobra)
    participant TM as TaskManager
    participant BS as BootstrapSystem
    participant IDG as TaskIDGenerator
    participant TMPL as TemplateManager
    participant WT as WorktreeCreator
    participant BL as BacklogStore

    User->>CLI: adb feat my-feature --repo ...
    CLI->>TM: CreateTask(feat, "my-feature", repoPath)
    TM->>BS: Bootstrap(BootstrapConfig)

    BS->>IDG: GenerateTaskID()
    IDG-->>BS: "TASK-00042"

    BS->>BS: MkdirAll tickets/TASK-00042/{communications,sessions,knowledge}/

    BS->>TMPL: ApplyTemplate(ticketPath, "feat")
    TMPL-->>BS: writes notes.md + design.md

    BS->>BS: Write context.md (scaffold)
    BS->>BS: Write status.yaml (Task struct)

    BS->>WT: CreateWorktree(config)
    WT-->>BS: worktree path

    BS->>BS: generateTaskContext(worktreePath, taskID, config)
    Note over BS: Writes .claude/rules/task-context.md into worktree

    BS-->>TM: BootstrapResult{TaskID, TicketPath, WorktreePath}

    TM->>TM: loadTaskFromTicket("TASK-00042")
    TM->>BL: Load()
    TM->>BL: AddTask(entry)
    TM->>BL: Save()
    TM-->>CLI: Task{ID: "TASK-00042", ...}
    CLI-->>User: Task TASK-00042 created
```

### What each step produces on disk

| Step | File / Directory |
|------|-----------------|
| GenerateTaskID | `.task_counter` (incremented) |
| MkdirAll | `tickets/TASK-00042/`, `tickets/TASK-00042/communications/`, `tickets/TASK-00042/sessions/`, `tickets/TASK-00042/knowledge/` |
| ApplyTemplate | `tickets/TASK-00042/notes.md`, `tickets/TASK-00042/design.md` |
| Write context.md | `tickets/TASK-00042/context.md` |
| Write status.yaml | `tickets/TASK-00042/status.yaml` |
| CreateWorktree | `repos/{platform}/{org}/{repo}/work/TASK-00042/` |
| generateTaskContext | `work/TASK-00042/.claude/rules/task-context.md` (non-fatal if fails) |
| BacklogStore.Save | `backlog.yaml` (updated) |

---

## Knowledge Extraction Flow

When a task is archived, the `KnowledgeExtractor` gathers learnings, decisions, and gotchas from the task's files and communications, then feeds them into organizational knowledge stores (wiki, ADRs, runbooks).

```mermaid
sequenceDiagram
    participant Caller
    participant KE as KnowledgeExtractor
    participant CTX as ContextManager
    participant COMM as CommunicationManager
    participant FS as Filesystem

    Caller->>KE: ExtractFromTask(taskID)

    KE->>CTX: LoadContext(taskID)
    CTX-->>KE: TaskContext{Notes, Context, Communications}

    KE->>COMM: GetAllCommunications(taskID)
    COMM-->>KE: []Communication

    KE->>KE: extractListItems(notes, "## Learnings")
    KE->>KE: extractListItems(notes, "## Gotchas")
    KE->>KE: extractListItems(context, "## Decisions Made")

    KE->>KE: Scan communications for #decision tags

    KE->>FS: ReadFile(tickets/{taskID}/design.md)
    KE->>KE: extractDesignDocDecisions(designContent)
    KE->>KE: extractComponentLearnings(designContent)

    KE->>KE: extractListItems(notes, "## Wiki Updates")
    KE->>KE: extractListItems(notes, "## Runbook Updates")

    KE-->>Caller: ExtractedKnowledge{Learnings, Decisions, Gotchas, WikiUpdates, RunbookUpdates}

    Note over Caller,FS: Caller may then invoke UpdateWiki() or CreateADR()

    Caller->>KE: UpdateWiki(knowledge)
    KE->>FS: Write docs/wiki/{topic}.md

    Caller->>KE: CreateADR(decision, taskID)
    KE->>FS: Write docs/decisions/ADR-XXXX-{title}.md
```

---

## AI Context Generation Flow

The `AIContextGenerator` assembles project-wide context from multiple sources into a single markdown file (e.g., `CLAUDE.md` or `kiro.md`) that AI coding assistants read for project awareness.

```mermaid
flowchart TD
    A[GenerateContextFile / SyncContext called] --> B[assembleAll]

    B --> C[AssembleProjectOverview]
    B --> D[AssembleDirectoryStructure]
    B --> E[AssembleConventions]
    B --> F[AssembleGlossary]
    B --> G[AssembleDecisionsSummary]
    B --> H[AssembleActiveTaskSummaries]
    B --> I[assembleCriticalDecisions]
    B --> J[assembleRecentSessions]
    B --> J2[assembleCapturedSessions]
    B --> J3[renderWhatsChanged]
    B --> K[assembleStakeholders + assembleContacts]

    C --> |"Static overview text"| L[AIContextSections]
    D --> |"Directory tree description"| L
    E --> |"Read docs/wiki/*convention*<br/>or use defaults"| L
    F --> |"Read docs/glossary.md<br/>or use defaults"| L
    G --> |"Scan docs/decisions/*.md<br/>for accepted ADRs"| L
    H --> |"BacklogManager.FilterTasks<br/>active statuses"| L
    I --> |"Read tickets/*/knowledge/decisions.yaml<br/>from active tasks"| L
    J --> |"Read latest session from<br/>tickets/*/sessions/"| L
    J2 --> |"Read recent captured sessions<br/>from workspace sessions/ store"| L
    J3 --> |"Load .context_state.yaml,<br/>compute diff, render changes"| L
    K --> |"Check docs/stakeholders.md<br/>and docs/contacts.md"| L

    L --> M{Target AI type?}
    M --> |claude| N["Write CLAUDE.md"]
    M --> |kiro| O["Write kiro.md"]

    N --> P[Done]
    O --> P
```

### Section data sources

| Section | Data Source |
|---------|-----------|
| Project Overview | Hardcoded summary |
| Directory Structure | Hardcoded tree description |
| Conventions | `docs/wiki/*convention*.md` files, else defaults |
| Glossary | `docs/glossary.md`, else defaults |
| Decisions Summary | `docs/decisions/*.md` (accepted ADRs only) |
| Active Tasks | `backlog.yaml` filtered by active statuses |
| Critical Decisions | `tickets/*/knowledge/decisions.yaml` for active tasks |
| Recent Sessions | Latest `.md` file from `tickets/*/sessions/` for active tasks (first 20 lines) |
| Captured Sessions | Recent captured sessions from workspace-level `sessions/` store (via `SessionCapturer` interface) |
| What's Changed | Semantic diff: loads `.context_state.yaml`, computes current state, diffs, renders changes. Appends to `.context_changelog.md` (pruned to 50 entries). |
| Stakeholders/Contacts | `docs/stakeholders.md`, `docs/contacts.md` |

---

## Session Capture Flow

When a Claude Code session ends, the `SessionEnd` hook fires and triggers automatic session capture. The hook is installed at the user level by `adb sync-claude-user`, enabling workspace-wide capture without per-project setup.

```mermaid
sequenceDiagram
    participant CC as Claude Code
    participant Hook as SessionEnd Hook
    participant CLI as adb session capture
    participant TP as TranscriptParser
    participant SS as StructuralSummarizer
    participant SM as SessionStoreManager
    participant FS as Filesystem

    CC->>Hook: Session ends (pipes metadata to stdin)
    Hook->>CLI: adb session capture --from-hook

    CLI->>CLI: Read session metadata from stdin (JSON)
    CLI->>CLI: Locate JSONL transcript file

    CLI->>TP: ParseTranscript(transcriptPath)
    TP->>TP: Scan JSONL line by line
    TP->>TP: Skip meta, thinking, unknown types
    TP->>TP: Extract user/assistant turns, tools
    TP-->>CLI: TranscriptResult{Turns, ToolsUsed, Summary}

    CLI->>CLI: Check min_turns_capture threshold
    Note over CLI: Skip sessions with fewer than 3 turns (default)

    CLI->>SS: Summarize(turns)
    SS-->>CLI: "Help me fix login bug — 5 turns, tools: Read(3), Edit(1)"

    CLI->>SM: GenerateID()
    SM-->>CLI: "S-00042"

    CLI->>SM: AddSession(session, turns)
    SM->>FS: Write sessions/S-00042/session.yaml
    SM->>FS: Write sessions/S-00042/turns.yaml
    SM->>FS: Write sessions/S-00042/summary.md
    SM->>SM: Update sessions/index.yaml

    CLI->>CLI: Check ADB_TASK_ID env var
    opt Task-scoped linking
        CLI->>FS: Symlink/copy to tickets/TASK-XXXXX/sessions/
    end

    CLI-->>Hook: exit 0
    Hook-->>CC: (non-blocking, always exits 0)
```

### Session storage format

Each captured session is stored in its own subdirectory under `sessions/`:

```
sessions/
  index.yaml                  # Master index of all captured sessions
  .session_counter            # Sequential counter for S-XXXXX IDs
  S-00001/
    session.yaml              # Session metadata (CapturedSession)
    turns.yaml                # Full turn data ([]SessionTurn)
    summary.md                # Structural or LLM-generated summary
  S-00002/
    ...
```

The `index.yaml` is a lightweight lookup table mapping session IDs to metadata. Full turn data is stored separately in `turns.yaml` to keep the index small.

---

## Context Evolution Tracking

The context evolution system tracks how the project context changes between `sync-context` runs. This enables AI assistants to see a "What's Changed" section showing recent context drift.

```mermaid
flowchart TD
    A["adb sync-context called"] --> B["loadState(.context_state.yaml)"]
    B --> C{"State file exists?"}
    C -->|No| D["First sync: previousState = empty"]
    C -->|Yes| E["previousState loaded"]
    D --> F["computeCurrentState()"]
    E --> F

    F --> G["Count active tasks, knowledge entries,<br/>ADR titles, section hashes (FNV-1a)"]
    G --> H["diffStates(previous, current)"]

    H --> I{"Changes detected?"}
    I -->|No| J["Skip What's Changed section"]
    I -->|Yes| K["renderWhatsChanged(changes)"]

    K --> L["Include in CLAUDE.md"]
    K --> M["appendChangelog(changes)"]
    M --> N[".context_changelog.md<br/>(pruned to 50 entries)"]

    H --> O["saveState(.context_state.yaml)"]

    style N fill:#9b59b6,color:#fff
```

### State tracking fields

The `ContextState` struct captures a snapshot of the project context:

| Field | Type | Description |
|-------|------|-------------|
| `SyncedAt` | timestamp | When this state was captured |
| `ActiveTaskIDs` | []string | IDs of tasks in active statuses |
| `KnowledgeCount` | int | Number of knowledge entries |
| `DecisionCount` | int | Number of decisions across active tasks |
| `SessionCount` | int | Number of captured sessions |
| `ADRTitles` | []string | Titles of accepted ADRs |
| `SectionHashes` | map[string]string | FNV-1a hashes of static sections (glossary, conventions) |

### Semantic diff types

The `diffStates` function produces `[]ContextChange` entries describing what changed:

- Tasks added or completed since last sync
- New knowledge entries or decisions added
- New captured sessions
- New or removed ADRs
- Static section changes (glossary, conventions) detected via hash comparison
- First-time sync notification

---

## Hook System Architecture

The hook system replaces standalone shell-script hooks with compiled Go code running inside the `adb` binary. A thin shell wrapper delegates to `adb hook <type>`, which processes the event and updates adb artifacts.

### Execution Model

```mermaid
sequenceDiagram
    participant CC as Claude Code
    participant SH as Shell Wrapper
    participant ADB as adb hook <type>
    participant HE as HookEngine
    participant CT as ChangeTracker
    participant FS as Filesystem

    CC->>SH: Hook fires (pipes JSON to stdin)
    SH->>SH: Check ADB_HOOK_ACTIVE
    alt ADB_HOOK_ACTIVE=1
        SH-->>CC: exit 0 (prevent recursion)
    else Not set
        SH->>SH: export ADB_HOOK_ACTIVE=1
        SH->>ADB: pipe stdin to adb hook <type>
        ADB->>ADB: ParseStdin[T](os.Stdin)
        ADB->>HE: Handle<Type>(input)

        alt PreToolUse / TaskCompleted (blocking)
            HE->>HE: Run validation checks
            alt Check fails
                HE-->>ADB: error
                ADB->>ADB: fmt.Fprintln(stderr, err)
                ADB-->>SH: exit 2
                SH-->>CC: exit 2 (block operation)
            else Check passes
                HE-->>ADB: nil
                ADB-->>SH: exit 0
                SH-->>CC: exit 0 (allow operation)
            end
        else PostToolUse / Stop / SessionEnd (non-blocking)
            HE->>CT: Append change / Read changes
            HE->>FS: Update context.md, status.yaml
            HE-->>ADB: nil (always)
            ADB-->>SH: exit 0
            SH-->>CC: exit 0
        end
    end
```

### Change Tracker Data Flow

The change tracker is the key coordination mechanism between hooks within a session:

```mermaid
flowchart TD
    PTU["PostToolUse fires<br/>(on each Edit/Write)"] --> APPEND["tracker.Append()<br/>timestamp|tool|filepath"]
    APPEND --> FILE[".adb_session_changes<br/>(append-only file)"]

    STOP["Stop hook fires"] --> READ1["tracker.Read()"]
    READ1 --> FILE
    READ1 --> GROUP1["GroupChangesByDirectory()"]
    GROUP1 --> FMT1["FormatSessionSummary()"]
    FMT1 --> CTX1["AppendToContext(ticketPath)"]
    CTX1 --> CTXMD["context.md updated"]
    STOP --> CLEAN1["tracker.Cleanup()"]
    CLEAN1 --> FILE

    SE["SessionEnd hook fires"] --> READ2["tracker.Read()"]
    READ2 --> FILE
    READ2 --> GROUP2["GroupChangesByDirectory()"]
    GROUP2 --> FMT2["FormatSessionSummary()"]
    FMT2 --> CTX2["AppendToContext(ticketPath)"]
    CTX2 --> CTXMD

    style FILE fill:#e74c3c,color:#fff
    style CTXMD fill:#2ecc71,color:#fff
```

### Hook Types and Behavior

| Hook Type | Blocking | Exit Code | Key Actions |
|-----------|----------|-----------|-------------|
| PreToolUse | Yes | 0 or 2 | Vendor guard, go.sum guard, architecture guard, ADR conflict check |
| PostToolUse | No | Always 0 | Go format, change tracking, dependency detection |
| Stop | No | Always 0 | Advisory build/vet/uncommitted, context update, status timestamp, tracker cleanup |
| TaskCompleted | Yes (Phase A) | 0 or 2 | Phase A: tests/lint/uncommitted. Phase B: knowledge/wiki/ADR/context |
| SessionEnd | No | Always 0 | Context update from tracked changes |
| TeammateIdle | No | Always 0 | No-op |

### Configuration

Hook behavior is controlled via `.taskconfig` under the `hooks:` key. Each hook type has an `enabled` flag plus feature-specific flags. `DefaultHookConfig()` enables Phase 1 features; Phase 2/3 features are opt-in.

The `HookEngine` receives its configuration at construction time via `NewHookEngine(basePath, config, knowledgeX, conflictDt)`. The `knowledgeX` (KnowledgeExtractor) and `conflictDt` (ConflictDetector) parameters are optional -- Phase 2/3 features are disabled when nil.

---

## Configuration Loading Flow

Configuration follows a three-level precedence chain: per-repo `.taskrc` overrides global `.taskconfig`, which overrides built-in defaults. The `ConfigurationManager` uses Viper for YAML parsing.

```mermaid
flowchart TD
    A[GetMergedConfig called with repoPath] --> B[LoadGlobalConfig]

    B --> C{".taskconfig exists?"}
    C -->|No| D[Use built-in defaults]
    C -->|Yes| E[Viper reads .taskconfig YAML]

    D --> F[GlobalConfig]
    E --> F

    F --> G{repoPath provided?}
    G -->|No| H[Return MergedConfig with GlobalConfig only]
    G -->|Yes| I[LoadRepoConfig]

    I --> J{".taskrc exists in repoPath?"}
    J -->|No| H
    J -->|Yes| K[Viper reads .taskrc YAML]

    K --> L[MergedConfig with Repo overlay]
    L --> M[Return MergedConfig]
    H --> M

    subgraph "Precedence: highest to lowest"
        P1[".taskrc (per-repo)"]
        P2[".taskconfig (global)"]
        P3["Built-in defaults"]
        P1 -.-> P2 -.-> P3
    end
```

### Key configuration fields

| Source | Fields |
|--------|--------|
| `.taskconfig` (global) | `defaults.ai`, `defaults.priority`, `defaults.owner`, `task_id.prefix`, `task_id.counter`, `screenshot.hotkey`, `offline_mode`, `cli_aliases[]` |
| `.taskrc` (per-repo) | `build_command`, `test_command`, `default_reviewers[]`, `conventions[]`, `templates{}` |
| Built-in defaults | `ai=kiro`, `priority=P2`, `prefix=TASK`, `hotkey=ctrl+shift+s` |

---

## Observability Architecture

The observability layer (`internal/observability/`) provides structured event logging, on-demand metrics derivation, and threshold-based alerting. It operates on an append-only JSONL event log and requires no external services. The core package accesses the event log through a narrow `EventLogger` interface, following the same local interface pattern used for storage and integration decoupling.

```mermaid
flowchart TD
    subgraph "Event Sources"
        CLI["CLI Commands"]
        TM["TaskManager"]
        KE["KnowledgeExtractor"]
        FL["FeedbackLoopOrchestrator"]
    end

    subgraph "Observability Layer"
        EL["EventLog<br/>(JSONL append-only)"]
        MC["MetricsCalculator"]
        AE["AlertEngine"]
        NT["Notifier"]
    end

    subgraph "Persistence"
        JSONL[".adb_events.jsonl"]
    end

    CLI -->|"Write(Event)"| EL
    TM -->|"LogEvent() via adapter"| EL
    KE -->|"LogEvent() via adapter"| EL
    FL -->|"LogEvent() via adapter"| EL

    EL -->|"append JSON + newline"| JSONL

    MC -->|"Read(filter)"| EL
    AE -->|"Read(filter)"| EL

    MC -->|"Aggregate counts,<br/>status transitions"| METRICS["Metrics struct"]
    AE -->|"Check thresholds"| ALERTS["[]Alert"]
    ALERTS -->|"Notify()"| NT

    style EL fill:#e74c3c,color:#fff
    style MC fill:#e74c3c,color:#fff
    style AE fill:#e74c3c,color:#fff
    style NT fill:#e74c3c,color:#fff
    style JSONL fill:#c0392b,color:#fff
```

### Event structure

Every event is a JSON object with a fixed schema, written as a single line in the JSONL file:

```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "INFO",
  "type": "task.created",
  "msg": "task.created",
  "data": {
    "task_id": "TASK-00042",
    "type": "feat",
    "branch": "add-user-auth"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `time` | RFC 3339 timestamp | When the event occurred (UTC) |
| `level` | string | Severity: `INFO`, `WARN`, `ERROR` |
| `type` | string | Dot-namespaced event type (e.g., `task.created`, `task.status_changed`, `agent.session_started`, `knowledge.extracted`) |
| `msg` | string | Human-readable message |
| `data` | object | Arbitrary key-value payload (event-type-specific) |

### Event types

| Event Type | Trigger | Data Fields |
|------------|---------|-------------|
| `task.created` | New task bootstrapped | `task_id`, `type`, `branch` |
| `task.completed` | Task status set to `done` | `task_id` |
| `task.status_changed` | Any status transition | `task_id`, `old_status`, `new_status` |
| `agent.session_started` | AI agent begins a session | `task_id`, `agent` |
| `knowledge.extracted` | Knowledge extracted on archive | `task_id`, `learnings_count`, `decisions_count` |

### Metrics calculation

The `MetricsCalculator` reads all events since a given time and produces an aggregated `Metrics` struct:

| Metric | Derivation |
|--------|------------|
| `TasksCreated` | Count of `task.created` events |
| `TasksCompleted` | Count of `task.completed` events |
| `TasksByStatus` | Count of `task.status_changed` events grouped by `new_status` |
| `TasksByType` | Count of `task.created` events grouped by `type` |
| `AgentSessions` | Count of `agent.session_started` events |
| `KnowledgeExtracted` | Count of `knowledge.extracted` events |
| `EventCount` | Total events in the time range |
| `OldestEvent` / `NewestEvent` | Timestamp boundaries |

### Alert evaluation

The `AlertEngine` reads events, reconstructs current task state, and evaluates four threshold-based conditions:

| Alert Condition | Severity | Default Threshold | Description |
|----------------|----------|-------------------|-------------|
| `task_blocked_too_long` | high | 24 hours | A task has been in `blocked` status longer than the threshold |
| `task_stale` | medium | 3 days | An `in_progress` task has had no activity beyond the threshold |
| `review_too_long` | medium | 5 days | A task has been in `review` status beyond the threshold |
| `backlog_too_large` | low | 10 tasks | More tasks in `backlog` status than the configured maximum |

Thresholds are configurable via `.taskconfig`:

```yaml
notifications:
  alerts:
    blocked_threshold_hours: 24
    stale_threshold_days: 3
    review_threshold_days: 5
    max_backlog_size: 10
```

### Graceful degradation

If the JSONL event log file cannot be opened during `NewApp`, observability is disabled entirely (non-fatal). The `EventLog`, `AlertEngine`, and `MetricsCalculator` fields on `App` are set to `nil`, and the CLI skips observability operations when these are absent. This ensures that a permissions error or full disk never prevents normal task management.

---

## Storage and Directory Structure

All data is persisted as human-readable files (YAML, Markdown, and JSONL) under a single base directory. There is no database. This design is intentional: files are git-friendly, diff-able, and require no runtime dependencies.

```mermaid
graph TD
    ROOT[".adb/ or project root"]

    ROOT --> TC[".taskconfig<br/>(global YAML config)"]
    ROOT --> COUNTER[".task_counter<br/>(integer file)"]
    ROOT --> BACKLOG["backlog.yaml<br/>(central task registry)"]
    ROOT --> EVENTS[".adb_events.jsonl<br/>(observability event log)"]
    ROOT --> CLAUDE["CLAUDE.md / kiro.md<br/>(AI context files)"]
    ROOT --> CTXSTATE[".context_state.yaml<br/>(context evolution state)"]
    ROOT --> CTXLOG[".context_changelog.md<br/>(context change log)"]
    ROOT --> SESSCTR[".session_counter<br/>(session ID counter)"]
    ROOT --> MCP[".mcp.json<br/>(MCP server config)"]

    ROOT --> WSESS["sessions/<br/>(captured sessions)"]
    WSESS --> SESSIDX["index.yaml<br/>(session index)"]
    WSESS --> SESSDIR["S-00001/<br/>(per-session dir)"]
    SESSDIR --> SESSYAML["session.yaml"]
    SESSDIR --> SESSTURNS["turns.yaml"]
    SESSDIR --> SESSSUMM["summary.md"]

    ROOT --> TICKETS["tickets/"]
    TICKETS --> T1["TASK-00001/<br/>(active task)"]
    T1 --> STATUS["status.yaml"]
    T1 --> CONTEXT["context.md"]
    T1 --> NOTES["notes.md"]
    T1 --> DESIGN["design.md"]
    T1 --> COMMS["communications/"]
    COMMS --> COMM1["2025-01-15-slack-alice-api-design.md"]
    COMMS --> COMM2["2025-01-16-email-bob-review.md"]
    T1 --> SESSIONS["sessions/"]
    SESSIONS --> SESS1["2025-01-15-session.md"]
    T1 --> KNOWLEDGE["knowledge/"]
    KNOWLEDGE --> KDEC["decisions.yaml"]

    TICKETS --> ARCHIVED["_archived/"]
    ARCHIVED --> AT1["TASK-00002/<br/>(archived task)"]
    AT1 --> ASTATUS["status.yaml"]
    AT1 --> AHANDOFF["handoff.md"]
    AT1 --> ADESIGN["design.md"]

    ROOT --> CHANNELS["channels/"]
    CHANNELS --> CHINBOX["inbox/<br/>(pending channel items)"]
    CHANNELS --> CHOUTBOX["outbox/<br/>(outgoing items)"]
    CHANNELS --> CHPROCESSED["processed/<br/>(archived items)"]

    ROOT --> REPOS["repos/"]
    REPOS --> PLATFORM["github.com/"]
    PLATFORM --> ORG["org/"]
    ORG --> REPO["repo/"]
    REPO --> WORK["work/"]
    WORK --> WT1["TASK-00001/<br/>(git worktree)"]
    WT1 --> CLRULES[".claude/rules/<br/>task-context.md"]

    ROOT --> DOCS["docs/"]
    DOCS --> WIKI["wiki/<br/>(extracted knowledge)"]
    DOCS --> DECISIONS["decisions/<br/>(ADR-XXXX-*.md)"]
    DOCS --> KSTORE["knowledge/<br/>(long-term knowledge store)"]
    KSTORE --> KIDX["index.yaml"]
    KSTORE --> KTOP["topics.yaml"]
    KSTORE --> KENT["entities.yaml"]
    KSTORE --> KTMR["timeline.yaml"]
    DOCS --> GLOSSARY["glossary.md"]
    DOCS --> STAKEHOLDERS["stakeholders.md"]
    DOCS --> CONTACTS["contacts.md"]

    style ROOT fill:#34495e,color:#fff
    style TICKETS fill:#e67e22,color:#fff
    style ARCHIVED fill:#95a5a6,color:#fff
    style CHANNELS fill:#f39c12,color:#fff
    style REPOS fill:#2ecc71,color:#fff
    style DOCS fill:#3498db,color:#fff
    style EVENTS fill:#c0392b,color:#fff
    style WSESS fill:#9b59b6,color:#fff
    style CTXSTATE fill:#9b59b6,color:#fff
    style CTXLOG fill:#9b59b6,color:#fff
    style SESSCTR fill:#9b59b6,color:#fff
    style SESSIONS fill:#e67e22,color:#fff
    style KNOWLEDGE fill:#e67e22,color:#fff
    style KSTORE fill:#3498db,color:#fff
```

### File format reference

**backlog.yaml** -- Central task registry:

```yaml
version: "1.0"
tasks:
  TASK-00001:
    id: TASK-00001
    title: Add authentication
    status: in_progress
    priority: P1
    owner: alice
    repo: github.com/org/repo
    branch: feat/TASK-00001-auth
    created: "2025-01-15T10:00:00Z"
    tags: [security, backend]
    blocked_by: []
    related: [TASK-00002]
```

**status.yaml** -- Per-task metadata:

```yaml
id: TASK-00001
title: Add authentication
type: feat
status: in_progress
priority: P1
owner: alice
repo: github.com/org/repo
branch: feat/TASK-00001-auth
worktree: /home/user/.adb/repos/github.com/org/repo/work/TASK-00001
ticket_path: /home/user/.adb/tickets/TASK-00001
created: 2025-01-15T10:00:00Z
updated: 2025-01-15T14:30:00Z
```

**context.md** -- AI-maintained task context:

```markdown
# Task Context: TASK-00001

## Summary
## Current Focus
## Recent Progress
## Open Questions
## Decisions Made
## Blockers
## Next Steps
## Related Resources
```

**Communication files** -- Chronological markdown in `communications/`:

```markdown
# 2025-01-15-slack-alice-api-design.md

**Date:** 2025-01-15
**Source:** slack
**Contact:** alice
**Topic:** API design

## Content
Discussed REST vs GraphQL approach...

## Tags
- decision
- requirement
```

**.adb_events.jsonl** -- Append-only observability event log:

```jsonl
{"time":"2025-01-15T10:00:00Z","level":"INFO","type":"task.created","msg":"task.created","data":{"task_id":"TASK-00001","type":"feat","branch":"add-auth"}}
{"time":"2025-01-15T10:05:00Z","level":"INFO","type":"task.status_changed","msg":"task.status_changed","data":{"task_id":"TASK-00001","old_status":"backlog","new_status":"in_progress"}}
{"time":"2025-01-15T14:30:00Z","level":"INFO","type":"agent.session_started","msg":"agent.session_started","data":{"task_id":"TASK-00001","agent":"claude"}}
```

Each line is an independent JSON object. The file is opened in append-only mode and writes are mutex-protected. Reads scan line-by-line and skip malformed entries, making the log resilient to partial writes.

**Session summaries** -- Markdown files in `tickets/TASK-XXXXX/sessions/`:

```markdown
# Session: 2025-01-15

## What was accomplished
- Implemented authentication middleware
- Added JWT token validation

## Decisions made
- Use RS256 for token signing

## Open questions
- How to handle token refresh for mobile clients?

## Next steps
- Add refresh token endpoint
```

Session files are named with timestamps (e.g., `2025-01-15-session.md`) and sorted lexicographically. The AI context generator reads the latest session file per active task to provide continuity across AI assistant sessions.

**Knowledge decisions** -- YAML in `tickets/TASK-XXXXX/knowledge/decisions.yaml`:

```yaml
- decision: Use RS256 for JWT token signing
  rationale: Asymmetric keys allow verification without sharing the signing key
  date: 2025-01-15
  status: accepted
```

These per-task decisions are surfaced in the "Critical Decisions" section of the generated AI context files and may be promoted to ADRs during knowledge extraction on archive.

**Per-worktree task context** -- `work/TASK-XXXXX/.claude/rules/task-context.md`:

Generated automatically during bootstrap, this file gives AI assistants immediate awareness of the task they are working on, including task ID, type, branch, and pointers to key files (`context.md`, `notes.md`, `design.md`, `sessions/`, `knowledge/`).

---

## Design Decisions

### File-based storage with no database

All state is stored in YAML and Markdown files. This was chosen because:

- **Git-friendly**: Every change to task state, communications, and context is visible in `git diff` and can be committed alongside code.
- **Human-readable**: Developers can inspect and manually edit any file with a text editor.
- **Zero runtime dependencies**: No database server, no migrations, no connection strings.
- **Portable**: Copy the directory to move the entire workspace.

The tradeoff is that concurrent writes are not safe (single-user assumption) and query performance is O(n) over files rather than indexed.

### Single binary distribution

The `adb` CLI compiles to a single Go binary with no external runtime dependencies. The only external tools it shells out to are `git` (for worktrees) and OS-specific screenshot utilities. This minimizes installation friction: download the binary, place it on `PATH`, done.

### Interface-based design for testability

Every component is defined by a Go interface. Concrete implementations are unexported. This enables:

- **Unit testing with fakes**: Any dependency can be replaced with an in-memory implementation.
- **Adapter pattern**: The core layer does not import the storage or integration packages. Adapters in `internal/app.go` bridge the gap, keeping the dependency graph acyclic.
- **Open/closed principle**: New storage backends or integration targets can be added by implementing an existing interface.

### Property-based testing

The project uses `pgregory.net/rapid` for property-based testing alongside standard unit tests. Property tests verify invariants that must hold for all possible inputs:

- **Property 26 (CLI Argument Passthrough)**: For any alias and argument list, the resolved command preserves argument order with default args prepended.
- **Property 27 (Task Context Environment Injection)**: For any `TaskEnvContext`, `BuildEnv` injects exactly four `ADB_*` variables; with nil context, none appear.
- **Property 29 (CLI Alias Resolution)**: Known aliases resolve to their configured command; unknown aliases pass through unchanged.
- **Property 30 (CLI Failure Propagation)**: Non-zero exit codes are captured and logged to the task's context when a task is active.

Property testing catches edge cases that example-based tests miss, particularly around string handling, argument ordering, and environment variable construction.

### Local interface definitions to avoid import cycles

The core package defines narrow local interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`, `WorktreeRemover`, `EventLogger`, `KnowledgeStoreAccess`, `SessionCapturer`) that mirror subsets of the storage, integration, and observability interfaces. This pattern is idiomatic Go: define the interface where it is consumed, not where it is implemented. It keeps the core package's `import` list free of storage, integration, and observability packages, preventing circular dependencies.

### JSONL for event logging

The observability event log uses JSON Lines (JSONL) -- one JSON object per line, appended to a single file. This was chosen because:

- **Append-only**: Writes never modify existing data, making the log safe against partial writes and corruption. Malformed lines are silently skipped on read.
- **Human-inspectable**: Events can be read with standard tools (`cat`, `jq`, `grep`).
- **No external dependencies**: No need for a time-series database or log aggregation service.
- **Git-friendly**: While the file grows over time, it can be `.gitignored` for repos that do not want to track operational data.

The tradeoff is that reads scan the entire file (O(n)), which is acceptable for the single-user, local-first design of `adb`. For workspaces with very large event logs, time-based filtering (`EventFilter.Since`) limits the scan window.

### Per-worktree task context files

When a new task is bootstrapped, `generateTaskContext` writes a `.claude/rules/task-context.md` file inside the git worktree. This gives AI coding assistants immediate awareness of the task context without requiring the AI to search for the ticket directory. This is non-fatal: if the write fails (e.g., the worktree is read-only), bootstrap continues normally. The file contains the task ID, type, branch, and pointers to key files.

### Per-task sessions and knowledge directories

Each task ticket now includes `sessions/` and `knowledge/` directories alongside the existing `communications/` directory:

- **sessions/**: Stores markdown session summaries, allowing AI assistants to save progress between sessions and resume with continuity. The AI context generator reads the latest session file to include in generated context.
- **knowledge/**: Stores structured knowledge artifacts like `decisions.yaml`, which are surfaced in the "Critical Decisions" section of AI context files and fed into the knowledge extraction pipeline on archive.

These directories are created during bootstrap and provide a structured way to accumulate per-task intelligence that was previously only captured informally in `context.md`.

### Automatic session capture via SessionEnd hook

Claude Code sessions are captured automatically via a user-level `SessionEnd` hook. Design choices:

- **SessionEnd over Stop hook**: SessionEnd fires once per session, cannot block the user, and is non-intrusive. Stop fires on every conversation stop (including mid-session pauses).
- **User-level installation**: `adb sync-claude-user` installs the hook in `~/.claude/settings.json`, enabling workspace-wide capture without per-project configuration.
- **Thin hook + thick binary**: The hook script is 4 lines of bash that pipes stdin to `adb session capture --from-hook`. All parsing, storage, and summarization logic lives in testable Go code.
- **Structural summary as default**: The `StructuralSummarizer` produces a zero-dependency summary (first user message + tool counts). LLM-powered summarization is designed but deferred to Phase 2.
- **min_turns_capture threshold**: Sessions with fewer than 3 turns (default) are skipped to avoid capturing trivial interactions (e.g., "what time is it?").
- **Symlink for task-scoped linking**: When `ADB_TASK_ID` is set, the captured session is symlinked into the task's `sessions/` directory, maintaining a single source of truth.

### Context evolution via state snapshots

Context evolution tracking uses a lightweight state-comparison approach:

- **FNV-1a hashing**: Static sections (glossary, conventions) are hashed with FNV-1a (fast, non-cryptographic) to detect changes without storing full content.
- **50-entry changelog pruning**: The `.context_changelog.md` is pruned to 50 entries (~2-3 weeks of daily syncs), balancing history retention with file size.
- **Semantic diffs over text diffs**: `diffStates` produces human-readable change descriptions ("2 tasks added", "Glossary section changed") rather than raw text diffs.
- **sync-context as sole trigger**: Context state is only updated during `adb sync-context`, avoiding mid-session context rewrites.

### Embedded Claude templates for portability

Claude Code configuration templates (skills, agents, hooks, rules, and config templates) are embedded directly into the `adb` binary using Go's `//go:embed` directive. This was chosen because:

- **Self-contained binary**: No need to ship a separate `templates/` directory alongside the binary or resolve template paths at runtime.
- **Simplified installation**: Users download a single binary with all templates included. No risk of missing template files breaking `adb init-claude` or `adb sync-claude-user`.
- **Version consistency**: Templates are locked to the binary version. Upgrading `adb` upgrades templates atomically.
- **Cross-platform path handling**: Embedded filesystems always use forward slashes (`path.Join`), eliminating OS-specific path separator issues.

The `templates/claude` package exports an `embed.FS` named `FS` containing all template files. CLI commands (`internal/cli/syncclaudeuser.go`, `internal/cli/initclaude.go`) import `github.com/valter-silva-au/ai-dev-brain/templates/claude` and read from `claudetpl.FS` instead of the disk filesystem.

The tradeoff is that template customization requires recompiling the binary. For users who need per-project template overrides, `.taskrc` still supports custom template paths for task templates (notes.md, design.md) as described in the Extension Points section.

### Hybrid hook execution model

The hook system uses a hybrid shell-wrapper + Go binary approach:

- **Thin shell wrappers**: Each hook type has a 4-line bash script that checks `ADB_HOOK_ACTIVE`, sets it, pipes stdin to `adb hook <type>`, and propagates the exit code. This matches the proven `adb-session-capture.sh` pattern.
- **Compiled Go logic**: All validation, formatting, tracking, and knowledge work runs in the `adb` binary. This provides type safety, testability, and access to all adb packages.
- **Recursive hook prevention**: The `ADB_HOOK_ACTIVE` environment variable prevents infinite loops (e.g., PostToolUse -> gofmt writes file -> PostToolUse fires again). The shell wrapper checks `ADB_HOOK_ACTIVE` before exporting it.
- **Graceful degradation**: If `adb` is not on PATH or fails to execute, the shell wrapper exits 0 (non-blocking hooks) or propagates the error (blocking hooks). Claude Code continues either way.

The alternative approaches considered were:
1. **Pure shell scripts**: Limited logic, no access to adb packages, hard to test, brittle string manipulation.
2. **Pure adb binary** (registered directly in settings.json): Would require the Go binary to be the hook script itself. Not supported by Claude Code's hook contract (expects a shell command).

### Change tracker pattern

The `.adb_session_changes` file serves as a coordination mechanism between PostToolUse and Stop/SessionEnd hooks:

- **Append-only**: Each PostToolUse appends one line (`timestamp|tool|filepath`). No reads during writes.
- **Batched consumption**: Stop and SessionEnd read all entries, group by directory, format a summary, and append to context.md. This produces clean, batched summaries instead of per-file noise.
- **Cleanup after consumption**: The tracker file is deleted after Stop consumes it, resetting for the next session.
- **Malformed line tolerance**: The reader silently skips lines that don't parse as `timestamp|tool|filepath`, making the format resilient to partial writes.

### Two-phase TaskCompleted gate

TaskCompleted uses a deliberate two-phase architecture:

- **Phase A (blocking)**: Quality gates (uncommitted check, tests, lint) that must pass before task completion is allowed. Exits with code 2 on failure.
- **Phase B (non-blocking)**: Knowledge extraction, wiki updates, and ADR generation. Failures are logged to stderr but do not prevent task completion.

This separation ensures that a knowledge extraction bug never blocks a developer from completing their task.

---

## Extension Points

### Custom templates via .taskrc

Per-repository template overrides can be configured in `.taskrc`:

```yaml
templates:
  feat: path/to/custom-feature-template.md
  bug: path/to/custom-bug-template.md
```

When `TemplateManager.ApplyTemplate` runs, it checks for a registered custom template before falling back to the built-in defaults. Custom templates are plain Markdown files optionally containing Go `text/template` placeholders (e.g., `{{.TaskID}}`).

The four built-in task types each have their own notes and design templates:
- `feat` -- Requirements, acceptance criteria, implementation notes
- `bug` -- Steps to reproduce, root cause analysis, fix notes
- `spike` -- Research questions, findings, recommendations, time-box
- `refactor` -- Motivation, current state, target state, rollback plan

### CLI aliases

Aliases defined in `.taskconfig` allow short names for frequently used external commands:

```yaml
cli_aliases:
  - name: lint
    command: golangci-lint
    default_args: ["run", "--fix"]
  - name: test
    command: go
    default_args: ["test", "./..."]
```

Running `adb exec lint` resolves to `golangci-lint run --fix`. The `CLIExecutor` handles alias resolution, argument merging, and task context injection (`ADB_TASK_ID`, `ADB_BRANCH`, `ADB_WORKTREE_PATH`, `ADB_TICKET_PATH` environment variables).

### Taskfile.yaml integration

The `TaskfileRunner` discovers and executes tasks from a `Taskfile.yaml` in the current directory. Each task is a list of shell commands executed sequentially through the `CLIExecutor`, inheriting the task context environment:

```yaml
version: "3"
tasks:
  build:
    desc: Build the project
    cmds:
      - go build ./...
  check:
    desc: Run all checks
    cmds:
      - go vet ./...
      - go test ./...
```

Running `adb run build` discovers the Taskfile, finds the `build` task, and executes its commands in order.

### AI context files

The `AIContextGenerator` produces root-level context files (`CLAUDE.md`, `kiro.md`) that AI coding assistants can read for project awareness. Running `adb sync-context` regenerates these files by assembling sections from:

- The project overview
- The directory structure
- Coding conventions from `docs/wiki/`
- The glossary from `docs/glossary.md`
- Active ADR summaries from `docs/decisions/`
- Currently active tasks from `backlog.yaml`
- Stakeholder and contact information from `docs/`

Individual sections can be regenerated with `RegenerateSection(section)` without rebuilding the entire file.

### Claude Code templates

Claude Code configuration templates are embedded in the `adb` binary via the `templates/claude` package. The commands `adb init-claude` and `adb sync-claude-user` read from the embedded filesystem (`claudetpl.FS`) rather than resolving a template directory at runtime. This ensures:

- **Portable binary**: No external template directory required
- **Consistent behavior**: Templates version-locked to the binary
- **Simplified deployment**: Single-file distribution with all templates included

The embedded templates include:
- **Skills**: git workflow skills (commit, pr, push, review, sync, changelog)
- **Agents**: universal code-reviewer agent
- **Hooks**: SessionEnd hook for automatic session capture
- **Rules**: workspace.md, go-standards.md, cli-patterns.md
- **Config templates**: .claudeignore, settings.json

For per-project customization of task templates (notes.md, design.md), `.taskrc` template overrides (described above) remain available and are read from disk.

### Knowledge feedback loop

The `KnowledgeExtractor` creates a feedback loop from completed tasks back into organizational documentation:

1. **Wiki updates** -- Learnings tagged with `## Wiki Updates` in notes.md are written to `docs/wiki/`.
2. **ADR creation** -- Decisions tagged in communications are promoted to Architecture Decision Records in `docs/decisions/`.
3. **Runbook updates** -- Items tagged with `## Runbook Updates` feed into operational runbooks.
4. **Handoff documents** -- On archive, a `handoff.md` captures summary, completed work, open items, learnings, and decisions for the next person who picks up the work.

### Conflict detection

The `ConflictDetector` scans existing ADRs (`docs/decisions/`), previous task decisions (`tickets/*/design.md`), and stakeholder requirements (`docs/wiki/`) before proposed changes are applied. It uses keyword overlap analysis to flag potential conflicts, categorized by type (`adr_violation`, `previous_decision`, `stakeholder_requirement`) and severity (`high`, `medium`, `low`).

### Observability and alerting

The observability layer is designed for extensibility:

- **Custom event types**: Any component can write events with arbitrary `type` and `data` fields. The `MetricsCalculator` and `AlertEngine` only process known event types; unrecognized types are preserved in the log but do not affect metrics or alerts.
- **Configurable alert thresholds**: All alert thresholds (`blocked_threshold_hours`, `stale_threshold_days`, `review_threshold_days`, `max_backlog_size`) are configurable in `.taskconfig` under `notifications.alerts`. The `AlertEngine` applies these thresholds at evaluation time.
- **Notification webhooks**: The `Notifier` interface (`observability/notifier.go`) sends alerts to external channels. A Slack webhook implementation (`slackNotifier`) is provided. Alert results (`[]Alert`) are passed to `Notifier.Notify()` when configured.
- **Dashboard integration**: The `MetricsCalculator` returns a structured `Metrics` object that can be serialized to JSON and consumed by external dashboards or monitoring tools.

### MCP server configuration

The `.mcp.json` file at the project root configures Model Context Protocol (MCP) servers that AI coding assistants can connect to for enhanced capabilities:

```json
{
  "mcpServers": {
    "aws-knowledge": {
      "type": "http",
      "url": "https://knowledge-mcp.global.api.aws"
    },
    "context7": {
      "type": "http",
      "url": "https://mcp.context7.com/mcp"
    }
  }
}
```

This is a static configuration file read by AI assistants (e.g., Claude Code). It is not consumed by the `adb` binary itself. Adding new MCP servers is a matter of adding entries to this file.

---

## Package Reference

| Package | Responsibility | Key Interfaces |
|---------|---------------|----------------|
| `cmd/adb` | Binary entrypoint | -- |
| `internal` | Composition root, adapters | `App` struct, `backlogStoreAdapter`, `contextStoreAdapter`, `worktreeAdapter`, `worktreeRemoverAdapter`, `eventLogAdapter`, `knowledgeStoreAdapter`, `sessionCapturerAdapter` |
| `internal/cli` | Cobra command definitions | -- |
| `internal/core` | Business logic | `TaskManager`, `BootstrapSystem`, `ConfigurationManager`, `KnowledgeExtractor`, `ConflictDetector`, `AIContextGenerator`, `UpdateGenerator`, `TaskDesignDocGenerator`, `TaskIDGenerator`, `TemplateManager`, `ProjectInitializer`, `KnowledgeManager`, `FeedbackLoopOrchestrator`, `ChannelAdapter`, `ChannelRegistry`, `HookEngine`, `EventLogger`, `BacklogStore`, `ContextStore`, `WorktreeCreator`, `WorktreeRemover`, `KnowledgeStoreAccess`, `SessionCapturer` |
| `internal/hooks` | Hook support library | `PreToolUseInput`, `PostToolUseInput`, `StopInput`, `TaskCompletedInput`, `SessionEndInput`, `ParseStdin[T]`, `ChangeTracker`, `AppendToContext`, `UpdateStatusTimestamp`, `GroupChangesByDirectory`, `FormatSessionSummary` |
| `internal/observability` | Event logging, metrics, alerting, notifications | `EventLog`, `MetricsCalculator`, `AlertEngine`, `Notifier` |
| `internal/storage` | File-based persistence | `BacklogManager`, `ContextManager`, `CommunicationManager`, `KnowledgeStoreManager`, `SessionStoreManager` |
| `internal/integration` | External system interaction | `GitWorktreeManager`, `OfflineManager`, `TabManager`, `ScreenshotPipeline`, `CLIExecutor`, `TaskfileRunner`, `TranscriptParser` |
| `templates/claude` | Embedded Claude Code templates | `FS` (embed.FS containing skills, agents, hooks, rules, config templates) |
| `pkg/models` | Shared data types | `Task`, `GlobalConfig`, `RepoConfig`, `MergedConfig`, `Communication`, `ExtractedKnowledge`, `HandoffDocument`, `Decision`, `HookConfig`, `PreToolUseConfig`, `PostToolUseConfig`, `StopConfig`, `TaskCompletedConfig`, `SessionEndConfig`, `SessionChangeEntry`, `KnowledgeEntry`, `KnowledgeIndex`, `Topic`, `TopicGraph`, `Entity`, `EntityRegistry`, `Timeline`, `TimelineEntry`, `ChannelItem`, `OutputItem`, `ChannelConfig`, `CapturedSession`, `SessionTurn`, `SessionFilter`, `SessionCaptureConfig`, `SessionIndex` |

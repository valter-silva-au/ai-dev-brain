# Architecture

This document describes the internal architecture of AI Dev Brain (`adb`), covering the layered package structure, interface contracts, data flow, storage design, dependency injection, and extension points.

---

## System Overview

AI Dev Brain is organized into four layers, each with a clear responsibility boundary. The CLI layer accepts user commands. The core layer contains all business logic. The integration layer communicates with external systems (git, OS tools, Taskfile). The storage layer persists data to the local filesystem. A shared `pkg/models` package defines the data structures used across layers.

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
    end

    subgraph "Integration Layer"
        GWM[GitWorktreeManager]
        OM[OfflineManager]
        TABM[TabManager]
        SP[ScreenshotPipeline]
        EXEC[CLIExecutor]
        TFR[TaskfileRunner]
    end

    subgraph "Storage Layer"
        BM[BacklogManager]
        CTX[ContextManager]
        COMM[CommunicationManager]
    end

    subgraph "Shared Models"
        MDL["pkg/models<br/>(Task, Config, Knowledge, Communication)"]
    end

    CLI --> TM
    CLI --> UG
    CLI --> ACG
    CLI --> EXEC
    CLI --> TFR

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
    TDG --> COMM
    CD --> MDL

    TFR --> EXEC

    BM --> MDL
    CTX --> MDL
    COMM --> MDL

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
    style GWM fill:#2ecc71,color:#fff
    style OM fill:#2ecc71,color:#fff
    style TABM fill:#2ecc71,color:#fff
    style SP fill:#2ecc71,color:#fff
    style EXEC fill:#2ecc71,color:#fff
    style TFR fill:#2ecc71,color:#fff
    style BM fill:#e67e22,color:#fff
    style CTX fill:#e67e22,color:#fff
    style COMM fill:#e67e22,color:#fff
    style MDL fill:#95a5a6,color:#fff
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

    TaskManager --> BootstrapSystem : delegates creation
    TaskManager --> BacklogStore : persists entries
    TaskManager --> ContextStore : loads context
    BootstrapSystem --> TaskIDGenerator : generates IDs
    BootstrapSystem --> WorktreeCreator : creates worktrees
    BootstrapSystem --> TemplateManager : applies templates
```

---

## Adapter Pattern and Dependency Injection

The core layer defines narrow "store" interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`) that mirror subsets of the storage and integration interfaces. This keeps the core package free of import dependencies on the storage and integration packages. The `App` struct bridges the gap using adapter structs.

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
    }

    namespace storage {
        class BacklogManager {
            <<interface>>
        }
        class ContextManager {
            <<interface>>
        }
    }

    namespace integration {
        class GitWorktreeManager {
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
    }

    backlogStoreAdapter ..|> BacklogStore : implements
    backlogStoreAdapter --> BacklogManager : delegates to

    contextStoreAdapter ..|> ContextStore : implements
    contextStoreAdapter --> ContextManager : delegates to

    worktreeAdapter ..|> WorktreeCreator : implements
    worktreeAdapter --> GitWorktreeManager : delegates to
```

The `NewApp` function in `internal/app.go` performs all wiring in a fixed order:

1. **Configuration** -- `ConfigurationManager` loads `.taskconfig` with Viper.
2. **Storage** -- `BacklogManager`, `ContextManager`, `CommunicationManager` are created with the base path.
3. **Integration** -- `GitWorktreeManager`, `OfflineManager`, `TabManager`, `ScreenshotPipeline`, `CLIExecutor`, `TaskfileRunner` are created.
4. **Core** -- Core services receive their dependencies through constructors, using adapter structs where cross-layer communication is needed.
5. **CLI wiring** -- Package-level variables in `internal/cli` are set to the core and integration service instances.

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

    BS->>BS: MkdirAll tickets/TASK-00042/communications/

    BS->>TMPL: ApplyTemplate(ticketPath, "feat")
    TMPL-->>BS: writes notes.md + design.md

    BS->>BS: Write context.md (scaffold)
    BS->>BS: Write status.yaml (Task struct)

    BS->>WT: CreateWorktree(config)
    WT-->>BS: worktree path

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
| MkdirAll | `tickets/TASK-00042/` and `tickets/TASK-00042/communications/` |
| ApplyTemplate | `tickets/TASK-00042/notes.md`, `tickets/TASK-00042/design.md` |
| Write context.md | `tickets/TASK-00042/context.md` |
| Write status.yaml | `tickets/TASK-00042/status.yaml` |
| CreateWorktree | `repos/{platform}/{org}/{repo}/work/TASK-00042/` |
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
    B --> I[assembleStakeholders + assembleContacts]

    C --> |"Static overview text"| J[AIContextSections]
    D --> |"Directory tree description"| J
    E --> |"Read docs/wiki/*convention*<br/>or use defaults"| J
    F --> |"Read docs/glossary.md<br/>or use defaults"| J
    G --> |"Scan docs/decisions/*.md<br/>for accepted ADRs"| J
    H --> |"BacklogManager.FilterTasks<br/>active statuses"| J
    I --> |"Check docs/stakeholders.md<br/>and docs/contacts.md"| J

    J --> K{Target AI type?}
    K --> |claude| L["Write CLAUDE.md"]
    K --> |kiro| M["Write kiro.md"]

    L --> N[Done]
    M --> N
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
| Stakeholders/Contacts | `docs/stakeholders.md`, `docs/contacts.md` |

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

## Storage and Directory Structure

All data is persisted as human-readable files (YAML and Markdown) under a single base directory. There is no database. This design is intentional: files are git-friendly, diff-able, and require no runtime dependencies.

```mermaid
graph TD
    ROOT[".adb/ or project root"]

    ROOT --> TC[".taskconfig<br/>(global YAML config)"]
    ROOT --> COUNTER[".task_counter<br/>(integer file)"]
    ROOT --> BACKLOG["backlog.yaml<br/>(central task registry)"]
    ROOT --> CLAUDE["CLAUDE.md / kiro.md<br/>(AI context files)"]

    ROOT --> TICKETS["tickets/"]
    TICKETS --> T1["TASK-00001/<br/>(active task)"]
    T1 --> STATUS["status.yaml"]
    T1 --> CONTEXT["context.md"]
    T1 --> NOTES["notes.md"]
    T1 --> DESIGN["design.md"]
    T1 --> COMMS["communications/"]
    COMMS --> COMM1["2025-01-15-slack-alice-api-design.md"]
    COMMS --> COMM2["2025-01-16-email-bob-review.md"]

    TICKETS --> ARCHIVED["_archived/"]
    ARCHIVED --> AT1["TASK-00002/<br/>(archived task)"]
    AT1 --> ASTATUS["status.yaml"]
    AT1 --> AHANDOFF["handoff.md"]
    AT1 --> ADESIGN["design.md"]

    ROOT --> REPOS["repos/"]
    REPOS --> PLATFORM["github.com/"]
    PLATFORM --> ORG["org/"]
    ORG --> REPO["repo/"]
    REPO --> WORK["work/"]
    WORK --> WT1["TASK-00001/<br/>(git worktree)"]

    ROOT --> DOCS["docs/"]
    DOCS --> WIKI["wiki/<br/>(extracted knowledge)"]
    DOCS --> DECISIONS["decisions/<br/>(ADR-XXXX-*.md)"]
    DOCS --> GLOSSARY["glossary.md"]
    DOCS --> STAKEHOLDERS["stakeholders.md"]
    DOCS --> CONTACTS["contacts.md"]

    style ROOT fill:#34495e,color:#fff
    style TICKETS fill:#e67e22,color:#fff
    style ARCHIVED fill:#95a5a6,color:#fff
    style REPOS fill:#2ecc71,color:#fff
    style DOCS fill:#3498db,color:#fff
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

The core package defines narrow local interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`) that mirror subsets of the storage and integration interfaces. This pattern is idiomatic Go: define the interface where it is consumed, not where it is implemented. It keeps the core package's `import` list free of storage and integration packages, preventing circular dependencies.

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

### Knowledge feedback loop

The `KnowledgeExtractor` creates a feedback loop from completed tasks back into organizational documentation:

1. **Wiki updates** -- Learnings tagged with `## Wiki Updates` in notes.md are written to `docs/wiki/`.
2. **ADR creation** -- Decisions tagged in communications are promoted to Architecture Decision Records in `docs/decisions/`.
3. **Runbook updates** -- Items tagged with `## Runbook Updates` feed into operational runbooks.
4. **Handoff documents** -- On archive, a `handoff.md` captures summary, completed work, open items, learnings, and decisions for the next person who picks up the work.

### Conflict detection

The `ConflictDetector` scans existing ADRs (`docs/decisions/`), previous task decisions (`tickets/*/design.md`), and stakeholder requirements (`docs/wiki/`) before proposed changes are applied. It uses keyword overlap analysis to flag potential conflicts, categorized by type (`adr_violation`, `previous_decision`, `stakeholder_requirement`) and severity (`high`, `medium`, `low`).

---

## Package Reference

| Package | Responsibility | Key Interfaces |
|---------|---------------|----------------|
| `cmd/adb` | Binary entrypoint | -- |
| `internal` | Composition root, adapters | `App` struct |
| `internal/cli` | Cobra command definitions | -- |
| `internal/core` | Business logic | `TaskManager`, `BootstrapSystem`, `ConfigurationManager`, `KnowledgeExtractor`, `ConflictDetector`, `AIContextGenerator`, `UpdateGenerator`, `TaskDesignDocGenerator`, `TaskIDGenerator`, `TemplateManager` |
| `internal/storage` | File-based persistence | `BacklogManager`, `ContextManager`, `CommunicationManager` |
| `internal/integration` | External system interaction | `GitWorktreeManager`, `OfflineManager`, `TabManager`, `ScreenshotPipeline`, `CLIExecutor`, `TaskfileRunner` |
| `pkg/models` | Shared data types | `Task`, `GlobalConfig`, `RepoConfig`, `MergedConfig`, `Communication`, `ExtractedKnowledge`, `HandoffDocument`, `Decision` |

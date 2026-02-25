# AI Dev Brain (adb)

## Project Overview

AI Dev Brain (adb) is a Go CLI tool that wraps AI coding assistants with persistent context management, task lifecycle automation, and knowledge accumulation. It provides commands for managing tasks, bootstrapping git worktrees, tracking stakeholder communications, maintaining organizational knowledge across multiple repositories, and observability through event logging, metrics, and alerting.

## Architecture

Layered architecture: CLI -> Core -> Storage/Integration/Observability. All dependencies are wired via the `internal/app.go` `App` struct using constructor injection. Interface-based design with local interface definitions in `core/` to avoid import cycles between packages. Adapter structs in `app.go` bridge the `core` and `storage`/`integration`/`observability` packages.

### Package Responsibilities

- `cmd/adb/` -- Entry point. Sets version info (ldflags), resolves base path, creates App, executes CLI.
- `internal/cli/` -- Cobra command definitions. Package-level variables (`TaskMgr`, `UpdateGen`, `AICtxGen`, `Executor`, `Runner`, `EventLog`, `AlertEngine`, `MetricsCalc`, `SessionCapture`, `HookEngine`) are set during App init.
- `internal/core/` -- Business logic. Task management, bootstrap, configuration, templates, AI context generation (with context evolution tracking), update generation, design doc generation, knowledge extraction, conflict detection, hook engine. Defines `EventLogger` and `SessionCapturer` local interfaces for cross-package decoupling.
- `internal/hooks/` -- Hook support library. Stdin JSON parsing (generic `ParseStdin[T]`), session change tracker (`.adb_session_changes` append-only file), and artifact helpers (context.md append, status.yaml timestamp, change grouping/formatting).
- `internal/storage/` -- Persistence layer. Backlog (YAML), context (Markdown), communication (Markdown files per entry), session store (YAML index + per-session directories).
- `internal/integration/` -- External system integrations. Git worktrees, CLI execution with alias resolution, Taskfile runner, tab renaming, screenshot OCR pipeline, offline mode with operation queuing, Claude Code JSONL transcript parsing.
- `internal/observability/` -- Event logging, metrics calculation, and alerting. Uses append-only JSONL files for event persistence. Derives metrics and evaluates alert conditions on-demand from the event log.
- `pkg/models/` -- Shared domain types. Task, Communication, Config (including NotificationConfig, SessionCaptureConfig, HookConfig), Knowledge/Handoff/Decision models, CapturedSession/SessionTurn/SessionFilter types.
- `templates/claude/` -- Embedded Claude Code templates. Package `claudetpl` uses `//go:embed` to bundle skills, agents, hooks, rules, and config templates into the binary. Accessed via `claudetpl.FS` (embed.FS).

## Technology Stack

- Go 1.26
- Cobra (CLI framework), Viper (configuration), yaml.v3 (persistence)
- pgregory.net/rapid (property-based testing)
- GoReleaser (release automation), golangci-lint (linting)
- Docker multi-stage builds

## Project Structure

```
cmd/adb/main.go              # Entry point
internal/
  app.go                      # Dependency wiring, adapters (incl. eventLogAdapter, sessionCapturerAdapter)
  cli/
    root.go                   # Root command and version
    task.go                   # `adb task` command group (create, resume, archive, unarchive, cleanup, status, priority, update)
    sync.go                   # `adb sync` command group (context, task-context, repos, claude-user, all)
    feat.go                   # feat/bug/spike/refactor task creation (deprecated, use `adb task create --type=`)
    resume.go                 # Resume a task (deprecated, use `adb task resume`)
    archive.go                # Archive a task (deprecated, use `adb task archive`)
    unarchive.go              # Restore archived task (deprecated, use `adb task unarchive`)
    migratearchive.go         # Migrate existing archived tasks to _archived/ (hidden)
    status.go                 # Display tasks by status (deprecated, use `adb task status`)
    priority.go               # Reorder task priorities (deprecated, use `adb task priority`)
    update.go                 # Generate stakeholder update plan (deprecated, use `adb task update`)
    synccontext.go            # Regenerate AI context files (deprecated, use `adb sync context`)
    exec.go                   # Execute external CLI with alias resolution
    run.go                    # Run Taskfile tasks
    alerts.go                 # Show active alerts and warnings
    metrics.go                # Display task and agent metrics
    session.go                # Manage session summaries (save, ingest)
    sessioncapture.go         # Session capture commands (capture --from-hook, list, show)
    team.go                   # Launch multi-agent team sessions (adb team)
    agents.go                 # List Claude Code agents (adb agents)
    synctaskcontext.go        # Sync task context (deprecated, use `adb sync task-context`)
    worktreehook.go           # Worktree hook event handlers (hidden, internal)
    worktreelifecycle.go      # Worktree lifecycle automation (hidden, internal)
    vars.go                   # Package-level variables (EventLog, AlertEngine, MetricsCalc, SessionCapture)
  core/
    config.go                 # ConfigurationManager (Viper-based)
    bootstrap.go              # BootstrapSystem (task init, directory scaffold incl. sessions/ and knowledge/)
    taskmanager.go            # TaskManager (lifecycle: create, resume, archive)
    ticketpath.go             # Ticket path resolution (active vs _archived/)
    taskid.go                 # TaskIDGenerator (sequential TASK-XXXXX IDs)
    templates.go              # TemplateManager (notes.md, design.md per type)
    doctemplates.go           # Built-in template content (notes, design, handoff templates)
    branchformat.go           # Branch name sanitization and formatting
    updategen.go              # UpdateGenerator (stakeholder communication plans)
    aicontext.go              # AIContextGenerator (CLAUDE.md, kiro.md) with critical decisions, recent sessions, context evolution tracking ("What's Changed"), and captured sessions sections
    sessioncapturer.go        # SessionCapturer local interface (avoids importing storage)
    designdoc.go              # TaskDesignDocGenerator (task-level design docs)
    knowledge.go              # KnowledgeExtractor (learnings, ADRs, wiki)
    conflict.go               # ConflictDetector (ADR/decision/requirement checks)
    projectinit.go            # ProjectInitializer (full workspace scaffolding)
    eventlogger.go            # EventLogger local interface (avoids importing observability)
    hookengine.go             # HookEngine interface and implementation (PreToolUse, PostToolUse, Stop, TaskCompleted, SessionEnd)
  storage/
    backlog.go                # BacklogManager (backlog.yaml CRUD)
    context.go                # ContextManager (per-task context.md, notes.md)
    communication.go          # CommunicationManager (per-task comms as .md files)
    sessionstore.go           # SessionStoreManager (workspace-level captured session storage)
    ticketpath.go             # Ticket path resolution for storage layer
  integration/
    worktree.go               # GitWorktreeManager (git worktree operations)
    cliexec.go                # CLIExecutor (external tool invocation, alias resolution)
    taskfilerunner.go         # TaskfileRunner (Taskfile.yaml discovery and execution)
    tab.go                    # TabManager (terminal tab renaming via ANSI)
    transcript.go             # TranscriptParser and StructuralSummarizer (Claude Code JSONL transcript parsing)
    screenshot.go             # ScreenshotPipeline (capture, OCR, classify, file)
    offline.go                # OfflineManager (connectivity detection, op queuing)
    version.go                # Claude Code version detection with semver parsing and feature gates
    mcpclient.go              # MCP server health checking with HTTP/stdio support and TTL cache
  observability/
    doc.go                    # Package documentation
    eventlog.go               # EventLog interface and JSONL implementation
    metrics.go                # MetricsCalculator interface and implementation
    alerting.go               # AlertEngine interface, alert thresholds, and alert conditions
  hooks/
    doc.go                    # Package documentation
    stdin.go                  # Stdin JSON structs and generic ParseStdin[T] parser
    tracker.go                # ChangeTracker for .adb_session_changes file (append/read/cleanup)
    artifacts.go              # Context append, status timestamp update, change grouping/formatting
  integration_test.go         # Cross-package integration tests
  qa_edge_cases_test.go       # QA edge case tests
pkg/models/
  task.go                     # Task (incl. Teams field, TeamMetadata), TaskType, TaskStatus, Priority
  config.go                   # GlobalConfig (incl. NotificationConfig, TeamRoutingConfig, HookConfig), RepoConfig, MergedConfig, CLIAliasConfig
  communication.go            # Communication, CommunicationTag
  session.go                  # CapturedSession, SessionTurn, SessionFilter, SessionCaptureConfig, SessionIndex
  knowledge.go                # ExtractedKnowledge, Decision, HandoffDocument, WikiUpdate, RunbookUpdate
templates/claude/
  embed.go                    # Embedded template files (//go:embed directive, exports FS as embed.FS)
  skills/                     # Reusable Claude Code skills (embedded)
  agents/                     # Specialized agent definitions (embedded)
  hooks/                      # Hook scripts (adb-session-capture.sh, adb-worktree-create.sh, adb-worktree-remove.sh, adb-worktree-boundary.sh, embedded)
  statusline.sh               # Universal status line script for Claude Code
  artifacts/                  # BMAD artifact templates (PRD, product-brief, tech-spec, architecture-doc, epics)
  checklists/                 # BMAD quality gate checklists (architecture, code-review, prd, readiness, story)
  rules/                      # Project rules (go-standards.md, cli-patterns.md, workspace.md, embedded)
.claude/
  settings.json               # Permissions and hooks configuration
  agents/                     # Specialized Claude Code agent definitions (18 agents)
  skills/                     # Reusable Claude Code skills (23 skills)
  hooks/                      # Quality gate hook scripts (5 hooks)
templates/claude/hooks/
  adb-hook-pre-tool-use.sh    # Shell wrapper: pipes stdin to adb hook pre-tool-use (blocking)
  adb-hook-post-tool-use.sh   # Shell wrapper: pipes stdin to adb hook post-tool-use (non-blocking)
  adb-hook-stop.sh            # Shell wrapper: pipes stdin to adb hook stop (non-blocking)
  adb-hook-task-completed.sh  # Shell wrapper: pipes stdin to adb hook task-completed (blocking)
  adb-hook-session-end.sh     # Shell wrapper: pipes stdin to adb hook session-end (non-blocking)
  adb-hook-teammate-idle.sh   # No-op: exit 0
  adb-session-capture.sh      # Legacy SessionEnd hook script for automatic session capture
  adb-worktree-create.sh      # Validates task context exists, logs worktree creation event
  adb-worktree-remove.sh      # Archives orphaned sessions, logs worktree removal event
  adb-worktree-boundary.sh    # Validates tool paths are within worktree boundary, blocks violations
  rules/                      # Project rules (go-standards.md, cli-patterns.md, workspace.md)
.mcp.json                     # MCP server configuration (aws-knowledge, context7)
```

## Common Commands

- Build: `go build -ldflags="-s -w" -o adb ./cmd/adb/`
- Test all: `go test ./... -count=1`
- Test with race detector: `go test ./... -race -count=1`
- Test property-based only: `go test ./... -run "TestProperty" -count=1 -v`
- Lint: `golangci-lint run`
- Vet: `go vet ./...`
- Format: `gofmt -s -w .`
- Format check: `gofmt -l .`
- Run: `go run ./cmd/adb/ [command]`
- Docker build: `docker build -t adb:latest .`
- Security scan: `govulncheck ./...`
- All Makefile targets: `make help`

## Go Coding Standards

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the error chain.
- Define interfaces close to where they are consumed, not where they are implemented. Core package defines local interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`, `WorktreeRemover`, `EventLogger`, `SessionCapturer`) to avoid importing storage/integration/observability.
- Use constructor functions that return interfaces: `func NewFoo(...) FooInterface { return &foo{...} }`.
- All struct fields that need persistence use `yaml` struct tags.
- File permissions: directories `0o755`, files `0o644`.
- Use `time.Now().UTC()` for all timestamps.
- Error messages start lowercase and describe the operation: `"creating task: %w"`.

## Key Patterns in This Codebase

- **Local interface definitions** in `core/` (`BacklogStore`, `ContextStore`, `WorktreeCreator`, `WorktreeRemover`, `EventLogger`, `SessionCapturer`) avoid importing `storage`/`integration`/`observability` packages.
- **Adapter pattern** in `internal/app.go` bridges packages: `backlogStoreAdapter`, `contextStoreAdapter`, `worktreeAdapter`, `eventLogAdapter`, `sessionCapturerAdapter`.
- **CLI package-level variables** (`TaskMgr`, `UpdateGen`, `AICtxGen`, `Executor`, `Runner`, `ExecAliases`, `EventLog`, `AlertEngine`, `MetricsCalc`, `SessionCapture`, `HookEngine`, `BasePath`, `ProjectInit`) are set during `App` init in `app.go`.
- **Property tests** use `rapid.Check` with the `TestProperty` prefix naming convention.
- **Template rendering** via `text/template` for `notes.md`, `design.md`, `handoff.md`.
- **Embedded templates**: Claude Code templates (skills, agents, hooks, rules) are embedded into the binary via `//go:embed` in `templates/claude/embed.go`. The `claudetpl` package exports `FS` as an `embed.FS`. CLI commands (`initclaude.go`, `syncclaudeuser.go`) read templates from `claudetpl.FS` using `path.Join` (forward slashes only, required for embed.FS cross-platform compatibility). Template writing uses `writeIfNotExists(path, data)` pattern instead of file copying.
- **File-based storage**: YAML for structured data (`backlog.yaml`, `status.yaml`), Markdown for human-readable docs (`context.md`, `notes.md`, `design.md`, communications, sessions).
- **Base path resolution**: checks `ADB_HOME` env var, then walks up directory tree looking for `.taskconfig`, falls back to cwd.
- **JSONL event logging**: `internal/observability/` uses append-only JSONL files (`.adb_events.jsonl`) for structured event persistence. Events are JSON-encoded with time, level, type, message, and data fields. Metrics and alerts are derived on-demand from the event log.
- **Graceful degradation**: Observability is non-fatal. If the event log file cannot be created, observability features are disabled without affecting core functionality.
- **Task context generation**: Bootstrap creates `.claude/rules/task-context.md` inside worktrees so AI assistants have immediate task awareness.
- **Post-command workflow**: `launchWorkflow` in `internal/cli/feat.go` renames the terminal tab, launches Claude Code in the worktree, then drops the user into an interactive shell. Accepts a `resume bool` parameter: task creation commands pass `false` (launches Claude Code without `--resume`), while `adb resume` passes `true` (launches with `--resume` to continue the most recent conversation).
- **Hybrid hook execution**: Thin shell wrapper scripts (in `templates/claude/hooks/`) set `ADB_HOOK_ACTIVE=1` env var and pipe stdin to `adb hook <type>`. The Go binary does all validation, formatting, tracking, and knowledge work. Recursion is prevented by checking `ADB_HOOK_ACTIVE` before exporting it.
- **Change tracker pattern**: PostToolUse appends modified file paths to `.adb_session_changes` (append-only, `timestamp|tool|filepath` per line). Stop and SessionEnd hooks consume this file to produce batched context summaries, then clean up.
- **Two-phase TaskCompleted**: Phase A (blocking) runs quality gates (tests, lint, uncommitted check) with `os.Exit(2)` on failure. Phase B (non-blocking) runs knowledge extraction, wiki updates, and ADR generation. Quality gates are never weakened by knowledge failures.
- **Hook configuration**: `models.HookConfig` in `.taskconfig` under `hooks:` key controls all features per hook type. `DefaultHookConfig()` enables Phase 1 features; Phase 2/3 features are disabled by default and opt-in via config.

## Task Types and Statuses

- Types: `feat`, `bug`, `spike`, `refactor`
- Statuses: `backlog` -> `in_progress` -> `blocked` | `review` -> `done` -> `archived`
- Priorities: `P0` (critical), `P1` (high), `P2` (medium/default), `P3` (low)
- Task ID format: `{prefix}-{counter:05d}` (e.g., `TASK-00001`)

## Important Interfaces

### Core Package (`internal/core/`)

| Interface | Purpose |
|-----------|---------|
| `TaskManager` | Task lifecycle: create (with CreateTaskOpts for priority, owner, tags, prefix), resume, archive, unarchive, status/priority updates |
| `BootstrapSystem` | Initialize new tasks with directory structure (incl. sessions/, knowledge/), templates, worktree, and task context generation |
| `ConfigurationManager` | Load/merge/validate global (.taskconfig) and repo (.taskrc) config |
| `TaskIDGenerator` | Generate sequential TASK-XXXXX IDs via file-based counter |
| `TemplateManager` | Apply task-type-specific templates (notes.md, design.md) |
| `UpdateGenerator` | Analyze task context/comms to produce stakeholder update plans |
| `AIContextGenerator` | Generate root-level AI context files (CLAUDE.md, kiro.md) with critical decisions, recent sessions, captured sessions, and context evolution ("What's Changed") sections |
| `SessionCapturer` | Local interface for session capture operations (capture, list, get, query) -- avoids importing storage |
| `TaskDesignDocGenerator` | Manage task-level technical design documents |
| `KnowledgeExtractor` | Extract learnings, decisions, gotchas from completed tasks |
| `ConflictDetector` | Check proposed changes against ADRs, decisions, requirements |
| `HookEngine` | Process Claude Code hook events: PreToolUse (blocking), PostToolUse (non-blocking), Stop (advisory), TaskCompleted (blocking quality gates + non-blocking knowledge), SessionEnd (non-blocking) |
| `ProjectInitializer` | Initialize full project workspace with directory structure, config, and docs |
| `EventLogger` | Local interface for event logging, avoids importing observability package |
| `BacklogStore` | Local interface mirroring storage.BacklogManager for decoupling |
| `ContextStore` | Local interface mirroring storage.ContextManager for decoupling |
| `WorktreeCreator` | Local interface mirroring integration.GitWorktreeManager for decoupling |
| `WorktreeRemover` | Local interface for worktree removal, used by TaskManager for cleanup |

### Storage Package (`internal/storage/`)

| Interface | Purpose |
|-----------|---------|
| `BacklogManager` | CRUD operations on backlog.yaml (central task registry) |
| `ContextManager` | Persist and load per-task context (context.md, notes.md) |
| `CommunicationManager` | Store and search task communications as markdown files |
| `SessionStoreManager` | CRUD operations for captured sessions (YAML index + per-session directories with session.yaml, turns.yaml, summary.md) |

### Integration Package (`internal/integration/`)

| Interface | Purpose |
|-----------|---------|
| `GitWorktreeManager` | Create, remove, list git worktrees |
| `CLIExecutor` | Execute external CLI tools with alias resolution and task env injection |
| `TaskfileRunner` | Discover and execute Taskfile.yaml tasks |
| `TabManager` | Rename terminal tabs via ANSI escape sequences |
| `ScreenshotPipeline` | Capture screenshots, OCR, classify, and file content |
| `TranscriptParser` | Parse Claude Code JSONL session transcripts into structured turn data (TranscriptResult) |
| `OfflineManager` | Detect connectivity, queue operations for later sync |
| `ClaudeCodeVersionChecker` | Detect Claude Code version, check feature gates, cache results |
| `MCPClient` | Validate MCP server health (HTTP/stdio), cache discovery results with TTL |

### Observability Package (`internal/observability/`)

| Interface | Purpose |
|-----------|---------|
| `EventLog` | Write and read structured events from JSONL files. Methods: `Write(Event)`, `Read(EventFilter)`, `Close()` |
| `MetricsCalculator` | Derive aggregated metrics (tasks created/completed, by status/type, agent sessions, knowledge extracted) from the event log |
| `AlertEngine` | Evaluate alert conditions (blocked tasks, stale tasks, long reviews, backlog size) against configurable thresholds |

## Testing Conventions

- Unit tests: standard `testing.T` with table-driven subtests
- Property tests: `pgregory.net/rapid` with `TestPropertyXX` naming convention
- Integration tests: `internal/integration_test.go`
- Edge case tests: `internal/qa_edge_cases_test.go`
- All tests use `t.TempDir()` for filesystem isolation
- 16 property test files across core, storage, hooks, and integration packages
- Test files live alongside their implementation files

## File Naming Conventions

- Implementation: `lowercase.go` (e.g., `taskmanager.go`, `backlog.go`)
- Unit tests: `lowercase_test.go` (e.g., `taskmanager_test.go`)
- Property tests: `lowercase_property_test.go` (e.g., `taskmanager_property_test.go`)
- Doc files: `doc.go` for package-level documentation

## Configuration Files

- `.taskconfig` -- Global config (YAML, read via Viper). Contains `defaults.ai`, `task_id.prefix`, `task_id.counter`, `defaults.priority`, `defaults.owner`, `screenshot.hotkey`, `offline_mode`, `cli_aliases`, `notifications`, `team_routing`, `hooks`.
- `.taskrc` -- Per-repo config (YAML, read via Viper). Contains `build_command`, `test_command`, `default_reviewers`, `conventions`, `templates`.
- Precedence: `.taskrc` > `.taskconfig` > defaults. `ADB_HOME` env var overrides base path resolution.
- `.task_counter` -- File-based sequential counter for task ID generation
- `.adb_events.jsonl` -- Append-only event log used by the observability package
- `.session_counter` -- File-based sequential counter for captured session ID generation (S-XXXXX format)
- `.context_state.yaml` -- Context evolution state snapshot (section hashes, task counts) used by `sync-context`
- `.context_changelog.md` -- Accumulated context change log (pruned to 50 entries) used for "What's Changed" section
- `sessions/` -- Workspace-level directory for captured Claude Code sessions (YAML index + per-session subdirectories)
- `.mcp.json` -- MCP server configuration for external knowledge services (aws-knowledge, context7)
- `.adb_mcp_cache.json` -- Cached MCP server health check results with TTL (used by `adb mcp check`)
- `.claude/settings.json` -- Claude Code permissions and hooks configuration

### Notification and Alert Configuration

The `notifications` key in `.taskconfig` controls alerting thresholds:

```yaml
notifications:
  enabled: true
  slack:
    webhook_url: "https://hooks.slack.com/..."
  alerts:
    blocked_threshold_hours: 24
    stale_threshold_days: 3
    review_threshold_days: 5
    max_backlog_size: 10
```

Default thresholds (used when not configured): blocked 24h, stale 3d, review 5d, max backlog 10.

### Team Routing Configuration

The `team_routing` key in `.taskconfig` enables tag-based task-to-team routing:

```yaml
team_routing:
  enabled: true
  rules:
    - tags: [security, audit]
      team_name: security-review
      members: [security-auditor, code-reviewer]
    - tags: [design, architecture]
      team_name: design-review
      members: [design-reviewer, architecture-guide]
```

### Hook Error Handling Configuration

The `hooks` key in `.taskconfig` configures per-hook error handling:

```yaml
hooks:
  - name: pre-edit-validate
    timeout_sec: 10
    retries: 0
    on_failure: block
  - name: post-edit-go-fmt
    timeout_sec: 30
    retries: 2
    on_failure: warn
```

## CLI Commands

### Task Management (`adb task`)

- `adb task create <branch> --type=feat|bug|spike|refactor [--repo] [--priority] [--owner] [--tags] [--prefix]` -- Create a new task (default type: feat)
- `adb task resume <task-id>` -- Resume a task (promotes backlog to in_progress, launches Claude Code with `--resume`)
- `adb task archive <task-id> [--force] [--keep-worktree]` -- Archive a task (generates handoff.md, moves ticket to _archived/, removes worktree)
- `adb task unarchive <task-id>` -- Restore an archived task (moves ticket back from _archived/)
- `adb task cleanup <task-id>` -- Remove a task's git worktree without archiving
- `adb task status [--filter <status>]` -- Display tasks grouped by status
- `adb task priority <task-id>...` -- Reorder task priorities
- `adb task update <task-id>` -- Generate stakeholder update plan

### Sync Commands (`adb sync`)

- `adb sync context` -- Regenerate AI context files (CLAUDE.md, kiro.md)
- `adb sync task-context [--hook-mode]` -- Regenerate .claude/rules/task-context.md in the current worktree
- `adb sync repos` -- Fetch, prune, and clean all tracked repositories
- `adb sync claude-user [--dry-run] [--mcp]` -- Sync Claude Code agents, skills, and status line to user config
- `adb sync all` -- Run context + repos + claude-user sequentially

### Project Setup

- `adb init [path] [--name] [--ai] [--prefix]` -- Initialize a new adb project workspace
- `adb init claude [path] [--managed]` -- Bootstrap Claude Code configuration for a repository
- `adb version` -- Print version information

### External Tools

- `adb exec <cli> [args...]` -- Execute external CLI with alias resolution
- `adb run <task-name>` -- Run a Taskfile task

### Observability

- `adb metrics [--json] [--since 7d]` -- Display task and agent metrics from the event log
- `adb alerts [--notify]` -- Show active alerts (blocked tasks, stale tasks, long reviews, backlog size)
- `adb dashboard` -- Interactive TUI dashboard for task metrics and alerts

### Sessions

- `adb session save [task-id]` -- Save a session summary to the task's sessions/ directory
- `adb session ingest [task-id]` -- Ingest knowledge from the latest session file
- `adb session capture --from-hook` -- Capture a Claude Code session from a JSONL transcript (called by SessionEnd hook)
- `adb session list [--task-id ID] [--since 7d]` -- List captured sessions with optional filters
- `adb session show <session-id>` -- Show details and turns for a captured session

### Claude Code & Teams

- `adb team <team-name> <prompt>` -- Launch a multi-agent team session with task context
- `adb agents` -- List available Claude Code agents
- `adb mcp check [--no-cache]` -- Validate configured MCP servers and cache results

### Deprecated Commands (still work, print warnings)

Old top-level commands are deprecated in favor of the noun-verb hierarchy above:
- `adb feat/bug/spike/refactor` -> `adb task create --type=`
- `adb resume/archive/unarchive/cleanup` -> `adb task resume/archive/unarchive/cleanup`
- `adb status/priority/update` -> `adb task status/priority/update`
- `adb sync-context/sync-repos/sync-claude-user/sync-task-context` -> `adb sync context/repos/claude-user/task-context`
- `adb init-claude` -> `adb init claude`

### Internal (hidden from help, still functional)

- `adb hook {install,status,pre-tool-use,post-tool-use,stop,task-completed,session-end}`
- `adb worktree-hook {create,remove,violation}`
- `adb worktree-lifecycle {pre-create,post-create,pre-remove,post-remove}`
- `adb migrate-archive`

## Observability

### Event Types

Events are structured with `time`, `level` (INFO/WARN/ERROR), `type`, `msg`, and `data` fields.

| Event Type | Description |
|------------|-------------|
| `task.created` | Task was created (data includes task_id, type) |
| `task.completed` | Task was completed |
| `task.status_changed` | Task status changed (data includes task_id, new_status) |
| `agent.session_started` | AI agent session began |
| `knowledge.extracted` | Knowledge was extracted from a task |
| `team.session_started` | Multi-agent team session began (data includes team_name, task_id) |
| `team.session_ended` | Multi-agent team session completed |
| `worktree.created` | Worktree was created (from WorktreeCreate hook) |
| `worktree.removed` | Worktree was removed (from WorktreeRemove hook) |
| `worktree.isolation_violation` | Tool use attempted outside worktree boundary (severity HIGH) |
| `config.task_context_synced` | Task context file was regenerated |

### Alert Conditions

| Condition | Severity | Trigger |
|-----------|----------|---------|
| `task_blocked_too_long` | High | Task blocked longer than threshold (default 24h) |
| `task_stale` | Medium | In-progress task with no activity beyond threshold (default 3d) |
| `review_too_long` | Medium | Task in review longer than threshold (default 5d) |
| `backlog_too_large` | Low | Backlog exceeds maximum size (default 10 tasks) |

### AI Context Sections

The AIContextGenerator assembles these sections into CLAUDE.md/kiro.md:

| Section | Data Source |
|---------|-----------|
| Project Overview | Hardcoded summary |
| Directory Structure | Hardcoded tree description |
| Conventions | `docs/wiki/*convention*.md` files, else defaults |
| Glossary | `docs/glossary.md`, else defaults |
| Decisions Summary | `docs/decisions/*.md` (accepted ADRs only) |
| Active Tasks | `backlog.yaml` filtered by active statuses |
| Critical Decisions | `tickets/*/knowledge/decisions.yaml` from active tasks |
| Recent Sessions | Latest `tickets/*/sessions/*.md` from active tasks (truncated to 20 lines) |
| Captured Sessions | Recent captured sessions from workspace-level `sessions/` store (via SessionCapturer) |
| What's Changed | Semantic diff of context state since last sync: tasks added/completed, new knowledge, section hash changes (`.context_state.yaml`) |
| Stakeholders/Contacts | `docs/stakeholders.md`, `docs/contacts.md` |

Additionally, `BootstrapSystem.generateTaskContext` writes a per-worktree `.claude/rules/task-context.md` file at task creation time (not part of `AIContextGenerator` or `sync-context`).

## Task Directory Structure

Each task bootstraps the following directory tree:

```
tickets/TASK-XXXXX/
  status.yaml                # Task metadata (YAML)
  context.md                 # AI-maintained running context
  notes.md                   # Requirements and acceptance criteria
  design.md                  # Technical design document
  communications/            # Stakeholder communications as dated .md files
  sessions/                  # Session summaries (timestamped .md files)
  knowledge/                 # Extracted decisions and facts (decisions.yaml)
```

When a worktree is created, the bootstrap also generates `.claude/rules/task-context.md` inside the worktree for immediate AI assistant awareness.

## Claude Code Configuration

### Agents (`.claude/agents/`)

| Agent | Model | Description |
|-------|-------|-------------|
| `team-lead` | opus | Orchestrates multi-agent teams. Breaks down work, assigns tasks, monitors progress, synthesizes results. Routes to BMAD workflow personas. |
| `analyst` | sonnet | Requirements elicitation, PRD creation, market/domain/technical research. BMAD discovery phase. Has project memory. |
| `product-owner` | sonnet | PRD facilitation, epic/story decomposition, backlog prioritization, implementation readiness. Has project memory. |
| `design-reviewer` | sonnet | Architecture validation, checklist certification, cross-artifact alignment checks. Has project memory. |
| `scrum-master` | sonnet | Sprint planning, story preparation, retrospectives, course correction. Has project memory. |
| `quick-flow-dev` | sonnet | Rapid spec + implementation for small tasks with built-in adversarial review. Has project memory. |
| `go-tester` | sonnet | Runs tests, analyzes failures, writes missing test cases. Has project memory. |
| `code-reviewer` | sonnet | Reviews Go code for quality, security, correctness, and adherence to project patterns. Has project memory. |
| `architecture-guide` | sonnet | Explains architecture, guides design decisions, ensures new code follows patterns. Has project memory. |
| `knowledge-curator` | sonnet | Maintains wiki, ADRs, glossary. Extracts learnings from completed tasks. Has project memory. |
| `doc-writer` | sonnet | Generates and updates CLAUDE.md, architecture.md, commands.md, README. |
| `researcher` | sonnet | Deep investigation for spikes, technology evaluations, pre-design research. Has user memory. |
| `debugger` | sonnet | Root cause analysis for errors, test failures, runtime issues. Has project memory. |
| `observability-reporter` | sonnet | Generates health dashboards, coverage reports, task progress summaries. |
| `security-auditor` | sonnet | Audits code and dependencies for security vulnerabilities. |
| `release-manager` | sonnet | Manages releases, changelogs, and version bumping with GoReleaser. |

### Skills (`.claude/skills/`)

| Skill | Description | Argument |
|-------|-------------|----------|
| `build` | Build the adb binary for current platform or cross-compile | `[platform]` |
| `test` | Run tests with coverage, race detection, specific packages, or property tests | `[package\|coverage\|property\|race]` |
| `lint` | Run linting, formatting checks, and static analysis | `[fix]` |
| `security` | Run security scans including govulncheck and gosec | -- |
| `docker` | Build and run the adb Docker image | `[build\|run\|push]` |
| `release` | Prepare and create a new release with GoReleaser | `<version>` |
| `coverage-report` | Generate detailed test coverage report with per-package breakdown | -- |
| `status-check` | Quick health check: build, tests, lint, vet | -- |
| `health-dashboard` | Comprehensive health check with build, test, lint, coverage, security, and task metrics | -- |
| `add-command` | Scaffold a new Cobra CLI command following project patterns | `<command-name>` |
| `add-interface` | Scaffold a new interface following architecture patterns | `<package> <interface-name>` |
| `standup` | Generate a daily standup summary from recent task activity and git commits | -- |
| `retrospective` | Analyze completed tasks and extract patterns, improvements, recurring themes | -- |
| `knowledge-extract` | Extract learnings, decisions, gotchas from a task into organizational knowledge | `<task-id>` |
| `context-refresh` | Update a task's context.md with latest progress from git history | `<task-id>` |
| `onboard` | Generate an onboarding guide for new contributors or AI sessions | -- |
| `dependency-check` | Identify blocked/blocking tasks and priority conflicts in the backlog | -- |
| `quick-spec` | Create implementation-ready tech spec through code investigation | `<task-id or description>` |
| `quick-dev` | Implement from tech spec or instructions with self-checking and adversarial review | `<task-id or tech-spec path>` |
| `adversarial-review` | Self-review uncommitted changes with hostile intent and information asymmetry | -- |

### Hooks (`.claude/settings.json` and `.claude/hooks/`)

**adb-native hooks** (installed via `adb hook install`): Shell wrappers delegate to `adb hook <type>` for compiled Go execution.

| Hook | Trigger | What It Does |
|------|---------|--------------|
| `adb-hook-pre-tool-use.sh` | `PreToolUse` (Edit\|Write) | Blocks vendor/, go.sum edits. Architecture guard (Phase 2). ADR conflict check (Phase 3). Exit 2 to block. |
| `adb-hook-post-tool-use.sh` | `PostToolUse` (Edit\|Write) | Auto-formats Go files with gofmt. Tracks changed files to `.adb_session_changes`. Dependency change detection (Phase 2). |
| `adb-hook-stop.sh` | `Stop` | Advisory checks: uncommitted changes, build, vet. Updates context.md with session summary. Updates status.yaml timestamp. |
| `adb-hook-task-completed.sh` | `TaskCompleted` | Phase A (blocking): uncommitted check, tests, lint. Phase B (non-blocking): knowledge extraction, wiki, ADRs, context update. |
| `adb-hook-session-end.sh` | `SessionEnd` | Captures Claude Code session transcript. Updates context.md from tracked changes. |
| `adb-hook-teammate-idle.sh` | `TeammateIdle` | No-op (exit 0). |

**Legacy hooks** (pre-adb-native, still available):

| Hook | Trigger | What It Does |
|------|---------|--------------|
| `teammate-idle-check.sh` | `TeammateIdle` | Runs `go test` and `golangci-lint` to verify project health when a teammate is idle |
| `task-completed-check.sh` | `TaskCompleted` | Verifies no uncommitted Go changes, runs tests and lint before marking a task complete |
| `stop-quality-check.sh` | `Stop` | Checks for uncommitted changes, runs build and vet before stopping |
| `post-edit-go-fmt.sh` | `PostToolUse` (Edit\|Write) | Auto-formats Go files with `gofmt -s -w` after Edit/Write tool use |
| `pre-edit-validate.sh` | `PreToolUse` (Edit\|Write) | Blocks editing vendor/ files and go.sum directly |
| `adb-session-capture.sh` | `SessionEnd` (user-level) | Calls `adb session capture --from-hook` to automatically capture Claude Code sessions workspace-wide. Installed by `adb sync-claude-user`. |
| `adb-worktree-create.sh` | `WorktreeCreate` | Validates task context exists, logs worktree creation event |
| `adb-worktree-remove.sh` | `WorktreeRemove` | Archives orphaned sessions, logs worktree removal event |
| `adb-worktree-boundary.sh` | `PreToolUse` (Edit\|Write\|Read) | Validates tool paths are within worktree boundary, blocks violations |

### MCP Servers (`.mcp.json`)

| Server | Type | Purpose |
|--------|------|---------|
| `aws-knowledge` | HTTP | AWS documentation search, regional availability, and documentation reading |
| `context7` | HTTP | Up-to-date documentation and code examples for any programming library |

## Linter Configuration

Enabled linters (via `.golangci.yml`): errcheck, gosimple, govet, ineffassign, staticcheck, unused, gosec, bodyclose, exhaustive, nilerr, unparam. gosec and unparam are excluded from test files.

## Additional Context

@docs/architecture.md
@docs/commands.md

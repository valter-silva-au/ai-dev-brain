# AI Dev Brain (adb)

## Project Overview

AI Dev Brain (adb) is a Go CLI tool that wraps AI coding assistants with persistent context management, task lifecycle automation, and knowledge accumulation. It provides commands for managing tasks, bootstrapping git worktrees, tracking stakeholder communications, and maintaining organizational knowledge across multiple repositories.

## Architecture

Layered architecture: CLI -> Core -> Storage/Integration. All dependencies are wired via the `internal/app.go` `App` struct using constructor injection. Interface-based design with local interface definitions in `core/` to avoid import cycles between packages. Adapter structs in `app.go` bridge the `core` and `storage`/`integration` packages.

### Package Responsibilities

- `cmd/adb/` -- Entry point. Sets version info (ldflags), resolves base path, creates App, executes CLI.
- `internal/cli/` -- Cobra command definitions. Package-level variables (`TaskMgr`, `UpdateGen`, `AICtxGen`, `Executor`, `Runner`) are set during App init.
- `internal/core/` -- Business logic. Task management, bootstrap, configuration, templates, AI context generation, update generation, design doc generation, knowledge extraction, conflict detection.
- `internal/storage/` -- Persistence layer. Backlog (YAML), context (Markdown), communication (Markdown files per entry).
- `internal/integration/` -- External system integrations. Git worktrees, CLI execution with alias resolution, Taskfile runner, tab renaming, screenshot OCR pipeline, offline mode with operation queuing.
- `pkg/models/` -- Shared domain types. Task, Communication, Config, Knowledge/Handoff/Decision models.

## Technology Stack

- Go 1.24
- Cobra (CLI framework), Viper (configuration), yaml.v3 (persistence)
- pgregory.net/rapid (property-based testing)
- GoReleaser (release automation), golangci-lint (linting)
- Docker multi-stage builds

## Project Structure

```
cmd/adb/main.go              # Entry point
internal/
  app.go                      # Dependency wiring, adapters
  cli/
    root.go                   # Root command and version
    feat.go                   # feat/bug/spike/refactor task creation
    resume.go                 # Resume a task
    archive.go                # Archive a task (generates handoff.md, moves to _archived/)
    unarchive.go              # Restore archived task (moves back from _archived/)
    migratearchive.go         # Migrate existing archived tasks to _archived/
    status.go                 # Update task status
    priority.go               # Update task priority
    update.go                 # Generate stakeholder update plan
    synccontext.go            # Regenerate AI context files
    exec.go                   # Execute external CLI with alias resolution
    run.go                    # Run Taskfile tasks
  core/
    config.go                 # ConfigurationManager (Viper-based)
    bootstrap.go              # BootstrapSystem (task init, directory scaffold)
    taskmanager.go            # TaskManager (lifecycle: create, resume, archive)
    ticketpath.go             # Ticket path resolution (active vs _archived/)
    taskid.go                 # TaskIDGenerator (sequential TASK-XXXXX IDs)
    templates.go              # TemplateManager (notes.md, design.md per type)
    doctemplates.go           # Built-in template content (unused alias)
    updategen.go              # UpdateGenerator (stakeholder communication plans)
    aicontext.go              # AIContextGenerator (CLAUDE.md, kiro.md)
    designdoc.go              # TaskDesignDocGenerator (task-level design docs)
    knowledge.go              # KnowledgeExtractor (learnings, ADRs, wiki)
    conflict.go               # ConflictDetector (ADR/decision/requirement checks)
  storage/
    backlog.go                # BacklogManager (backlog.yaml CRUD)
    context.go                # ContextManager (per-task context.md, notes.md)
    communication.go          # CommunicationManager (per-task comms as .md files)
    ticketpath.go             # Ticket path resolution for storage layer
  integration/
    worktree.go               # GitWorktreeManager (git worktree operations)
    cliexec.go                # CLIExecutor (external tool invocation, alias resolution)
    taskfilerunner.go         # TaskfileRunner (Taskfile.yaml discovery and execution)
    tab.go                    # TabManager (terminal tab renaming via ANSI)
    screenshot.go             # ScreenshotPipeline (capture, OCR, classify, file)
    offline.go                # OfflineManager (connectivity detection, op queuing)
  integration_test.go         # Cross-package integration tests
  qa_edge_cases_test.go       # QA edge case tests
pkg/models/
  task.go                     # Task, TaskType, TaskStatus, Priority
  config.go                   # GlobalConfig, RepoConfig, MergedConfig, CLIAliasConfig
  communication.go            # Communication, CommunicationTag
  knowledge.go                # ExtractedKnowledge, Decision, HandoffDocument, WikiUpdate, RunbookUpdate
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
- Define interfaces close to where they are consumed, not where they are implemented. Core package defines local interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator`) to avoid importing storage/integration.
- Use constructor functions that return interfaces: `func NewFoo(...) FooInterface { return &foo{...} }`.
- All struct fields that need persistence use `yaml` struct tags.
- File permissions: directories `0o755`, files `0o644`.
- Use `time.Now().UTC()` for all timestamps.
- Error messages start lowercase and describe the operation: `"creating task: %w"`.

## Key Patterns in This Codebase

- **Local interface definitions** in `core/` (`BacklogStore`, `ContextStore`, `WorktreeCreator`) avoid importing `storage`/`integration` packages.
- **Adapter pattern** in `internal/app.go` bridges packages: `backlogStoreAdapter`, `contextStoreAdapter`, `worktreeAdapter`.
- **CLI package-level variables** (`TaskMgr`, `UpdateGen`, `AICtxGen`, `Executor`, `Runner`, `ExecAliases`) are set during `App` init in `app.go`.
- **Property tests** use `rapid.Check` with the `TestProperty` prefix naming convention.
- **Template rendering** via `text/template` for `notes.md`, `design.md`, `handoff.md`.
- **File-based storage**: YAML for structured data (`backlog.yaml`, `status.yaml`), Markdown for human-readable docs (`context.md`, `notes.md`, `design.md`, communications).
- **Base path resolution**: checks `ADB_HOME` env var, then walks up directory tree looking for `.taskconfig`, falls back to cwd.

## Task Types and Statuses

- Types: `feat`, `bug`, `spike`, `refactor`
- Statuses: `backlog` -> `in_progress` -> `blocked` | `review` -> `done` -> `archived`
- Priorities: `P0` (critical), `P1` (high), `P2` (medium/default), `P3` (low)
- Task ID format: `{prefix}-{counter:05d}` (e.g., `TASK-00001`)

## Important Interfaces

### Core Package (`internal/core/`)

| Interface | Purpose |
|-----------|---------|
| `TaskManager` | Task lifecycle: create, resume, archive, unarchive, status/priority updates |
| `BootstrapSystem` | Initialize new tasks with directory structure, templates, worktree |
| `ConfigurationManager` | Load/merge/validate global (.taskconfig) and repo (.taskrc) config |
| `TaskIDGenerator` | Generate sequential TASK-XXXXX IDs via file-based counter |
| `TemplateManager` | Apply task-type-specific templates (notes.md, design.md) |
| `UpdateGenerator` | Analyze task context/comms to produce stakeholder update plans |
| `AIContextGenerator` | Generate root-level AI context files (CLAUDE.md, kiro.md) |
| `TaskDesignDocGenerator` | Manage task-level technical design documents |
| `KnowledgeExtractor` | Extract learnings, decisions, gotchas from completed tasks |
| `ConflictDetector` | Check proposed changes against ADRs, decisions, requirements |
| `BacklogStore` | Local interface mirroring storage.BacklogManager for decoupling |
| `ContextStore` | Local interface mirroring storage.ContextManager for decoupling |
| `WorktreeCreator` | Local interface mirroring integration.GitWorktreeManager for decoupling |

### Storage Package (`internal/storage/`)

| Interface | Purpose |
|-----------|---------|
| `BacklogManager` | CRUD operations on backlog.yaml (central task registry) |
| `ContextManager` | Persist and load per-task context (context.md, notes.md) |
| `CommunicationManager` | Store and search task communications as markdown files |

### Integration Package (`internal/integration/`)

| Interface | Purpose |
|-----------|---------|
| `GitWorktreeManager` | Create, remove, list git worktrees |
| `CLIExecutor` | Execute external CLI tools with alias resolution and task env injection |
| `TaskfileRunner` | Discover and execute Taskfile.yaml tasks |
| `TabManager` | Rename terminal tabs via ANSI escape sequences |
| `ScreenshotPipeline` | Capture screenshots, OCR, classify, and file content |
| `OfflineManager` | Detect connectivity, queue operations for later sync |

## Testing Conventions

- Unit tests: standard `testing.T` with table-driven subtests
- Property tests: `pgregory.net/rapid` with `TestPropertyXX` naming convention
- Integration tests: `internal/integration_test.go`
- Edge case tests: `internal/qa_edge_cases_test.go`
- All tests use `t.TempDir()` for filesystem isolation
- 13 property test files across core, storage, and integration packages
- Test files live alongside their implementation files

## File Naming Conventions

- Implementation: `lowercase.go` (e.g., `taskmanager.go`, `backlog.go`)
- Unit tests: `lowercase_test.go` (e.g., `taskmanager_test.go`)
- Property tests: `lowercase_property_test.go` (e.g., `taskmanager_property_test.go`)
- Doc files: `doc.go` for package-level documentation

## Configuration Files

- `.taskconfig` -- Global config (YAML, read via Viper). Contains `defaults.ai`, `task_id.prefix`, `task_id.counter`, `defaults.priority`, `defaults.owner`, `screenshot.hotkey`, `offline_mode`, `cli_aliases`.
- `.taskrc` -- Per-repo config (YAML, read via Viper). Contains `build_command`, `test_command`, `default_reviewers`, `conventions`, `templates`.
- Precedence: `.taskrc` > `.taskconfig` > defaults
- `.task_counter` -- File-based sequential counter for task ID generation

## CLI Commands

- `adb feat <branch>` -- Create a feature task
- `adb bug <branch>` -- Create a bug task
- `adb spike <branch>` -- Create a spike task
- `adb refactor <branch>` -- Create a refactor task
- `adb resume <task-id>` -- Resume a task (promotes backlog to in_progress)
- `adb archive <task-id>` -- Archive a task (generates handoff.md, moves ticket to _archived/, removes worktree)
- `adb unarchive <task-id>` -- Restore an archived task (moves ticket back from _archived/)
- `adb cleanup <task-id>` -- Remove a task's git worktree without archiving
- `adb migrate-archive` -- Move existing archived tasks to tickets/_archived/
- `adb status <task-id> <status>` -- Update task status
- `adb priority <task-id> <priority>` -- Update task priority
- `adb update <task-id>` -- Generate stakeholder update plan
- `adb sync-context` -- Regenerate AI context files
- `adb exec <cli> [args...]` -- Execute external CLI with alias resolution
- `adb run <task-name>` -- Run a Taskfile task
- `adb version` -- Print version information

## Linter Configuration

Enabled linters (via `.golangci.yml`): errcheck, gosimple, govet, ineffassign, staticcheck, unused, gosec, bodyclose, exhaustive, nilerr, unparam. gosec and unparam are excluded from test files.

## Additional Context

@docs/architecture.md
@docs/commands.md

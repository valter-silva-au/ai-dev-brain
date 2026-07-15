# AI Dev Brain (adb)

A Go CLI that wraps AI coding assistants with persistent context, a task
lifecycle, knowledge extraction, and observability. It manages tasks and git
worktrees, syncs context files, tracks stakeholder communications, reconciles
issues with GitHub/GitLab, and emits a structured event log across many repos.
On top of that dev-tooling core it also runs a **Founder-Playbook OS**: a
connected Org→Initiative→Stage graph that models and *enforces* the
Idea→MVP→Launch→Scale lifecycle through adversarial gates, declarative automation,
staged ingestion, ADR/compliance governance, and a GTM stack (see @docs/learning/L500-founder-playbook-os.md).

> This root file is intentionally short. The reference material lives in the
> imported docs below — read them for anything not on this page.

## Toolchain facts (verified against the code)

- **Go 1.25.5** (`go.mod`).
- Cobra (CLI), Viper (config), `yaml.v3` (persistence), `pgregory.net/rapid`
  (property tests). GoReleaser + golangci-lint + Docker multi-stage build.
- 6 property-test files (`*_property_test.go`): `core/conflict`, `core/taskid`,
  `integration/worktree`, `storage/backlog`, `pkg/models/backlog`, `pkg/models/task`.

## Architecture spine

Layered: **CLI → Core → Storage / Integration / Observability**. Dependencies are
wired in `internal/app.go` via constructor injection. To avoid import cycles,
`core/` defines *local interfaces* (`BacklogStore`, `ContextStore`,
`WorktreeCreator/Remover`, `EventLogger`, `SessionCapturer`) and `app.go`'s
adapter structs bridge them to the concrete `storage`/`integration`/`observability`
implementations. `internal/cli/vars.go` holds the package-level singletons set at
`App` init; thin `RunE` handlers guard `if App == nil` and delegate to core.

The full package map, subsystem inventory, command surface, and event schema are
in **@docs/claude/subsystems.md** — read that before assuming anything exists.

## Go coding standards

- Wrap errors: `fmt.Errorf("context: %w", err)`; messages start lowercase.
- Define interfaces where they are consumed (see the local interfaces above);
  constructors return interfaces: `func NewFoo(...) FooInterface`.
- `time.Now().UTC()` for timestamps. Dirs `0o755`, files `0o644`.
- Persisted struct fields carry `yaml` tags.
- New top-level command → register in `internal/cli/root.go:NewRootCmd`, keep the
  handler thin, put logic in `core`.
- New event type → declare it, add to `KnownEventTypes`
  (`internal/observability/schema.go`), and cover it in `schema_test.go`.

## Task model

- **Types** (`pkg/models/task.go` → `ValidTaskTypes`): 8 Conventional **code**
  types `feat`, `fix`, `refactor`, `docs`, `chore`, `test`, `perf`, `spike` +
  2 **non-code** types `work` (artifact/graph deliverable — **no worktree/branch**,
  exempt from code gates) and `prototype` (time-boxed experiment; `chore/` branch
  like `spike`). Types are stage-agnostic. Create with
  `adb task create <branch> --type=`. `bug` is a **retired** alias — rejected at
  create (use `fix`); `adb task migrate-types` rewrites old `bug` entries.
- **Statuses**: `backlog → in_progress → blocked | review → done → archived`.
- **Priorities**: `P0` (critical) … `P3` (low), default `P2`.
- **Task IDs**: `{prefix}-{counter:05d}` (e.g. `TASK-00001`), file-based counter.
- Tickets live under `tickets/<platform>/<org>/<repo>/TASK-XXXXX-<slug>/`
  (the correlation layout — see L100). Repo-less tasks land in `tickets/_local/`.

## Common commands

```bash
make all                              # fmt, vet, lint, test, build (green before PR)
go test ./... -race -count=1          # full suite with race detector
go test ./... -run TestProperty -v    # property tests only
go build -ldflags="-s -w" -o adb ./cmd/adb/
go run ./cmd/adb/ [command]
```

## Base-path resolution

`ADB_HOME` env var → else walk up for `.taskconfig` → else cwd. Config precedence
(most-specific wins): `.taskrc` (per-repo) > `orgs/<id>/config.yaml` (per-org) >
`.taskconfig` (global) > defaults. The org tier is optional — its id comes from
`ADB_ORG` or the `org:` field in `.taskrc`; absent → the historical two-tier merge.
`adb config show`/`get` inspect the resolved tiers.

## Deeper docs (imported)

- @docs/claude/subsystems.md — package map, subsystem inventory, command surface, event schema.
- @docs/learning/L100-fundamentals.md — task model, status lifecycle, type taxonomy, branch shape, correlation layout.
- @docs/learning/L200-daily-workflows.md — the daily loop, VS Code extension, event streaming, backlog maintenance.
- @docs/learning/L300-integrations.md — issue sync, cloud sync, the MCP server.
- @docs/learning/L400-architecture-and-extending.md — interfaces/adapters, the event pipeline, and HOW-TOs (add a command / event type / sync provider).
- @docs/learning/L500-founder-playbook-os.md — the Founder-Playbook OS: Org→Initiative→Stage lifecycle, the four gates (incl. the human-only Launch→Scale), the graph + catalog, config tiers, the rule engine + conformance-drift, ingestion, ADR/tech-debt/compliance/SLO governance, CRM/GTM, the governance stream, and the Claude Code plugin.

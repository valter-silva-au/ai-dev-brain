# L400 — Architecture & Extending

> **Tier:** L400 (advanced) · **Goal after this page:** *"I can contribute."*
>
> **You should already be comfortable with:** the task model, type taxonomy, and lifecycle
> ([L100 — Fundamentals](./L100-fundamentals.md)); the daily VS Code / parallel-terminal
> workflow ([L200 — Daily Workflows](./L200-daily-workflows.md)); and connecting adb to
> remotes/cloud/MCP ([L300 — Integrations](./L300-integrations.md)). This tier is for
> **contributors** — people who want to add a command, an event type, a sync provider, or an
> extension config key without breaking the seams that keep the codebase testable.

---

## 1. The big picture: logic in Go, a thin extension on top

`adb` is **one repository, two shipped artifacts**:

| Artifact | Lives in | Language | What it is |
|----------|----------|----------|------------|
| The `adb` CLI/core | `cmd/adb`, `internal/*`, `pkg/models` | Go | Every bit of behaviour — task lifecycle, storage, sync, observability, MCP server. |
| The VS Code extension | `vscode-extension/` | TypeScript | A **thin front-end** that shells out to the `adb` CLI. It owns *no* state. |

The load-bearing rule that makes the whole thing maintainable:

> **All logic is in Go. The extension never touches `adb` state directly — every action
> shells out to the `adb` CLI, mostly via `adb … --json`.**

The extension reads exactly two files on disk directly, and only as fallbacks:
`<ADB_HOME>/.events.jsonl` (the dashboard's fallback event feed when `adb events tail` isn't
available) and `~/.adb_terminal_launch.json` (the legacy launcher hand-off). Everything else
is an `execFile`/`spawn` of `adb` with an argv array — **never** a shell string.

```
┌──────────────────────────┐        execFile("adb", ["task","status","--json"])
│  VS Code extension (TS)   │  ───────────────────────────────────────────────►  ┌───────────┐
│  vscode-extension/src/    │                                                      │  adb CLI  │
│  - thin command handlers  │  ◄───────────────────────────────────────────────   │  (Go)     │
│  - webview dashboard      │        stdout: JSON / JSONL                          └─────┬─────┘
└──────────────────────────┘                                                            │
                                                                                         ▼
                                                                          internal/* + pkg/models
                                                                          (all behaviour + storage)
```

Because the extension only ever *invokes* the CLI, the Go binary is the single source of
truth. If you want the extension to do something new, the usual path is: **add it to the CLI
first, then have the extension shell out to it.**

### What was removed — don't go looking for it

The 2026-07-01 overhaul **deleted** the old web/cluster surface. If you find references to
these in older commits, they are **stale** and describe code that no longer exists:

- ❌ **`adb serve`** — the web-UI dashboard command. **Gone.** There is no HTTP server in the
  repo. The dashboard is now an in-editor VS Code **webview** (`vscode-extension/src/webview/`),
  and the terminal health view is `adb dashboard` (a Bubbletea TUI).
- ❌ **`internal/server/`** — the web server package. **Gone** (absent from the tree).
- ❌ **`internal/hive/`** — the "hive-mind" multi-agent cluster package. **Gone**, along with
  its `pkg/models/hive.go` data model and its design doc.

The **only** "serve" verb in the codebase is `adb mcp serve` — an MCP-over-stdio server
(see §7), which is unrelated to the removed web UI.

---

## 2. Interfaces + adapters: how the packages avoid an import cycle

The core design pattern is **"core defines interfaces, implementations live elsewhere,
adapters bridge them."** This lets `internal/core` (the task lifecycle engine) stay ignorant
of `internal/storage`, `internal/integration`, and `internal/observability` — no import
cycles, and every dependency is a mockable interface.

The wiring lives in one place: `internal/app.go:NewApp`. `App` is the dependency-injection
container. It:

1. Constructs the real implementations (`storage.NewFileBacklogManager`,
   `integration.NewGitWorktreeManager`, `observability.NewEventLog`, …).
2. Wraps each in a small **adapter** struct that satisfies the interface `core` declares
   (`backlogStoreAdapter`, `contextStoreAdapter`, `worktreeCreatorAdapter`,
   `worktreeRemoverAdapter`, `eventLoggerAdapter`, …).
3. Hands the adapters to `core.NewTaskManager(...)`.

```
pkg/models          plain data types (Task, TaskType, TaskStatus, Backlog, …) — no logic deps
   ▲
internal/core       TaskManager + the interfaces it needs (BacklogStore, WorktreeCreator,
   ▲                EventLogger, …). Knows NOTHING about storage/integration/observability.
   │  (adapters implement core's interfaces)
internal/app.go     NewApp() constructs concrete impls + adapters, injects them into core
   ▲
internal/cli        cobra commands; each reads the package-level `cli.App` set in cmd/adb/main.go
   ▲
cmd/adb/main.go     resolves ADB_HOME/base path, builds App, injects cli.App, runs root cmd
```

**Anchor:** `internal/app.go:NewApp` (the whole wiring), `internal/app.go` adapter structs
(e.g. `worktreeCreatorAdapter.CreateWorktree`, `eventLoggerAdapter.Log`).

The entrypoint `cmd/adb/main.go:main` resolves the workspace root
(`resolveBasePath`: `ADB_HOME` env → walk up for `.taskconfig`/`.taskrc` → cwd fallback),
builds the `App`, assigns it to the package-level `cli.App`, and executes the root command.
Every CLI handler starts with an `if App == nil { return fmt.Errorf("app not initialized") }`
guard (the one exception is `adb version`, which uses cobra `Run` not `RunE` and works
without a workspace).

**Why this matters when you extend:** if you add a new capability, decide which layer owns
it. Behaviour → `internal/core` (behind an interface if it needs storage/git/IO). A concrete
IO implementation → `internal/storage` or `internal/integration`. Then bridge it with an
adapter in `internal/app.go`. Do not make `core` import `storage`/`integration` — that's the
cycle the adapters exist to prevent.

---

## 3. The observability event pipeline

Every meaningful thing `adb` does is recorded as an append-only JSONL event. This is the
backbone the metrics, alerts, `adb events`, and the webview dashboard all read from.

### The pipeline

```
core/integration emits ─► observability.EventLog.Log(type, data)
                           writes one json.Marshal(Event)+"\n" line, under a mutex,
                           to  <ADB_HOME>/.events.jsonl   (wired in internal/app.go)
                                              │
              ┌───────────────────────────────┼───────────────────────────────┐
              ▼                                ▼                                ▼
   MetricsCalculator.ComputeMetrics   AlertEvaluator.EvaluateAll     adb events query / tail
   (replays the whole log)            (thresholds over metrics)       (the VS Code webview feed)
```

- **`Event`** = `{Timestamp time.Time, Type EventType, Data map[string]interface{}}`
  (`internal/observability/eventlog.go`). `Timestamp` is always `time.Now().UTC()` at log
  time. Numeric payload values round-trip through `encoding/json` as `float64` — type-assert
  accordingly when reading `Data`.
- **`EventLog.Log`** is thread-safe (mutex) and **non-fatal**: if the log file can't be
  created, `NewEventLog` sets `enabled=false` and `Log()` silently no-ops. `ReadAll()`
  gracefully **skips** malformed lines rather than erroring.
- The log path is `<basePath>/.events.jsonl`, set in `internal/app.go`.

### The schema is a contract

`internal/observability/schema.go` declares **`KnownEventTypes`** — the authoritative,
ordered set of every event type `adb` emits or reserves. Consumers (metrics, dashboards,
`adb events`) rely on this being complete. It is exactly these 20:

```
task.created        task.completed(reserved)  task.status_changed
task.archived       task.unarchived           task.priority_changed   task.deleted
worktree.created             worktree.removed
knowledge.extracted(reserved)
agent.session_started        agent.session_active        agent.session_ended
issue.synced        issue.conflict            issue.skipped
stage.advanced      stage.override
config.task_context_synced
serena.effectiveness_recorded
```

The const declarations are **split across two files** (deliberately, for locality):
`eventlog.go` declares the `task.*` (subset) / `worktree.*` / `knowledge.*` /
`agent.session_started` / `issue.*` consts; `schema.go` declares the remaining
`task.archived` / `task.unarchived` / `task.priority_changed` / `task.deleted` /
`agent.session_active` / `agent.session_ended` / `stage.advanced` / `stage.override` /
`config.task_context_synced` (`session_active` is the same-machine live-digest heartbeat
added after the overhaul; the two `stage.*` types are the founder-playbook gate events —
see L500; `config.task_context_synced` is emitted by `adb task resume` on a worktree
context refresh, #155). If you're hunting "the full list", the aggregate is
`KnownEventTypes` in `schema.go`.

> **Governance mirror:** the two `stage.*` events are *also* written to a **separate**
> `.governance.jsonl` (read via `adb governance`) — the same types, a second sink, kept
> distinct from the high-volume dev telemetry (D19/#137). See L500 §8.

> **Gotcha — the `cloud.*` events are NOT in `KnownEventTypes`.** `adb sync cloud` emits
> `cloud.sync_pushed` / `cloud.sync_pulled` / `cloud.sync_status` / `cloud.sync_destroyed`,
> but those consts are declared locally in `internal/cli/sync_cloud.go` and were never added
> to the schema set. So `IsKnownEventType()` returns `false` for them, and any tool that uses
> `KnownEventTypes` as an allowlist will drop cloud-sync events. When documenting the events
> surface, say "`KnownEventTypes` is every event *except* the four `cloud.*` ones."

### Reading events

```bash
# One-shot query — emits a single indented JSON ARRAY with --json
adb events query --type task.status_changed --since 7d --json
adb events query --task TASK-00042

# Live tail — --json here emits JSONL (one object PER LINE), NOT an array
adb events tail --follow --json
```

`adb events query --json` and `adb events tail --json` intentionally have **different**
shapes (array vs JSONL) — the webview reloads its overview from the query array and streams
the tail line-by-line. The `--since` parser is custom: a trailing `d` means days (`7d`,
`30d`); it falls back to `time.ParseDuration` for `h`/`m`/`s`. It is shared by
`adb metrics --since` and `adb events query --since` (`internal/cli/metrics.go:parseDuration`).

---

## 4. HOW-TO: add a new top-level `adb` command

The entire top-level command surface is registered in **one place**:
`internal/cli/root.go:NewRootCmd` — 40 `AddCommand` calls, no others. To add a command:

1. **Write the constructor.** Add `internal/cli/mything.go` with a `NewMyThingCmd()
   *cobra.Command`. Follow the house pattern:

   ```go
   package cli

   import (
       "fmt"
       "github.com/spf13/cobra"
   )

   // NewMyThingCmd creates the `adb mything` command.
   func NewMyThingCmd() *cobra.Command {
       var someFlag string
       cmd := &cobra.Command{
           Use:   "mything",
           Short: "One-line description",
           RunE: func(cmd *cobra.Command, args []string) error {
               if App == nil { // the standard guard — App is the injected DI container
                   return fmt.Errorf("app not initialized")
               }
               // delegate to App.TaskManager / App.EventLog / a core service …
               fmt.Println("did the thing:", someFlag)
               return nil
           },
       }
       cmd.Flags().StringVar(&someFlag, "some-flag", "", "what it does")
       return cmd
   }
   ```

2. **Register it** in `internal/cli/root.go:NewRootCmd`:

   ```go
   rootCmd.AddCommand(NewMyThingCmd())
   ```

3. **Put the *logic* in `internal/core` (or a service), not the handler.** The cobra `RunE`
   should be thin — parse flags, call into `App.TaskManager`/a core type, print output.
   That keeps the behaviour unit-testable without a cobra harness.

4. **Emit an event** if it changes task state (see §5), and **test** it (see §8).

> Sub-command groups (like `adb task`, `adb sync`, `adb events`) follow the same shape: a
> parent `cobra.Command` with no `RunE`, plus `parent.AddCommand(child1(), child2())`. See
> `internal/cli/task.go:NewTaskCmd` (14 subcommands) and `internal/cli/sync.go:NewSyncCmd`
> (8 subcommands) for the canonical examples.

---

## 5. HOW-TO: add a new `EventType`

Events are a contract, so adding one has **three mandatory steps** (spelled out in the
comment block at the top of `internal/observability/schema.go`):

1. **Declare the const** — either in `internal/observability/eventlog.go` (next to the
   `task.*` / `issue.*` block for locality) or in `schema.go`:

   ```go
   // internal/observability/eventlog.go
   const EventTaskSnoozed EventType = "task.snoozed"
   ```

2. **Add it to `KnownEventTypes`** in `internal/observability/schema.go` (place it in
   lifecycle order) and document its payload keys in the file's header comment:

   ```go
   var KnownEventTypes = []EventType{
       // …
       EventTaskSnoozed,
   }
   ```

3. **Emit it** from wherever the state change happens — via the `EventLogger` interface in
   `core` (so `core` doesn't import `observability` directly), which the
   `eventLoggerAdapter` in `internal/app.go` bridges to `EventLog.Log`. The `EventLogger.Log`
   signature takes a plain `string`, and `core` can't see the `observability` const, so it
   emits the **raw type string** (matching the existing `tm.eventLogger.Log("task.created", …)`
   calls in `internal/core/taskmanager.go`) — keep the literal in lock-step with the const:

   ```go
   tm.eventLogger.Log("task.snoozed", map[string]interface{}{
       "task_id": task.ID,
       "until":   until.Format(time.RFC3339),
   })
   ```

4. **Keep the drift guard green.** `internal/observability/schema_test.go` has
   `TestKnownEventTypes_CoversEmittedSet` — it asserts every *emitted* type is
   `IsKnownEventType`. If you emit a type you forgot to add to `KnownEventTypes`, this test
   fails. (There's a sibling test that keeps the two *reserved-but-unemitted* types
   — `task.completed`, `knowledge.extracted` — in the set; `worktree.created`
   graduated to an emitted type in #206.)

> Don't repeat the `cloud.*` mistake: if a command emits an event, its type belongs in
> `KnownEventTypes`, not just as a local const in a `cli/*.go` file.

---

## 6. HOW-TO: add a sync `Provider` (a new issue backend)

Issue sync (`adb sync issues`) reconciles adb tickets with remote issues. It is built around
a single seam: **`internal/integration/issuesync/provider.go:Provider`**.

```go
type Provider interface {
    Name() string                                                    // "github" / "gitlab" — used in logs
    Get(owner, name string, number int) (RemoteIssue, bool, error)   // found=false ⇒ create; number 0 ⇒ unlinked
    Create(owner, name string, want RemoteIssue) (RemoteIssue, error)
    Update(owner, name string, number int, want RemoteIssue) (RemoteIssue, error)
}
```

The existing implementations shell out to the host CLI, mirroring the `os/exec` model:
`github.go` runs `gh issue view/create/edit/close/reopen`; `gitlab.go` runs
`glab issue view/create/update/close/reopen`. **Auth is per-host and owned entirely by the
user's `gh`/`glab` login** — adb never reads `~/.config/gh/hosts.yml`, never takes a
`--token` flag, and never writes a token/PII into `backlog.yaml`, `status.yaml`, or
`.events.jsonl`. Argv-boundary tests enforce this (a token-shaped arg in the exec call fails
the test).

To add, say, a Gitea backend:

1. **Implement the interface** in `internal/integration/issuesync/gitea.go` with a
   `NewGiteaProvider()` returning something that satisfies `Provider`. Shell out to the
   host's `tea` CLI, same as `github.go`/`gitlab.go`. Do **not** invent a token flag —
   rely on the host CLI's own auth.

2. **Wire the selector.** `internal/integration/issuesync/select.go:ProviderFor` maps a
   platform-qualified `Repo` (`<host>/<org>/<repo>`) to a provider. Add a case:

   ```go
   case strings.HasPrefix(host, "gitea."):
       return NewGiteaProvider(), owner, name, true
   ```

   `ProviderFor` returns `ok=false` (⇒ ticket is **skipped**, logged as `issue.skipped`) for
   repo-less `_local` tickets, local-path repos, non-3-part repos, and unknown hosts
   (including enterprise-internal hosts that aren't `github.com` / a `gitlab` host).

3. **Reuse the reconcile engine — don't reinvent it.** The pure, provider-agnostic
   last-writer-wins logic lives in `internal/integration/issuesync/reconcile.go:Reconcile`
   over a *fixed* synced-fields allowlist: **title, body, labels, status, priority** (owner,
   tags outside labels, timestamps, and paths are never overwritten). Change-detection uses
   the stored per-sync baseline `Task.SyncHash`, not the local `Updated` timestamp. Status
   maps via `mapping.go` (done/archived → closed; others → open + an `adb:<status>` label).
   Your provider only translates the abstract `RemoteIssue` to/from your backend's API.

4. **Add a fake** for tests (see `issuesync_test.go`'s `fakeProvider`) — the interface is
   designed so unit tests never shell out.

The linkage fields on the task model (`pkg/models/task.go`) are `RemoteIssue int`,
`RemoteURL string`, `LastSynced time.Time`, `SyncHash string` — all `yaml:",omitempty"` so
pre-sync backlog entries marshal byte-identically.

```bash
# Dry-run one repo's sync without writing anything
adb sync issues --repo github.com/valter-silva-au/ai-dev-brain --dry-run
adb sync issues --direction push          # both | push | pull (default both)
```

---

## 7. HOW-TO: add a VS Code extension config key

Extension config keys are two coordinated edits (schema + normalizer) plus a read. The
existing keys back the Start-All / tmux / dashboard behaviour; there are 9 today (e.g.
`adb.binaryPath`, `adb.home`, `adb.dashboard.maxFeedItems`, `adb.startAll.groupByOrg`,
`adb.tmux.enabled`).

1. **Declare it** in `vscode-extension/package.json` under
   `contributes.configuration.properties`, with a type, default, and description:

   ```json
   "adb.startAll.myOption": {
     "type": "boolean",
     "default": true,
     "description": "What this toggles."
   }
   ```

2. **Normalize it** in a **pure** normalizer in `vscode-extension/src/config.ts` (no
   `vscode` import, so it's unit-testable under plain node). VS Code hands back a raw value
   that may not match the declared type — a user-edited `settings.json` can lie — so coerce
   defensively (see `normalizeStartAllConfig` clamping `cap` to a non-negative integer, and
   `normalizeTmuxConfig` sanitizing the prefix):

   ```ts
   export interface StartAllConfig { /* … */ myOption: boolean; }
   export function normalizeStartAllConfig(raw: RawStartAll): StartAllConfig {
     return { /* … */ myOption: raw.myOption !== false /* undefined → true */ };
   }
   ```

3. **Read it** in `vscode-extension/src/extension.ts` — the only *impure* layer — with
   `getConfig().get<T>(...)`, pass the raw value into the normalizer, and feed the typed
   result to the pure planners (`launch.ts`, `orggroup.ts`) or the terminal-env builder
   (`adbEnv()`). Keep the `vscode` API confined to `extension.ts`; all decision logic stays
   in the pure modules so it tests without VS Code.

4. **Test it** by adding a case to `vscode-extension/src/config.test.ts` (the hand-rolled
   harness — see §8).

> **Architecture note:** the extension is a thin CLI front-end. Most config either shapes how
> terminals are laid out (`startAll.*`) or is threaded into the child `adb` process's
> environment (`adbEnv()` sets `ADB_TMUX`, `ADB_TMUX_PREFIX`, `ADB_HOME`). New behaviour that
> needs to touch adb state belongs in the CLI, surfaced through a flag the extension shells to.

---

## 8. Testing: two planes

`adb` has two independent test planes. **We work test-driven** (red → green → refactor) —
see the repo's TDD skill/conventions — so a new command/event/provider starts with a failing
test.

### Go plane (CLI/core)

Driven by the `Makefile`:

```bash
make test      # go test -race -v ./... -count=1   (race detector + no test cache)
make lint      # golangci-lint run ./...
make vet       # go vet ./...
make fmt       # gofmt -s -w .
make security  # govulncheck ./...
make all       # fmt + vet + lint + test + build
make build     # builds ./cmd/adb with version ldflags
make install-local   # build + install to ~/.local/bin (re-signs on macOS Apple Silicon)
```

Follow Go table-driven / subtest conventions. Interfaces exist precisely so you can inject
fakes: `core.TaskManager`'s dependencies are all interfaces, `issuesync.Provider` has a
`fakeProvider`, and `observability.Chat` refuses a `nil` runner so a test can never
accidentally shell out to `claude`. The event schema has its own **drift guard**
(`schema_test.go:TestKnownEventTypes_CoversEmittedSet`).

### npm plane (extension)

The extension has a **hand-rolled test harness** — no Mocha/Jest, no VS Code test runner:

```bash
cd vscode-extension
npm test        # tsc -p ./  &&  node out/run-tests.js
npm run coverage   # same, under c8, over the pure logic modules
```

`out/run-tests.js` registers cases across the `*.test.ts` files (`launch.test.ts`,
`actions.test.ts`, `orggroup.test.ts`, `config.test.ts`, `webview/*.test.ts`, …). The key
enabler: **every logic module is pure (no `vscode` import)** — `config.ts`, `launch.ts`,
`orggroup.ts`, `terminals.ts`, `icons.ts`, and the `webview/*` view-models all test under
plain node. Only `extension.ts` imports `vscode`, and it's kept as thin glue. When you add
logic, put it in a pure module and test it here; keep the impure `vscode` calls out of it.

---

## 9. Quick map of `internal/` (where things live)

| Package | Owns |
|---------|------|
| `internal/cli` | Every cobra command (`root.go:NewRootCmd` registers all 40 top-level cmds). |
| `internal/core` | `TaskManager`, the lifecycle engine, and the interfaces it depends on; `ResolveTicketDir` (`resolve.go`), bootstrap/layout logic. |
| `internal/storage` | File-backed `BacklogManager`, `ContextManager`, `SessionStoreManager` (`backlog.yaml`, ticket dirs, sessions). |
| `internal/integration` | Git worktrees, terminal-state writer, `reposync`, and `issuesync/` (the `Provider` seam), `cloudsync/` (S3 archive engine). |
| `internal/observability` | `EventLog`, `EventType`/`KnownEventTypes` schema, `MetricsCalculator`, `AlertEvaluator`, `Chat`. |
| `internal/mcpserver` | `adb mcp serve` — the MCP-over-stdio adapter (`server.go:New`/`Serve`/`registerTaskTools`). Thin: delegates to the same `App.TaskManager`/`BacklogManager` the CLI uses. |
| `internal/hooks` | Claude Code hook processors (`adb hook …` reads event JSON from stdin). |
| `internal/memory` | Vector memory store behind `adb memory`. |
| `internal/scheduler` | The `adb scheduler` background daemon. |
| `pkg/models` | Plain data types: `Task`, `TaskType`, `TaskStatus`, `Backlog`, `MergedConfig`. |
| `internal/app.go` | `NewApp` — the DI container + adapters that wire it all together. |

The MCP server (`adb mcp serve`) exposes 7 task-lifecycle tools
(`adb_task_list/create/start/close/update/start_all/close_all`) plus 4 graph/knowledge tools
(`graph_neighbors`, `related_tickets`, `get_initiative`, `search_knowledge`) — every one
delegates to the same `App` subsystems as the CLI (TaskManager, GraphManager, StageManager, the
memory store), so behaviour and storage are identical regardless of entry point. It exposes
**no** issue-sync or cloud-sync tools. Its `parseTaskType` enforces the full `ValidTaskTypes`
set (8 Conventional code types + `work`/`prototype`) and rejects the retired `bug` alias with a
hint to use `fix`. `search_knowledge` degrades gracefully (a clear notice, never an error) when
the workspace has no vector-memory store.

---

## 10. When you touch task types, statuses, or the branch shape

These are frozen contracts (details in [L100 — Fundamentals](./L100-fundamentals.md)) — but
as a contributor you'll trip on them:

- **Task types** live in `pkg/models/task.go:ValidTaskTypes`: the 8 Conventional-Commits
  **code** types `feat, fix, refactor, docs, chore, test, perf, spike` **plus** two
  **non-code** types (D10) `work` and `prototype`. `work` is an artifact/graph deliverable —
  `TaskManager.Create` builds **no worktree and no branch** for it (even with `--repo`), so it
  is exempt from code gates by construction; `prototype` is a time-boxed experiment that is
  code-shaped (gets a worktree/`chore/` branch). All types are **stage-agnostic**. `bug` is
  **retired** — not in `ValidTaskTypes`; the create path and the MCP server reject it.
  `ConventionalType` maps legacy `bug → fix` and `spike`/`prototype → chore` for branch naming
  (`work` never branches); `adb task migrate-types --apply` rewrites old `bug` entries.
- **Branch names** come from `BranchName` = `<conventional-type>/<slug>` (e.g.
  `chore/my-spike`), **never** `task/<id>`.
- **`backlog.yaml` is the sole authoritative type store** (`status.yaml` has no type field),
  so anything that migrates/normalizes types touches only `backlog.yaml` and defaults to
  dry-run (`--apply` to write).

If you're adding another type, add it to `ValidTaskTypes`, give it a `ConventionalType` mapping
(or let it fall through 1:1), add an icon in `vscode-extension/src/icons.ts`, and add it to
the extension's create QuickPick in `extension.ts` (which today still offers only a legacy
4-type subset — `feat/bug/spike/refactor` — and so exposes **neither** the full Conventional
set **nor** the new `work`/`prototype` non-code types; a known gap, don't assume the palette
exposes the full taxonomy — use the CLI `--type` for anything outside that subset).

---

## Where to go next

- Contributing a fix end-to-end? Read [L100 — Fundamentals](./L100-fundamentals.md) for the
  task model and lifecycle you'll be emitting events into, [L200 — Daily
  Workflows](./L200-daily-workflows.md) for the extension/parallel-terminal loop, and
  [L300 — Integrations](./L300-integrations.md) for the issue-sync / cloud-sync / MCP
  surfaces you may extend.
- The authoritative command surface is always `internal/cli/root.go:NewRootCmd` — the README
  command list is a subset, not the contract.

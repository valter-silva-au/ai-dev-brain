# L400 — Architecture & Extending

> **Tier:** L400 (advanced) · **Goal after this page:** *"I can contribute."*
>
> **You should already be comfortable with:** the task model, type taxonomy, and lifecycle
> ([L100 — Fundamentals](./L100-fundamentals.md)); the daily parallel-terminal workflow
> ([L200 — Daily Workflows](./L200-daily-workflows.md)); and connecting adb to
> remotes/cloud/MCP ([L300 — Integrations](./L300-integrations.md)). This tier is for
> **contributors** — people who want to add a command, an event type, a sync provider, or a
> config key without breaking the seams that keep the codebase testable.

---

## 1. The big picture: one Go binary, terminal-native

`adb` is **one repository, one shipped artifact**: the `adb` CLI (`cmd/adb`, `internal/*`,
`pkg/models`). All behaviour — task lifecycle, storage, sync, observability, MCP server —
lives in Go.

It is deliberately **not** tied to an editor. A TypeScript VS Code extension shipped until
2026-09-14 as a thin front-end that shelled out to the CLI; it was removed, along with the
`~/.adb_terminal_launch.json` hand-off, the terminal-state file, and the `--here` flag that
existed only to bypass that hand-off. Launching in place is now unconditional.

The load-bearing consequence for contributors: **there is exactly one place behaviour can
live.** If you want adb to do something new, it goes in Go and is reachable from the CLI and
the MCP server — the two surfaces any agent can drive.

```
      any MCP client / your shell
                  │
                  ▼
        ┌──────────────────┐
        │  adb CLI  (Go)   │   cmd/adb + internal/v3cli + internal/cli
        └────────┬─────────┘
                 ▼
        internal/* + pkg/models   (all behaviour + storage)
```

### What was removed — don't go looking for it

The 2026-07-01 overhaul deleted the old web/cluster surface, 2026-09-14 deleted the IDE
surface, and 2026-09-15 (TASK-00039) cut the command surface from 46 to 40. If you find
references to any of these, they are **stale**:

- ❌ **`adb dashboard`** and `internal/cli/dashboard.go` — the Bubble Tea TUI. Gone, along
  with the `bubbletea`/`bubbles`/`lipgloss` dependencies, which nothing else used.
- ❌ **`adb chat`**, `internal/cli/chat.go`, and `internal/observability/chat.go` — the
  one-shot LLM adapter and its `claude -p` shell-out.
- ❌ **`adb exec`** / **`adb run`**, `internal/cli/exec.go`, `internal/cli/run.go`,
  `internal/core/taskfilerunner.go`, `internal/integration/cliexec.go`, and
  `internal/integration/taskfilerunner.go` — generic process runners. The last two were a
  mutually-referencing test-only island once `exec` went.
- ❌ **`adb team`** / **`adb agents`** — they only ever wrote a static plan file and printed
  a hardcoded list. `internal/cli/team.go` became `internal/cli/mcp.go`, since `NewMCPCmd`
  was all that remained in it.
- ❌ **`adb task start-all`** / **`close-all`**, and `core.BulkResult`/`StartAll`/`CloseAll`.
  A shell loop replaces them and reports which tickets moved.
- ⚠️ **`adb sync` is retired, not removed** — its children were redistributed by noun
  (`context`, `wiki`, `issues`, `archive`, `harness`), and every old spelling survives as a
  **hidden deprecated alias**. Same for `adb memory` → `adb context memory` (pruned to
  `index`/`search`), `adb repos` → `adb repo pull`/`inventory`, and `adb plugin` →
  `adb harness`. The aliases live in `internal/cli/namespaces.go`; the mechanism is
  `deprecatedAlias` in `internal/cli/alias.go`, which builds each one from the target's own
  constructor so the two cannot drift in flags or defaults. Its notice goes to **stderr**
  by hand rather than through cobra's `Deprecated` field, because cobra prints that one via
  `OutOrStderr()` — which resolves to *stdout* as soon as any ancestor calls `SetOut`, and a
  sentence prepended to `--json` output is a silently broken pipeline.
- ❌ **`adb session`** and `internal/cli/session.go` — the captured-session surface,
  together with `internal/integration/transcript.go` (Claude Code JSONL parsing), which
  had **no callers even before this**. Note the App-level session store
  (`storage.SessionStoreManager`, `sessionCapturerAdapter`) is still wired but provably
  unused: `TaskManager.sessionCapturer` is assigned and never read. It is left in place
  deliberately — `sessions/` exists in real workspaces, so what happens to that data is a
  product decision, not a mechanical consequence of dropping the command.
- ❌ **`adb task run-with-ruflo`** and `internal/cli/task_runwith.go` — a launcher for one
  specific third-party tool. **Its removal moved real telemetry:** it was the *only*
  producer of `agent.session_started`/`agent.session_ended`, so emission moved to
  `internal/cli/launch.go:launchWorkflow`, the single seam every `task
  create`/`start`/`resume` launch goes through. Without that, `adb events digest` and
  `metrics.AgentSessions` would have reported nothing forever while the schema still
  claimed the events were emitted.
- ❌ **`adb task migrate-types`** / **`normalize-titles`** / **`normalize`** and the core
  funcs behind them (`DedupTicketSeed`, `NormalizeTicketFrontmatter`,
  `PlanTicketFrontmatter`) — one-shot repairs for pre-schema-change workspaces that had
  become permanent public surface. `adb task validate` still *reports* the drift.

- ❌ **`adb serve`** and `internal/server/` — the web-UI dashboard. Gone; there is no HTTP
  server in the repo.
- ❌ **`internal/hive/`** — the "hive-mind" multi-agent cluster package. Gone.
- ❌ **`vscode-extension/`** — the `adb-brain` extension. Gone (2026-09-14).
- ❌ **`internal/integration/terminalstate.go`**, `statedir.FileTerminalState`,
  `~/.adb_terminal_launch.json`, `core.TerminalStateUpdater`, and `--here`. All gone with it.
- ❌ **`RepoSyncManager`** (`internal/integration/reposync.go`) — the old fetch-all/ff-merge
  engine. Gone (2026-07-25); `adb repo pull` runs `PullAllRepos` (`reposync_walk.go`).
- ❌ **`FileChannel`** (`internal/integration/filechannel.go`) — a file-based
  inbox/outbox/archive message channel. Gone (2026-09-17), as a leftover that **never had a
  caller**: `NewFileChannel(` appears nowhere outside its own two files in *any* commit on
  *any* ref. Its ancestor was real — a Feb-2026 `NewFileChannelAdapter` wired through
  `core.NewChannelRegistry()` in `internal/app.go` — but that channel-adapter system did not
  survive the 2026-03-13 rebuild-from-spec, while the rebuild's TASK-020 re-authored a
  differently-shaped `FileChannel` whose consumer was never rebuilt. So it was born dead
  rather than orphaned later. It was **not** a reserved seam: nothing outside declared an
  interface it satisfied, and its own comments claimed no future caller — unlike the
  deliberately-kept App-level session store (below). `docs/RESEARCH.md` still describes it
  under the older `FileChannelAdapter` name; that file is historical (see below).
- ⚠️ `pkg/models/hive.go` still exists with **no importers** — dead code from the hive
  deletion. Don't build on it.
- ⚠️ The retired Hive-Mind design doc and the project narrative (both removed from the
  public tree, TASK-00070) described removed code, as do `docs/RESEARCH.md`
  and `docs/adb-llm-wiki-monorepo-assessment.md`. Historical.

The **only** "serve" verb is `adb mcp serve` — MCP over stdio (see §7), unrelated to the
removed web UI.

---

## 2. Interfaces + adapters: how the packages avoid an import cycle

The core design pattern is **"core defines interfaces, implementations live elsewhere,
adapters bridge them."** This lets `internal/core` (the task lifecycle engine) stay ignorant
of `internal/storage`, `internal/integration`, and `internal/observability` — no import
cycles, and every dependency is a mockable interface.

The wiring lives in one place: `internal/app.go:NewAppWithOptions` (which `NewApp` and
`NewAppIsolated` both call — see the constructor table below). `App` is the
dependency-injection container. It:

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

**Anchor:** `internal/app.go:NewAppWithOptions` (the whole wiring), `internal/app.go` adapter
structs (e.g. `worktreeCreatorAdapter.CreateWorktree`, `eventLoggerAdapter.Log`).

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

### `basePath` is not the whole boundary — the three constructors

`basePath` isolates exactly **one** thing: the **repo** config tier, `<basePath>/.taskrc`.
Three further inputs are ambient no matter what `basePath` says:

| Input | Where it comes from | How `NewApp` gets it |
|---|---|---|
| the **global** config tier | `$HOME/.taskconfig` | `app.go` passes an **empty** global path; `core.NewViperConfigManager` fills it in from `os.UserHomeDir()` |
| the **terminal-state** file | `$HOME/.adb_terminal_state.json` | the same empty-path default, in `integration.NewTerminalStateWriter` |
| the active **org** tier | `$ADB_ORG` | read inside `ConfigManager.LoadConfig` |

So `NewApp(t.TempDir())` merges the developer's real machine config, and the terminal-state
writer *writes* into their real home directory. `internal/app.go` therefore ships three
constructors over one `AppOptions` struct:

| Constructor | What it still resolves from `$HOME` / the environment |
|---|---|
| `NewApp(basePath)` | all three of the above — the historical behaviour, unchanged. This is what the CLI uses. |
| `NewAppIsolated(basePath)` | **nothing.** The global tier becomes `<basePath>/.taskconfig`, the terminal-state file becomes `<basePath>/.adb/terminal_state.json` (`statedir.FileTerminalState`), and `$ADB_ORG` is ignored. |
| `NewAppWithOptions(basePath, AppOptions{…})` | whatever you leave unset — the fields are `GlobalConfigPath`, `TerminalStatePath`, `Org`, `Isolated`. |

`AppOptions`' zero value reproduces the historical behaviour exactly, which is why `NewApp`
is now a one-line call into `NewAppWithOptions` and the CLI is unaffected.

**`Isolated` does not suppress the repo config's `org:` field**, and that distinction is
what makes the design correct rather than merely blunt. That field lives *inside*
`basePath`, so it is part of the workspace the caller handed over — workspace **data**, not
ambient state. Only the process-wide `$ADB_ORG` is dropped. `internal/core/config.go` models
this as `ConfigManagerOptions{Org, IgnoreOrgEnv}` plus a
`NewViperConfigManagerWithOptions` constructor, so org resolution now reads, most explicit
first:

```
AppOptions.Org (pinned — suppresses BOTH ambient sources)
  → $ADB_ORG (unless IgnoreOrgEnv)
    → the repo config's `org:` field
      → empty ⇒ no org tier (the historical two-tier merge)
```

Which constructor a **test** should use, and why a constructor beats `t.Setenv`, is §8.

---

## 3. The observability event pipeline

Every meaningful thing `adb` does is recorded as an append-only JSONL event. This is the
backbone the metrics, alerts, `adb events`, and the webview dashboard all read from.

### The pipeline

```
core/integration emits ─► observability.EventLog.Log(type, data)
                           writes one json.Marshal(Event)+"\n" line, under a mutex,
                           to  <ADB_HOME>/.adb/events.jsonl   (wired in internal/app.go)
                                              │
              ┌───────────────────────────────┼───────────────────────────────┐
              ▼                                ▼                                ▼
   MetricsCalculator.ComputeMetrics   AlertEvaluator.EvaluateAll     adb events query / tail
   (replays the whole log)            (thresholds over metrics)
```

- **`Event`** = `{Timestamp time.Time, Type EventType, Data map[string]interface{}}`
  (`internal/observability/eventlog.go`). `Timestamp` is always `time.Now().UTC()` at log
  time. Numeric payload values round-trip through `encoding/json` as `float64` — type-assert
  accordingly when reading `Data`.
- **`EventLog.Log`** is thread-safe (mutex) and **non-fatal**: if the log file can't be
  created, `NewEventLog` sets `enabled=false` and `Log()` silently no-ops. `ReadAll()`
  gracefully **skips** malformed lines rather than erroring.
- The log path is `<basePath>/.adb/events.jsonl` — `app.StatePath(statedir.FileEventsLog)` in
  `internal/app.go`. Every adb-owned state file resolves through that one seam
  (`internal/statedir.Path` → `<basePath>/.adb/<name>`, #186), so nothing joins `.adb_<thing>`
  onto the workspace root by hand. Workspaces predating #186 are fixed up by the one-shot,
  idempotent `migrateStateToADB` (`internal/migration.go`), which moves each legacy root-level
  file into `.adb/` only when the target is absent — so it never overwrites and never loses data.

### The schema is a contract

`internal/observability/schema.go` declares **`KnownEventTypes`** — the authoritative,
ordered set of every event type `adb` emits or reserves. Consumers (metrics, dashboards,
`adb events`) rely on this being complete. It is exactly these 19:

```
task.created        task.completed(reserved)  task.status_changed
task.archived       task.unarchived           task.priority_changed   task.deleted
worktree.created             worktree.removed
knowledge.extracted(reserved)
agent.session_started        agent.session_active        agent.session_ended
issue.synced        issue.conflict            issue.skipped
stage.advanced      stage.override
config.task_context_synced
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
> `.adb/governance.jsonl` (read via `adb governance`) — the same types, a second sink, kept
> distinct from the high-volume dev telemetry in `.adb/events.jsonl` (D19/#137). See L500 §8.

> **Gotcha — the `cloud.*` events are NOT in `KnownEventTypes`.** `adb archive` emits
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
`30d`); it falls back to `time.ParseDuration` for `h`/`m`/`s`, and it rejects a day count with
trailing text (`1.5d` is an error, not 24h — see [L200](./L200-daily-workflows.md)).

**There is one implementation of that rule: `observability.ParseDuration`.** It lives in
`internal/observability` because that is the lowest package owning the durations a user types —
the event/metrics window *and* the alert thresholds `internal/app.go` resolves — and because
`internal/cli` already imports it, so `internal/cli/metrics.go:parseDuration` is a one-line
delegate rather than a second copy. It is shared by `adb metrics --since`,
`adb events query --since` and `adb task timeline --since`. **Don't add a third spelling:** the
second one existed for about an hour and shipped a silent-truncation bug with it.

---

## 4. HOW-TO: add a new top-level `adb` command

The entire top-level command surface is registered in **one place**:
`internal/cli/root.go:NewRootCmd` — 39 `AddCommand` calls, no others (the composed
public root is `internal/v3cli/root.go:NewRoot`, which omits two of them and adds four
v3 commands). To add a command:

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
> `internal/cli/task.go:NewTaskCmd` (13 subcommands) and
> `internal/cli/namespaces.go:NewContextCmd` (4 subcommands) for the canonical examples.
> `namespaces.go` is also where to look for the **alias** pattern: a retired spelling is
> built from the target's own constructor via `deprecatedAlias` (`alias.go`), so a rename
> cannot silently drop a flag.

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

Issue sync (`adb issues sync`) reconciles adb tickets with remote issues. It is built around
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
`.adb/events.jsonl`. Argv-boundary tests enforce this (a token-shaped arg in the exec call fails
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
   (including Amazon-internal `code.aws.dev` / `code.amazon.com`).

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
adb issues sync --repo github.com/valter-silva-au/ai-dev-brain --dry-run
adb issues sync --direction push          # both | push | pull (default both)
```

---

## 6b. HOW-TO: add a coding agent

Agents are **data**. One descriptor in `agentRegistry` (`internal/cli/agents.go`) and
every dispatch reads it:

```go
"gemini": {
    Name:        "gemini",
    Binary:      "gemini",
    DisplayName: "Gemini CLI",
    Args: func(resume, priorSession bool) []string {
        if resume && priorSession {
            return []string{"--continue"}   // whatever the real CLI wants
        }
        return nil
    },
    HasPriorSession: geminiSessionExistsIn,
    TmuxPrefix:      "gm-",
    Verified:        false,
},
```

That is the whole change. `validAgentNames`, the `--agent` usage string, the flag's
rejection message, `agentDisplayName`, `priorSessionExists` and `launchAgent` are all
derived from the registry, and `internal/cli/agents_test.go` fails if a descriptor is
incomplete.

**It was not always one place.** Adding an agent used to mean five scattered edits — an
entry in `validAgents`, a case in `agentDisplayName`, an args builder, a probe, and a
branch in each of `launchAgent` and `priorSessionExists`. Five places is how an agent ends
up *half-wired*: accepted by `--agent`, then launching claude, which looks like it worked.

Four things to get right in a new descriptor:

1. **`Args` returns the WHOLE argv after the binary**, not just trailing flags, because
   some agents put the verb first (`codex resume --last`, `q chat`).
2. **Honour `priorSession`.** Every agent so far errors or misbehaves when told to continue
   a conversation that does not exist — Claude Code exits 1 with "No conversation found to
   continue". A freshly-created worktree never has one.
3. **Scope the resume to the directory.** This is the subtle one. `codex resume --last` is
   safe only because Codex filters by cwd unless given `--all`; an agent whose "resume last"
   is *global* would continue another worktree's conversation, which is worse than not
   resuming at all. Check before wiring it.
4. **Give it its own `TmuxPrefix`.** Two agents in one worktree would otherwise collide on
   a single tmux session.

`Verified` is documentation, not behaviour: set it `false` when you wired the agent from
its docs without driving the real CLI, and say so in L100's agent table. `amazon-q` is
`false` today for exactly that reason.

## 7. HOW-TO: add a config key

Config resolves **Repo > Org > Global > defaults** (`.taskrc` > `orgs/<id>/config.yaml` >
`$HOME/.taskconfig`). Free-form settings live under `custom_settings` as a flat
`map[string]string`.

1. **Read it through `MergedConfig.SettingSource`**, which returns the value *and* the tier it
   came from, so `adb config get <key> --source` can explain itself:

   ```go
   v, tier, ok := App.MergedConfig.SettingSource("launch_agent")
   ```

2. **Parse defensively in one place.** A config value is user input: it may be absent, blank,
   or the wrong shape. Put the parse in a pure function next to its consumer and unit-test it
   — `core.ParseInstructionPointers` is the pattern (it also *drops* entries containing a path
   separator, because those names get joined onto the workspace root).

3. **Name the fallback in the doc comment**, not just the code. `configuredAgent` and
   `configuredInstructionPointers` in `internal/cli` both spell out the precedence chain they
   implement.

> **A dotted key is a trap.** Viper treats `.` as a nesting delimiter, so
> `custom_settings: {programs.search_paths: …}` decodes as a nested map where a string was
> expected and used to kill *every* command in the workspace. `normalizeCustomSettings`
> (`internal/core/config.go`) flattens each tier back to dotted keys before decode. See
> L600 §11.

---

## 8. Testing: two planes

`adb` has two independent test planes. **We work test-driven** (red → green → refactor) —
see the repo's TDD skill/conventions — so a new command/event/provider starts with a failing
test.

The Go plane itself has **three levels**, and picking the right one matters:

| Level | Shape | Use it for |
|---|---|---|
| **Unit** | pure functions, `fstest.MapFS` for embedded-FS input | engine logic that should never depend on the shipped template tree |
| **In-process CLI** | swap the package-level `cli.App` for an **`internal.NewAppIsolated(t.TempDir())`**, call the cobra handler | rendered output, `--json` contracts, config resolution — fast, and the default choice |
| **Binary e2e** (`test/e2e/`) | build `./cmd/adb` once in `TestMain`, spawn it against temp workspaces | anything that only breaks when a **separate process** resolves its own workspace, config tiers, and embedded FS |

#### In-process: build the App isolated, don't juggle the environment

A `t.TempDir()` basePath is **not** enough. It isolates only the repo config tier; the
global tier, the terminal-state file, and the org tier still come from `$HOME` and the
process environment (§2). So an in-process test builds its container with
**`internal.NewAppIsolated(dir)`** rather than `NewApp(dir)` plus a pile of `t.Setenv`. Two
reasons that is a better *mechanism*, not just tidier:

- **Hermetic by construction, not per-test opt-in.** With `NewApp`, every new test has to
  *remember* to neutralize three ambient inputs — and one that forgets still passes, on the
  machine whose config happens to agree. That is precisely how two tests came to fail on one
  developer's machine (their real `.taskconfig` enabled hooks and an Ollama embedder) while
  staying green everywhere else, and how a real home directory quietly accreted 14 dead test
  entries from a test that *wrote* terminal state. `NewAppIsolated` removes the thing that
  can be forgotten.
- **It does not forbid `t.Parallel()`.** `t.Setenv` is incompatible with `t.Parallel` by
  design — the runtime refuses the combination — so env-based isolation silently makes every
  hermetic test serial. An explicitly-constructed `App` carries no such constraint.

Reach for `NewAppWithOptions` only when you want one input pinned rather than all of them
(e.g. `AppOptions{Isolated: true, Org: "acme"}` to exercise the org tier without a `.taskrc`).

#### Two rules the e2e level exists to enforce

Both learned the hard way — and note the contrast with the rule above: a **child process**
cannot be handed a constructor, so for `test/e2e/` environment pinning genuinely *is* the
only mechanism available.

- **Pin `ADB_HOME` *and* `HOME`, and strip inherited `ADB_*`.** Base-path resolution is
  `ADB_HOME` → walk up for `.taskconfig` → cwd. `ADB_HOME` is commonly exported
  session-wide, so a child `adb` with an inherited environment silently resolves the
  *developer's real workspace*. (The cwd fallback is not a safety net: `adb init project`
  scaffolds a `.taskrc`, **not** a `.taskconfig`.) `HOME` needs pinning **independently**,
  because the global config tier is `$HOME/.taskconfig` and follows `$HOME` whatever
  `ADB_HOME` says — pin only `ADB_HOME` and the developer's global config still merges into
  the binary under test. `test/e2e`'s `adbEnv` pins `HOME`/`USERPROFILE` and, for the same
  reason, `CLAUDE_CONFIG_DIR` (see L600 §14).
- **Never copy the built binary.** On macOS Apple Silicon, `cp` invalidates the Go linker's
  ad-hoc code signature and the copy is `SIGKILL`'d on exec — exit 137, no message. Build
  with `go build -o <final-path>` directly, or re-sign with `codesign --force --sign -`
  (which is what `make install-local` does).

The in-process pattern to copy is the `App`-swapping helper at the top of
`internal/cli/program_test.go`; the e2e harness is `test/e2e/main_test.go`.

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


---

## 9. Quick map of `internal/` (where things live)

| Package | Owns |
|---------|------|
| `internal/cli` | Every legacy cobra command (`root.go:NewRootCmd` registers 39; the public root is `internal/v3cli/root.go:NewRoot`, 40 visible top-level commands). |
| `internal/core` | `TaskManager`, the lifecycle engine, and the interfaces it depends on; `ResolveTicketDir` (`resolve.go`), bootstrap/layout logic. |
| `internal/storage` | File-backed `BacklogManager`, `ContextManager`, `SessionStoreManager` (`backlog.yaml`, ticket dirs, sessions). |
| `internal/integration` | Git worktrees, terminal-state writer, the bulk repo-pull engine (`reposync_walk.go:PullAllRepos`), and `issuesync/` (the `Provider` seam), `cloudsync/` (S3 archive engine). |
| `internal/observability` | `EventLog`, `EventType`/`KnownEventTypes` schema, `MetricsCalculator`, `AlertEvaluator`, `Chat`. |
| `internal/mcpserver` | `adb mcp serve` — the MCP-over-stdio adapter (`server.go:New`/`Serve`/`registerTaskTools`). Thin: delegates to the same `App.TaskManager`/`BacklogManager` the CLI uses. |
| `internal/hooks` | Claude Code hook processors (`adb hook …` reads event JSON from stdin). |
| `internal/memory` | Vector memory store behind `adb memory`. |
| `internal/scheduler` | The `adb scheduler` background daemon. |
| `internal/statedir` | The one convention for **where per-workspace state lives**: `.adb/` under the workspace root (#186). `Name`, the `File*` basename consts, and `Dir`/`Path`/`Ensure`. Stdlib-only on purpose, so `core`/`cli`/`hooks`/`internal` can all route state paths through it with no import cycle. New state file ⇒ add a `File*` const and call `Path` — don't join `.adb_<thing>` onto the root. |
| `pkg/models` | Plain data types: `Task`, `TaskType`, `TaskStatus`, `Backlog`, `MergedConfig`. (`hive.go` is dead — see §1.) |
| `internal/app.go` | The DI container + adapters that wire it all together, over three constructors: `NewApp` (environment-aware, what the CLI uses), `NewAppIsolated` (hermetic — nothing outside `basePath`, what tests use), `NewAppWithOptions` (`AppOptions{GlobalConfigPath, TerminalStatePath, Org, Isolated}`). See §2. |

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
  (`work` never branches). There is no migration command: retyping a legacy `bug` row
  is a manual `backlog.yaml` edit.
- **Branch names** come from `BranchName` = `<conventional-type>/<slug>` (e.g.
  `chore/my-spike`), **never** `task/<id>`.
- **`backlog.yaml` is the sole authoritative type store** (`status.yaml` has no type field),
  so anything that migrates/normalizes types touches only `backlog.yaml` and defaults to
  dry-run (`--apply` to write).

If you're adding another type, add it to `ValidTaskTypes`, give it a `ConventionalType` mapping
(or let it fall through 1:1). Since the taxonomy is validated in one place
(`validateTaskType`), that plus a `ConventionalType` entry is the whole change.

---

## Where to go next

- Contributing a fix end-to-end? Read [L100 — Fundamentals](./L100-fundamentals.md) for the
  task model and lifecycle you'll be emitting events into, [L200 — Daily
  Workflows](./L200-daily-workflows.md) for the parallel-terminal loop, and
  [L300 — Integrations](./L300-integrations.md) for the issue-sync / cloud-sync / MCP
  surfaces you may extend.
- The authoritative command surface is always `internal/cli/root.go:NewRootCmd` — the README
  command list is a subset, not the contract.

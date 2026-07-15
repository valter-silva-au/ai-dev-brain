# L200 — Daily Workflows

> **Tier goal:** *"I use adb fluently."* You already know the basics (see
> [L100 — Fundamentals](./L100-fundamentals.md)). This tier is the muscle-memory layer:
> living inside the VS Code extension, running many tickets in parallel through survivable
> terminals, steering work from the in-editor dashboard, and keeping your backlog tidy with the
> maintenance commands. When you want to know *why* the pieces fit together the way they do, jump
> to [L400 — Architecture & Extending](./L400-architecture-and-extending.md).

Everything here is real on the current `adb` and the `adb-brain` VS Code extension (`v0.4.0`).
Every command and flag below is taken directly from the code — no invented surface.

---

## Table of contents

- [The daily loop at a glance](#the-daily-loop-at-a-glance)
- [Living in the VS Code extension](#living-in-the-vs-code-extension)
  - [The tickets tree](#the-tickets-tree)
  - [Start All Tasks — org grouping + priority order](#start-all-tasks--org-grouping--priority-order)
  - [The six Start-All / tmux config toggles](#the-six-start-all--tmux-config-toggles)
  - [tmux-durable terminals (survive a reload)](#tmux-durable-terminals-survive-a-reload)
  - [Ad-hoc Claude sessions (cmd/ctrl+shift+`)](#ad-hoc-claude-sessions-cmdctrlshift)
- [The in-editor dashboard](#the-in-editor-dashboard)
  - [The event feed](#the-event-feed)
  - [The org overview](#the-org-overview)
  - [The chat-steer box](#the-chat-steer-box)
- [Watching the event stream from the CLI](#watching-the-event-stream-from-the-cli)
- [Backlog maintenance](#backlog-maintenance)
  - [migrate-types (bug → fix)](#migrate-types-bug--fix)
  - [normalize-titles (strip doubled prefixes)](#normalize-titles-strip-doubled-prefixes)
- [Real command sequences](#real-command-sequences)
- [Where to go next](#where-to-go-next)

---

## The daily loop at a glance

A fluent day with adb looks like this:

1. Open your workspace in VS Code (the folder that contains `backlog.yaml` — your `ADB_HOME`).
2. Hit **ADB: Start All Tasks** to fan every launchable ticket out into its own terminal tab,
   grouped by org, ordered by priority.
3. Work each ticket's Claude session. Tabs are tmux-hosted, so a window reload reattaches instead
   of restarting.
4. Keep the **ADB Dashboard** webview open on the side for a live event feed + per-org status
   overview, and use its chat box to steer a ticket without leaving the editor.
5. Tail `adb events` in a scratch terminal when you want the raw stream.
6. When a ticket lands, close it; when you're done for the day, **ADB: Close All Tasks**.

The extension is a **thin front-end** — it never touches adb's state files directly. Every action
shells out to the `adb` CLI via `execFile`/`spawn` (argv arrays, never a shell string). The one
file it reads directly is `<ADB_HOME>/.events.jsonl`, and only as a *fallback* feed source when the
`adb events` command isn't available (see [the event feed](#the-event-feed)).

---

## Living in the VS Code extension

Install the extension (`vscode-extension/`, publisher `valter-silva-au`, name `adb-brain`). It
activates `onStartupFinished` and contributes 13 commands, one keybinding, a tickets tree view, and
9 configuration keys. Command handlers, the tree provider, and the terminal machinery all live in
`vscode-extension/src/extension.ts`.

### The tickets tree

The **ADB Tickets** view (`adb.ticketsView`) lists your backlog. Behind it, `listTasks()` runs

```bash
adb task status --json
```

and parses the result as an array (`vscode-extension/src/extension.ts:listTasks`). The JSON shape
is defined by `taskStatusJSON` in `internal/cli/task.go` and carries exactly the fields the
extension needs to decide launchability and working directory without re-reading `backlog.yaml`:

```json
{
  "id": "TASK-00042",
  "title": "add retry to uploader",
  "type": "feat",
  "status": "in_progress",
  "priority": "P1",
  "owner": "valter",
  "tags": ["upload"],
  "repo": "github.com/awslabs/mcp",
  "worktree_path": "/…/work/github.com/awslabs/mcp/TASK-00042-add-retry-to-uploader",
  "ticket_path": "/…/tickets/github.com/awslabs/mcp/TASK-00042-add-retry-to-uploader",
  "branch": "feat/add-retry-to-uploader"
}
```

From the tree (or the command palette) you can run per-task commands:

| Command                     | Palette title            | What it shells                        |
|-----------------------------|--------------------------|---------------------------------------|
| `adb.createTask`            | ADB: Create Task         | `adb task create <title> --type=<type> --priority=<priority>` |
| `adb.startTask`             | ADB: Start Task          | `adb task resume <id>`                |
| `adb.closeTask`             | ADB: Close Task          | `adb task update <id> --status=done`  |
| `adb.updateStatus`          | ADB: Update Task Status  | `adb task update <id> --status=<status>` |
| `adb.taskStatus`            | ADB: Task Status         | `adb task status` (to the Output panel) |
| `adb.showMetrics`           | ADB: Show Metrics        | `adb metrics`                         |
| `adb.showAlerts`            | ADB: Show Alerts         | `adb alerts`                          |
| `adb.refreshTickets`        | ADB: Refresh Tickets     | *(no CLI — just reloads the tree)*    |

> **Taxonomy gotcha — the Create Task palette is behind the CLI.** The type QuickPick
> (`extension.ts:505`) still offers only `["feat", "bug", "spike", "refactor"]`. That is *not* the
> full taxonomy: the CLI's canonical set is the 8 Conventional code types
> `feat, fix, refactor, docs, chore, test, perf, spike` **plus** the two non-code types
> `work` and `prototype` (D10)
> (`pkg/models/task.go` → `ValidTaskTypes`). It also still offers legacy `bug`, which the CLI now
> **rejects** at create time (``task type "bug" is retired; use `fix` instead``, from
> `validateTaskType` in `internal/cli/task.go`). To create a `docs`, `chore`, `test`, `perf`,
> `fix`, `work`, or `prototype` ticket, use the CLI directly:
>
> ```bash
> adb task create "wire up cloud sync docs" --type docs --priority P2 --no-launch
> ```

### Start All Tasks — org grouping + priority order

**ADB: Start All Tasks** (`adb.startAll`) is the headline command. It opens **one styled terminal
per launchable task**, grouped by org, and each terminal runs:

```bash
adb task resume <id> --here
```

The `--here` flag makes `adb task resume` exec Claude in place (no VS Code launch-file hand-off).
`composeTaskLaunchCommand` (`vscode-extension/src/launch.ts`) always emits `adb task resume <id>
--here` for every launchable task — it reads only `task.id`, and the CLI resolves the launch
directory itself (worktree → ticket dir → `ADB_HOME`).

**What counts as launchable and what order they open in** is pure decision logic in
`vscode-extension/src/launch.ts` and `vscode-extension/src/orggroup.ts`:

- **`planStartAll`** filters to *active* tasks (not `done`/`archived`) → *launchable*
  (`isLaunchable` = an existing worktree **or** a `repo` **or** a `ticket_path`) → *not already open*
  in a terminal → then applies the `cap`.
- **`planStartAllGrouped`** takes that flat "to-start" slice and groups + orders it by org.
- **`deriveOrg`** (`orggroup.ts`) reads the org from `ticket_path`
  (`tickets/<platform>/<org>/<repo>/…`, skipping an `_archived` layer, `_local` → the `LOCAL_ORG`
  bucket), falling back to the raw `repo` field.

When `groupByOrg` is on, each org gets a **divider terminal** named like `━━ awslabs (3) ━━`
(`orgHeaderName` in `vscode-extension/src/terminals.ts`) and that org's ticket terminals split
beneath it. `_local` sorts last.

> **Start All does NOT run `adb task start-all`.** It opens terminals per task, each running
> `adb task resume <id> --here`. The tickets are already promoted to `in_progress` by `resume`.
> The only bulk command it shells is **Close All** (below).

**ADB: Close All Tasks** (`adb.closeAll`) shells:

```bash
adb task close-all -y
```

which flips every *active* task (`in_progress`/`blocked`/`review`) to `done`. It does **not**
archive tickets or remove worktrees — it's reversible with `adb task update <id> --status …`.

**ADB: Close Terminals** (`adb.closeTerminals`) disposes every open `TASK-NNNNN` terminal (killing
the Claude inside), but changes *no* task status and makes *no* adb call. Org-divider terminals and
the dashboard are deliberately left alone — `isAnyTaskTerminal` only matches `/^TASK-\d+/`.

### The six Start-All / tmux config toggles

These are the knobs behind the WS-C / WS-D work (defaults from `vscode-extension/package.json`;
normalizers in `vscode-extension/src/config.ts`):

| Setting                        | Type       | Default | Meaning |
|--------------------------------|------------|---------|---------|
| `adb.startAll.groupByOrg`      | boolean    | `true`  | Group Start-All terminals under a per-org divider header (flat sorted list when off). |
| `adb.startAll.orderByPriority` | boolean    | `true`  | Order tasks P0→P3 within each group. Off = keep status order. |
| `adb.startAll.cap`             | number     | `0`     | Max task terminals per Start-All click. **`0` = unlimited**. |
| `adb.startAll.orgOrder`        | string[]   | `[]`    | Explicit org ordering. Empty = alphabetical with `_local` last. |
| `adb.tmux.enabled`             | boolean    | `true`  | Host each Claude session in a survivable tmux session. Off = bare (NON-durable) claude. |
| `adb.tmux.sessionPrefix`       | string     | `cc-`   | Prefix for the tmux session name (`<prefix><sanitized-dir-basename>`). |

Plus three general keys, for **9 total**: `adb.binaryPath` (`"adb"`), `adb.home` (`""` → falls back
to the first workspace folder), and `adb.dashboard.maxFeedItems` (`500`, min 1).

Example `settings.json` for a machine with a lot of orgs where you only want the top few tickets to
fan out:

```jsonc
{
  "adb.startAll.groupByOrg": true,
  "adb.startAll.orderByPriority": true,
  "adb.startAll.cap": 6,                 // never open more than 6 terminals per click
  "adb.startAll.orgOrder": ["awslabs", "aws-samples"],  // these two first, rest alpha, _local last
  "adb.tmux.enabled": true,
  "adb.tmux.sessionPrefix": "cc-"
}
```

### tmux-durable terminals (survive a reload)

Task terminals are **reload-survivable**. `openTaskTerminal` builds a terminal whose *process* is

```bash
<login-shell> -l -c 'adb task resume <id> --here; exec <shell> -l'
```

via `shellPath`/`shellArgs` (`composeShellArgs`, `resolveLoginShell` in
`vscode-extension/src/launch.ts`) — **not** `sendText`. That matters: when you reload the VS Code
window, VS Code re-runs `shellArgs`, and because `adb task resume --here` is idempotent (the CLI
does `tmux new-session -A -D` — attach-or-create on `cc-<basename>`), the revived tab reattaches to
the *same live Claude session* instead of duplicating it.

The extension threads the tmux config to the CLI through the environment (`adbEnv()` in
`extension.ts`):

- `ADB_TMUX` = `"1"` when `adb.tmux.enabled`, else `"0"` — this is the gate the CLI reads.
- `ADB_TMUX_PREFIX` = the sanitized `sessionPrefix` (`[A-Za-z0-9_-]` on both the TS and Go sides).
- `ADB_HOME` = `adb.home` or the first workspace folder.

> **Durability has host prerequisites the extension can't enforce:**
> - `tmux` must be on PATH (absent → it falls back to a plain, non-durable launch).
> - VS Code's `terminal.integrated.enablePersistentSessions` must be on (and
>   `persistentSessionReviveProcess` set to `onExitAndWindowClose` to survive a full window reload).
> - Setting `adb.tmux.enabled: false` isn't cosmetic — it sets `ADB_TMUX=0` so the CLI launches a
>   bare claude that **dies on reload**. It's a durability off-switch.

### Ad-hoc Claude sessions (cmd/ctrl+shift+`)

For a scratch Claude that isn't tied to a ticket, press **cmd+shift+`** (macOS) / **ctrl+shift+`**
(`adb.newClaude`, declared in `package.json` `contributes.keybindings`). It launches a fresh ad-hoc
session — and `nextAdhocIndex` (`vscode-extension/src/launch.ts`) picks the lowest free integer from
the open/revived terminal names, so each press is independent (and, on POSIX, reload-reattachable).
The exact command is **platform-specific** (`composeAdhocCommand`, `vscode-extension/src/launch.ts`),
mirroring how `resolveLoginShell`/`composeShellArgs` branch on the platform:

```bash
# POSIX (host: $SHELL -l): a survivable cc-adhoc-<n> tmux session
cc-survive adhoc-<n>

# Windows (host: powershell.exe -NoExit): a bare, non-survivable session
claude --dangerously-skip-permissions
```

On **POSIX** this needs `cc-survive` on PATH and yields a reload-reattachable `cc-adhoc-<n>` session.
On **Windows** `cc-survive`/tmux is unavailable (MSYS tmux can't give claude a console pty), so the
press runs a bare `claude` in the PowerShell host — a **working but non-survivable** session (a
reload starts fresh), the same degradation task terminals take on Windows. This is the ad-hoc half of
the Windows launch fix started by #227 for task/Start-All terminals; before it, `ctrl+shift+`` ran
`cc-survive` in PowerShell and just left a `command not found` dead tab.

Two independent tmux naming schemes coexist on POSIX: **task** terminals attach `cc-<basename>`
(basename of the worktree/ticket/`ADB_HOME` dir), while **ad-hoc** sessions attach `cc-adhoc-<n>`.

---

## The in-editor dashboard

**ADB: Open Dashboard** (`adb.openDashboard`) opens an in-editor **VS Code WebviewPanel** — there
is no browser, no localhost, and no server process. It's a singleton (revealed on repeat click),
with `retainContextWhenHidden`, `enableScripts`, a nonce'd inline script, and a strict CSP
(`default-src 'none'`). The HTML scaffold is `renderPanelHtml` in
`vscode-extension/src/webview/panel.ts`.

> This is **not** the old web dashboard. The web UI and its server were removed; the surviving
> terminal dashboard is the separate `adb dashboard` (a Bubbletea TUI), and the only "serve" verb in
> adb is `adb mcp serve` (MCP over stdio — see [L300](./L300-integrations.md)).

### The event feed

The dashboard's feed pulls from `adb`'s event log. Source selection (`pickFeedSource` in
`extension.ts`):

1. Probe `adb events --help` (3s timeout).
2. **On success** → `commandFeedSource` spawns

   ```bash
   adb events tail --follow --json
   ```

   and forwards each stdout line into the webview (this is JSONL — one JSON object per line).
3. **Otherwise** → `fileTailFeedSource` polls `<ADB_HOME>/.events.jsonl` every 750 ms from the
   current file size, tailing only *new* events (handles truncation/rotation).

Each line is turned into a feed item by `eventToFeedItem` + `SUMMARY_BY_TYPE`
(`vscode-extension/src/webview/feed.ts`), which recognizes the observability event schema —
including the issue-sync events (`issue.synced` with direction, `issue.conflict`, `issue.skipped`)
alongside `task.*`, `worktree.*`, `agent.*`, and `knowledge.extracted`. Unknown types still render
via a generic fallback, so nothing is dropped. `adb.dashboard.maxFeedItems` (default 500) caps how
many items the feed keeps.

### The org overview

`buildOverview` (`vscode-extension/src/webview/overview.ts`) aggregates per-org status counts
(reusing the same `deriveOrg` as Start-All) into `OverviewCard`s — a glance at how many tickets sit
in each status, per org.

### The chat-steer box

The dashboard has a chat box that can propose — and, after your explicit confirmation, execute — a
small allowlist of mutations. This is a deliberately **narrow security boundary**, so it's worth
understanding the flow:

```
you type → adb chat --message <text> → reply (may contain ```adb-action fenced JSON) →
  parsed into proposals → you click a proposal → MODAL "Apply?" → execFile / append
```

- The only chat subprocess is:

  ```bash
  adb chat --message <text>
  ```

  (`-m`/`--message` is required and non-empty; run via `execFile` with an argv array, never a
  shell). `adb chat` is a *pure LLM adapter* — it builds a system prompt from the live task list +
  metrics, shells out to `claude -p`, and prints the reply. **Go never executes any mutation.**
- The reply may contain fenced ` ```adb-action ` JSON blocks. `parseSteerActions`
  (`vscode-extension/src/webview/chat.ts`) parses them; the raw actions stay extension-side and the
  webview only receives `id + summary + executable`.
- **The allowlist is reject-by-default** (`vscode-extension/src/webview/actions.ts`):
  - `task.update` — only to one of the 6 canonical statuses, task ID must match `/^TASK-[0-9]+$/`.
  - `notes.append` — capped at 4096 bytes, can only target `<ticketDir>/notes.md`, guarded by a
    realpath'd `isPathInside` check (defeats `..`/symlink escapes).
  - `wiki.capture` — never lowered to argv; it surfaces as a `/wiki` hint only.
  - Any unknown verb → rejected.
- `applyProposal` re-runs the allowlist (belt-and-braces) and then requires an explicit **modal
  "Apply" click** before any `execFile`/`fs.appendFileSync`.

So the chat box can move a ticket's status or append a note — but it is not a free-form agent
controller, and every real mutation is one modal click away from you.

---

## Watching the event stream from the CLI

You don't need the dashboard to watch what adb is doing. `adb events` reads the append-only JSONL
at `<ADB_HOME>/.events.jsonl` (`internal/cli/events.go`).

**One-shot query with filters** (`--json` emits a single indented JSON *array*):

```bash
# everything in the last 24h
adb events query --since 24h

# just task lifecycle for one ticket, as JSON
adb events query --task TASK-00042 --json

# only the issue-sync events over the past week
adb events query --type issue.synced --since 7d
```

Flags: `--type`, `--task` (filters `data.task_id`), `--since`, `--json`.

**Live tail** (`--json` here emits *JSONL* — one object per line, the shape the webview consumes):

```bash
# one-shot dump of the current log
adb events tail

# stream new events as they land
adb events tail --follow

# stream as JSONL (what the dashboard pipes)
adb events tail --follow --json
```

Flags: `-f`/`--follow`, `--json`. The follow loop polls every ~500 ms and writes its
`streaming events…` notice to **stderr**, keeping stdout clean JSONL.

> Two `--json` shapes on purpose: `events query --json` = one array; `events tail --json` = JSONL.
> Don't feed one to a parser expecting the other.
>
> `--since` uses a small custom parser (`parseDuration`, `internal/cli/metrics.go`): a trailing `d`
> means **days** (`7d` = 168h), then it falls back to `time.ParseDuration` for `h`/`m`/`s`. Standard
> Go duration parsing has no `d`, so `7d` only works because of this special-case (shared with
> `adb metrics --since`).

---

## Backlog maintenance

Two idempotent commands keep `backlog.yaml` — the sole authoritative type/title store — clean. Both
**default to dry-run** and require `--apply` to write.

### migrate-types (bug → fix)

The taxonomy moved to Conventional types; `bug` is retired. If you have pre-migration backlog
entries typed `bug`, rewrite them to `fix`:

```bash
# preview — prints how many would change, writes nothing
adb task migrate-types

#   Dry run: N task type(s) would change. Pass --apply to rewrite backlog.yaml.

# commit the rewrite
adb task migrate-types --apply
```

Defined in `internal/cli/task_migrate_types.go`. It's idempotent — a second `--apply` is a no-op.

### normalize-titles (strip doubled prefixes)

Strips a duplicated `[type]` prefix that older tooling sometimes baked into stored titles (e.g.
`[feat] [feat] add retry` → `add retry`):

```bash
# preview
adb task normalize-titles

#   Dry run: N title(s) would change. Pass --apply to rewrite backlog.yaml.

# commit
adb task normalize-titles --apply
```

Defined in `internal/cli/task_normalize.go`. Also idempotent, also touches only `backlog.yaml`.

---

## Real command sequences

### Morning: fan out and start working

```bash
# 1. Sanity-check the backlog (from the workspace root, or set ADB_HOME)
adb task status --filter backlog

# 2. In VS Code: run "ADB: Start All Tasks" from the palette or the tickets view.
#    → each launchable ticket opens as a tmux-hosted terminal running:
#         adb task resume <id> --here
#    → grouped under "━━ <org> (n) ━━" dividers, P0→P3 within each org.

# 3. Keep an eye on the stream in a scratch terminal:
adb events tail --follow
```

### Create a ticket the palette can't (a `docs` or `chore` type)

```bash
# the extension QuickPick only offers feat/bug/spike/refactor — use the CLI for the rest.
# --no-launch keeps it out of an interactive Claude launch (safe for scripting).
adb task create "document the daily workflow tier" \
  --type docs --priority P2 --repo github.com/valter-silva-au/ai-dev-brain --no-launch

# then refresh the tickets tree in VS Code (ADB: Refresh Tickets) and Start it.
```

### Steer an in-flight ticket from the dashboard chat

```text
1. ADB: Open Dashboard
2. In the chat box: "Mark TASK-00042 as review and note that CI is green."
   → adb chat --message "…" runs, the reply proposes:
        task.update TASK-00042 → review
        notes.append TASK-00042 → "CI is green"
3. Click each proposal → confirm the modal "Apply".
   → the extension shells `adb task update TASK-00042 --status=review`
     and appends to that ticket's notes.md.
```

### Reconcile a drifted backlog after pulling old ticket data

```bash
# preview both maintenance passes
adb task migrate-types
adb task normalize-titles

# apply them
adb task migrate-types --apply
adb task normalize-titles --apply

# verify
adb task status --json | jq '.[] | {id, type, title}'
```

### End of day

```bash
# In VS Code: "ADB: Close All Tasks"  →  adb task close-all -y
#   (flips active → done; does NOT archive or remove worktrees — fully reversible)

# Optional: dispose the terminals without touching status
#   "ADB: Close Terminals"  →  no adb call, just kills the TASK-NNNNN tabs
```

---

## Where to go next

- **New to adb?** Start at [L100 — Fundamentals](./L100-fundamentals.md) for the task model, type
  taxonomy, branch naming, and the core `adb task` lifecycle.
- **Connecting adb to the outside world?** [L300 — Integrations](./L300-integrations.md) covers
  issue sync (`adb sync issues`), cloud sync (`adb sync cloud`), and the MCP server (`adb mcp
  serve`).
- **Want the "why"?** [L400 — Architecture & Extending](./L400-architecture-and-extending.md) covers
  the nested `tickets/<platform>/<org>/<repo>` correlation layout, the `TaskStatus`/`TaskType`
  models, the observability event schema, and how to add a command, event type, or config key.

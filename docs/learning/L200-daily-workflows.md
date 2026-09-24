# L200 — Daily Workflows

> **Tier goal:** *"I use adb fluently."* You already know the basics (see
> [L100 — Fundamentals](./L100-fundamentals.md)). This tier is the muscle-memory layer:
> running many tickets in parallel in durable terminal sessions, watching the event stream,
> and keeping your backlog tidy. When you want to know *why* the pieces fit together the way
> they do, jump to [L400 — Architecture & Extending](./L400-architecture-and-extending.md).

adb is **terminal-native**. It launches coding-agent sessions, manages worktrees, and prints
to stdout; it is not tied to an editor and does not run a UI. (A VS Code extension shipped
until 2026-09-14 and was removed — see [What was removed](#what-was-removed).)

Every command and flag below is taken directly from the code — no invented surface.

---

## Table of contents

- [The daily loop at a glance](#the-daily-loop-at-a-glance)
- [Running many tickets at once](#running-many-tickets-at-once)
- [Durable sessions: tmux hosting](#durable-sessions-tmux-hosting)
- [Watching the event stream from the CLI](#watching-the-event-stream-from-the-cli)
- [Alerts and their thresholds](#alerts-and-their-thresholds)
- [Backlog maintenance](#backlog-maintenance)
- [Real command sequences](#real-command-sequences)
- [What was removed](#what-was-removed)
- [Where to go next](#where-to-go-next)

---

## The daily loop at a glance

A fluent day with adb looks like this:

1. `cd` to your workspace (the folder holding `backlog.yaml` — your `ADB_HOME`).
2. `adb task list --filter backlog` to see what is waiting.
3. `adb task start <id>` (fresh session) or `adb task resume <id>` (continue the last one) per
   ticket you intend to work. Each launches your configured agent in that ticket's worktree.
4. Tail `adb events --follow` in a scratch pane, and `adb task list --git` for the cross-repo
   view.
5. When a ticket lands: `adb task close <id>`, then `adb task archive <id>`.

Nothing here needs an editor open, and every step is a command you can put in a script.

---

## Running many tickets at once

Each repo-backed task has its own git worktree, so parallel work cannot collide (L100 §5).
Running several at once is just running several sessions.

**One ticket per terminal pane.** Whatever multiplexer you use, the unit is:

```bash
adb task start TASK-00042      # fresh session in that ticket's worktree
adb task resume TASK-00042     # or continue where you left off
```

**Fan out from the shell.** `start --no-launch` promotes without opening a session, which is
what you want when you are queueing work rather than doing it (note `resume` has no such
flag — it always launches):

```bash
# promote every backlog ticket, launching nothing
adb task list --json --filter backlog \
  | jq -r '.[].id' \
  | xargs -n1 -I{} adb task start {} --no-launch

# then open the two you actually intend to work on
adb task start TASK-00042
adb task start TASK-00043
```

**Know where each one lives.** `adb task worktree list` prints the worktree behind every
repo-backed task, and `adb task worktree switch <id>` prints a path you can `cd` to:

```bash
cd "$(adb task worktree switch TASK-00042)"
```

**See the whole fleet.** `adb task list --git` joins `backlog.yaml` with live per-worktree git
state — branch, dirty/clean, ahead/behind, and whether the worktree still exists — over the
same filterable task list you use for everything else:

```bash
adb task list --git                        # table
adb task list --git --json                 # for a script — an array, same rows plus git keys
adb task list --git --filter in_progress   # only what you are actually working
```

This used to be a separate top-level `adb status`. It is a flag now because it was never a
different question: "which of my tickets is dirty or behind?" is the ticket list with more
columns, and having it live in two places meant `--filter` worked on one of them and not the
other. The alias still runs (`adb status` → `adb task list --git`), but see the note below
before you point a script at it.

> **`--git` no longer drops rows, and the `--json` shape changed.** The old `adb status`
> path silently omitted archived tasks and tasks with no worktree; `--git` now enriches
> every row it can and leaves the git keys at zero values for the rest, because a *flag*
> should not change which tasks appear. And `adb status --json` used to emit
> `{"tickets": …, "orphaned_worktrees": …}` — it now emits the plain array, like every other
> `adb task list --json`. **Orphaned worktrees moved** to `adb task worktree list --json`,
> whose `{"worktrees": [...], "orphaned": [...]}` is the only task listing with an envelope,
> because an orphan has no task row to be a key on. If a script of yours read
> `.orphaned_worktrees`, that is where to point it.

**Reclaim what is no longer in use.** Worktrees outlive the tickets that made them, so the
sweep is its own verb — and it *previews* by default:

```bash
adb task worktree prune            # what would be removed
adb task worktree prune --apply    # remove it
adb task worktree reconcile        # what is recorded in backlog.yaml but missing on disk
adb task worktree reconcile --apply
```

The rule, which holds across both worktree nouns: **a mutation that names its target acts
immediately; one that sweeps a set previews unless `--apply`.** So
`adb task worktree remove TASK-00042` removes on sight — you said which one — while `prune`
and `reconcile` show you the set first. Bare `adb work prune` used to remove; the alias now
previews, which is a behaviour change in the fail-safe direction.

---

## Durable sessions: tmux hosting

A long agent session should survive the terminal that started it. adb hosts each launched
session in a **tmux** session named deterministically from the launch directory, so
reattaching is idempotent — running `adb task resume` again attaches to the live session
instead of starting a second one.

| Environment variable | Effect |
|---|---|
| `ADB_TMUX=0` | Disable tmux hosting. The agent runs bare and **dies with the terminal**. |
| `ADB_TMUX_PREFIX` | Override the session-name prefix (default `cc-`, and `pi-` for pi). |

Prerequisite: **`tmux` on `PATH`**. Absent, adb falls back to a plain, non-durable launch —
it does not fail, so a missing tmux shows up only as a session that does not survive.

Because the session name derives from the directory, a task session and a shell you attach by
hand converge on one session per worktree rather than competing:

```bash
tmux ls                      # what is alive
tmux attach -t cc-TASK-00042-add-retry   # attach by hand
```

---

## Watching the event stream from the CLI

You don't need the dashboard to watch what adb is doing. `adb events` reads the append-only JSONL
at `<ADB_HOME>/.adb/events.jsonl` (`internal/cli/events.go`).

> **Upgrading from an older workspace?** adb's per-workspace state files moved out of the workspace
> root into `.adb/` in #186 (`internal/statedir` — 12 of them today), losing their leading dot /
> `.adb_` prefix — so
> the event log is `.adb/events.jsonl`, not `.events.jsonl`. You don't have to move anything: a
> one-shot, idempotent migration (`internal/migration.go`) relocates each legacy root-level file
> the first time any `adb` command builds its `App`, and it never overwrites a target that already
> exists.

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

**One ticket's history** is the same filter with a shorter name, and it is the one you
actually reach for when writing a handoff or a PR description:

```bash
adb task timeline TASK-00042             # oldest first: created, promoted, worktree, sessions
adb task timeline TASK-00042 --since 7d
adb task timeline TASK-00042 --json      # identical shape to `adb events query --json`
```

It is a **view**, not a second store — the same `data.task_id` filter over
`.adb/events.jsonl` — so the honest limit is that a ticket older than the event log has an
empty timeline rather than a reconstructed one.

**Live tail** (`--json` here emits *JSONL* — one object per line):

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
> `--since` uses a small custom parser: a trailing `d` means **days** (`7d` = 168h), then it falls
> back to `time.ParseDuration` for `h`/`m`/`s`. Standard Go duration parsing has no `d`, so `7d`
> only works because of this special case. The canonical implementation is
> `observability.ParseDuration`; `internal/cli`'s `parseDuration` delegates to it, so the rule is
> the same one `adb metrics --since`, `adb task timeline --since` and the alert thresholds read.
>
> **A partial day count is now rejected rather than truncated.** `1.5d` used to return **24h and
> no error** — the parser used `fmt.Sscanf("%d")`, which reads the leading integer and stops
> without requiring the rest, so `1 2d`, `1abcd` and `1e3d` all silently meant one day too. It
> uses `strconv.Atoi` now, which refuses trailing text. If you want 36 hours, write `36h`.

---

## Alerts and their thresholds

`adb alerts` evaluates four conditions over the metrics derived from the event log. They are the
"what needs attention" half of `adb metrics`:

| Condition | Config key | Default | Severity |
|---|---|---|---|
| `task_blocked_too_long` | `alert_task_blocked_too_long` | 24 hours in `blocked` | High |
| `task_stale` | `alert_task_stale` | 3 days in `in_progress` | Medium |
| `review_too_long` | `alert_review_too_long` | 5 days in `review` | Medium |
| `backlog_too_large` | `alert_backlog_too_large` | 10 tasks in `backlog` | Low |

```bash
adb alerts              # active alerts
adb metrics --since 7d  # the metrics they are evaluated against
```

### Setting a threshold

Each threshold is one flat `custom_settings` key — `alert_` plus the condition's own name — so it
follows the standard three-tier precedence: **`.taskrc` (repo) > `orgs/<id>/config.yaml` (org) >
`~/.taskconfig` (global) > the defaults above.**

```yaml
# .taskrc — a repo where review really does take a week and the backlog is meant to be deep
custom_settings:
  alert_task_blocked_too_long: 48h
  alert_task_stale: 7d
  alert_review_too_long: 7d
  alert_backlog_too_large: 40
```

```bash
adb config get alert_task_stale --source   # → 7d  [repo]
adb alerts                                 # evaluated against the resolved thresholds
```

Four things worth knowing before you write one:

- **Three of them take a duration, one takes a count.** The durations use the same spelling
  `--since` does — a trailing `d` means **days** (`7d` = 168h), otherwise it is Go's own syntax
  (`48h`, `90m`, `1h30m`). `alert_backlog_too_large` is a whole number of tasks. Both `40` and
  `"40"` work; quoting is not required.
- **Per-key precedence, not per-block.** A repo `.taskrc` can lower one threshold while the other
  three keep resolving from the org or global tier. That is the reason there are four keys rather
  than one packed value.
- **Underscores, never dots.** `alert.task_stale` is a different (and worse) key: Viper reads `.`
  as a nesting delimiter — see [L600 §11](./L600-document-programs.md#11-external-packs) for the
  full story. Write `alert_task_stale`.
- **A bad value costs you that one threshold and nothing else.** An unparseable duration, a
  non-positive duration, a non-numeric or negative count — each is skipped, the default is kept,
  and a `Warning:` line naming the key and the tier goes to **stderr**. Config resolves at startup,
  so a fatal error here would kill every command in the workspace; instead stdout stays clean and
  `--json` keeps parsing:

  ```console
  $ adb alerts 2>/dev/null
  ✓ No active alerts
  $ adb alerts 2>&1 >/dev/null
  Warning: ignoring alert threshold "alert_task_blocked_too_long" from the repo tier: "3 weeks" is not a duration (want e.g. 24h, 3d, 90m); keeping the default
  ```

**Severities are not configurable.** They are a fixed triage label per condition (the table above),
not a threshold — the question a config answers here is *when does this fire?*

> The thresholds were genuinely unreachable until TASK-00039: `internal/app.go` built the evaluator
> with a nil config, so `AlertConfig.SetThreshold` sat there with no caller and editing config
> changed nothing. If you find a doc that still says so, it predates this.

---

## Backlog maintenance

**The maintenance commands are gone.** `adb task migrate-types`,
`adb task normalize-titles`, and `adb task normalize` were removed in TASK-00039. Each
existed to repair workspaces that predated a schema change — a one-shot fix that had
become permanent public surface.

`backlog.yaml` is the sole authoritative type/title store, so the repairs they performed
are now direct edits to that file:

| What it did | Now |
|---|---|
| `migrate-types` rewrote legacy `bug` rows to `fix` | edit the `type:` field. `bug` is rejected at create and still maps to a `fix/` branch, so an unmigrated row keeps working |
| `normalize-titles` stripped a doubled `[type]` prefix from stored titles | edit the `title:` field. The `[type]` prefix you see in `adb task list` is added **at render time**, so a stored title should not contain one |
| `normalize --dedup` / `--frontmatter` repaired ticket files | `adb task validate <id>` still *reports* this drift; fixing it is a manual edit |

`adb task validate` and the hidden `adb task migrate-blocked-by` (the
`blocked_by`→`depends_on` graph migration) both survive.

---

## Real command sequences

### Morning: see what is waiting, open what you'll work

```bash
# 1. Sanity-check the backlog (from the workspace root, or set ADB_HOME)
adb task list --filter backlog

# 2. Cross-repo state: which worktrees are dirty, behind, or missing
adb task list --git

# 3. Open the two you actually intend to work, each in its own pane
adb task start TASK-00042      # fresh session
adb task resume TASK-00043     # continue yesterday's

# 4. Keep the stream in a scratch pane
adb events tail --follow
```

### Queue a batch without opening any sessions

```bash
# --no-launch promotes only — safe in scripts and CI, and honoured via ADB_NO_LAUNCH=1
adb task create "document the daily workflow tier" \
  --type docs --priority P2 --repo github.com/valter-silva-au/ai-dev-brain --no-launch

adb task list --json --filter backlog | jq -r '.[].id' \
  | xargs -n1 -I{} adb task start {} --no-launch
```


### End of day

```bash
# Close what landed (reversible — does NOT archive or remove worktrees)
adb task close TASK-00042

# Refresh the instruction files so tomorrow's session starts current
adb context build       # regenerate AGENTS.md + its harness pointers

# Retire anything merged
adb task archive TASK-00042
```

---

## What was removed

A VS Code extension (`vscode-extension/`, `adb-brain`) shipped until **2026-09-14**. It provided a
tickets tree, a Start-All fan-out, an in-editor dashboard webview with an event feed and a
chat-steer box, and styled terminal tabs.

It was removed because adb is a terminal and AI-agent tool, not an IDE plug-in — the same reason
`cli-surface-review.md` excludes dashboard, TUI, web and IDE surfaces from the core. Removed with
it: the `~/.adb_terminal_launch.json` hand-off, the terminal-state file, and `--here` (which
existed only to bypass that hand-off, so launching in place is now unconditional).

Nothing it did is lost, only relocated to commands you can script:

| Extension feature | Now |
|---|---|
| Tickets tree | `adb task list`, `adb task list --git` |
| Start All Tasks | `adb task start <id>` per ticket, or the `xargs` loop above |
| Close All Tasks | `adb task close <id>`, in the same loop |
| Styled terminal tabs | your multiplexer; adb still sets the tab title |
| Durable sessions | unchanged — tmux hosting lives in the CLI, not the extension |
| Dashboard event feed | `adb events tail --follow` |
| Dashboard org overview | `adb task list --git`, `adb metrics`, `adb alerts` |
| Chat-steer box | removed with `adb chat` |

---

## Where to go next

- **New to adb?** Start at [L100 — Fundamentals](./L100-fundamentals.md) for the task model, type
  taxonomy, branch naming, and the core `adb task` lifecycle.
- **Connecting adb to the outside world?** [L300 — Integrations](./L300-integrations.md) covers
  issue sync (`adb issues sync`), cloud sync (`adb archive`), and the MCP server (`adb mcp
  serve`).
- **Want the "why"?** [L400 — Architecture & Extending](./L400-architecture-and-extending.md) covers
  the nested `tickets/<platform>/<org>/<repo>` correlation layout, the `TaskStatus`/`TaskType`
  models, the observability event schema, and how to add a command, event type, or config key.

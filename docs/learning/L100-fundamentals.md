# L100 — Fundamentals

> **Learning tier:** L100 (foundations) · **Goal after this page:** *"I can manage tasks."*
>
> By the end you will understand how `adb` models a unit of work — its **status
> lifecycle**, its **type taxonomy**, how a type becomes a **branch name**, where the
> **ticket and worktree live on disk**, and the handful of `adb task` commands that
> carry a ticket from idea to done. Everything here is verified against the current
> `main`.

**Next tiers:**

- [L200 — Daily Workflows](./L200-daily-workflows.md) — running many tickets in parallel, durable sessions, the event stream, metrics.
- [L300 — Integrations](./L300-integrations.md) — cloud archive, the scheduler, memory, MCP, and hooks.

---

## 1. What is a task?

A **task** (interchangeably a *ticket*) is the atom of work in `adb`. It is a single
YAML record in `backlog.yaml` (the sole authoritative store) plus a **ticket
directory** on disk that holds its context, notes, design, and knowledge.

The data model lives in
[`pkg/models/task.go`](../../pkg/models/task.go) — `type Task struct`. The fields you
touch daily:

| Field | Meaning |
|-------|---------|
| `ID` | Globally-unique key, `TASK-00001` (5-digit, zero-padded) |
| `Title` | Free-form title (also the slug source) |
| `Type` | One of the 10 task types — 8 Conventional code + 2 non-code (see §3) |
| `Status` | One of the 6 lifecycle states (see §2) |
| `Priority` | `P0`–`P3` (default `P2`) |
| `Repo` | Platform-qualified repo, e.g. `github.com/org/repo` (optional) |
| `Slug` | Kebab-case slug derived from the title |
| `Branch` | Derived Conventional branch, e.g. `feat/my-slug` (see §4) |
| `WorktreePath` / `TicketPath` | Where the worktree and ticket dir live (see §5) |

A new task is built by `NewTask(id, title, taskType)`
([`pkg/models/task.go`](../../pkg/models/task.go) — `func NewTask`), which always sets
`Status = backlog`, `Priority = P2`, and stamps `Created`/`Updated` in UTC.

---

## 2. The status lifecycle

Every task moves through a small state machine. The six states are declared as
`TaskStatus` constants in [`pkg/models/task.go`](../../pkg/models/task.go):

```
backlog → in_progress → review → done → archived
              ↕
           blocked
```

| Status | Meaning | Set by |
|--------|---------|--------|
| `backlog` | Created but not started (the default on create) | `adb task create` |
| `in_progress` | Actively being worked | `adb task resume`, `adb task start` |
| `blocked` | Waiting on a dependency | `adb task update --status blocked` |
| `review` | Work done, under review | `adb task update --status review` |
| `done` | Complete | `adb task close`, or `adb task update --status done` |
| `archived` | Retired; ticket moved to `tickets/_archived/`, worktree removed | `adb task archive` — **only**; see below |

Two helpers on the model classify these states
([`pkg/models/task.go`](../../pkg/models/task.go)):

- `Task.IsActive()` → `true` for `in_progress`, `review`, or `blocked`.
- `Task.IsBlocked()` → `true` for `blocked` **or** any non-empty `BlockedBy` list.

### Verified transitions

These are the transitions the code actually performs (not everything is a free-for-all):

- **create** → always lands at `backlog` (`NewTask`).
- **`adb task resume`** / **`adb task start`** → flip `backlog` → `in_progress` (`resume`
  only when currently `backlog`, and it refuses an already-archived task; `start` is
  idempotent — a task that is not in `backlog` keeps its current status).
- **`adb task close`** → sets `done`. It is exactly `UpdateStatus(id, done)` —
  `core.TaskManager.Close`, the same method MCP's `adb_task_close` calls — so it adds no
  storage and no new event type (`task.status_changed` already covers it). It changes
  *only* the status: the ticket dir, worktree, and branch are untouched, and
  `adb task update <id> --status in_progress` reverses it.
- **`adb task update --status`** → the escape hatch, for **five** of the six states.
- **`adb task archive`** → moves any task to `archived`, relocates the ticket dir under
  `tickets/_archived/` (preserving its nested sub-path), and removes the worktree.
- **`adb task update --status backlog`** on an *archived* task → un-archives it: moves the
  ticket dir back out of `_archived/`, prunes the nesting it was the last occupant of, and
  returns the status to `backlog`.

### `archived` is a move, not a field

The sixth state is the one exception to "`update --status` sets any of them", and the
reason is worth knowing because it used to be a bug you could hit by accident.

`archived` is not a label on a row — it is a *relocation*: the ticket directory moves under
`tickets/_archived/` and the worktree is removed. `TaskManager.Archive`/`Unarchive` perform
that move; `UpdateStatus` only rewrites the field. So `adb task update <id> --status
archived` used to produce a workspace that *claimed* a task was archived while its ticket
directory and worktree were both still sitting there live — and the mirror image,
`--status backlog` on an archived task, left the directory in `_archived/` wearing a
`backlog` status.

Both halves are now closed, in the direction each deserves:

| You type | What happens |
|---|---|
| `adb task update <id> --status archived` | **rejected**, naming `adb task archive` — which is where `--force`, `--keep-worktree`, and `--prune-branch` live |
| `adb task update <id> --status backlog` on an archived task | routed through `TaskManager.Unarchive`, so the ticket dir genuinely comes back |

Un-archiving also **prunes the empty nesting the ticket vacated** — a repo-backed ticket
archives to `tickets/_archived/github.com/acme/thing/TASK-…`, and moving it back used to
leave those three directories behind, so `_archived/` accumulated a tree that read as
"there are archived tickets here". Only genuinely empty directories go, only up as far as
`_archived/` (which is a boundary, never removed), and the walk stops at the first
directory that still has an entry — a nesting shared with another archived ticket survives
with that ticket intact.

The rejection is what makes the second one honest: with archiving reachable only through
the verb that performs the move, `update --status` can be trusted to mean exactly "rewrite
this field" for every value it accepts.

---

## 3. The task-type taxonomy (8 code types + 2 non-code types)

The accepted-at-create set is `ValidTaskTypes` in
[`pkg/models/task.go`](../../pkg/models/task.go) — the **eight** Conventional
([Conventional Commits](https://www.conventionalcommits.org)) **code** types plus **two**
**non-code** types (`work`, `prototype`, added in D10):

| `--type` | Use it for | Branch prefix (see §4) |
|----------|------------|------------------------|
| `feat` | A new feature / capability (the **default**) | `feat/` |
| `fix` | A bug fix | `fix/` |
| `refactor` | Restructuring with no behaviour change | `refactor/` |
| `docs` | Documentation-only change | `docs/` |
| `chore` | Tooling, deps, housekeeping | `chore/` |
| `test` | Adding or fixing tests only | `test/` |
| `perf` | A performance improvement | `perf/` |
| `spike` | Time-boxed investigation / research | **`chore/`** (mapped, see §4) |
| `work` | A **non-code** artifact/graph deliverable (a doc, a decision, research) | **none** — no worktree, no branch |
| `prototype` | A **non-code**, time-boxed experiment (a.k.a. validation-spike) | **`chore/`** (mapped, like `spike`) |

Pick the type that matches what the work *ships*, not what it feels like — a "feature"
ticket that ends up shipping only docs should be `docs`.

> **Non-code types (`work`, `prototype`).** `work` models a deliverable that isn't code, so
> `adb task create --type work` builds **no git worktree and no branch** even when `--repo` is
> given (the repo, if any, only nests the ticket dir); with no code checkout it is exempt from
> code gates by construction. `prototype` is code-shaped — a time-boxed experiment that still
> gets a worktree and a `chore/`-prefixed branch. Both are **stage-agnostic**: the founder-
> playbook StageGate enforces stage discipline, the type never does.

> ⚠️ **`bug` is not a valid type.** It is a *retired legacy alias*. Running
> `adb task create --type bug` is **rejected**:
>
> ```
> task type "bug" is retired; use `fix` instead
> ```
>
> (see `validateTaskType` in [`internal/cli/task.go`](../../internal/cli/task.go)).
> Old `backlog.yaml` entries that still say `bug` keep working — `ConventionalType`
> still maps them to a `fix/` branch — but there is no longer a migration command
> (`adb task migrate-types` was removed in TASK-00039). `backlog.yaml` is the sole
> authoritative type store, so retyping such a row is a one-line edit there.

---

## 4. From type to branch name

`adb` never uses a `task/<id>` branch. It derives a **Conventional branch** from the
type and slug via `BranchName(taskType, slug, id)` in
[`pkg/models/task.go`](../../pkg/models/task.go):

```
BranchName = ConventionalType(type) + "/" + slug
```

- **`Slugify`** ([`pkg/models/task.go`](../../pkg/models/task.go)) lowercases the title,
  turns every run of non-`[a-z0-9-]` characters into a single dash, and trims dashes.
  So `"Fix ECS datetime crash"` → `fix-ecs-datetime-crash`. If the slug comes out empty,
  the lowercased task ID is used as a fallback.
- **`ConventionalType`** ([`pkg/models/task.go`](../../pkg/models/task.go)) maps the type
  to its branch prefix. Seven types map **1:1** to themselves; there are exactly **two
  non-identity mappings** mandated by the correlation-layout ADR:

  | Type | Prefix |
  |------|--------|
  | `spike` | **`chore`** |
  | `bug` (legacy) | **`fix`** |

  Unknown types fall through to the raw string so a future type never breaks branch
  creation.

**Worked example:** a `spike` task titled *"Insurability probe G0"* →
slug `insurability-probe-g0` → branch **`chore/insurability-probe-g0`** (not
`spike/…`).

> **Heads-up on `adb task create <branch>`:** the positional argument is a *title / slug
> source*, **not** a ready-made branch name. Passing `feat/my-thing` would get *slugified
> into the slug* (`feat-my-thing`) rather than used verbatim. Just pass a title; let
> `adb` derive the branch.

---

## 5. Where a ticket lives on disk (the correlation layout)

The **path is the correlation**: platform → org → repo → which task → what it does,
readable without opening `backlog.yaml`. The rule is `resolveTaskDir` in
[`internal/core/bootstrap.go`](../../internal/core/bootstrap.go), with four cases:

| # | Inputs | Ticket path |
|---|--------|-------------|
| 1 | `--repo` **and** slug | `tickets/<platform>/<org>/<repo>/TASK-id-slug/` |
| 2 | `--repo` only (no slug) | `tickets/<platform>/<org>/<repo>/TASK-id/` |
| 3 | slug only (repo-less) | `tickets/_local/TASK-id-slug/` |
| 4 | neither | `tickets/TASK-id/` (legacy flat) |

The **worktree path mirrors the ticket path** — a repo-backed task gets
`work/<platform>/<org>/<repo>/TASK-id-slug/` as its git worktree. In the live create
path, a worktree is created **only when `--repo` is set**; a repo-less task gets a ticket
under `_local/` but no worktree.

### Reading a ticket back by ID

When you have only a `TASK-id` (no `TicketPath`), use `ResolveTicketDir(ticketsDir, id)`
in [`internal/core/resolve.go`](../../internal/core/resolve.go). It walks `tickets/` at
any depth and matches a directory whose base name is exactly the id **or** starts with
`id-` (the trailing dash stops `TASK-1` from matching `TASK-10-foo`). A **live** match
always wins over an `_archived/` one; ties break to the shallowest, lexically-first path.

### What's inside a ticket directory

`BootstrapSystem` ([`internal/core/bootstrap.go`](../../internal/core/bootstrap.go))
scaffolds each ticket dir with:

```
tickets/<…>/TASK-00001-my-slug/
├── status.yaml            # machine-readable status/phase/progress
├── context.md             # the ticket's identity: description, requirements, acceptance
├── notes.md               # running scratchpad (chronological, informal)
├── design.md              # design notes
├── sessions/              # Claude Code session transcripts
└── knowledge/
    └── decisions.yaml     # seeded "decisions: []"
```

When a **worktree** exists, `adb` also writes the ticket's Tier-0 context **inside the
worktree** (not the ticket dir), so an agent started there gets it automatically. The
layout mirrors the workspace's canonical-plus-pointer shape:

```text
<worktree>/
├── .adb/task-context.md            ← the context itself, agent-agnostic
├── .claude/rules/task-context.md   → pointer (@.adb/task-context.md)
└── AGENTS.md                       → pointer, written only when absent
```

Two properties are worth knowing, because both were deliberate choices:

- **The canonical file lives under `.adb/`, not at the worktree root.** A worktree is a
  checkout of somebody else's repo, and a growing number of repos ship their own
  `AGENTS.md`. Had the context itself gone there, adb's never-clobber rule would correctly
  leave that file alone and the agent would silently get *no* context. As a pointer, a
  skipped `AGENTS.md` costs one harness's convenience and the context is still on disk.
- **adb excludes its own output from git**, by appending these paths to the clone's
  `.git/info/exclude` — local to the clone, never committed, and not the repo's tracked
  `.gitignore`, which belongs to you. Without it, a brand-new worktree was instantly
  *dirty*: `adb task list --git` reported `dirty` for every ticket and the safe-teardown
  guard refused to remove the worktree, over a file adb itself had just written.

---

## 6. The core `adb task` commands

These are the subcommands you'll use to move a ticket through its lifecycle. Every flag
below is real — registered in [`internal/cli/task.go`](../../internal/cli/task.go).

### Create a task

```bash
# Minimal — a repo-less feature task (lands in tickets/_local/, no worktree)
adb task create "Add retry to the uploader"

# A repo-backed fix, high priority, with metadata (creates a worktree + branch)
adb task create "Fix ECS datetime string crash" \
  --type fix \
  --repo github.com/awslabs/mcp \
  --priority P1 \
  --owner valter \
  --tags ecs,crash \
  --description "Datetime string is not parsed on the ECS path" \
  --acceptance "unit test reproduces the crash,fix passes CI"

# Scripting / CI / MCP — create without launching an interactive Claude session
adb task create "Chore: bump deps" --type chore --no-launch
```

`adb task create <branch>` flags:

| Flag | Default | Notes |
|------|---------|-------|
| `--type` | `feat` | Must be one of the 10 `ValidTaskTypes` (8 code + `work`/`prototype`); `bug` is rejected |
| `--repo` | *(none)* | Platform-qualified, e.g. `github.com/org/repo`; enables the worktree + nesting |
| `--priority` | `P2` | `P0`, `P1`, `P2`, or `P3` |
| `--owner` | *(none)* | Task owner |
| `--tags` | *(none)* | Comma-separated |
| `--description` | *(none)* | Free-form |
| `--acceptance` | *(none)* | Comma-separated acceptance criteria |
| `--no-launch` | `false` | Skip the post-create agent launch (also honours `ADB_NO_LAUNCH=1`) |
| `--agent` | `claude` | Coding agent to launch: `claude`, `pi`, `codex`, `amazon-q` (precedence: `--agent` > `ADB_AGENT` env > `custom_settings.launch_agent` config > `claude`) |

> After creating a repo-backed task, `adb` launches a coding-agent workflow in the new
> worktree **unless** `--no-launch` is passed or `ADB_NO_LAUNCH=1` is set — this is what
> makes `create` safe for scripts, CI, and the MCP server.

### List / inspect

```bash
adb task list                       # human table of all tasks, grouped by status
adb task list --filter in_progress  # only in-progress tasks
adb task list --json                # a JSON array (id/status/branch/worktree_path/ticket_path/…)
adb task list --git                 # + live worktree state: branch, dirty, ahead/behind, missing
adb task show TASK-00001            # one task in detail
adb task show TASK-00001 --json     # …as a single JSON object
```

`adb task list` takes **no positional arguments** — it lists and filters. The per-task read
is `adb task show <id>`, which `list` never was: reaching for a single task used to mean
`--json` plus a `jq` select over the whole array. Filter values: `backlog`, `in_progress`,
`blocked`, `review`, `done`, `archived`.

#### The JSON contract: a flag never changes the shape

This is the one property to internalise before you script against it:

| Command | `--json` emits |
|---|---|
| `adb task list` | **always an array**, with or without `--git` |
| `adb task show <id>` | a single **object**, same field set as a `list` row — not a one-element array |
| `adb task worktree list` | `{"worktrees": [...], "orphaned": [...]}` — the only one with an envelope |

`--git` **adds keys** to each row — `worktree_exists`, `dirty`, `ahead`, `behind`,
`worktree_missing` — instead of switching the document to a different top-level shape. All
five are non-`omitempty`, deliberately: `false` and `0` are real answers here, and a missing
`dirty` key would read as "unknown" rather than "clean".

Two consequences worth stating outright:

- **`--git` does not filter.** It used to: the old `--git` path ran through
  `buildStatusRows`, which silently drops archived tasks and tasks with no worktree. Merging
  that into the task array would have made a *flag* change which tasks appear. Now every
  task keeps its row and a task with no worktree simply carries the git keys at their zero
  values.
- **`adb task worktree list` is the only listing with an envelope**, and only because it
  owns data that is not a task. An **orphaned** worktree — one sitting on disk that no
  active ticket owns — has no task row for `orphaned` to be a key on, so it needs a
  document-level slot. Everything that *is* a task is a row.

Empty is `[]`, never `null` — pinned by an end-to-end test, because a consumer that pipes
into `jq '.[]'` should not have to special-case a fresh workspace.

> **This is the one breaking change in the TASK-00039 rename.** `adb status --json` and
> `adb task status --git --json` used to emit
> `{"tickets": […], "orphaned_worktrees": […]}`; they now emit the array. The hidden alias
> keeps the *command* working — it does not keep the old shape, because a flag-dependent
> shape is precisely the thing being fixed. If you parsed `.tickets`, read the array
> directly; if you parsed `.orphaned_worktrees`, read `adb task worktree list --json`.

### Read a task's history

```bash
adb task timeline TASK-00001            # every recorded event, oldest first
adb task timeline TASK-00001 --since 24h
adb task timeline TASK-00001 --json     # same array shape as `adb events query --json`
```

`timeline` is a **view**, not a store: it filters `.adb/events.jsonl` on `data.task_id`,
reusing the same `filterEvents` helper `adb events query` uses. So its `--json` is
byte-compatible with `adb events query --json`, and — the honest limit — a task created
before the event log existed has an **empty** timeline rather than a reconstructed one.
Nothing is inferred from `backlog.yaml` timestamps.

### Move a task through the lifecycle

```bash
adb task resume TASK-00001            # backlog → in_progress, launches your agent
adb task resume TASK-00001 --agent pi # …launch pi instead of the default

adb task update TASK-00001 --status blocked
adb task update TASK-00001 --status review
adb task close TASK-00001                                  # → done (status only)

adb task update TASK-00001 --priority P0                   # priority is a property, not a verb
adb task update TASK-00001 --owner valter --priority P1    # any combination, one call
```

`adb task update <task-id>` is the **single setter**: `--status`, `--priority`, `--owner`,
and `--initiative` (pass `--initiative ""` to clear it), all optional — it prints a no-op
message if you supply none. There is no separate `priority` verb: a task's priority is one
of its properties, so setting it belongs with setting the others rather than in a command
of its own.

`adb task close <id>` is the exception, and it earns its place by being the end of the loop
you type every day. It is shorthand for `--status done` and nothing more — see §2 for what
closing does and does not touch.

#### `resume` vs `start` — the difference is memory

Both promote the task to `in_progress` **and** launch your agent in the task's worktree
(or its ticket directory, for a repo-less task). They differ in one thing:

| Command | Session |
|---|---|
| `adb task start <id>` | always starts a **new** conversation |
| `adb task resume <id>` | **continues** the last conversation for that directory, and starts a fresh one when there is none — printing which of the two it did |

Reach for `start` when you want the agent to come in cold — a stale conversation is worse
than none when the plan has changed. Reach for `resume` to pick up where you left off.

Both accept `--agent`. **Only `start` accepts `--no-launch`** — it promotes without launching
anything, which is what scripts, CI, and the MCP server want. So the non-interactive spelling
of "promote this ticket" is `adb task start <id> --no-launch`, which is why every scripted
loop below uses `start`.

**But `ADB_NO_LAUNCH=1` suppresses the launch for all three of `create`, `start` and
`resume`.** The flag and the environment variable are different things, and the distinction
is deliberate:

| | `--no-launch` | `ADB_NO_LAUNCH=1` |
|---|---|---|
| scope | one invocation | the whole shell session |
| `task create` | yes | yes |
| `task start` | yes | yes |
| `task resume` | **no** | **yes** |

`resume` has no flag because a `resume --no-launch` would mean "continue the previous
conversation, but do not" — which is just `adb task update <id> --status in_progress`. A
flag whose meaning is another command is worth leaving out. The env var is different in kind:
it is a global you already set, and until TASK-00039 `resume` silently ignored it, so a
script that exported it still got handed an interactive session it had explicitly disabled.
When it fires, `resume` says so on **stderr** and exits 0 — a silent no-launch is
indistinguishable from a broken agent.

```bash
adb task start TASK-00001              # fresh session
adb task resume TASK-00001             # continue, or fresh if none — it says so
adb task start TASK-00001 --no-launch  # promote only

export ADB_NO_LAUNCH=1                 # promote only, for every command in this shell
adb task resume TASK-00001             # → "ADB_NO_LAUNCH=1 is set — promoted only, …"
```

### Acting on many tasks at once

`adb task start-all` and `adb task close-all` were **removed** in TASK-00039. A shell
loop does the same thing, is scriptable, and — the actual reason — leaves a record of
*which* tickets moved instead of a summary count:

```bash
# promote every backlog ticket, launching nothing
adb task list --json --filter backlog | jq -r '.[].id' \
  | xargs -n1 -I{} adb task start {} --no-launch

# close everything currently active
adb task list --json --filter in_progress | jq -r '.[].id' \
  | xargs -n1 -I{} adb task close {}
```

Closing a task only flips its status: it does **not** archive it or remove its worktree,
and `adb task update --status in_progress` reverses it.

### Manage the worktrees behind your tasks

A repo-backed task owns a git worktree (§5), and `adb task worktree` is the namespace for
that half of a ticket's existence — it used to be a top-level `adb work`, which made a
task's worktree look like a separate kind of thing:

```bash
adb task worktree list                     # worktree-bearing tasks + branch + present/missing, and orphans
adb task worktree switch TASK-00001        # prints the path — cd "$(adb task worktree switch TASK-00001)"
adb task worktree remove TASK-00001        # remove ONLY the worktree; keep all ticket data
adb task worktree remove TASK-00001 --force  # …even with uncommitted/unpushed work
adb task worktree prune                    # PREVIEW: which orphans would be removed
adb task worktree prune --apply            # actually remove them
adb task worktree reconcile                # PREVIEW: rebuild worktrees backlog.yaml records but disk lacks
adb task worktree reconcile --apply --prune
```

`list` shows only worktree-bearing tasks; for every task, worktree or not, use
`adb task list --git`.

#### One rule decides whether a worktree verb acts or previews

> **A worktree mutation that names its target acts immediately. One that sweeps a set
> previews unless `--apply`.**

So `worktree remove <id>` removes when you press enter — you said which worktree — while
`prune` and `reconcile` print a plan and change nothing until `--apply`. They sweep a set
you did not enumerate, and the whole point of running them is that you do not already know
what is in it.

That rule is why the same word is now safe under two different nouns. `adb repo worktree
prune` (the v3 repository surface) has always previewed unless `--apply`, and additionally
requires `--path <exact-path>`; `adb work prune` removed on sight. Two commands spelled
`worktree prune` with opposite safety postures is a trap regardless of which one you meant,
so the sweeping verbs now agree.

**Behaviour change on a deprecated alias:** bare `adb work prune` used to remove and now
previews. `--dry-run` survives on both sweepers as a hidden, accepted no-op — previewing
*is* the default now — so `adb work prune --dry-run` keeps parsing and keeps meaning exactly
what it always meant.

### Retire a task

```bash
adb task archive TASK-00001            # → archived: move ticket to _archived/, remove worktree
adb task archive TASK-00001 --force    # remove the worktree even with dirty/unpushed work
adb task archive TASK-00001 --keep-worktree   # archive the ticket but leave the worktree
adb task archive TASK-00001 --prune-branch    # also delete the task's local branch
adb task update TASK-00001 --status backlog   # un-archive: move the ticket back out of _archived/
adb task remove TASK-00001 --yes       # DESTROY: worktree + ticket dir + backlog entry
```

`adb task remove` is the irreversible one — it deletes the ticket directory, so the notes,
decisions, and knowledge go with it. `--yes` is required (rather than an interactive prompt)
so the destructive spelling is explicit in a scripted invocation too. When you want to
retire a ticket but keep its record, that is `archive`; when you want to reclaim disk but
keep the ticket, that is `task worktree remove`.

> **Safe teardown (#207).** `task worktree remove`/`archive` refuse to remove a worktree
> that has uncommitted/untracked changes or unpushed commits — `worktree remove` errors,
> `archive` leaves the worktree in place with a warning — so in-flight work is never
> discarded silently. Pass `--force` to override. `adb task archive --keep-worktree` now
> genuinely **keeps** the worktree (it was previously a no-op that removed it anyway), and
> `--prune-branch` deletes the task's local `<type>/<slug>` branch once its worktree is gone
> ([`internal/cli/task.go`](../../internal/cli/task.go), [`internal/integration/worktree.go`](../../internal/integration/worktree.go)).

---

## 7. A complete worked example

Take a bug fix from creation to done:

```bash
# 1. Create it against a real repo → ticket at
#    tickets/github.com/awslabs/mcp/TASK-00007-fix-ecs-datetime-string-crash/
#    worktree at work/github.com/awslabs/mcp/TASK-00007-fix-ecs-datetime-string-crash/
#    branch: fix/fix-ecs-datetime-string-crash
adb task create "Fix ECS datetime string crash" \
  --type fix --repo github.com/awslabs/mcp --priority P1 --no-launch

# 2. See where it landed
adb task show TASK-00007          # or: adb task list --json --filter backlog

# 3. Start working (promotes backlog → in_progress)
adb task resume TASK-00007

# 4. …do the work in the worktree, then move to review
adb task update TASK-00007 --status review

# 5. Land it
adb task close TASK-00007

# 6. Once merged, retire it
adb task archive TASK-00007

# …and if you ever need to explain what happened when
adb task timeline TASK-00007
```

At each step, the ticket's `notes.md` and `status.yaml` in the ticket directory are
yours to keep current, and the branch/worktree stay in lockstep with the correlation
layout.

---

## 8. Which coding agent runs

`adb` manages tickets, worktrees, and written-down context; the model, the harness rules,
the hooks, and the session store belong to whichever agent you use. Which one it launches is
resolved most-specific-first: **`--agent` flag → `ADB_AGENT` env → the `launch_agent` custom
setting in the layered config → the default**.

```bash
adb task resume TASK-00001 --agent pi
export ADB_AGENT=pi
```

```yaml
# ~/.taskconfig (global), orgs/<id>/config.yaml (org), or .taskrc (per-repo)
custom_settings:
  launch_agent: pi
```

**Four agents are wired today:**

| `--agent` | Binary | Resume | Verified against the real CLI |
|---|---|---|---|
| `claude` (default) | `claude` | `--continue` | yes |
| `pi` | `pi` | `--continue` | yes |
| `codex` | `codex` | `codex resume --last` | yes |
| `amazon-q` | `q` | `q chat --resume` | **no** — see below |

An unrecognized name is rejected **up front** with the valid list, before anything is
created or promoted. That last part was a real bug: validation used to happen inside the
launch block, which a repo-less task never reaches, so `--agent gpt5` created the task,
printed a checkmark, and silently ignored the flag.

Two honest notes:

- **`amazon-q` is unverified.** The `q` CLI was not installed on the machine this was
  wired on, so its argv and transcript location come from Amazon Q's documentation rather
  than from driving it. The failure modes are bounded: a missing binary gives
  `q CLI not found in PATH`, and a wrong resume flag surfaces as `q`'s own usage error.
- **`codex resume --last` is directory-scoped**, which is what makes it safe to use here —
  Codex filters the session picker by cwd unless you pass `--all`. Without that, a resume
  in one worktree could have continued another worktree's conversation. Codex also stores
  transcripts date-partitioned rather than per project, so adb's "is there a prior session
  here?" probe reads each recent rollout's recorded `cwd`.

Adding a fifth is now **one registry entry** (`internal/cli/agents.go`), not five scattered
edits — see [L400](./L400-architecture-and-extending.md).

### The instruction file is `AGENTS.md`

`adb context build` writes the generated context to **`AGENTS.md`** — the open,
vendor-neutral convention ([agents.md](https://agents.md), stewarded by the Agentic AI
Foundation) that Codex, Cursor, Aider, Gemini CLI, Zed, Devin, Copilot's coding agent and
others read. Agents pick up the nearest such file up the directory tree.

Claude Code reads `CLAUDE.md` instead, so adb keeps a **thin pointer** beside the canonical
file rather than a second copy of the context:

```text
AGENTS.md              ← the whole generated context
├── CLAUDE.md          → @AGENTS.md
└── CODEX.md           → @AGENTS.md      (opt in)
```

The `@AGENTS.md` line is load-bearing, not decorative: Claude Code only actually loads a
referenced file through its `@` import syntax, so a pointer carrying just a markdown link
would look right and supply no context at all.

Configure the pointer list with the `instruction_pointers` custom setting — comma-separated,
or `none` for only `AGENTS.md`:

```yaml
custom_settings:
  instruction_pointers: CLAUDE.md,CODEX.md
```

> **A hand-written instruction file is never overwritten.** adb marks the files it generates;
> a target that exists without that marker (or a recognizable pre-marker generated header) is
> left byte-for-byte alone and reported as skipped on stderr. Pass `--force` to overwrite it
> deliberately.

Elsewhere the picture is still mixed, and worth knowing before you assume a name is generic.
**Named for Claude Code and genuinely specific to it:** `adb harness install`,
`adb init claude`, `adb harness build`, session-transcript parsing, and `adb hook install`.
The Claude harness itself is isolated under `harnesses/claude/`, and
`adb hook process --event <name>` is the neutral hook entrypoint.

The per-worktree context **used to be on that list** and no longer is: it is now
`.adb/task-context.md` with a pointer per harness (§5), so `.claude/rules/task-context.md`
is one consumer of a neutral file rather than the only place the context exists.

### Durable sessions

Launches are hosted in a **tmux** session named deterministically from the launch directory, so
a session survives the terminal that started it and re-running `resume` reattaches instead of
starting a second one. `ADB_TMUX=0` opts out (the agent then dies with the terminal), and
`ADB_TMUX_PREFIX` renames the session namespace. Needs `tmux` on `PATH`; absent, adb falls
back to a plain non-durable launch rather than failing.

adb is terminal-native — there is no editor integration. (A VS Code extension and its
`--here` bypass flag were removed on 2026-09-14; see [L200](./L200-daily-workflows.md#what-was-removed).)

---

## Where to go next

- **[L200 — Daily Workflows](./L200-daily-workflows.md):** syncing tickets with GitHub/GitLab
  issues (`adb issues sync`), watching the event stream (`adb events`), metrics
  (`adb metrics`), and running many tickets in parallel from the terminal.
- **[L300 — Integrations](./L300-integrations.md):** cloud archive
  (`adb archive`), the background scheduler, vector memory, the MCP server
  (`adb mcp serve`), and hooks.

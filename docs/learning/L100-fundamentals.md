# L100 — Fundamentals

> **Learning tier:** L100 (foundations) · **Goal after this page:** *"I can manage tasks."*
>
> By the end you will understand how `adb` models a unit of work — its **status
> lifecycle**, its **type taxonomy**, how a type becomes a **branch name**, where the
> **ticket and worktree live on disk**, and the handful of `adb task` commands that
> carry a ticket from idea to done. Everything here is verified against the current
> `main`.

**Next tiers:**

- [L200 — Daily Workflows](./L200-daily-workflows.md) — issue sync, the event stream, metrics, and the VS Code extension.
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
| `in_progress` | Actively being worked | `adb task resume`, `adb task start-all` |
| `blocked` | Waiting on a dependency | `adb task update --status blocked` |
| `review` | Work done, under review | `adb task update --status review` |
| `done` | Complete | `adb task update --status done`, `adb task close-all` |
| `archived` | Retired; ticket moved to `tickets/_archived/`, worktree removed | `adb task archive` |

Two helpers on the model classify these states
([`pkg/models/task.go`](../../pkg/models/task.go)):

- `Task.IsActive()` → `true` for `in_progress`, `review`, or `blocked`.
- `Task.IsBlocked()` → `true` for `blocked` **or** any non-empty `BlockedBy` list.

### Verified transitions

These are the transitions the code actually performs (not everything is a free-for-all):

- **create** → always lands at `backlog` (`NewTask`).
- **`adb task resume`** → flips `backlog` → `in_progress` (only when currently `backlog`;
  it refuses an already-archived task).
- **`adb task update --status`** → the escape hatch: sets any of the 6 states directly.
- **`adb task start-all`** → promotes **only** `backlog` tasks to `in_progress`
  (idempotent — skips everything else).
- **`adb task close-all`** → flips **only** active (`in_progress`/`blocked`/`review`) tasks
  to `done`. It does **not** archive or remove worktrees — it is reversible with
  `adb task update --status`.
- **`adb task archive`** → moves any task to `archived`, relocates the ticket dir under
  `tickets/_archived/` (preserving its nested sub-path), and removes the worktree.
- **`adb task unarchive`** → moves an archived ticket back and returns it to `backlog`.

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
> Old `backlog.yaml` entries that still say `bug` are rewritten to `fix` by
> `adb task migrate-types` (L300).

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

When a **worktree** exists, `adb` also writes
`.claude/rules/task-context.md` **inside the worktree** (not the ticket dir) so an agent
started there gets the ticket's context automatically.

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
| `--no-launch` | `false` | Skip the post-create Claude Code launch (also honours `ADB_NO_LAUNCH=1`) |

> After creating a repo-backed task, `adb` launches a Claude Code workflow in the new
> worktree **unless** `--no-launch` is passed or `ADB_NO_LAUNCH=1` is set — this is what
> makes `create` safe for scripts, CI, and the MCP server.

### List / inspect

```bash
adb task status                       # human table of all tasks
adb task status --filter in_progress  # only in-progress tasks
adb task status --json                # stable JSON array (id/status/branch/worktree_path/ticket_path/…)
```

`adb task status` takes **no positional arguments** — it lists and filters, it is not a
per-task show. Machine-readable per-task data comes from `--json` (an array of all/
filtered tasks). Filter values: `backlog`, `in_progress`, `blocked`, `review`, `done`,
`archived`.

### Move a task through the lifecycle

```bash
adb task resume TASK-00001            # backlog → in_progress, launches the workflow
adb task resume TASK-00001 --here     # …but exec claude in-place (no VS Code launch handoff)

adb task update TASK-00001 --status blocked
adb task update TASK-00001 --status review
adb task update TASK-00001 --status done --owner valter   # can also set --priority/--owner

adb task priority TASK-00001 TASK-00002 --priority P0      # --priority is REQUIRED here; accepts multiple IDs
```

`adb task update <task-id>` accepts `--status`, `--priority`, and `--owner` (all
optional; it prints a no-op message if you supply none). `adb task priority` **requires**
`--priority` and accepts one or more IDs.

### Bulk operations

```bash
adb task start-all          # promote ALL backlog tasks → in_progress (prompts to confirm)
adb task start-all -y       # …skip the confirmation

adb task close-all          # flip ALL active (in_progress/blocked/review) tasks → done
adb task close-all -y       # …skip the confirmation
```

Both take `-y`/`--yes` to skip the interactive confirm, process each task independently,
and exit non-zero if any task failed. `close-all` is **not** destructive — it only flips
status; it does not archive or remove worktrees.

### Retire a task

```bash
adb task cleanup TASK-00001            # remove ONLY the worktree; keep all ticket data
adb task cleanup TASK-00001 --force    # …even if it has uncommitted/unpushed work
adb task archive TASK-00001            # → archived: move ticket to _archived/, remove worktree
adb task archive TASK-00001 --force    # remove the worktree even with dirty/unpushed work
adb task archive TASK-00001 --keep-worktree   # archive the ticket but leave the worktree
adb task archive TASK-00001 --prune-branch    # also delete the task's local branch
adb task unarchive TASK-00001          # archived → backlog; move the ticket back
```

> **Safe teardown (#207).** `cleanup`/`archive` refuse to remove a worktree that has
> uncommitted/untracked changes or unpushed commits — `cleanup` errors, `archive` leaves the
> worktree in place with a warning — so in-flight work is never discarded silently. Pass
> `--force` to override. `adb task archive --keep-worktree` now genuinely **keeps** the
> worktree (it was previously a no-op that removed it anyway), and `--prune-branch` deletes the
> task's local `<type>/<slug>` branch once its worktree is gone
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
adb task status --json --filter backlog

# 3. Start working (promotes backlog → in_progress)
adb task resume TASK-00007 --here

# 4. …do the work in the worktree, then move to review
adb task update TASK-00007 --status review

# 5. Land it
adb task update TASK-00007 --status done

# 6. Once merged, retire it
adb task archive TASK-00007
```

At each step, the ticket's `notes.md` and `status.yaml` in the ticket directory are
yours to keep current, and the branch/worktree stay in lockstep with the correlation
layout.

---

## Where to go next

- **[L200 — Daily Workflows](./L200-daily-workflows.md):** syncing tickets with GitHub/GitLab
  issues (`adb sync issues`), watching the event stream (`adb events`), metrics
  (`adb metrics`), and driving all of this from the VS Code extension.
- **[L300 — Integrations](./L300-integrations.md):** cloud archive
  (`adb sync cloud`), the background scheduler, vector memory, the MCP server
  (`adb mcp serve`), and hooks.

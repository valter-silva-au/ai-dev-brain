# Getting Started with adb

Zero-to-productive in about 15 minutes. This guide installs **adb** (AI Dev Brain), explains the three-plane workspace model, and walks the first task loop (create → start/resume → close). By the end you will have created a real task, opened an agent session in an isolated git worktree, and closed it out.

> **adb** is a Go CLI that wraps AI coding assistants with persistent context, task-lifecycle automation, and knowledge accumulation. It keeps AI sessions stateful across runs by maintaining structured per-task context and tracking your work in a version-controlled workspace.

---

## Table of contents

- [Prerequisites](#prerequisites)
- [1. Install](#1-install)
- [2. The three-plane mental model](#2-the-three-plane-mental-model)
- [3. Initialize a workspace](#3-initialize-a-workspace)
- [4. The first loop: create → resume → close](#4-the-first-loop-create--resume--close)
- [6. Where to go next](#6-where-to-go-next)

---

## Prerequisites

- **Go 1.21+** — to build the CLI from source (`go build`, see [`Makefile`](../Makefile)).
- **Git** — adb creates per-task work in git worktrees, so the repos you work in must be git repositories.
- **Claude Code** — adb launches `claude` for you at the end of `adb task create` / `adb task resume`. Install it and confirm `claude --version` works.

Optional, only needed for specific features covered in the higher tiers:

- **`gh` / `glab`** (authenticated) — for `adb issues sync` (GitHub/GitLab issue sync).
- **`gitleaks`** and an S3 bucket — for `adb archive`.

---

## 1. Install

Build and install the CLI to `~/.local/bin` with the Makefile:

```bash
git clone https://github.com/valter-silva-au/ai-dev-brain.git
cd ai-dev-brain
make install-local
```

`make install-local` builds the binary with version ldflags and copies it to `~/.local/bin/adb`. On macOS (Apple Silicon) it also re-signs the copied binary with an ad-hoc signature — `cp` invalidates the Go linker's ad-hoc signature and the copied binary would otherwise be killed on exec. See [`Makefile`](../Makefile) (`install-local` target).

Make sure `~/.local/bin` is on your `PATH`, then verify:

```bash
adb version
```

This prints the version, commit, and build date. `adb version` works without a workspace (unlike most commands, which require an initialized workspace).

> **Just want the binary?** `make build` produces `./adb` in the repo root without installing it.

---

## 2. The three-plane mental model

adb splits your workspace into three parallel trees. Understanding this layout is the single most important concept — everything else follows from it.

| Plane | Path root | What lives here | Git status |
|-------|-----------|-----------------|------------|
| **Planning** | `tickets/` | Per-task planning docs: `context.md`, `notes.md`, `design.md`, `status.yaml`, plus `sessions/` and `knowledge/` subdirs | Version-controlled |
| **Work** | `work/` | The git **worktree** for each task — an isolated checkout on the task's own branch, where code actually changes | Gitignored (worktrees have their own git) |
| **Baselines** | `repos/` | Read-only clones of the repositories you contribute to | Reference only |

The planning plane and the work plane are **symmetric**: a task's ticket dir and its worktree dir mirror each other, differing only in the `tickets/` vs `work/` root.

### The nested `<platform>/<org>/<repo>` layout

When a task targets a repository, adb nests both its ticket and its worktree under a platform → org → repo path so the location itself tells you what the task is:

```
tickets/github.com/valter-silva-au/ai-dev-brain/TASK-00083-adb-docs-getting-started-and-learning-tiers/
work/github.com/valter-silva-au/ai-dev-brain/TASK-00083-adb-docs-getting-started-and-learning-tiers/
```

The directory selection rule lives in [`internal/core/bootstrap.go:resolveTaskDir`](../internal/core/bootstrap.go). Four cases:

1. **repo + slug** → `tickets/<platform>/<org>/<repo>/TASK-<id>-<slug>` (the normal case).
2. **repo only** → `tickets/<platform>/<org>/<repo>/TASK-<id>`.
3. **no repo, slug only** → `tickets/_local/TASK-<id>-<slug>` — the reserved bucket for repo-less / workspace-meta tasks.
4. **neither** → legacy flat `tickets/TASK-<id>` (older path; the live create flow always supplies a slug).

Tasks always get a globally-unique `TASK-<id>` key. Tools resolve a task back to its directory by id via [`internal/core/resolve.go:ResolveTicketDir`](../internal/core/resolve.go), which walks the tree matching any dir whose base is `<id>` or starts with `<id>-` (the trailing dash keeps `TASK-1` from matching `TASK-10-foo`). A live ticket always wins over an archived one.

---

## 3. Initialize a workspace

If you are starting a fresh workspace (as opposed to working inside this repo, which is already set up), scaffold one:

```bash
adb init workspace ~/my-workspace
```

Flags (all optional): `--name <str>`, `--ai <provider>` (default `claude`), `--prefix <str>` (task-ID prefix, default `TASK`). See [`internal/cli/init.go`](../internal/cli/init.go).

adb finds its workspace root via the `ADB_HOME` environment variable, or by walking up from the current directory. Export it once so every invocation (including scripts and the MCP server, whose launch directory is unpredictable) agrees on the root:

```bash
export ADB_HOME=~/my-workspace
```

---

## 4. The first loop: create → start → close

This is the core daily cycle. All three commands are subcommands of `adb task` ([`internal/cli/task.go`](../internal/cli/task.go)).

### 4a. Create a task

```bash
adb task create "add getting-started doc" --type docs --repo github.com/valter-silva-au/ai-dev-brain --priority P2
```

What happens:

- The positional argument is a **title**, not a ready-made branch name. adb derives the branch as `<conventional-type>/<slug>` via [`pkg/models/task.go:BranchName`](../pkg/models/task.go) — e.g. the above becomes `docs/add-getting-started-doc`. (Never `task/<id>`.)
- `--type` accepts the 8 Conventional-Commits **code** types **`feat`, `fix`, `refactor`, `docs`, `chore`, `test`, `perf`, `spike`** (default `feat`) plus two **non-code** types **`work`** (artifact/graph deliverable — no worktree/branch) and **`prototype`** (time-boxed experiment). `spike`/`prototype` map to a `chore/` branch prefix. The legacy `bug` type is **retired** — passing `--type bug` is rejected with a hint to use `fix`.
- `--priority` is one of `P0`, `P1`, `P2`, `P3` (default `P2`).
- `--repo` should be platform-qualified (`<platform>/<org>/<repo>`). When set, adb creates the isolated worktree under `work/...`; without it, the task is repo-less and lands under `tickets/_local/`.
- Because `--repo` was set, adb creates the worktree and then **launches a Claude Code session** in it.

Other create flags: `--owner <str>`, `--tags <csv>`, `--description <str>`, `--acceptance <csv>`, and `--no-launch`.

> **Scripting / CI:** pass `--no-launch` (or set `ADB_NO_LAUNCH=1`) to create the task and worktree **without** starting an interactive Claude Code session — otherwise a non-interactive caller would block. See `suppressLaunch` in [`internal/cli/task.go`](../internal/cli/task.go).

```bash
# CI-safe: create without launching Claude Code
adb task create "add getting-started doc" --type docs \
  --repo github.com/valter-silva-au/ai-dev-brain --no-launch
```

The command prints the new task's ID, branch, worktree path, and ticket path. A newly created task starts in **`backlog`** at priority `P2` (see [`pkg/models/task.go:NewTask`](../pkg/models/task.go)).

### 4b. See what you have

```bash
adb task list
adb task list --filter backlog
adb task list --json
adb task list --git              # + live worktree state: branch, dirty, ahead/behind
adb task show TASK-00083         # one task, in detail
```

`adb task list` lists and filters; `adb task show <id>` is the per-task read. `--filter` accepts any of the six statuses: `backlog`, `in_progress`, `blocked`, `review`, `done`, `archived`.

One `--json` shape per question, and the split is deliberate: **`list --json` is always an array**, including `worktree_path`, `ticket_path`, `branch`, and `initiative`; **`show --json` is a single object** with the same field set, so one decoder handles both. `--git` *adds keys* to each row (`worktree_exists`, `dirty`, `ahead`, `behind`, `worktree_missing`) rather than changing the document's shape — a flag should never do that — and it does not filter, so a task with no worktree keeps its row with those keys at zero values.

> Worktrees that no ticket owns have no task row to hang off, so they are reported by the command that owns them: `adb task worktree list` (`--json` emits `{"worktrees": [...], "orphaned": [...]}` — the only task listing with an envelope, for exactly that reason).

### 4c. Resume a task

Promote a backlog task to `in_progress` and launch its workflow:

```bash
adb task resume TASK-00083
```

adb launches your configured coding agent in the task's worktree (or, for a repo-less task, in its ticket dir), hosted in tmux so the session survives the terminal:

```bash
adb task resume TASK-00083
```

`resume` only flips a task from `backlog` → `in_progress`; it is a no-op on the status of a task that is already active, and it refuses archived tasks. See `newTaskResumeCmd` in [`internal/cli/task.go`](../internal/cli/task.go).

### 4d. Update as you work

```bash
adb task update TASK-00083 --status review
adb task update TASK-00083 --priority P1
adb task timeline TASK-00083        # what has happened to this ticket so far
```

`adb task update` is the one setter: `--status`, `--priority`, `--owner`, `--initiative`, in any combination. `adb task timeline` replays the ticket's slice of the event log (creation, status and priority changes, worktree create/remove, agent sessions), oldest first, `--since 24h` to narrow.

### 4e. Close it out

Mark the task done:

```bash
adb task close TASK-00083           # or: adb task update TASK-00083 --status done
```

Closing only changes the status. The ticket directory, the worktree, and the branch are all left alone, and `adb task update TASK-00083 --status in_progress` reverses it.

When you are completely finished and want to reclaim the worktree, archive it:

```bash
adb task archive TASK-00083
```

`archive` moves the ticket dir into `tickets/_archived/` (preserving its nested sub-path), writes a `handoff.md`, and **removes the worktree**. A worktree with uncommitted or unpushed work is left in place with a warning; `--force` removes it anyway, `--keep-worktree` always keeps it, and `--prune-branch` also deletes the task's local branch once the worktree is gone.

If you only want to reclaim the worktree but keep the ticket data, use `adb task worktree remove TASK-00083`. To bring an archived task back, `adb task update TASK-00083 --status backlog` — which really does move the ticket directory back out of `_archived/`, not just relabel it.

> **`adb task update --status archived` is rejected**, and deliberately so: archiving is a *move*, not a field. Setting the field alone used to leave a workspace claiming a task was archived while its worktree and ticket directory were still sitting there live. The error names `adb task archive`, which owns the flags above.

To wipe a task entirely — worktree, ticket directory, and backlog entry — `adb task remove TASK-00083 --yes`. That one is irreversible, which is why `--yes` is required rather than a prompt.

### Bulk operations

```bash
# Bulk verbs were removed in TASK-00039 — loop instead, so you can see which
# tickets actually moved:
adb task list --json --filter backlog | jq -r '.[].id' \
  | xargs -n1 -I{} adb task start {} --no-launch

# close everything currently in progress
adb task list --json --filter in_progress | jq -r '.[].id' \
  | xargs -n1 -I{} adb task close {}
```

---


## 5. Where to go next

You now know the loop. The **learning tiers** go deeper, each building on the last:

- **[L100 — Fundamentals](learning/L100-fundamentals.md):** the workspace model, task lifecycle, and daily commands in depth.
- **[L200 — Daily workflows](learning/L200-daily-workflows.md):** running many tickets in parallel, durable tmux-hosted sessions, and observability (`adb events`, `adb metrics`, `adb alerts`).
- **[L300 — Integrations](learning/L300-integrations.md):** GitHub/GitLab issue sync (`adb issues sync`), cloud archive (`adb archive`), and the MCP server (`adb mcp serve`).
- **[L400 — Architecture & extending](learning/L400-architecture-and-extending.md):** architecture, the layered package design, the correlation layout, and extending adb.

Reference material:

- **[README.md](../README.md)** — architecture overview and package structure.
- **[docs/architecture/](architecture/)** — design docs.
- Full command surface: [`internal/cli/root.go:NewRootCmd`](../internal/cli/root.go) is the authoritative registration of every top-level command.

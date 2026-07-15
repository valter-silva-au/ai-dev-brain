# Getting Started with adb

Zero-to-productive in about 15 minutes. This guide installs **adb** (AI Dev Brain), explains the three-plane workspace model, walks the first task loop (create → resume → close), and sets up the VS Code extension. By the end you will have created a real task, opened a Claude Code session in an isolated git worktree, and closed it out.

> **adb** is a Go CLI that wraps AI coding assistants with persistent context, task-lifecycle automation, and knowledge accumulation. It keeps AI sessions stateful across runs by maintaining structured per-task context and tracking your work in a version-controlled workspace.

---

## Table of contents

- [Prerequisites](#prerequisites)
- [1. Install](#1-install)
- [2. The three-plane mental model](#2-the-three-plane-mental-model)
- [3. Initialize a workspace](#3-initialize-a-workspace)
- [4. The first loop: create → resume → close](#4-the-first-loop-create--resume--close)
- [5. VS Code extension setup](#5-vs-code-extension-setup)
- [6. Where to go next](#6-where-to-go-next)

---

## Prerequisites

- **Go 1.21+** — to build the CLI from source (`go build`, see [`Makefile`](../Makefile)).
- **Git** — adb creates per-task work in git worktrees, so the repos you work in must be git repositories.
- **Claude Code** — adb launches `claude` for you at the end of `adb task create` / `adb task resume`. Install it and confirm `claude --version` works.
- **VS Code** (optional) — for the extension in [step 5](#5-vs-code-extension-setup).

Optional, only needed for specific features covered in the higher tiers:

- **`gh` / `glab`** (authenticated) — for `adb sync issues` (GitHub/GitLab issue sync).
- **`gitleaks`** and an S3 bucket — for `adb sync cloud`.

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

## 4. The first loop: create → resume → close

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
adb task status
adb task status --filter backlog
adb task status --json
```

`adb task status` lists/filters tasks (it takes no positional argument — it is not a per-task show). `--filter` accepts any of the six statuses: `backlog`, `in_progress`, `blocked`, `review`, `done`, `archived`. `--json` emits a stable array including `worktree_path`, `ticket_path`, and `branch`.

### 4c. Resume a task

Promote a backlog task to `in_progress` and launch its workflow:

```bash
adb task resume TASK-00083
```

adb launches Claude Code in the task's worktree (or, for a repo-less task, in its ticket dir). Add `--here` to `exec claude` in the current terminal instead of handing off to a new VS Code terminal:

```bash
adb task resume TASK-00083 --here
```

`resume` only flips a task from `backlog` → `in_progress`; it is a no-op on the status of a task that is already active, and it refuses archived tasks. See `newTaskResumeCmd` in [`internal/cli/task.go`](../internal/cli/task.go).

### 4d. Update as you work

```bash
adb task update TASK-00083 --status review
adb task priority TASK-00083 --priority P1
```

`adb task update` sets any of `--status` / `--priority` / `--owner`. `adb task priority` **requires** `--priority` and accepts multiple task IDs at once.

### 4e. Close it out

Mark the task done:

```bash
adb task update TASK-00083 --status done
```

When you are completely finished and want to reclaim the worktree, archive it:

```bash
adb task archive TASK-00083
```

`archive` moves the ticket dir into `tickets/_archived/` (preserving its nested sub-path), writes a `handoff.md`, and **removes the worktree**. Use `--force` to archive even if errors occur.

> **Heads-up:** `--keep-worktree` is **not implemented** — it prints a warning and removes the worktree anyway. Don't rely on it to preserve a worktree.

If you only want to reclaim the worktree but keep the ticket data, use `adb task cleanup TASK-00083` (removes the worktree only). To bring an archived task back, `adb task unarchive TASK-00083` moves it back to `backlog`.

### Bulk operations

```bash
adb task start-all -y     # promote every backlog task → in_progress
adb task close-all -y     # flip every active (in_progress/blocked/review) task → done
```

Both prompt for confirmation unless you pass `-y`/`--yes`. Note `close-all` only flips statuses — it does **not** archive tasks or remove worktrees, and it is reversible via `adb task update --status`.

---

## 5. VS Code extension setup

The extension (**"AI Dev Brain"**, publisher `valter-silva-au`, id `adb-brain`, **v0.4.0**) is a thin front-end: every action shells out to the `adb` CLI. It gives you a tickets tree view, one-click task terminals, and an in-editor dashboard. It requires VS Code ^1.85.0.

### Install

Build and install the VSIX from [`vscode-extension/`](../vscode-extension/):

```bash
cd vscode-extension
npm install
npm run compile
npx @vscode/vsce package        # produces adb-brain-0.4.0.vsix
code --install-extension adb-brain-0.4.0.vsix
```

### Configure

Set these two settings first (Settings → search "adb", or `settings.json`):

- **`adb.binaryPath`** (default `adb`) — path to the `adb` binary if it is not on your `PATH`.
- **`adb.home`** (default empty) — your workspace root. The extension threads this to the CLI as `ADB_HOME`; if empty it falls back to the first open workspace folder.

```jsonc
// .vscode/settings.json
{
  "adb.binaryPath": "adb",
  "adb.home": "/Users/you/my-workspace"
}
```

### Key commands

Open the Command Palette and type "ADB":

- **ADB: Start All Tasks** (`adb.startAll`) — opens one terminal per launchable task, grouped by org, each running `adb task resume <id> --here`. Grouping/order/cap are controlled by the `adb.startAll.*` settings below — this command does **not** call `adb task start-all`.
- **ADB: Create Task** — prompts for title/type/priority and shells `adb task create`.
- **ADB: Task Status** — shells `adb task status` into the Output panel.
- **ADB: Open Dashboard** (`adb.openDashboard`) — opens the **in-editor webview dashboard** (a VS Code `WebviewPanel`). It streams the live event feed from `adb events tail --follow --json`, falling back to tailing `<ADB_HOME>/.events.jsonl` if that subcommand isn't available. There is **no browser and no local web server** — the dashboard is entirely in-editor.

### Start-All configuration keys

Six `adb.startAll.*` / `adb.tmux.*` keys tune the multi-terminal Start-All flow, plus the two settings above and one dashboard key — 9 in total:

| Key | Default | Meaning |
|-----|---------|---------|
| `adb.startAll.groupByOrg` | `true` | Group task terminals under per-org divider terminals. |
| `adb.startAll.orderByPriority` | `true` | Order launched tasks by priority. |
| `adb.startAll.cap` | `0` | Max terminals to open (`0` = unlimited). |
| `adb.startAll.orgOrder` | `[]` | Explicit org ordering (`[]` = alpha, `_local` last). |
| `adb.tmux.enabled` | `true` | Host task terminals in tmux so they survive VS Code reloads. |
| `adb.tmux.sessionPrefix` | `cc-` | tmux session-name prefix. |
| `adb.binaryPath` | `adb` | Path to the `adb` binary. |
| `adb.home` | `""` | Workspace root (threaded as `ADB_HOME`). |
| `adb.dashboard.maxFeedItems` | `500` | Max feed items retained in the dashboard (min 1). |

> Terminal survivability across a VS Code reload also depends on `tmux` being on your `PATH` and VS Code's `terminal.integrated.enablePersistentSessions` being enabled.

---

## 6. Where to go next

You now know the loop. The **learning tiers** go deeper, each building on the last:

- **[L100 — Fundamentals](learning/L100-fundamentals.md):** the workspace model, task lifecycle, and daily commands in depth.
- **[L200 — Daily workflows](learning/L200-daily-workflows.md):** the VS Code extension end-to-end, sessions, sync, and observability (`adb events`, `adb metrics`, `adb chat`).
- **[L300 — Integrations](learning/L300-integrations.md):** GitHub/GitLab issue sync (`adb sync issues`), cloud archive (`adb sync cloud`), and the MCP server (`adb mcp serve`).
- **[L400 — Architecture & extending](learning/L400-architecture-and-extending.md):** architecture, the layered package design, the correlation layout, and extending adb.

Reference material:

- **[README.md](../README.md)** — architecture overview and package structure.
- **[docs/architecture/](architecture/)** — design docs.
- Full command surface: [`internal/cli/root.go:NewRootCmd`](../internal/cli/root.go) is the authoritative registration of every top-level command.

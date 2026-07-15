# AI Dev Brain — VS Code extension

Manage [AI Dev Brain (`adb`)](https://github.com/valter-silva-au/ai-dev-brain)
tickets from VS Code: a command-palette namespace, a tickets tree view, and
**survivable, tmux-hosted terminal tabs** for `adb` task launches that auto-
reattach after a window reload.

## Command palette (`ADB:` namespace)

Open the palette (`Ctrl/Cmd+Shift+P`) and type **ADB**:

| Command | Runs |
|---|---|
| ADB: Start All Tasks | `adb task start-all -y` |
| ADB: Close All Tasks | `adb task close-all -y` |
| ADB: Task Status | `adb task status` (Output panel) |
| ADB: Create Task | `adb task create` (prompts title/type/priority) |
| ADB: Start Task | `adb task resume <pick>` |
| ADB: Close Task | `adb task update <pick> --status=done` |
| ADB: Update Task Status | `adb task update <pick> --status=<pick>` |
| ADB: Open Dashboard | `adb serve` + opens `http://localhost:<port>` |
| ADB: Show Metrics | `adb metrics` (Output panel) |
| ADB: Show Alerts | `adb alerts` (Output panel) |
| ADB: Refresh Tickets | reloads the tree |
| ADB: New Claude Session | opens a fresh Claude (`ctrl+shift+\``); survivable on POSIX, bare on Windows |

## Tickets tree view

The **AI Dev Brain** activity-bar icon opens a Tickets panel grouping tasks by
status (reads `adb task status --json`). Per-ticket inline actions: start, close.
Title-bar actions: refresh, create, start-all, close-all.

## Survivable terminals (tmux) & reload auto-reattach

Every task terminal — whether opened from the tree view, **Start All**, or the
legacy launch-file path — runs `adb task resume <id> --here`, which the `adb`
binary hosts inside a **survivable tmux session** named `cc-<basename>` (the
ticket's worktree dir, or its ticket dir for repo-less tickets). Because tmux is
an independent daemon, the Claude session outlives a VS Code reload / quit /
crash.

Terminals are created with `shellPath`/`shellArgs` (not `sendText`) so VS Code's
**terminal revival re-runs the launch command on window reload** — VS Code
replays `shellArgs`, never `sendText`. Since `adb task resume --here` is
idempotent (`tmux new-session -A -D` = attach-or-create), the revived tab
**reattaches to the same live session** instead of spawning a duplicate. Requires
the user's `terminal.integrated.enablePersistentSessions` on (and
`persistentSessionReviveProcess` = `onExitAndWindowClose` to also cover a full
window reload).

### `ctrl+shift+\`` — a new survivable Claude each press

**ADB: New Claude Session** (bound to `ctrl+shift+\``) opens a fresh,
independent Claude. Each press picks the lowest-free ad-hoc index from the
open/revived terminals so presses never collapse onto one shared session.

- **POSIX** launches `cc-survive adhoc-<n>`, and because the command lives in
  `shellArgs`, each tab auto-reattaches to its own **survivable** `cc-adhoc-<n>`
  session on reload. (This is the `cc-survive` wrapper; the deterministic per-index
  name is what makes both independence *and* revival work.)
- **Windows** launches a bare `claude --dangerously-skip-permissions` in the
  PowerShell host instead — `cc-survive`/tmux is Unix-only (and MSYS tmux can't give
  claude a console pty on Windows), so the session is **non-survivable** (a reload
  starts fresh). This mirrors how task terminals degrade on Windows, and replaces the
  old behaviour where the press ran `cc-survive` in PowerShell and left a dead tab.

## Settings

| Setting | Default | Purpose |
|---|---|---|
| `adb.binaryPath` | `adb` | Path to the `adb` binary (use an absolute path if not on VS Code's PATH). |
| `adb.home` | _(empty)_ | `ADB_HOME` workspace path. Defaults to the first workspace folder. |
| `adb.startAll.groupByOrg` | `true` | Group **Start All Tasks** terminals under a per-org divider. |
| `adb.startAll.orderByPriority` | `true` | Order tasks P0 → P3 within each group. |
| `adb.startAll.cap` | `0` | Max terminals per **Start All** click (`0` = unlimited). |
| `adb.startAll.orgOrder` | `[]` | Explicit org ordering; empty = alphabetical, `_local` last. |
| `adb.tmux.enabled` | `true` | Host each Claude session in tmux (survives reload). **Off = bare claude, NON-durable — the session dies on reload.** |
| `adb.tmux.sessionPrefix` | `cc-` | tmux session-name prefix (session = `<prefix><sanitized-basename>`). Match `cc-survive` to share sessions with the `ctrl+shift+\`` profile. |

## Requirements

- The `adb` binary installed (`make install-local` in the adb repo →
  `~/.local/bin/adb`). Requires `adb task status --json` (v0.2.0+ of adb) and the
  tmux-hosted `adb task resume --here` launch path (adb ≥ commit `3110936`).
- `tmux` on `PATH` for survivable sessions (falls back to a plain direct launch
  if absent). On POSIX, `cc-survive` on `PATH` for the `ctrl+shift+\`` ad-hoc
  sessions; on Windows the ad-hoc press launches a bare `claude` (no cc-survive/tmux)
  and only needs `claude` on `PATH`.

## Building & installing

```sh
cd vscode-extension
npm install
npm test                       # 180 unit tests, 100% coverage on the logic modules
npx @vscode/vsce package --allow-missing-repository --skip-license
code --install-extension adb-brain-<version>.vsix --force
```

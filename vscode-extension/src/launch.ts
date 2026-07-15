// Pure decision + composition logic for the "Start All" flow and related
// launchability checks. No vscode/fs imports — fs lookups are injected as
// predicates so this module can be unit-tested outside the extension host.
//
// The extension's runtime glue (extension.ts) calls these to decide WHAT to do
// with the tasks it has; it then performs the side effects (createTerminal,
// showInformationMessage) on top of these decisions.

// Minimal task shape these helpers need. The runtime AdbTask in extension.ts
// is a strict superset; we keep the local shape narrow so this module stays
// independently importable in tests.
export interface LaunchTask {
  id: string;
  status: string;
  type?: string;
  priority?: string;
  worktree_path?: string;
  repo?: string;
  ticket_path?: string;
}

// Plan returned by planStartAll: the slice of tasks to launch plus the counts
// the toast wants to report.
export interface StartAllPlan {
  toStart: LaunchTask[];
  launchable: number;
  alreadyOpen: number;
  deferred: number;
  skipped: number;
}

// isLaunchable answers: can we start a Claude session for this task right now?
//   - existing worktree on disk → yes (we cd into it)
//   - no worktree but a recorded repo → yes (adb resume will clone+branch)
//   - no worktree/repo but a ticket dir → yes (open claude in the ticket dir)
//   - none → no (nothing to launch from)
//
// `worktreeExists` is injected so tests don't have to touch the filesystem.
export function isLaunchable(
  task: LaunchTask,
  worktreeExists: (p: string) => boolean,
): boolean {
  if (task.worktree_path && worktreeExists(task.worktree_path)) {
    return true;
  }
  return !!task.repo || !!task.ticket_path;
}

// resolveCwd picks the directory a task's terminal should start in.
//   - existing worktree → run from there
//   - else a ticket dir → run from the ticket dir (repo-less / no-worktree
//     tasks: claude opens where the planning docs live)
//   - otherwise → fall back to adbHome (`resume` creates the worktree first,
//     then claude runs in it; the initial cwd just needs to be valid)
//
// Returns undefined when none is usable (caller decides what to do).
export function resolveCwd(
  task: LaunchTask,
  worktreeExists: (p: string) => boolean,
  adbHome: string | undefined,
): string | undefined {
  if (task.worktree_path && worktreeExists(task.worktree_path)) {
    return task.worktree_path;
  }
  if (task.ticket_path) {
    return task.ticket_path;
  }
  return adbHome;
}

// Terminal statuses: tasks in these states are finished and are never opened
// by Start All. Everything else (backlog, in_progress, review, blocked) is an
// "active" ticket the user may still want a terminal for.
const TERMINAL_STATUSES = new Set(["done", "archived"]);

// planStartAll is the pure decision pipeline for `Start All`. Given the full
// task list, a predicate for which tasks already have an open terminal, and
// the per-click cap, it returns the slice we should actually launch plus the
// counts the toast wants to report.
//
// Order matters: we filter to active (non-terminal) → launchable →
// not-already-open, then cap. Already-open tasks are NOT counted toward the
// cap (they're free — we only reveal them, which Start All doesn't even do;
// this handler skips them).
//
// cap <= 0 means UNLIMITED — launch every not-already-open launchable task in
// one click (no per-click ceiling). A positive cap limits to that many.
//
// "active" = any non-terminal status: backlog, in_progress, review, blocked.
// Only done/archived are excluded — a click opens a terminal for every ticket
// you might still be working, not just the untouched backlog.
export function planStartAll(
  tasks: LaunchTask[],
  hasOpenTerminal: (t: LaunchTask) => boolean,
  worktreeExists: (p: string) => boolean,
  cap: number,
): StartAllPlan {
  const active = tasks.filter((t) => !TERMINAL_STATUSES.has(t.status));
  const launchable = active.filter((t) => isLaunchable(t, worktreeExists));
  const skipped = active.length - launchable.length;
  const notOpen = launchable.filter((t) => !hasOpenTerminal(t));
  const alreadyOpen = launchable.length - notOpen.length;
  // cap <= 0 → unlimited (start them all).
  const toStart = cap > 0 ? notOpen.slice(0, cap) : notOpen;
  const deferred = notOpen.length - toStart.length;
  return {
    toStart,
    launchable: launchable.length,
    alreadyOpen,
    deferred,
    skipped,
  };
}

// composeStartAllToast picks the right info-toast message for the outcome of
// a Start All click. Pure given the plan + how many we actually started (the
// caller may have created fewer terminals than planned, e.g. if a duplicate
// raced in between plan and create).
//
// Branches:
//   - launchable === 0 → "no launchable tasks" (with a skipped note)
//   - started === 0 with notes → "nothing new to start" (everything was open
//     or deferred or skipped)
//   - otherwise → "started N task(s) in terminals" with the notes list
export function composeStartAllToast(
  plan: StartAllPlan,
  started: number,
): string {
  if (plan.launchable === 0) {
    if (plan.skipped > 0) {
      return `ADB: no launchable tasks (${plan.skipped} task(s) have no worktree, repo, or ticket dir to launch)`;
    }
    return "ADB: no launchable tasks";
  }
  const notes: string[] = [];
  if (plan.deferred > 0) {
    notes.push(`${plan.deferred} more launchable (re-run to continue)`);
  }
  if (plan.alreadyOpen > 0) {
    notes.push(`${plan.alreadyOpen} already open`);
  }
  if (plan.skipped > 0) {
    notes.push(`${plan.skipped} skipped (no worktree/repo)`);
  }
  const noteStr = notes.length ? ` — ${notes.join(", ")}` : "";
  if (started === 0 && notes.length) {
    return `ADB: nothing new to start${noteStr}`;
  }
  return `ADB: started ${started} task(s) in terminals${noteStr}`;
}

// composeTaskLaunchCommand returns the shell line a task's terminal should run.
//
// ALWAYS `adb task resume <id> --here`, for every launchable task — worktree,
// repo, AND ticket-only. `--here` makes the binary exec claude in THIS terminal
// (no shared ~/.adb_terminal_launch.json race), and adb's launchClaudeCode hosts
// that claude inside a survivable `tmux new-session -A -D -s cc-<basename>`
// session keyed to the launch dir (worktree → ticket dir → adbHome). Resume is
// idempotent (an already-in_progress task is a no-op status-wise), so re-running
// this on VS Code revival reattaches to the SAME live tmux session.
//
// Historical note: ticket-only tasks used to launch bare `claude` here, which ran
// as a child of VS Code's pty-host (NOT tmux) and died on reload. The adb CLI now
// falls back to the ticket dir for repo-less tasks (task.go) and tmux-hosts them,
// so the bare-claude special case is gone — every ticket gets its own
// reattachable cc-TASK-NNNNN session.
export function composeTaskLaunchCommand(
  task: LaunchTask,
  adbBinary: string,
): string {
  return `${adbBinary} task resume ${task.id} --here`;
}

// resolveLoginShell picks the shell used to host a task terminal's launch
// command. On POSIX it prefers $SHELL (the user's real login shell — zsh here),
// falling back to /bin/bash when unset. On Windows the Unix login-shell model
// doesn't apply and "/bin/bash" is not a valid shellPath, so it hosts the launch
// in PowerShell (always present) — this is the extension half of the Windows
// launch fix started in the CLI by #214 (see #227). env + platform are injected
// so it stays pure/testable.
export function resolveLoginShell(
  env: { SHELL?: string },
  platform: NodeJS.Platform,
): string {
  if (platform === "win32") {
    return "powershell.exe";
  }
  return env.SHELL || "/bin/bash";
}

// composeShellArgs builds the argv for the login shell that hosts a task's launch
// command, so the terminal's PROCESS is `<shell> -l -c "<cmd>; exec <shell> -l"`
// rather than a bare shell fed via sendText.
//
// Why this matters: VS Code only revives a terminal by RE-RUNNING its
// shellPath/shellArgs after a window reload — text sent via Terminal.sendText is
// NOT replayed. Encoding the launch command in shellArgs is therefore what makes
// the terminal auto-reattach to its live tmux session on reload (the resume is
// idempotent, so this attaches rather than duplicates).
//
//   -l  → login shell, so ~/.zshenv/.zprofile (→ env.sh) load and the model/
//         Claude env is present even on a GUI cold-start (defends against 403s,
//         mirroring cc-survive).
//   -c  → run the launch command, then `exec <shell> -l` drops to a usable login
//         shell after claude/tmux detaches so the VS Code tab stays alive instead
//         of closing (mirrors cc-survive's `exec "$SHELL" -l` tail).
//
// SECURITY: the `-c` string is NOT shell-escaped. `cmd` MUST come from a trusted
// composer (composeTaskLaunchCommand / composeAdhocCommand) over known-safe tokens
// (adb binary path from settings + a TASK-\d+ id, or a fixed `cc-survive adhoc-<n>`
// / `claude --dangerously-skip-permissions` literal).
// Never pass raw end-user text here. (A hostile `adb.binaryPath` setting could
// inject, but editing workspace settings already grants arbitrary execution via
// tasks.json, so this is not a new trust boundary.)
export function composeShellArgs(
  cmd: string,
  shell: string,
  platform: NodeJS.Platform,
): string[] {
  if (platform === "win32") {
    // PowerShell runs the command then drops to an interactive prompt
    // (-NoExit), so the tab stays alive after claude detaches and VS Code
    // re-runs cmd (idempotent) on reload — the Windows analogue of the POSIX
    // `<cmd>; exec <shell> -l` tail. shell is unused here (always PowerShell).
    return ["-NoExit", "-Command", cmd];
  }
  return ["-l", "-c", `${cmd}; exec ${shell} -l`];
}

// ===========================================================================
// Ad-hoc survivable Claude sessions (ctrl+shift+` — "new Claude each press")
// ===========================================================================

// ADHOC_NAME_RE matches the display name of an ad-hoc Claude terminal:
// "🌙 claude" (index 1) or "🌙 claude <n>" (index n). The leading emoji + label
// is fixed; only the trailing integer varies. Used to find which ad-hoc indices
// are already taken so the next press picks a free one — INCLUDING terminals
// revived after a window reload (they're back in window.terminals with the same
// name), so numbering continues cleanly instead of colliding.
const ADHOC_NAME_RE = /^🌙 claude(?: (\d+))?$/;

// adhocIndexFromName returns the ad-hoc index encoded in a terminal display name,
// or undefined if the name is not an ad-hoc Claude terminal. "🌙 claude" → 1,
// "🌙 claude 2" → 2.
export function adhocIndexFromName(name: string): number | undefined {
  const m = ADHOC_NAME_RE.exec(name);
  if (!m) {
    return undefined;
  }
  return m[1] === undefined ? 1 : parseInt(m[1], 10);
}

// nextAdhocIndex returns the smallest positive integer not already used by an
// open/revived ad-hoc Claude terminal. Pure over the list of current terminal
// names so it's unit-testable without the VS Code runtime.
export function nextAdhocIndex(existingNames: string[]): number {
  const used = new Set<number>();
  for (const n of existingNames) {
    const i = adhocIndexFromName(n);
    if (i !== undefined) {
      used.add(i);
    }
  }
  let i = 1;
  while (used.has(i)) {
    i++;
  }
  return i;
}

// adhocDisplayName / adhocSessionArg map an index to the terminal's display name
// and the raw session arg passed to cc-survive. cc-survive prepends "cc-" and
// sanitizes, so arg "adhoc-3" → tmux session "cc-adhoc-3". Index 1 has no suffix
// in the display name (just "🌙 claude") but a stable session arg "adhoc-1", so
// every ad-hoc terminal maps to a deterministic, unique session that survives
// reload (cc-survive attach-or-creates on the same name).
export function adhocDisplayName(index: number): string {
  return index === 1 ? "🌙 claude" : `🌙 claude ${index}`;
}
export function adhocSessionArg(index: number): string {
  return `adhoc-${index}`;
}

// ADHOC_WINDOWS_COMMAND is the ad-hoc launch on Windows: a bare claude. cc-survive
// (a Unix/tmux wrapper) cannot run under PowerShell, and MSYS tmux denies claude a
// console pty on Windows — so there is no survivable session to attach to. We run
// claude directly instead, mirroring the CLI's task-launch fallback on win32
// (shouldUseTmux → false, then a direct claude) and its claudeArgs (see
// internal/cli/launch.go: `--dangerously-skip-permissions`).
const ADHOC_WINDOWS_COMMAND = "claude --dangerously-skip-permissions";

// composeAdhocCommand returns the command an ad-hoc Claude terminal runs, per
// platform (mirroring resolveLoginShell/composeShellArgs, which also branch on it):
//
//   - POSIX   → `cc-survive <sessionArg>`. cc-survive is an attach-or-create wrapper
//     (session name = "cc-" + sanitize(arg)), so passing a UNIQUE arg per press
//     yields a unique, independent, survivable session — and re-running the SAME
//     arg on revival reattaches to it (no collapse onto one shared session, no
//     orphans). Run via a login shell (see composeShellArgs) so cc-survive resolves
//     on PATH and env.sh loads; if cc-survive is absent the login shell still drops
//     to a usable prompt (graceful degradation).
//   - Windows → bare `claude --dangerously-skip-permissions` (ADHOC_WINDOWS_COMMAND).
//     cc-survive/tmux is Unix-only, so the session is non-survivable — the same
//     degradation the CLI applies for task terminals on Windows. Before this the
//     press ran `cc-survive …` in PowerShell, which just errored (command not
//     found) and left a dead tab. The sessionArg is unused (no tmux session to
//     name); numbering/naming still comes from adhocDisplayName/nextAdhocIndex so
//     the tab still gets a unique "🌙 claude <n>" label and doesn't collide.
export function composeAdhocCommand(
  sessionArg: string,
  platform: NodeJS.Platform,
): string {
  if (platform === "win32") {
    return ADHOC_WINDOWS_COMMAND;
  }
  return `cc-survive ${sessionArg}`;
}

// composeCloseTerminalsToast picks the toast wording for the Close Terminals
// command. Pure: just the count → message.
export function composeCloseTerminalsToast(closedCount: number): string {
  if (closedCount > 0) {
    return `ADB: closed ${closedCount} task terminal(s)`;
  }
  return "ADB: no task terminals open";
}

// selectVictimNames returns the subset of terminal names that are adb task
// terminals (the ones Close Terminals should dispose). Pure wrapper that
// preserves order — exported for tests so we can assert non-task terminals
// like `toolbox-exec` and `ADB Dashboard` are NEVER selected.
export function selectVictimNames(
  names: string[],
  isAnyTaskTerminalFn: (name: string) => boolean,
): string[] {
  return names.filter(isAnyTaskTerminalFn);
}

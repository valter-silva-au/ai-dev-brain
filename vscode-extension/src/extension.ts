import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import { execFile } from "child_process";
import {
  taskTerminalName,
  isTaskTerminal,
  isAnyTaskTerminal,
  orgHeaderName,
  orgHeaderBannerCommand,
} from "./terminals";
import { planStartAllGrouped, StartAllGroupConfig } from "./orggroup";
import { normalizeStartAllConfig, normalizeTmuxConfig } from "./config";
import { eventsLogPath } from "./feedpath";
import {
  composeStartAllToast,
  composeTaskLaunchCommand,
  composeCloseTerminalsToast,
  composeShellArgs,
  resolveLoginShell,
  resolveCwd,
  nextAdhocIndex,
  adhocDisplayName,
  adhocSessionArg,
  composeAdhocCommand,
  LaunchTask,
} from "./launch";
import {
  parseLaunchRequest,
  isStale,
  composeLaunchTerminalName,
} from "./launchRequest";
import { iconForType } from "./icons";
import {
  renderPanelHtml,
  makeNonce,
  PanelToWebviewMessage,
  ChatProposal,
} from "./webview/panel";
import { buildOverview } from "./webview/overview";
import { splitLines, FeedSource, Disposable } from "./webview/source";
import { parseSteerActions, summarize, SteerAction } from "./webview/chat";
import {
  lower,
  describeRejection,
  isPathInside,
  LowerContext,
} from "./webview/actions";
import { spawn, ChildProcess } from "child_process";

// ===========================================================================
// Shared adb invocation
// ===========================================================================

interface AdbTask extends LaunchTask {
  id: string;
  title: string;
  type: string;
  status: string;
  priority: string;
  owner?: string;
  tags?: string[];
  repo?: string;
  worktree_path?: string;
  ticket_path?: string;
}

const STATUS_ORDER = [
  "in_progress",
  "review",
  "blocked",
  "backlog",
  "done",
  "archived",
];

const STATUS_ICONS: Record<string, string> = {
  in_progress: "play-circle",
  review: "eye",
  blocked: "error",
  backlog: "circle-outline",
  done: "pass-filled",
  archived: "archive",
};

const PRIORITY_COLORS: Record<string, string> = {
  P0: "terminal.ansiRed",
  P1: "terminal.ansiYellow",
  P2: "terminal.ansiCyan",
  P3: "terminal.ansiWhite",
};

function getConfig(): vscode.WorkspaceConfiguration {
  return vscode.workspace.getConfiguration("adb");
}

function adbBinary(): string {
  return getConfig().get<string>("binaryPath", "adb") || "adb";
}

function adbHome(): string | undefined {
  const configured = getConfig().get<string>("home", "");
  if (configured) {
    return configured;
  }
  const folders = vscode.workspace.workspaceFolders;
  if (folders && folders.length > 0) {
    return folders[0].uri.fsPath;
  }
  return undefined;
}

function adbEnv(): NodeJS.ProcessEnv {
  const env = { ...process.env };
  const home = adbHome();
  if (home) {
    env.ADB_HOME = home;
  }
  // Thread tmux config to the CLI. adb.tmux.enabled=false → ADB_TMUX=0 →
  // shouldUseTmux() returns false → bare (non-durable) claude launch.
  // adb.tmux.sessionPrefix flows through ADB_TMUX_PREFIX; the CLI re-runs
  // the same [A-Za-z0-9_-] sanitizer on the far end (defence in depth).
  const tmux = tmuxConfig();
  env.ADB_TMUX = tmux.enabled ? "1" : "0";
  env.ADB_TMUX_PREFIX = tmux.sessionPrefix;
  return env;
}

// runAdb invokes the adb binary with the given args and resolves with stdout.
// Rejects with a readable error (including stderr) on non-zero exit.
function runAdb(args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      adbBinary(),
      args,
      { env: adbEnv(), cwd: adbHome(), maxBuffer: 10 * 1024 * 1024 },
      (err, stdout, stderr) => {
        if (err) {
          const detail = (stderr || stdout || err.message).trim();
          reject(new Error(detail));
          return;
        }
        resolve(stdout);
      },
    );
  });
}

async function listTasks(): Promise<AdbTask[]> {
  const out = await runAdb(["task", "status", "--json"]);
  try {
    const parsed = JSON.parse(out);
    return Array.isArray(parsed) ? (parsed as AdbTask[]) : [];
  } catch {
    return [];
  }
}

// Run an adb mutation, then surface the result and refresh the tree.
async function runAndReport(
  args: string[],
  successMessage: string,
  refresh: () => void,
): Promise<void> {
  try {
    await runAdb(args);
    vscode.window.showInformationMessage(successMessage);
    refresh();
  } catch (e) {
    vscode.window.showErrorMessage(
      `adb ${args.join(" ")} failed: ${(e as Error).message}`,
    );
  }
}

// findOpenTaskTerminal returns an already-open terminal for the task, if any.
function findOpenTaskTerminal(task: AdbTask): vscode.Terminal | undefined {
  return vscode.window.terminals.find((t) => isTaskTerminal(t.name, task.id));
}

// Open a styled terminal for a single task and resume it IN that terminal.
// If a terminal for this task is already open, reveal it instead of creating a
// duplicate (returns false = nothing newly created).
//
// The terminal's PROCESS is `<login-shell> -l -c "adb task resume <id> --here;
// exec <shell> -l"` — the launch command lives in shellArgs, NOT in a post-hoc
// sendText. This is what makes the terminal AUTO-REVIVE after a VS Code window
// reload: VS Code re-runs a persisted terminal's shellPath/shellArgs (it does
// not replay sendText), and because `adb task resume --here` is idempotent and
// tmux-hosts claude in `cc-<basename>` via attach-or-create, the revived run
// reattaches to the SAME live session instead of spawning a duplicate. Every
// task (worktree, repo, ticket-only) gets its own survivable cc-TASK-NNNNN.
function openTaskTerminal(task: AdbTask, parent?: vscode.Terminal): boolean {
  const existing = findOpenTaskTerminal(task);
  if (existing) {
    existing.show();
    return false;
  }
  const iconName = iconForType(task.type);
  const colorName = PRIORITY_COLORS[task.priority] || "terminal.ansiCyan";
  const cwd = resolveCwd(task, fs.existsSync, adbHome());
  const shell = resolveLoginShell(process.env, process.platform);
  const options: vscode.TerminalOptions = {
    name: taskTerminalName(task.id, task.type, task.priority),
    iconPath: new vscode.ThemeIcon(iconName),
    color: new vscode.ThemeColor(colorName),
    cwd,
    env: adbEnv(),
    shellPath: shell,
    shellArgs: composeShellArgs(
      composeTaskLaunchCommand(task, adbBinary()),
      shell,
      process.platform,
    ),
  };
  // When launched under an org header (Start-All grouping), split into the
  // header's terminal group so VS Code clusters the org's terminals together.
  if (parent) {
    options.location = { parentTerminal: parent };
  }
  const terminal = vscode.window.createTerminal(options);
  terminal.show();
  return true;
}

// openOrgHeaderTerminal creates a cheap divider terminal "━━ <org> (<n>) ━━" and
// returns it so the org's ticket terminals can be split into the SAME group via
// location.parentTerminal. Not tmux-hosted — it's a disposable visual marker,
// and deliberately not matched by isAnyTaskTerminal (Close Terminals leaves it).
function openOrgHeaderTerminal(org: string, count: number): vscode.Terminal {
  const shell = resolveLoginShell(process.env, process.platform);
  const terminal = vscode.window.createTerminal({
    name: orgHeaderName(org, count),
    iconPath: new vscode.ThemeIcon("organization"),
    color: new vscode.ThemeColor("terminal.ansiBlue"),
    cwd: adbHome(),
    env: adbEnv(),
    shellPath: shell,
    shellArgs: composeShellArgs(
      orgHeaderBannerCommand(org, count),
      shell,
      process.platform,
    ),
  });
  terminal.show();
  return terminal;
}

// startAllConfig reads Start-All grouping config. The package.json
// contributes.configuration keys back these getter calls; the pure
// normalizer in ./config clamps cap to a non-negative integer, defensively
// slices orgOrder, and defaults any missing/malformed field. Kept in the
// glue (not the pure planner) so launch.ts/orggroup.ts stay config-free.
function startAllConfig(): StartAllGroupConfig {
  const c = getConfig();
  return normalizeStartAllConfig({
    cap: c.get<number>("startAll.cap"),
    groupByOrg: c.get<boolean>("startAll.groupByOrg"),
    orderByPriority: c.get<boolean>("startAll.orderByPriority"),
    orgOrder: c.get<string[]>("startAll.orgOrder"),
  });
}

// tmuxConfig reads the tmux hardening settings. sessionPrefix is trimmed
// and sanitized to [A-Za-z0-9_-] so a hostile prefix cannot inject into
// the tmux argv; empty/whitespace falls back to "cc-". The Go CLI
// re-sanitizes on the far end (defence in depth).
function tmuxConfig() {
  const c = getConfig();
  return normalizeTmuxConfig({
    enabled: c.get<boolean>("tmux.enabled"),
    sessionPrefix: c.get<string>("tmux.sessionPrefix"),
  });
}

// openAdhocClaude opens a NEW, independent Claude session — the `ctrl+shift+\``
// workflow. Each press picks the lowest free ad-hoc index from the currently-
// open/revived terminals (so sessions never collapse onto one shared name, and
// numbering reuses gaps), then launches the platform's ad-hoc command via
// shellArgs (composeAdhocCommand). On POSIX that is `cc-survive adhoc-<n>` — an
// attach-or-create on a deterministic name, so the terminal AUTO-REVIVES on window
// reload and reattaches to the SAME session (no duplicate, no orphan). On Windows
// cc-survive/tmux is unavailable, so it launches a bare claude (non-survivable —
// a reload starts a fresh session), which is at least a WORKING ad-hoc Claude
// instead of the `cc-survive: not found` dead tab that ran there before. Returns
// the chosen index (for tests/telemetry).
function openAdhocClaude(): number {
  const index = nextAdhocIndex(vscode.window.terminals.map((t) => t.name));
  const shell = resolveLoginShell(process.env, process.platform);
  const cmd = composeAdhocCommand(adhocSessionArg(index), process.platform);
  const terminal = vscode.window.createTerminal({
    name: adhocDisplayName(index),
    iconPath: new vscode.ThemeIcon("terminal-tmux"),
    color: new vscode.ThemeColor("terminal.ansiMagenta"),
    cwd: adbHome(),
    env: adbEnv(),
    shellPath: shell,
    shellArgs: composeShellArgs(cmd, shell, process.platform),
  });
  terminal.show();
  return index;
}

// closeTaskTerminals disposes every open adb task terminal (TASK-NNNNN ...).
// This kills the claude process running inside each. It does NOT change task
// status — use the /close skill or "Close Task" for the ticket lifecycle.
function closeTaskTerminals(): number {
  const victims = vscode.window.terminals.filter((t) =>
    isAnyTaskTerminal(t.name),
  );
  for (const t of victims) {
    t.dispose();
  }
  return victims.length;
}

// startAllInTerminals opens launchable tasks in terminals, grouped by org.
// Launchable = has an existing worktree, or a repo (worktree auto-created on
// resume). Per org it drops a "━━ <org> (n) ━━" divider terminal, then splits
// that org's ticket terminals beneath it (P0→P3), so VS Code clusters each org.
//
// The decision pipeline (filter → cap → group → order → toast wording) lives in
// ./launch + ./orggroup as pure functions; this function is just the vscode-side
// adapter that runs listTasks, calls planStartAllGrouped with the injected
// config, performs the createTerminal side effects, and shows the toast. Grouping
// and priority ordering are config-gated (adb.startAll.*, WS-D-backed; WS-C
// defaults on).
async function startAllInTerminals(refresh: () => void): Promise<void> {
  let tasks: AdbTask[];
  try {
    tasks = await listTasks();
  } catch (e) {
    vscode.window.showErrorMessage(
      `ADB: failed to list tasks: ${(e as Error).message}`,
    );
    return;
  }
  const cfg = startAllConfig();
  const plan = planStartAllGrouped(
    tasks,
    (t) => findOpenTaskTerminal(t as AdbTask) !== undefined,
    fs.existsSync,
    cfg,
  );
  if (plan.launchable === 0) {
    vscode.window.showInformationMessage(composeStartAllToast(plan, 0));
    return;
  }
  let started = 0;
  for (const group of plan.groups) {
    if (group.tasks.length === 0) {
      continue;
    }
    // Header terminal only when grouping is on (org is non-empty); the org's
    // ticket terminals then split beneath it into the same VS Code group.
    let parent: vscode.Terminal | undefined;
    if (cfg.groupByOrg && group.org) {
      parent = openOrgHeaderTerminal(group.org, group.tasks.length);
    }
    for (const t of group.tasks) {
      if (openTaskTerminal(t as AdbTask, parent)) {
        started++;
      }
    }
  }
  vscode.window.showInformationMessage(composeStartAllToast(plan, started));
  refresh();
}

// ===========================================================================
// Tickets tree view
// ===========================================================================

type TreeNode = StatusGroup | TaskItem;

class StatusGroup extends vscode.TreeItem {
  constructor(
    public readonly status: string,
    public readonly tasks: AdbTask[],
  ) {
    super(
      `${status.toUpperCase()} (${tasks.length})`,
      vscode.TreeItemCollapsibleState.Expanded,
    );
    this.iconPath = new vscode.ThemeIcon(
      STATUS_ICONS[status] || "circle-outline",
    );
    this.contextValue = "adbStatusGroup";
  }
}

class TaskItem extends vscode.TreeItem {
  constructor(public readonly task: AdbTask) {
    super(`${task.id}  ${task.title}`, vscode.TreeItemCollapsibleState.None);
    this.description = `${task.type} · ${task.priority}${
      task.owner ? ` · ${task.owner}` : ""
    }`;
    this.tooltip = [
      `${task.id}: ${task.title}`,
      `type: ${task.type}`,
      `status: ${task.status}`,
      `priority: ${task.priority}`,
      task.owner ? `owner: ${task.owner}` : "",
      task.tags && task.tags.length ? `tags: ${task.tags.join(", ")}` : "",
    ]
      .filter(Boolean)
      .join("\n");
    this.iconPath = new vscode.ThemeIcon(iconForType(task.type));
    this.contextValue = "adbTask";
  }
}

class TicketsProvider implements vscode.TreeDataProvider<TreeNode> {
  private readonly _onDidChangeTreeData = new vscode.EventEmitter<
    TreeNode | undefined | void
  >();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: TreeNode): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: TreeNode): Promise<TreeNode[]> {
    // Children of a status group are its tasks.
    if (element instanceof StatusGroup) {
      return element.tasks.map((t) => new TaskItem(t));
    }
    // Task items are leaves.
    if (element instanceof TaskItem) {
      return [];
    }
    // Root: group tasks by status in a stable order.
    let tasks: AdbTask[];
    try {
      tasks = await listTasks();
    } catch (e) {
      vscode.window.showErrorMessage(
        `ADB: failed to load tickets: ${(e as Error).message}`,
      );
      return [];
    }
    if (tasks.length === 0) {
      const empty = new vscode.TreeItem(
        "No tickets — create one with ADB: Create Task",
      );
      empty.iconPath = new vscode.ThemeIcon("info");
      return [empty as TreeNode];
    }

    const byStatus = new Map<string, AdbTask[]>();
    for (const t of tasks) {
      const list = byStatus.get(t.status) || [];
      list.push(t);
      byStatus.set(t.status, list);
    }

    const groups: TreeNode[] = [];
    for (const status of STATUS_ORDER) {
      const list = byStatus.get(status);
      if (list && list.length > 0) {
        groups.push(new StatusGroup(status, list));
      }
    }
    // Any unknown statuses not in STATUS_ORDER, appended last.
    for (const [status, list] of byStatus) {
      if (!STATUS_ORDER.includes(status)) {
        groups.push(new StatusGroup(status, list));
      }
    }
    return groups;
  }
}

// ===========================================================================
// Command handlers
// ===========================================================================

// Let the user pick a task via QuickPick; returns the task id or undefined.
async function pickTask(
  placeHolder: string,
  filter?: (t: AdbTask) => boolean,
): Promise<string | undefined> {
  let tasks: AdbTask[];
  try {
    tasks = await listTasks();
  } catch (e) {
    vscode.window.showErrorMessage(`ADB: ${(e as Error).message}`);
    return undefined;
  }
  const candidates = filter ? tasks.filter(filter) : tasks;
  if (candidates.length === 0) {
    vscode.window.showInformationMessage("ADB: no matching tickets.");
    return undefined;
  }
  const picked = await vscode.window.showQuickPick(
    candidates.map((t) => ({
      label: `${t.id}  ${t.title}`,
      description: `${t.status} · ${t.type} · ${t.priority}`,
      id: t.id,
    })),
    { placeHolder, matchOnDescription: true },
  );
  return picked?.id;
}

async function createTask(refresh: () => void): Promise<void> {
  const title = await vscode.window.showInputBox({
    prompt: "New ticket title (used as the branch/title)",
    placeHolder: "e.g. add-export-button",
  });
  if (!title) {
    return;
  }
  const type = await vscode.window.showQuickPick(
    ["feat", "bug", "spike", "refactor"],
    { placeHolder: "Task type" },
  );
  if (!type) {
    return;
  }
  const priority = await vscode.window.showQuickPick(["P0", "P1", "P2", "P3"], {
    placeHolder: "Priority",
  });
  if (!priority) {
    return;
  }
  await runAndReport(
    ["task", "create", title, `--type=${type}`, `--priority=${priority}`],
    `ADB: created task "${title}"`,
    refresh,
  );
}

async function updateStatus(refresh: () => void): Promise<void> {
  const id = await pickTask("Select a ticket to update");
  if (!id) {
    return;
  }
  const status = await vscode.window.showQuickPick(
    ["backlog", "in_progress", "blocked", "review", "done", "archived"],
    { placeHolder: `New status for ${id}` },
  );
  if (!status) {
    return;
  }
  await runAndReport(
    ["task", "update", id, `--status=${status}`],
    `ADB: ${id} → ${status}`,
    refresh,
  );
}

function showOutput(
  channel: vscode.OutputChannel,
  title: string,
  body: string,
): void {
  channel.clear();
  channel.appendLine(`=== ${title} ===`);
  channel.appendLine(body.trimEnd());
  channel.show(true);
}

// ===========================================================================
// Styled terminal launcher (preserved from v0.1.0)
// ===========================================================================

function handleLaunchRequest(launchFile: string): void {
  let data: string;
  try {
    data = fs.readFileSync(launchFile, "utf8");
  } catch {
    return;
  }
  const req = parseLaunchRequest(data);
  if (!req) {
    return;
  }
  if (isStale(req, Date.now())) {
    return;
  }
  try {
    fs.unlinkSync(launchFile);
  } catch {
    // ignore
  }
  // If a terminal for this task is already open (e.g. the user clicked the tree
  // item AND ran `adb task resume` manually), reveal it instead of duplicating.
  const existing = vscode.window.terminals.find((t) =>
    isTaskTerminal(t.name, req.task_id),
  );
  if (existing) {
    existing.show();
    return;
  }
  const iconName = iconForType(req.task_type);
  const colorName = PRIORITY_COLORS[req.priority] || "terminal.ansiCyan";
  const shell = resolveLoginShell(process.env, process.platform);
  // Converge the legacy launch-file path onto the SAME survivable, tmux-hosted,
  // revivable mechanism as openTaskTerminal: re-run `adb task resume <id> --here`
  // through shellArgs (idempotent → attaches to the live cc-<basename> session)
  // rather than firing a bare, non-survivable `claude` via sendText.
  // The minimal LaunchTask shape (id + status only) is safe: composeTaskLaunchCommand
  // reads only task.id — it always returns `adb task resume <id> --here` regardless
  // of worktree/repo/ticket fields, which we don't have in a LaunchRequest anyway.
  const cmd = composeTaskLaunchCommand(
    { id: req.task_id, status: req.status },
    adbBinary(),
  );
  const terminal = vscode.window.createTerminal({
    name: composeLaunchTerminalName(req),
    iconPath: new vscode.ThemeIcon(iconName),
    color: new vscode.ThemeColor(colorName),
    cwd: req.worktree_path,
    env: adbEnv(),
    shellPath: shell,
    shellArgs: composeShellArgs(cmd, shell, process.platform),
  });
  terminal.show();
}

// ===========================================================================
// Activation
// ===========================================================================

export function activate(context: vscode.ExtensionContext): void {
  const provider = new TicketsProvider();
  const refresh = () => provider.refresh();
  const output = vscode.window.createOutputChannel("AI Dev Brain");

  context.subscriptions.push(
    vscode.window.registerTreeDataProvider("adb.ticketsView", provider),
    output,
  );

  const register = (
    command: string,
    handler: (...args: unknown[]) => unknown,
  ) =>
    context.subscriptions.push(
      vscode.commands.registerCommand(command, handler),
    );

  // --- Bulk + status ---
  // Open one styled terminal per backlog task that has a worktree, running
  // `adb task resume <id> --here` so Claude Code launches IN that terminal.
  // Tasks without a worktree (or whose worktree path no longer exists) are
  // skipped — there's nowhere to launch a session. Sequential creation gives
  // one terminal per ticket with no shared-launch-file race.
  register("adb.startAll", () => startAllInTerminals(refresh));
  register("adb.closeAll", () =>
    runAndReport(
      ["task", "close-all", "-y"],
      "ADB: closed all active tasks",
      refresh,
    ),
  );
  // Dispose every open adb task terminal (kills the claude process inside).
  // Does NOT change task status — use the /close skill for the ticket lifecycle.
  register("adb.closeTerminals", () => {
    const n = closeTaskTerminals();
    vscode.window.showInformationMessage(composeCloseTerminalsToast(n));
  });
  // Open a fresh, independent, survivable Claude session. Bound to ctrl+shift+`
  // (see keybindings.json) so every press spawns a NEW cc-adhoc-<n> session
  // instead of collapsing onto one shared cc-<folder> session.
  register("adb.newClaude", () => {
    openAdhocClaude();
  });
  register("adb.taskStatus", async () => {
    try {
      const body = await runAdb(["task", "status"]);
      showOutput(output, "adb task status", body || "No tasks found");
    } catch (e) {
      vscode.window.showErrorMessage(`ADB: ${(e as Error).message}`);
    }
  });

  // --- Single-task lifecycle ---
  register("adb.createTask", () => createTask(refresh));
  register("adb.startTask", async (item?: unknown) => {
    const taskItem = item as TaskItem | undefined;
    const id =
      taskItem?.task?.id ??
      (await pickTask(
        "Select a ticket to start",
        (t) => t.status === "backlog",
      ));
    if (id) {
      await runAndReport(["task", "resume", id], `ADB: ${id} started`, refresh);
    }
  });
  register("adb.closeTask", async (item?: unknown) => {
    const taskItem = item as TaskItem | undefined;
    const id =
      taskItem?.task?.id ?? (await pickTask("Select a ticket to close"));
    if (id) {
      await runAndReport(
        ["task", "update", id, "--status=done"],
        `ADB: ${id} marked done`,
        refresh,
      );
    }
  });
  register("adb.updateStatus", () => updateStatus(refresh));

  // --- Dashboard + observability ---
  // adb.openDashboard replaces the old browser hand-off (adb serve + openExternal)
  // with an in-editor WebviewPanel that streams live events + shows per-org
  // overview cards. Read-only surface — chat/steer is F4 behind the ARCC gate.
  //
  // The panel is a singleton per extension host (revealed on repeat click).
  // Feed source selection is deferred to openDashboardWebview so the pure
  // panel scaffold + protocol stay vscode-free.
  register("adb.openDashboard", () => {
    openDashboardWebview(context);
  });
  register("adb.showMetrics", async () => {
    try {
      const body = await runAdb(["metrics"]);
      showOutput(output, "adb metrics", body);
    } catch (e) {
      vscode.window.showErrorMessage(`ADB: ${(e as Error).message}`);
    }
  });
  register("adb.showAlerts", async () => {
    try {
      const body = await runAdb(["alerts"]);
      showOutput(output, "adb alerts", body || "No active alerts");
    } catch (e) {
      vscode.window.showErrorMessage(`ADB: ${(e as Error).message}`);
    }
  });

  // --- Tree ---
  register("adb.refreshTickets", refresh);

  // --- Styled terminal launcher (preserved) ---
  const homeDir = process.env.HOME || "";
  if (homeDir) {
    const launchFile = path.join(homeDir, ".adb_terminal_launch.json");
    const watcher = fs.watch(homeDir, (_event, filename) => {
      if (filename === ".adb_terminal_launch.json") {
        handleLaunchRequest(launchFile);
      }
    });
    context.subscriptions.push({ dispose: () => watcher.close() });
    if (fs.existsSync(launchFile)) {
      handleLaunchRequest(launchFile);
    }
  }
}

// ===========================================================================
// Webview dashboard (F3 — read-only feed + per-org overview)
// ===========================================================================

// Singleton panel + its currently-attached feed source. Repeat clicks on
// "ADB: Open Dashboard" reveal the existing panel rather than duplicate it.
let dashboardPanel: vscode.WebviewPanel | undefined;
let dashboardFeed: Disposable | undefined;

// Chat proposals from the most recent reply, indexed by proposalId. The
// webview only ever sends back a numeric id via chat.confirm — the actual
// SteerAction stays extension-side, so a hostile webview payload cannot
// smuggle a fresh action past the allowlist. Cleared on every new reply.
let dashboardChatProposals: SteerAction[] = [];

function dashboardMaxFeedItems(): number {
  const raw = getConfig().get<number>("dashboard.maxFeedItems", 500);
  if (typeof raw !== "number" || !isFinite(raw) || raw < 1) {
    return 500;
  }
  return Math.floor(raw);
}

// Post the initial + refreshed overview to the webview. Failures are logged
// to the console — the panel still renders the feed even if listTasks fails.
async function pushOverview(panel: vscode.WebviewPanel): Promise<void> {
  try {
    const tasks = await listTasks();
    const cards = buildOverview(tasks);
    const msg: PanelToWebviewMessage = { kind: "overview", cards };
    await panel.webview.postMessage(msg);
  } catch (e) {
    console.error("adb: failed to build overview", e);
  }
}

// pickFeedSource picks the concrete FeedSource for this open. Preference
// order: `adb events tail --follow --json` (once F1 lands and is installed)
// → tail of .events.jsonl (works on any adb workspace). This is the ONE
// place F1 wiring plugs in — everything downstream consumes lines through
// the FeedSource interface.
function pickFeedSource(): FeedSource {
  // Command-source is preferred once available: adb owns the tail-loop
  // (`--follow` polls the file and emits appended lines as JSONL). We
  // detect availability by shelling out `adb events --help`; if it exits
  // 0, the subcommand exists and we use it. Detection is lazy — done at
  // start() time so opening the panel doesn't stall waiting on process
  // spawn. If detection fails or the process errors, we fall back to the
  // file-tail source.
  return {
    start(onLine, onError) {
      let disposed = false;
      let inner: Disposable | undefined;

      // Probe: `adb events --help` succeeds → command source; else fallback.
      execFile(
        adbBinary(),
        ["events", "--help"],
        { env: adbEnv(), timeout: 3000 },
        (err) => {
          if (disposed) {
            return;
          }
          if (err) {
            inner = fileTailFeedSource().start(onLine, onError);
          } else {
            inner = commandFeedSource().start(onLine, onError);
          }
        },
      );

      return {
        dispose() {
          disposed = true;
          if (inner) {
            inner.dispose();
          }
        },
      };
    },
  };
}

// commandFeedSource: spawn `adb events tail --follow --json` and forward
// stdout lines. F1 owns the CLI subcommand; this wiring is unchanged when
// F1 lands. On non-zero exit (e.g. a workspace with no .events.jsonl yet),
// onError fires once; the panel's existing "Waiting for events…" affordance
// stays visible.
function commandFeedSource(): FeedSource {
  return {
    start(onLine, onError) {
      let child: ChildProcess | undefined;
      try {
        child = spawn(adbBinary(), ["events", "tail", "--follow", "--json"], {
          env: adbEnv(),
          cwd: adbHome(),
        });
      } catch (e) {
        onError(e as Error);
        return { dispose() {} };
      }
      let held = "";
      child.stdout?.setEncoding("utf8");
      child.stdout?.on("data", (chunk: string) => {
        const r = splitLines(chunk, held);
        held = r.held;
        for (const line of r.lines) {
          if (line.length > 0) {
            onLine(line);
          }
        }
      });
      child.on("error", (err) => onError(err));
      child.on("exit", (code) => {
        if (code !== 0 && code !== null) {
          onError(new Error(`adb events tail exited with code ${code}`));
        }
      });
      return {
        dispose() {
          if (child && !child.killed) {
            try {
              child.kill();
            } catch {
              // ignore — the child may already be gone.
            }
          }
        },
      };
    },
  };
}

// fileTailFeedSource: poll <ADB_HOME>/.adb/events.jsonl for appended bytes and
// emit each new line (the log moved under .adb/ in adb #186; the path lives in
// the pure feedpath module, in lockstep with the Go internal/statedir seam).
// Works TODAY, before F1 ships the CLI subcommand — internal/app.go writes the
// JSONL file regardless of the CLI surface. Poll interval is 750ms — cheap on a
// small file, imperceptible on a UI timescale.
function fileTailFeedSource(): FeedSource {
  return {
    start(onLine, onError) {
      const home = adbHome();
      if (!home) {
        onError(
          new Error("adb: no ADB_HOME resolved (open a workspace folder)"),
        );
        return { dispose() {} };
      }
      const filePath = eventsLogPath(home);
      let offset = 0;
      let held = "";
      let stopped = false;

      // Seed offset from file size at open — we tail NEW events only, we
      // don't replay the whole file (that would be a burst of stale rows).
      try {
        if (fs.existsSync(filePath)) {
          offset = fs.statSync(filePath).size;
        }
      } catch (e) {
        onError(e as Error);
      }

      const poll = () => {
        if (stopped) {
          return;
        }
        try {
          if (!fs.existsSync(filePath)) {
            return;
          }
          const size = fs.statSync(filePath).size;
          if (size < offset) {
            // File was truncated/rotated — restart from 0.
            offset = 0;
            held = "";
          }
          if (size === offset) {
            return;
          }
          const fd = fs.openSync(filePath, "r");
          try {
            const buf = Buffer.alloc(size - offset);
            fs.readSync(fd, buf, 0, buf.length, offset);
            offset = size;
            const r = splitLines(buf.toString("utf8"), held);
            held = r.held;
            for (const line of r.lines) {
              if (line.length > 0) {
                onLine(line);
              }
            }
          } finally {
            fs.closeSync(fd);
          }
        } catch (e) {
          onError(e as Error);
        }
      };

      const timer = setInterval(poll, 750);
      return {
        dispose() {
          stopped = true;
          clearInterval(timer);
        },
      };
    },
  };
}

// ---- F4: chat glue ------------------------------------------------------
// Every path from the webview → a mutation goes through this section. The
// invariants are:
//
//   1. Webview → extension: only 'chat.send' (message text) and
//      'chat.confirm' (numeric proposalId). Anything else is dropped.
//   2. `adb chat` is the ONLY subprocess we spawn for chat. execFile with
//      an argv (never a shell string) — mirroring runAdb() above.
//   3. Reply → parseSteerActions → for-each: lower(action, ctx). We store
//      the raw actions extension-side, keyed by index. The webview only
//      ever knows about the summary and the numeric id.
//   4. Confirm → look up the SteerAction by id → re-run lower() (belt AND
//      braces — the id could point to a stale action if the webview lags,
//      re-checking the allowlist here catches it) → showWarningMessage
//      MODAL → only on Apply do we execFile / fs.appendFileSync.
//   5. wiki.capture and unknown NEVER execute. wiki.capture surfaces as an
//      informational hint; unknown surfaces as a "refused" toast.

// handleChatSend spawns `adb chat --message <userMessage>` and, when it
// exits, parses the reply for steer proposals + posts them back. Returns
// nothing — errors are surfaced as {kind:'chat.error'} messages. NEVER
// throws (the webview would just stall).
function handleChatSend(panel: vscode.WebviewPanel, userMessage: string): void {
  const trimmed = (userMessage ?? "").trim();
  if (trimmed.length === 0) {
    void panel.webview.postMessage({
      kind: "chat.error",
      message: "empty message ignored",
    } as PanelToWebviewMessage);
    return;
  }
  // execFile, not shell. userMessage is passed as a distinct argv slot,
  // so no metacharacter can escape into the shell (there is no shell).
  execFile(
    adbBinary(),
    ["chat", "--message", trimmed],
    {
      env: adbEnv(),
      cwd: adbHome(),
      maxBuffer: 4 * 1024 * 1024, // 4 MiB — plenty for a reply
    },
    (err, stdout, stderr) => {
      if (err) {
        const detail =
          (stderr || stdout || err.message).trim() || "chat failed";
        void panel.webview.postMessage({
          kind: "chat.error",
          message: detail,
        } as PanelToWebviewMessage);
        return;
      }
      const replyText = stdout.trim();
      const actions = parseSteerActions(replyText);
      // Store raw actions extension-side. The webview only sees a summary
      // and a numeric id — a hostile webview cannot craft a new action.
      dashboardChatProposals = actions;
      const proposals: ChatProposal[] = actions.map((a, i) => ({
        id: i,
        summary: summarize(a),
        // Executable = anything but wiki.capture and unknown. wiki.capture
        // is a "run /wiki interactively" hint; unknown is refused. Both
        // render as visible-but-disabled buttons so the user sees why.
        executable: a.kind === "task.update" || a.kind === "notes.append",
      }));
      void panel.webview.postMessage({
        kind: "chat.reply",
        text: replyText,
        proposals,
      } as PanelToWebviewMessage);
    },
  );
}

// resolveTicketDirForAction is the extension-side realpath resolver that
// LowerContext requires for path-scoped actions (currently notes.append).
// It looks up the ticket by id via `adb task status --json`, resolves the
// ticket_path to an absolute filesystem path, and realpaths it — which is
// what defeats symlink escapes. Returns undefined if we can't resolve the
// ticket, in which case actions.lower() rejects with reason=path_escape.
async function resolveTicketDirForAction(
  taskId: string,
): Promise<string | undefined> {
  let tasks: AdbTask[];
  try {
    tasks = await listTasks();
  } catch {
    return undefined;
  }
  const t = tasks.find((x) => x.id === taskId);
  if (!t || !t.ticket_path) {
    return undefined;
  }
  // Realpath resolution — critical: this is what makes isPathInside()
  // safe against symlink escapes. If realpathSync throws (missing dir),
  // we return undefined and lower() rejects.
  try {
    const abs = path.resolve(t.ticket_path);
    return fs.realpathSync(abs);
  } catch {
    return undefined;
  }
}

// applyProposal is the confirm-and-execute path. It rebuilds the
// LowerContext, re-runs the allowlist (belt-and-braces), and only after
// an explicit modal Apply click does it hit the disk / spawn adb.
async function applyProposal(
  proposalId: number,
  refresh: () => void,
): Promise<void> {
  if (
    !Number.isInteger(proposalId) ||
    proposalId < 0 ||
    proposalId >= dashboardChatProposals.length
  ) {
    vscode.window.showErrorMessage(
      `ADB chat: proposal ${proposalId} not found (webview state stale?)`,
    );
    return;
  }
  const action = dashboardChatProposals[proposalId];

  // Path-scoped actions need a resolved ticket dir. Non-path-scoped ones
  // (task.update, wiki.capture, unknown) don't — passing undefined is
  // safe, actions.lower ignores it for those verbs.
  let ctx: LowerContext | undefined;
  if (action.kind === "notes.append") {
    const dir = await resolveTicketDirForAction(action.taskId);
    ctx = dir ? { resolvedTicketDir: dir } : undefined;
  }

  const lowered = lower(action, ctx);
  if (lowered.kind === "reject") {
    vscode.window.showErrorMessage(`ADB chat: ${describeRejection(lowered)}`);
    return;
  }
  if (lowered.kind === "skill.run") {
    // wiki.capture — never auto-executed. We surface a hint the user
    // can copy/paste into their Claude session.
    vscode.window.showInformationMessage(
      `ADB chat: run ${lowered.hint || "/wiki"} in Claude to capture this.`,
    );
    return;
  }

  // MANDATORY MODAL CONFIRM. Every path below this line executes — nothing
  // above did, and that's the point. showWarningMessage with {modal:true}
  // is the vscode API's blocking confirm dialog; the user must actively
  // click Apply.
  const summaryLine = summarize(action);
  let detail = "";
  if (lowered.kind === "adb") {
    detail = `Will run: adb ${lowered.argv.join(" ")}`;
  } else if (lowered.kind === "file.append") {
    detail = `Will append to ${lowered.path}\n\n${lowered.text}`;
  }
  const choice = await vscode.window.showWarningMessage(
    `Apply this change?\n\n${summaryLine}`,
    { modal: true, detail },
    "Apply",
  );
  if (choice !== "Apply") {
    return; // Cancel / dismiss. No mutation.
  }

  // Execute — narrow paths only.
  if (lowered.kind === "adb") {
    try {
      await runAdb(lowered.argv);
      vscode.window.showInformationMessage(`ADB chat: ${summaryLine}`);
      refresh();
    } catch (e) {
      vscode.window.showErrorMessage(
        `ADB chat: adb ${lowered.argv.join(" ")} failed: ${(e as Error).message}`,
      );
    }
    return;
  }

  // file.append — the ONLY fs.write path in F4. Belt-and-braces: verify
  // once more that the resolved path lands inside the resolved ticket
  // dir. If ctx is missing here it means resolveTicketDirForAction failed
  // between the click and the confirm — bail rather than write.
  if (lowered.kind === "file.append") {
    if (!ctx || !ctx.resolvedTicketDir) {
      vscode.window.showErrorMessage(
        `ADB chat: refused: could not resolve ticket dir for ${(action as { taskId: string }).taskId}`,
      );
      return;
    }
    // Realpath the target's parent to defeat any symlink race between
    // resolveTicketDirForAction and the write. Even if the ticket dir was
    // swapped for a symlink pointing elsewhere in the interim, this
    // catches it.
    let targetRealParent: string;
    try {
      const parent = path.dirname(lowered.path);
      targetRealParent = fs.realpathSync(parent);
    } catch (e) {
      vscode.window.showErrorMessage(
        `ADB chat: refused: cannot resolve target dir (${(e as Error).message})`,
      );
      return;
    }
    if (!isPathInside(ctx.resolvedTicketDir, targetRealParent)) {
      vscode.window.showErrorMessage(
        `ADB chat: refused: target dir escaped ticket dir`,
      );
      return;
    }
    // Prepend a newline so the append is on its own line even if notes.md
    // doesn't end with \n — cheap defensive normalization.
    const line = `\n${lowered.text}\n`;
    try {
      fs.appendFileSync(lowered.path, line, { encoding: "utf8" });
      vscode.window.showInformationMessage(`ADB chat: ${summaryLine}`);
    } catch (e) {
      vscode.window.showErrorMessage(
        `ADB chat: append failed: ${(e as Error).message}`,
      );
    }
  }
}

function openDashboardWebview(context: vscode.ExtensionContext): void {
  // Reveal existing panel on repeat click.
  if (dashboardPanel) {
    dashboardPanel.reveal(vscode.ViewColumn.Active);
    return;
  }
  const panel = vscode.window.createWebviewPanel(
    "adb.dashboard",
    "ADB Dashboard",
    vscode.ViewColumn.Active,
    {
      enableScripts: true,
      // No local resource roots — the panel is a single self-contained
      // HTML string with a nonce'd inline script; no external assets.
      retainContextWhenHidden: true,
    },
  );
  panel.iconPath = new vscode.ThemeIcon("dashboard");
  const nonce = makeNonce();
  panel.webview.html = renderPanelHtml(nonce);
  dashboardPanel = panel;

  const maxFeed = dashboardMaxFeedItems();

  // Webview-to-extension message handler. Kinds:
  //   'hello'         F3: webview announces readiness → send initial overview.
  //   'chat.send'     F4: user typed a message → spawn `adb chat`.
  //   'chat.confirm'  F4: user clicked a proposal button → modal-confirm-and-run.
  // Anything else is silently dropped (defence-in-depth: a compromised
  // webview cannot invent new kinds this handler will honour).
  panel.webview.onDidReceiveMessage((msg: unknown) => {
    if (!msg || typeof msg !== "object") {
      return;
    }
    const kind = (msg as { kind?: unknown }).kind;
    if (kind === "hello") {
      pushOverview(panel);
      return;
    }
    if (kind === "chat.send") {
      const text = (msg as { message?: unknown }).message;
      if (typeof text === "string") {
        handleChatSend(panel, text);
      }
      return;
    }
    if (kind === "chat.confirm") {
      const id = (msg as { proposalId?: unknown }).proposalId;
      if (typeof id === "number") {
        // Fire-and-forget — errors surface via showErrorMessage inside.
        void applyProposal(id, () => {
          // Refresh the tickets tree if the mutation was task.update.
          // We don't have a direct handle to the refresh callback here,
          // so we execute the built-in refresh command — cheap and safe.
          void vscode.commands.executeCommand("adb.refreshTickets");
        });
      }
      return;
    }
  });

  // Wire the FeedSource. Each incoming JSONL line becomes a postMessage
  // to the webview; the nonce'd script parses it (the exact same
  // eventToFeedItem logic feed.ts pins in tests) and prepends into #feed
  // with a client-side cap. maxFeed is passed as an initial hint the
  // webview enforces on its side.
  dashboardFeed = pickFeedSource().start(
    (raw) => {
      const message: PanelToWebviewMessage = { kind: "event", raw };
      panel.webview.postMessage(message);
    },
    (err) => {
      console.error("adb feed source:", err.message);
    },
  );
  // Hint: max feed items. We don't have a message kind for this yet, but
  // wiring it via a non-invasive channel keeps the surface small — the
  // client enforces its own MAX_FEED (500) and this config lets a user
  // tune it upward. We expose it via config for now; a future 'config'
  // message can push it live if needed.
  void maxFeed;

  panel.onDidDispose(
    () => {
      if (dashboardFeed) {
        dashboardFeed.dispose();
        dashboardFeed = undefined;
      }
      dashboardPanel = undefined;
      // Clear the per-panel steer proposals so a next open starts fresh.
      // (Also defensive: prevents a stale id from mapping to an action if
      // the panel is disposed and reopened while the webview state races.)
      dashboardChatProposals = [];
    },
    null,
    context.subscriptions,
  );
}

export function deactivate(): void {
  if (dashboardFeed) {
    dashboardFeed.dispose();
    dashboardFeed = undefined;
  }
  if (dashboardPanel) {
    dashboardPanel.dispose();
    dashboardPanel = undefined;
  }
}

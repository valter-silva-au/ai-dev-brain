// Pure decision logic for the legacy styled-terminal launch-file flow:
// the binary writes ~/.adb_terminal_launch.json and the extension watches
// for it. The vscode side effects (createTerminal, sendText) live in the
// extension; everything else — parsing, staleness, arg + name composition —
// is here so it can be tested without a running editor.

// LaunchRequest mirrors the JSON the binary writes to ~/.adb_terminal_launch.json.
export interface LaunchRequest {
  task_id: string;
  task_type: string;
  priority: string;
  status: string;
  worktree_path: string;
  branch: string;
  resume: boolean;
  timestamp: string;
}

// Stale-threshold for launch requests. The binary writes the file then exits;
// if the extension wakes up more than this long after, the user has likely
// dismissed/changed intent and we should drop it instead of opening a stray
// terminal. Mirrors extension.ts's prior inline 5_000 ms.
export const LAUNCH_REQUEST_STALE_MS = 5000;

// parseLaunchRequest validates and returns the parsed JSON, or undefined on
// any error (malformed JSON, wrong shape). Pure — never throws.
export function parseLaunchRequest(raw: string): LaunchRequest | undefined {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return undefined;
  }
  if (!parsed || typeof parsed !== "object") {
    return undefined;
  }
  if (Array.isArray(parsed)) {
    return undefined;
  }
  const r = parsed as Record<string, unknown>;
  // Minimum shape check: all string fields present; resume is boolean.
  // We don't reject extra fields — the binary may add metadata over time.
  const stringFields: (keyof LaunchRequest)[] = [
    "task_id",
    "task_type",
    "priority",
    "status",
    "worktree_path",
    "branch",
    "timestamp",
  ];
  for (const f of stringFields) {
    if (typeof r[f] !== "string") {
      return undefined;
    }
  }
  if (typeof r.resume !== "boolean") {
    return undefined;
  }
  return parsed as LaunchRequest;
}

// isStale returns true if the request was written more than the stale-threshold
// ago. `now` is injected so tests can pin the clock; `staleMs` defaults to the
// production constant. An unparseable timestamp is treated as stale (defensive:
// we'd rather drop one launch than spawn a stray terminal).
export function isStale(
  req: { timestamp: string },
  now: number,
  staleMs: number = LAUNCH_REQUEST_STALE_MS
): boolean {
  const ts = new Date(req.timestamp).getTime();
  if (Number.isNaN(ts)) {
    return true;
  }
  return now - ts > staleMs;
}

// composeClaudeArgs builds the argv tail passed to `claude`. The binary always
// gets --dangerously-skip-permissions (this terminal is for an adb session,
// not interactive editing); --continue is added when resuming an existing
// thread.
export function composeClaudeArgs(req: { resume: boolean }): string[] {
  const args = ["--dangerously-skip-permissions"];
  if (req.resume) {
    args.push("--continue");
  }
  return args;
}

// composeClaudeCommand returns the full shell line sent to the styled
// terminal: `claude <args...>`. Pure given the request.
export function composeClaudeCommand(req: { resume: boolean }): string {
  return `claude ${composeClaudeArgs(req).join(" ")}`;
}

// composeLaunchTerminalName returns the styled terminal's display name for a
// launch request. Identical shape to taskTerminalName (`<id> <type> <pri>`)
// but kept here so the launch-request module is self-contained.
export function composeLaunchTerminalName(req: {
  task_id: string;
  task_type: string;
  priority: string;
}): string {
  return `${req.task_id} ${req.task_type} ${req.priority}`;
}

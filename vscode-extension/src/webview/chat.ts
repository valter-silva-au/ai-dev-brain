// Pure chat-reply parser for the F4 dashboard steer flow.
//
// The convention is that the LLM (adb chat, wrapping observability.Chat) may
// propose mutations by embedding a fenced block in its reply:
//
//   ```adb-action
//   {"verb":"task.update","taskId":"TASK-1","status":"blocked"}
//   {"verb":"notes.append","taskId":"TASK-1","text":"…"}
//   ```
//
// Each non-empty line inside the block is one JSON object → one SteerAction.
// This module NEVER runs anything; it only lowers text into typed proposals.
// The extension glue then routes each proposal through actions.toArgv()
// (which is the actual allowlist) and a modal confirm gate before any
// mutation executes.
//
// SECURITY POSTURE:
//   1. This parser is deliberately permissive on shape (unknown verbs still
//      produce a SteerAction with kind='unknown') and STRICT on execution:
//      the allowlist lives in actions.ts and rejects anything outside it.
//      That way an attacker cannot smuggle a mutation by choosing a verb
//      the parser doesn't recognise — the reject-by-default gate catches it.
//   2. No shell strings, no eval, no dynamic dispatch here. The output is
//      structured data only.
//   3. Malformed JSON, non-object payloads, missing fields, and arbitrary
//      extra fields are silently dropped (we don't want a hostile block to
//      DoS the panel by throwing). The corresponding tests pin this.
//
// This module is vscode/fs-free so it unit-tests under `node`, following the
// launch.ts / orggroup.ts / feed.ts / overview.ts pattern.

// ---- Public shapes -------------------------------------------------------

export type SteerAction =
  | TaskUpdateAction
  | NotesAppendAction
  | WikiCaptureAction
  | UnknownAction;

// task.update: change a ticket's status. `status` is deliberately a plain
// string here — actions.toArgv() validates it against the 6 canonical statuses
// (backlog, in_progress, blocked, review, done, archived). Keeping the check
// in one place (the allowlist) is the point.
export interface TaskUpdateAction {
  kind: "task.update";
  taskId: string;
  status: string;
  note?: string;
}

// notes.append: append a line to <ticket_path>/notes.md. The extension glue
// resolves ticket_path from `adb task status --json`; actions.ts validates
// that the resolved append path stays *inside* that ticket dir.
export interface NotesAppendAction {
  kind: "notes.append";
  taskId: string;
  text: string;
}

// wiki.capture: request a wiki-capture run. NEVER auto-executed. The
// extension surfaces this as a button that runs the /wiki skill in the
// user's normal workflow, so the confirm gate is the user typing /wiki.
export interface WikiCaptureAction {
  kind: "wiki.capture";
  category?: string;
  slug?: string;
  note?: string;
}

// unknown: a well-formed JSON object with a verb the parser doesn't map to a
// known action. Kept explicit (rather than dropped) so the UI can render
// "unknown action, refusing to run" — trace-visible without failing open.
export interface UnknownAction {
  kind: "unknown";
  verb: string;
  raw: Record<string, unknown>;
}

// ---- Parser --------------------------------------------------------------

// Fenced-block regex. We accept ```adb-action or ```adb_action (both live in
// the wild — Claude sometimes swaps `-` for `_` in fenced language tags).
// The fence must start at a line boundary; the content is greedy up to the
// closing ```. Multiple blocks in one reply are all consumed.
const ACTION_BLOCK_RE = /```adb[-_]action\r?\n([\s\S]*?)```/g;

// parseSteerActions walks every ```adb-action fenced block in reply and
// returns one SteerAction per parseable JSON line. Malformed JSON, empty
// lines, and non-object payloads are silently skipped; this is deliberate —
// the reject-by-default happens at execute-time in actions.toArgv().
export function parseSteerActions(reply: string): SteerAction[] {
  if (typeof reply !== "string" || reply.length === 0) {
    return [];
  }
  const out: SteerAction[] = [];
  let m: RegExpExecArray | null;
  // Reset lastIndex — the regex is a top-level const and would carry state
  // between calls otherwise (the /g flag makes .exec stateful).
  ACTION_BLOCK_RE.lastIndex = 0;
  while ((m = ACTION_BLOCK_RE.exec(reply)) !== null) {
    const body = m[1] ?? "";
    for (const rawLine of body.split(/\r?\n/)) {
      const line = rawLine.trim();
      if (line.length === 0) {
        continue;
      }
      const action = parseOneLine(line);
      if (action) {
        out.push(action);
      }
    }
  }
  return out;
}

// parseOneLine turns one JSON line into a SteerAction. Returns null on
// malformed JSON or non-object payloads. Missing verb → null (not
// UnknownAction — an object with no verb at all is not a proposal).
function parseOneLine(line: string): SteerAction | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(line);
  } catch {
    return null;
  }
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    return null;
  }
  const obj = parsed as Record<string, unknown>;
  const verb = typeof obj.verb === "string" ? obj.verb : "";
  if (verb.length === 0) {
    return null;
  }
  switch (verb) {
    case "task.update": {
      if (typeof obj.taskId !== "string" || obj.taskId.length === 0) {
        return null;
      }
      if (typeof obj.status !== "string" || obj.status.length === 0) {
        return null;
      }
      const a: TaskUpdateAction = {
        kind: "task.update",
        taskId: obj.taskId,
        status: obj.status,
      };
      if (typeof obj.note === "string") {
        a.note = obj.note;
      }
      return a;
    }
    case "notes.append": {
      if (typeof obj.taskId !== "string" || obj.taskId.length === 0) {
        return null;
      }
      if (typeof obj.text !== "string" || obj.text.length === 0) {
        return null;
      }
      return { kind: "notes.append", taskId: obj.taskId, text: obj.text };
    }
    case "wiki.capture": {
      const a: WikiCaptureAction = { kind: "wiki.capture" };
      if (typeof obj.category === "string") {
        a.category = obj.category;
      }
      if (typeof obj.slug === "string") {
        a.slug = obj.slug;
      }
      if (typeof obj.note === "string") {
        a.note = obj.note;
      }
      return a;
    }
    default:
      // Preserve the unknown verb + payload so the UI can render it as
      // "refused: unknown verb '<x>'". This is defence-in-depth: the
      // allowlist in actions.ts is what actually blocks execution.
      return { kind: "unknown", verb, raw: obj };
  }
}

// summarize renders a one-line human summary for a proposed action. The
// webview uses this for the "click to review" button label. Kept pure so
// tests pin the string shape (screen readers rely on it).
export function summarize(a: SteerAction): string {
  switch (a.kind) {
    case "task.update":
      return `${a.taskId}: status → ${a.status}`;
    case "notes.append": {
      const preview = a.text.length > 40 ? a.text.slice(0, 40) + "…" : a.text;
      return `${a.taskId}: append note "${preview}"`;
    }
    case "wiki.capture": {
      const parts: string[] = [];
      if (a.category) {
        parts.push(a.category);
      }
      if (a.slug) {
        parts.push(a.slug);
      }
      const target = parts.length > 0 ? parts.join("/") : "(auto)";
      return `wiki capture: ${target}`;
    }
    case "unknown":
      return `unknown verb "${a.verb}" — refused`;
  }
}

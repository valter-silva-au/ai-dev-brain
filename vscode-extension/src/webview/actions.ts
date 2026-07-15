// Steer-action allowlist + guarded lowering to concrete effects. This is the
// SECURITY BOUNDARY of the F4 chat/steer flow — every mutation an LLM reply
// can trigger passes through this file.
//
// DESIGN INVARIANTS (each guarded by a test in actions.test.ts):
//
//   1. **Reject-by-default.** Every function returns null / a rejection
//      Result for anything not in the explicit allowlist. There is NO
//      generic exec path. There is NO shell string anywhere. adb subcommand
//      argv is *reconstructed from a fixed template* using validated inputs
//      — we never pass through free-form strings from the LLM.
//
//   2. **Task IDs must match /^TASK-[0-9]+$/.** Refuses `TASK-1;rm`, spaces,
//      empty, absurdly long — anything that could smuggle argv or a path.
//
//   3. **Status must be one of the 6 canonical statuses** (backlog,
//      in_progress, blocked, review, done, archived). Any other string →
//      rejected. Matches the existing extension.ts updateStatus() enum.
//
//   4. **notes.append path escape guard.** The resolved append target must
//      resolve to a path *inside* the ticket dir (path.resolve prefix
//      match). Rejects absolute paths, `..`/traversal, and — crucially —
//      any resolvedPath whose real prefix is not the ticket dir. Callers
//      that want to defend against symlink escape must additionally
//      realpath before calling; we document that requirement here (the
//      extension glue does that check, tested in actions.test.ts via a
//      resolvedTarget parameter that mirrors what fs.realpathSync returns).
//
//   5. **wiki.capture is never lowered to argv.** It surfaces as a button
//      that runs the /wiki skill in the user's normal flow. This module
//      REFUSES to produce an executable form for it — that's the point.
//
//   6. **UnknownAction always rejects.** No path from unknown verb → argv.
//
// This module is vscode/fs-free. The extension glue (extension.ts) does the
// actual `execFile('adb', argv)` call and the `fs.appendFileSync` — both
// only after this module returns a non-null argv/path.
import * as path from "path";
import type { SteerAction } from "./chat";

// ---- Constants (the allowlists themselves) -------------------------------

// The 6 canonical adb task statuses. Mirrored from the extension's own
// updateStatus() QuickPick (extension.ts) and the Go-side task.Status set
// (pkg/models/task.go). Keeping this as a frozen literal so a hostile
// SteerAction can't reach in and mutate it at runtime.
export const ALLOWED_STATUSES: readonly string[] = Object.freeze([
  "backlog",
  "in_progress",
  "blocked",
  "review",
  "done",
  "archived",
]);

// Task ID shape. adb mints ids as TASK-<zero-padded int>; we accept any
// TASK-<digits> so a hand-typed short id works, but nothing else.
const TASK_ID_RE = /^TASK-[0-9]+$/;

// Cap to make DoS via a huge notes.append payload impossible-ish. 4KiB is
// well beyond a normal chat suggestion but nowhere near enough to fill the
// disk from a single button-click.
const MAX_NOTE_TEXT_BYTES = 4096;

// ---- Result types --------------------------------------------------------

// AdbInvocation is the ONLY way this module lets you run the adb binary.
// argv is the fixed [subcommand, ...args] slice — no shell involved. The
// extension glue passes this straight to execFile(adbBinary(), inv.argv).
export interface AdbInvocation {
  kind: "adb";
  argv: string[];
}

// FileAppend is the ONLY way this module lets you touch the filesystem.
// path is the pre-resolved absolute path *inside the ticket dir*; text is
// the appended content (single line, newline appended by the writer).
export interface FileAppend {
  kind: "file.append";
  path: string;
  text: string;
}

// SkillRun is a NON-executable descriptor of a skill the user should run
// interactively (e.g. /wiki). This exists precisely to make wiki.capture
// visible without giving it an execution path.
export interface SkillRun {
  kind: "skill.run";
  skill: string;
  hint?: string;
}

// LoweredAction is what the extension glue receives per proposed action.
export type LoweredAction = AdbInvocation | FileAppend | SkillRun;

// Rejection carries a machine-readable reason so the UI can render "refused
// because <reason>" without hard-coding messages. Reason codes are stable
// across releases (see actions.test.ts).
export type Rejection =
  | { kind: "reject"; reason: "unknown_verb"; verb: string }
  | { kind: "reject"; reason: "invalid_task_id"; taskId: string }
  | { kind: "reject"; reason: "invalid_status"; status: string }
  | { kind: "reject"; reason: "empty_text" }
  | { kind: "reject"; reason: "text_too_long"; bytes: number }
  | { kind: "reject"; reason: "path_escape"; target: string }
  | { kind: "reject"; reason: "not_executable"; verb: string };

// ---- Task ID / status validators (pure) ---------------------------------

// isAllowedStatus is exported so the extension glue can pre-filter without
// re-implementing the check. The check is the exact string set above; no
// fuzzy matching, no case-fold — the LLM MUST emit the canonical form.
export function isAllowedStatus(s: string): boolean {
  return ALLOWED_STATUSES.includes(s);
}

// isValidTaskId enforces /^TASK-[0-9]+$/. This catches any argv-injection
// attempt: shell metacharacters (;, |, &&, backticks), path components
// (../, /, C:\), spaces, and unicode confusables all fail.
export function isValidTaskId(id: string): boolean {
  return typeof id === "string" && TASK_ID_RE.test(id);
}

// ---- Path escape guard ---------------------------------------------------

// isPathInside returns true iff resolvedTarget is at or below resolvedRoot
// on the filesystem, using path.resolve semantics. Callers pre-resolve both
// (fs.realpathSync in the extension glue) so symlink escapes are also
// caught. This is the ONLY path check in the module — everything else
// delegates here.
//
// Guards against:
//   * "../../etc/passwd"           → resolves outside root
//   * "/etc/passwd"                → absolute, not under root
//   * "notes.md" when root=/tmp/x  → root/notes.md — inside, allowed
//   * "..\\..\\Windows\\..." (win) → path.resolve normalises separators
//
// Does NOT guard against symlinks itself; the extension glue MUST realpath
// both arguments before calling. Tests pin that convention.
export function isPathInside(resolvedRoot: string, resolvedTarget: string): boolean {
  if (typeof resolvedRoot !== "string" || typeof resolvedTarget !== "string") {
    return false;
  }
  if (resolvedRoot.length === 0 || resolvedTarget.length === 0) {
    return false;
  }
  // path.resolve normalises `..` and `.` segments; if the target still isn't
  // prefixed by the root after that, it's outside.
  const normRoot = path.resolve(resolvedRoot);
  const normTarget = path.resolve(resolvedTarget);
  if (normTarget === normRoot) {
    return true;
  }
  // Require a directory separator boundary — "/tmp/adb" must not match
  // "/tmp/adb-evil/notes.md". Using path.sep so the check is cross-platform.
  const withSep = normRoot.endsWith(path.sep) ? normRoot : normRoot + path.sep;
  return normTarget.startsWith(withSep);
}

// ---- Lowering ------------------------------------------------------------

// toArgv is the historical name (from the plan) but the module actually
// returns a discriminated LoweredAction — an argv[] alone would leak the
// existence of the file-append and skill-run paths. Kept as an alias for
// discoverability + a matching pure-argv helper for the plan's tests.
export function toAdbArgv(a: SteerAction): string[] | null {
  const lowered = lower(a, undefined);
  if (lowered.kind === "adb") {
    return lowered.argv;
  }
  return null;
}

// LowerContext is the ambient info the guard needs for path-scoped verbs.
// resolvedTicketDir MUST be an absolute, realpath'd directory the caller
// vouches is the ticket's own directory (the extension glue derives it
// from `adb task status --json`.ticket_path). Empty/missing → any
// path-scoped action is rejected by default.
export interface LowerContext {
  // Absolute realpath of the ticket dir this action targets. The extension
  // glue MUST resolve this from `adb task status --json`; the parser output
  // never carries a path.
  resolvedTicketDir?: string;
}

// lower is the full guard: takes one SteerAction + optional LowerContext,
// returns either a LoweredAction (safe to run) or a Rejection (why we
// refused). The extension calls this per action and shows the rejection
// reason in the UI so users see WHY a mutation was blocked.
export function lower(a: SteerAction, ctx: LowerContext | undefined): LoweredAction | Rejection {
  switch (a.kind) {
    case "task.update":
      return lowerTaskUpdate(a.taskId, a.status);
    case "notes.append":
      return lowerNotesAppend(a.taskId, a.text, ctx);
    case "wiki.capture":
      return lowerWikiCapture(a);
    case "unknown":
      return { kind: "reject", reason: "unknown_verb", verb: a.verb };
  }
}

function lowerTaskUpdate(taskId: string, status: string): LoweredAction | Rejection {
  if (!isValidTaskId(taskId)) {
    return { kind: "reject", reason: "invalid_task_id", taskId };
  }
  if (!isAllowedStatus(status)) {
    return { kind: "reject", reason: "invalid_status", status };
  }
  // Fixed argv template. Nothing here is user-controlled at the shell
  // level: taskId is /^TASK-[0-9]+$/, status is a fixed enum, and both
  // become distinct argv slots (execFile, not shell). --status=<x> matches
  // the existing updateStatus() invocation in extension.ts:530.
  return {
    kind: "adb",
    argv: ["task", "update", taskId, `--status=${status}`],
  };
}

function lowerNotesAppend(
  taskId: string,
  text: string,
  ctx: LowerContext | undefined
): LoweredAction | Rejection {
  if (!isValidTaskId(taskId)) {
    return { kind: "reject", reason: "invalid_task_id", taskId };
  }
  const trimmed = text.replace(/\r/g, "");
  if (trimmed.length === 0) {
    return { kind: "reject", reason: "empty_text" };
  }
  const bytes = Buffer.byteLength(trimmed, "utf8");
  if (bytes > MAX_NOTE_TEXT_BYTES) {
    return { kind: "reject", reason: "text_too_long", bytes };
  }
  const ticketDir = ctx?.resolvedTicketDir ?? "";
  if (ticketDir.length === 0 || !path.isAbsolute(ticketDir)) {
    return { kind: "reject", reason: "path_escape", target: ticketDir };
  }
  // Fixed filename — the LLM never chooses which file inside the ticket
  // dir to touch; only notes.md is writable via this verb. This is the
  // pin that makes "notes.append" specifically about notes.md.
  const target = path.join(ticketDir, "notes.md");
  if (!isPathInside(ticketDir, target)) {
    // Should be impossible with a fixed filename, but the check is here
    // so a future refactor that lets the LLM pick a filename can't
    // silently drop the guard.
    return { kind: "reject", reason: "path_escape", target };
  }
  return { kind: "file.append", path: target, text: trimmed };
}

function lowerWikiCapture(a: {
  category?: string;
  slug?: string;
  note?: string;
}): LoweredAction | Rejection {
  // wiki.capture never gets an executable form. It's surfaced to the user
  // as a hint to run the /wiki skill in their normal chat — which is
  // itself an interactive, user-driven flow. No auto-execution.
  const parts: string[] = [];
  if (typeof a.category === "string" && a.category.length > 0) {
    parts.push(a.category);
  }
  if (typeof a.slug === "string" && a.slug.length > 0) {
    parts.push(a.slug);
  }
  const hint = parts.length > 0 ? `/wiki ${parts.join(" ")}` : `/wiki`;
  return { kind: "skill.run", skill: "wiki", hint };
}

// ---- Convenience: describe a rejection for a modal --------------------

// describeRejection renders a short, user-facing reason for a rejection.
// Kept pure so the tests pin the strings screen readers rely on.
export function describeRejection(r: Rejection): string {
  switch (r.reason) {
    case "unknown_verb":
      return `refused: unknown verb "${r.verb}"`;
    case "invalid_task_id":
      return `refused: invalid task id "${r.taskId}"`;
    case "invalid_status":
      return `refused: invalid status "${r.status}"`;
    case "empty_text":
      return `refused: empty note text`;
    case "text_too_long":
      return `refused: note text too long (${r.bytes} bytes)`;
    case "path_escape":
      return `refused: path escape (target "${r.target}")`;
    case "not_executable":
      return `refused: verb "${r.verb}" is never auto-executed`;
  }
}

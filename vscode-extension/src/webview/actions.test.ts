// SECURITY-CRITICAL tests for the F4 steer-action allowlist.
//
// These tests are the guard-rail for the /security-review pass. Every
// invariant in actions.ts's header comment gets at least one test here.
// Adding a new verb to lower() MUST come with a "rejects <hostile
// variant>" test in this file.
//
// The rejection reasons are a stable string enum — the UI reads them, so
// tests pin the exact reason codes.

import * as path from "path";
import { t, eq, ok, deepEq, notOk } from "../test-harness";
import type { SteerAction } from "./chat";
import {
  ALLOWED_STATUSES,
  isAllowedStatus,
  isValidTaskId,
  isPathInside,
  toAdbArgv,
  lower,
  describeRejection,
  LowerContext,
} from "./actions";

t.setModule("actions");

// =========================================================================
// Allowed-status enum: the 6 canonical statuses; nothing else.
// =========================================================================

t.testSync("ALLOWED_STATUSES: exactly the 6 canonical statuses", () => {
  deepEq([...ALLOWED_STATUSES], [
    "backlog",
    "in_progress",
    "blocked",
    "review",
    "done",
    "archived",
  ]);
});

t.testSync("isAllowedStatus: accepts every canonical status", () => {
  for (const s of ALLOWED_STATUSES) {
    ok(isAllowedStatus(s), `expected ${s} allowed`);
  }
});

t.testSync("isAllowedStatus: rejects garbage / lookalikes / injection", () => {
  const hostile = [
    "",
    "InProgress", // wrong case
    "in-progress", // hyphen instead of underscore
    "in_progress ", // trailing space
    "done;rm -rf /", // shell injection attempt
    "blocked --extra=arg", // extra argv attempt
    "arbitrary",
    "..",
  ];
  for (const s of hostile) {
    notOk(isAllowedStatus(s), `expected "${s}" rejected`);
  }
});

t.testSync("ALLOWED_STATUSES: is frozen (defence against runtime mutation)", () => {
  ok(Object.isFrozen(ALLOWED_STATUSES), "ALLOWED_STATUSES must be frozen");
});

// =========================================================================
// Task-ID shape guard (blocks argv/path smuggling via the id).
// =========================================================================

t.testSync("isValidTaskId: accepts canonical shape", () => {
  ok(isValidTaskId("TASK-1"));
  ok(isValidTaskId("TASK-00081"));
  ok(isValidTaskId("TASK-999999"));
});

t.testSync("isValidTaskId: rejects hostile shapes", () => {
  const hostile = [
    "",
    "task-1", // lowercase
    "TASK-", // no number
    "TASK-1a", // trailing letter
    "TASK-1;rm -rf /", // shell injection
    "TASK-1 --force", // extra argv
    "TASK-1/../etc", // path traversal
    "../TASK-1", // path escape prefix
    "TASK-1\n--secret", // newline injection
    "TASK-1\x00", // NUL byte
    "TASK-1 TASK-2", // two ids
    "OTHER-1", // wrong prefix
  ];
  for (const id of hostile) {
    notOk(isValidTaskId(id), `expected "${JSON.stringify(id)}" rejected`);
  }
});

t.testSync("isValidTaskId: rejects non-string inputs", () => {
  // Cast so tsc allows the "wrong type" call — this is the whole point
  // (real callers might pass through untyped LLM JSON).
  notOk(isValidTaskId(undefined as unknown as string));
  notOk(isValidTaskId(null as unknown as string));
  notOk(isValidTaskId(42 as unknown as string));
});

// =========================================================================
// Path-inside guard (the notes.append escape check).
// =========================================================================

t.testSync("isPathInside: file directly inside root is allowed", () => {
  const root = path.resolve("/tmp/adb-ticket");
  ok(isPathInside(root, path.join(root, "notes.md")));
});

t.testSync("isPathInside: same path as root is allowed", () => {
  const root = path.resolve("/tmp/adb-ticket");
  ok(isPathInside(root, root));
});

t.testSync("isPathInside: nested subdir is allowed", () => {
  const root = path.resolve("/tmp/adb-ticket");
  ok(isPathInside(root, path.join(root, "sub", "deep", "notes.md")));
});

t.testSync("isPathInside: `..` escape is rejected", () => {
  const root = path.resolve("/tmp/adb-ticket");
  const bad = path.join(root, "..", "..", "etc", "passwd");
  notOk(isPathInside(root, bad), "must reject ../ escape");
});

t.testSync("isPathInside: absolute /etc/passwd is rejected", () => {
  const root = path.resolve("/tmp/adb-ticket");
  notOk(isPathInside(root, "/etc/passwd"));
});

t.testSync("isPathInside: sibling with matching prefix is rejected", () => {
  // The sep-boundary check is what catches this. /tmp/adb must NOT allow
  // /tmp/adb-evil/notes.md.
  const root = path.resolve("/tmp/adb");
  const sibling = path.resolve("/tmp/adb-evil/notes.md");
  notOk(isPathInside(root, sibling), "must require dir-separator boundary");
});

t.testSync("isPathInside: empty inputs rejected", () => {
  notOk(isPathInside("", "/tmp/x"));
  notOk(isPathInside("/tmp/x", ""));
  notOk(isPathInside("", ""));
});

// =========================================================================
// task.update: SteerAction → adb argv
// =========================================================================

t.testSync("lower(task.update): valid → correct fixed argv", () => {
  const a: SteerAction = { kind: "task.update", taskId: "TASK-1", status: "blocked" };
  const out = lower(a, undefined);
  eq(out.kind, "adb");
  if (out.kind === "adb") {
    deepEq(out.argv, ["task", "update", "TASK-1", "--status=blocked"]);
  }
});

t.testSync("lower(task.update): invalid status → rejected with reason", () => {
  const a: SteerAction = { kind: "task.update", taskId: "TASK-1", status: "arbitrary" };
  const out = lower(a, undefined);
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "invalid_status");
    if (out.reason === "invalid_status") {
      eq(out.status, "arbitrary");
    }
  }
});

t.testSync("lower(task.update): status with shell injection → rejected", () => {
  const a: SteerAction = {
    kind: "task.update",
    taskId: "TASK-1",
    status: "done; rm -rf /",
  };
  const out = lower(a, undefined);
  eq(out.kind, "reject");
});

t.testSync("lower(task.update): invalid task id → rejected before any argv is built", () => {
  const a: SteerAction = {
    kind: "task.update",
    taskId: "TASK-1;evil",
    status: "blocked",
  };
  const out = lower(a, undefined);
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "invalid_task_id");
  }
});

t.testSync("toAdbArgv(task.update): convenience returns argv[]", () => {
  const argv = toAdbArgv({ kind: "task.update", taskId: "TASK-42", status: "done" });
  deepEq(argv, ["task", "update", "TASK-42", "--status=done"]);
});

t.testSync("toAdbArgv(task.update): rejected → null (never partial argv)", () => {
  const argv = toAdbArgv({ kind: "task.update", taskId: "bogus", status: "done" });
  eq(argv, null);
});

// =========================================================================
// notes.append: SteerAction → guarded FileAppend
// =========================================================================

t.testSync("lower(notes.append): valid → FileAppend inside ticket dir", () => {
  const ticketDir = path.resolve("/tmp/adb-ticket-42");
  const ctx: LowerContext = { resolvedTicketDir: ticketDir };
  const a: SteerAction = {
    kind: "notes.append",
    taskId: "TASK-42",
    text: "observed X",
  };
  const out = lower(a, ctx);
  eq(out.kind, "file.append");
  if (out.kind === "file.append") {
    // Fixed filename — LLM cannot pick.
    eq(out.path, path.join(ticketDir, "notes.md"));
    eq(out.text, "observed X");
  }
});

t.testSync("lower(notes.append): missing ticket dir → path_escape rejection", () => {
  const a: SteerAction = { kind: "notes.append", taskId: "TASK-1", text: "hi" };
  const out = lower(a, undefined);
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "path_escape");
  }
});

t.testSync("lower(notes.append): non-absolute ticket dir → rejected", () => {
  // A relative path could resolve anywhere depending on cwd — the guard
  // insists on an absolute realpath.
  const a: SteerAction = { kind: "notes.append", taskId: "TASK-1", text: "hi" };
  const out = lower(a, { resolvedTicketDir: "some/relative/dir" });
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "path_escape");
  }
});

t.testSync("lower(notes.append): empty text → rejected", () => {
  const ticketDir = path.resolve("/tmp/adb-ticket");
  const out = lower(
    { kind: "notes.append", taskId: "TASK-1", text: "" },
    { resolvedTicketDir: ticketDir }
  );
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "empty_text");
  }
});

t.testSync("lower(notes.append): huge text → rejected", () => {
  const ticketDir = path.resolve("/tmp/adb-ticket");
  const huge = "x".repeat(5000); // > 4KiB cap
  const out = lower(
    { kind: "notes.append", taskId: "TASK-1", text: huge },
    { resolvedTicketDir: ticketDir }
  );
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "text_too_long");
  }
});

t.testSync("lower(notes.append): invalid task id → rejected before path resolution", () => {
  const ticketDir = path.resolve("/tmp/adb-ticket");
  const out = lower(
    { kind: "notes.append", taskId: "../../../root", text: "hi" },
    { resolvedTicketDir: ticketDir }
  );
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "invalid_task_id");
  }
});

// =========================================================================
// wiki.capture: NEVER auto-executed. Always lowered to a SkillRun hint.
// =========================================================================

t.testSync("lower(wiki.capture): produces skill.run hint, never adb argv", () => {
  const out = lower(
    { kind: "wiki.capture", category: "projects", slug: "foo" },
    undefined
  );
  eq(out.kind, "skill.run");
  if (out.kind === "skill.run") {
    eq(out.skill, "wiki");
    ok(out.hint && out.hint.includes("/wiki"));
    ok(out.hint && out.hint.includes("projects"));
  }
});

t.testSync("toAdbArgv(wiki.capture): returns null — no adb argv path", () => {
  eq(toAdbArgv({ kind: "wiki.capture", category: "projects", slug: "foo" }), null);
});

// =========================================================================
// unknown verb: ALWAYS rejected. This is the shell.exec smuggling test.
// =========================================================================

t.testSync("lower(unknown): shell.exec verb → reject with reason unknown_verb", () => {
  const a: SteerAction = {
    kind: "unknown",
    verb: "shell.exec",
    raw: { cmd: "rm -rf /" },
  };
  const out = lower(a, undefined);
  eq(out.kind, "reject");
  if (out.kind === "reject") {
    eq(out.reason, "unknown_verb");
    if (out.reason === "unknown_verb") {
      eq(out.verb, "shell.exec");
    }
  }
});

t.testSync("toAdbArgv(unknown): NEVER produces argv — always null", () => {
  const verbs = ["shell.exec", "eval", "task.delete", "wiki.publish", ""];
  for (const v of verbs) {
    const out = toAdbArgv({ kind: "unknown", verb: v, raw: {} });
    eq(out, null, `verb "${v}" must lower to null`);
  }
});

// =========================================================================
// Rejection descriptions (screen-reader / UI text pins).
// =========================================================================

t.testSync("describeRejection: unknown_verb names the verb", () => {
  const s = describeRejection({ kind: "reject", reason: "unknown_verb", verb: "shell.exec" });
  ok(s.toLowerCase().includes("refused"));
  ok(s.includes("shell.exec"));
});

t.testSync("describeRejection: path_escape names the target", () => {
  const s = describeRejection({
    kind: "reject",
    reason: "path_escape",
    target: "/etc/passwd",
  });
  ok(s.toLowerCase().includes("refused"));
  ok(s.includes("/etc/passwd"));
});

t.testSync("describeRejection: invalid_status names the status", () => {
  const s = describeRejection({
    kind: "reject",
    reason: "invalid_status",
    status: "arbitrary",
  });
  ok(s.includes("arbitrary"));
});

// =========================================================================
// Belt-and-braces: no path from any SteerAction shape to a shell string.
// =========================================================================

t.testSync("no argv slot ever contains a shell metacharacter for a valid input", () => {
  const argv = toAdbArgv({ kind: "task.update", taskId: "TASK-1", status: "blocked" });
  ok(argv);
  for (const slot of argv!) {
    // Since each slot is either a fixed literal, a validated TASK-<int>, or
    // a fixed --status=<enum>, none can carry a shell metachar. If this
    // ever fails, someone regressed the guards.
    notOk(/[;&|`$<>\\]/.test(slot), `slot ${JSON.stringify(slot)} contains a shell metachar`);
  }
});

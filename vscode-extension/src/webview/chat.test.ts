// Tests for the pure chat-reply parser (F4).
//
// The parser is permissive on shape (unknown verbs → UnknownAction, malformed
// JSON silently dropped) — the security-critical filter is actions.toArgv()
// and lives in actions.test.ts. These tests pin only the parser's own
// contract: fenced-block extraction, per-line JSON parsing, well-formed
// mapping to SteerAction discriminants, and defensive rejection of shapes
// that shouldn't produce a proposal at all (missing fields, non-object JSON).

import { t, eq, ok, deepEq } from "../test-harness";
import { parseSteerActions, summarize } from "./chat";

t.setModule("chat");

// ---- Fenced-block extraction --------------------------------------------

t.testSync("parseSteerActions: empty/no-fence input yields no actions", () => {
  deepEq(parseSteerActions(""), []);
  deepEq(parseSteerActions("no fence here, just prose"), []);
});

t.testSync("parseSteerActions: extracts one action from a valid fence", () => {
  const reply =
    "sure — let's mark that blocked.\n" +
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-1","status":"blocked"}\n' +
    "```\n" +
    "let me know if that's not right.";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  eq(actions[0].kind, "task.update");
  if (actions[0].kind === "task.update") {
    eq(actions[0].taskId, "TASK-1");
    eq(actions[0].status, "blocked");
  }
});

t.testSync("parseSteerActions: extracts multiple actions from one fence", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-1","status":"in_progress"}\n' +
    '{"verb":"notes.append","taskId":"TASK-1","text":"kicking off"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 2);
  eq(actions[0].kind, "task.update");
  eq(actions[1].kind, "notes.append");
});

t.testSync("parseSteerActions: extracts across multiple fences", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-1","status":"blocked"}\n' +
    "```\n\nthinking...\n\n" +
    "```adb-action\n" +
    '{"verb":"wiki.capture","category":"projects","slug":"foo"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 2);
  eq(actions[0].kind, "task.update");
  eq(actions[1].kind, "wiki.capture");
});

t.testSync("parseSteerActions: accepts adb_action underscore variant", () => {
  const reply =
    "```adb_action\n" +
    '{"verb":"task.update","taskId":"TASK-2","status":"done"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  eq(actions[0].kind, "task.update");
});

// ---- Per-line JSON handling ---------------------------------------------

t.testSync("parseSteerActions: skips malformed JSON lines but keeps siblings", () => {
  const reply =
    "```adb-action\n" +
    "{not valid json\n" +
    '{"verb":"task.update","taskId":"TASK-3","status":"review"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  eq(actions[0].kind, "task.update");
});

t.testSync("parseSteerActions: skips empty lines", () => {
  const reply =
    "```adb-action\n" +
    "\n" +
    '{"verb":"task.update","taskId":"TASK-4","status":"blocked"}\n' +
    "\n" +
    "```";
  eq(parseSteerActions(reply).length, 1);
});

t.testSync("parseSteerActions: JSON that isn't an object → dropped", () => {
  const reply =
    "```adb-action\n" +
    '"just a string"\n' +
    "42\n" +
    '["array"]\n' +
    "null\n" +
    "```";
  deepEq(parseSteerActions(reply), []);
});

// ---- Verb → discriminant mapping ----------------------------------------

t.testSync("parseSteerActions: task.update requires taskId + status", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-1"}\n' + // no status
    '{"verb":"task.update","status":"blocked"}\n' + // no taskId
    '{"verb":"task.update","taskId":"","status":"blocked"}\n' + // empty
    "```";
  deepEq(parseSteerActions(reply), []);
});

t.testSync("parseSteerActions: task.update carries optional note", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-1","status":"blocked","note":"why"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  if (actions[0].kind === "task.update") {
    eq(actions[0].note, "why");
  } else {
    ok(false, "expected task.update");
  }
});

t.testSync("parseSteerActions: notes.append requires taskId + text", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"notes.append","taskId":"TASK-1"}\n' + // no text
    '{"verb":"notes.append","text":"hi"}\n' + // no taskId
    '{"verb":"notes.append","taskId":"TASK-1","text":""}\n' + // empty text
    "```";
  deepEq(parseSteerActions(reply), []);
});

t.testSync("parseSteerActions: notes.append carries text", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"notes.append","taskId":"TASK-1","text":"observed X"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  if (actions[0].kind === "notes.append") {
    eq(actions[0].taskId, "TASK-1");
    eq(actions[0].text, "observed X");
  } else {
    ok(false, "expected notes.append");
  }
});

t.testSync("parseSteerActions: wiki.capture with just category", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"wiki.capture","category":"projects"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  eq(actions[0].kind, "wiki.capture");
  if (actions[0].kind === "wiki.capture") {
    eq(actions[0].category, "projects");
    eq(actions[0].slug, undefined);
  }
});

// ---- Unknown-verb defensive path ----------------------------------------

t.testSync("parseSteerActions: unknown verb is preserved as UnknownAction", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":"shell.exec","cmd":"rm -rf /"}\n' +
    "```";
  const actions = parseSteerActions(reply);
  eq(actions.length, 1);
  eq(actions[0].kind, "unknown");
  if (actions[0].kind === "unknown") {
    eq(actions[0].verb, "shell.exec");
    // The raw payload is retained so the UI can render "refused" with
    // context — but nothing in the parser dispatches on it.
    eq(actions[0].raw.cmd, "rm -rf /");
  }
});

t.testSync("parseSteerActions: object without verb → dropped, not unknown", () => {
  const reply =
    "```adb-action\n" +
    '{"taskId":"TASK-1","status":"blocked"}\n' +
    "```";
  deepEq(parseSteerActions(reply), []);
});

t.testSync("parseSteerActions: verb must be a string", () => {
  const reply =
    "```adb-action\n" +
    '{"verb":42,"taskId":"TASK-1"}\n' +
    "```";
  deepEq(parseSteerActions(reply), []);
});

// ---- summarize -----------------------------------------------------------

t.testSync("summarize: task.update includes id and status", () => {
  const s = summarize({
    kind: "task.update",
    taskId: "TASK-1",
    status: "blocked",
  });
  ok(s.includes("TASK-1"));
  ok(s.includes("blocked"));
});

t.testSync("summarize: notes.append truncates long text preview", () => {
  const long = "x".repeat(80);
  const s = summarize({ kind: "notes.append", taskId: "TASK-1", text: long });
  ok(s.includes("TASK-1"));
  ok(s.includes("…"), `expected ellipsis for long preview, got: ${s}`);
});

t.testSync("summarize: unknown verb makes it obvious it was refused", () => {
  const s = summarize({ kind: "unknown", verb: "shell.exec", raw: {} });
  ok(s.toLowerCase().includes("refused"));
  ok(s.includes("shell.exec"));
});

// ---- Regex statefulness regression --------------------------------------

t.testSync("parseSteerActions: the /g regex does not carry state between calls", () => {
  // If ACTION_BLOCK_RE.lastIndex weren't reset, the second call would miss
  // its match. Pin this by calling twice with the same input and expecting
  // the same output both times.
  const reply =
    "```adb-action\n" +
    '{"verb":"task.update","taskId":"TASK-9","status":"done"}\n' +
    "```";
  eq(parseSteerActions(reply).length, 1, "first call");
  eq(parseSteerActions(reply).length, 1, "second call");
});

import {
  parseLaunchRequest,
  isStale,
  composeClaudeArgs,
  composeClaudeCommand,
  composeLaunchTerminalName,
  LAUNCH_REQUEST_STALE_MS,
  LaunchRequest,
} from "./launchRequest";
import { t, eq, deepEq, ok, notOk } from "./test-harness";

t.setModule("launchRequest");

const validReq = (over: Partial<LaunchRequest> = {}): LaunchRequest => ({
  task_id: "TASK-00005",
  task_type: "feat",
  priority: "P2",
  status: "in_progress",
  worktree_path: "/wt/TASK-00005",
  branch: "feat/TASK-00005",
  resume: false,
  timestamp: new Date().toISOString(),
  ...over,
});

// ---- parseLaunchRequest --------------------------------------------------
t.testSync("parseLaunchRequest: well-formed JSON → parsed", () => {
  const req = validReq();
  const parsed = parseLaunchRequest(JSON.stringify(req));
  ok(parsed !== undefined);
  eq(parsed!.task_id, "TASK-00005");
  eq(parsed!.resume, false);
});
t.testSync("parseLaunchRequest: malformed JSON → undefined", () => {
  eq(parseLaunchRequest("{not json"), undefined);
});
t.testSync("parseLaunchRequest: empty string → undefined", () => {
  eq(parseLaunchRequest(""), undefined);
});
t.testSync("parseLaunchRequest: null → undefined", () => {
  eq(parseLaunchRequest("null"), undefined);
});
t.testSync("parseLaunchRequest: array → undefined", () => {
  eq(parseLaunchRequest("[]"), undefined);
});
t.testSync("parseLaunchRequest: missing string field → undefined", () => {
  const partial = { ...validReq(), task_id: undefined };
  eq(parseLaunchRequest(JSON.stringify(partial)), undefined);
});
t.testSync("parseLaunchRequest: wrong-type string field → undefined", () => {
  const partial = { ...validReq(), priority: 42 };
  eq(parseLaunchRequest(JSON.stringify(partial)), undefined);
});
t.testSync("parseLaunchRequest: resume must be boolean", () => {
  const partial = { ...validReq(), resume: "true" };
  eq(parseLaunchRequest(JSON.stringify(partial)), undefined);
});
t.testSync("parseLaunchRequest: extra fields are ignored (forward-compat)", () => {
  const withExtra = { ...validReq(), some_future_field: "x" };
  const parsed = parseLaunchRequest(JSON.stringify(withExtra));
  ok(parsed !== undefined);
  eq(parsed!.task_id, "TASK-00005");
});
t.testSync("parseLaunchRequest: number → undefined", () => {
  eq(parseLaunchRequest("42"), undefined);
});
t.testSync("parseLaunchRequest: string-literal JSON → undefined", () => {
  eq(parseLaunchRequest('"hi"'), undefined);
});
t.testSync("parseLaunchRequest: missing resume field → undefined", () => {
  const { resume: _resume, ...rest } = validReq();
  eq(parseLaunchRequest(JSON.stringify(rest)), undefined);
});
t.testSync("parseLaunchRequest: missing timestamp → undefined", () => {
  const { timestamp: _timestamp, ...rest } = validReq();
  eq(parseLaunchRequest(JSON.stringify(rest)), undefined);
});

// ---- isStale -------------------------------------------------------------
t.testSync("isStale: just-written request → not stale", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  const now = new Date(ts).getTime() + 1000; // 1s old
  notOk(isStale(req, now));
});
t.testSync("isStale: at exactly the threshold → not stale (strictly >)", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  const now = new Date(ts).getTime() + LAUNCH_REQUEST_STALE_MS;
  notOk(isStale(req, now));
});
t.testSync("isStale: 1ms past threshold → stale", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  const now = new Date(ts).getTime() + LAUNCH_REQUEST_STALE_MS + 1;
  ok(isStale(req, now));
});
t.testSync("isStale: 30 seconds old → stale", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  const now = new Date(ts).getTime() + 30000;
  ok(isStale(req, now));
});
t.testSync("isStale: clock-skew (now < timestamp) → not stale", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  // Clock moved backwards 2s — diff is negative, < threshold.
  const now = new Date(ts).getTime() - 2000;
  notOk(isStale(req, now));
});
t.testSync("isStale: garbage timestamp → stale (defensive)", () => {
  ok(isStale({ timestamp: "not-a-date" }, Date.now()));
});
t.testSync("isStale: empty timestamp → stale", () => {
  ok(isStale({ timestamp: "" }, Date.now()));
});
t.testSync("isStale: custom staleMs override is honored", () => {
  const ts = "2026-06-26T10:00:00Z";
  const req = validReq({ timestamp: ts });
  const now = new Date(ts).getTime() + 10000;
  // With default 5s threshold this is stale.
  ok(isStale(req, now));
  // With a custom 60s threshold it's not.
  notOk(isStale(req, now, 60000));
});

// ---- composeClaudeArgs / composeClaudeCommand -----------------------------
t.testSync("composeClaudeArgs: resume=false → just permissions flag", () => {
  deepEq(composeClaudeArgs({ resume: false }), ["--dangerously-skip-permissions"]);
});
t.testSync("composeClaudeArgs: resume=true → adds --continue", () => {
  deepEq(composeClaudeArgs({ resume: true }), [
    "--dangerously-skip-permissions",
    "--continue",
  ]);
});
t.testSync("composeClaudeCommand: fresh launch", () => {
  eq(composeClaudeCommand({ resume: false }), "claude --dangerously-skip-permissions");
});
t.testSync("composeClaudeCommand: resume launch", () => {
  eq(
    composeClaudeCommand({ resume: true }),
    "claude --dangerously-skip-permissions --continue"
  );
});

// ---- composeLaunchTerminalName -------------------------------------------
t.testSync("composeLaunchTerminalName: canonical shape", () => {
  eq(
    composeLaunchTerminalName({
      task_id: "TASK-00031",
      task_type: "bug",
      priority: "P1",
    }),
    "TASK-00031 bug P1"
  );
});
t.testSync("composeLaunchTerminalName: round-trip with another type/priority", () => {
  eq(
    composeLaunchTerminalName({
      task_id: "TASK-00099",
      task_type: "spike",
      priority: "P0",
    }),
    "TASK-00099 spike P0"
  );
});

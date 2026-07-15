import {
  taskTerminalName,
  isTaskTerminal,
  isAnyTaskTerminal,
  orgHeaderName,
  orgHeaderBannerCommand,
} from "./terminals";
import { t, eq, ok, notOk } from "./test-harness";

t.setModule("terminals");

// ---- taskTerminalName ----------------------------------------------------
t.testSync("taskTerminalName: canonical format", () => {
  eq(taskTerminalName("TASK-00005", "refactor", "P2"), "TASK-00005 refactor P2");
});
t.testSync("taskTerminalName: empty type/priority is preserved (no crash)", () => {
  eq(taskTerminalName("TASK-00005", "", ""), "TASK-00005  ");
});
t.testSync("taskTerminalName: unusual but plausible inputs round-trip", () => {
  eq(taskTerminalName("TASK-99999", "spike", "P0"), "TASK-99999 spike P0");
});

// ---- isTaskTerminal ------------------------------------------------------
t.testSync("isTaskTerminal: matches canonical name", () => {
  ok(isTaskTerminal("TASK-00005 refactor P2", "TASK-00005"));
});
t.testSync("isTaskTerminal: matches bare id", () => {
  ok(isTaskTerminal("TASK-00005", "TASK-00005"));
});
t.testSync("isTaskTerminal: rejects longer numeric id (false-prefix bug)", () => {
  // Critical regression: TASK-000050 must NOT match TASK-00005.
  notOk(isTaskTerminal("TASK-000050 bug P1", "TASK-00005"));
});
t.testSync("isTaskTerminal: rejects different task", () => {
  notOk(isTaskTerminal("TASK-00006 feat P3", "TASK-00005"));
});
t.testSync("isTaskTerminal: rejects unrelated terminal name", () => {
  notOk(isTaskTerminal("toolbox-exec", "TASK-00005"));
});
t.testSync("isTaskTerminal: rejects ADB Dashboard", () => {
  notOk(isTaskTerminal("ADB Dashboard", "TASK-00005"));
});
t.testSync("isTaskTerminal: empty name does not match", () => {
  notOk(isTaskTerminal("", "TASK-00005"));
});
t.testSync("isTaskTerminal: id-as-substring-mid-name does not match", () => {
  // Defensive: "foo TASK-00005 bar" should NOT match — only leading-id is valid.
  notOk(isTaskTerminal("foo TASK-00005 bar", "TASK-00005"));
});

// ---- isAnyTaskTerminal ---------------------------------------------------
t.testSync("isAnyTaskTerminal: canonical task terminal", () => {
  ok(isAnyTaskTerminal("TASK-00031 bug P1"));
});
t.testSync("isAnyTaskTerminal: bare task id", () => {
  ok(isAnyTaskTerminal("TASK-00031"));
});
t.testSync("isAnyTaskTerminal: 6-digit id", () => {
  ok(isAnyTaskTerminal("TASK-100000 feat P2"));
});
t.testSync("isAnyTaskTerminal: rejects toolbox-exec", () => {
  notOk(isAnyTaskTerminal("toolbox-exec"));
});
t.testSync("isAnyTaskTerminal: rejects ADB Dashboard", () => {
  notOk(isAnyTaskTerminal("ADB Dashboard"));
});
t.testSync("isAnyTaskTerminal: rejects bash", () => {
  notOk(isAnyTaskTerminal("bash"));
});
t.testSync("isAnyTaskTerminal: rejects empty string", () => {
  notOk(isAnyTaskTerminal(""));
});
t.testSync("isAnyTaskTerminal: rejects non-numeric task id", () => {
  notOk(isAnyTaskTerminal("TASK-foo bug P1"));
});
t.testSync("isAnyTaskTerminal: rejects task-like prefix in middle", () => {
  notOk(isAnyTaskTerminal("foo TASK-00005"));
});
t.testSync("isAnyTaskTerminal: rejects 'TASK-' alone (no digits)", () => {
  notOk(isAnyTaskTerminal("TASK-"));
});
t.testSync("isAnyTaskTerminal: trailing chars without space rejected", () => {
  // "TASK-00005x" — x must be a space or end-of-string for the match.
  notOk(isAnyTaskTerminal("TASK-00005x"));
});

// ---- org-divider header helpers (WS-C) -----------------------------------
t.testSync("orgHeaderName: divider format with count", () => {
  eq(orgHeaderName("awslabs", 3), "━━ awslabs (3) ━━");
});
t.testSync("orgHeaderName: _local passes through as-is", () => {
  eq(orgHeaderName("_local", 1), "━━ _local (1) ━━");
});
t.testSync("orgHeaderBannerCommand: prints the banner then keeps the shell alive", () => {
  const cmd = orgHeaderBannerCommand("awslabs", 3);
  ok(cmd.includes("awslabs"));
  ok(cmd.includes("3"));
});
t.testSync("orgHeaderBannerCommand: singular ticket wording for count 1", () => {
  const cmd = orgHeaderBannerCommand("anthropics", 1);
  ok(cmd.includes("1 ticket") && !cmd.includes("1 tickets"));
});
t.testSync("isAnyTaskTerminal: does NOT match an org header (guards Close Terminals)", () => {
  // Critical regression guard: header terminals must not be swept as task terminals.
  notOk(isAnyTaskTerminal(orgHeaderName("awslabs", 3)));
});

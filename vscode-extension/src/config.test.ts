// Unit tests for the pure config normalizers. The extension glue
// (extension.ts) reads raw workspace-config values via getConfig().get<T>()
// and hands them here; these normalizers turn possibly-undefined/malformed
// input into typed, sane options for the pure planner and the terminal-env
// builder. No vscode import → runnable via the hand-rolled harness.
import {
  normalizeStartAllConfig,
  normalizeTmuxConfig,
  StartAllConfig,
  TmuxConfig,
} from "./config";
import { t, eq, deepEq } from "./test-harness";

t.setModule("config");

// ---- normalizeStartAllConfig --------------------------------------------
t.testSync("startAll: defaults when all undefined", () => {
  const c = normalizeStartAllConfig({});
  deepEq(c, {
    groupByOrg: true,
    orderByPriority: true,
    cap: 0,
    orgOrder: [],
  } as StartAllConfig);
});

t.testSync("startAll: negative cap clamps to 0 (unlimited)", () => {
  eq(normalizeStartAllConfig({ cap: -5 }).cap, 0);
});

t.testSync("startAll: non-finite cap → 0", () => {
  eq(normalizeStartAllConfig({ cap: NaN }).cap, 0);
});

t.testSync("startAll: fractional cap floors to integer", () => {
  eq(normalizeStartAllConfig({ cap: 3.7 }).cap, 3);
});

t.testSync("startAll: booleans pass through, orgOrder copied", () => {
  const c = normalizeStartAllConfig({
    groupByOrg: false,
    orderByPriority: false,
    cap: 3,
    orgOrder: ["awslabs", "_local"],
  });
  deepEq(c, {
    groupByOrg: false,
    orderByPriority: false,
    cap: 3,
    orgOrder: ["awslabs", "_local"],
  });
});

t.testSync("startAll: undefined orgOrder → []", () => {
  deepEq(normalizeStartAllConfig({ orgOrder: undefined }).orgOrder, []);
});

t.testSync("startAll: orgOrder is copied (defensive slice)", () => {
  const raw = ["a", "b"];
  const norm = normalizeStartAllConfig({ orgOrder: raw });
  raw.push("c");
  // The normalized value should not have observed the mutation.
  deepEq(norm.orgOrder, ["a", "b"]);
});

// ---- normalizeTmuxConfig -------------------------------------------------
t.testSync("tmux: defaults enabled + cc- prefix", () => {
  deepEq(normalizeTmuxConfig({}), {
    enabled: true,
    sessionPrefix: "cc-",
  } as TmuxConfig);
});

t.testSync("tmux: disabled honored", () => {
  eq(normalizeTmuxConfig({ enabled: false }).enabled, false);
});

t.testSync("tmux: empty prefix falls back to cc-", () => {
  eq(normalizeTmuxConfig({ sessionPrefix: "" }).sessionPrefix, "cc-");
});

t.testSync("tmux: whitespace-only prefix falls back to cc-", () => {
  eq(normalizeTmuxConfig({ sessionPrefix: "   " }).sessionPrefix, "cc-");
});

t.testSync("tmux: illegal prefix chars sanitized to dashes", () => {
  // tmux forbids '.' and ':' in names; keep the extension side conservative
  // so we never emit a prefix that breaks the argv. Runs of '-' are NOT
  // collapsed here — the CLI collapses on the full name (defence in depth).
  eq(normalizeTmuxConfig({ sessionPrefix: "a.b:c" }).sessionPrefix, "a-b-c");
});

t.testSync("tmux: legal prefix passes through unchanged", () => {
  eq(normalizeTmuxConfig({ sessionPrefix: "adb-" }).sessionPrefix, "adb-");
});

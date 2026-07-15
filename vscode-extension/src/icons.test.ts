import { iconForType, TYPE_ICONS } from "./icons";
import { t, eq } from "./test-harness";

t.setModule("icons");

// ---- iconForType: the 8-type Conventional set ----------------------------
t.testSync("iconForType: feat -> add", () => {
  eq(iconForType("feat"), "add");
});
t.testSync("iconForType: fix -> bug", () => {
  eq(iconForType("fix"), "bug");
});
t.testSync("iconForType: refactor -> wrench", () => {
  eq(iconForType("refactor"), "wrench");
});
t.testSync("iconForType: docs -> book", () => {
  eq(iconForType("docs"), "book");
});
t.testSync("iconForType: chore -> gear", () => {
  eq(iconForType("chore"), "gear");
});
t.testSync("iconForType: test -> beaker", () => {
  eq(iconForType("test"), "beaker");
});
t.testSync("iconForType: perf -> dashboard", () => {
  eq(iconForType("perf"), "dashboard");
});
t.testSync("iconForType: spike -> beaker", () => {
  eq(iconForType("spike"), "beaker");
});

// ---- legacy + fallback ---------------------------------------------------
t.testSync("iconForType: legacy bug -> bug (still maps for old entries)", () => {
  eq(iconForType("bug"), "bug");
});
t.testSync("iconForType: unknown type -> tasklist fallback", () => {
  eq(iconForType("banana"), "tasklist");
});
t.testSync("iconForType: undefined type -> tasklist fallback", () => {
  eq(iconForType(undefined), "tasklist");
});

// ---- TYPE_ICONS map covers the full set ----------------------------------
t.testSync("TYPE_ICONS covers all 8 Conventional types + legacy bug", () => {
  for (const type of ["feat", "fix", "refactor", "docs", "chore", "test", "perf", "spike", "bug"]) {
    eq(typeof TYPE_ICONS[type], "string", `TYPE_ICONS missing ${type}`);
  }
});

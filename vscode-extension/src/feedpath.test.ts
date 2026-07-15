// Unit tests for the pure file-tail fallback feed-path resolver. No vscode
// import → runnable via the hand-rolled harness. Pins that the dashboard's
// direct-file-read fallback points at the relocated .adb/events.jsonl (adb
// #186/#190), in lockstep with the Go side (internal/statedir).
import * as path from "path";
import { STATE_DIR, eventsLogPath } from "./feedpath";
import { t, eq } from "./test-harness";

t.setModule("feedpath");

t.testSync("STATE_DIR is .adb (matches internal/statedir.Name)", () => {
  eq(STATE_DIR, ".adb");
});

t.testSync("eventsLogPath resolves <home>/.adb/events.jsonl", () => {
  const home = path.join("/ws", "root");
  eq(eventsLogPath(home), path.join(home, ".adb", "events.jsonl"));
});

t.testSync("eventsLogPath no longer points at the legacy root path", () => {
  const home = path.join("/ws", "root");
  const got = eventsLogPath(home);
  eq(got === path.join(home, ".events.jsonl"), false);
});

// Pure resolver for the dashboard's file-tail fallback feed source. When the
// `adb events tail` subcommand isn't available, the extension tails the event
// log directly off disk. As of adb #186 that log lives under the workspace's
// .adb/ state directory (was <ADB_HOME>/.events.jsonl at the root). Keeping the
// path in one pure, vscode-free module lets the hand-rolled harness pin it and
// keeps it in lockstep with the Go side (internal/statedir.Path → .adb/<name>).
import * as path from "path";

// STATE_DIR mirrors internal/statedir.Name — adb's private per-workspace state
// directory under ADB_HOME.
export const STATE_DIR = ".adb";

// eventsLogPath returns the absolute path of the events JSONL the dashboard's
// file-tail fallback reads: <home>/.adb/events.jsonl. `home` is the resolved
// ADB_HOME (workspace root).
export function eventsLogPath(home: string): string {
  return path.join(home, STATE_DIR, "events.jsonl");
}

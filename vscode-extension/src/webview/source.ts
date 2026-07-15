// Data-source seam for the webview event feed. The extension glue picks a
// concrete implementation at open time; the pure feed model consumes lines
// from whichever one is wired up. This is the ONE spot F1 (adb events tail)
// and F4 (event log ⇄ steer) will plug into — everything downstream of
// `onLine` is vscode-free and unit-tested.
//
// Two impls ship in F3:
//   - CommandFeedSource: spawn `adb events tail --follow --json` and forward
//     stdout lines. Used once F1 lands (the CLI subcommand doesn't exist on
//     the base commit F3 branches from — see the plan's coordination note).
//   - FileTailFeedSource: poll ".events.jsonl" for appended lines. Fallback
//     that works TODAY on any adb workspace (the file is written by
//     internal/app.go regardless of whether F1's CLI is installed).
//
// Both live in the vscode glue (they touch child_process / fs); this module
// only defines the shared shape so callers depend on the interface, not the
// impl. The interface itself is pure — no runtime dependency, testable.

// Disposer returned from start(); the glue calls it on panel.onDidDispose.
export interface Disposable {
  dispose(): void;
}

// FeedSource emits raw JSONL lines (one event per line, matching the
// on-disk format of .events.jsonl and the --json shape of `adb events tail`).
// The webview's parseEventLine tolerates garbage, so the source doesn't
// have to filter — it just streams what it reads.
export interface FeedSource {
  // start begins streaming. onLine fires per raw line (no trailing newline).
  // onError fires for source-level failures (spawn error, missing binary,
  // fs error). Return a Disposable — calling dispose() must stop all I/O
  // and release any spawned child.
  start(
    onLine: (raw: string) => void,
    onError: (err: Error) => void
  ): Disposable;
}

// Kind of source the extension picked, useful for the "Waiting for events…"
// affordance and for future telemetry. Not user-facing today.
export type FeedSourceKind = "command" | "file-tail" | "stub";

// Pure helper: split a chunk of stdout/read data into complete lines,
// returning [lines, remainder] so a caller can hold the incomplete tail
// until the next chunk arrives. Kept in this pure module so future source
// impls (e.g. WebSocket, F4 push) share the same buffered-line logic.
export function splitLines(chunk: string, held: string): { lines: string[]; held: string } {
  const combined = held + chunk;
  const parts = combined.split(/\r?\n/);
  // The last element is the incomplete remainder if chunk didn't end with \n.
  const heldNext = parts.pop() ?? "";
  return { lines: parts, held: heldNext };
}

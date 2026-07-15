// Pure feed view-model for the webview dashboard. Parses JSONL event lines
// (matching the Go internal/observability.Event shape — see eventlog.go) into
// display-ready FeedItems, applies a newest-first bounded window.
//
// This module is DELIBERATELY vscode/fs-free so it unit-tests under `node`
// (repo pattern; see launch.ts / orggroup.ts). The extension glue in
// extension.ts owns the data source (a spawn of `adb events tail --follow
// --json` once F1 lands, or a fallback tail of .events.jsonl) and calls into
// these pure functions.

// Event mirrors the JSONL wire shape Go emits. `data` is deliberately loose
// (map[string]interface{} on the Go side) — helpers below extract known keys
// defensively. Timestamp is kept as the raw ISO string so the webview can
// format it however it likes.
export interface Event {
  timestamp: string;
  type: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: Record<string, any>;
}

// FeedItem is what the webview actually renders — a flat, one-line view.
export interface FeedItem {
  timestamp: string; // raw ISO
  type: string;
  taskId?: string; // extracted from data.task_id when present
  summary: string; // single-line human summary
}

// parseEventLine turns one JSONL line into an Event, or returns null if the
// line is empty, malformed, or doesn't look like an event object. Used by
// buildFeed and by the extension's live-tail loop for each incoming line.
export function parseEventLine(raw: string): Event | null {
  const s = (raw ?? "").trim();
  if (s.length === 0) {
    return null;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(s);
  } catch {
    return null;
  }
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    return null;
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const obj = parsed as Record<string, any>;
  if (typeof obj.type !== "string") {
    return null;
  }
  return {
    timestamp: typeof obj.timestamp === "string" ? obj.timestamp : "",
    type: obj.type,
    data:
      obj.data && typeof obj.data === "object" && !Array.isArray(obj.data)
        ? obj.data
        : {},
  };
}

// Per-type summary formatters. Keys are the canonical event type strings; any
// event whose type isn't in this map falls back to the generic formatter.
// Keeping this as a lookup (not a switch) so schema growth (F1) is a data-only
// change here — any type F1 declares that we don't format specially still
// renders via the fallback, no code change needed.
type SummaryFn = (data: Record<string, unknown>) => string;

const SUMMARY_BY_TYPE: Record<string, SummaryFn> = {
  "task.created": (d) => {
    const title = typeof d.title === "string" ? ` — ${d.title}` : "";
    return `task created${title}`;
  },
  "task.completed": () => "task completed",
  "task.status_changed": (d) => {
    const from = typeof d.old_status === "string" ? d.old_status : "?";
    const to = typeof d.new_status === "string" ? d.new_status : "?";
    return `status ${from} → ${to}`;
  },
  "task.archived": () => "task archived",
  "task.unarchived": () => "task unarchived",
  "task.priority_changed": (d) => {
    const from = typeof d.old_priority === "string" ? d.old_priority : "?";
    const to = typeof d.new_priority === "string" ? d.new_priority : "?";
    return `priority ${from} → ${to}`;
  },
  "task.deleted": () => "task deleted",
  "worktree.created": () => "worktree created",
  "worktree.removed": () => "worktree removed",
  "agent.session_started": () => "agent session started",
  "agent.session_ended": () => "agent session ended",
  "knowledge.extracted": () => "knowledge extracted",
  "issue.synced": (d) => {
    const dir = typeof d.direction === "string" ? ` (${d.direction})` : "";
    return `issue synced${dir}`;
  },
  "issue.conflict": () => "issue conflict",
  "issue.skipped": () => "issue skipped",
};

// singleLine collapses newlines/CR/whitespace runs so summaries never span two
// rendered lines even if the source data.title contains a newline.
function singleLine(s: string): string {
  return s.replace(/[\r\n]+/g, " ").replace(/\s+/g, " ").trim();
}

// eventToFeedItem lowers an Event into the flat FeedItem view-model. Never
// returns null — an unknown type still yields a rendered row (safer than
// silently dropping events we haven't taught the UI about).
export function eventToFeedItem(ev: Event): FeedItem {
  const summarize = SUMMARY_BY_TYPE[ev.type];
  const rawSummary = summarize ? summarize(ev.data) : `${ev.type}`;
  const summary = singleLine(rawSummary) || ev.type;
  const taskId = typeof ev.data.task_id === "string" ? ev.data.task_id : undefined;
  return {
    timestamp: ev.timestamp,
    type: ev.type,
    taskId,
    summary,
  };
}

// appendFeedItem prepends a new item and enforces the newest-first cap. cap<=0
// means unlimited (matches the "0 = unlimited" idiom used elsewhere in the
// extension config, e.g. adb.startAll.cap).
export function appendFeedItem(
  feed: readonly FeedItem[],
  next: FeedItem,
  cap: number
): FeedItem[] {
  const out = [next, ...feed];
  if (cap > 0 && out.length > cap) {
    out.length = cap;
  }
  return out;
}

// buildFeed parses a batch of raw JSONL lines (order-agnostic input) and
// returns the newest-first, cap-limited feed. Malformed lines are silently
// skipped — the JSONL log is intentionally forgiving (matches the Go-side
// ReadAll, which skips unparseable lines rather than failing the whole read).
export function buildFeed(lines: readonly string[], cap: number): FeedItem[] {
  const items: FeedItem[] = [];
  for (const raw of lines) {
    const ev = parseEventLine(raw);
    if (ev) {
      items.push(eventToFeedItem(ev));
    }
  }
  // Sort newest-first by timestamp string. ISO-8601 sorts lexicographically.
  items.sort((a, b) => (a.timestamp < b.timestamp ? 1 : a.timestamp > b.timestamp ? -1 : 0));
  if (cap > 0 && items.length > cap) {
    items.length = cap;
  }
  return items;
}

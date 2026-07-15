// Tests for the pure feed view-model (parses adb events → FeedItem, applies
// windowing/dedup, orders newest-first). No vscode/fs — runs under `node`.
import { t, eq, ok, deepEq, notOk } from "../test-harness";
import {
  parseEventLine,
  eventToFeedItem,
  appendFeedItem,
  buildFeed,
  FeedItem,
  Event,
} from "./feed";

t.setModule("feed");

// ---- parseEventLine: JSONL line -> Event | null --------------------------
t.testSync("parseEventLine: valid JSONL becomes an Event", () => {
  const raw = JSON.stringify({
    timestamp: "2026-07-01T00:00:00Z",
    type: "task.created",
    data: { task_id: "TASK-1" },
  });
  const ev = parseEventLine(raw);
  ok(ev, "expected an Event");
  eq(ev!.type, "task.created");
  eq(ev!.data.task_id, "TASK-1");
});

t.testSync("parseEventLine: garbage JSON returns null", () => {
  eq(parseEventLine("{not json"), null);
});

t.testSync("parseEventLine: empty/whitespace returns null", () => {
  eq(parseEventLine(""), null);
  eq(parseEventLine("   \n"), null);
});

t.testSync("parseEventLine: JSON that isn't an event object returns null", () => {
  // Missing type field is not a legal event.
  eq(parseEventLine(JSON.stringify({ timestamp: "2026-07-01T00:00:00Z" })), null);
  eq(parseEventLine(JSON.stringify(["array", "not", "object"])), null);
  eq(parseEventLine(JSON.stringify(42)), null);
});

t.testSync("parseEventLine: missing data becomes an empty object", () => {
  const raw = JSON.stringify({ timestamp: "2026-07-01T00:00:00Z", type: "task.created" });
  const ev = parseEventLine(raw);
  ok(ev, "expected an Event");
  deepEq(ev!.data, {});
});

// ---- eventToFeedItem: Event -> FeedItem ----------------------------------
t.testSync("eventToFeedItem: task.created maps to a task feed item", () => {
  const ev: Event = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "task.created",
    data: { task_id: "TASK-1", title: "hello world" },
  };
  const item = eventToFeedItem(ev);
  ok(item);
  eq(item.taskId, "TASK-1");
  eq(item.type, "task.created");
  eq(item.timestamp, "2026-07-01T00:00:00Z");
  ok(item.summary.length > 0, "expected a non-empty summary");
});

t.testSync("eventToFeedItem: unknown event type still yields a feed item (no crash)", () => {
  const ev: Event = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "totally.made.up",
    data: { some: "value" },
  };
  const item = eventToFeedItem(ev);
  ok(item);
  eq(item.type, "totally.made.up");
  // Unknown types get a generic summary; not empty.
  ok(item.summary.length > 0);
});

t.testSync("eventToFeedItem: task.status_changed reflects new status in summary", () => {
  const ev: Event = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "task.status_changed",
    data: { task_id: "TASK-2", old_status: "backlog", new_status: "in_progress" },
  };
  const item = eventToFeedItem(ev);
  eq(item.taskId, "TASK-2");
  ok(item.summary.includes("in_progress"), `expected summary to name new status; got: ${item.summary}`);
});

t.testSync("eventToFeedItem: no task_id in data → taskId is undefined", () => {
  const ev: Event = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "agent.session_started",
    data: {},
  };
  const item = eventToFeedItem(ev);
  eq(item.taskId, undefined);
});

// ---- appendFeedItem: windowed newest-first append ------------------------
t.testSync("appendFeedItem: newest goes to the front", () => {
  const older: FeedItem = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "task.created",
    taskId: "TASK-1",
    summary: "task created",
  };
  const newer: FeedItem = {
    timestamp: "2026-07-01T00:00:05Z",
    type: "task.status_changed",
    taskId: "TASK-1",
    summary: "task -> in_progress",
  };
  const feed = appendFeedItem([older], newer, 100);
  eq(feed.length, 2);
  eq(feed[0], newer, "newest first");
  eq(feed[1], older);
});

t.testSync("appendFeedItem: caps at maxItems, evicting oldest", () => {
  let feed: FeedItem[] = [];
  for (let i = 0; i < 5; i++) {
    feed = appendFeedItem(
      feed,
      {
        timestamp: `2026-07-01T00:00:0${i}Z`,
        type: "task.created",
        taskId: `TASK-${i}`,
        summary: `#${i}`,
      },
      3 // cap
    );
  }
  eq(feed.length, 3);
  // Newest first, oldest evicted.
  eq(feed[0].taskId, "TASK-4");
  eq(feed[1].taskId, "TASK-3");
  eq(feed[2].taskId, "TASK-2");
});

t.testSync("appendFeedItem: cap of 0 or negative treated as unlimited", () => {
  let feed: FeedItem[] = [];
  for (let i = 0; i < 4; i++) {
    feed = appendFeedItem(
      feed,
      {
        timestamp: `2026-07-01T00:00:0${i}Z`,
        type: "task.created",
        taskId: `TASK-${i}`,
        summary: `#${i}`,
      },
      0
    );
  }
  eq(feed.length, 4);
});

// ---- buildFeed: parse a batch of raw JSONL lines, cap the result ---------
t.testSync("buildFeed: parses multiple lines newest-first, ignores garbage", () => {
  const raws = [
    JSON.stringify({ timestamp: "2026-07-01T00:00:00Z", type: "task.created", data: { task_id: "TASK-1" } }),
    "{not json",
    JSON.stringify({ timestamp: "2026-07-01T00:00:05Z", type: "task.status_changed", data: { task_id: "TASK-1", new_status: "in_progress" } }),
    "", // blank
  ];
  const feed = buildFeed(raws, 10);
  eq(feed.length, 2, "garbage/blank lines skipped");
  eq(feed[0].timestamp, "2026-07-01T00:00:05Z", "newest first");
  eq(feed[1].timestamp, "2026-07-01T00:00:00Z");
});

t.testSync("buildFeed: applies cap after ordering", () => {
  const raws: string[] = [];
  for (let i = 0; i < 6; i++) {
    raws.push(
      JSON.stringify({
        timestamp: `2026-07-01T00:00:0${i}Z`,
        type: "task.created",
        data: { task_id: `TASK-${i}` },
      })
    );
  }
  const feed = buildFeed(raws, 3);
  eq(feed.length, 3);
  eq(feed[0].taskId, "TASK-5", "newest");
  eq(feed[2].taskId, "TASK-3", "oldest kept after cap");
});

t.testSync("buildFeed: empty input yields empty array", () => {
  deepEq(buildFeed([], 100), []);
});

// ---- summary is 1-line, doesn't contain raw newlines ---------------------
t.testSync("eventToFeedItem: summary has no newlines (single-line render)", () => {
  const ev: Event = {
    timestamp: "2026-07-01T00:00:00Z",
    type: "task.created",
    data: { task_id: "TASK-1", title: "line1\nline2" },
  };
  const item = eventToFeedItem(ev);
  notOk(item.summary.includes("\n"), "summary must be single-line");
});

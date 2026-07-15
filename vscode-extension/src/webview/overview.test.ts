// Tests for the pure per-org overview aggregator. Groups tasks by their org
// (reusing deriveOrg from ../orggroup) and produces per-status counts + a
// total per card, ordered so real orgs sort alphabetically and _local is last.
import { t, eq, deepEq, ok } from "../test-harness";
import { buildOverview, OverviewCard, OverviewTask } from "./overview";

t.setModule("overview");

// Small helper — build a task shape the aggregator expects (only the fields
// deriveOrg + status counting need). Extras are ignored.
function task(id: string, status: string, opts: Partial<OverviewTask> = {}): OverviewTask {
  return {
    id,
    status,
    priority: "P2",
    ...opts,
  };
}

t.testSync("buildOverview: empty task list yields empty cards", () => {
  deepEq(buildOverview([]), []);
});

t.testSync("buildOverview: groups by org via ticket_path", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "in_progress", {
      ticket_path: "/root/tickets/github.com/awslabs/mcp/TASK-1-foo",
    }),
    task("TASK-2", "backlog", {
      ticket_path: "/root/tickets/github.com/awslabs/mcp/TASK-2-bar",
    }),
    task("TASK-3", "done", {
      ticket_path: "/root/tickets/github.com/valter-silva-au/ai-dev-brain/TASK-3-baz",
    }),
  ];
  const cards = buildOverview(tasks);
  eq(cards.length, 2);
  // Alphabetical: awslabs before valter-silva-au.
  eq(cards[0].org, "awslabs");
  eq(cards[0].total, 2);
  eq(cards[1].org, "valter-silva-au");
  eq(cards[1].total, 1);
});

t.testSync("buildOverview: falls back to repo when no ticket_path", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "in_progress", { repo: "github.com/awslabs/mcp" }),
    task("TASK-2", "in_progress", { repo: "github.com/awslabs/other" }),
  ];
  const cards = buildOverview(tasks);
  eq(cards.length, 1);
  eq(cards[0].org, "awslabs");
  eq(cards[0].total, 2);
});

t.testSync("buildOverview: repo-less tasks bucket into _local, sorted last", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "in_progress", { ticket_path: "/root/tickets/github.com/aws/aws-cli/TASK-1-x" }),
    task("TASK-2", "backlog"), // no repo, no ticket_path → _local
    task("TASK-3", "review", { ticket_path: "/root/tickets/_local/TASK-3-y" }),
  ];
  const cards = buildOverview(tasks);
  eq(cards.length, 2);
  eq(cards[0].org, "aws");
  eq(cards[1].org, "_local", "_local sorts last");
  eq(cards[1].total, 2);
});

t.testSync("buildOverview: per-status counts are correct", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "in_progress", { repo: "github.com/awslabs/mcp" }),
    task("TASK-2", "in_progress", { repo: "github.com/awslabs/mcp" }),
    task("TASK-3", "backlog", { repo: "github.com/awslabs/mcp" }),
    task("TASK-4", "done", { repo: "github.com/awslabs/mcp" }),
    task("TASK-5", "review", { repo: "github.com/awslabs/mcp" }),
    task("TASK-6", "blocked", { repo: "github.com/awslabs/mcp" }),
    task("TASK-7", "archived", { repo: "github.com/awslabs/mcp" }),
  ];
  const [card] = buildOverview(tasks);
  eq(card.total, 7);
  eq(card.byStatus.in_progress, 2);
  eq(card.byStatus.backlog, 1);
  eq(card.byStatus.done, 1);
  eq(card.byStatus.review, 1);
  eq(card.byStatus.blocked, 1);
  eq(card.byStatus.archived, 1);
});

t.testSync("buildOverview: unknown status still counted (open extension)", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "wibble", { repo: "github.com/awslabs/mcp" }),
  ];
  const [card] = buildOverview(tasks);
  eq(card.total, 1);
  eq(card.byStatus.wibble, 1, "unknown status keys are preserved");
});

t.testSync("buildOverview: card shape includes zero-counts for known statuses", () => {
  // The webview wants to render "in_progress: 0" cells when a card has none,
  // so buildOverview seeds every canonical status key at 0.
  const tasks: OverviewTask[] = [task("TASK-1", "backlog", { repo: "github.com/x/y" })];
  const [card] = buildOverview(tasks);
  eq(card.byStatus.in_progress, 0);
  eq(card.byStatus.backlog, 1);
  eq(card.byStatus.done, 0);
  eq(card.byStatus.review, 0);
  eq(card.byStatus.blocked, 0);
  eq(card.byStatus.archived, 0);
});

t.testSync("buildOverview: card totals are stable regardless of input order", () => {
  const shuffled: OverviewTask[] = [
    task("TASK-3", "done", { repo: "github.com/awslabs/mcp" }),
    task("TASK-1", "in_progress", { repo: "github.com/awslabs/mcp" }),
    task("TASK-2", "backlog", { repo: "github.com/awslabs/mcp" }),
  ];
  const inOrder: OverviewTask[] = [
    task("TASK-1", "in_progress", { repo: "github.com/awslabs/mcp" }),
    task("TASK-2", "backlog", { repo: "github.com/awslabs/mcp" }),
    task("TASK-3", "done", { repo: "github.com/awslabs/mcp" }),
  ];
  const a = buildOverview(shuffled);
  const b = buildOverview(inOrder);
  deepEq(a, b);
});

t.testSync("buildOverview: multiple orgs sorted alphabetically, _local last", () => {
  const tasks: OverviewTask[] = [
    task("TASK-1", "in_progress", { repo: "github.com/valter-silva-au/x" }),
    task("TASK-2", "in_progress", { repo: "github.com/awslabs/y" }),
    task("TASK-3", "in_progress"), // _local
    task("TASK-4", "in_progress", { repo: "github.com/aws/z" }),
  ];
  const cards = buildOverview(tasks);
  const orgs = cards.map((c: OverviewCard) => c.org);
  deepEq(orgs, ["aws", "awslabs", "valter-silva-au", "_local"]);
});

t.testSync("buildOverview: invariant — sum of all card totals equals input length", () => {
  const tasks: OverviewTask[] = [];
  const orgs = ["awslabs", "aws", "microsoft", "valter-silva-au"];
  let n = 0;
  for (const org of orgs) {
    for (const status of ["in_progress", "backlog", "done"]) {
      tasks.push(
        task(`TASK-${n++}`, status, {
          repo: `github.com/${org}/repo`,
        })
      );
    }
  }
  // Add a couple of _local tasks.
  tasks.push(task(`TASK-${n++}`, "backlog"));
  tasks.push(task(`TASK-${n++}`, "review"));
  const cards = buildOverview(tasks);
  const sum = cards.reduce((acc: number, c: OverviewCard) => acc + c.total, 0);
  eq(sum, tasks.length);
});

t.testSync("buildOverview: card exposes a stable set of known status keys", () => {
  const tasks: OverviewTask[] = [task("TASK-1", "backlog", { repo: "github.com/x/y" })];
  const [card] = buildOverview(tasks);
  const keys = Object.keys(card.byStatus).sort();
  ok(keys.includes("in_progress"));
  ok(keys.includes("backlog"));
  ok(keys.includes("done"));
  ok(keys.includes("review"));
  ok(keys.includes("blocked"));
  ok(keys.includes("archived"));
});

import {
  deriveOrg,
  LOCAL_ORG,
  orgSortKey,
  sortByPriority,
  groupTasksByOrg,
  OrgGroup,
  planStartAllGrouped,
  GroupedStartAllPlan,
} from "./orggroup";
import { LaunchTask } from "./launch";
import { t, eq } from "./test-harness";

t.setModule("orggroup");

const task = (over: Partial<LaunchTask> = {}): LaunchTask => ({
  id: "TASK-00001",
  status: "backlog",
  ...over,
});

// ---- deriveOrg: ticket_path is authoritative ----
t.testSync("deriveOrg: nested ticket_path → org segment", () => {
  eq(deriveOrg(task({ ticket_path: "/x/tickets/github.com/awslabs/mcp/TASK-00001-slug" })), "awslabs");
});
t.testSync("deriveOrg: _local ticket_path → LOCAL_ORG", () => {
  eq(deriveOrg(task({ ticket_path: "/x/tickets/_local/TASK-00053-epic" })), LOCAL_ORG);
});
t.testSync("deriveOrg: _archived nested ticket_path still yields the org", () => {
  eq(deriveOrg(task({ ticket_path: "/x/tickets/_archived/github.com/aws-samples/foo/TASK-9-b" })), "aws-samples");
});
// ---- deriveOrg: fall back to repo ----
t.testSync("deriveOrg: platform-qualified repo → org", () => {
  eq(deriveOrg(task({ repo: "github.com/anthropics/claude" })), "anthropics");
});
t.testSync("deriveOrg: https repo url → org", () => {
  eq(deriveOrg(task({ repo: "https://github.com/awslabs/mcp.git" })), "awslabs");
});
t.testSync("deriveOrg: ssh repo url → org", () => {
  eq(deriveOrg(task({ repo: "git@github.com:aws-samples/x.git" })), "aws-samples");
});
// ---- deriveOrg: repo-less / unresolvable → LOCAL_ORG ----
t.testSync("deriveOrg: no ticket_path, no repo → LOCAL_ORG", () => {
  eq(deriveOrg(task({})), LOCAL_ORG);
});
t.testSync("deriveOrg: bare local repo path → LOCAL_ORG", () => {
  eq(deriveOrg(task({ repo: "/Users/v/some/local/repo" })), LOCAL_ORG);
});
t.testSync("deriveOrg: ticket_path wins over repo when both present", () => {
  eq(deriveOrg(task({ ticket_path: "/x/tickets/github.com/awslabs/mcp/TASK-1-s", repo: "git@github.com:other/x.git" })), "awslabs");
});
t.testSync("deriveOrg: ticket_path without a 'tickets' segment falls back to repo", () => {
  eq(deriveOrg(task({ ticket_path: "/somewhere/else/TASK-1-s", repo: "github.com/awslabs/x" })), "awslabs");
});
t.testSync("deriveOrg: loose bare org/repo (no host) → org (defensive)", () => {
  eq(deriveOrg(task({ repo: "awslabs/mcp" })), "awslabs");
});
t.testSync("deriveOrg: single-segment repo → LOCAL_ORG (no org derivable)", () => {
  eq(deriveOrg(task({ repo: "just-a-name" })), LOCAL_ORG);
});

// ---- orgSortKey ----
t.testSync("orgSortKey: alpha within real orgs", () => {
  const orgs = ["yt-dlp", "awslabs", "anthropics"];
  const sorted = [...orgs].sort((a, b) => orgSortKey(a).localeCompare(orgSortKey(b)));
  eq(sorted.join(","), "anthropics,awslabs,yt-dlp");
});
t.testSync("orgSortKey: _local sorts last", () => {
  const orgs = ["_local", "awslabs"];
  const sorted = [...orgs].sort((a, b) => orgSortKey(a).localeCompare(orgSortKey(b)));
  eq(sorted.join(","), "awslabs,_local");
});

// ---- sortByPriority ----
t.testSync("sortByPriority: P0 before P3, stable ties", () => {
  const tasks = [
    task({ id: "A", priority: "P3" }),
    task({ id: "B", priority: "P0" }),
    task({ id: "C", priority: "P2" }),
    task({ id: "D", priority: "P0" }),
  ];
  eq(sortByPriority(tasks).map((t) => t.id).join(","), "B,D,C,A");
});
t.testSync("sortByPriority: missing/unknown priority sorts after known ones, stable", () => {
  const tasks = [
    task({ id: "A" }),                 // no priority
    task({ id: "B", priority: "P1" }),
    task({ id: "C", priority: "PX" }), // unknown
  ];
  eq(sortByPriority(tasks).map((t) => t.id).join(","), "B,A,C");
});
t.testSync("sortByPriority: does not mutate input", () => {
  const tasks = [task({ id: "A", priority: "P3" }), task({ id: "B", priority: "P0" })];
  sortByPriority(tasks);
  eq(tasks.map((t) => t.id).join(","), "A,B");
});

// ---- groupTasksByOrg ----
t.testSync("groupTasksByOrg: groups by org, orders groups (_local last), sorts each P0→P3", () => {
  const tasks = [
    task({ id: "A", priority: "P2", ticket_path: "/x/tickets/github.com/awslabs/mcp/TASK-A-s" }),
    task({ id: "B", priority: "P0", ticket_path: "/x/tickets/github.com/awslabs/mcp/TASK-B-s" }),
    task({ id: "C", priority: "P1", repo: "github.com/anthropics/x" }),
    task({ id: "D", priority: "P0", ticket_path: "/x/tickets/_local/TASK-D-s" }),
  ];
  const groups: OrgGroup[] = groupTasksByOrg(tasks, []);
  eq(groups.map((g) => `${g.org}:${g.tasks.map((t) => t.id).join("")}`).join("|"),
     "anthropics:C|awslabs:BA|_local:D");
});
t.testSync("groupTasksByOrg: explicit orgOrder overrides alpha; unlisted orgs alpha after", () => {
  const tasks = [
    task({ id: "A", repo: "github.com/awslabs/x" }),
    task({ id: "B", repo: "github.com/anthropics/y" }),
    task({ id: "C", repo: "github.com/zzz/z" }),
  ];
  const groups = groupTasksByOrg(tasks, ["awslabs", "anthropics"]);
  eq(groups.map((g) => g.org).join(","), "awslabs,anthropics,zzz");
});
t.testSync("groupTasksByOrg: group count field matches tasks length", () => {
  const tasks = [
    task({ id: "A", repo: "github.com/awslabs/x" }),
    task({ id: "B", repo: "github.com/awslabs/y" }),
  ];
  const groups = groupTasksByOrg(tasks, []);
  eq(groups[0].tasks.length, 2);
});

// ---- planStartAllGrouped ----
const noneOpen = (_: LaunchTask) => false;
const noWorktree = (_: string) => false;

t.testSync("planStartAllGrouped: flat plan preserved + groups added", () => {
  const tasks = [
    task({ id: "A", priority: "P2", repo: "github.com/awslabs/x" }),
    task({ id: "B", priority: "P0", repo: "github.com/awslabs/y" }),
    task({ id: "C", priority: "P1", repo: "github.com/anthropics/z" }),
    task({ id: "D", status: "done", repo: "github.com/awslabs/w" }), // excluded (terminal)
  ];
  const plan: GroupedStartAllPlan = planStartAllGrouped(tasks, noneOpen, noWorktree, {
    cap: 0, groupByOrg: true, orderByPriority: true, orgOrder: [],
  });
  eq(plan.launchable, 3);
  eq(plan.toStart.length, 3);
  eq(plan.groups.map((g) => `${g.org}(${g.tasks.length}):${g.tasks.map((t) => t.id).join("")}`).join("|"),
     "anthropics(1):C|awslabs(2):BA");
});
t.testSync("planStartAllGrouped: groupByOrg=false → single synthetic group, priority sort still applies", () => {
  const tasks = [
    task({ id: "A", priority: "P3", repo: "github.com/awslabs/x" }),
    task({ id: "B", priority: "P0", repo: "github.com/anthropics/y" }),
  ];
  const plan = planStartAllGrouped(tasks, noneOpen, noWorktree, {
    cap: 0, groupByOrg: false, orderByPriority: true, orgOrder: [],
  });
  eq(plan.groups.length, 1);
  eq(plan.groups[0].tasks.map((t) => t.id).join(","), "B,A");
});
t.testSync("planStartAllGrouped: orderByPriority=false preserves planStartAll order within a group", () => {
  const tasks = [
    task({ id: "A", priority: "P3", repo: "github.com/awslabs/x" }),
    task({ id: "B", priority: "P0", repo: "github.com/awslabs/y" }),
  ];
  const plan = planStartAllGrouped(tasks, noneOpen, noWorktree, {
    cap: 0, groupByOrg: true, orderByPriority: false, orgOrder: [],
  });
  eq(plan.groups[0].tasks.map((t) => t.id).join(","), "A,B");
});
t.testSync("planStartAllGrouped: cap flows through (only capped toStart is grouped)", () => {
  const tasks = [1, 2, 3].map((i) => task({ id: `T${i}`, repo: "github.com/awslabs/x" }));
  const plan = planStartAllGrouped(tasks, noneOpen, noWorktree, {
    cap: 2, groupByOrg: true, orderByPriority: true, orgOrder: [],
  });
  eq(plan.toStart.length, 2);
  eq(plan.deferred, 1);
  eq(plan.groups.reduce((n, g) => n + g.tasks.length, 0), 2); // only started tasks grouped
});

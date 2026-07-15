// Pure org-derivation + grouping + ordering for Start-All. No vscode/fs import
// so it unit-tests outside the extension host (repo's pure-core pattern). The
// extension glue (extension.ts) turns these decisions into terminals.
import { LaunchTask, planStartAll, StartAllPlan } from "./launch";

// LOCAL_ORG is the bucket for repo-less tickets (tickets/_local/…). Sorts last.
export const LOCAL_ORG = "_local";

// orgFromTicketPath reads the org from a nested ticket_path by locating the
// "tickets" segment and reading the org as the segment one level below the
// platform: tickets/<platform>/<org>/<repo>/…  → org. An _archived layer
// (tickets/_archived/<platform>/<org>/…) is skipped first; _local → LOCAL_ORG.
function orgFromTicketPath(p: string): string | undefined {
  const segs = p.split(/[\\/]+/).filter(Boolean);
  const ti = segs.lastIndexOf("tickets");
  if (ti < 0) {
    return undefined;
  }
  let rest = segs.slice(ti + 1);
  if (rest[0] === "_archived") {
    rest = rest.slice(1);
  }
  if (rest[0] === LOCAL_ORG) {
    return LOCAL_ORG;
  }
  // rest = [platform, org, repo, TASK-…]; org is index 1.
  return rest.length >= 2 ? rest[1] : undefined;
}

// orgFromRepo extracts the org from a raw repo field. Handles github.com/org/repo,
// https URLs, and git@host:org/repo.git. Bare local paths (absolute or ./ ../)
// have no org → undefined. Mirrors the Go NormalizeRepoPath shape
// (internal/integration/worktree.go:86).
function orgFromRepo(repo: string): string | undefined {
  if (repo.startsWith("/") || repo.startsWith("./") || repo.startsWith("../")) {
    return undefined;
  }
  if (repo.startsWith("git@") && repo.includes(":")) {
    // git@github.com:org/repo.git → org/repo.git ; org is first after ':'
    const afterColon = repo.slice(repo.indexOf(":") + 1);
    const parts = afterColon.split("/").filter(Boolean);
    return parts.length >= 2 ? parts[0] : undefined;
  }
  // https://github.com/org/repo(.git) or bare github.com/org/repo
  const s = repo.replace(/^https?:\/\//, "");
  const parts = s.split("/").filter(Boolean);
  // platform-qualified: [host, org, repo…] → org at index 1.
  // loose bare org/repo (2 segments, no host) → org at index 0 (defensive).
  if (parts.length >= 3) {
    return parts[1];
  }
  if (parts.length === 2) {
    return parts[0];
  }
  return undefined;
}

// deriveOrg returns the org a task belongs to, for grouping. Prefers ticket_path
// (the canonical nested layout), falling back to the raw repo field. Repo-less /
// unresolvable → LOCAL_ORG.
export function deriveOrg(task: LaunchTask): string {
  if (task.ticket_path) {
    const o = orgFromTicketPath(task.ticket_path);
    if (o) {
      return o;
    }
  }
  if (task.repo) {
    const o = orgFromRepo(task.repo);
    if (o) {
      return o;
    }
  }
  return LOCAL_ORG;
}

// orgSortKey maps an org name to a sort key so a plain localeCompare puts real
// orgs alphabetically and pushes LOCAL_ORG to the end. We prefix a rank digit
// ("0" for real orgs, "1" for _local) rather than a punctuation char: under
// locale-aware comparison punctuation like "~" sorts BEFORE letters, so a "~"
// prefix would (wrongly) float _local to the top. A leading digit is
// locale-stable — "1…" always orders after "0…".
export function orgSortKey(org: string): string {
  return org === LOCAL_ORG ? "1" + org : "0" + org;
}

// Priority rank: lower = launched first. Unknown/absent priority ranks after all
// known ones (999) so mis-tagged tasks still launch, just last within the group.
const PRIORITY_RANK: Record<string, number> = { P0: 0, P1: 1, P2: 2, P3: 3 };

function rank(p?: string): number {
  return p && p in PRIORITY_RANK ? PRIORITY_RANK[p] : 999;
}

// sortByPriority returns a NEW array sorted P0→P3, stable within a priority
// (Array.prototype.sort is stable per ECMAScript 2019+, satisfied by Node ≥12).
export function sortByPriority(tasks: LaunchTask[]): LaunchTask[] {
  return [...tasks].sort((a, b) => rank(a.priority) - rank(b.priority));
}

// One org's launch group: the org name + its priority-ordered tasks. The count
// for the divider banner is tasks.length.
export interface OrgGroup {
  org: string;
  tasks: LaunchTask[];
}

// groupTasksByOrg buckets tasks by deriveOrg, sorts each bucket P0→P3, and orders
// the buckets by orgOrder (explicit prefix) then orgSortKey (alpha, _local last).
// orgOrder is the WS-D `adb.startAll.orgOrder` value (empty = pure alpha).
export function groupTasksByOrg(tasks: LaunchTask[], orgOrder: string[]): OrgGroup[] {
  const buckets = new Map<string, LaunchTask[]>();
  for (const task of tasks) {
    const org = deriveOrg(task);
    const list = buckets.get(org) || [];
    list.push(task);
    buckets.set(org, list);
  }
  const explicit = orgOrder.filter((o) => buckets.has(o));
  const rest = [...buckets.keys()]
    .filter((o) => !orgOrder.includes(o))
    .sort((a, b) => orgSortKey(a).localeCompare(orgSortKey(b)));
  const ordered = [...explicit, ...rest];
  return ordered.map((org) => ({ org, tasks: sortByPriority(buckets.get(org)!) }));
}

// Config injected by the extension glue (WS-D provides real values from settings;
// WS-C uses these defaults). Keeping it a param preserves purity — the planner
// never reads vscode config itself.
export interface StartAllGroupConfig {
  cap: number;
  groupByOrg: boolean;
  orderByPriority: boolean;
  orgOrder: string[];
}

export const DEFAULT_GROUP_CONFIG: StartAllGroupConfig = {
  cap: 0,
  groupByOrg: true,
  orderByPriority: true,
  orgOrder: [],
};

// GroupedStartAllPlan extends the flat StartAllPlan (same 5 fields, unchanged
// semantics) with an ordered `groups` view over ONLY the toStart slice.
export interface GroupedStartAllPlan extends StartAllPlan {
  groups: OrgGroup[];
}

// planStartAllGrouped runs the untouched flat planStartAll to get the launchable/
// capped toStart slice + counts, then groups+orders THAT slice. Config-injected so
// the planner stays pure (WS-D threads real settings values in).
export function planStartAllGrouped(
  tasks: LaunchTask[],
  hasOpenTerminal: (t: LaunchTask) => boolean,
  worktreeExists: (p: string) => boolean,
  cfg: StartAllGroupConfig
): GroupedStartAllPlan {
  const flat = planStartAll(tasks, hasOpenTerminal, worktreeExists, cfg.cap);
  let groups: OrgGroup[];
  if (cfg.groupByOrg) {
    if (cfg.orderByPriority) {
      groups = groupTasksByOrg(flat.toStart, cfg.orgOrder);
    } else {
      // Group by org but keep planStartAll's order inside each bucket (no sort).
      const ordered = groupTasksByOrg(flat.toStart, cfg.orgOrder);
      groups = ordered.map((g) => ({
        org: g.org,
        tasks: flat.toStart.filter((t) => deriveOrg(t) === g.org),
      }));
    }
  } else {
    const tasksForGroup = cfg.orderByPriority ? sortByPriority(flat.toStart) : flat.toStart;
    groups = [{ org: "", tasks: tasksForGroup }];
  }
  return { ...flat, groups };
}

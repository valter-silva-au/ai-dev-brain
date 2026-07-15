// Pure per-org overview aggregator. Groups the current adb task list by the
// org that owns them (deriveOrg from ../orggroup.ts — same rule Start-All
// uses, so the webview shows the same buckets), and produces per-status
// counts per card, ordered so real orgs sort alphabetically and _local is
// last.
//
// vscode/fs-free — the extension glue owns `listTasks()` and passes the
// snapshot in.
import { deriveOrg, LOCAL_ORG, orgSortKey } from "../orggroup";

// OverviewTask is the field-set the aggregator needs. AdbTask in extension.ts
// is a strict superset — the extension passes its list straight through.
export interface OverviewTask {
  id: string;
  status: string;
  priority: string;
  repo?: string;
  ticket_path?: string;
  worktree_path?: string;
}

// The 6 canonical statuses the extension already recognises (STATUS_ORDER in
// extension.ts). We seed each card's counts with these keys at 0 so the
// rendered card always has a fixed set of cells even for empty statuses.
const KNOWN_STATUSES = [
  "in_progress",
  "review",
  "blocked",
  "backlog",
  "done",
  "archived",
];

// OverviewCard is the flat per-org row the webview renders. `byStatus` is a
// dictionary rather than a fixed struct so unknown statuses (e.g. a future
// adb release adding one) still surface — the UI just prints whatever cells
// exist.
export interface OverviewCard {
  org: string;
  total: number;
  byStatus: Record<string, number>;
}

// buildOverview groups tasks by deriveOrg and counts per status. Card order:
// real orgs alphabetically, then _local last (orgSortKey — same discipline
// Start-All uses so the two views agree on ordering).
export function buildOverview(tasks: readonly OverviewTask[]): OverviewCard[] {
  const buckets = new Map<string, OverviewTask[]>();
  for (const task of tasks) {
    // deriveOrg's LaunchTask signature is a strict superset of what we need
    // (id/status/priority + optional repo/ticket_path/worktree_path).
    const org = deriveOrg(task);
    const list = buckets.get(org) || [];
    list.push(task);
    buckets.set(org, list);
  }
  const orgs = [...buckets.keys()].sort((a, b) =>
    orgSortKey(a).localeCompare(orgSortKey(b))
  );
  return orgs.map((org) => {
    const list = buckets.get(org)!;
    const byStatus: Record<string, number> = {};
    for (const s of KNOWN_STATUSES) {
      byStatus[s] = 0;
    }
    for (const t of list) {
      byStatus[t.status] = (byStatus[t.status] || 0) + 1;
    }
    return { org, total: list.length, byStatus };
  });
}

// Re-export LOCAL_ORG so callers building UI can label the card without
// re-importing orggroup themselves.
export { LOCAL_ORG };

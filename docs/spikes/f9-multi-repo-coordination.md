# Eval (F9, #213): multi-repo coordination — myrepos vs in-house; git-town adopt-or-not

> **Status:** complete — gates the F9 code. **Decisions:** (1) a **thin in-house
> registry over `backlog.yaml`**, not `myrepos`; (2) **do not adopt git-town** —
> it's a complementary tool that sits *under* adb's worktree/branch model, not a
> dependency. No cloud, no paid tier.

## The question

adb already spans many repos (each ticket carries a platform-qualified `repo`).
What's the lightest way to (a) know which repos a unit of work spans and (b) do a
correlated cross-repo fetch/pull — and should adb take on external multi-repo
tooling (`myrepos`) or a branch-lifecycle tool (`git-town`)?

## Registry: `myrepos` (`~/.mrconfig`) vs a thin in-house registry

| | `myrepos` (GPL) | In-house over `backlog.yaml` |
|---|---|---|
| Source of truth | a second file (`~/.mrconfig`) to author + keep in sync | `backlog.yaml` — already the single source of truth, one `repo` per ticket |
| Dependency | external binary + GPL | none |
| Ticket-awareness | none — a flat repo list | native — repos derive from tickets, so "which repos does this initiative span" is a query, not a maintained list |
| Cross-repo pull | `mr update` (all repos) | reuse the verified `repos pull` engine, scoped to a ticket/initiative's repo set |

**Decision: in-house registry.** adb already knows every repo from `backlog.yaml`,
so a `.mrconfig` would duplicate (and drift from) that truth and add a GPL
dependency for no new capability. The registry is therefore a *derivation* over
the backlog — distinct repos and which tickets reference each — not a stored
list. (adb could later *emit* an `.mrconfig` for interop with an existing
`myrepos` workflow, but that's export, not adoption.)

## Branch lifecycle: adopt `git-town` (MIT) or not

`git-town` automates a branch lifecycle (`hack`/`sync`/`ship`/`delete`) on top of
plain git. It manages **branches only** — it has no concept of worktrees or
tickets, so it sits *under* adb's layer.

**Decision: do not adopt.** adb's branch model is already established and
opinionated — a Conventional `<type>/<slug>` branch, one branch per worktree per
ticket, ADR-0002 issue-encoded when linked (F6), created/cut/reconciled by adb
(F1–F7). git-town's assumptions (a single working copy, its own parent-branch
metadata, its own sync/ship flow) overlap and conflict with that. Adopting it
would mean two tools fighting over branch creation and naming. It remains a fine
*optional companion* a developer can run **inside** a worktree for its `sync`/
`ship` ergonomics; adb neither requires nor integrates it. Kept MIT-clean:
reference only, nothing vendored.

## What F9 ships (gated by the eval)

1. **In-house repo registry** — `adb repos list` derives, from `backlog.yaml`,
   the distinct repos across non-archived tickets and which tickets span each
   (table + `--json`). Pure derivation over the backlog; no stored list.
2. **Correlated, ticket-aware cross-repo pull** — `adb repos pull` gains
   `--initiative <id>` / `--ticket <id>`, pulling only the repos that unit of
   work spans (reusing the verified `PullAllRepos` engine per repo). With neither
   flag it retains today's pull-everything behaviour.

Explicitly **out of scope** (per "ship last, least-certain bet"): a stored
registry file, `.mrconfig` emission, and any git-town integration — all deferred
unless a concrete need appears.

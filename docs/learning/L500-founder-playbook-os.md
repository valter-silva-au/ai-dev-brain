# L500 — The Founder-Playbook OS

> **Tier:** L500 (the product story) · **Goal after this page:** *"I can drive a
> business through the founder-playbook lifecycle with adb."*
>
> **Prereqs:** [L100 — Fundamentals](./L100-fundamentals.md) (the task/ticket model),
> and ideally [L400 — Architecture & Extending](./L400-architecture-and-extending.md)
> (interfaces/adapters, the event pipeline) — the founder-playbook layer is built with
> the exact same seams.
>
> L100–L400 describe adb as a **dev-tooling core**: tasks, worktrees, sync,
> observability. This tier describes the layer Increments 5–6 built **on top** of
> that core — the **Founder-Playbook OS**: one connected graph that models and
> **enforces** the Idea→MVP→Launch→Scale lifecycle across many businesses. Every
> command and threshold below is checked against the current `main`
> (`internal/cli/root.go`, `internal/core/`, `pkg/models/`).

---

## Table of contents

- [1. The mental model](#1-the-mental-model)
- [2. The entity graph: Org → Initiative → Ticket](#2-the-entity-graph-org--initiative--ticket)
- [3. The four stages and their gates](#3-the-four-stages-and-their-gates)
- [4. The typed graph + entity catalog](#4-the-typed-graph--entity-catalog)
- [5. PMF metrics](#5-pmf-metrics)
- [6. Three-tier config](#6-three-tier-config)
- [7. Automation + ingestion](#7-automation--ingestion)
- [8. Launch/Scale governance](#8-launchscale-governance)
- [9. The Claude Code plugin](#9-the-claude-code-plugin)
- [Honest limits](#honest-limits)

---

## 1. The mental model

The dev-tooling core answers *"how is this unit of work progressing?"* (a `Task`'s
`TaskStatus`). The Founder-Playbook OS answers a different question — *"how is this
**business** progressing?"* — and keeps the two orthogonal. Four invariants (the
architecture spine) shape everything here:

1. **Stage sits above status.** A dev ticket's `TaskStatus` is untouched; a startup
   **stage** lives on an **Initiative** (§2), orthogonal to any ticket.
2. **A gate is a durable evidence bundle + a blocking check.** Advancing a stage is
   blocked until the bundle passes; the evidence and the decision persist on the
   initiative (§3).
3. **One typed graph.** Org, Initiative, Ticket, ADR, Metric, CRM deal, ingested node …
   are typed nodes; correlations are typed edges declared in frontmatter and compiled
   into a rebuildable index (§4).
4. **Bridge, don't rebuild.** The harness ships as SKILL.md skills + subagents + a hook
   + an MCP server, and graduates to a Claude Code plugin (§9) — it never rebuilds an
   agent loop.

Everything is authored as a **triplet**: a pluggable **template** (the artifact) + a
**skill/agent** (produces it and adversarially pressure-tests it) + optionally an
**automation rule** (keeps it fresh).

## 2. The entity graph: Org → Initiative → Ticket

The business hierarchy is **Workspace → Organization → Initiative → Ticket**
(`pkg/models/stage.go`):

- **Organization** = a first-class business; defaults to the git-host org but may span
  git orgs. Registry: `orgs/index.yaml`.
- **Initiative** = belongs to exactly one org and **carries the stage**. Registry:
  `initiatives/index.yaml`. A ticket (L100) associates to an initiative via
  `Task.Initiative`, and the per-worktree `task-context.md` renders the initiative's
  stage + 1-hop graph neighbourhood.

The registries are the source of truth; the physical ticket path stays
`tickets/<platform>/<org>/<repo>/…` (zero migration) and the business graph is
*computed*, not read off the filesystem. Backing store: `storage.FileStageStore`
(`stagestore.go`), behind a `core` interface, wired in `app.go`.

```bash
adb org create "Acme"                     # id derived as a slug (acme); --git-host maps a git org
adb catalog show --kind orgs              # list them — NOT `adb org list`, see the note below
adb initiative create "Widget" --org acme # --org is required; lands at stage Idea. + list / show
adb initiative set-stage widget MVP       # direct set (no gate) — the gate is `adb stage advance`
adb task create "build the widget" --initiative widget   # associate a ticket
```

> **`adb org` carries two registries, and only one of them feeds this page.**
> `adb org create` writes the playbook registry above. Everything else under
> `adb org` (`init`, `list`, `show`, `update`, `adopt`, `move`, `archive`,
> `validate`) manages **v3 `.aidb` trust scopes** — a separate store that
> `adb initiative` cannot read. So `adb org list` will never show a playbook
> org, and `adb initiative --org` will not accept a trust scope; use
> `adb catalog show --kind orgs` to list playbook orgs.
>
> This is worth knowing because it was recently a hard blocker rather than a
> curiosity: the composed v3 root registers the legacy command tree with
> `IncludeOrg:false` (v3's `adb org` supersedes the legacy one), which
> unregistered `org create` along with it. `adb initiative create --org X` then
> failed with `organization "X" not found` and **no public command could create
> that org**, so `adb initiative`, `adb stage`, and every gate on this page were
> unreachable on a fresh workspace. `create` is now mounted onto the v3 `org`
> parent (TASK-00039 Q1), and `test/e2e/playbook_reachability_test.go` holds the
> whole path — create → initiative → gate — against the real binary.

## 3. The four stages and their gates

The stage set is fixed to the playbook's four (`pkg/models/stage.go` `ValidStages`):

```
Idea ──▶ MVP ──▶ Launch ──▶ Scale
```

Movement is via **`adb stage advance <initiative>`**, which runs a **hybrid gate**
(`internal/core/stagegate.go`, `stagemanager.go`) for the current transition:

- **Deterministic items** — file-evidence presence and **numeric metric thresholds**
  (read from `adb pmf` nodes, §5).
- **An adversarial verdict** — the devil's-advocate agent records `VERDICT: pass|fail`
  in `evidence/<item>.verdict.md`; a `fail` blocks. Absent → the item degrades to
  *pending* (never silently passes).

The gate **blocks by default**; a human can bypass with
`adb stage advance <id> --override --reason "…"` (D5), which logs a `stage.override`
event. Automations may advance **only on a clean pass** and may **never** override.

| Transition | Deterministic bar | Notes |
|------------|-------------------|-------|
| **Idea → MVP** | validation evidence bundle (problem-hypothesis, interviews, evidence-ledger, …) | scaffold with `adb initiative scaffold-evidence`; Mom-Test check via `adb initiative lint-interview` |
| **MVP → Launch** | **Sean-Ellis ≥ 40%** + **retention/effort ≥ 40%** (`internal/core/stagegate.go`) | the numeric PMF bar (#103, closed) |
| **Launch → Scale** | **net-revenue-retention ≥ 100%** + **growth ≥ 15%** | **human-only** (`HumanOnly: true`): an automation may never advance it or set an override; a human advances/overrides it normally (#137, D5) |

A clean pass or an override emits `stage.advanced` (an override additionally emits
`stage.override`) to **both** the dev event log **and** the separate governance stream
(§8).

## 4. The typed graph + entity catalog

**Generic typed edges (D6, #109).** Entities declare `links: [{type, target}]` in
frontmatter over a **closed** vocabulary — `relates_to`, `part_of`, `blocks`,
`depends_on`, `duplicates` (`pkg/models/edge.go`) — validated on write, tolerant on
read. `Task.BlockedBy` is folded onto `depends_on`. A derived index is persisted at
`graph/index.yaml` (gitignored, rebuildable). `core.GraphManager` + `storage.FileGraphStore`.

```bash
adb graph rebuild                 # recompute the index from frontmatter links
adb graph neighbors TASK-00042    # incident edges (--type to filter)
```

> **"Validated on write, tolerant on read" is enforced, not aspirational — but it
> only became true in TASK-00039.** `EdgeType.IsCanonical` shipped as "the validation
> primitive" with **no production caller**, so for the whole of #109's life an
> arbitrary type could enter the graph's source of truth: `adb schedule add
> --write-edge mentions:TASK-1` was accepted and later minted onto a task's
> frontmatter, and an extraction skill's `adb ingest propose` document could land
> anything with a non-empty `type`. `EdgeType.Validate` (one wrapper over
> `IsCanonical`, naming the set in its error) is now called at every surface that
> **accepts** an edge type: `--write-edge` and `--if-edge` at authoring time, the rule
> engine's output **plan** phase, `EntityProposal.Validate`, and the ingestion accept
> path. `adb adr new --relates-to` / `adb debt add --relates-to` need no gate — they
> hard-code `relates_to`, so no user-chosen type exists to check.
>
> **Read is untouched, deliberately.** A workspace whose frontmatter, `graph/index.yaml`,
> `automation/rules.yaml`, or ingestion review queue already carries a non-canonical
> type keeps loading, keeps listing, keeps traversing, and `graph rebuild` keeps
> persisting the edge verbatim — `adb graph neighbors <id> --type mentions` is how you
> find one. Only the *write* is refused: a legacy rule declaring `mentions` still shows
> in `schedule list` and fails as an `error` firing that writes nothing (the plan/apply
> split means a rejected edge does not leave a half-written artifact behind).

> **The event vocabulary had the opposite problem — enforced but undiscoverable.** The
> paragraph above is about **edge** types, where the gate was missing. `adb schedule add
> --on-event` had a gate from its first commit; what was wrong was that its error said
> *"see `adb events`"* and **no adb command prints `KnownEventTypes`**, so the pointer
> bottomed out at "read `internal/observability/schema.go`", and `Rule.Validate`'s comment
> asserted "the write surface does that" without naming which surface. Fixed in
> TASK-00039: the gate moved into `buildRuleFromFlags` beside the edge gate, its error
> names all 19 types, and its flag help derives from the same set (see subsystems.md for
> the shape). The check must stay **out** of `Rule.Validate`, and this is the load-bearing
> difference from the edge case: `RuleSet.Validate` runs on `FileRuleStore.Save`, so a
> single legacy rule would make `automation/rules.yaml` unwritable — the user could
> neither author new automation nor delete the offending rule. **Read tolerance here is
> about *save*, not load**, because `Load` never validated at all; a read-tolerance test
> that only exercised `schedule list` passed even with the check wrongly placed, and was
> caught only by writing that control.

**Entity catalog (Backstage-style, #128).** `adb catalog show` (`--json`, `--kind`)
generates an inventory of orgs / initiatives / tickets / ingested-nodes / metrics from
the registries + the graph, each annotated with graph degree
(`core.CatalogService`/`CatalogBuilder`).

The same graph is surfaced to agents dynamically via the MCP tools `graph_neighbors` /
`related_tickets` / `get_initiative` / `search_knowledge` (L300 §3).

## 5. PMF metrics

Product/PMF signals are **provenance-carrying graph nodes** (D11, #122), manual-entry
first, `part_of` an initiative (`storage.FileMetricStore`, `pkg/models/metric.go`):

```bash
adb pmf record --initiative widget --metric sean-ellis --value 42 --source survey   # a metric node
adb pmf list
```

The MVP→Launch and Launch→Scale gates (§3) read these nodes for their numeric
thresholds — the gate is source-agnostic, so a future connector that feeds metrics
Just Works.

## 6. Three-tier config

Config resolves **Global → Org → Repo**, most-specific wins:
**`.taskrc` (repo) > `orgs/<id>/config.yaml` (org) > `.taskconfig` (global) > defaults**
(#128). The optional org tier's id comes from `ADB_ORG` or the `org:` field in `.taskrc`;
absent → the historical two-tier merge, byte-identical. `MergedConfig` owns precedence
and reports each setting's source.

```bash
adb config show          # the merged view (+ where each value came from)
adb config get <key>
```

## 7. Automation + ingestion

**Declarative rule engine (D7, #119).** `automation/rules.yaml` encodes
`on {schedule|event} [if graph-condition] run {skill|exec} → write {artifact|edge}`.

```bash
adb schedule list
adb schedule add --name conformance-nightly --every 24h --run-exec "adb conformance check"   # a real rule (--name is required)
adb schedule run [name]                 # fire a rule / all time rules now
adb schedule dispatch --event <type>    # fire event rules for one event
```

`--on-event` takes a type from the **event** vocabulary (`observability.KnownEventTypes`,
19 of them — the same set `adb events` reads), and it is validated at **authoring** time:
an unknown type is rejected naming the whole set, and nothing is written. `dispatch
--event` is deliberately *not* gated, so a rules file that already carries an unknown type
stays fireable — see §4 for why read tolerance here is about *save* rather than load.

The `adb scheduler` daemon (L300) runs enabled time rules and, when
`automation.enabled`, an `automation-dispatch` job that drains the event log to fire
event rules. Skill actions are **recorded as request files** (no hard `claude`
dependency); exec actions run; edge/artifact outputs are idempotent.

**Conformance-drift (#128).** `adb conformance check` (`--json`, `--exit-code`) flags
stale-template / missing-file (vs a project's `.adb/template-manifest.yaml`, written by
`adb init project` and re-synced by `adb init update`) and dangling-org /
dangling-initiative registry references. It's the first real consumer of the rule engine
(the D7 rule above).

**Staged ingestion (D8, #120).** Connectors land **immutable `raw/`** artifacts with
provenance (source, hash, cursor) + dual-key dedup; an extraction skill proposes
entities/edges; a **confidence gate** auto-lands high-confidence proposals and queues
fuzzy ones for review.

```bash
adb ingest land … / raw / propose --file … / review / accept / reject
```

Accepted proposals land as typed graph edges (via the shared `EdgeWriter`) or ingested
nodes that join the catalog; provenance is preserved in `ingested/ledger.yaml`.
(`core.IngestManager` + `storage.FileRawStore`/`FileProposalStore`/`FileNodeStore`.)

## 8. Launch/Scale governance

Once a product is launched, adb adds the governance surface a scaling business needs.
All of these are `core` services behind interfaces, with file-backed registries in
`storage`.

- **Architecture decisions (#131).** `adb adr new|list|show|set-status` — MADR records
  in `docs/adr/NNNN-*.md` + `adr/index.yaml`, surfaced as `adr:NNNN` graph nodes in the
  catalog. A **spec-gate** hook (`hooks.spec_gate`) blocks guarded writes until an
  accepted ADR exists — fail-safe: a nil checker while enabled **blocks** (the
  `SpecGateConfig` in `internal/core/hookengine.go` fails safe-and-loud, not open).
- **Tech-debt (#131).** `adb debt add|list|resolve` — a lightweight registry
  (`storage.FileDebtStore`). Each item is a **`debt:DEBT-NNNN` graph node** and appears
  in `adb catalog --kind debt` with its graph degree, the same integration ADR has;
  `adb debt add --relates-to <target>` mints the `relates_to` edge toward the ticket the
  debt was incurred in. **Added in TASK-00039** — before it, debt was the only registry
  here that nothing read, which is precisely what made it look like a duplicate of the
  ticket model. A debt item is deliberately *not* a task: it carries `area` and an
  explicit `resolved` timestamp that `models.Task` has no field for, and it never gets a
  worktree, a branch, or a ticket directory.
- **Compliance & audit (#133).** `adb audit security` runs a deterministic control
  catalog (secret-scan config, `.env` hygiene, pre-commit, SLOs-defined) plus `manual`
  framework-attestation controls (`--framework`, `--json`, `--exit-code`).
  `adb compliance list|scaffold <soc2|gdpr|hipaa>` scaffolds embedded control-checklist
  packs.
- **SLOs (#133).** `adb slo set|list` — an SLO registry (`storage.FileSLOStore`).
- **GTM (#135).** `adb crm add|list|show|set-stage` — a MEDDPICC/Bowtie deal registry
  (funnel-ordered, 0–8 qualification score, `storage.FileCRMStore`).
  `adb gtm list|scaffold <positioning|moat>` — positioning-canvas / moat-narrative packs
  (7 Powers / NFX / a16z + switching-cost prompts). GTM and compliance packs share one
  generic scaffolder (`core/packs.go`).
- **The governance event stream (D19, #137).** `stage.advanced` / `stage.override`
  decisions are mirrored to a **separate** `.adb/governance.jsonl` — distinct from the
  high-volume dev telemetry in `.adb/events.jsonl` — so a compliance/audit reader sees
  governance without task/agent noise. `adb governance list` (`--json`) reads it
  (`app.GovernanceLog`, `StageManager.WithGovernanceLogger`). Both sinks are `App.StatePath`
  files under the workspace's `.adb/` state directory (`internal/statedir`, #186), so neither
  is at the workspace root any more.

## 9. The Claude Code plugin

The harness (embedded `agents/` + `skills/`, enumerated by `core.HarnessManifest`)
graduates from *files you install* to a **distributable Claude Code plugin** (D12 phase
2, #139):

```bash
adb harness build [dest]   # --version / --dry-run / --force
adb harness manifest       # print plugin.json (the identity)
```

`adb harness build` (`core.BuildPlugin`, `internal/core/plugin.go`) emits
`.claude-plugin/plugin.json` + `.claude-plugin/marketplace.json` + a `.mcp.json`
registering `adb mcp serve`, alongside the embedded `agents/` and `skills/` — an
installable **single-plugin marketplace**.

## Honest limits

- **`KnowledgeExtractor.ListAllKnowledge` still scans flat** (`internal/core/knowledge.go`).
  It walks `tickets/<id>/knowledge/`, **not** the nested
  `tickets/<platform>/<org>/<repo>/…/knowledge/` correlation layout — so its consumers,
  `adb wiki publish` and conflict detection, only see flat-path ticket knowledge today.
  (`adb context memory index` is unaffected — its `KnowledgeIndexer.IndexWorkspace` resolves each
  ticket via the nested-aware `ResolveTicketDir`.) Same flat-vs-nested gap #121 fixed for
  communications via a ticket-dir resolver; a future fix would make the extractor walk the
  nested tree.
- **Deferred in Increment 6** (honestly, in the issues): the niche-industry connector
  *builder* (the D8 ingestion pipeline is its substrate); a standalone switching-cost
  audit (it lives as prompts in the GTM moat pack).
- **Open design fork** (never became load-bearing): whether non-product initiatives
  (pure research/ops) run the same four stages or a lighter track. Working assumption:
  the same four; they just sit loosely.

---

## Where to go next

- **The commands in one table:** `docs/claude/subsystems.md` (command surface + package
  map + event schema).
- **The task/ticket atom these initiatives group:** [L100 — Fundamentals](./L100-fundamentals.md).
- **The seams this layer is built from** (interfaces/adapters, events, HOW-TOs):
  [L400 — Architecture & Extending](./L400-architecture-and-extending.md).

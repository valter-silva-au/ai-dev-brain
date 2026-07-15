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
adb org create "Acme"                     # id derived as a slug (acme); --git-host maps a git org. + list / show
adb initiative create "Widget" --org acme # --org is required; lands at stage Idea. + list / show
adb initiative set-stage widget MVP       # direct set (no gate) — the gate is `adb stage advance`
adb task create "build the widget" --initiative widget   # associate a ticket
```

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
  (`storage.FileDebtStore`).
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
  decisions are mirrored to a **separate** `.governance.jsonl` — distinct from the
  high-volume dev telemetry in `.events.jsonl` — so a compliance/audit reader sees
  governance without task/agent noise. `adb governance list` (`--json`) reads it
  (`app.GovernanceLog`, `StageManager.WithGovernanceLogger`).

## 9. The Claude Code plugin

The harness (embedded `agents/` + `skills/`, enumerated by `core.HarnessManifest`)
graduates from *files you install* to a **distributable Claude Code plugin** (D12 phase
2, #139):

```bash
adb plugin build [dest]   # --version / --dry-run / --force
adb plugin manifest       # print plugin.json (the identity)
```

`adb plugin build` (`core.BuildPlugin`, `internal/core/plugin.go`) emits
`.claude-plugin/plugin.json` + `.claude-plugin/marketplace.json` + a `.mcp.json`
registering `adb mcp serve`, alongside the embedded `agents/` and `skills/` — an
installable **single-plugin marketplace**.

## Honest limits

- **`KnowledgeExtractor.ListAllKnowledge` still scans flat** (`internal/core/knowledge.go`).
  It walks `tickets/<id>/knowledge/`, **not** the nested
  `tickets/<platform>/<org>/<repo>/…/knowledge/` correlation layout — so its consumers,
  `adb sync wiki` and conflict detection, only see flat-path ticket knowledge today.
  (`adb memory index` is unaffected — its `KnowledgeIndexer.IndexWorkspace` resolves each
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

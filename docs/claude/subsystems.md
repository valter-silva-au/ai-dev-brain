# adb — Subsystem & command reference

Reference companion to the root `CLAUDE.md`. Every claim here is checked against
`git ls-files`, `internal/cli/root.go`, and `internal/observability/schema.go`. If
you change the code, update this file (or the L-tier docs it points to).

## Package map (`internal/`)

| Package | What ships here |
|---------|-----------------|
| `internal/cli/` | Cobra commands. `root.go:NewRootCmd` registers every top-level command; `vars.go` holds the package-level singletons wired by `app.go`. |
| `internal/core/` | Business logic + the local interfaces (`BacklogStore`, `ContextStore`, `WorktreeCreator/Remover`, `EventLogger`, `SessionCapturer`) that decouple core from the outer layers. TaskManager, BootstrapSystem, ConfigurationManager, TemplateManager, AIContextGenerator, KnowledgeExtractor, ConflictDetector, HookEngine, ProjectInitializer, StageManager, GraphManager, RuleEngine (the D7 declarative automation engine + its RuleStore/ActionRunner/EdgeWriter/ArtifactWriter seams), IngestManager (the D8 staged-ingestion engine + its RawStore/ProposalStore/NodeStore seams), KnowledgeIndexer (indexes ticket knowledge + graph edges into vector memory for search_knowledge, #121). **Inc 5–6 governance/GTM services:** `ConfigurationManager` also resolves the three-tier Global→Org→Repo config merge (#128); `CatalogService`/`CatalogBuilder` (Backstage-style entity catalog, #128); `DriftChecker` (conformance-drift, #128); `ADRManager` (MADR ADRs + spec-gate, #131); `DebtManager` (tech-debt registry, #131); `SecurityAuditor` (`adb audit security` control catalog, #133); `SLOManager` (#133); `CRMManager` (MEDDPICC/Bowtie deals, #135); the generic pack scaffolder (`packs.go`, shared by the #133 compliance + #135 GTM template packs); the plugin builder (`plugin.go` `BuildPlugin`, #139). `StageManager` gained `WithGovernanceLogger`, `AdvanceOptions.Automated`, and the human-only Launch→Scale gate (#137, D5). `SerenaProvisioner` (`serena_provision.go`) auto-writes a per-worktree `.serena/project.yml` on the worktree-bootstrap seam using the `serena_langdetect.go` detector — idempotent, non-clobbering, fail-open; configures Serena only, never installs a language server (#201/#202). |
| `internal/storage/` | File-backed persistence, each behind a `core` interface: backlog (`backlog.go`), context/notes (`context.go`), communications (`communication.go`), captured sessions (`sessionstore.go`); plus the graph + founder-playbook stores: `FileStageStore` (orgs/initiatives + gate state, `stagestore.go`), `FileGraphStore` (derived edge index, `graphstore.go`), `FileRuleStore` (automation rules, `rulestore.go`), the ingestion stores `FileRawStore`/`FileProposalStore`/`FileNodeStore` (`ingeststore.go`), `FileMetricStore` (`metricstore.go`), and the Launch/Scale governance registries `FileADRStore`/`FileDebtStore`/`FileSLOStore`/`FileCRMStore`. `app.go` also wires a **separate** `GovernanceLog` at `.governance.jsonl` (see the event-schema note). |
| `internal/integration/` | External systems: git worktrees, CLI exec + alias resolution, Taskfile runner, terminal-tab renaming, screenshot/OCR, offline queue, Claude Code JSONL transcript parsing, version + MCP-health checks. Sub-packages `cloudsync/` and `issuesync/` (below). |
| `internal/observability/` | Append-only JSONL event log (`.events.jsonl`), on-demand metrics + alerting, and `schema.go` (the authoritative `KnownEventTypes` set). |
| `internal/hooks/` | Hook support library: generic `ParseStdin[T]`, the `.adb_session_changes` change tracker, context/status artifact helpers. |
| `internal/memory/` | Namespaced vector-memory store. SQLite backend (`sqlite_store.go`) + pluggable embedders (`embedder_fake.go`, `embedder_ollama.go`, `embedder_openai.go`). Surfaced by `adb memory`. |
| `internal/scheduler/` | Recurring background maintenance jobs (`jobs.go`, `scheduler.go`, persisted `state.go`). Surfaced by `adb scheduler`. |
| `internal/mcpserver/` | The adb MCP server (`server.go`), started by `adb mcp serve`. |
| `pkg/models/` | Shared domain types: Task/TaskType/TaskStatus/Priority (`task.go`), Config + `OrgConfig` (`config.go`), Communication (`communication.go`), session + knowledge models; plus the graph + founder-playbook types: Stage/Organization/Initiative + gate state (`stage.go`), `Link` + the closed edge vocabulary (`edge.go`), automation `Rule` (`rule.go`), ingestion provenance (`ingestion.go`), `Metric` (`metric.go`), catalog entities (`catalog.go`), ADR (`adr.go`), tech-debt (`debt.go`), audit controls (`audit.go`), SLO (`slo.go`), CRM deal (`crm.go`), plugin manifest (`plugin.go`), template manifest (`template_manifest.go`), drift findings (`drift.go`). |
| `templates/claude/` | `//go:embed` bundle (package `claude`, exported as `FS`). Six embed groups (`embed.go`): the root task-artifact templates (`*.md *.yaml *.sh rules/*.md` — `context.md`, `notes.md`, `design.md`, `handoff.md`, `status.yaml`, `task-context.md`, `adb-prompt.sh`, `rules/`), `projectinit/` (the `base`/`git`/`bmad` scaffolds, #86), `skills/` + `agents/` (the harness — the devil's-advocate agent + the `stage-gate`/`ingest-extract` skills, #100), `validation/` (the Idea/MVP validation pack, #104), and `compliance/` + `gtm/` (the control-checklist and GTM template packs, #133/#135). `HarnessManifest`/the plugin builder enumerate the `skills/`+`agents/` trees. |
| `vscode-extension/` | The `adb-brain` VS Code extension: command palette + tickets tree view + styled terminal tabs for adb tasks. |

### Integration sub-packages

- `internal/integration/cloudsync/` — S3 archive plane. Gitleaks secret scanning
  (`gitleaks.go`) + allowlist (`allowlist.go`) gate every push; `manifest.go`
  tracks synced content; `s3client.go` is the transport. Surfaced by `adb sync cloud`.
- `internal/integration/issuesync/` — bidirectional GitHub/GitLab issue sync. A
  `provider.go` abstraction with `github.go` / `gitlab.go` backends, `mapping.go`
  (ticket↔issue), `reconcile.go`, and `select.go`. Surfaced by `adb sync issues`.

## Command surface (`root.go:NewRootCmd`)

Top-level commands actually registered:

`task` · `session` · `sync` · `init` · `exec` · `run` · `metrics` · `alerts` ·
`events` · `chat` · `dashboard` · `hook` · `version` · `team` · `agents` · `mcp` ·
`prompt` · `memory` · `comm` · `repos` · `scheduler` · `schedule` · `ingest` · `org` · `initiative` · `stage` · `graph` · `pmf` · `config` · `catalog` · `conformance` · `adr` · `debt` · `audit` · `compliance` · `slo` · `crm` · `gtm` · `governance` · `plugin` · `status` · `work` · `serena`

> There is **no `adb serve`**. The dashboards are `adb dashboard` (TUI) and the
> VS Code in-editor webview. `adb mcp serve` starts the MCP server.

| Command | Purpose |
|---------|---------|
| `adb task` | Task lifecycle: `create`, `resume`, `start` (singular promote → in_progress, no launch, #210), `archive`, `unarchive`, `cleanup`, `delete` (wires TaskManager.Delete — worktree + ticket dir + backlog entry; requires `--yes`, #210), `status` (`--git` joins live worktree git state, #209), `priority`, `update`, `start-all`, `close-all`, `run-with-ruflo`, `normalize-titles`, `migrate-types` (+ hidden `migrate-blocked-by` — the `blocked_by`→`depends_on` graph migration). Issue-linked tickets get an ADR-0002-aware `<type>/<issue>-<slug>` branch (#210). |
| `adb session` | Captured Claude Code sessions: `save`, `ingest`, `capture`, `list`, `show`. |
| `adb sync` | `context`, `task-context`, `repos`, `claude-user`, `wiki` (publishes ticket knowledge as a navigable LLM-consumable corpus — graph cross-links, org/initiative namespacing, index/tag/initiative pages, `llms.txt` + `AGENTS.md`, opt-in semantic indexing — #127), `issues`, `cloud`, `all`. |
| `adb init` | `workspace`, `claude`, `project` (records a `.adb/template-manifest.yaml` provenance manifest — version + answers + per-file content hashes), `update` (copier/cruft-style re-sync of a scaffolded project to the current template version: three-way diff → added/updated/conflict/unchanged; dry-run by default, `--apply`/`--force`). |
| `adb exec` | Execute an external CLI with alias resolution + task env injection. |
| `adb run` | Run a Taskfile task. |
| `adb metrics` | Workspace metrics derived from the event log. |
| `adb alerts` | Active alerts (blocked/stale/long-review/backlog-size). |
| `adb events` | Inspect the structured event log (`digest`, `query`, `tail`). |
| `adb chat` | One-shot LLM chat seeded with live workspace context. |
| `adb dashboard` | TUI dashboard for metrics + alerts. |
| `adb hook` | Claude Code hook handlers: `install`, `status`, `pre-tool-use`, `post-tool-use`, `stop`, `task-completed`, `session-end`. |
| `adb team` | Launch multi-agent orchestration. |
| `adb agents` | List available specialized agents. |
| `adb mcp` | `serve` (start the MCP server), `check` (validate MCP server health). |
| `adb prompt` | Output a shell prompt prefix carrying task context. |
| `adb memory` | Namespaced vector store: `store`, `search`, `delete`, `list`, `index` (index ticket knowledge + graph edges so `search_knowledge` surfaces real content — #121), `export`, `import`. |
| `adb comm` | Stakeholder communications on a ticket (#121): `log` (with `--direction inbound\|outbound`), `list`. Stored as dated markdown under the ticket's `communications/`. |
| `adb repos` | Manage cloned repos under `<workspace>/repos`: `pull` (fetch + ff-only; `--initiative`/`--ticket` correlate the pull to just the repos that unit of work spans, #213), `list` (the in-house multi-repo registry derived from `backlog.yaml` — distinct repos + the tickets spanning each, `--json`, #213). |
| `adb scheduler` | Background maintenance daemon: `start`, `stop`, `restart`, `status`, `run`, `list`. Also runs every enabled time-triggered rule (D7) and, when `automation.enabled`, an `automation-dispatch` job that drains the event log to fire event rules. |
| `adb schedule` | Declarative automation rules (D7, `automation/rules.yaml`): `list`, `add`, `remove`, `run [name]` (fire a rule / all time rules now), `dispatch --event <type> [--data k=v]` (fire event rules for one event). |
| `adb ingest` | Staged ingestion pipeline (D8): `land` (immutable `raw/` landing + provenance/hash/cursor dedup), `raw` (provenance ledger), `propose --file` (confidence-gated: auto-land ≥ threshold, else queue), `review`/`accept`/`reject` (the review queue). Accepted proposals land as typed graph edges or ingested nodes; the `ingest-extract` skill authors proposals. |
| `adb org` | Founder-playbook organizations (businesses): `create`, `list`, `show`. |
| `adb initiative` | Founder-playbook initiatives: `create`, `list`, `show`, `set-stage`, `scaffold-evidence`, `lint-interview`. |
| `adb stage` | Stage gates: `advance` (blocks until required items pass — file evidence, **numeric metric thresholds** (D11), + the adversarial verdict; `--override --reason` for human bypass). MVP→Launch requires Sean-Ellis ≥40% + retention; **Launch→Scale** requires net-revenue-retention ≥100% + growth ≥15% and is **human-only** (D5) — an automation may never advance it or set an override, but a human advances/overrides it normally. |
| `adb governance` | Read the governance event stream (`.governance.jsonl`, #137): `list` (`--json`). stage.advanced/stage.override decisions kept **distinct from the dev-telemetry** event log (D19) so a compliance/audit reader sees governance without task/agent noise. |
| `adb plugin` | Graduate the harness to a Claude Code plugin (#139, D12 phase 2): `build [dest]` (`--version`/`--dry-run`/`--force`) emits `.claude-plugin/plugin.json` + `marketplace.json` + `.mcp.json` (registers `adb mcp serve`) + the embedded `agents/`+`skills/` — an installable single-plugin marketplace; `manifest` prints plugin.json. |
| `adb pmf` | Product/PMF metric nodes (D11): `record` (manual-entry metric against an initiative, a provenance-carrying graph node), `list`. Stage gates read these for numeric thresholds. |
| `adb graph` | Generic typed edge graph (D6): `rebuild` (derive the index cache from entity frontmatter links), `neighbors <id>` (incident edges, `--type` filter). |
| `adb config` | Inspect the layered config (Global → Org → Repo): `show` (tiers + active org + resolved custom settings, `--json`), `get <key>` (resolve one custom setting, `--source` names the winning tier). Precedence Repo > Org > Global; the org tier (`orgs/<id>/config.yaml`) is selected by `ADB_ORG` or `.taskrc`'s `org:` field. |
| `adb catalog` | Backstage-style generated entity catalog (#128): `show` (`--json`, `--kind orgs\|initiatives\|tickets\|nodes\|metrics\|adrs`). One read-only inventory of orgs/initiatives/tickets/ingested-nodes/metrics/ADRs derived from the registries + the #109 graph, each annotated with its graph degree. |
| `adb conformance` | Conformance-drift check (#128): `check` (`--json`, `--exit-code`). Flags `stale-template` / `missing-file` (vs the `.adb/template-manifest.yaml`) and `dangling-org` / `dangling-initiative` (registry reference integrity). Deterministic; a scheduled D7 rule drives it (`adb schedule add --name conformance-nightly --every 24h --run-exec "adb conformance check"`) — the first real consumer of the #119 rule engine. |
| `adb adr` | MADR architecture decision records (#131): `new <title>` (next-numbered `docs/adr/NNNN-<slug>.md` + `adr/index.yaml` entry, status proposed), `list`, `show <n>`, `set-status <n> <status>`. Each ADR is an `adr:NNNN` graph node and shows in `adb catalog`. The **spec-gate** hook (`hooks.spec_gate` in config) blocks guarded Write/Edit until an accepted ADR exists. |
| `adb debt` | Architecture-audit / tech-debt registry (#131, `debt/index.yaml`): `add <title>` (`--priority`/`--area`/`--note`), `list` (triage order: open first, then priority; `--open`), `resolve <id>`. Lightweight priority-triageable items, not tickets. |
| `adb audit` | Security/compliance posture audit (#133): `security` (`--json`, `--framework soc2\|gdpr\|hipaa`, `--exit-code`). Deterministic controls verify workspace facts (secret-scanning config, `.env` hygiene, pre-commit, SLOs); framework controls needing attestation report `manual`. Findings are pass/fail/warn/manual; `--exit-code` fails on any `fail`. |
| `adb compliance` | Compliance control-checklist packs (#133): `list`, `scaffold <soc2\|gdpr\|hipaa> [dest]` (`--dry-run`/`--force`). Scaffolds the framework's control docs (embedded `templates/claude/compliance/`) into `compliance/<framework>/`, idempotent/clobber-safe like the validation pack. |
| `adb slo` | Service-level objective registry (#133, `slo/index.yaml`): `set <name> --objective <n> [--window] [--description]` (upsert), `list`. The security audit's `slo-defined` control passes once SLOs are recorded. |
| `adb crm` | MEDDPICC/Bowtie sales-deal registry (#135, `crm/index.yaml`): `add <name>` (8 MEDDPICC flags + `--stage`), `list` (Bowtie-funnel order, MEDDPICC score), `show <id>`, `set-stage <id> <stage>`. Bowtie stages: awareness→education→selection→onboarding→impact→expansion. |
| `adb gtm` | Go-to-market template packs (#135): `list`, `scaffold <positioning\|moat> [dest]` (`--dry-run`/`--force`). Scaffolds a positioning/messaging canvas or a moat-narrative (7 Powers / NFX / a16z, switching-cost prompts) from embedded `templates/claude/gtm/` into `gtm/<pack>/`. |
| `adb serena` | Serena effectiveness telemetry (#203): `record` (non-interactive scorecard — `--verdict helped\|neutral\|hindered\|unused`, `--score 1..5`, `--used-for`/`--beat`/`--friction`/`--task` — emits one `serena.effectiveness_recorded` event) and `report` (rolls the event log up: counts by verdict, average score, recent entries; `--json`). |
| `adb work` | Worktree namespace (#210): `list` (task worktrees + branch + present/missing, `--json`), `switch <id>` (prints the worktree path as a `cd` target), `prune` (removes worktrees no active ticket owns, `--dry-run`/`--force`, respecting the #207 dirty guard), `reconcile` (#211: rebuilds missing worktrees from `backlog.yaml` — clone-on-demand + attach/recreate branch — so `work/`+`repos/` are rebuildable; `--prune`/`--force`/`--dry-run`). |
| `adb status` | Cross-repo status (#209): joins `backlog.yaml` with live per-worktree git state (branch, dirty, ahead/behind, worktree exists), `--json` or table, and flags missing/orphaned worktrees. `adb task status --git` produces the same enriched view over the (filterable) task list. |
| `adb version` | Version info. |

## Event schema (authoritative)

`internal/observability/schema.go` defines `KnownEventTypes` — the 20-element
contract every consumer (metrics, alerting, `adb events`, the VS Code webview)
relies on. Adding an event requires: declare the const, add it to
`KnownEventTypes`, and cover it in `TestKnownEventTypes_CoversEmittedSet`
(`schema_test.go`).

| Event type | Group | Notes |
|------------|-------|-------|
| `task.created` | task | payload: task_id, title, type, status, priority |
| `task.completed` | task | reserved |
| `task.status_changed` | task | old_status, new_status |
| `task.archived` | task | archived_at, archived_dir |
| `task.unarchived` | task | unarchived_at |
| `task.priority_changed` | task | old_priority, new_priority |
| `task.deleted` | task | deleted_at |
| `worktree.created` | worktree | task_id, path (emitted by TaskManager.Create for a repo-backed task) |
| `worktree.removed` | worktree | task_id, path (from cleanup / archive / delete) |
| `knowledge.extracted` | knowledge | reserved |
| `agent.session_started` | agent | task_id, worktree, bin, args |
| `agent.session_active` | agent | heartbeat: task_id, worktree, activity |
| `agent.session_ended` | agent | +optional error |
| `issue.synced` | issue-sync | repo, provider, action, reason |
| `issue.conflict` | issue-sync | action?, reason?, error? |
| `issue.skipped` | issue-sync | repo |
| `stage.advanced` | stage | initiative_id, from, to, overridden (gate advance across Idea→MVP→Launch→Scale, #90/#137) |
| `stage.override` | stage | initiative_id, from, to, reason (human-only bypass of a blocked gate, #90) |
| `config.task_context_synced` | config | task_id, trigger (emitted by `adb task resume` when it re-renders a worktree's Tier-0 task-context.md, #155) |
| `serena.effectiveness_recorded` | serena | verdict, score, used_for, beat, friction, task_id? (emitted by `adb serena record`, rolled up by `adb serena report`, #203) |

> **Governance stream (D19/#137):** `stage.advanced` / `stage.override` are *also*
> mirrored to a **separate** append-only `.governance.jsonl` (read via `adb governance`)
> so a compliance/audit reader sees governance decisions without the high-volume
> task/agent telemetry. Same two event types, written to a second sink — not new types.
> Separately, `adb sync cloud`'s `cloud.sync_*` events remain declared locally and are
> **not** in `KnownEventTypes` (see L300/L400).

## Deeper references (accurate, maintained)

- Task model, statuses, the task-type taxonomy (8 code + 2 non-code), branch shape, correlation layout —
  `docs/learning/L100-fundamentals.md`.
- Daily loop, VS Code extension, event streaming, backlog maintenance —
  `docs/learning/L200-daily-workflows.md`.
- Issue sync, cloud sync, the MCP server — `docs/learning/L300-integrations.md`.
- Layered architecture, interfaces/adapters, event pipeline, and the HOW-TOs for
  adding a command / event type / sync provider —
  `docs/learning/L400-architecture-and-extending.md`.
- The **Founder-Playbook OS** — Org→Initiative→Stage lifecycle, the four gates
  (Idea→MVP→Launch→Scale, incl. the human-only D5), the #109 graph + catalog, the
  three-tier config, the D7 rule engine + conformance-drift, D8 ingestion, ADR /
  tech-debt / compliance / audit / SLO governance, CRM/GTM, the governance stream,
  and the Claude Code plugin — `docs/learning/L500-founder-playbook-os.md`.

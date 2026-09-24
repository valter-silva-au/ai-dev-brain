# Can AI Dev Brain (ADB) Bootstrap an LLM-Wiki Monorepo? — Readiness Assessment

_Analyst report. Date: 2026-07-06. Repo audited: `github.com/valter-silva-au/ai-dev-brain` (local: `C:\Users\valte\Code\valter\repos\github.com\valter-silva-au\ai-dev-brain`)._

---

## 1. Executive Summary

**What ADB is.** AI Dev Brain (`adb`) is a Go CLI that wraps AI coding assistants (chiefly Claude Code) with a stateful "brain" layer: per-task context, task-lifecycle automation over isolated git worktrees, quality-gate hooks, knowledge extraction, and observability. Its shipped surface today is deliberately lean — "one repository, two shipped artifacts": a Go CLI/core that owns all state, and a thin VS Code extension that owns none (`docs/learning/L400-architecture-and-extending.md`). The core value proposition is making AI sessions *stateful* so "the AI never starts from scratch again".

**Verdict on readiness to bootstrap an LLM-wiki monorepo: NOT READY as-is, but a strong foundation.** ADB already ships the read-side of an LLM-consumable knowledge base (auto-generated `CLAUDE.md`, task-context templates, a frontmatter wiki via `adb sync wiki`), a real hook engine, and genuine bidirectional integrations (issue sync, S3 cloud sync, repo sync, a vector-memory substrate). What it lacks are the exact primitives a multi-org/multi-project "LLM wiki" needs:

- **No first-class Organization or Project tenant** carrying config/templates/owners. (A `Project` struct exists in `pkg/models/hive.go` but is dead, unwired code left over from the retired hive-mind.)
- **No org/project config tier** — only two-level global + per-repo config.
- **No pluggable template-set / scaffold-profile mechanism** — the `ProjectInitializer` ignores its injected `embed.FS` and hardcodes Go-only inline strings.
- **No cross-repo/cross-org wiki aggregation, indexing, cross-linking, or entity/topic model** — the wiki is a flat per-task page dump rooted in a single workspace.
- **No knowledge-base entity templates** (stakeholders, systems, ADRs, glossary, communications, ingestion sources) — only task-scoped templates exist.
- **No real harness distribution** — the documented 18 agents / 23 skills / embedded skills+agents+hooks do **not exist in the repo**; `adb team` launches no agent, only writes a plan file.
- **Severe CLAUDE.md doc-vs-reality drift** that would poison any wiki ingesting it as ground truth.

The good news: every gap maps to a mature, citable external pattern (Backstage catalog, C4/arc42/ADR, Copier/cruft/projen, Claude Code plugins, llms.txt, contextual RAG) that ADB's existing architecture can absorb without a rewrite. This report specifies exactly what to build.

---

## 2. What "LLM Wiki" Means Here + Target End-State

**The phrase "LLM wiki" is not defined anywhere in the repo.** A grep for `llm-wiki`/`llm wiki` returns no matches across all docs. The closest existing concept is ADB's **wiki pipeline**: `adb sync wiki` publishes each completed task's extracted knowledge (decisions, learnings, gotchas) as markdown pages with YAML frontmatter, default output `<workspace>/docs/wiki/knowledge` (`internal/core/wikipublisher.go`, surfaced by the `KnowledgeExtractor`). In the retired hive-mind vision this scaled to a cross-project "Knowledge Aggregator" indexing all 30+ repos into one searchable store.

So **"LLM wiki" here = an AI-consumable, auto-extracted, cross-project knowledge base** — a wiki written and read primarily by LLM agents rather than humans, with humans reviewing.

**Target end-state (as framed for this assessment):** a single **monorepo of many organizations, each with many projects**, all sharing **one updatable pattern-set** that covers these knowledge domains:

| Domain | Meaning |
|---|---|
| Stakeholders / entities | People, teams, orgs, contacts — with structured identity and relationships |
| Systems | Service/component/resource inventory with ownership and dependencies |
| Architecture | C4-style decomposition + ADRs + arc42-style docs |
| Design | Per-system/feature design docs (beyond task-scoped `design.md`) |
| Sales | Leads, deals, opportunities, pipeline, revenue |
| Comms (in/out) | Incoming and outgoing communications, threaded, with action items |
| Data ingestion | Connectors, sources, datasets, freshness/dedup, multimodal (screenshots/transcripts) |
| Org/project multi-tenancy | An org→project hierarchy that can be enumerated, configured, and templated |

ADB's author already gestures at this: the "Great Migration" converged scattered state into one git-tracked `ADB_HOME` workspace with 30+ repos cloned and gitignored beneath it, plus a conceptual Project Registry (`projects/index.yaml`) and Agent Registry (`agents/index.yaml`). Critically, both the multi-agent Hive Mind and the `adb serve` web UI are explicitly marked **RETIRED / HISTORICAL (2026-07-01)** — prototyped in `internal/hive/` and removed (TASK-00053, PR #76). The shipped end-state today is the *single-brain workspace*; the org→project hierarchy and cross-repo aggregation are aspirational, not built.

---

## 3. ADB Today — THE GOOD

**Clean, layered architecture with a real DI container.** Strict CLI → Core → Storage/Integration/Observability layering where Core defines interfaces and knows nothing of the other packages; `internal/app.go:NewApp` is the single DI container wiring concrete implementations behind adapter structs to prevent import cycles (`docs/learning/L400-architecture-and-extending.md`).

**A real hook engine matching the 5 documented hook types.** `ProcessPreToolUse` (blocking), `PostToolUse`, `Stop`, `TaskCompleted` (two-phase: blocking quality gates then non-blocking knowledge extraction), `SessionEnd` all exist in `internal/core/hookengine.go` (lines 134/260/290/332/403; phases at 574/598). `adb hook install`/`adb hook status` genuinely deploy/track 5 wrapper scripts (`internal/cli/hook.go:52-58,99-105`).

**A nested "correlation layout" that physically supports multi-org/multi-repo.** `resolveTaskDir` nests tickets under `tickets/<platform>/<org>/<repo>/TASK-id-slug` with mirrored worktrees under `work/...` (`internal/core/bootstrap.go:119-140`); repo IDs are canonicalized once via `NormalizeRepoPath` and reused as both path prefix and `Task.Repo` (`internal/core/taskmanager.go:167-247`). "The path is the correlation" (`docs/learning/L100-fundamentals.md`).

**Genuine, well-architected integrations.**
- **Issue sync** — provider-agnostic `Provider` interface with GitHub/GitLab implementations shelling out to `gh`/`glab`, a pure I/O-free `Reconcile` engine using last-writer-wins over a synced-fields allowlist with a stored `SyncHash` baseline (`internal/integration/issuesync/reconcile.go:96-136`, `provider.go:50-61`).
- **Cloud sync** — S3 push/pull over a deny-first allowlist with a fail-closed `gitleaks` scan and path-traversal hardening (`internal/integration/cloudsync/sync.go:40-231`).
- **Repo sync** — parallel `git fetch --all --prune` / `git pull --ff-only` (`internal/cli/repos.go:25-75`, `internal/integration/reposync_walk.go`).

**A namespaced vector-memory substrate already exists.** HNSW over SQLite with pluggable embedders (Ollama/OpenAI/fake), keyed by `(namespace, key)` (`internal/memory/memory.go:1-55`). This is a ready foundation for semantic wiki indexing — though it is currently *not wired* to the wiki publisher.

**A clean, deterministic wiki publisher.** `WikiPublisher` is a pure, side-effect-free transform with an injectable clock, sorted task IDs, and empty-knowledge skipping; frontmatter shape (`title/created/updated/tags/source`) is portable and drops cleanly into Obsidian/Foam-style file wikis (`internal/core/wikipublisher.go:24-105`).

**A rich Communications domain model.** `pkg/models/communication.go` (fields `From`, `To[]`, `Subject`, `Content`, `Tags` enum, `Channel`, `ThreadID`, `References`, `Attachments`, embedded `ActionItem[]`, `Metadata`) with a complete file-backed CRUD store (`internal/storage/communication.go:16-212`). This maps almost exactly onto a CRM interaction log.

**Good drift hygiene where it counts.** The retired Hive-Mind design carried an explicit RETIRED/HISTORICAL banner pointing to PR #76 until it was removed from the public tree (TASK-00070) — an exemplary way to mark stale design.

**Well-structured learning docs.** The L100–L400 tiers (`docs/learning/`) are accurate, current (post-2026-07-01 overhaul), and teach genuine extension points.

---

## 4. ADB Today — THE BAD (Drift / Broken / Stale)

> This section reports only items whose audit evidence is concrete (file-cited) and, where an adversarial verify pass ran, whose verdict was **CONFIRMED** or **PARTIAL**. Where the verify pass softened a claim, the nuance is stated.

### 4a. CLAUDE.md doc-vs-reality drift (prominent)

`CLAUDE.md` reads like documentation for an earlier, larger version of `adb`. It actively misleads on file locations, command surface, toolchain, and — most dangerously for a wiki — describes an entire harness that **does not exist**.

**The documented harness is largely fictional (`harness-skills-agents` audit):**
- **23 skills DO NOT EXIST.** `CLAUDE.md:126,491-514` list 23 skills (build, test, lint, quick-spec, quick-dev, adversarial-review, knowledge-extract…) under `.claude/skills/`. There is no `.claude/` dir, no `templates/claude/skills/` dir; `embed.go` embeds no skills. `find` for any `skills/` dir returns nothing.
- **18 agent definitions DO NOT EXIST.** `CLAUDE.md:125,472-489` (repeated in `docs/RESEARCH.md:434-435`) list 18 named BMAD agents (team-lead, analyst, product-owner, design-reviewer, scrum-master, go-tester, code-reviewer…). No agent markdown files exist anywhere.
- **`adb agents` prints a *different*, hardcoded set** unrelated to the 18: `backend-dev, frontend-dev, devops, test-engineer, qa-lead, automation, ux-designer, ui-designer, researcher, architect, tech-lead, sre, security` (`internal/cli/team.go:118-144`).
- **`adb team` launches no agent and no Claude process.** It validates a hardcoded team map, writes a static `orchestration-plan.md`, and tells the human to execute it manually (`internal/cli/team.go:24-103`). Multi-agent "teams" are non-functional.
- **Embedded `hooks/`, `artifacts/`, `checklists/`, `statusline.sh` dirs claimed by `CLAUDE.md:118-141` do not exist.** Hook wrapper scripts are inline Go constants in `internal/cli/hook.go`, not embedded templates. BMAD artifacts are inline strings in `projectinit.go`, producing 5 docs and one checklist (`quality-gates.md`), not the five named checklists or an `epics` artifact.
- **Stale filenames:** `CLAUDE.md:175` names `initclaude.go` and `syncclaudeuser.go` (real files are `init.go` / `sync.go`); names embed package `claudetpl` (real package is `claude`, `templates/claude/embed.go:1`).

**Retired `adb serve` still documented as live (`drift-quality` audit).** `CLAUDE.md:370` lists `adb serve [--port 8400] [--tv]`, but commit `803e0b4` (PR #76) deleted `serve.go`, `internal/server/`, and `internal/hive/`; no `NewServeCmd` is registered in `root.go`.

**Structure tree is wrong.** `CLAUDE.md:38-79` lists ~14 nonexistent `internal/cli` files (`feat.go`, `resume.go`, `archive.go`, `status.go`, `agents.go`, `worktreehook.go`…) and nonexistent `internal/core` files (`doctemplates.go`, `updategen.go`, `branchformat.go`, `sessioncapturer.go`, `eventlogger.go`…). Meanwhile entire *shipped* subsystems are undocumented: `internal/memory/`, `internal/integration/cloudsync/`, `internal/integration/issuesync/`, `internal/scheduler/`, `internal/mcpserver/`, and the `vscode-extension/`.

**Toolchain/metadata wrong.** `CLAUDE.md:25` claims "Go 1.26" but `go.mod` says `go 1.25.5` (CI pins 1.25). Claims "16 property test files incl. hooks"; actual is 6, none in hooks. Tech-stack list omits `aws-sdk-go-v2`, `bubbletea`, `mcp-go`, `modernc.org/sqlite`, `coder/hnsw`. `.claude/` and `.mcp.json` are heavily documented but not tracked in the repo.

**`UpdateGenerator` / `ScreenshotPipeline` / `WikiUpdate` / `RunbookUpdate` documented but absent.** `internal/core/updategen.go` does not exist; `screenshot.go` is capture-only (no OCR/classify); no `WikiUpdate`/`RunbookUpdate` Go types exist.

### 4b. Bootstrap/template defects (verified)

- **No org/project config tier (CONFIRMED).** `pkg/models/config.go:91-124` defines exactly `GlobalConfig` + `RepoConfig`; `MergedConfig` combines only those two. A shared cross-org pattern must be global or duplicated per-repo — no per-org overlay.
- **`FileProjectInitializer` ignores its `embed.FS` (CONFIRMED).** `templatesFS` is declared/assigned (`projectinit.go:28,32,34`) but **never read**; every scaffolded file is a hardcoded inline Go string literal (`projectinit.go:148-465`). Templates are neither pluggable nor swappable.
- **Scaffolded `.taskrc` hardcodes Go tooling (CONFIRMED).** Both `init workspace` (`init.go:117-118`) and `init project` (`projectinit.go:182-183`) write `go build ./...` / `go test ./... -count=1` verbatim, regardless of language.
- **`init workspace` omits the `repos/` baseline plane (CONFIRMED).** It creates only `tickets/work/sessions/.adb` (`init.go:66-71`), yet `adb repos pull` hard-errors on a missing `repos/` root (`repos.go:52-54` → `reposync_walk.go:65-67`). A freshly-initialized workspace cannot `repos pull`.
- **Two divergent "repos" surfaces (CONFIRMED).** `adb sync repos` regenerates *context text* (`GenerateRepoContext`, `sync.go:171-201`) while `adb repos pull` does the *git fetch* (`repos.go:25-75`) — a confusing overlap.
- **Duplicated scaffolding logic (CONFIRMED).** `init workspace` (`init.go:66-134`) and `init project` (`projectinit.go:124-200`) duplicate the `.taskrc`/backlog scaffolding byte-for-byte with slightly different dir sets — a shared-template change must be edited twice.
- **No first-class Organization entity; Project type is dead code (PARTIAL).** There is no `Organization`/`Tenant` type anywhere. A `Project` struct *does* exist (`pkg/models/hive.go:14-28`, with Name/RepoPath/Status/Tags/RelatedProjects/counts) but is **unused dead code** — grep for `models.Project` finds zero `.go` instantiations; no `ProjectStore`/`OrgStore`/`ListProjects` exists. "Org" is only a path segment derived by the VS Code extension (`orggroup.ts`).
- **No template-set / scaffold-profile abstraction (CONFIRMED).** No named profile type, no data-driven manifest, no per-language branch; `InitOptions` exposes only Name/AIProvider/TaskIDPrefix/GitInit/WithBMAD.
- **No monorepo-bootstrap command (PARTIAL).** `init` subcommands are only `workspace`/`claude`/`project`; there is no `adb init org`, no bulk per-repo init. Nuance: ADB *does* lazily clone remote repos into `repos/<platform>/<org>/<repo>` per-task during worktree creation (`internal/integration/worktree.go:186-197,315-377`) — a plain `git clone` with no template applied — but there is no dedicated bulk clone-and-scaffold command.

### 4c. Wiki-quality defects

- **No cross-links.** `Decision.RelatedTo` exists in the model (`pkg/models/knowledge.go:18`) but `renderPage` never emits `[[wikilinks]]` — pages are disconnected (`wikipublisher.go:133-159`).
- **Tags dropped.** Frontmatter tags are hardcoded `[adb, knowledge, task-knowledge]`; real per-item `Tags`/`Category`/`Severity` are discarded, so no tag/category index can be built (`wikipublisher.go:103`).
- **No index/home/TOC page** is generated (`WIKI.md` is allowlisted for cloud upload but never produced).
- **"Extraction" is a verbatim dump.** `ExtractFromTask` copies all of `context.md` into `Summary`; structured decisions/learnings depend entirely on external callers (`internal/core/knowledge.go:37-49`).
- **Memory and wiki are disconnected** — the vector store is never queried to build pages.
- **`CommunicationManager` is orphaned dead code.** Not constructed in `app.go`, no CLI command — no user can record or read a communication in the shipped binary (`internal/storage/communication.go`).
- **`session ingest` cannot parse transcripts** — prints "parsing not implemented yet" (`internal/cli/session.go:104-108`).

---

## 5. THE GAPS — Mapped to Each Target Domain

| Domain | What exists | What's missing (gap) |
|---|---|---|
| **Stakeholders / entities** | Free-form `docs/stakeholders.md` / `docs/contacts.md` spliced into CLAUDE.md by `generateStakeholders()` (`aicontext.go:462-486`) | No structured Entity/Stakeholder/Contact/Org model, no CRUD, no stable IDs, no link from `Communication.From/To` (plain strings) to any entity. No template. |
| **Systems** | Nothing | No system/service catalog, no per-system template (ownership, dependencies, endpoints, SLAs). |
| **Architecture** | Task-scoped `design.md`; ADRs read from `docs/decisions/*.md` but never scaffolded | No ADR template (Nygard/MADR), no glossary template, no arc42/C4 doc templates, no reusable per-system architecture doc. |
| **Design** | Task-scoped `design.md` template only | No reusable per-system/feature design-doc template outside the hardcoded BMAD strings. |
| **Sales** | **Nothing** | No lead/deal/opportunity/pipeline/revenue concept anywhere in code. Entirely absent. |
| **Comms (in/out)** | Rich `Communication` model + file store (both orphaned) | Not wired into `app.go`, no CLI, no template. No `direction` (inbound/outbound/internal) field. No entity ref. No ingestion connectors (email/Slack/Teams/calendar). |
| **Data ingestion** | Real dev-artifact sync: issuesync, cloudsync, reposync | No generic connectors for external comms/CRM sources; no source/dataset/pipeline template; screenshot OCR pipeline advertised but unimplemented; transcript parsing is a TODO; no incremental hash/cursor dedup; no unified ingestion abstraction. |
| **Org/project multi-tenancy** | Path-encoded `<platform>/<org>/<repo>` correlation over one `backlog.yaml`; dead `Project` struct | No modeled Org/Project hierarchy, no registry, no org/project config tier, no enumeration/management of projects-as-data. |

**Cross-cutting knowledge-wiki gaps:** no cross-repo/cross-org aggregation (publisher is rooted at one `App.BasePath`); no entity/topic model (N tasks touching "auth" produce N unrelated pages); no indexing/search/backlink graph; no controlled taxonomy or frontmatter contract that lets many repos interlink; no dedup/merge at publish time; no stable page identity beyond `<taskid>-knowledge.md`.

---

## 6. Harness / Skills / Agents — Have vs Need + Proposed Distribution

### What exists vs what's needed

| Component | Documented (CLAUDE.md) | Actually ships | Needed for LLM-wiki monorepo |
|---|---|---|---|
| Hook engine | 5 hooks | **Real** (`hookengine.go`) | Keep; make hooks distributable |
| Agents | 18 BMAD personas | **None** (only hardcoded names in `team.go`) | Real, versioned agent definition files |
| Skills | 23 skills | **None** | Progressive-disclosure `SKILL.md` files |
| `adb team` orchestration | Multi-agent launch | **Plan-file only, no exec** | RESOLVED: removed in TASK-00039 rather than given real dispatch |
| `sync claude-user` | Sync agents/skills/statusline | **~4-line stub** (`contextgen.go:167-196`) | A real distribution path |
| Embedded template dirs | skills/agents/hooks/artifacts/checklists | **8 task-doc files + 1 rules file** | A KB-entity template catalog (§7) |

### Proposed harness distribution mechanism (embed + sync to many repos)

Adopt the **Claude Code plugin + marketplace** model (the industry-standard, citable pattern for shipping a reusable agent harness):

1. **Bundle the harness as a Claude Code plugin** — a `.claude-plugin/plugin.json` manifest bundling `skills/` (each `<name>/SKILL.md`), `agents/` (ADB's team subagents as portable markdown+frontmatter), `hooks/hooks.json` (using `${CLAUDE_PROJECT_DIR}`-relative script paths), `.mcp.json`, and `bin/`. One versioned unit carries the whole harness surface instead of the currently-scattered inline constants. (`code.claude.com/docs/en/plugins`)
2. **Publish a `marketplace.json` catalog** from a dedicated (optionally private) git repo that downstream repos add via `/plugin marketplace add` and refresh via `update` — the concrete "one harness, many repos" channel. (`code.claude.com/docs/en/plugin-marketplaces`)
3. **Dual versioning:** explicit SemVer in `plugin.json` for stable releases; commit-SHA delivery for fast internal iteration. Consumers pin to an exact `ref`/`sha` for reproducibility.
4. **Scope precedence:** project-committed `.claude/` for team defaults, `~/.claude/` for personal, managed/enterprise settings for non-overridable org policy (managed > project > user).
5. **Progressive-disclosure skills** (small trigger/description, detail in the body) following the open Agent Skills standard so context cost stays near-zero and skills are portable beyond Claude Code.
6. **A CLI installer** (à la `specify init` / `npx bmad-method install`) that scaffolds repo-local, agent-specific command files and steering/context files, with a non-interactive CI mode for onboarding new repos.
7. **Lean core + opt-in expansion packs** (per BMAD) with a `validate`-before-publish gate and CI that auto-repins the catalog on release.

ADB is itself a CLI and already wires `.mcp.json` health checks and a hook system, so it can replicate this distribution mechanism directly. The immediate prerequisite is that the harness assets *actually exist as files* before any distribution matters.

---

## 7. Missing Templates — Proposed Template Catalog

Today ADB has **8 task-scoped templates only** (`templates/claude/`: `adb-prompt.sh`, `context.md`, `design.md`, `handoff.md`, `notes.md`, `status.yaml`, `task-context.md`, `rules/standing-dos-donts.md`). There is **no KB-entity template of any kind**. Below is a concrete proposed catalog, informed by Backstage catalog kinds, C4, ADR (Nygard/MADR), arc42, CRM/stakeholder modeling, and ingestion-source patterns. All are markdown-with-YAML-frontmatter, one entity per file.

| Template | Purpose | Frontmatter / schema (proposed) |
|---|---|---|
| `entity-component.md` | System/service inventory unit | Backstage envelope: `apiVersion`, `kind: Component`, `metadata{name,namespace,title,description,tags,links[]}`, `spec{type,lifecycle,owner,system,dependsOn[],providesApis[],consumesApis[]}` |
| `entity-system.md` | System grouping | `kind: System`, `spec{owner,domain,type}` |
| `entity-domain.md` | Domain grouping | `kind: Domain`, `spec{owner,subdomainOf,type}` |
| `entity-resource.md` | Infra resource (db, bucket) | `kind: Resource`, `spec{type,owner,system,dependsOn[]}` |
| `entity-api.md` | API contract | `kind: API`, `spec{type: openapi/asyncapi/graphql/grpc,lifecycle,owner,definition}` |
| `entity-group.md` | Team / business unit | `kind: Group`, `spec{type,parent,children[],members[],profile{displayName,email}}` |
| `entity-user.md` | Person | `kind: User`, `spec{memberOf[],profile{displayName,email,picture}}` |
| `stakeholder.md` | Stakeholder register entry | `id`, `name`, `user_ref` (→ entity-user), `power`, `interest`, `attitude`, `salience{power,legitimacy,urgency}`, `raci_role`, `engagement_status`, `communication_strategy` (Mendelow + Mitchell salience + RACI) |
| `adr.md` | Architecture Decision Record | MADR-style: `status: proposed/accepted/deprecated/superseded-by`, `date`, `decision-makers`, `consulted[]`, `informed[]`; body: Title, Context & Problem, Considered Options, Decision Outcome, Consequences. Superseded-by chains decisions over time. |
| `arch-doc.md` | System architecture doc | arc42 12-chapter skeleton; §1 Stakeholders table, §5 Building Blocks, §9 Architecture Decisions (index into ADR store) |
| `design-doc.md` | Feature/system design | Context, goals/non-goals, C4 container/component diagram refs, alternatives, risks |
| `glossary.md` | Controlled vocabulary | `terms: [{term, definition, aliases[], related[]}]` |
| `context-map.md` | Inter-system relationships | DDD edges typed: Upstream/Downstream, Customer/Supplier, ACL, OHS, Published Language, Shared Kernel, Conformist, Separate Ways |
| `communication.md` | Comms log entry (in/out) | `id`, `date`, `direction: incoming/outgoing/internal`, `from`, `to[]`, `stakeholder_ref`, `channel`, `subject`, `thread_id`, `references[]`, `tags[]`, `action_items[]` (extends existing model + new `direction`/`stakeholder_ref`) |
| `account.md` / `deal.md` | Sales CRM | account: `id,name,owner,stakeholders[]`; deal: `id,account_ref,stage,value,currency,close_date,probability,stage_history[]` |
| `ingestion-source.md` | Data source connector spec | `id`, `source_type` (email/slack/teams/calendar/webhook/git), `cursor_field`, `primary_key`, `schedule`, `dedup_hash`, `provenance_url`, `last_synced` |
| `dataset.md` | Ingested dataset/page provenance | `id`, `source_ref`, `content_hash`, `chunk_strategy`, `embedded: bool`, `raw_pointer`, `supersedes` |
| `llms.txt` (generated, not a template) | Agent entrypoint index | H1 project name, blockquote summary, H2 link sections → CLAUDE.md, decisions, learnings, glossary, ADRs |

**Enabling change:** make `TemplateType` data-driven (drop-a-file) rather than the current fixed 6-value enum (`templates.go:14-27`), and actually *read* the injected `embed.FS`, so new KB templates require no recompile.

---

## 8. Recommended Architecture to Bootstrap the LLM-Wiki Monorepo

### 8.1 Org/project hierarchy (multi-tenancy)

Introduce first-class, persisted **Organization** and **Project** entities (resurrect and wire the dead `pkg/models/hive.go:Project`; add `Organization`). Back them with a registry (`orgs/index.yaml`, `projects/index.yaml`) as the author originally envisioned. Add a **three-tier config model** — Global → **Org** → Repo/Project — closing the current gap where a shared pattern must be global or per-repo-duplicated. Front the whole thing with a **Backstage-style Software Catalog** (`catalog-info.yaml` per entity, derived relations `ownedBy`/`partOf`/`dependsOn`) so every project is discoverable with an owner and its source-template reference. (`backstage.io/docs/features/software-catalog/`)

### 8.2 Template inheritance / updatability (the load-bearing decision)

Do **not** rely on one-shot scaffolding (the current `ProjectInitializer` model, which freezes a project at birth). Adopt the **two-tier** pattern:

- **Creation:** a monorepo generator (Nx `nx g` or `turbo gen`) or the ADB CLI installer for uniform in-repo project creation. (`nx.dev/features/generate-code`, `turborepo.dev/docs/guides/generating-code`)
- **Lifecycle:** **Copier** (or **cruft**) so every generated project keeps a provenance/answers file (`.copier-answers.yml` / `.cruft.json`) and can *receive template updates later* as a computed diff between pinned and target template versions. Version the shared template with git tags. (`copier.readthedocs.io`, `cruft.github.io/cruft/`)
- **Config-as-code option:** where the pattern is mostly build/CI/tooling, prefer **projen** — a shared base project type published as a package propagates policy via a single dependency bump + re-synth. (`projen.io`)
- **Conformance:** run a scheduled drift check across all consuming repos (`cruft check` cron → auto-PRs, or projen re-synth CI), mirroring Spotify's "Golden State". Use skip/exclude lists so updates never clobber project-specific code. (`engineering.atspotify.com`, `tag-app-delivery.cncf.io/whitepapers/platforms/`)

This directly fixes the CONFIRMED "no template-set abstraction / unused embed.FS / edit-twice" defects.

### 8.3 Wiki aggregation & indexing

- **Emit an `llms.txt` index** from `adb sync wiki` (H1 + blockquote + H2 link sections + `llms-full.txt`) — the exact format Anthropic and MCP publish — giving any agent a deterministic entrypoint. (`llmstxt.org`, `code.claude.com/docs/llms.txt`)
- **Split the ~39KB generated CLAUDE.md** to respect the <200-line guidance: a short root `CLAUDE.md` that `@imports` topic files, with big reference sections moved to path-scoped `.claude/rules/*.md` (`paths:` globs) that load only when relevant. Also emit an `AGENTS.md` for cross-tool portability. (`code.claude.com/docs/en/memory`, `agents.md`)
- **Aggregate across repos/orgs:** extend the publisher beyond a single `App.BasePath` — namespace pages by `org/project`, generate an index/home/TOC page, and emit real `[[wikilinks]]` from `Decision.RelatedTo` (currently ignored) plus per-tag/per-category index pages from the dropped item tags.
- **Wire the existing vector-memory substrate** (`internal/memory/`) to the wiki for semantic search, and add **hybrid retrieval + rerank** (Pinecone full-text/BM25 + `rerank-documents`) for exact-identifier queries (TASK-#, error codes, names). Prepend a 50–100 token contextual blurb to each chunk before embedding. (`anthropic.com/news/contextual-retrieval`, `pinecone.io/learn/chunking-strategies/`)
- **Human-vs-agent memory split:** treat human-authored context as the CLAUDE.md layer and `KnowledgeExtractor` output as an agent-written `MEMORY.md`-style index + per-topic pages loaded on demand.
- **Agent write-back via an ADB "knowledge" MCP server** (ADB already ships `internal/mcpserver/`): expose `search_knowledge`, `record_decision`, `add_learning` so agents contribute mid-session, routed through the hook/knowledge pipeline for validation. (`modelcontextprotocol.io/examples`)

### 8.4 Entity catalog

Adopt the Backstage envelope verbatim (§7), with Domain > System > Component > Resource/API containment and C4 Container/Component decomposition inside each System; type inter-system edges with the DDD context-map vocabulary. Cross-link the four stores into one navigable graph: Communication → ADR/stakeholder; ADR `consulted/informed` → stakeholder register; stakeholder → entity-user/group; entity `owner` → group/user. (`backstage.io/docs/features/software-catalog/descriptor-format`, `c4model.com`, `arc42.org/overview`, `adr.github.io/madr/`, `github.com/ddd-crew/context-mapping`)

### 8.5 Ingestion layer

Build a unified ingestion abstraction that the existing issuesync/cloudsync/reposync share, plus new connectors:
- **Incremental, hash + cursor driven:** a `doc_id → content_hash` manifest (LlamaIndex pattern) so only changed pages re-embed, with tombstone deletes for removed pages; cursor-based connectors (`updated_at`) for live sources plus periodic full reconciliation to catch cursor-less edits. (`developers.llamaindex.ai/.../ingestion_pipeline/`, `docs.airbyte.com/.../incremental-append-deduped`)
- **Multimodal summarize-then-index:** describe screenshots/tables/transcripts with an LLM, index the description, keep the raw artifact linked with a source pointer on every derived chunk; hash-cache descriptions. This finally implements the advertised-but-missing screenshot OCR pipeline and transcript ingestion. (`learn.microsoft.com/.../rag-chunking-phase`)
- **Provenance metadata** (`citation`, `source-type`, `from`, `date`, `links`) written into every record so agents can filter and trace facts to ground truth.

---

## 9. Prioritized Roadmap

### P0 — Truth & foundations (do first; nothing else is safe until docs match reality)
1. **Rewrite `CLAUDE.md` to match the codebase.** Delete the fictional 18-agent/23-skill/`adb serve`/embedded-dir claims; document the real shipped subsystems (`memory`, `cloudsync`, `issuesync`, `scheduler`, `mcpserver`, `vscode-extension`); fix Go version, file tree, test counts. A wiki that ingests today's `CLAUDE.md` as ground truth will hallucinate.
2. **Model Org + Project as first-class entities** (wire the dead `hive.go:Project`, add `Organization`), with `orgs/index.yaml` / `projects/index.yaml` registries and enumeration commands.
3. **Add the Org config tier** (Global → Org → Repo).
4. **Make templates pluggable:** actually read the injected `embed.FS`, make `TemplateType` data-driven, and add non-Go language support to scaffolded `.taskrc`. De-duplicate the `init workspace` / `init project` scaffolding.
5. **Wire the orphaned `CommunicationManager`** into `app.go` + a CLI (`adb comm add/list/show`) and add a `direction` field.

### P1 — Templates, harness, wiki spine
6. **Ship the KB-entity template catalog** (§7): entity/*, stakeholder, adr, glossary, arch-doc, communication, ingestion-source.
7. **Make the harness real and distributable:** author actual agent + skill files; package as a Claude Code plugin + marketplace; give `adb team` real dispatch or reframe it honestly.
8. **Upgrade the wiki publisher:** cross-links, index/home page, per-tag indexes, `org/project` namespacing, `llms.txt` emission; split CLAUDE.md via `@import` + path-scoped rules; emit `AGENTS.md`.
9. **Adopt Copier/cruft** for template provenance + updatability; version templates with git tags; add a scheduled conformance-drift check.

### P2 — Aggregation, ingestion, sales, semantic layer
10. **Cross-repo/org wiki aggregation** + Backstage-style catalog with derived relations and ownership.
11. **Wire vector memory to the wiki**; add hybrid + rerank retrieval and contextual chunking; stand up an ADB "knowledge" MCP server for agent write-back.
12. **Unified ingestion layer:** incremental hash/cursor connectors, multimodal summarize-then-index (implement the missing screenshot OCR + transcript parsing), provenance metadata.
13. **Add the Sales/CRM layer** (accounts, deals, pipeline) — currently entirely absent.
14. **A `adb init org` / bulk clone-and-scaffold command** that populates `repos/<platform>/<org>/<repo>` and applies the shared template.

---

## 10. Sources

**ADB repo (audit evidence):**
- `github.com/valter-silva-au/ai-dev-brain` — `README.md`, `docs/getting-started.md`, `docs/RESEARCH.md`, `docs/learning/L100-L600`, `internal/core/*` (`bootstrap.go`, `taskmanager.go`, `projectinit.go`, `templates.go`, `wikipublisher.go`, `knowledge.go`, `aicontext.go`, `hookengine.go`, `contextgen.go`), `internal/cli/*` (`init.go`, `sync.go`, `repos.go`, `hook.go`, `team.go`, `session.go`), `internal/integration/*` (`issuesync/`, `cloudsync/`, `reposync_walk.go`, `worktree.go`, `screenshot.go`), `internal/memory/memory.go`, `pkg/models/*` (`config.go`, `task.go`, `communication.go`, `knowledge.go`, `hive.go`), `internal/storage/communication.go`, `vscode-extension/src/orggroup.ts`.

**LLM-wiki standards & agent memory:**
- llms.txt standard — https://llmstxt.org
- Claude Code docs index (llms.txt) — https://code.claude.com/docs/llms.txt
- AGENTS.md spec — https://agents.md
- Claude Code memory (CLAUDE.md, auto memory, `.claude/rules/`) — https://code.claude.com/docs/en/memory
- Cursor Rules — https://cursor.com/docs/context/rules
- Graphiti temporal knowledge graph — https://github.com/getzep/graphiti
- MCP example servers (Memory) — https://modelcontextprotocol.io/examples

**Scaffolding / monorepo / template updatability:**
- Backstage Software Templates — https://backstage.io/docs/features/software-templates/
- Backstage Software Catalog — https://backstage.io/docs/features/software-catalog/
- Backstage descriptor format — https://backstage.io/docs/features/software-catalog/descriptor-format
- Backstage system model — https://backstage.io/docs/features/software-catalog/system-model
- Nx generators — https://nx.dev/features/generate-code
- Turborepo generators — https://turborepo.dev/docs/guides/generating-code
- Cookiecutter — https://cookiecutter.readthedocs.io/en/stable/
- cruft — https://cruft.github.io/cruft/
- Copier — https://copier.readthedocs.io/en/stable/
- projen — https://projen.io/docs/introduction/getting-started/
- Yeoman — https://yeoman.io/authoring/
- Spotify Golden Paths — https://engineering.atspotify.com/2020/08/how-we-use-golden-paths-to-solve-fragmentation-in-our-software-ecosystem/
- CNCF Platforms whitepaper — https://tag-app-delivery.cncf.io/whitepapers/platforms/

**Harness packaging & distribution:**
- Claude Code plugins — https://code.claude.com/docs/en/plugins
- Claude Code plugin marketplaces — https://code.claude.com/docs/en/plugin-marketplaces
- Claude Code subagents — https://code.claude.com/docs/en/sub-agents
- Claude Code skills — https://code.claude.com/docs/en/skills
- Claude Code hooks — https://code.claude.com/docs/en/hooks
- GitHub spec-kit — https://github.com/github/spec-kit
- AWS Kiro specs — https://kiro.dev/docs/specs/
- BMAD Method — https://github.com/bmad-code-org/BMAD-METHOD

**Entity / architecture / decision / stakeholder modeling:**
- C4 model — https://c4model.com/
- arc42 — https://arc42.org/overview
- ADR templates (Nygard) — https://github.com/joelparkerhenderson/architecture-decision-record
- MADR — https://adr.github.io/madr/
- DDD context mapping — https://github.com/ddd-crew/context-mapping
- Stakeholder analysis (Mendelow; Mitchell/Agle/Wood) — https://en.wikipedia.org/wiki/Stakeholder_analysis

**Data ingestion / RAG:**
- Anthropic Contextual Retrieval — https://www.anthropic.com/news/contextual-retrieval
- Pinecone chunking strategies — https://www.pinecone.io/learn/chunking-strategies/
- LlamaIndex ingestion pipeline — https://developers.llamaindex.ai/python/framework/module_guides/loading/ingestion_pipeline/
- Airbyte incremental append+deduped — https://docs.airbyte.com/using-airbyte/core-concepts/sync-modes/incremental-append-deduped
- Microsoft RAG chunking phase — https://learn.microsoft.com/en-us/azure/architecture/ai-ml/guide/rag/rag-chunking-phase

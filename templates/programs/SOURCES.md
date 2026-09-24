# Template provenance register

Every document-program template shipped under `templates/claude/programs/` is **authored by adb**
from **publicly published** sources. This file records which sources informed which pack, so the
lineage of any shipped artifact is auditable without reading its git history.

Two rules govern this directory, and both are enforced by
`TestProvenance_*` in `internal/core/provenance_test.go` rather than by convention:

1. Every `program.yaml` declares a non-empty `lineage`.
2. No template contains a marker string from a non-public source (see **Excluded sources**).

## Why this register exists

adb is a public repository. Document templates are exactly the kind of artifact where
methodology text gets copied in from wherever the author last saw it — including from material
that is not redistributable. A register plus a build-failing test makes the boundary
a property of the codebase instead of a promise in a commit message.

If you add a pack or a template, add its source here in the same commit.

## Excluded sources

The following were deliberately **not** used as sources. Some were available to the author at
authoring time; none were drawn from.

| Source | Why excluded |
|---|---|
| A cloud vendor's internal consulting-practice template library | Internal practice material; not redistributable |
| Vendor-internal enterprise-architecture guide libraries | Marked internal-visibility — confidential information stamps |
| A proprietary customer-outcomes canvas | Explicitly stamped "Not Public" |
| Internal corporate wikis and doc sites | Internal-only |
| Internal delivery-agent documentation | Internal-only |

The guard test fails the build on non-public marker strings anywhere under `programs/`:
confidentiality stamps ("Not Public", internal-use-only stamps), internal practice names,
internal constructs, internal wiki/doc hostnames, and `.kiro` workspaces.

Note on one near-miss worth understanding: generic document-type names — PRD, HLD, LLD, RAID log,
ADR, test strategy, runbook, deployment guide — are industry-standard terminology and carry no
provenance constraint. What is constrained is *prescriptive content*: section text, maturity
scales, scoring rubrics, and named internal constructs.

## Sources by pack

### `product-discovery`

| Source | Author / owner | Licence or terms | Used for |
|---|---|---|---|
| [Shape Up](https://basecamp.com/shapeup) ch. 6 "Write the Pitch" | Ryan Singer / 37signals | Free to read online | `pitch.md` — the five ingredients: problem, appetite, solution, rabbit holes, no-gos |
| [Lean Canvas](https://leanstack.com/lean-canvas) | Ash Maurya | CC BY-SA 3.0 (adapted from Business Model Canvas) | `outcomes-canvas.md`, `problem-solution-canvas.md` |
| Value Proposition Canvas | Alexander Osterwalder / Strategyzer | Publicly published framework | `outcomes-canvas.md` |
| [Working Backwards / PR-FAQ](https://aws.amazon.com/executive-insights/content/product-management-at-amazon/) | Amazon — **public** page | Publicly published by AWS | `press-release.md` (one page, future-dated), `faq.md` (customer-facing + internal split) |
| Design thinking / How-Might-We | Stanford d.school, IDEO | Publicly published method | `how-might-we.md` |
| [Now/Next/Later roadmap](https://www.prodpad.com/blog/how-to-build-a-product-roadmap-everyone-understands/) | Janna Bastow / ProdPad | Public blog post | `roadmap.md` — three horizons, no dates, initiatives not features |
| HEART framework | Kerry Rodden et al., Google (published paper) | Publicly published | `measurement-framework.md`, `success-metrics.md` |
| OKRs; SMART criteria | Public management canon | Public | `success-metrics.md` |
| Design Sprint | Jake Knapp / Google Ventures | Publicly published | `workshop-summary.md` |
| RAID log; RACI; power/interest grid | PMI / PRINCE2 canon; Mendelow (1981) | Public | `raid-log.md`, `stakeholder-map.md` |

### `technical-design`

| Source | Author / owner | Licence or terms | Used for |
|---|---|---|---|
| ["Design Docs at Google"](https://www.industrialempathy.com/posts/design-docs-at-google/) | Malte Ubl | Public blog post | `design-doc.md`, `design-doc-mini.md`, `interface-sketch.md` — context/scope, goals **and non-goals**, alternatives considered, cross-cutting concerns, size tiers, *when not to write one* |
| [Rust RFC template](https://github.com/rust-lang/rfcs/blob/master/0000-template.md) | rust-lang | MIT / Apache-2.0 | `rfc.md`, `detailed-design.md` — drawbacks, prior art, unresolved questions, future possibilities |
| [Oxide RFD 1](https://rfd.shared.oxide.computer/rfd/0001) | Oxide Computer Company | Publicly published | `rfc.md` — numbering, labels, options → determination |
| [C4 model](https://c4model.com/) | Simon Brown | CC BY 4.0 | `design-doc.md` (context, container), `detailed-design.md` (component, code) |
| STRIDE; [OWASP](https://owasp.org/) | Microsoft (STRIDE); OWASP Foundation | Publicly published | `threat-model.md` |
| BABOK | IIBA | Publicly published standard | `business-requirements.md` |

Architecture decision records are **not** in this pack — adb already ships `adb adr` (MADR
format, [adr.github.io/madr](https://adr.github.io/madr/)). The pack cross-references it.

### `delivery-readiness`

| Source | Author / owner | Licence or terms | Used for |
|---|---|---|---|
| [Google SRE book](https://sre.google/sre-book/evolving-sre-engagement-model/) — Production Readiness Review | Google | Free to read online (CC BY-NC-ND for the book) | `prr-checklist.md` — the six review axes and PRR-as-acceptance-gate |
| Google SRE book — launch coordination | Google | as above | `launch-checklist.md` |
| Google SRE book — operational handover / onboarding phase | Google | as above | `operational-handover.md`, `runbook.md` |
| Test pyramid | Mike Cohn; Martin Fowler (public writeups) | Public | `test-strategy.md` |
| Change-management / release-engineering practice | Public canon | Public | `rollback-plan.md` |

Only the *structure* is taken from the SRE book (which axes a readiness review covers), never its
prose. The book is CC BY-NC-ND, so verbatim reuse would not be permitted.

### `operations`

| Source | Author / owner | Licence or terms | Used for |
|---|---|---|---|
| [Google SRE book — Postmortem Culture](https://sre.google/sre-book/postmortem-culture/) | Google | Free to read online | `postmortem.md` — blameless framing, the trigger criteria, the review criteria |
| Google SRE book — incident management | Google | as above | `incident-timeline.md` |
| Google SRE book — SLOs and error budgets | Google | as above | `slo-error-budget.md` (cross-references `adb slo set`) |
| Google SRE book — production meeting | Google | as above | `weekly-ops-review.md` |
| HEART framework | Kerry Rodden et al., Google | Publicly published | `adoption-kpis.md` |

### `change-adoption`

| Source | Author / owner | Licence or terms | Used for |
|---|---|---|---|
| [Team Topologies](https://teamtopologies.com/key-concepts) | Matthew Skelton, Manuel Pais | Publicly published concepts | `capability-assessment.md` — four team types, three interaction modes, **cognitive load** as the capability constraint |
| ADKAR | Prosci | Publicly described model | `change-management.md` |
| 8-Step Process for Leading Change | John Kotter | Publicly described model | `change-management.md` |
| Training-needs analysis | Public L&D canon | Public | `enablement-plan.md` |

## Adding a pack

1. Create `templates/claude/programs/<id>/program.yaml` with a non-empty `lineage`.
2. Add a row per source to this file.
3. Confirm the guard test still passes: `go test ./internal/core -run TestProvenance -v`.

If a source is not publicly published under terms permitting this use, it does not go in. Author
the structure from a public equivalent instead — for every artifact type in this register, one
exists.

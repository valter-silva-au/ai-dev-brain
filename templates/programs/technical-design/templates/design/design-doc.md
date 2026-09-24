---
title: High-Level Design
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: 'Design Docs at Google' (Malte Ubl, industrialempathy.com) for context/scope, goals and non-goals, the actual design, alternatives considered, and cross-cutting concerns; the C4 model (Simon Brown, CC-BY-4.0) for the system-context and container levels."
---

# High-Level Design

<!-- WHEN NOT TO WRITE THIS DOC
     A design doc exists to expose trade-offs and let reviewers change your mind
     before code is written. If there are no real trade-offs — the approach is
     forced, the change is small and reversible, or the document would just be an
     implementation manual restating what the code will obviously say — do not
     write it. Write the program instead, and if a decision was made along the
     way record it with `adb adr new`. Also skip it if the work is incremental
     and the mini form (design-doc-mini) would cover it. -->

## Context and scope

<!-- Enough background for a reader who knows the domain but not this project.
     What exists today, what is changing, what stays untouched. Keep it factual
     and short — this is orientation, not persuasion. A weak answer opens with
     the proposed solution before establishing why anything needs to change. -->

## Goals

<!-- What this design must achieve, traced to the PRD requirement IDs. -->

| # | Goal | Traces to |
|---|------|-----------|
| G-1 | | FR-001 |

## Non-goals

<!-- Mandatory. Things a reviewer might reasonably expect this design to handle
     that it deliberately does not, each with one line of reasoning. This is
     where you head off the "but what about..." review cycle. Distinguish
     "not now" from "not ever". -->

- **Non-goal:** <!-- ... --> — **why:** <!-- ... -->

## The actual design

<!-- The bulk of the document. Start from the outside and work inward. Prose
     first, diagrams to support it — a diagram with no accompanying explanation
     leaves every interesting question unanswered. -->

### System context (C4 level 1)

<!-- One diagram: the system as a single box, the people and external systems it
     talks to, and the nature of each interaction. Nothing about internals here.
     Use whatever diagram tool the repo already uses; text-based (e.g. Mermaid)
     keeps it reviewable in diff. -->

```
<!-- system context diagram -->
```

<!-- Below the diagram, list each external actor and what it depends on us for
     (or what we depend on it for), including whether the dependency is
     synchronous. -->

| Actor / external system | Direction | Interaction | Sync or async |
|-------------------------|-----------|-------------|---------------|
| | | | |

### Containers (C4 level 2)

<!-- The deployable/runnable units inside the system — services, apps, jobs,
     datastores — and how they communicate. State the technology choice for each
     and the protocol between them. Component-level internals belong in the
     detailed design, not here. -->

```
<!-- container diagram -->
```

| Container | Responsibility | Technology | Talks to (protocol) |
|-----------|----------------|------------|---------------------|
| | | | |

### APIs

<!-- Sketch the interfaces the design introduces or changes: the operations,
     roughly what goes in and out, and the errors callers must handle. Do NOT
     paste formal schemas or IDL here — they age badly inside prose and drown the
     reader. If the surface needs more room, put it in the interface sketch and
     link to it. -->

### Data storage

<!-- What is stored, where, in what shape at a conceptual level, and the
     properties that matter: ownership, consistency model, expected volume and
     growth, retention and deletion, indexing needs, and what happens on
     restore. A weak answer names a database and stops. -->

### Degree of constraint

<!-- How much freedom this design has, and why. Are you working in a greenfield
     space, or inside a system whose conventions and existing data model dictate
     most choices? State the constraints that removed options — it saves
     reviewers from proposing alternatives that were never available. -->

### Failure modes and behaviour under load

<!-- What breaks first, what the system does when a dependency is unavailable,
     what is retried, what is dropped, and what the user sees. Include
     backpressure and timeout posture. "It will retry" without stating bounds is
     a weak answer. -->

## Alternatives considered

<!-- One subsection per serious alternative. For each: what it was, and the
     specific trade-off that lost. "We didn't consider any" is a red flag —
     if there genuinely was no choice, say why. Alternatives that were never
     credible are padding; leave them out. -->

### Alternative 1: <name>

**What it was:**

**Why not:**

### Alternative 2: <name>

**What it was:**

**Why not:**

## Cross-cutting concerns

### Security

<!-- Authentication, authorisation, secrets handling, trust boundaries crossed,
     and the blast radius of a compromise of each container. Reference the threat
     model rather than duplicating it, but state here what the design does about
     the top threats. -->

### Privacy and data handling

<!-- Personal or sensitive data touched, lawful basis / policy that permits it,
     minimisation, retention, deletion path, and where it crosses a region or
     tenancy boundary. "No PII" is a claim that needs checking, not an answer. -->

### Observability

<!-- What signals prove the system is working: the specific metrics, the events
     worth logging, trace propagation across containers, and the alerts a
     responder would actually be paged on. State how you would detect each
     failure mode listed above — if a failure mode has no signal, say so. -->

### Operability

<!-- Deployment shape, configuration and feature flags, migration/backfill,
     rollback path, capacity assumptions, and who is on call. -->

### Testing strategy

<!-- What is proven by unit tests, what needs integration or contract tests, and
     what can only be verified in a real environment. Name what you will not be
     able to test and how you compensate. -->

## Traceability

| PRD requirement | Where this design satisfies it |
|-----------------|--------------------------------|
| FR-001 | |
| NFR-001 | |

## Decisions

<!-- Record architecture decisions in the ADR log, not in this document: run
     `adb adr new` and link the id below. This table is a pointer so a reader can
     find the reasoning without this doc becoming the decision log. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |

## Open questions

| Question | Blocks | Owner |
|----------|--------|-------|
| | | |

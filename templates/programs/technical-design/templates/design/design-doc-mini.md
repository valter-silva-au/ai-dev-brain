---
title: Design Doc (Mini)
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: 'Design Docs at Google' (Malte Ubl, industrialempathy.com) — the short, one-to-three-page variant recommended for incremental work."
---

# Design Doc (Mini)

<!-- WHEN TO USE THIS INSTEAD OF THE FULL DOC
     Use the mini form for incremental work inside a system whose shape is
     already settled: one or two components change, the interfaces are mostly
     fixed, and the interesting content is a single trade-off. Target one to
     three pages.

     WHEN NOT TO WRITE ONE AT ALL
     If there is no trade-off to expose — the approach is forced, or the doc
     would just narrate the diff — skip it and write the program. Record any
     decision made along the way with `adb adr new`.

     WHEN TO ESCALATE
     If you are adding a container, changing a data model others depend on,
     crossing a trust boundary, or you cannot fit it in three pages, stop and
     write the full design doc instead. -->

## Context

<!-- Two or three paragraphs at most. What exists, what is wrong or missing, and
     what triggered this now. Assume the reader knows the system. A weak answer
     re-explains the whole service. -->

## Goals

<!-- Two to four bullets, each measurable or at least unambiguous. Trace to a
     requirement id where one exists. -->

-

## Non-goals

<!-- Mandatory even in the short form — it is the cheapest way to keep review
     focused. One line each. -->

-

## The design

<!-- The change itself: which components are touched, what the new behaviour is,
     and how data flows differ afterwards. A small diagram is fine if it earns
     its space. Include the one or two decisions a reader would want to argue
     with — that is the reason this document exists. -->

### What changes

| Component | Change |
|-----------|--------|
| | |

### Interfaces touched

<!-- Sketch level only: operation names and the shape of the change (added
     field, new error, changed default). Do not paste schemas. Note any
     backwards-compatibility implication for existing callers. -->

### Risks and how they are handled

<!-- The two or three things most likely to go wrong, with the mitigation and the
     rollback path. "Low risk" with no reasoning is a weak answer. -->

| Risk | Mitigation |
|------|------------|
| | |

## Alternatives considered

<!-- Even in the mini form, name the alternatives you rejected and the trade-off
     that lost. One or two sentences each is enough. If there was genuinely only
     one option, say what constrained it — and reconsider whether this document
     is needed at all. -->

### Alternative: <name>

**Why not:**

## Testing and verification

<!-- How you will know it worked, before and after rollout. Name the test level
     and the signal you will watch in production. -->

## Decisions

<!-- Decisions belong in the ADR log: run `adb adr new` and link the id here. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |

## Open questions

| Question | Owner |
|----------|-------|
| | |

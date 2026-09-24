---
title: Product Requirements Document
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the goals / non-goals discipline described in 'Design Docs at Google' (Malte Ubl, industrialempathy.com), and SMART criteria for objective statements."
---

# Product Requirements Document

<!-- The PRD says what the product must do and how each statement will be
     verified. It does not say how the system is built — that is the design doc.
     Every requirement here gets an ID, because the design doc, the detailed
     design, and the tests all trace back to these IDs. -->

## Summary

<!-- Three to five sentences: who this is for, what changes for them, and why
     now. A reader who stops here should be able to repeat the point back. -->

## Goals

<!-- What success looks like, stated SMART: specific, measurable, achievable,
     relevant, time-bound. Each goal should be falsifiable — if you cannot
     imagine the sentence "we missed this goal" being said with evidence, the
     goal is decoration. -->

| # | Goal | Measure | Target | By when |
|---|------|---------|--------|---------|
| G-1 | | | | |

## Non-goals

<!-- Mandatory section. Things this work deliberately does NOT do, and one line
     on why. Non-goals are the cheapest scope control in the document: they
     pre-answer "could we also..." questions. Note the difference between a
     non-goal (we chose not to) and a future possibility (we might later).
     "None" is almost never true — if you write it, justify it. -->

- **Not doing:** <!-- ... --> — **why:** <!-- ... -->

## Users and use cases

<!-- Who uses this and for what. One subsection per distinct user type. Describe
     the job they are trying to get done, not the UI they will click. -->

### <User type>

- **Job to be done:**
- **Today they:**
- **After this they:**

## Functional requirements

<!-- Numbered, atomic, testable. One behaviour per requirement — if a
     requirement contains "and", it is probably two. State the observable
     outcome, not the implementation. The verification column is what makes it
     testable; "manual check" is a weak answer unless you say what is checked. -->

| ID | Requirement | Priority (must / should / could) | Traces to | Verified by |
|----|-------------|----------------------------------|-----------|-------------|
| FR-001 | | must | BR-001 | |
| FR-002 | | | | |

## Non-functional requirements

<!-- Performance, availability, scale, security, privacy, accessibility,
     operability, localisation, compatibility. Each needs a number and a
     condition under which it holds ("p99 under 300 ms at 500 rps sustained"),
     otherwise it cannot be tested or breached. -->

| ID | Category | Requirement (with number + condition) | Traces to | Verified by |
|----|----------|---------------------------------------|-----------|-------------|
| NFR-001 | performance | | BNFR-001 | |
| NFR-002 | availability | | | |
| NFR-003 | security | | | |

## Constraints

<!-- Inherited from the business requirements plus anything discovered since.
     Mark which are fixed and which are merely current. -->

## Dependencies

<!-- What must exist or be delivered by someone else for these requirements to
     be satisfiable. Name the owner. -->

## Acceptance criteria

<!-- The set of conditions under which the work is accepted as done. Keep these
     at the level of observable product behaviour. If they simply restate the FR
     list, cut them and say "acceptance = all must-priority FRs verified". -->

## Rollout and migration considerations

<!-- Flags, phased exposure, data migration, backwards compatibility window,
     and how the change is turned off if it goes wrong. "Ship it" is a weak
     answer for anything with existing users. -->

## Open questions

| Question | Blocks | Owner |
|----------|--------|-------|
| | | |

## Decisions

<!-- Product and architecture decisions go in the ADR log, not inline here:
     run `adb adr new` and reference the ADR id so the reasoning has one home. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |

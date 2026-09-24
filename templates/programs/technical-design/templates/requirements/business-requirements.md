---
title: Business Requirements
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: BABOK / IIBA business analysis canon (requirements classification: business, stakeholder, solution, transition)."
---

# Business Requirements

<!-- This artifact answers "what outcome does the business want, and how will we
     know we got it" — in business language, before any solution is chosen.
     If you find yourself naming a database or a framework here, you have
     skipped ahead: that belongs in the design doc. -->

## Business objectives

<!-- One to five outcomes the business wants, each with a measure and a current
     baseline. "Improve the customer experience" is a weak answer: it has no
     measure, so nothing can ever be shown to have failed. -->

| # | Objective | Measure | Baseline today | Target |
|---|-----------|---------|----------------|--------|
| BO-001 | | | | |

## Background and problem statement

<!-- What is happening now that made this worth funding. Include the cost of
     doing nothing. A weak answer describes the desired feature instead of the
     problem it relieves. -->

## Scope

### In scope

<!-- The business capabilities, processes, user groups, and systems this work
     covers. Be concrete enough that someone can tell whether a given request
     is inside or outside. -->

### Out of scope

<!-- Things a reader would reasonably assume are included but are not. An empty
     out-of-scope list almost always means scope has not been discussed yet. -->

## Stakeholders and their needs

<!-- One row per stakeholder group, not per individual. "Need" is what they must
     be able to do or rely on; "concern" is what they are afraid this will break.
     A weak answer lists job titles with no stated need. -->

| Stakeholder group | Need | Concern | Who represents them |
|-------------------|------|---------|---------------------|
| | | | |

## Business-level functional requirements

<!-- Capabilities stated at the business level: what the organisation must be
     able to do. Keep them solution-neutral. Numbering here (BR-nnn) is what the
     PRD's FR-nnn items will trace back to. -->

| ID | Requirement | Priority | Traces to objective |
|----|-------------|----------|---------------------|
| BR-001 | | | BO-001 |

## Business-level non-functional requirements

<!-- Qualities the business is committing to: availability, response time,
     retention periods, regulatory obligations, accessibility, supported
     locales, data residency. Each needs a number or a named standard.
     "Must be fast" is not a requirement. -->

| ID | Quality | Requirement | Why the business needs this |
|----|---------|-------------|-----------------------------|
| BNFR-001 | | | |

## Constraints

<!-- Things that are fixed and not up for negotiation by this project: budget,
     deadline, mandated platform, existing contracts, headcount, compliance
     regime. Separate genuine constraints from preferences — labelling a
     preference as a constraint kills alternatives that were still viable. -->

| Constraint | Type (budget / time / technical / legal / organisational) | Source |
|------------|-----------------------------------------------------------|--------|
| | | |

## Assumptions

<!-- Statements taken as true without proof yet. Each needs an owner and a date
     by which it will be confirmed, otherwise it is a risk in disguise. -->

| Assumption | Impact if false | Owner | Confirm by |
|------------|-----------------|-------|------------|
| | | | |

## Dependencies

<!-- Other teams, vendors, or programmes this work needs. Name the team and the
     thing needed, not just the system. -->

## Risks

<!-- Business risks, not implementation bugs. Each with a likelihood, an impact,
     and a named response. A risk with no response is a wish. -->

| Risk | Likelihood | Impact | Response | Owner |
|------|------------|--------|----------|-------|
| | | | | |

## Success criteria and how we will measure them

<!-- The acceptance test for the whole effort, at the business level. State the
     data source for each measure. If no one can name where the number comes
     from, the criterion is not measurable. -->

## Open questions

<!-- Questions that must be answered before design can proceed, each with an
     owner. Do not resolve them here by guessing. -->

| Question | Blocks | Owner |
|----------|--------|-------|
| | | |

## Decisions

<!-- Do not record decisions in this file. Business and architecture decisions
     belong in the ADR log so they are searchable and have their own lifecycle:
     run `adb adr new` and link the ADR id here. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |

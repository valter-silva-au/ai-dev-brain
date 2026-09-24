---
title: SLO and Error Budget
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book and the SRE Workbook on service level indicators, objectives and error budgets (sre.google/sre-book)."
---

# SLO and Error Budget

<!-- This document is the reasoning; adb already holds the registry. Define each
     objective here, then register the machine-readable version with:

         adb slo set <name> --objective <percentage> --window <period>

     and link the registry entry below. Do not maintain a second copy of the
     numbers in this file — two sources of truth for a reliability target means the
     one used in an argument is whichever is more convenient. -->

## Service

- Service:
- Owning team:
- Users this protects: <!-- who suffers when the objective is missed -->
- Registry entries: <!-- link the `adb slo set` records rather than restating them -->

## Service level indicators

<!-- An SLI is a measurement, defined precisely enough that two people compute the
     same number. For each one state: what is counted as good, what is counted at
     all, where it is measured (client, load balancer, server), and how it is
     aggregated. Measurement point matters — server-side success rates hide the
     failures that never reached the server.
     Weak answer: "uptime". Uptime of what, observed from where, over what unit of
     work? -->

### SLI: <name>

- Definition (good events / valid events):
- Measured at:
- Data source:
- Known blind spots: <!-- what this SLI cannot see -->

## Objectives

<!-- The target for each SLI and the window over which it is evaluated. Pick a
     target that reflects what users actually need, not the highest number you
     think you can hit — every additional nine costs real engineering that could
     go elsewhere. Rolling windows are usually preferable to calendar ones because
     they do not reset the consequences on the first of the month.
     Say why the number is what it is. An objective with no rationale cannot be
     defended when it becomes inconvenient. -->

| SLI | Target | Window | Rationale |
|---|---|---|---|

## Error budget

<!-- The permitted unreliability implied by each objective, expressed in a unit
     people can feel — minutes of downtime, or failed requests per window. Then the
     part that gives the SLO teeth: how the budget is tracked, who watches the burn
     rate, and what alerting exists for fast burn.
     Note that spending the budget is not a failure. An unspent budget usually means
     the target is too conservative and the team is over-investing in reliability. -->

| SLI | Budget per window | Current consumption | Burn-rate alert |
|---|---|---|---|

## Error budget policy

<!-- Agreed in advance, in writing, by the people it will constrain. State what
     changes when the budget is exhausted — for example: feature releases pause and
     reliability work takes priority until the window recovers. Include who can
     grant an exception, on what grounds, and how the exception is recorded.
     A policy with an unlimited, unlogged override is decoration. -->

- When the budget is healthy:
- When burn is fast (define fast):
- When the budget is exhausted:
- Exception authority and how exceptions are recorded:

## Exclusions

<!-- What does not count against the budget: agreed maintenance windows, failures
     caused by a dependency outside the boundary, synthetic traffic. Keep this list
     short and justified — generous exclusions are the usual way an SLO becomes
     unfalsifiable. -->

## Reporting

<!-- Where the current status is visible, and where it gets discussed — normally the
     weekly operations review. State the cadence for revisiting the targets
     themselves; objectives set once and never revised stop describing the service. -->

- Dashboard:
- Reviewed at:
- Targets revisited every:

## Change log

<!-- Every change to an SLI definition or target, with the date and reason. Without
     this, a quietly relaxed target looks like an improvement in reliability. -->

| Date | Change | Reason | Approved by |
|---|---|---|---|

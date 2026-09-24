---
title: Weekly Operations Review
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's weekly production meeting, whose purpose is to keep operational load and reliability state visible to the whole team (sre.google/sre-book)."
---

# Weekly Operations Review

<!-- A standing meeting with a fixed agenda. Its value comes from being boring and
     unmissable: the same questions every week mean trends surface early instead of
     arriving as burnout. Keep these notes as the record — a meeting with no written
     output cannot hold anyone to an action item. -->

## Meeting details

- Week / period covered:
- Date:
- Chair:
- Attendees:
- Absent (and who covers their items):

## 1. Paging volume

<!-- Count of pages, split by alert and by whether the page was actionable. The
     number to watch is not the total but the share that required no human action —
     that is the noise slowly training people to ignore the pager.
     Also record out-of-hours pages separately. Two daytime pages and two 4am pages
     are not the same operational load. -->

| Alert | Pages | Actionable | Out of hours | Action |
|---|---|---|---|---|

<!-- Weak answer: "a few alerts, nothing major". If it is not counted it is not
     trended, and untrended paging load is how a rotation quietly becomes
     unsustainable. -->

## 2. SLO and error-budget status

<!-- For each objective: current attainment, budget consumed this window, and burn
     rate. Reference the registry (`adb slo set` entries) rather than retyping
     targets. State plainly whether the error-budget policy is in force this week
     and what that means for the release plan. -->

| SLO | Attainment | Budget consumed | Burn rate | Policy in force? |
|---|---|---|---|---|

## 3. Incidents since last review

<!-- Every incident, however small, with its severity, whether a postmortem was
     triggered, and the current status of that postmortem. Include incidents that
     did not meet the trigger criteria and say why — that judgement should be
     visible to the team, not made privately. -->

| Incident | Severity | Duration | Postmortem? | Status |
|---|---|---|---|---|

## 4. Action-item follow-through

<!-- The section that decides whether this meeting matters. Every open action item
     from postmortems and previous reviews, with its owner and age. Read the overdue
     ones aloud and either re-commit with a new date or close them explicitly as
     "not doing, because …".
     Items silently carried week after week teach everyone that postmortem actions
     are optional. -->

| # | Action | Source | Owner | Age | Due | Status |
|---|---|---|---|---|---|---|

- Items closed this week:
- Items explicitly dropped (with reason):
- Items overdue by more than one review cycle: <!-- escalate these -->

## 5. Toil and operational load

<!-- Manual, repetitive work that kept someone busy this week: manual restarts,
     data fixes, access requests, one-off queries. Name the largest single source
     and whether anyone owns automating it. Toil that is never written down is never
     budgeted against. -->

## 6. Upcoming risk

<!-- The week ahead: planned releases, migrations, dependency changes, expected
     traffic events, expiring certificates or credentials, on-call gaps and holidays.
     For each, who is watching it and whether a rollback plan exists.
     This is the cheapest slot in the week to notice that a risky change and a thin
     on-call rotation coincide. -->

| Risk / event | When | Owner | Mitigation in place |
|---|---|---|---|

## 7. Capacity and cost notes

<!-- Anything trending toward a limit: a quota, a disk, a queue depth, a spend line.
     Only entries with a direction and a rate — a static number here tells nobody
     anything. -->

## Decisions made

<!-- Decisions taken in the meeting, each with the decider. Discussion that produces
     no decision and no action item should be noted as such, so it is not
     rediscovered next week as if it were new. -->

## New action items

| # | Action | Owner | Priority | Due |
|---|---|---|---|---|

---
title: Production Readiness Review
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's Production Readiness Review and its six axes — architecture and dependencies, instrumentation and metrics, emergency response, capacity planning, change management, and performance (sre.google/sre-book)."
---

# Production Readiness Review

<!-- This is an ACCEPTANCE GATE, not a status report. It records the terms on
     which an on-call rotation agrees to be paged for this system. If nobody has
     to sign it, it is documentation theatre. Fill it in with the people who will
     carry the pager in the room. -->

## Service summary

<!-- Two or three sentences: what the service does, who depends on it, and what
     breaks for a user when it is down. If you cannot describe the user-visible
     impact of an outage, the rest of this review has no yardstick. -->

## Service level objectives

<!-- Agree these UP FRONT, before the review, not as an afterthought at sign-off.
     Availability, latency, and correctness targets with a measurement window,
     plus what the team does when the error budget is spent. Register them with
     `adb slo set` so there is one machine-readable copy; link it here rather than
     restating the numbers in prose that will drift.
     Weak answer: "99.9%" with no SLI definition, no window, and no budget policy —
     that is a slogan, not an objective. -->

## Axis 1 — System architecture and dependencies

<!-- The shape of the system and everything it needs to stay up. Diagram or
     describe the request path; then list each dependency with its criticality
     (hard / soft), its own reliability, and what your service does when it is
     unavailable — fail closed, fail open, degrade, queue.
     Watch for: a dependency chain whose weakest link makes your SLO arithmetically
     impossible. Say so here rather than discovering it in an incident. -->

| Dependency | Hard / soft | Behaviour on failure | Owner |
|---|---|---|---|

<!-- Also cover: single points of failure, blast radius of one instance or one
     region going away, and any data store whose loss is unrecoverable. -->

## Axis 2 — Instrumentation, metrics and monitoring

<!-- Can you tell, from outside the system, whether it is healthy? For each SLI:
     where the signal comes from, how it is aggregated, and the dashboard someone
     opens first. Then the alerts: each one must be actionable and tied to
     user-visible symptoms rather than to a cause.
     Weak answer: a wall of CPU and memory graphs with no alert on the thing the
     user actually experiences. Also state what is NOT observable yet. -->

## Axis 3 — Emergency response

<!-- Who is paged, through which channel, with what expected acknowledgement time,
     and what they read when the page fires. The runbook may not exist yet at this
     point — record which alerts still lack an entry, because that list is a
     release blocker, not a nice-to-have.
     Cover escalation: second-tier contacts, the dependency owners, and who can
     authorise a rollback out of hours. -->

## Axis 4 — Capacity planning

<!-- Current demand, expected demand, and the headroom between them. Include the
     resource that saturates first, the lead time to add more of it, quotas and
     limits that would bite before hardware does, and the load level at which the
     system degrades rather than falls over.
     Weak answer: "it autoscales". Autoscaling has limits, costs, and a warm-up
     time — state them. -->

## Axis 5 — Change management

<!-- How change reaches production safely: the release path, gating tests (see the
     test strategy), progressive rollout or canary strategy, feature flags, and the
     rollback route. Configuration and secrets count as change too — say how they
     are versioned and reviewed.
     Note the freeze windows and who can override them. -->

## Axis 6 — Performance

<!-- Availability, latency, and efficiency together. Measured latency at the
     percentiles that matter, the known bottleneck, and cost per unit of work with
     the trend. Efficiency belongs in a readiness review because an unaffordable
     service gets throttled or switched off, which is an availability problem with
     a finance label on it. -->

## Security and compliance

<!-- Run `adb audit security` and record the result and any accepted findings here
     rather than duplicating its checks. Add what the tool cannot see: data
     classification, retention, access review, and the audit trail. -->

## Backup and recovery

<!-- What is backed up, how often, where it lands, and — the part usually skipped —
     the last time a restore was actually performed. State the recovery point and
     recovery time you can defend, not the ones you would like. -->

## Blockers

<!-- Anything that must be resolved before acceptance, each with an owner and a
     date. An empty list on a first pass is a sign the review was not adversarial
     enough. -->

| # | Blocker | Owner | Due | Status |
|---|---|---|---|---|

## Accepted risks

<!-- Known gaps the reviewers agree to live with, each with the reason, the
     mitigation, and an expiry date after which it must be revisited. Risks
     without expiry dates become permanent by silence. -->

## Decision

<!-- Accepted / accepted with conditions / not accepted. Name the reviewer who
     will be paged, the date, and the conditions. A conditional acceptance must
     list the conditions and their deadlines here, in this file. -->

- Decision:
- Reviewer (carries the pager):
- Date:
- Conditions:

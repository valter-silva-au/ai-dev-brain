---
title: Launch Checklist
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's launch coordination practice and its use of a reusable launch checklist to make releases boring (sre.google/sre-book)."
---

# Launch Checklist

<!-- A launch checklist exists so that the same avoidable mistake is not made
     twice across the organisation. Tick items only when they are actually true —
     a checklist filled in optimistically is worse than none, because it transfers
     false confidence to the go/no-go decision.
     Every unchecked item is either a blocker or an explicitly accepted risk.
     There is no third state. -->

## Launch summary

- What is launching:
- Version / commit:
- Target date and time (with timezone):
- Launch coordinator:
- Rollout strategy (all at once / canary / percentage ramp / flag):

## Prerequisites

<!-- Upstream work that must be complete before launch day, each with evidence
     rather than an assertion: link the review, the ticket, the test run. -->

- [ ] Production readiness review accepted, conditions closed <!-- link it -->
- [ ] Test strategy gates green on the release candidate <!-- link the run -->
- [ ] Runbook entry exists for every alert that can page
- [ ] Rollback plan written and rehearsed
- [ ] Dependencies notified and their owners aware of the date
- [ ] Required approvals obtained <!-- name them; "someone said yes in a call" is not evidence -->

## Configuration and secrets

<!-- Configuration is the most common cause of launch-day failure because it is
     the part that is not in the artefact. Confirm the values in the target
     environment, not in the repository. -->

- [ ] Production configuration reviewed by a second person
- [ ] Secrets present in the target environment and rotated if newly created
- [ ] Feature flags in their intended launch state, with the default documented
- [ ] Quotas, limits and rate limits raised ahead of expected load
- [ ] Infrastructure changes applied and version-controlled

## Monitoring in place

<!-- "In place" means you have seen the signal move, not that the dashboard exists.
     Confirm before launch, because a monitoring gap discovered during the ramp
     leaves you flying blind at the worst moment. -->

- [ ] SLIs emitting and visible on the primary dashboard
- [ ] Alerts firing correctly (verified with a test or a known-bad condition)
- [ ] Alert routing reaches a human who is awake
- [ ] Logs searchable, with the query for the new code paths written down
- [ ] A named person watching during the launch window

## On-call readiness

- [ ] On-call rotation covers the launch window and the following days
- [ ] The person on call has read the runbook and knows the change is happening
- [ ] Escalation path confirmed, including out-of-hours
- [ ] Rollback authority named and reachable for the whole window

## Communications

<!-- Who needs to know what, before / during / after. Include the internal channel,
     any customer-facing notice, and the status-page plan if one exists. Silence
     during a launch reads as an outage to everyone outside the room. -->

| Audience | Message | When | Channel | Owner |
|---|---|---|---|---|

## Validation after launch

<!-- The specific checks performed immediately after the change is live, with the
     expected result. "Smoke test" is not a check; name the journeys and the
     signals. Include the observation window before declaring success. -->

- [ ] Key user journeys verified in production
- [ ] Error rate and latency within SLO after <!-- state the window -->
- [ ] No unexpected alerts during the observation window
- [ ] Dependency owners report no anomalies

## Go / no-go

<!-- A human decision at a stated time, recorded here. List any open blocker and
     the accepted risks the decider is knowingly taking on. Self-approval by the
     person shipping removes the last cheap chance to stop. -->

- Decision (go / no-go / go with conditions):
- Decided by:
- Timestamp:
- Open blockers at decision time:
- Accepted risks:

## Rollback readiness confirmation

<!-- Restate the single trigger most likely to fire and who is watching for it, so
     the rollback plan is loaded rather than merely filed. -->

- Primary rollback trigger being watched:
- Watcher:
- Rollback plan location:

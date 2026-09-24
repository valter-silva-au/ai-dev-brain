---
title: Rollback Plan
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: change-management and continuous-delivery practice on reversible releases, and the Google SRE book's guidance on rolling back rather than debugging forward during an incident (sre.google/sre-book)."
---

# Rollback Plan

<!-- The point of this document is that the decision to roll back can be made in
     seconds by someone under pressure, using criteria agreed while everyone was
     calm. Write the triggers first; the procedure is the easy half.
     Weak answer: "redeploy the previous version". That sentence hides the data
     migration, the cache, the feature flag, and the client that has already
     cached the new API shape. -->

## What this change does

<!-- One paragraph on the release being covered: version or commit, what it
     touches, and which components move. A rollback plan that does not name its
     change cannot be matched to an incident later. -->

- Change / release identifier:
- Components affected:
- Date of release:

## Trigger conditions

<!-- The conditions under which we roll back WITHOUT further debate. Make them
     observable and thresholded, so the on-call engineer does not have to
     interpret intent. Include a time box: "no diagnosis within N minutes" is a
     legitimate trigger and prevents heroic debugging during an outage.
     Weak answer: "if it looks bad". -->

| # | Trigger (observable) | Threshold | Who observes it |
|---|---|---|---|

## Conditions where we do NOT roll back

<!-- Cases where rolling back makes things worse — for example after an
     irreversible migration has run, or when the previous version has a known
     worse defect. Name the alternative mitigation for each. -->

## Decision authority

<!-- Who can call the rollback, who can call it out of hours, and what happens if
     that person is unreachable. A single named human, plus a fallback. Consensus
     is not a decision procedure during an outage. -->

- Primary decider:
- Out-of-hours / fallback:
- Who must be informed (not asked):

## Procedure

<!-- Numbered, exact, complete commands in the order they must run. Each step
     states its expected outcome and how to verify before moving on. Mark the
     point of no return, if there is one, in bold. Include the steps that are easy
     to forget: feature flags, cache invalidation, queue draining, client-side
     assets, scheduled jobs. -->

1. <!-- Announce: who to tell, in which channel, with what content. -->
2. <!-- Freeze: stop the pipeline and any in-flight deploys so the rollback is not
        overwritten by an automatic redeploy. -->
3. <!-- Revert the artefact: exact command, target version. -->
4. <!-- Revert configuration and flags: they usually do not travel with the
        artefact. -->
5. <!-- Handle data: see the section below. -->
6. <!-- Verify: the specific signals that confirm recovery, not just "it's up". -->
7. <!-- Unfreeze and record. -->

## Data and migration reversibility

<!-- The section that decides whether a rollback is possible at all. For each schema
     or data change: is it backward compatible, is there a down migration, has that
     down migration been executed against realistic data, and what happens to rows
     written by the new version after a revert.
     If a change is irreversible, say so plainly here and describe the
     forward-fix path instead. Discovering irreversibility mid-incident is how a
     recoverable outage becomes permanent data loss. -->

| Change | Reversible? | Down procedure | Rehearsed on |
|---|---|---|---|

## Expected duration

<!-- Realistic wall-clock time for detection, decision, execution, and verification,
     stated separately. If the total exceeds the acceptable outage window, the plan
     is not adequate and the release strategy needs to change (canary, flag, or
     dark launch). -->

- Detection to decision:
- Execution:
- Verification:
- Total:

## Blast radius during rollback

<!-- What users experience while the rollback runs: errors, degraded features, lost
     writes, cache misses. Also what other teams see, so they are not surprised. -->

## Rehearsal record

<!-- When this procedure was last executed for real, in which environment, by whom,
     and what surprised you. An unrehearsed rollback plan is a hypothesis with
     formatting. -->

## Post-rollback follow-up

<!-- What happens after the system is stable: the incident record, whether the
     postmortem trigger criteria are met, and who owns the forward fix. -->

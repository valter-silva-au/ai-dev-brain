---
title: Postmortem
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's blameless postmortem culture — its trigger criteria, review expectations, and the principle that an unreviewed postmortem might as well never have existed (sre.google/sre-book)."
---

# Postmortem

<!-- BLAMELESS. Focus on the contributing causes — the systems, defaults and
     processes that allowed this — without indicting any individual. Blameless is
     not toothless: it is the opposite of vague. It demands that you say precisely
     how the system must change, on the assumption that competent people acting
     reasonably will make the same choice again unless something structural moves.
     If a sentence names a person as the cause, rewrite it to name the mechanism
     that let one person's ordinary mistake reach production. -->

## When to write one — trigger criteria

<!-- Kept inline deliberately, so nobody has to look up whether this is warranted.
     Write a postmortem if ANY of these is true:
       - user-visible downtime or degradation beyond the agreed threshold
       - any data loss, however small
       - on-call intervention was required — a rollback, a traffic reroute, a
         manual failover
       - resolution time exceeded the agreed threshold
       - a monitoring failure: the problem was found by a human or a customer
         rather than by an alert
       - any stakeholder asks for one
     Record which criterion applied. Thresholds are a per-team decision; state
     yours next to the criterion rather than leaving "beyond the threshold"
     unresolved. -->

- Trigger criterion met:
- Threshold in force:

## Metadata

- Incident ID:
- Date of incident:
- Author:
- Reviewers: <!-- at least one non-responder -->
- Status: draft / in review / final
- Severity:
- Related timeline: <!-- link the incident timeline; do not restate it here -->

## Summary

<!-- Three or four sentences a reader outside the team can follow: what broke, who
     was affected, for how long, and what fixed it. Written last even though it
     appears first. Weak answer: internal jargon and component names with no user
     impact. -->

## Impact

<!-- Quantified where possible: duration, requests failed, users or tenants
     affected, data lost or corrupted, revenue or contractual consequence,
     downstream teams disrupted. Include error-budget consumption if an SLO exists.
     Mark estimates as estimates. -->

## Trigger

<!-- The proximate event that started it — the deploy, the config change, the
     traffic spike, the certificate expiry. One or two lines. This is the *trigger*,
     deliberately separated from the causes below so the two do not get conflated. -->

## Root cause

<!-- The contributing causes, not a person. Go deeper than the proximate trigger —
     "the deploy broke it" is a symptom, not a cause. Ask what allowed the change
     to reach production undetected, what made the failure mode invisible, and what
     made recovery slow. Blameless does not mean toothless: name the system or
     process that must change.
     Usually there is more than one cause. List them; a single-cause narrative is
     often a sign the analysis stopped early. -->

## Detection

<!-- How the problem was noticed, and how long that took. If a human or a customer
     found it before monitoring did, say so plainly — that is a finding in its own
     right and one of the trigger criteria above. Note any alert that should have
     fired and did not, and why. -->

## Response and recovery

<!-- What responders did, what worked, and what wasted time: a missing runbook
     entry, an access problem, an escalation that went unanswered, a dashboard that
     misled. Reference the timeline instead of repeating it. -->

## What went well

<!-- Genuinely — the alert that did fire, the rollback that worked, the drill that
     paid off. Keeping this section honest is what makes the critical sections
     credible rather than performative. -->

## Where we got lucky

<!-- The circumstances that limited the damage but cannot be relied on: off-peak
     timing, an engineer who happened to be online, a cache that happened to be
     warm. Each entry is a latent risk still in place, and often the most valuable
     part of the document. -->

## Action items

<!-- Each item must have a single named owner, a priority, and a tracking link.
     Prefer changes that remove the failure mode over changes that ask humans to be
     more careful; "be more careful" is not an action item.
     Distinguish mitigation (reduces impact next time) from prevention (stops it
     happening). A list of only mitigations means the cause is still live. -->

| # | Action | Type (prevent / mitigate / detect) | Owner | Priority | Tracking | Due |
|---|---|---|---|---|---|---|

## Lessons learned

<!-- What the team now knows that it did not before, phrased so another team could
     benefit. Not a restatement of the action items. -->

## Review

<!-- An unreviewed postmortem might as well never have existed. Review criteria:
       - the timeline is complete and consistent with the evidence
       - impact is quantified, not adjectival
       - the analysis goes past the proximate trigger to contributing causes
       - the language is blameless AND specific about required system change
       - every action item has one named owner, a priority and a tracking link
       - the "got lucky" section is filled in honestly
     Record the reviewers, the date, and any dissent that was not resolved. -->

- Reviewed by, date:
- Review criteria met (list any exceptions):
- Unresolved disagreement:
- Circulated to: <!-- who reads it beyond the team; a postmortem read only by its authors teaches nobody -->

---
title: Operational Runbook
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's operational documentation and playbook practice, and its guidance that on-call engineers should not have to reason from first principles during an incident (sre.google/sre-book)."
---

# Operational Runbook

<!-- Write this for someone woken at 3am who did not build the system, does not
     know your variable names, and will copy and paste whatever you put here.
     Every command must be complete and safe to run as written. Every step must
     say what it does to production.
     Weak answer: "investigate the logs". Which logs, queried how, looking for
     what string? -->

## How to use this document

<!-- Point the reader straight at the alert index below. State the one rule that
     overrides everything else: mitigate first, diagnose second. Say where to ask
     for help if none of the entries match. -->

## Service at a glance

<!-- The minimum context needed to act: what the service does, its entry points,
     where it runs, the primary dashboard, and the log query interface. Links, not
     prose. -->

- Purpose:
- Dashboards:
- Logs:
- Deploy tool / pipeline:
- Owning team and escalation channel:

## Safety rules

<!-- The things that must never be done under pressure: destructive commands that
     look harmless, the account or environment that is easily confused with
     production, the migration that cannot be reversed. Put them before the
     procedures, because that is where a tired reader will see them. -->

## Common operations

<!-- Routine, non-incident procedures: restart, scale, drain, rotate a credential,
     flush a cache, replay a queue. One heading each, with the exact command and
     the expected output so the reader can tell success from silence. -->

### <Operation name>

<!-- Preconditions, the command, expected result, and how to verify. Note whether
     it is safe to repeat — idempotency matters when a step appears to hang. -->

## Alert index

<!-- One entry per alert that can page a human. If an alert has no entry here, it
     should either get one or stop paging. Keep the alert names byte-identical to
     the monitoring configuration so the reader can search. -->

### Alert: <exact alert name>

**Symptom**

<!-- What the person sees: the alert text, the graph shape, the user complaint.
     Describe it in the words the pager will use, not in internal jargon. -->

**Likely causes**

<!-- Ranked, most common first, each with the signal that distinguishes it from
     the others. "Could be anything" means this entry is not finished. -->

**Diagnostic steps**

<!-- Numbered, each one a concrete command or query with the interpretation of its
     output. Say what result sends the reader down which branch. Keep read-only
     steps before anything that changes state. -->

**Remediation**

<!-- The action that stops the bleeding, with the exact command, its blast radius,
     and how to confirm it worked. If the fix is a rollback, link the rollback plan
     rather than duplicating it. Note anything that must be undone afterwards. -->

**Escalation**

<!-- When to stop trying and who to wake: the trigger (elapsed time, impact
     threshold, unknown cause), the contact, and what information to hand over. -->

**Notes**

<!-- Past occurrences, related postmortems, and known false-positive conditions. -->

<!-- Repeat the block above for each alert. Resist the urge to merge several alerts
     into one entry: at 3am the reader searches for the exact string on the page. -->

## Escalation paths

<!-- The full ladder: primary on-call, secondary, dependency owners, and who can
     authorise something irreversible or customer-visible. Include out-of-hours
     routes and the channel of last resort. -->

| Situation | Contact | Channel | Expected response |
|---|---|---|---|

## Known limitations

<!-- Failure modes with no good remediation yet, and the ticket tracking each one.
     Saying "there is no fix, mitigate by X and file against Y" is far more useful
     than an empty section that implies coverage you do not have. -->

## Verification

<!-- When this runbook was last exercised, by whom, and against which alert. An
     unexercised runbook is an untested code path. Record the date so its decay is
     visible. -->

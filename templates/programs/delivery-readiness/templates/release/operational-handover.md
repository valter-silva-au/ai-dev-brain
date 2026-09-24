---
title: Operational Handover
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's service onboarding and handover practice, including the expectation that developer involvement tapers rather than stops abruptly (sre.google/sre-book)."
---

# Operational Handover

<!-- This document is the moment ownership changes hands. It only means something
     if both sides sign it and the receiving side could run the system tomorrow
     without the builders. Anything the receiver cannot yet do is a gap listed
     below, not an optimistic omission. -->

## Parties

- Handing over (built it):
- Receiving (will operate it):
- Effective date:
- End of the support tail (see below):

## What is being handed over

<!-- Enumerate the concrete surface: services, jobs, data stores, dashboards,
     pipelines, infrastructure, and the on-call responsibility itself. Anything not
     on this list stays with the builders — make that explicit so there is no
     ambiguous middle ground. -->

| Component | Repository / location | Notes |
|---|---|---|

## What is explicitly NOT handed over

<!-- Retained ownership: roadmap and feature work, a component still in flight, a
     dependency owned elsewhere. Naming these prevents the receiver being paged for
     something they never accepted. -->

## Documentation transferred

<!-- Link the artefacts the receiver will actually rely on and state their
     freshness. A stale runbook is a liability, so say when each was last verified
     rather than merely that it exists. -->

| Artefact | Location | Last verified |
|---|---|---|
| Production readiness review | | |
| Runbook | | |
| Rollback plan | | |
| Architecture / dependency map | | |
| SLO definitions | | |

## Training delivered

<!-- What was actually done, not what was offered: walkthrough sessions, a shadowed
     on-call shift, a rehearsed failure drill, a rollback executed by the receiver's
     own hands. Record dates and attendees.
     Weak answer: "sent them the docs". Reading is not training. -->

| Session | Date | Attendees | Covered |
|---|---|---|---|

## Access and permissions

<!-- The receiver must have everything needed to act during an incident before the
     handover date, verified by them logging in — not by a ticket being closed.
     Include the things usually forgotten: dashboards, log retention, cloud console
     roles, deploy pipeline permissions, secret stores, third-party vendor
     accounts, paging tool. -->

| System | Access level needed | Granted | Verified by receiver |
|---|---|---|---|

<!-- Also state what access the handing-over team retains and for how long, and who
     removes it afterwards. Indefinite founder access quietly becomes the real
     escalation path. -->

## On-call and alerting

<!-- Which rotation now receives the pages, when routing changes, and whether the
     first weeks are shadowed. Confirm every alert has a runbook entry — the
     receiver should reject the handover if it does not. -->

## Known issues and residual risk

<!-- Open defects, accepted risks from the readiness review with their expiry dates,
     brittle areas, and any operational chore that is still manual. Handing over a
     clean-looking system that has known sharp edges destroys trust the first time
     one of them cuts. -->

| Issue | Impact | Workaround | Tracking |
|---|---|---|---|

## Residual developer involvement

<!-- The support tail: what the builders remain available for, through which channel,
     with what response expectation, and the date it ends. Taper deliberately —
     an unbounded tail means the handover never really happened, while a hard cut
     on day one means the first novel failure has nobody who understands it. -->

- Scope of continued involvement:
- Channel and response expectation:
- Taper schedule / end date:

## Open gaps at handover

<!-- Anything not ready, with an owner and a date. If this section is empty on a
     first draft, it is more likely incomplete than perfect. -->

## Sign-off

<!-- Both signatures, dated. The receiver's signature means: we have the access, we
     have the documentation, we have been trained, and we accept the pager. -->

- Handing over — name, role, date:
- Receiving — name, role, date:
- Conditions attached to acceptance:

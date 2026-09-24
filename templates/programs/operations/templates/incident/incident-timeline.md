---
title: Incident Timeline
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Google SRE book's incident management practice, in particular the live written record kept by a scribe during response (sre.google/sre-book)."
---

# Incident Timeline

<!-- FACTS ONLY. This document records what happened and when. It does not explain
     why, does not assign blame, and does not propose fixes — that is the
     postmortem's job, and it needs an uncontaminated record to work from.
     Keep it live during the incident if you can; a timeline reconstructed from
     memory a week later gets detection and escalation times wrong, which is
     precisely the data used to judge whether monitoring was adequate. -->

## Identification

- Incident ID:
- Severity:
- Start (UTC):
- End (UTC):
- Detected by: <!-- alert name, or the human/customer who reported it -->
- Incident commander:
- Scribe:
- Responders:

## Impact snapshot

<!-- One or two factual lines: what was unavailable or wrong, for whom, for how
     long. Numbers if you have them, "unknown" if you do not. Do not estimate here
     and then let the estimate harden into a fact. -->

- User-visible effect:
- Duration of impact:
- Affected surface (regions, tenants, percentage of traffic):
- Data loss: <!-- yes / no / unknown — this answer alone can trigger a postmortem -->

## Timeline

<!-- All timestamps in UTC, in the format YYYY-MM-DD HH:MM. One row per event.
     Include the boring rows: the failed command, the wrong dashboard, the paged
     person who did not answer. Those gaps are where the improvements live.
     Write only observations and actions. If you catch yourself writing "because",
     move that sentence to the postmortem. -->

| Time (UTC) | Actor | Event |
|---|---|---|
| | | <!-- change deployed / first anomaly in metrics --> |
| | | <!-- alert fired, or first human report --> |
| | | <!-- acknowledged by on-call --> |
| | | <!-- escalated to … --> |
| | | <!-- diagnostic action and what it showed --> |
| | | <!-- mitigation applied --> |
| | | <!-- impact confirmed ended --> |
| | | <!-- all-clear declared, monitoring returned to normal --> |

## Key intervals

<!-- Derived from the table above, stated once so the postmortem does not have to
     recompute them. If a number is not measurable from the timeline, say so
     rather than estimating. -->

- Time to detect (change or onset → alert/report):
- Time to acknowledge (alert → human response):
- Time to mitigate (acknowledgement → impact ends):
- Time to resolve (onset → all clear):

## Communications sent

<!-- Every external or cross-team message, with time and audience. Useful for
     checking later whether stakeholders learned about the incident from us or
     from their own users. -->

| Time (UTC) | Audience | Channel | Content summary |
|---|---|---|---|

## Evidence

<!-- Links to the durable artefacts: dashboard snapshots with a fixed time range,
     log queries, chat transcript, alert history, deploy record. Live links that
     will re-render "now" are useless in three months — pin the time window. -->

## Open factual questions

<!-- Things nobody could establish during response: a missing metric, an
     unretained log, an ambiguous timestamp. List them; each one is a monitoring or
     observability gap for the postmortem to act on. -->

## Postmortem trigger assessment

<!-- Check the trigger criteria stated in the postmortem template and record the
     answer here, with who decided. Recording "no postmortem required" and why is
     itself valuable — it stops the question being reopened from memory. -->

- Postmortem required (yes / no):
- Criterion met:
- Decided by, date:

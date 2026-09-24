---
title: Measurement Framework
owner:
date:
sources: []          # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the HEART framework and the goals-signals-metrics method published by Rodden, Hutchinson and Fu (Google), and general analytics-instrumentation practice."
---

# Measurement Framework

<!-- Purpose: get from "what we care about" to "what we can actually observe"
     without skipping the middle step. The middle step — signals — is where
     most measurement plans quietly fail, because a goal is mapped straight to
     whatever number happens to be available. -->

## Scope of measurement

<!-- What experience or feature this framework covers, and its boundary. A
     framework covering "the product" measures nothing in particular. -->

## Goals, signals, metrics

<!-- Work left to right, one row at a time, and resist filling metrics first.
     Goal: what success looks like for the user, stated without reference to a
     number.
     Signal: the observable behaviour that would change if the goal were met or
     missed. Behaviour, not a count.
     Metric: the specific number that captures the signal, with its
     denominator. A ratio without a denominator is not a metric.
     If a row's metric does not follow from its signal, you have found a
     measurement gap — record it rather than papering over it. -->

| Category | Goal (user-facing) | Signal (observable behaviour) | Metric (with denominator) |
|---|---|---|---|
| Happiness |  |  |  |
| Engagement |  |  |  |
| Adoption |  |  |  |
| Retention |  |  |  |
| Task success |  |  |  |

<!-- Not every category applies to every feature, and forcing all five
     produces filler. Cross out the ones that do not apply and say why in one
     line — an explicit omission is a finding; a padded row is noise. -->

### Categories deliberately omitted

<!-- Which of the five you are not measuring here, and why. For a one-off
     migration tool, for example, retention may be meaningless. -->

## Instrumentation status

<!-- The honest state of each metric today. This section is the difference
     between a measurement plan and a wish: a dashboard specified on data that
     is not collected will be empty on the day the decision has to be made.
     Note privacy and permission constraints here too. If you are not allowed
     to collect it, it does not matter that it is technically possible. -->

| Metric | Available today? | What is needed | Owner of the work | Ready by | Collection permitted? |
|---|---|---|---|---|---|
|  | yes / no / partial |  |  |  | yes / needs review |

## Segments

<!-- How you will cut the data. An aggregate number hides the case you most
     need to see — new versus existing users, small versus large accounts, the
     ones who abandoned partway.
     Name the segment that would most likely contradict a positive headline
     result, and commit to looking at it. -->

## Guardrails

<!-- Thresholds that would trigger action regardless of the headline metrics:
     error rates, latency, support volume, opt-outs. State the number and who
     gets told. -->

| Guardrail | Threshold | Who is alerted | Action on breach |
|---|---|---|---|
|  |  |  |  |

## Reporting

<!-- Where these land, how often, and who is accountable for the number being
     correct. Include the first date the framework produces a usable reading —
     if that is after the decision it informs, the framework needs rethinking
     now, not then. -->

| | |
|---|---|
| Where reported |  |
| Cadence |  |
| Accountable for data quality |  |
| First usable reading |  |

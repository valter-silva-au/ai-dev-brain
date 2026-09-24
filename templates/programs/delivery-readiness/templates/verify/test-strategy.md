---
title: Test Strategy
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the test pyramid (Mike Cohn, Succeeding with Agile), continuous-delivery quality gates, and the Google SRE book's treatment of testing for reliability (sre.google/sre-book)."
---

# Test Strategy

<!-- One page that answers: what do we test, at which layer, and what has to be
     green before this ships. Written before the tests, not reconstructed after.
     Weak answer: a list of frameworks. Nobody needs to be told you use a test
     runner; they need to know which risks the suite is designed to catch. -->

## Scope and risk

<!-- What is being tested and, more importantly, what could go wrong. List the
     failure modes you actually fear — data corruption, silent auth bypass, a
     migration that cannot be undone. Each layer below should trace back to one
     of these. Weak answer: "the new feature". -->

## What is out of scope

<!-- Named exclusions with a reason: third-party behaviour you trust, legacy paths
     frozen for deletion, load levels you will not reach this quarter. An empty
     out-of-scope section usually means the scope was never bounded. -->

## Layers

### Unit

<!-- What a unit test means here (isolation boundary, what gets faked), and which
     logic must have one: pure business rules, validation, edge cases, error
     branches. Fast and deterministic — if it touches a network or a clock it is
     not a unit test. -->

### Integration

<!-- Components crossing a real boundary: database, queue, cache, an internal API.
     State which boundaries are exercised for real and which are stubbed, and how
     test data and fixtures are set up and torn down. -->

### System / end-to-end

<!-- The handful of user-visible journeys that must work. Keep this small and
     ruthless — E2E tests are slow and flaky in proportion to their number. Say
     which environment they run against and how it is seeded. -->

### Performance

<!-- Latency and throughput targets and how they are measured: percentiles (not
     averages), the load profile, the duration, and where the test runs. Tie the
     numbers to the SLOs if they exist. Weak answer: "should be fast". -->

### Security

<!-- Authentication and authorisation tests, input validation, dependency and
     secret scanning, and any abuse cases worth asserting. Cross-check against
     `adb audit security` rather than restating what that command already
     enforces. -->

## Quality gates

<!-- The binary conditions that block a merge or a release. For each gate: what is
     measured, the threshold, where it runs (pre-commit / CI / pre-release), and
     who can waive it. A gate nobody may waive but everybody bypasses is worse
     than no gate. -->

| Gate | Measured by | Threshold | Runs at | Waiver |
|---|---|---|---|---|
| | | | | |

## Coverage target

<!-- State the number your team chose AND the reasoning. Coverage is a decision,
     not a constant handed down from elsewhere: pick a floor that matches the risk
     of this code and name the areas held to a higher bar. Say explicitly which
     paths are excluded from the measurement and why. Weak answer: a round
     percentage with no rationale, which invites tests written to move the metric
     rather than to catch defects. -->

## Acceptance criteria

<!-- The observable conditions under which this work is done, phrased so a third
     party can check them without asking you. One line each, testable, no "works
     correctly". These feed the production readiness review. -->

## Test data and environments

<!-- Where test data comes from, how production-like it is, and how anything
     sensitive is anonymised or synthesised. Name the environments and who can
     reset them. -->

## Flakiness policy

<!-- What happens when a test fails intermittently: quarantine, an owner, a
     deadline for repair, and the rule against re-running until green. Without a
     stated policy the suite decays into noise that everyone ignores. -->

## Ownership

<!-- Who maintains the suite after this change lands, and who is called when CI
     goes red on a branch that is not theirs. -->

## Open questions

<!-- Unresolved decisions, with the person who can settle each one. Leave them
     visible rather than guessing. -->

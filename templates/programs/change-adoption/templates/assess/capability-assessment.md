---
title: Capability Assessment
owner:
date:
sources: []          # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: Team Topologies (Matthew Skelton and Manuel Pais) — four team types, three interaction modes, and cognitive load as the real capability constraint — combined with standard skills-matrix practice."
---

# Capability Assessment

<!-- Purpose: establish what the team can actually absorb, before anyone
     commits to what it will do.
     The temptation is to treat this as a skills gap exercise. It usually is not.
     A team can hold every skill on the list and still be unable to take on the
     change because it has no remaining cognitive capacity — and training will
     not fix that. Assess load first, skills second. -->

## Scope

<!-- Which team or teams, and which change this assessment is for. A general
     capability assessment with no change in view produces a generic answer. -->

## Team type and boundaries

<!-- Classify each team involved. The four types:
     - Stream-aligned: owns a flow of work for a segment of the business.
     - Platform: provides a self-service internal product to reduce others' load.
     - Enabling: helps stream-aligned teams acquire a missing capability, then
       leaves.
     - Complicated-subsystem: owns a part requiring specialist knowledge.
     Misclassification matters: a stream-aligned team being asked to behave
     like a platform team will fail at both.
     Note if a team is really several teams sharing a name, or one team wearing
     three hats — that finding usually explains the load problem below. -->

| Team | Type | What it owns | Does the type match how it actually works? |
|---|---|---|---|
|  | stream-aligned / platform / enabling / complicated-subsystem |  |  |

## Cognitive load

<!-- The real capability constraint. Three kinds:
     - Intrinsic: fundamental to the task (knowing the language, the domain).
     - Extraneous: imposed by the environment (fragile deploys, undocumented
       steps, manual toil). This is the reducible kind.
     - Germane: the domain thinking that actually creates value.
     Count the domains a team is responsible for. Beyond a small handful, no
     amount of training raises capability — the answer is to remove
     responsibilities or reduce extraneous load.
     Include the unwritten load: on-call, escalations, the legacy system only
     one person understands, the meeting cadence. A human on the team must
     confirm this section; it cannot be inferred from a repository, and
     overstating available capacity is how change programmes turn into
     burnout. -->

| Team | Domains owned | Operational / on-call load | Largest source of extraneous load | Remaining capacity for change |
|---|---|---|---|---|
|  |  |  |  | none / minimal / some / clear |

**What we would have to take away to make room:**

<!-- Answer this even if the answer is "nothing, so the change must be smaller".
     A plan that adds without subtracting is a plan that fails quietly. -->

## Interaction modes

<!-- Three modes:
     - Collaboration: two teams work closely for a limited time to discover
       something. High cost, high bandwidth, deliberately temporary.
     - X-as-a-Service: one team consumes something with minimal overhead.
     - Facilitating: one team helps another learn, then withdraws.
     Name the mode you intend and its expected duration. Permanent
     collaboration is a signal that a boundary is in the wrong place. -->

| Teams involved | Mode | Purpose | Expected duration | Exit condition |
|---|---|---|---|---|
|  | collaboration / x-as-a-service / facilitating |  |  |  |

## Skills matrix — current state

<!-- Rate honestly against a defined scale, and validate ratings with the
     people being rated. Self-assessment plus manager check beats either alone.
     Scale: 0 = none, 1 = aware, 2 = can do with help, 3 = independent,
     4 = can teach others.
     Watch for the single-point-of-failure pattern: one person at 4 and
     everyone else at 0 or 1 is a capability the team does not really have. -->

| Capability | Person A | Person B | Person C | Team coverage | Single point of failure? |
|---|---|---|---|---|---|
|  |  |  |  |  | yes / no |

## Target state

<!-- The level actually required for the change to succeed — not the maximum
     achievable. Over-specifying the target inflates the gap and the enablement
     bill.
     For each capability: how many people need to be at what level, and why
     that number. -->

| Capability | Required level | How many people | Why this level is enough |
|---|---|---|---|
|  |  |  |  |

## Gaps

<!-- Current versus target, prioritised, and split by how the gap is best
     closed. Not every gap is a training gap: some are hiring gaps, some are
     borrow-an-enabling-team gaps, and some are best closed by removing the
     need for the capability altogether.
     These gap IDs are what the enablement plan must trace to. -->

| # | Gap | Current | Target | Priority | Best closed by |
|---|---|---|---|---|---|
| G1 |  |  |  | high / medium / low | training / hiring / facilitating team / removing the need |

## Constraints and risks

<!-- What limits how fast capability can grow here: attrition risk, delivery
     pressure, no slack, a key person leaving. Be specific; these determine
     whether the enablement plan is realistic. -->

---
title: Request for Comments
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the Rust RFC template (rust-lang/rfcs, MIT/Apache-2.0) for summary, motivation, guide-level and reference-level explanation, drawbacks, rationale and alternatives, prior art, unresolved questions and future possibilities; Oxide RFD 1 (oxidecomputer/rfd) for the discussion-then-determination lifecycle."
---

# RFC: <title>

<!-- WHEN NOT TO WRITE AN RFC
     An RFC is for a change that is contested, cross-team, or hard to reverse —
     something where writing it down and inviting objections is cheaper than
     finding out later. If the change has no trade-offs, affects only your own
     code, or the document would be an implementation manual with nothing for
     anyone to disagree with, write the program instead. If it is a single
     bounded decision, `adb adr new` is the right artifact — an RFC is heavier
     and expects a discussion period. -->

**State:** <!-- ideation | discussion | published | committed | abandoned -->
**Discussion:** <!-- link to the review thread / PR / meeting where comment happens -->

## Summary

<!-- One paragraph. Someone should be able to read only this and know what is
     being proposed and roughly what changes. No motivation, no justification,
     just the proposal. -->

## Motivation

<!-- Why are we doing this? What use cases does it support? What is the expected
     outcome? Describe the problem in terms of what people cannot do today, with
     evidence where you have it. A weak answer motivates the solution instead of
     the problem ("we need a queue" rather than "writes are lost when the
     downstream is down"). -->

## Guide-level explanation

<!-- Explain the proposal as if it were already shipped and you were teaching it
     to someone who will use it. Use examples. Introduce any new named concepts.
     If it changes existing behaviour, explain the difference in terms a current
     user would recognise. Avoid internals here entirely — if a reader needs the
     implementation to understand the feature, the design may be leaking. -->

## Reference-level explanation

<!-- The precise, technical part: how it works, how it interacts with existing
     behaviour, and enough detail that someone familiar with the system could
     implement it and reach the same result. Cover the corner cases and the
     error behaviour explicitly. This section is where handwaving is most
     expensive — "and then we handle conflicts" is not a reference-level
     explanation. -->

### Interaction with existing behaviour

### Corner cases and error behaviour

### Migration / compatibility

## Drawbacks

<!-- Mandatory. Why should we NOT do this? Cost, complexity added, capability
     given up, maintenance burden, precedent set. A proposal with no drawbacks
     has not been examined. If you struggle here, ask the strongest critic you
     know and write down what they said. -->

## Rationale and alternatives

<!-- Why this design over the others, what the impact of doing nothing is, and
     what the alternatives were. Each alternative gets the trade-off that lost —
     not a dismissal. -->

### Why this design

### Impact of doing nothing

### Alternative: <name>

**Why not:**

## Prior art

<!-- How other systems, languages, or teams solved this, and what happened to
     them. Include failures — knowing an approach was tried and abandoned is
     often the most useful content in the whole document. Say plainly if there is
     no relevant prior art rather than leaving the section empty. -->

## Unresolved questions

<!-- What is deliberately left open, what must be answered before this can be
     accepted, and what can be settled during implementation. Label which is
     which — an unresolved blocking question hidden in a list gets missed. -->

| Question | Blocking? | Owner |
|----------|-----------|-------|
| | | |

## Future possibilities

<!-- Natural extensions that are out of scope now. This is a place to park good
     ideas so they do not expand the current proposal. Keep it speculative and
     clearly non-committal. -->

## Options and determination

<!-- The lifecycle discipline: list the options that were genuinely on the table,
     then record the determination — what was chosen, by whom, on what date, and
     the reason given. An RFC that ends without a determination stays open
     forever and the same debate returns next quarter. If the determination is
     "no change", record that too.

     Where the determination is an architecture decision that other work must
     obey, also run `adb adr new` and cross-reference the ADR id below so it is
     discoverable outside this document. -->

| Option | Kept or rejected | Reason |
|--------|------------------|--------|
| | | |

**Determination:**
**Made by:**
**Date:**
**Recorded as ADR:** <!-- adb adr new -->

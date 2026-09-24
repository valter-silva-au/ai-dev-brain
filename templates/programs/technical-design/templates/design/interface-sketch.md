---
title: Interface Sketch
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: 'Design Docs at Google' (Malte Ubl, industrialempathy.com) — the guidance to sketch interfaces rather than paste formal definitions or generated schemas into a design document."
---

# Interface Sketch

<!-- WHAT THIS IS, AND WHAT IT IS DELIBERATELY NOT
     A sketch: just enough of each interface to let a reviewer judge whether the
     shape is right and whether it will be pleasant to call. It is NOT a schema
     dump, not generated IDL, and not the source of truth for wire format — those
     live with the code (OpenAPI, protobuf, type definitions) where they can be
     validated and versioned.

     If you catch yourself pasting a generated definition, stop: the formal
     artifact is the source of truth and this file should link to it instead.
     If there is nothing to review — the interface is dictated by an existing
     contract — you do not need this file. -->

## Scope of this sketch

<!-- Which interfaces are covered, and which are explicitly out of scope. Name
     the design doc section this elaborates. -->

## Consumers

<!-- Who calls each interface and what they are trying to do. An interface with
     no named consumer is speculative — say so plainly rather than designing for
     an imaginary caller. -->

| Interface | Consumer | What they are trying to do |
|-----------|----------|----------------------------|
| | | |

## Operation sketches

<!-- One subsection per operation. Keep each to a handful of lines: the intent,
     the essential inputs and outputs in plain terms or pseudo-signature form,
     and the errors a caller must handle. Field-by-field enumeration is a sign
     you have drifted into schema territory. -->

### <operation name>

- **Intent:**
- **Inputs (essential only):**
- **Returns:**
- **Errors a caller must handle:**
- **Idempotent:** <!-- yes / no, and what the idempotency key is if yes -->

### <operation name>

- **Intent:**
- **Inputs (essential only):**
- **Returns:**
- **Errors a caller must handle:**
- **Idempotent:**

## Core shapes

<!-- The two or three data shapes that appear across operations, described by
     what they mean rather than by every field. Note which fields are identity,
     which are mutable, and which are optional-and-why. Optional with no reason
     given is a weak answer — optionality is a contract. -->

### <shape name>

| Field (only the load-bearing ones) | Meaning | Notes |
|------------------------------------|---------|-------|
| | | |

## Naming and consistency notes

<!-- Where these names follow existing conventions in the system, and where they
     deliberately break from them. Inconsistent naming across a surface is the
     defect reviewers catch most easily and cheaply at sketch stage. -->

## Error and status conventions

<!-- The error model callers see: categories, whether errors are retryable, and
     what carries a machine-readable code versus human text. Do not enumerate
     every error — state the convention so the rest is predictable. -->

## Compatibility and evolution

<!-- How this surface changes later without breaking callers: what is additive,
     what would be breaking, versioning posture, and any deprecation already
     anticipated. -->

## Open questions for reviewers

<!-- Be specific about what you want challenged. "Any feedback welcome" gets
     none. Point at the two or three choices you are least sure of. -->

| Question | Who should answer |
|----------|-------------------|
| | |

## Where the formal definition lives

<!-- Link the authoritative schema / IDL / type definitions once they exist, so
     nobody treats this sketch as the contract. If a naming or shape decision
     here was contested, record it with `adb adr new` and link the ADR. -->

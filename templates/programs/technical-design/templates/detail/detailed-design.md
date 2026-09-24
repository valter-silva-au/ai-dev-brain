---
title: Detailed Design
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: the C4 model (Simon Brown, CC-BY-4.0) component and code levels; the Rust RFC template's reference-level explanation (rust-lang/rfcs, MIT/Apache-2.0) for the expected depth of technical precision."
---

# Detailed Design

<!-- WHEN NOT TO WRITE THIS DOC
     This artifact only earns its keep when the detail is genuinely contested or
     genuinely hard: a non-obvious algorithm, a concurrency or consistency
     problem, a schema many teams will depend on, a migration that cannot be
     re-run. If it would simply be an implementation manual — a prose restatement
     of code that has no trade-offs in it — do not write it. Write the program
     instead; the code and its tests are a better specification of themselves.

     Everything here must be consistent with the high-level design. If while
     writing this you discover the high-level design is wrong, stop and fix that
     document rather than quietly diverging. -->

## Scope

<!-- Which parts of the high-level design this elaborates, and which are
     deliberately left to implementation judgement. Being explicit about the
     second list is what stops this document from trying to be the code. -->

**Elaborated here:**

**Left to implementation:**

## Components (C4 level 3)

<!-- Inside each container from the high-level design: the components, their
     single responsibility, and their dependencies. A component whose
     responsibility needs "and" to describe it is probably two components. -->

```
<!-- component diagram for the container(s) in scope -->
```

| Component | Responsibility | Depends on | Owns which data |
|-----------|----------------|------------|-----------------|
| | | | |

## Code-level detail (C4 level 4)

<!-- Only for the parts that need it: class or module structure, the key
     abstractions and why they exist, extension points, and the invariants each
     structure must maintain (invariants are the detail most often lost between
     design and code). Do not mirror the whole codebase — sketch only the
     structures whose shape is a decision. A weak answer is a class list with no
     rationale. -->

### <component name>

```
<!-- class or module diagram -->
```

**Invariants:**

## Key sequences

<!-- One sequence per flow that is not obvious: the happy path plus at least one
     failure path. Show ordering, who waits on whom, and where state is
     committed. Concurrency, idempotency, and retry behaviour belong here — a
     sequence diagram with no failure branch hides exactly the case that will
     page someone. -->

### <flow name> — happy path

```
<!-- sequence diagram -->
```

### <flow name> — failure path

```
<!-- sequence diagram: what happens when the dependency times out / returns an
     error / the process dies mid-flow -->
```

## Data schemas

<!-- Concrete schemas for data this design owns: tables, collections, events,
     cached shapes. Include keys, indexes, nullability, and constraints. State
     the migration for each change to an existing schema and whether it is
     reversible. "Add a column" without stating backfill and default behaviour is
     incomplete. -->

### <schema name>

| Field | Type | Nullable | Constraint / index | Notes |
|-------|------|----------|--------------------|-------|
| | | | | |

**Migration:**
**Reversible:** <!-- yes / no — and if no, what the forward fix is -->

## API contracts

<!-- Precise contracts for the operations introduced or changed: request and
     response shapes, status/error codes, validation rules, authentication and
     authorisation required, idempotency semantics, pagination, and rate limits.
     Where a generated definition exists (OpenAPI, protobuf, types), link it and
     do not duplicate it — the generated artifact is the source of truth. -->

| Operation | Auth required | Idempotent | Errors returned | Formal definition |
|-----------|---------------|------------|-----------------|-------------------|
| | | | | |

### Validation rules

<!-- What is rejected and with what error. Be specific about boundaries: length
     limits, allowed values, size caps. -->

## Error handling

<!-- The error taxonomy for this component: which failures are retryable, retry
     bounds and backoff, timeouts per dependency, circuit-breaking, what is
     surfaced to the caller versus swallowed, and what state is left behind after
     a partial failure. Partial-failure cleanup is the part most often left
     undefined — say what happens to half-written state. -->

| Failure | Classification | Retry policy | What the caller sees | State left behind |
|---------|----------------|--------------|----------------------|-------------------|
| | | | | |

## Logging and monitoring

<!-- Logging: what is emitted, at what level, with which correlation identifiers,
     retention, and what must NEVER appear in a log line (credentials, tokens,
     personal data, full request bodies). Naming the forbidden fields is the part
     that keeps sensitive data out of log storage.
     Monitoring: concrete metric names, types, and labels, plus alert conditions
     with thresholds and the reason each threshold was chosen — an alert nobody
     can justify gets muted within a month. -->

| Event | Level | Fields included | Must never include |
|-------|-------|-----------------|--------------------|
| | | | |

| Metric | Type | Labels | Alert condition | Why this threshold |
|--------|------|--------|-----------------|--------------------|
| | | | | |

## Performance and capacity

<!-- Expected request volume and data growth, the hot paths, cost per operation,
     and where the design hits a limit first. Mark each number as measured or
     estimated — mixing the two silently is how capacity surprises happen. -->

## Testing

<!-- Per component: what unit tests cover, what needs integration or contract
     tests, the fixtures or test doubles required, and which of the failure paths
     above are actually exercised. Name any failure path you cannot test. -->

| Behaviour | Test level | Notes |
|-----------|------------|-------|
| | | |

## Traceability

<!-- Every requirement from the PRD and every element of the high-level design
     must land somewhere here, or be explicitly deferred. Gaps in this table are
     the point of the table. -->

| Requirement / design element | Where satisfied in this document | Status |
|------------------------------|----------------------------------|--------|
| FR-001 | | |
| NFR-001 | | |
| <high-level design section> | | |

## Divergences from the high-level design

<!-- Anything that had to change once the detail was worked out, and why. If this
     section is non-empty, the high-level design needs updating too — note whether
     that has been done. -->

| Divergence | Reason | High-level design updated? |
|------------|--------|----------------------------|
| | | |

## Decisions

<!-- Implementation decisions with lasting consequence go in the ADR log rather
     than being buried here: run `adb adr new` and link the id. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |

## Open questions

| Question | Blocks | Owner |
|----------|--------|-------|
| | | |

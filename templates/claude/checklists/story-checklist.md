# Story Quality Gate Checklist

**Gate:** Stories → Implementation
**Purpose:** Validate that stories meet INVEST criteria and have clear acceptance criteria before implementation begins.
**Certifying Agent:** scrum-master or product-owner

---

## INVEST Criteria (per story)

- [ ] **Independent**: Story can be implemented without depending on other incomplete stories
- [ ] **Negotiable**: Story describes the what/why, not the exact how (room for implementation decisions)
- [ ] **Valuable**: Story delivers clear value to a user or stakeholder
- [ ] **Estimable**: Story is understood well enough to estimate effort
- [ ] **Small**: Story can be completed in a single sprint/iteration
- [ ] **Testable**: Story has concrete conditions that prove it is done

## Acceptance Criteria

- [ ] Every story has at least one acceptance criterion
- [ ] Acceptance criteria use Given/When/Then format or equivalent structured format
- [ ] Happy path scenario is covered
- [ ] Error/edge case scenarios are covered
- [ ] Acceptance criteria are specific enough to write tests from
- [ ] No ambiguous language ("appropriate", "reasonable", "as needed")

## Requirement Coverage

- [ ] Every functional requirement in the PRD maps to at least one story
- [ ] No stories exist without a corresponding requirement (no scope creep)
- [ ] Epic groupings logically organize related stories
- [ ] Story dependencies are identified and sequenced

## Implementation Readiness

- [ ] Technical approach is clear from the architecture doc
- [ ] Required APIs, data models, and interfaces are defined
- [ ] External dependencies are available or have a mock/stub plan
- [ ] Test data requirements are identified

## Definition of Done

- [ ] Code is written and reviewed
- [ ] All acceptance criteria pass
- [ ] Unit tests cover new logic
- [ ] Integration tests cover critical paths
- [ ] Documentation is updated (if applicable)
- [ ] No regressions in existing tests

---

## Certification

| Field | Value |
|-------|-------|
| Checklist run date | |
| Task ID | |
| Certifying agent | |
| Result | PASS / FAIL |
| Items passed | /22 |
| Notes | |

# PRD Quality Gate Checklist

**Gate:** Requirements → Architecture
**Purpose:** Validate PRD completeness and measurability before proceeding to architecture design.
**Certifying Agent:** product-owner or design-reviewer

---

## Problem & Vision

- [ ] Problem statement is clearly defined with measurable impact
- [ ] Target users/personas are identified with specific characteristics
- [ ] Success metrics are quantifiable (not vague aspirations)
- [ ] Business value or user value is articulated

## Functional Requirements

- [ ] Each requirement has a unique identifier (FR-001, FR-002, etc.)
- [ ] Requirements use testable language ("The system shall..." not "The system should try to...")
- [ ] Requirements are atomic (one requirement per item, not compound)
- [ ] Happy path and error cases are both addressed
- [ ] User-facing requirements include acceptance criteria
- [ ] No implementation details leaked into requirements (what, not how)

## Non-Functional Requirements

- [ ] Performance requirements specified with concrete thresholds (response time, throughput)
- [ ] Security requirements identified (authentication, authorization, data protection)
- [ ] Scalability requirements stated (expected load, growth projections)
- [ ] Reliability requirements defined (uptime, recovery time)
- [ ] Compatibility/integration requirements listed

## Scope & Boundaries

- [ ] In-scope items are explicitly listed
- [ ] Out-of-scope items are explicitly listed (prevents scope creep)
- [ ] Dependencies on external systems or teams are identified
- [ ] Assumptions are documented

## Traceability

- [ ] Each requirement traces to a user need or business goal
- [ ] Requirements are prioritized (must-have vs. nice-to-have)
- [ ] No orphan requirements (requirements without a purpose)
- [ ] No gaps between user stories and requirements

## Completeness

- [ ] All user personas have at least one associated requirement
- [ ] Edge cases and error scenarios are addressed
- [ ] Data requirements (input/output formats, validation rules) are specified
- [ ] Migration or backward compatibility needs are addressed (if applicable)

---

## Certification

| Field | Value |
|-------|-------|
| Checklist run date | |
| Task ID | |
| Certifying agent | |
| Result | PASS / FAIL |
| Items passed | /24 |
| Notes | |

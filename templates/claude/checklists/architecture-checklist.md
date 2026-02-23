# Architecture Quality Gate Checklist

**Gate:** Architecture → Stories
**Purpose:** Validate architecture soundness, security posture, and alignment with PRD before story decomposition.
**Certifying Agent:** design-reviewer

---

## PRD Alignment

- [ ] Every functional requirement in the PRD has a corresponding architectural component
- [ ] Non-functional requirements are addressed by specific architectural decisions
- [ ] No architectural components exist without a corresponding requirement (no gold-plating)
- [ ] Scope matches PRD scope (not broader or narrower)

## System Design

- [ ] System context diagram shows external dependencies and integrations
- [ ] Component boundaries are clearly defined with explicit responsibilities
- [ ] Data flow between components is documented
- [ ] API contracts (endpoints, request/response formats) are specified
- [ ] Data model is defined with relationships and constraints

## Key Decisions

- [ ] Each significant decision is recorded with rationale (ADR format preferred)
- [ ] Alternatives considered are documented with trade-off analysis
- [ ] Decisions are traceable to requirements they satisfy
- [ ] Technology choices are justified (not assumed)

## Scalability & Performance

- [ ] Architecture can handle the load specified in NFRs
- [ ] Bottlenecks are identified with mitigation strategies
- [ ] Caching strategy is defined (if applicable)
- [ ] Database indexing and query patterns are considered

## Security

- [ ] Authentication mechanism is specified
- [ ] Authorization model is defined (roles, permissions)
- [ ] Data protection at rest and in transit is addressed
- [ ] Input validation boundaries are identified
- [ ] Secrets management approach is documented
- [ ] OWASP Top 10 risks are considered

## Reliability & Operations

- [ ] Failure modes are identified with recovery strategies
- [ ] Monitoring and observability approach is defined
- [ ] Deployment strategy is specified (zero-downtime, rollback plan)
- [ ] Logging strategy covers audit and debugging needs

## Maintainability

- [ ] Component coupling is minimized (changes don't cascade)
- [ ] Extension points are identified for likely future changes
- [ ] Testing strategy covers unit, integration, and end-to-end layers
- [ ] Code organization and package structure is defined

---

## Certification

| Field | Value |
|-------|-------|
| Checklist run date | |
| Task ID | |
| Certifying agent | |
| Result | PASS / FAIL |
| Items passed | /25 |
| Notes | |

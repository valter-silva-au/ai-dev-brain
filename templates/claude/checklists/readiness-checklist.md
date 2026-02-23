# Implementation Readiness Checklist

**Gate:** Stories → Implementation (cross-artifact validation)
**Purpose:** Verify alignment across PRD, architecture, and stories before writing code. Catches gaps and contradictions that single-artifact checklists miss.
**Certifying Agent:** design-reviewer

---

## Cross-Artifact Alignment

- [ ] PRD requirements → Architecture components: every requirement has an implementation path
- [ ] Architecture components → Stories: every component change is covered by a story
- [ ] Stories → PRD requirements: every story traces back to a requirement
- [ ] No circular dependencies between stories that would block implementation
- [ ] Story sequencing respects architectural dependencies (foundations first)

## Data Model Consistency

- [ ] Data entities in PRD match entities in architecture doc
- [ ] Data entities in architecture match fields referenced in story acceptance criteria
- [ ] Data validation rules in PRD are reflected in architecture and stories
- [ ] Migration strategy exists for schema changes (if applicable)

## API Contract Consistency

- [ ] API endpoints in architecture match the operations described in stories
- [ ] Request/response formats are consistent across architecture and stories
- [ ] Error codes and error handling are consistent
- [ ] Authentication/authorization requirements align across all artifacts

## Non-Functional Alignment

- [ ] Performance requirements in PRD have architectural support (caching, indexing, etc.)
- [ ] Security requirements in PRD have architectural components and story coverage
- [ ] Scalability requirements have both architectural design and test stories
- [ ] Monitoring requirements have corresponding implementation stories

## Gap Analysis

- [ ] No orphan PRD requirements (requirements not covered by architecture + stories)
- [ ] No orphan architecture components (components not driven by requirements)
- [ ] No orphan stories (stories not traceable to requirements)
- [ ] Edge cases identified in any artifact are covered in stories
- [ ] Error handling patterns are consistent across all artifacts

## Team Readiness

- [ ] Development environment setup is documented or automated
- [ ] External service access (APIs, databases, credentials) is available
- [ ] Test environment mirrors production architecture sufficiently
- [ ] Code review process is agreed upon

---

## Certification

| Field | Value |
|-------|-------|
| Checklist run date | |
| Task ID | |
| Certifying agent | |
| Result | PASS / FAIL |
| Items passed | /23 |
| Notes | |

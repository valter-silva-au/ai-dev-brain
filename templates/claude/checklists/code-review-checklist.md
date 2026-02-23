# Code Review Quality Gate Checklist

**Gate:** Implementation → Review/Done
**Purpose:** Validate implementation quality, test coverage, and acceptance criteria satisfaction before marking a task complete.
**Certifying Agent:** code-reviewer

---

## Acceptance Criteria

- [ ] Every acceptance criterion from the story is satisfied
- [ ] Acceptance criteria have corresponding test cases
- [ ] Edge cases identified in the story are handled
- [ ] No acceptance criteria are partially implemented

## Code Quality

- [ ] Code follows project conventions and style guide
- [ ] No dead code, commented-out code, or TODO/FIXME without a tracking reference
- [ ] Functions and methods have clear single responsibilities
- [ ] Variable and function names are descriptive and consistent
- [ ] No unnecessary complexity (premature abstraction, over-engineering)
- [ ] Error handling is consistent and follows project patterns

## Security

- [ ] No hardcoded secrets, credentials, or API keys
- [ ] User input is validated at system boundaries
- [ ] No SQL injection, XSS, or command injection vulnerabilities
- [ ] Authentication and authorization checks are in place where required
- [ ] Sensitive data is not logged or exposed in error messages

## Testing

- [ ] Unit tests cover new/changed logic
- [ ] Integration tests cover critical paths
- [ ] Tests are deterministic (no flaky tests introduced)
- [ ] Test assertions are specific (not just "no error")
- [ ] Edge cases and error paths have test coverage
- [ ] Tests can run independently (no shared state between tests)

## Performance

- [ ] No obvious performance regressions (N+1 queries, unbounded loops, excessive allocations)
- [ ] Resource cleanup is handled (connections closed, files closed, goroutines terminated)
- [ ] Caching is used where architecture specified

## Architecture Compliance

- [ ] Implementation follows the architecture document's component boundaries
- [ ] API contracts match the architecture specification
- [ ] Data model matches the architecture specification
- [ ] No import cycles or inappropriate cross-package dependencies

## Documentation

- [ ] Public APIs have adequate documentation
- [ ] Complex logic has explanatory comments
- [ ] Architecture Decision Records are updated if implementation diverged from plan
- [ ] README or user-facing docs updated if behavior changed

---

## Certification

| Field | Value |
|-------|-------|
| Checklist run date | |
| Task ID | |
| Certifying agent | |
| Result | PASS / FAIL |
| Items passed | /28 |
| Notes | |

---
name: adversarial-review
description: Self-review uncommitted changes with information asymmetry to catch issues the implementer is blind to
allowed-tools: Bash, Read, Grep, Glob
---

# Adversarial Review Skill

Perform a hostile code review of recent changes using information asymmetry -- review only the diff without implementation context to catch issues the implementer is blind to.

## Steps

### 1. Construct the Diff

Determine the baseline and build the diff:

**If on a feature branch:**
```bash
git merge-base HEAD main
```
Use the merge-base as baseline, then:
```bash
git diff $(git merge-base HEAD main)
```

**If changes are uncommitted:**
```bash
git diff HEAD
```
Also check for new untracked files:
```bash
git status --porcelain
```
For each new file, include its full content in the review scope.

**If a specific baseline is provided as argument:**
```bash
git diff {baseline}
```

### 2. Review with Hostile Intent

Review the diff as if you are an adversarial reviewer who has:
- **No implementation context** -- Only the diff, not the design rationale
- **No sympathy** -- Every line is suspect until proven correct
- **Domain expertise** -- Full knowledge of Go patterns, security, and adb conventions

Check each category systematically:

#### Logic Errors
- Off-by-one errors in loops or slices
- Nil pointer dereferences (especially on interface values and map lookups)
- Race conditions on shared state
- Incorrect boolean logic or edge case handling
- Resource leaks (unclosed files, channels, connections)

#### Security Issues
- Command injection via `os/exec` (check all user-controlled arguments)
- Path traversal in file operations
- Credential or secret exposure in code or logs
- Missing input validation at system boundaries
- Insecure file permissions

#### Error Handling
- Errors returned but not checked
- Missing `fmt.Errorf("context: %w", err)` wrapping
- Silent error swallowing without justification
- Error messages that expose internal details to users

#### Test Coverage Gaps
- New functions without corresponding tests
- Missing edge case tests (empty input, nil, boundary values)
- Missing error path tests
- Property test candidates (invariants that should hold for all inputs)
- Tests that don't use `t.TempDir()` for file isolation

#### Project Pattern Violations
- Imports from `core/` to `storage/` or `integration/` (import cycle risk)
- Constructors returning concrete types instead of interfaces
- Missing `yaml` struct tags on persistent fields
- Timestamps not using `time.Now().UTC()`
- File permissions not using `0o644`/`0o755`

#### Interface Contract Violations
- Changed method signatures that affect callers
- New dependencies that break the layered architecture
- Removed or renamed exports that other packages depend on

### 3. Rate and Order Findings

For each finding, assign a severity:

| Severity | Criteria |
|----------|---------|
| **Critical** | Data loss, security vulnerability, import cycle, build break |
| **High** | Logic error, unchecked error, race condition, missing test for critical path |
| **Medium** | Pattern violation, missing edge case test, suboptimal error message |
| **Low** | Style issue, minor optimization opportunity, documentation gap |

Order findings by severity (Critical first).
Number them sequentially (F1, F2, F3, ...).

### 4. Assess Validity

For each finding, assess:
- **Real** -- Genuine issue that should be fixed
- **Noise** -- False positive from limited context
- **Undecided** -- Needs implementation context to determine

Do NOT filter out findings based on validity. Present all findings and let the implementer decide.

### 5. Present Report

```
=== Adversarial Review ===

Diff scope: [N] files changed, [N] insertions, [N] deletions
Baseline: [commit hash or description]

## Findings ([N] total)

### Critical ([N])

[F1] [Title]
     File: [path:line]
     Evidence: [What the code does wrong]
     Validity: Real/Noise/Undecided
     Fix: [Recommended action]

### High ([N])

[F2] [Title]
     ...

### Medium ([N])

[F3] [Title]
     ...

### Low ([N])

[F4] [Title]
     ...

## Summary

| Severity | Count | Real | Noise | Undecided |
|----------|-------|------|-------|-----------|
| Critical | | | | |
| High | | | | |
| Medium | | | | |
| Low | | | | |

Recommendation: [Fix All / Fix Critical+High / Approve with Notes]
```

### 6. Zero-Finding Check

If the review produces zero findings, this is suspicious. Re-examine:
- Is the diff actually loaded and complete?
- Are you being too lenient?
- Are there implicit issues (missing tests, missing error handling)?

A real codebase change almost always has at least one finding. State explicitly if the diff is genuinely clean and why.

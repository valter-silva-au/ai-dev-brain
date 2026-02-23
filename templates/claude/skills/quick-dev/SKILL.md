---
name: quick-dev
description: Implement from a tech spec or direct instructions with self-checking and adversarial review
argument-hint: "<task-id or tech-spec path>"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
---

# Quick-Dev Skill

Implement a task from a tech spec or direct instructions, with built-in self-checking and adversarial review.

## Steps

### 1. Capture Baseline

Record the current git state for later diff construction:

```bash
git rev-parse HEAD
```

Save this as the baseline commit hash.

### 2. Determine Execution Mode

**Mode A: Tech Spec** (if a task ID or spec path is provided)
- If task ID: read `tickets/{taskID}/tech-spec.md`
- If path: read the specified tech spec file
- Verify the spec meets the ready-for-development standard (no TBDs, all tasks have file paths)

**Mode B: Direct** (if instructions are given directly)
- Clarify the scope: what to implement, what to leave alone
- Identify affected files and patterns before starting

### 3. Load Context

Regardless of mode:
- Read all files referenced in the spec or relevant to the task
- Understand the test patterns in adjacent test files
- Check `docs/decisions/` for applicable ADRs
- Load project patterns from `docs/architecture.md`

### 4. Implement

Execute the implementation tasks in order:

- Follow the spec's task sequence (if Mode A)
- Match existing code patterns exactly
- Write tests alongside implementation, not after
- After each logical unit of work, run:
  ```bash
  go build ./...
  go test ./... -count=1
  ```
- If a test fails, fix it before moving on
- Follow Go conventions: error wrapping, local interfaces, constructors return interfaces

### 5. Self-Check

Before proceeding to adversarial review, verify:

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

If any check fails, fix the issue and re-run.

Additionally verify:
- [ ] All acceptance criteria from the spec are satisfied
- [ ] All new code follows project patterns (check `docs/architecture.md`)
- [ ] Error messages start lowercase and describe the operation
- [ ] File permissions are correct (0o755 for directories, 0o644 for files)
- [ ] No hardcoded secrets or credentials

### 6. Adversarial Review

Construct the diff from baseline:

```bash
git diff {baseline_commit}
```

Also identify any new untracked files created during implementation.

Review the diff as a hostile reviewer with NO implementation context. Look for:

| Category | What to Check |
|----------|--------------|
| **Logic** | Off-by-one errors, nil pointer dereferences, race conditions |
| **Security** | Injection, path traversal, credential exposure, OWASP top 10 |
| **Error handling** | Unchecked errors, missing context wrapping, silent failures |
| **Test gaps** | Missing edge cases, untested error paths, missing property tests |
| **Pattern violations** | Divergence from project conventions, import cycle risks |
| **Interface contracts** | Changes that affect callers, broken backward compatibility |

Rate each finding: **Critical** / **High** / **Medium** / **Low**

Present findings as:

```
=== Adversarial Review Findings ===

[F1] CRITICAL: [Description]
     File: [path:line]
     Fix: [What to do]

[F2] HIGH: [Description]
     File: [path:line]
     Fix: [What to do]

[F3] MEDIUM: [Description]
     ...
```

### 7. Resolve Findings

- Fix all **Critical** and **High** findings immediately
- Re-run tests after fixes
- Document any **Medium** or **Low** findings not fixed as known issues
- If zero findings were found in step 6, re-analyze -- this is suspicious

### 8. Report

Print a summary:

```
=== Quick Dev Complete ===

Implementation:
  Files modified: [N]
  Files created: [N]
  Tests added: [N]

Self-Check:
  Build: PASS
  Tests: PASS ([N] total, [N] new)
  Vet: PASS

Adversarial Review:
  Findings: [N] total ([N] critical, [N] high, [N] medium, [N] low)
  Resolved: [N]
  Remaining: [N] (documented as known issues)

Status: Ready for code review
```

Update the task's `context.md` with implementation progress and decisions made.

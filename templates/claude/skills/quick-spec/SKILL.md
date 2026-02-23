---
name: quick-spec
description: Create an implementation-ready tech spec through code investigation and structured documentation
argument-hint: "<task-id or description>"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
---

# Quick-Spec Skill

Produce an implementation-ready technical specification by investigating the codebase, understanding the request, and documenting everything a developer agent needs to implement the change.

## Ready-for-Development Standard

A spec is ready ONLY if it meets ALL of the following:

- **Actionable**: Every task has a clear file path and specific action
- **Logical**: Tasks are ordered by dependency (lowest level first)
- **Testable**: All ACs follow Given/When/Then and cover happy path and edge cases
- **Complete**: All investigation results are inlined; no placeholders or "TBD"
- **Self-Contained**: A fresh agent can implement the feature without reading the workflow history

## Steps

### 1. Understand the Request

Identify what is being asked:
- If a task ID is given, read `tickets/{taskID}/context.md`, `notes.md`, and `design.md`
- If a description is given, clarify scope: what is in, what is out
- Ask clarifying questions if the request is ambiguous

### 2. Investigate the Codebase

Before writing anything, understand the implementation landscape:

- **Find relevant files** -- Use Glob and Grep to locate code that will be affected
- **Read existing patterns** -- How do adjacent files handle similar concerns?
- **Check architecture** -- Read `docs/architecture.md` for structural constraints
- **Check ADRs** -- Read `docs/decisions/` for past decisions that apply
- **Identify test patterns** -- How are tests structured in the affected packages?
- **Map dependencies** -- What other code depends on the files being changed?

### 3. Generate the Tech Spec

Write the spec to the task's ticket directory or current directory:

- If task ID available: `tickets/{taskID}/tech-spec.md`
- Otherwise: `./tech-spec.md`

Use the template at `templates/bmad/tech-spec.md` as a starting point, but fill in ALL sections with concrete, investigated information. Every field must contain actual content from your investigation, not placeholders.

The spec must include:

1. **Problem Statement** -- What needs to change and why
2. **Solution** -- How we'll solve it
3. **Scope** -- In scope and out of scope boundaries
4. **Codebase Patterns** -- Actual patterns from the code that must be followed
5. **Files to Reference** -- Table of files with their purpose (from your investigation)
6. **Technical Decisions** -- Key choices with rationale
7. **Implementation Tasks** -- Ordered list with specific file paths and actions
8. **Acceptance Criteria** -- Given/When/Then for each behavior
9. **Testing Strategy** -- What tests to write, which patterns to match

### 4. Self-Review

Before presenting the spec, validate:

- [ ] Every implementation task references a specific file path
- [ ] Tasks are ordered by dependency (data models -> interfaces -> implementations -> tests)
- [ ] Every AC is testable with Given/When/Then
- [ ] No placeholders, TODOs, or "TBD" remain
- [ ] The spec is self-contained (a fresh agent can implement from it alone)
- [ ] The spec doesn't contradict existing ADRs in `docs/decisions/`

### 5. Present to User

Print a summary:

```
=== Tech Spec Created ===
File: [path to tech-spec.md]
Tasks: [N] implementation steps
ACs: [N] acceptance criteria
Files affected: [N] files

Ready for implementation with /quick-dev or the quick-flow-dev agent.
```

## Transition to Implementation

After the spec is approved, it can be executed via:
- The `/quick-dev` skill (for inline execution)
- The `quick-flow-dev` agent (for delegated execution)

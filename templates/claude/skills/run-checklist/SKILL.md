---
name: run-checklist
description: Run a named quality gate checklist against a task's artifacts
argument-hint: "<checklist-name> [task-id]"
allowed-tools: Read, Write, Grep, Glob, Bash
---

# Run Checklist

Run a named quality gate checklist against a task's artifacts. Checklists validate that artifacts meet quality standards before progressing to the next phase.

## Available Checklists

| Name | Gate | What It Validates |
|------|------|-------------------|
| `prd` | Requirements → Architecture | PRD completeness, measurability, traceability |
| `architecture` | Architecture → Stories | Architecture soundness, security, PRD alignment |
| `story` | Stories → Implementation | INVEST criteria, acceptance criteria clarity |
| `readiness` | Stories → Implementation | Cross-artifact alignment (PRD + arch + stories) |
| `code-review` | Implementation → Done | Code quality, test coverage, AC satisfaction |

## Steps

1. **Parse arguments**: First argument is the checklist name (required). Second argument is the task ID (optional, falls back to current task context or ADB_TASK_ID).

2. **Load the checklist**: Read the corresponding checklist template:
   - `prd` → `prd-checklist.md`
   - `architecture` → `architecture-checklist.md`
   - `story` → `story-checklist.md`
   - `readiness` → `readiness-checklist.md`
   - `code-review` → `code-review-checklist.md`

3. **Load task artifacts**: Read the relevant artifacts for the checklist:
   - PRD checklist: `prd.md`
   - Architecture checklist: `architecture-doc.md`, `prd.md`
   - Story checklist: `epics.md`, `prd.md`
   - Readiness checklist: `prd.md`, `architecture-doc.md`, `epics.md`
   - Code review checklist: git diff, test results, `epics.md`

4. **Evaluate each item**: Go through every checklist item and determine:
   - PASS: The artifact satisfies this criterion
   - FAIL: The artifact does not satisfy this criterion (explain why)
   - N/A: Not applicable for this task

5. **Report results**: Present findings:
   - Each item with PASS/FAIL/N/A status
   - For FAIL items: specific explanation and suggestion for resolution
   - Overall score: X/Y items passed
   - Verdict: CERTIFIED (all critical items pass) or NOT CERTIFIED

6. **Save results**: Write the completed checklist to the task's directory.

## Output

- Completed checklist with PASS/FAIL per item
- Explanation for each FAIL item
- Overall verdict: CERTIFIED or NOT CERTIFIED
- Saved to task's ticket directory

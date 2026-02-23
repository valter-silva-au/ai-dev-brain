---
name: check-readiness
description: Cross-artifact validation to verify implementation readiness
argument-hint: "<task-id>"
allowed-tools: Read, Grep, Glob, Bash
---

# Check Implementation Readiness

Validate alignment across all planning artifacts (PRD, architecture, stories) before implementation begins. This catches gaps and contradictions that single-artifact reviews miss.

## Steps

1. **Load all artifacts**: Read the task's planning artifacts:
   - `prd.md` — requirements and success metrics
   - `architecture-doc.md` — components, decisions, data model
   - `epics.md` — stories with acceptance criteria
   - Any existing checklist results in `checklists/`

2. **Cross-artifact alignment check**:
   - **PRD → Architecture**: Every requirement has an implementation path in the architecture
   - **Architecture → Stories**: Every component change is covered by a story
   - **Stories → PRD**: Every story traces back to a requirement
   - **Data model consistency**: Entities match across PRD, architecture, and stories
   - **API contract consistency**: Endpoints match across architecture and stories

3. **Run the readiness checklist**: Evaluate each item in the readiness checklist:
   - Cross-artifact alignment (5 items)
   - Data model consistency (4 items)
   - API contract consistency (4 items)
   - Non-functional alignment (4 items)
   - Gap analysis (5 items)
   - Team readiness (3 items)

4. **Report findings**: For each issue found:
   - Describe the gap or contradiction
   - Identify which artifacts are affected
   - Suggest a resolution
   - Rate severity (critical/high/medium/low)

5. **Certification**: If all critical and high items pass, certify readiness. Otherwise, list blocking items that must be resolved.

## Output

- Readiness report with pass/fail per checklist item
- List of gaps, contradictions, and missing coverage
- Severity-rated findings with resolution suggestions
- Overall verdict: READY / NOT READY (with blocking items)
- If ready: "Proceed to implementation. Run `/quick-dev` for small tasks or start coding for larger ones."
- If not ready: "Resolve the blocking items above, then re-run `/check-readiness`."

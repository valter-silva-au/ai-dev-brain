---
name: create-stories
description: Decompose PRD and architecture into implementable epics and stories
argument-hint: "<task-id>"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash
---

# Create Epics and Stories

Decompose a PRD and architecture document into implementable epics and stories with INVEST criteria and Given/When/Then acceptance criteria. This is the Stories phase of the BMAD workflow.

## Steps

1. **Load all artifacts**: Read the task's planning artifacts:
   - `prd.md` — functional requirements, user personas, priorities
   - `architecture-doc.md` — components, data model, API design
   - `product-brief.md` — vision and success metrics (if available)
   - `notes.md` and `context.md` — additional context

2. **Identify epics**: Group related requirements into epics:
   - Each epic delivers a user-facing capability
   - Epics map to PRD requirement groups
   - Epics reference architecture components

3. **Decompose into stories**: For each epic, create stories that:
   - Follow INVEST criteria (Independent, Negotiable, Valuable, Estimable, Small, Testable)
   - Use "As a [persona], I want to [action], so that [benefit]" format
   - Include Given/When/Then acceptance criteria (happy path + error cases)
   - Reference relevant architecture components and decisions
   - Have estimated size (S/M/L)
   - Identify dependencies on other stories

4. **Build dependency map**: Create a story dependency graph:
   - Identify which stories must be completed before others
   - Suggest implementation order
   - Flag any circular dependencies

5. **Create traceability matrix**: Map every PRD requirement to stories:
   - Every FR-XXX must have at least one story
   - Every story must trace to a requirement (no scope creep)
   - Identify any gaps

6. **Output location**: Write to the task's ticket directory.

## Output

- An `epics.md` file with all epics, stories, acceptance criteria, and dependency map
- Traceability matrix showing requirement → story coverage
- Recommendation: "Run `/check-readiness` to validate cross-artifact alignment before implementation"

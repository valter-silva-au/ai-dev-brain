---
name: create-prd
description: Facilitated PRD creation from a product brief or requirements
argument-hint: "<task-id or brief-path>"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash
---

# Create PRD

Create a Product Requirements Document (PRD) from an existing product brief or from scratch. This is the Requirements phase of the BMAD workflow.

## Steps

1. **Load context**: Read the argument to determine the source:
   - If a task ID: read the task's `product-brief.md`, `notes.md`, and `context.md`
   - If a file path: read the specified product brief
   - If neither: ask the user for requirements

2. **Elicit requirements**: For each section, work with the user to define:
   - **Functional requirements**: What the system must do (use FR-001 numbering)
   - **Non-functional requirements**: Performance, security, reliability, scalability
   - **User personas**: Who uses this and what are their goals
   - **Success metrics**: How we measure success

3. **Structure the PRD**: Write a `prd.md` following the artifact template:
   - Overview with purpose, background, goals, and non-goals
   - User personas with roles, goals, pain points
   - Functional requirements with priority and acceptance criteria (Given/When/Then)
   - Non-functional requirements with measurable thresholds
   - User stories overview (high-level, detailed stories come later)
   - Success metrics, dependencies, assumptions

4. **Validate**: Cross-check requirements against the product brief:
   - Every brief feature maps to at least one requirement
   - Success metrics from brief are refined into measurable targets
   - Constraints from brief are reflected as NFRs or out-of-scope items

5. **Output location**: Write to the task's ticket directory if a task ID is provided, otherwise to the current directory.

## Output

- A `prd.md` file with all sections filled in
- Traceability note showing brief → PRD coverage
- Recommendation: "Run `/create-architecture` to design the technical architecture"

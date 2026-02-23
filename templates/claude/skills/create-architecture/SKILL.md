---
name: create-architecture
description: Guided architecture document creation from PRD and codebase analysis
argument-hint: "<task-id or prd-path>"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash
---

# Create Architecture Document

Create a technical architecture document from a PRD and codebase analysis. This is the Architecture phase of the BMAD workflow.

## Steps

1. **Load context**: Read the argument to determine the source:
   - If a task ID: read the task's `prd.md`, `product-brief.md`, `notes.md`, and `design.md`
   - If a file path: read the specified PRD
   - Also read the project's existing architecture docs, CLAUDE.md, and relevant source code

2. **Analyze the codebase**: Understand the current architecture:
   - Search for existing patterns, interfaces, and conventions
   - Identify components that will be affected by the new requirements
   - Review existing ADRs in `docs/decisions/` for established decisions
   - Check for potential conflicts with existing architecture

3. **Design the architecture**: For each PRD requirement, determine:
   - Which components need to change or be created
   - Key architectural decisions (record as ADRs)
   - Data model changes
   - API design (endpoints, contracts)
   - Security considerations
   - Deployment and operational impact

4. **Structure the document**: Write an `architecture-doc.md` following the artifact template:
   - System context and component overview
   - Key decisions in ADR format with alternatives and rationale
   - Component design with responsibilities and interfaces
   - Data model with entities and relationships
   - API design with endpoints and formats
   - Security, deployment, and monitoring sections
   - Testing strategy across unit/integration/E2E layers

5. **Validate against PRD**: Cross-check:
   - Every functional requirement has an architectural implementation path
   - NFRs are addressed by specific design choices
   - No over-engineering (components without corresponding requirements)

6. **Output location**: Write to the task's ticket directory if a task ID is provided, otherwise to the current directory.

## Output

- An `architecture-doc.md` file with all sections filled in
- List of key decisions made
- Recommendation: "Run `/create-stories` to decompose into implementable stories"

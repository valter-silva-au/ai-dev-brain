---
name: create-brief
description: Guided product brief creation through discovery and requirements elicitation
argument-hint: "<task-id or topic>"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash, WebFetch, WebSearch
---

# Create Product Brief

Create a product brief for a feature or product through structured discovery. This is the first step in the BMAD workflow, producing a concise vision document that feeds into PRD creation.

## Steps

1. **Understand the request**: Read the argument to determine the topic. If a task ID is provided, read the task's `notes.md` and `context.md` for existing context.

2. **Discovery questions**: Ask the user about:
   - What problem are we solving?
   - Who are the target users?
   - What does success look like?
   - What are the known constraints?
   - What alternatives exist today?

3. **Research** (if applicable): Search the codebase, documentation, or web for relevant context:
   - Existing code that relates to this feature
   - Documentation or ADRs that inform the approach
   - Similar implementations or patterns

4. **Create the brief**: Write a `product-brief.md` file using the artifact template structure:
   - Vision (2-3 sentences)
   - Problem statement with current vs. desired state
   - Target users table
   - Key features (3-5 items)
   - Success metrics with measurable targets
   - Constraints and risks
   - Open questions

5. **Output location**: Write to the task's ticket directory if a task ID is provided, otherwise to the current directory.

## Output

- A `product-brief.md` file with all sections filled in
- Summary of key findings and open questions
- Recommendation for next step: "Run `/create-prd` to create a detailed PRD from this brief"

---
name: correct-course
description: Structured change management when plans derail or requirements change
argument-hint: "<task-id>"
allowed-tools: Read, Write, Edit, Grep, Glob, Bash
---

# Correct Course

Initiate a structured course correction when implementation has diverged from the plan, requirements have changed, or blockers have been discovered. This replaces ad-hoc improvisation with a disciplined change management process.

## Steps

1. **Assess current state**: Read the task's artifacts and current progress:
   - `context.md` — current state and blockers
   - `notes.md` — original requirements
   - `prd.md`, `architecture-doc.md`, `epics.md` — planning artifacts (if they exist)
   - Recent git history to understand what's been implemented
   - Any existing session summaries in `sessions/`

2. **Identify the deviation**: Ask the user to describe what changed:
   - New requirements discovered during implementation?
   - Technical blocker that invalidates the original approach?
   - Scope change requested by stakeholders?
   - Dependency that became unavailable?
   - Performance or security issue discovered?

3. **Impact analysis**: For each change, assess:
   - Which artifacts are affected (PRD, architecture, stories)?
   - Which completed work needs to be modified or reverted?
   - Which remaining stories are still valid?
   - What new work is created by this change?
   - Impact on timeline and dependencies

4. **Propose correction options**: Present 2-3 options:
   - **Option A**: Minimal change — adapt within current architecture
   - **Option B**: Moderate change — update architecture and affected stories
   - **Option C**: Major change — revisit PRD and rearchitect

   For each option, describe: scope of changes, affected artifacts, new risks, and trade-offs.

5. **Execute correction**: After the user chooses an option:
   - Update affected artifacts (PRD, architecture, stories) to reflect the new direction
   - Update `context.md` with the course correction decision and rationale
   - Record the decision in `knowledge/decisions.yaml`
   - Identify any new stories or modified acceptance criteria
   - Update story dependencies and implementation order

6. **Output location**: Write to the task's ticket directory.

## Output

- Impact analysis report
- Updated artifacts reflecting the chosen correction
- New or modified stories (if applicable)
- Decision record documenting the course correction
- Updated implementation plan

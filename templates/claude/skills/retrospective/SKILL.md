---
name: retrospective
description: Analyze completed tasks and extract patterns, improvements, and recurring themes
allowed-tools: Read, Glob, Grep
---

# Retrospective Skill

Analyze completed and archived tasks to extract patterns, improvements, and recurring themes.

## Steps

### 1. Find Completed Tasks
Read `backlog.yaml` and identify tasks with status `done` or `archived`.

### 2. Analyze Each Task
For each completed task:
- Read `tickets/{taskID}/status.yaml` for task metadata (type, priority, dates)
- Read `tickets/{taskID}/context.md` for decisions, blockers, and progress
- Read `tickets/{taskID}/notes.md` for learnings, gotchas, and wiki updates
- Check for `tickets/{taskID}/handoff.md` for archive summaries

### 3. Extract Patterns

Look for:
- **Recurring blockers** -- Same types of issues blocking multiple tasks
- **Common gotchas** -- Mistakes or surprises that appeared in multiple tasks
- **Successful patterns** -- Approaches that worked well and should be repeated
- **Time in blocked status** -- Tasks that spent significant time blocked
- **Knowledge gaps** -- Areas where research spikes were needed

### 4. Generate Report

```
=== Retrospective Report ===
Tasks analyzed: N (M feat, X bug, Y spike, Z refactor)

-- What Went Well --
- Pattern or practice that worked effectively
- Pattern or practice that worked effectively

-- What Could Improve --
- Recurring issue or inefficiency
- Recurring issue or inefficiency

-- Recurring Blockers --
- Type of blocker (count of occurrences)

-- Key Learnings --
- Learning extracted from completed tasks

-- Action Items --
- Suggested improvement based on the analysis
```

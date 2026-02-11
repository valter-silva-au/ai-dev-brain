---
name: task-status
description: Show adb task status with context summaries
allowed-tools:
  - Bash
  - Read
  - Glob
---

Show the current state of all tasks managed by adb, with context for active work.

## Steps

1. Run `adb status` to display tasks grouped by lifecycle status
2. If there are `in_progress` tasks, read each task's `context.md` to summarize current focus
3. If there are `blocked` tasks, highlight what they are blocked on
4. Present a concise overview:
   - Active tasks with current focus
   - Blocked tasks with blocking reason
   - Backlog count and top priorities
   - Recently completed tasks

## Output Format

Summarize the workspace state concisely:
- How many tasks in each status
- What's actively being worked on
- Any blockers that need attention
- Suggested next actions

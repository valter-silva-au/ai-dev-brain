---
name: standup
description: Generate a daily standup summary from recent task activity and git commits
allowed-tools: Bash, Read, Glob, Grep
---

# Standup Skill

Generate a concise daily standup report based on recent task activity and git history.

## Steps

### 1. Gather Task Activity
Read `backlog.yaml` and identify tasks with status `in_progress`, `blocked`, or `review`.

For each active task:
- Read `tickets/{taskID}/status.yaml` for current status and priority
- Read `tickets/{taskID}/context.md` for recent progress and blockers

### 2. Gather Git Activity
Run `git log --oneline --since="1 day ago" --all` to find recent commits across all branches.

If no commits in the last day, extend to `--since="2 days ago"`.

### 3. Check for Blockers
Identify any tasks with status `blocked` and extract the blocker details from their context.md.

### 4. Generate Report

Output the standup report in this format:

```
=== Daily Standup ===
Date: YYYY-MM-DD

-- Yesterday --
- [TASK-XXXXX] Description of what was accomplished
- [TASK-XXXXX] Description of what was accomplished

-- Today --
- [TASK-XXXXX] Description of planned work (from Next Steps in context.md)
- [TASK-XXXXX] Description of planned work

-- Blockers --
- [TASK-XXXXX] Description of blocker (or "None" if no blockers)

-- Recent Commits --
- abc1234 commit message (branch-name)
- def5678 commit message (branch-name)
```

If there are no active tasks, report that and suggest checking `adb status`.

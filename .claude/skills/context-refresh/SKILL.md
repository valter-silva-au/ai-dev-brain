---
name: context-refresh
description: Update a task's context.md with latest progress from git history and current state
argument-hint: "<task-id>"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

# Context Refresh Skill

Update a task's context.md file with the latest progress based on git commits, code changes, and current state.

## Steps

### 1. Load Current Context
Read `tickets/{taskID}/context.md` and `tickets/{taskID}/status.yaml` to understand the current documented state.

### 2. Gather Recent Activity

Determine the task's branch from status.yaml, then:
- Run `git log --oneline {branch} --not main` to find all commits on the task branch
- Run `git diff --stat main...{branch}` to see changed files
- Check for any uncommitted changes in the worktree if one exists

### 3. Identify Updates Needed

Compare the current context.md content against the actual git activity:
- Are there commits not reflected in "Recent Progress"?
- Has the current focus shifted based on recent commits?
- Are there new decisions evident from the code changes?
- Have any blockers been resolved?
- Do the "Next Steps" still make sense?

### 4. Update context.md

Edit the following sections in `tickets/{taskID}/context.md`:
- **Summary** -- Update if the task scope has evolved
- **Current Focus** -- Set to what the most recent commits indicate
- **Recent Progress** -- Add entries for commits not yet documented
- **Decisions Made** -- Add any decisions evident from code changes
- **Blockers** -- Remove resolved blockers, add new ones if apparent
- **Next Steps** -- Update based on what remains to be done

### 5. Report

Summarize what was updated:
```
=== Context Refresh for {taskID} ===

Branch: {branch}
Commits since last update: N
Files changed: M

Updated sections:
- Recent Progress: added N entries
- Current Focus: updated to "..."
- Blockers: removed N, added M
- Next Steps: updated N items
```

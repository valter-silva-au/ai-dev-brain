---
name: knowledge-extract
description: Extract learnings, decisions, and gotchas from the current task into organizational knowledge
argument-hint: "<task-id>"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
---

# Knowledge Extract Skill

Extract learnings, decisions, and gotchas from a task and persist them into organizational knowledge stores.

## Steps

### 1. Load Task Context
Given the task ID argument:
- Read `tickets/{taskID}/context.md` for decisions made and progress
- Read `tickets/{taskID}/notes.md` for learnings, gotchas, wiki updates, and runbook updates
- Read `tickets/{taskID}/design.md` for architectural decisions
- Scan `tickets/{taskID}/communications/` for files containing `#decision` tags

### 2. Extract Knowledge

Parse the following sections from the task files:
- **Learnings**: Items under `## Learnings` in notes.md
- **Gotchas**: Items under `## Gotchas` in notes.md
- **Decisions**: Items under `## Decisions Made` in context.md, plus any `#decision` tagged communications
- **Design decisions**: Key choices documented in design.md
- **Wiki updates**: Items under `## Wiki Updates` in notes.md
- **Runbook updates**: Items under `## Runbook Updates` in notes.md

### 3. Persist to Knowledge Stores

For each extracted item:
- **Wiki updates** -- Write or update files in `docs/wiki/` with the learning content
- **Decisions** -- If significant, create an ADR in `docs/decisions/` using the next sequential number. Format: `ADR-XXXX-short-title.md`
- **Gotchas** -- Append to relevant wiki articles or create a new gotchas article
- **Runbook updates** -- Update or create files in `docs/runbooks/`

### 4. Report

```
=== Knowledge Extracted from {taskID} ===

Learnings:  N items
Decisions:  N items (M promoted to ADRs)
Gotchas:    N items
Wiki:       N articles created/updated
Runbooks:   N entries created/updated

Details:
- Created ADR-XXXX: title
- Updated docs/wiki/article-name.md
- Created docs/runbooks/procedure-name.md
```

If no extractable knowledge is found, report that the task has no documented learnings yet and suggest which sections to populate.

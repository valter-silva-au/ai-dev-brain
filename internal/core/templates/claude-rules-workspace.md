This project uses AI Dev Brain (adb) for task management and knowledge accumulation.

Workspace layout:
- `tickets/` contains task artifacts -- one directory per task with status.yaml, context.md, notes.md, design.md, and communications/
- `work/` contains git worktrees for active tasks
- `tools/` contains project scripts and utilities
- `docs/` contains documentation: wiki/, decisions/ (ADRs), runbooks/, stakeholders.md, contacts.md, glossary.md
- `backlog.yaml` is the central task registry (YAML format, do not edit directly -- use adb commands)
- `.taskconfig` holds project-wide adb configuration
- `.task_counter` tracks the next task ID number

Task IDs use the format `{{.Prefix}}-XXXXX` (e.g., {{.Prefix}}-00001).

When working on a task:
- Update the task's `context.md` with progress, decisions, and blockers
- Record technical decisions in the task's `design.md`
- Log stakeholder communications in `tickets/<task-id>/communications/`
- On completion, learnings are extracted into docs/wiki/ and docs/decisions/

Do not manually create or modify files in `tickets/` or `work/` -- use adb commands to manage tasks.

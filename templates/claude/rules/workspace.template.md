This project uses AI Dev Brain (adb) for task management.

## Workspace Layout

- `tickets/` contains task artifacts with status.yaml, context.md, notes.md, design.md
- `work/` contains git worktrees for active tasks
- `docs/` contains documentation: wiki/, decisions/ (ADRs), runbooks/
- `backlog.yaml` is the central task registry (do not edit directly)

## Conventions

- Task IDs use the format `TASK-XXXXX`
- Update context.md with progress, decisions, and blockers as you work
- Record technical decisions in design.md
- Do not manually create or modify files in `tickets/` or `work/` -- use adb commands

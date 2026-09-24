# Task Isolation Rules

## Git Worktrees
Each task gets an isolated git worktree in `work/TASK-XXXXX/`.

## Context Management
- Read task context from `tickets/TASK-XXXXX/context.md`
- Update notes in `tickets/TASK-XXXXX/notes.md`
- Follow handoff instructions in `tickets/TASK-XXXXX/handoff.md`

## Testing
Always run the project's configured test command before committing.

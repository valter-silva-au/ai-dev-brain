# Tickets

Task artifacts managed by [AI Dev Brain (adb)](https://github.com/valter-silva-au/ai-dev-brain). Each task gets its own subdirectory with structured context files.

## Structure

```
tickets/
└── TASK-00001/
    ├── status.yaml          # Task metadata (type, status, priority, owner)
    ├── context.md           # AI-maintained running context
    ├── notes.md             # Requirements, acceptance criteria, notes
    ├── design.md            # Technical design document
    ├── handoff.md           # Generated on archive (summary, learnings)
    └── communications/      # Stakeholder communications
        └── YYYY-MM-DD-source-contact-topic.md
```

## Task Lifecycle

| Status | Description |
|--------|-------------|
| `backlog` | Queued for future work |
| `in_progress` | Actively being worked on |
| `blocked` | Waiting on a dependency |
| `review` | In code review |
| `done` | Completed |
| `archived` | Archived with handoff document |

## Commands

- Create tasks: `adb feat|bug|spike|refactor <branch-name>`
- View tasks: `adb status`
- Resume a task: `adb resume <task-id>`
- Archive a task: `adb archive <task-id>`

# {{.Name}}

## Project Overview

<!-- Describe your project here: what it does, who it's for, and its key goals. -->

## Workspace Structure

This project uses [AI Dev Brain (adb)](https://github.com/valter-silva-au/ai-dev-brain) for task management, context persistence, and knowledge accumulation.

```
.
├── .taskconfig              # ADB configuration (AI type, task prefix, defaults)
├── .task_counter            # Sequential task ID counter
├── backlog.yaml             # Central task registry
├── tickets/                 # Task artifacts (one directory per task)
│   └── {{.Prefix}}-XXXXX/
│       ├── status.yaml      # Task metadata
│       ├── context.md       # AI-maintained running context
│       ├── notes.md         # Requirements and acceptance criteria
│       ├── design.md        # Technical design document
│       └── communications/  # Stakeholder communications
├── work/                    # Git worktrees for active tasks
├── tools/                   # Scripts, binaries, automation
├── docs/
│   ├── stakeholders.md      # Key people and roles
│   ├── contacts.md          # Subject matter experts
│   ├── glossary.md          # Project terminology
│   ├── wiki/                # Knowledge base (populated from completed tasks)
│   ├── decisions/           # Architecture Decision Records (ADRs)
│   └── runbooks/            # Operational procedures
└── .claude/                 # Claude Code configuration
    ├── settings.json        # Permissions
    ├── rules/               # Project rules for AI assistants
    ├── skills/              # Reusable Claude Code skills
    └── agents/              # Specialized Claude Code agents
```

## Task Management

### Commands

| Command | Description |
|---------|-------------|
| `adb feat <branch>` | Create a feature task |
| `adb bug <branch>` | Create a bug fix task |
| `adb spike <branch>` | Create a research/investigation task |
| `adb refactor <branch>` | Create a refactoring task |
| `adb status` | Show all tasks grouped by status |
| `adb resume <id>` | Resume working on a task |
| `adb archive <id>` | Archive a completed task |
| `adb priority <id> [id...]` | Reorder task priorities |
| `adb update <id>` | Generate stakeholder communication plan |
| `adb sync-context` | Regenerate AI context files |

### Task Lifecycle

```
backlog → in_progress → blocked|review → done → archived
```

### Task Types

- `feat` -- New feature work
- `bug` -- Bug fix
- `spike` -- Research or investigation (time-boxed)
- `refactor` -- Code restructuring or cleanup

### Priorities

- `P0` -- Critical (must address immediately)
- `P1` -- High (current sprint)
- `P2` -- Medium (default)
- `P3` -- Low (when convenient)

### Task IDs

Format: `{{.Prefix}}-XXXXX` (e.g., {{.Prefix}}-00001, {{.Prefix}}-00002)

## Conventions

<!-- Add your project conventions here. Examples: -->
<!-- - Branch naming: type/TASK-ID-short-description -->
<!-- - Commit format: conventional commits (feat:, fix:, etc.) -->
<!-- - Code review: all changes reviewed before merge -->

## Key Documentation

- [Stakeholders](docs/stakeholders.md) -- Outcome owners and decision makers
- [Contacts](docs/contacts.md) -- Subject matter experts
- [Glossary](docs/glossary.md) -- Project terminology
- [Decisions](docs/decisions/) -- Architecture Decision Records
- [Runbooks](docs/runbooks/) -- Operational procedures

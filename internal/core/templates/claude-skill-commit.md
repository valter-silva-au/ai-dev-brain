---
name: commit
description: Create a conventional commit with proper format and scope
allowed-tools:
  - Bash
  - Read
  - Grep
---

Create a git commit following the conventional commit format.

## Steps

1. Run `git status` to see all changed files
2. Run `git diff --staged` to review what will be committed
3. If nothing is staged, help the user stage relevant files
4. Determine the appropriate commit type from the changes
5. Write a commit message in the format: `<type>(<scope>): <description>`
6. Create the commit

## Conventional Commit Types

| Type | When to Use |
|------|-------------|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `refactor` | Code restructuring (no behavior change) |
| `docs` | Documentation only changes |
| `test` | Adding or updating tests |
| `chore` | Maintenance tasks (deps, config, CI) |
| `ci` | CI/CD pipeline changes |
| `perf` | Performance improvements |
| `style` | Formatting, whitespace (no logic change) |

## Format

```
<type>(<optional-scope>): <short description>

<optional body - explain WHY, not WHAT>

<optional footer - breaking changes, issue refs>
```

## Rules

- Description should be imperative mood ("add feature" not "added feature")
- First line under 72 characters
- Reference task IDs when available (e.g., "Implements TASK-00042")
- Mark breaking changes with `!` after type: `feat!: remove deprecated API`

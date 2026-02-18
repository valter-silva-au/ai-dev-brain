# Runbooks

Operational procedures for AI Dev Brain. Each runbook is a step-by-step guide for a specific operational task.

## Format

Runbooks follow a consistent structure:

```markdown
# Runbook: [Title]

## Prerequisites
- What must be true before starting

## Steps
1. First step
2. Second step
   - Sub-step detail
3. Third step

## Verification
- How to confirm the procedure succeeded

## Rollback
- How to undo the changes if something goes wrong

## Troubleshooting
- Common issues and their resolutions
```

## Conventions

- Each runbook is a single markdown file named descriptively: `releasing-a-new-version.md`, `setting-up-development.md`
- Steps should be concrete and copy-pasteable where possible (include exact commands)
- Runbooks are actionable procedures, not explanations -- link to wiki articles for background context
- Keep runbooks current: update them when procedures change
- Include a verification step so the operator knows the procedure succeeded

## Index

| Runbook | Description |
|---------|-------------|
| [releasing-a-new-version.md](releasing-a-new-version.md) | Build, test, and release a new version of adb |
| [setting-up-development.md](setting-up-development.md) | Set up a local development environment for adb |

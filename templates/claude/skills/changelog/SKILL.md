---
name: changelog
description: Generate a changelog from recent commits
allowed-tools:
  - Bash
  - Read
  - Grep
---

Generate a human-readable changelog from recent git commits, grouped by type.

## Steps

1. Determine the range of commits to include:
   - If a tag exists, use commits since the latest tag: `git log $(git describe --tags --abbrev=0)..HEAD --oneline`
   - If no tags exist, use the last 20 commits: `git log -20 --oneline`
2. Parse each commit message to extract:
   - Type (feat, fix, refactor, docs, chore, etc.) from conventional commit format
   - Scope (if present)
   - Description
3. Group commits by type and format as a changelog:
   - **Features** (feat)
   - **Bug Fixes** (fix)
   - **Refactoring** (refactor)
   - **Documentation** (docs)
   - **Other** (anything not matching conventional commit format)
4. Include the date range covered
5. Present the changelog in markdown format

## Output Format

```
# Changelog

## [Unreleased] - YYYY-MM-DD

### Features
- <scope>: <description> (<short-hash>)

### Bug Fixes
- <scope>: <description> (<short-hash>)

### Refactoring
- <description> (<short-hash>)

### Other
- <description> (<short-hash>)
```

## Rules

- Only include commits that are not yet in a tagged release (unless no tags exist)
- Commits that do not follow conventional commit format go into the "Other" section
- Include the short commit hash for each entry for traceability
- If there are no commits in the range, report that the changelog is empty
- Do NOT modify any files -- only display the changelog to the user

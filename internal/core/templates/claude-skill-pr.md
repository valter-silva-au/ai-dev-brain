---
name: pr
description: Create a pull request with auto-generated title and description
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
---

Create a GitHub pull request with an auto-generated title and description based on the branch commits and changes.

## Steps

1. Run `git branch --show-current` to get the current branch name
2. Determine the base branch: check if `main` exists, otherwise use `master`
3. Run `git log --oneline <base>..HEAD` to list commits on this branch
4. If there are no commits ahead of the base branch, stop and inform the user
5. Run `git diff <base>..HEAD --stat` to get a summary of changed files
6. Check if the branch is pushed to remote; run `git push -u origin <branch>` if needed
7. Check if a PR already exists: run `gh pr view --json url` -- if it does, show the URL and stop
8. Generate a PR title: derive from the branch name or first commit (keep under 72 characters)
9. Generate a PR description with:
   - A summary section explaining what changed and why
   - Key changes as bullet points from the commit log
   - A test plan section with checklist items
10. Create the PR: `gh pr create --title "<title>" --body "<body>"`
11. Report the PR URL to the user

## Title Format

- Derive from branch name: replace hyphens with spaces, capitalize first word
- If branch follows `type/description` pattern (e.g., `feat/add-auth`), use the type and description
- Keep under 72 characters

## Description Template

```
## Summary
<2-3 sentence description of changes>

## Changes
<bulleted list of key changes from commit log>

## Test Plan
- [ ] Verify <key behavior 1>
- [ ] Verify <key behavior 2>
```

## Rules

- Always check that `gh` CLI is available before attempting to create the PR
- If `gh` is not installed, provide the manual GitHub URL and instructions
- Default base branch is `main`; fall back to `master` if `main` does not exist
- Do NOT create a PR if there are no commits ahead of the base branch
- If a PR already exists for this branch, show the existing PR URL instead of creating a duplicate

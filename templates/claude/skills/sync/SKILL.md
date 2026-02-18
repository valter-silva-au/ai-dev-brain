---
name: sync
description: Sync current branch with the main branch via fetch and rebase
allowed-tools:
  - Bash
---

Synchronize the current branch with the latest changes from the main branch using fetch and rebase.

## Steps

1. Run `git branch --show-current` to get the current branch name
2. Run `git status --porcelain` to check for uncommitted changes
3. If there are uncommitted changes, stop and advise the user to commit or stash them first
4. Run `git fetch origin` to get the latest remote state
5. Determine the base branch: check if `main` exists, otherwise use `master`
6. Run `git rebase origin/<base-branch>` to rebase the current branch onto the latest base
7. If rebase encounters conflicts:
   - List the conflicting files
   - Explain the conflict situation
   - Ask the user how to proceed (resolve manually, abort, or skip)
   - Do NOT automatically resolve conflicts
8. Report the result: number of new commits pulled in, current branch position

## Rules

- NEVER run `git rebase` if there are uncommitted changes -- always require a clean working tree
- NEVER auto-resolve merge conflicts -- always show them to the user
- Use `rebase` (not `merge`) to maintain a clean linear history
- If the current branch IS the base branch (main/master), just run `git pull --rebase origin <base>` instead
- If rebase is aborted, ensure the branch is left in its original state
- Always fetch before rebasing to ensure the latest remote state

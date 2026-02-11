---
name: push
description: Push current branch to remote with upstream tracking
allowed-tools:
  - Bash
---

Push the current branch to the remote repository, setting upstream tracking if needed.

## Steps

1. Run `git branch --show-current` to get the current branch name
2. Run `git remote -v` to verify a remote exists
3. If no remote is configured, stop and inform the user they need to add a remote first
4. Run `git status --porcelain` to check for uncommitted changes
5. If there are uncommitted changes, warn the user and ask whether to proceed or commit first
6. Run `git push -u origin <branch>` to push with upstream tracking
7. If the push fails due to diverged history, inform the user and suggest `git pull --rebase` first -- do NOT force push unless the user explicitly requests it
8. Report the result: branch name, remote URL, and commit count pushed

## Rules

- NEVER run `git push --force` or `git push -f` unless the user explicitly asks for a force push
- NEVER push to `main` or `master` without explicit user confirmation
- Always use `-u` (set upstream) on the first push of a branch
- If the current branch is `main` or `master`, warn the user and ask for confirmation before pushing
- If push is rejected, explain why and suggest the appropriate resolution

---
name: review
description: Self-review all uncommitted changes before committing
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
---

Perform a thorough self-review of all uncommitted changes in the working tree, identifying issues before committing.

## Steps

1. Run `git status` to see all modified, staged, and untracked files
2. Run `git diff` to review unstaged changes
3. Run `git diff --staged` to review staged changes
4. For each changed file, analyze the diff for:
   - Correctness: logic errors, off-by-one, null handling
   - Completeness: TODO/FIXME left behind, incomplete implementations
   - Cleanup: debug statements, commented-out code, console.log/print statements
   - Security: hardcoded secrets, credentials, API keys
   - Style: inconsistent formatting, naming conventions
5. If there are untracked files, check whether they should be committed or added to .gitignore
6. Present findings grouped by severity:
   - **Must Fix**: Issues that should be addressed before committing
   - **Consider**: Suggestions that would improve the code
   - **Looks Good**: Files that passed review without issues
7. Provide a final recommendation: ready to commit, or needs changes first

## Rules

- Review ALL changed files, not just staged ones
- Flag any file that looks like it contains secrets (*.env, credentials, tokens, API keys)
- Flag any leftover debug or test code (console.log, fmt.Println used for debugging, debugger statements)
- Do NOT make changes -- this is a review-only skill. Suggest changes for the user to make.
- If no changes are found in the working tree, report that there is nothing to review

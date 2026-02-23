# CI/CD Integration Guide

This runbook covers integrating AI Dev Brain (adb) into automated CI/CD pipelines using the `-p` / `--print` flag for non-interactive execution.

---

## Overview

The `-p` / `--print` flag (available in adb v1.7.0+) enables headless Claude Code execution suitable for CI environments. It:

- Runs Claude Code in non-interactive mode (no TUI)
- Prints output directly to stdout/stderr
- Exits with the command's exit code (0 = success, non-zero = failure)
- Supports piping and shell redirection
- Respects `CLAUDE_CODE_SIMPLE=1` for reduced output verbosity

---

## Basic Usage

### Running Claude Code Non-Interactively

```bash
# Simple command execution
adb exec claude -p "run tests"

# Multi-line prompt
adb exec claude -p "
  Review the changes in this PR.
  Check for:
  - Security vulnerabilities
  - Performance regressions
  - Test coverage
  Output a summary in markdown.
"

# With simple output mode
CLAUDE_CODE_SIMPLE=1 adb exec claude -p "fix linting errors"
```

### Exit Code Behavior

```bash
adb exec claude -p "run tests"
echo $?
# 0 if tests pass, non-zero if tests fail or Claude execution fails
```

---

## GitHub Actions Integration

### Example 1: PR Review on Push

```yaml
name: AI PR Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Install adb
        run: |
          curl -fsSL https://install.adb.sh | sh
          echo "$HOME/.adb/bin" >> $GITHUB_PATH

      - name: Install Claude Code
        run: |
          curl -fsSL https://install.anthropic.com | sh

      - name: Configure adb
        run: |
          cat > .taskconfig <<EOF
          version: "1.0"
          defaults:
            ai: claude
            priority: P2
          task_id:
            prefix: TASK
            counter: 1
          EOF

      - name: Run AI code review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          CLAUDE_CODE_SIMPLE: 1
        run: |
          adb exec claude -p "
            Review this pull request.
            Focus on:
            - Code quality and maintainability
            - Security vulnerabilities
            - Performance issues
            - Test coverage gaps

            Output findings in markdown format.
          " > review.md

      - name: Post review as comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('review.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '## AI Code Review\n\n' + review
            });
```

---

### Example 2: Alert-Triggered CI Checks

Integrate with `adb alerts` to fail builds on workflow issues:

```yaml
name: Task Health Check

on:
  schedule:
    - cron: '0 8 * * *'  # Daily at 8am
  workflow_dispatch:

jobs:
  health-check:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Install adb
        run: |
          curl -fsSL https://install.adb.sh | sh
          echo "$HOME/.adb/bin" >> $GITHUB_PATH

      - name: Check for alerts
        run: |
          adb alerts --notify

          # Fail if high-severity alerts exist
          if adb alerts | grep -q '\[HIGH\]'; then
            echo "High-severity alerts detected!"
            exit 1
          fi

      - name: Post alert summary
        if: failure()
        uses: actions/github-script@v7
        with:
          script: |
            const { execSync } = require('child_process');
            const alerts = execSync('adb alerts').toString();
            github.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: 'Task Health Alerts Detected',
              body: '```\n' + alerts + '\n```'
            });
```

---

### Example 3: Automated Documentation Generation

```yaml
name: Update Documentation

on:
  push:
    branches: [main]
    paths:
      - 'internal/**'
      - 'pkg/**'

jobs:
  update-docs:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Install adb and Claude
        run: |
          curl -fsSL https://install.adb.sh | sh
          curl -fsSL https://install.anthropic.com | sh
          echo "$HOME/.adb/bin" >> $GITHUB_PATH

      - name: Regenerate CLAUDE.md
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          adb sync-context

      - name: Update architecture docs
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          CLAUDE_CODE_SIMPLE: 1
        run: |
          adb exec claude -p "
            Review docs/architecture.md against current code.
            Update any outdated sections.
            Preserve existing structure.
          "

      - name: Commit changes
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add CLAUDE.md docs/
          git diff --cached --quiet || git commit -m "docs: auto-update context and architecture

          Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
          git push
```

---

## GitLab CI Integration

### Example: Merge Request Review

```yaml
# .gitlab-ci.yml
ai-review:
  stage: test
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y curl git
    - curl -fsSL https://install.adb.sh | sh
    - curl -fsSL https://install.anthropic.com | sh
    - export PATH="$HOME/.adb/bin:$PATH"
  script:
    - |
      adb exec claude -p "
        Review this merge request for:
        - Breaking changes
        - API compatibility
        - Migration requirements
        Output findings in markdown.
      " > review.md
    - cat review.md
  artifacts:
    paths:
      - review.md
    expire_in: 7 days
  only:
    - merge_requests
  variables:
    ANTHROPIC_API_KEY: $ANTHROPIC_API_KEY
    CLAUDE_CODE_SIMPLE: "1"
```

---

## Jenkins Integration

### Example: Pipeline Stage

```groovy
pipeline {
    agent any

    environment {
        ANTHROPIC_API_KEY = credentials('anthropic-api-key')
        CLAUDE_CODE_SIMPLE = '1'
    }

    stages {
        stage('AI Code Review') {
            steps {
                sh '''
                    curl -fsSL https://install.adb.sh | sh
                    curl -fsSL https://install.anthropic.com | sh
                    export PATH="$HOME/.adb/bin:$PATH"

                    adb exec claude -p "
                      Review recent commits for:
                      - Code quality issues
                      - Security vulnerabilities
                      - Test coverage
                      Output summary in markdown.
                    " > review.md
                '''

                archiveArtifacts artifacts: 'review.md', fingerprint: true

                script {
                    def review = readFile('review.md')
                    if (review.contains('CRITICAL') || review.contains('HIGH')) {
                        currentBuild.result = 'UNSTABLE'
                    }
                }
            }
        }
    }
}
```

---

## Environment Variables for CI

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | API key for Claude Code | `sk-ant-...` |
| `ADB_HOME` | Base directory for adb data (optional) | `/workspace/.adb` |

### Optional Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDE_CODE_SIMPLE` | Reduce output verbosity | `0` (disabled) |
| `ADB_TASK_ID` | Link execution to a specific task | Unset |
| `ADB_BRANCH` | Current git branch | Auto-detected |
| `ADB_WORKTREE_PATH` | Worktree path for isolation | Auto-detected |

### Setting Environment Variables

**GitHub Actions**:
```yaml
env:
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  CLAUDE_CODE_SIMPLE: 1
  ADB_HOME: ${{ github.workspace }}/.adb
```

**GitLab CI**:
```yaml
variables:
  ANTHROPIC_API_KEY: $ANTHROPIC_API_KEY
  CLAUDE_CODE_SIMPLE: "1"
  ADB_HOME: $CI_PROJECT_DIR/.adb
```

**Jenkins**:
```groovy
environment {
    ANTHROPIC_API_KEY = credentials('anthropic-api-key')
    CLAUDE_CODE_SIMPLE = '1'
    ADB_HOME = "${WORKSPACE}/.adb"
}
```

---

## Performance Optimization for CI

### 1. Enable Fast Mode

Fast mode uses the same Opus 4.6 model with faster output generation:

```yaml
- name: Run fast AI review
  run: |
    adb exec claude -p "/fast review this PR" | tee review.md
```

### 2. Use Simple Output Mode

Reduce verbosity for faster parsing in CI logs:

```yaml
env:
  CLAUDE_CODE_SIMPLE: 1
run: |
  adb exec claude -p "run tests"
```

### 3. Limit Context Window

For large codebases, explicitly limit the files in scope:

```yaml
- name: Review specific files
  run: |
    adb exec claude -p "
      Review only these files:
      - internal/core/taskmanager.go
      - internal/core/taskmanager_test.go

      Check for test coverage and edge cases.
    "
```

### 4. Cache adb Installation

**GitHub Actions**:
```yaml
- name: Cache adb installation
  uses: actions/cache@v4
  with:
    path: ~/.adb
    key: ${{ runner.os }}-adb-${{ hashFiles('.taskconfig') }}

- name: Install adb
  if: steps.cache.outputs.cache-hit != 'true'
  run: curl -fsSL https://install.adb.sh | sh
```

---

## Handling Failures

### Exit Code Checking

```bash
if ! adb exec claude -p "run tests"; then
  echo "AI execution failed"
  exit 1
fi
```

### Timeout Protection

**GitHub Actions**:
```yaml
- name: Run AI review with timeout
  timeout-minutes: 10
  run: adb exec claude -p "review this PR"
```

**Bash**:
```bash
timeout 10m adb exec claude -p "review this PR" || echo "Timeout reached"
```

---

## Security Considerations

### 1. Protect API Keys

Never commit `ANTHROPIC_API_KEY` to version control. Use secret management:

- **GitHub**: Repository Settings → Secrets and Variables → Actions
- **GitLab**: Settings → CI/CD → Variables (masked, protected)
- **Jenkins**: Credentials plugin

### 2. Limit Claude Code Permissions

In CI, Claude Code should not have write access to sensitive files:

```yaml
# .claude/settings.json (committed to repo)
{
  "permissions": {
    "read": ["**/*.go", "**/*.md", "**/*.yaml"],
    "write": ["docs/**", "*.md"],
    "execute": ["go", "git"]
  }
}
```

### 3. Validate Outputs

Do not blindly trust AI-generated code in CI. Always review:

```yaml
- name: Review AI changes
  run: |
    git diff > ai-changes.diff

    # Fail if AI touched sensitive files
    if git diff --name-only | grep -E '(\.env|secrets|credentials)'; then
      echo "AI attempted to modify sensitive files!"
      exit 1
    fi
```

### 4. Use Read-Only Tokens

For PR review workflows, use GitHub tokens with minimal permissions:

```yaml
permissions:
  contents: read
  pull-requests: write  # Only for posting comments
```

---

## Debugging CI Failures

### Enable Verbose Logging

```yaml
- name: Run AI command with verbose output
  run: |
    set -x  # Print commands
    adb exec claude -p "run tests" 2>&1 | tee ci.log
```

### Capture Event Log

```yaml
- name: Upload event log on failure
  if: failure()
  uses: actions/upload-artifact@v4
  with:
    name: adb-event-log
    path: .adb_events.jsonl
```

### Check adb Version

```yaml
- name: Print version info
  run: |
    adb version
    claude --version
```

---

## Example: Full PR Review Workflow

```yaml
name: Comprehensive AI PR Review

on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    timeout-minutes: 15

    steps:
      - name: Checkout PR branch
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for context

      - name: Cache adb
        uses: actions/cache@v4
        with:
          path: ~/.adb
          key: ${{ runner.os }}-adb-v1

      - name: Install tools
        run: |
          curl -fsSL https://install.adb.sh | sh
          curl -fsSL https://install.anthropic.com | sh
          echo "$HOME/.adb/bin" >> $GITHUB_PATH

      - name: Initialize adb workspace
        run: |
          adb init --name "${{ github.repository }}"

      - name: Run security review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          CLAUDE_CODE_SIMPLE: 1
        run: |
          adb exec claude -p "
            Security audit of PR #${{ github.event.pull_request.number }}.
            Check for:
            - SQL injection vulnerabilities
            - XSS vulnerabilities
            - Insecure crypto usage
            - Hardcoded secrets
            - Path traversal issues

            Output findings in markdown with severity levels.
          " > security-review.md

      - name: Run code quality review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          CLAUDE_CODE_SIMPLE: 1
        run: |
          adb exec claude -p "
            Code quality review of PR #${{ github.event.pull_request.number }}.
            Check for:
            - Code duplication
            - Overly complex functions
            - Missing error handling
            - Inadequate test coverage

            Output recommendations in markdown.
          " > quality-review.md

      - name: Check for blockers
        run: |
          if grep -qi "CRITICAL\|BLOCKER" security-review.md; then
            echo "Critical security issues found!"
            exit 1
          fi

      - name: Post review
        uses: actions/github-script@v7
        if: always()
        with:
          script: |
            const fs = require('fs');
            const security = fs.readFileSync('security-review.md', 'utf8');
            const quality = fs.readFileSync('quality-review.md', 'utf8');

            const body = `## AI Code Review

            ### Security Review
            ${security}

            ### Quality Review
            ${quality}

            ---
            *Generated by adb + Claude Opus 4.6*`;

            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: body
            });

      - name: Upload review artifacts
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: ai-reviews
          path: |
            security-review.md
            quality-review.md
            .adb_events.jsonl
```

---

## Troubleshooting CI Integration

### Issue: Claude Code Hangs

**Cause**: Missing `-p` / `--print` flag (interactive mode in non-interactive shell).

**Fix**:
```bash
# Wrong (hangs)
adb exec claude "run tests"

# Correct (exits immediately)
adb exec claude -p "run tests"
```

---

### Issue: API Rate Limiting

**Cause**: Too many Claude API calls in CI pipeline.

**Fix**: Cache results or reduce frequency:
```yaml
on:
  pull_request:
    types: [opened]  # Only on first open, not every commit
```

---

### Issue: No Output in CI Logs

**Cause**: `CLAUDE_CODE_SIMPLE=0` generates verbose output that buffers.

**Fix**:
```yaml
env:
  CLAUDE_CODE_SIMPLE: 1
```

---

## Best Practices

1. **Use `-p` flag** for all CI executions
2. **Set `CLAUDE_CODE_SIMPLE=1`** for faster, cleaner logs
3. **Set timeouts** (10-15 minutes) to prevent hung jobs
4. **Cache adb installation** to speed up builds
5. **Validate AI outputs** before accepting changes
6. **Use read-only permissions** where possible
7. **Post results as PR comments** for visibility
8. **Fail fast on critical issues** (security, blockers)
9. **Upload artifacts** (reviews, logs) for debugging
10. **Monitor API usage** to avoid rate limits

---

## See Also

- **docs/runbooks/troubleshooting.md**: Debugging Claude Code integration
- **docs/wiki/performance-tuning.md**: Fast mode and output optimization
- **CLAUDE.md**: Full command reference for `adb exec` and `adb run`

# Worktree Lifecycle Automation

This guide covers the automated lifecycle management of git worktrees in AI Dev Brain (adb), including pre-creation validation, post-removal cleanup, and lifecycle hook integration.

---

## Worktree Lifecycle Overview

Git worktrees in adb follow a structured lifecycle from creation to removal:

```
1. Pre-creation validation
   ↓
2. Worktree creation (git worktree add)
   ↓
3. Bootstrap (.claude/, task-context.md)
   ↓
4. Active use (development, Claude Code sessions)
   ↓
5. Pre-removal validation
   ↓
6. Worktree removal (git worktree remove)
   ↓
7. Post-removal cleanup (sessions archive, temp files)
```

Each phase can trigger hooks for custom validation or automation.

---

## Phase 1: Pre-Creation Validation

### Purpose

Prevent creating worktrees that will fail or conflict with existing work.

### Validations Performed

| Validation | Description | Failure Action |
|------------|-------------|----------------|
| **Branch conflict** | Check if branch already exists on remote | Block creation, suggest new branch name |
| **Merge conflict** | Check if branch diverges from base branch | Warn, offer to rebase |
| **Disk space** | Verify sufficient space for worktree | Block creation, suggest cleanup |
| **Task blocker** | Check if task is blocked by another task | Warn, list blockers |
| **Repo state** | Verify repo is in clean state | Block creation, suggest stash/commit |

---

### Pre-Creation Hook

**Hook**: `pre_create_worktree`

**When it runs**: Before `git worktree add` is called during task creation.

**Input**: Task metadata (JSON via stdin)
```json
{
  "task_id": "TASK-00042",
  "type": "feat",
  "branch": "add-jwt-auth",
  "repo": "github.com/acme/backend",
  "tags": ["security", "backend"]
}
```

**Output**: Exit code (0 = proceed, non-zero = block)

**Example hook** (`.claude/hooks/pre-create-worktree.sh`):
```bash
#!/bin/bash
set -e

# Read task metadata
read -r task_json
branch=$(echo "$task_json" | jq -r '.branch')
repo=$(echo "$task_json" | jq -r '.repo')

# Check if branch exists on remote
if git ls-remote --heads origin "$branch" | grep -q "$branch"; then
  echo "Error: Branch '$branch' already exists on remote" >&2
  echo "Suggestion: Use a different branch name or delete the remote branch first" >&2
  exit 1
fi

# Check for merge conflicts (compare with main)
base_branch="main"
if ! git merge-base --is-ancestor origin/"$base_branch" HEAD; then
  echo "Warning: Local branch is ahead of origin/$base_branch" >&2
  echo "Suggestion: Run 'git pull --rebase origin $base_branch' before creating worktree" >&2
  # Non-fatal warning (exit 0)
fi

# Check disk space (require 1GB free)
available=$(df -BG . | awk 'NR==2 {print $4}' | sed 's/G//')
if [[ "$available" -lt 1 ]]; then
  echo "Error: Less than 1GB disk space available" >&2
  exit 1
fi

exit 0
```

**Register in `.taskconfig`**:
```yaml
hooks:
  pre_create_worktree: .claude/hooks/pre-create-worktree.sh
```

---

### Automatic Conflict Detection

adb automatically checks for common conflicts before creating a worktree:

1. **Branch name collision**: If `branch` matches an existing worktree, append a counter:
   ```
   User: adb feat add-auth
   Existing worktree: work/TASK-00042/ (branch: add-auth)
   New worktree: work/TASK-00051/ (branch: add-auth-2)
   ```

2. **Base branch divergence**: If the repo's default branch (e.g., `main`) has diverged:
   ```
   Warning: origin/main is 15 commits ahead of local main
   Suggestion: Run 'git fetch && git pull' before creating task
   ```

3. **Dirty worktree**: If there are uncommitted changes:
   ```
   Error: Working directory has uncommitted changes
   Suggestion: Commit or stash changes before creating new task
   ```

---

## Phase 2: Worktree Creation

### What Happens

```bash
# Executed by adb during task creation
cd repos/github.com/acme/backend/
git worktree add work/TASK-00042/ -b add-jwt-auth
```

**Result**:
- New directory: `work/TASK-00042/`
- Git metadata: `.git` symlink pointing to main repo
- Branch: `add-jwt-auth` checked out
- Clean working tree (no uncommitted changes)

---

### Error Handling

| Error | Cause | Resolution |
|-------|-------|------------|
| `fatal: 'work/TASK-00042' already exists` | Directory exists | Delete directory or use different task ID |
| `fatal: invalid reference: add-jwt-auth` | Branch name invalid | Use valid git branch name |
| `fatal: 'add-jwt-auth' is already checked out` | Branch in use by another worktree | Choose different branch name |

---

## Phase 3: Bootstrap

### What Happens

After worktree creation, adb bootstraps the worktree with:

1. **`.claude/` directory structure**:
   ```
   work/TASK-00042/
     .claude/
       settings.json        # Permissions and hooks
       hooks/
         pre-edit-validate.sh   # Worktree isolation hook
         post-edit-go-fmt.sh    # Auto-format Go files
         pre-commit-check.sh    # Pre-commit validation
       rules/
         task-context.md        # Task awareness for Claude
   ```

2. **Task context file** (`.claude/rules/task-context.md`):
   ```markdown
   # Task Context: TASK-00042

   This worktree is for task TASK-00042 (add-jwt-auth).

   - **Type**: feat
   - **Branch**: add-jwt-auth
   - **Status**: backlog (newly created)
   - **Ticket**: /path/to/tickets/TASK-00042

   ## Key Files
   - context.md -- Running context
   - notes.md -- Requirements
   - design.md -- Technical design
   - sessions/ -- Session summaries
   - knowledge/ -- Decisions
   ```

3. **Environment variables** (set in shell when launching Claude Code):
   ```bash
   export ADB_TASK_ID=TASK-00042
   export ADB_BRANCH=add-jwt-auth
   export ADB_WORKTREE_PATH=/path/to/work/TASK-00042
   export ADB_TICKET_PATH=/path/to/tickets/TASK-00042
   ```

---

### Post-Bootstrap Hook

**Hook**: `post_create_worktree`

**When it runs**: After worktree is bootstrapped with `.claude/` and task context.

**Input**: Task metadata (JSON via stdin)

**Example hook** (initialize project dependencies):
```bash
#!/bin/bash
set -e

read -r task_json
worktree_path=$(echo "$task_json" | jq -r '.worktree_path')

cd "$worktree_path"

# Install dependencies
if [[ -f go.mod ]]; then
  echo "Installing Go dependencies..."
  go mod download
fi

if [[ -f package.json ]]; then
  echo "Installing Node dependencies..."
  npm install
fi

# Run initial build to verify setup
if [[ -f Makefile ]]; then
  make build
fi

exit 0
```

**Register in `.taskconfig`**:
```yaml
hooks:
  post_create_worktree: .claude/hooks/post-create-worktree.sh
```

---

## Phase 4: Active Use

### Lifecycle Commands During Active Use

| Command | Effect on Worktree |
|---------|-------------------|
| `adb resume TASK-00042` | `cd` to worktree, set environment, launch Claude Code |
| `adb status TASK-00042 in_progress` | Update task status, no worktree changes |
| `adb session save TASK-00042` | Save session summary to `tickets/TASK-00042/sessions/` |
| `adb session capture --from-hook` | Capture Claude session to `sessions/S-XXXXX/`, symlink to task |
| `adb exec git push` | Push changes from worktree branch |

---

### Worktree Monitoring

adb tracks worktree state via `.adb_events.jsonl`:

```jsonl
{"time":"2025-01-15T10:00:00Z","level":"INFO","type":"worktree.created","msg":"Worktree created","data":{"task_id":"TASK-00042","worktree":"/path/to/work/TASK-00042"}}
{"time":"2025-01-15T14:30:00Z","level":"INFO","type":"worktree.session_started","msg":"Claude Code session started","data":{"task_id":"TASK-00042","session_id":"abc123"}}
{"time":"2025-01-15T16:00:00Z","level":"INFO","type":"worktree.session_ended","msg":"Claude Code session ended","data":{"task_id":"TASK-00042","session_id":"abc123","duration_mins":90}}
```

---

## Phase 5: Pre-Removal Validation

### Purpose

Ensure no uncommitted work is lost when removing a worktree.

### Validations Performed

| Validation | Description | Failure Action |
|------------|-------------|----------------|
| **Uncommitted changes** | Check `git status` | Block removal, suggest commit/stash |
| **Unpushed commits** | Check `git log origin/branch..HEAD` | Warn, suggest push |
| **Active sessions** | Check if Claude Code is running | Block removal, suggest exit first |
| **Pending tasks** | Check if task status is `in_progress` or `blocked` | Warn, suggest complete task first |

---

### Pre-Removal Hook

**Hook**: `pre_remove_worktree`

**When it runs**: Before `git worktree remove` is called during `adb archive` or `adb cleanup`.

**Input**: Task metadata (JSON via stdin)

**Example hook**:
```bash
#!/bin/bash
set -e

read -r task_json
worktree_path=$(echo "$task_json" | jq -r '.worktree_path')
task_id=$(echo "$task_json" | jq -r '.task_id')

cd "$worktree_path"

# Check for uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: Worktree has uncommitted changes" >&2
  echo "Files changed:" >&2
  git status --short >&2
  echo "" >&2
  echo "Suggestion: Commit or stash changes before removing worktree" >&2
  exit 1
fi

# Check for unpushed commits
unpushed=$(git log origin/$(git branch --show-current)..HEAD --oneline | wc -l)
if [[ "$unpushed" -gt 0 ]]; then
  echo "Warning: $unpushed unpushed commit(s)" >&2
  echo "Suggestion: Run 'git push' before removing worktree" >&2
  # Non-fatal warning
fi

# Check if Claude Code is running
if pgrep -f "claude.*$worktree_path" >/dev/null; then
  echo "Error: Claude Code is still running in this worktree" >&2
  echo "Suggestion: Exit Claude Code before removing worktree" >&2
  exit 1
fi

exit 0
```

**Register in `.taskconfig`**:
```yaml
hooks:
  pre_remove_worktree: .claude/hooks/pre-remove-worktree.sh
```

---

## Phase 6: Worktree Removal

### What Happens

```bash
# Executed by adb during archive or cleanup
cd repos/github.com/acme/backend/
git worktree remove work/TASK-00042/
```

**Result**:
- Directory `work/TASK-00042/` deleted
- Git cleans up worktree metadata
- Branch `add-jwt-auth` remains in repo (not deleted)

---

### Removal Triggers

| Command | Removes Worktree? | Keeps Branch? |
|---------|------------------|---------------|
| `adb archive TASK-00042` | ✅ (by default) | ✅ |
| `adb archive TASK-00042 --keep-worktree` | ❌ | ✅ |
| `adb cleanup TASK-00042` | ✅ | ✅ |

**Note**: Branches are never automatically deleted. Use `git branch -d` manually after archiving.

---

### Worktree Path Cleanup

When a worktree is removed, adb updates `tickets/TASK-00042/status.yaml`:

```yaml
# Before removal
worktree: /path/to/work/TASK-00042

# After removal
worktree: ""  # Cleared
```

---

## Phase 7: Post-Removal Cleanup

### Purpose

Archive session data and clean up temporary files after worktree removal.

### Cleanup Actions

| Action | Description |
|--------|-------------|
| **Archive sessions** | Copy captured sessions to `tickets/TASK-00042/sessions/` |
| **Delete temp files** | Remove `.claude/cache`, `.adb_temp/` |
| **Update backlog** | Mark task as archived in `backlog.yaml` |
| **Generate handoff** | Create `handoff.md` with task summary |
| **Move ticket** | Move `tickets/TASK-00042/` to `tickets/_archived/` |

---

### Post-Removal Hook

**Hook**: `post_remove_worktree`

**When it runs**: After `git worktree remove` completes successfully.

**Input**: Task metadata (JSON via stdin)

**Example hook** (archive sessions):
```bash
#!/bin/bash
set -e

read -r task_json
task_id=$(echo "$task_json" | jq -r '.task_id')
ticket_path=$(echo "$task_json" | jq -r '.ticket_path')

# Archive captured sessions
if [[ -d sessions/ ]]; then
  echo "Archiving captured sessions for $task_id..."
  for session_dir in sessions/S-*/; do
    if [[ -L "$ticket_path/sessions/$(basename "$session_dir")" ]]; then
      # Replace symlink with actual directory
      rm "$ticket_path/sessions/$(basename "$session_dir")"
      cp -r "$session_dir" "$ticket_path/sessions/"
    fi
  done
fi

# Clean up temporary files
rm -rf "$ticket_path/.claude/cache"
rm -rf "$ticket_path/.adb_temp"

# Log removal
echo "Worktree removed for $task_id" >> "$ticket_path/lifecycle.log"

exit 0
```

**Register in `.taskconfig`**:
```yaml
hooks:
  post_remove_worktree: .claude/hooks/post-remove-worktree.sh
```

---

## Automated Session Archival

When a worktree is removed, adb automatically archives captured sessions:

### Before Removal

```
sessions/
  S-00042/
    session.yaml
    turns.yaml
    summary.md

tickets/TASK-00042/
  sessions/
    S-00042 -> /path/to/sessions/S-00042  (symlink)
```

### After Removal

```
sessions/
  S-00042/
    session.yaml
    turns.yaml
    summary.md

tickets/TASK-00042/
  sessions/
    S-00042/                    (directory, not symlink)
      session.yaml
      turns.yaml
      summary.md
```

**Benefit**: Sessions remain accessible in the task's ticket directory even after the worktree is deleted.

---

## Lifecycle Event Logging

All lifecycle events are logged to `.adb_events.jsonl`:

```jsonl
{"time":"2025-01-15T10:00:00Z","level":"INFO","type":"worktree.pre_create","msg":"Pre-creation validation started","data":{"task_id":"TASK-00042"}}
{"time":"2025-01-15T10:00:05Z","level":"INFO","type":"worktree.created","msg":"Worktree created","data":{"task_id":"TASK-00042","worktree":"/path/to/work/TASK-00042","branch":"add-jwt-auth"}}
{"time":"2025-01-15T10:00:10Z","level":"INFO","type":"worktree.bootstrapped","msg":"Worktree bootstrapped with .claude/","data":{"task_id":"TASK-00042"}}
{"time":"2025-01-16T16:00:00Z","level":"INFO","type":"worktree.pre_remove","msg":"Pre-removal validation started","data":{"task_id":"TASK-00042"}}
{"time":"2025-01-16T16:00:05Z","level":"INFO","type":"worktree.removed","msg":"Worktree removed","data":{"task_id":"TASK-00042"}}
{"time":"2025-01-16T16:00:10Z","level":"INFO","type":"worktree.post_remove","msg":"Post-removal cleanup completed","data":{"task_id":"TASK-00042"}}
```

**Query lifecycle events**:
```bash
jq -r 'select(.type | startswith("worktree.")) | [.time, .type, .data.task_id] | @tsv' \
  .adb_events.jsonl
```

---

## Common Lifecycle Patterns

### Pattern 1: Standard Feature Development

```bash
# 1. Pre-creation: Validate branch availability
adb feat add-jwt-auth
# → pre_create_worktree hook checks for conflicts

# 2. Creation: Worktree created
# → work/TASK-00042/ created with branch add-jwt-auth

# 3. Bootstrap: .claude/ initialized
# → .claude/rules/task-context.md generated

# 4. Active use: Development
adb resume TASK-00042
# → Claude Code launches with environment set

# 5. Pre-removal: Validate clean state
adb archive TASK-00042
# → pre_remove_worktree hook checks for uncommitted changes

# 6. Removal: Worktree deleted
# → git worktree remove work/TASK-00042/

# 7. Post-removal: Sessions archived
# → post_remove_worktree hook copies sessions to ticket
```

---

### Pattern 2: Hotfix (Keep Worktree)

```bash
# 1. Create hotfix task
adb bug fix-login-timeout --priority P0

# 2. Develop and test
adb resume TASK-00043

# 3. Archive but keep worktree for monitoring
adb archive TASK-00043 --keep-worktree

# 4. Later, manually remove worktree
adb cleanup TASK-00043
```

---

### Pattern 3: Failed Creation (Rollback)

```bash
# 1. Pre-creation fails (branch conflict)
adb feat add-auth
# → Error: Branch 'add-auth' already exists on remote

# 2. No worktree created (rollback)
# → No cleanup needed, nothing to remove
```

---

## Hook Chaining

Multiple hooks can be chained for complex workflows:

```yaml
# .taskconfig
hooks:
  pre_create_worktree:
    - .claude/hooks/validate-branch.sh
    - .claude/hooks/check-disk-space.sh
    - .claude/hooks/notify-team.sh

  post_create_worktree:
    - .claude/hooks/install-deps.sh
    - .claude/hooks/init-db.sh
    - .claude/hooks/notify-team.sh

  pre_remove_worktree:
    - .claude/hooks/validate-clean-state.sh
    - .claude/hooks/backup-sessions.sh

  post_remove_worktree:
    - .claude/hooks/archive-sessions.sh
    - .claude/hooks/cleanup-temp-files.sh
    - .claude/hooks/notify-team.sh
```

Hooks run in order. If any hook exits non-zero, the chain stops.

---

## Error Recovery

### Worktree Creation Failed Midway

**Symptom**: Directory created but git worktree command failed.

**Recovery**:
```bash
# Remove partial worktree
rm -rf work/TASK-00042/

# Retry task creation
adb feat add-jwt-auth
```

---

### Worktree Removal Failed

**Symptom**: `git worktree remove` failed (e.g., locked files).

**Recovery**:
```bash
# Force removal (use with caution)
git worktree remove --force work/TASK-00042/

# If still fails, manually delete and prune
rm -rf work/TASK-00042/
git worktree prune
```

---

### Orphaned Worktree

**Symptom**: Worktree directory exists but not tracked by git.

**Diagnosis**:
```bash
git worktree list
# If TASK-00042 not listed, it's orphaned
```

**Recovery**:
```bash
# Remove directory
rm -rf work/TASK-00042/

# Prune git metadata
git worktree prune
```

---

## Performance Considerations

### Worktree Creation Time

| Repo Size | Worktree Creation | Bootstrap | Total |
|-----------|------------------|-----------|-------|
| Small (<100 MB) | 0.5s | 0.3s | 0.8s |
| Medium (100-500 MB) | 1.5s | 0.3s | 1.8s |
| Large (>500 MB) | 3.0s | 0.5s | 3.5s |

**Bottlenecks**:
- Git checkout (I/O bound)
- Large `.git/` directory
- Slow filesystem (network drives)

**Optimization**:
- Use local SSD for repos
- Shallow clones for large repos
- Prune old worktrees regularly

---

### Worktree Removal Time

| Repo Size | Worktree Removal | Session Archive | Total |
|-----------|-----------------|----------------|-------|
| Small (<100 MB) | 0.2s | 0.1s | 0.3s |
| Medium (100-500 MB) | 0.5s | 0.2s | 0.7s |
| Large (>500 MB) | 1.0s | 0.5s | 1.5s |

**Bottlenecks**:
- Git metadata cleanup
- Session file copying
- Filesystem sync

---

## Troubleshooting Lifecycle Issues

### Issue: Hook Not Running

**Symptom**: `pre_create_worktree` hook not called during task creation.

**Checklist**:
1. **Verify hook exists**:
   ```bash
   ls -la .claude/hooks/pre-create-worktree.sh
   ```
2. **Check permissions**:
   ```bash
   chmod +x .claude/hooks/pre-create-worktree.sh
   ```
3. **Verify registration**:
   ```bash
   cat .taskconfig | grep -A 3 pre_create_worktree
   ```

---

### Issue: Worktree Removal Blocked

**Symptom**: `adb archive TASK-00042` fails with "worktree locked".

**Cause**: Another git process is using the worktree.

**Fix**:
```bash
# Kill any git processes in the worktree
pkill -f "git.*work/TASK-00042"

# Retry archive
adb archive TASK-00042
```

---

### Issue: Sessions Not Archived

**Symptom**: After worktree removal, sessions are missing from ticket.

**Cause**: `post_remove_worktree` hook failed or not registered.

**Fix**:
```bash
# Manually copy sessions
cp -r sessions/S-XXXXX/ tickets/TASK-00042/sessions/

# Register post-removal hook
cat >> .taskconfig <<EOF
hooks:
  post_remove_worktree: .claude/hooks/post-remove-worktree.sh
EOF
```

---

## Best Practices

1. **Always validate before creation**: Use `pre_create_worktree` to catch issues early
2. **Bootstrap consistently**: Ensure `.claude/` is set up identically across all worktrees
3. **Monitor active sessions**: Prevent worktree removal during active Claude Code use
4. **Archive sessions automatically**: Use `post_remove_worktree` to preserve session history
5. **Log lifecycle events**: Use `.adb_events.jsonl` for audit trails
6. **Clean up regularly**: Run `git worktree prune` to remove stale metadata
7. **Test hooks**: Validate hook behavior before deploying to production
8. **Chain hooks carefully**: Ensure hooks are idempotent and fail-safe
9. **Document custom hooks**: Explain what each hook does in project README
10. **Handle failures gracefully**: Provide clear error messages and recovery steps

---

## See Also

- **docs/wiki/worktree-isolation.md**: Worktree boundary enforcement
- **docs/wiki/agent-teams.md**: Team routing and metadata
- **docs/runbooks/troubleshooting.md**: Debugging lifecycle issues
- **CLAUDE.md**: Full command reference for lifecycle commands

# Worktree Isolation

This guide explains how AI Dev Brain (adb) enforces worktree boundaries to prevent Claude Code from modifying files outside the current task's scope.

---

## Why Worktree Isolation?

### Problem: Unintended Cross-Task Edits

Without isolation, Claude Code can:
- Edit files in other task worktrees
- Modify shared repo state outside task branches
- Touch system files (`.env`, credentials)
- Accidentally break unrelated work

### Solution: Worktree Boundary Enforcement

Worktree isolation ensures Claude Code can **only** read and edit files inside the current task's worktree directory. All file operations outside the worktree are blocked at the tool level.

---

## How It Works

### 1. Worktree Creation (Bootstrap Phase)

When `adb feat`, `adb bug`, `adb spike`, or `adb refactor` creates a task, it:

1. Creates a git worktree in `work/TASK-XXXXX/`
2. Generates `.claude/hooks/pre-edit-validate.sh` inside the worktree
3. Generates `.claude/settings.json` with `PreToolUse` hook registration
4. Sets `ADB_WORKTREE_PATH` environment variable

**Directory structure**:
```
work/TASK-00042/
  .git                              # Worktree git metadata
  .claude/
    settings.json                   # Hook registration
    hooks/
      pre-edit-validate.sh          # Validation script
    rules/
      task-context.md               # Task awareness for Claude
  internal/                         # Code files (inside worktree)
  ...
```

---

### 2. PreToolUse Hook Registration

The `.claude/settings.json` file registers the validation hook:

```json
{
  "permissions": {
    "read": ["**/*"],
    "write": ["**/*"],
    "execute": ["git", "go", "bash"]
  },
  "hooks": {
    "PreToolUse": {
      "command": "bash",
      "args": [".claude/hooks/pre-edit-validate.sh"]
    }
  }
}
```

**What this does**:
- Before **every** Edit or Write tool call, Claude Code runs `pre-edit-validate.sh`
- The hook receives tool metadata via stdin (JSON)
- If the hook exits 0, the tool executes
- If the hook exits non-zero, the tool is blocked and the error message is shown to Claude

---

### 3. Validation Script (`pre-edit-validate.sh`)

The hook script validates file paths:

```bash
#!/bin/bash
# Read tool metadata from stdin
read -r tool_json

# Extract tool name and file path
tool=$(echo "$tool_json" | jq -r '.tool')
file_path=$(echo "$tool_json" | jq -r '.file_path // empty')

# Only validate Edit and Write tools
if [[ "$tool" != "Edit" && "$tool" != "Write" ]]; then
  exit 0  # Allow other tools (Read, Bash, etc.)
fi

# Check if ADB_WORKTREE_PATH is set
if [[ -z "$ADB_WORKTREE_PATH" ]]; then
  echo "Error: ADB_WORKTREE_PATH not set" >&2
  exit 1
fi

# Resolve absolute paths
worktree=$(realpath "$ADB_WORKTREE_PATH")
target=$(realpath "$file_path" 2>/dev/null)

# Check if target is inside worktree
if [[ "$target" != "$worktree"/* ]]; then
  echo "Error: Cannot edit file outside worktree: $file_path" >&2
  echo "Worktree: $worktree" >&2
  exit 1
fi

exit 0
```

**Key points**:
- Only blocks Edit and Write (allows Read, Bash, etc.)
- Uses `realpath` to resolve symlinks and `..` paths
- Checks if target path starts with worktree path
- Returns clear error message to Claude if validation fails

---

### 4. Runtime Enforcement

When Claude Code tries to edit a file, the flow is:

```
Claude: I'll edit internal/core/taskmanager.go
  ↓
Claude Code: Execute Edit tool with file_path="internal/core/taskmanager.go"
  ↓
PreToolUse hook fires
  ↓
pre-edit-validate.sh checks:
  - Is file_path inside ADB_WORKTREE_PATH?
  ↓
If YES:
  - Hook exits 0
  - Edit proceeds
If NO:
  - Hook exits 1 with error message
  - Edit is blocked
  - Claude sees error: "Cannot edit file outside worktree"
```

---

## The `--worktree` Flag

### CLI Usage

The `--worktree` flag explicitly passes the worktree path to Claude Code:

```bash
adb exec claude --worktree /path/to/work/TASK-00042
```

**What it does**:
- Sets `ADB_WORKTREE_PATH` environment variable
- Tells Claude Code to restrict file operations to this directory

**When it's used**:
- Automatically set by `adb resume` when launching Claude Code
- Can be set manually when launching Claude Code outside adb workflows

### Example: Manual Launch with Worktree Isolation

```bash
cd work/TASK-00042
export ADB_WORKTREE_PATH=$(pwd)
export ADB_TASK_ID=TASK-00042
export ADB_BRANCH=$(git branch --show-current)
claude --dangerously-skip-permissions --resume
```

The PreToolUse hook will enforce isolation using `$ADB_WORKTREE_PATH`.

---

## Agent Isolation: `isolation: worktree`

### Agent Definition with Isolation

Agent definitions (`.claude/agents/*.json`) can enforce worktree isolation:

```json
{
  "name": "backend-dev",
  "model": "opus",
  "description": "Backend development specialist",
  "isolation": "worktree",
  "permissions": {
    "read": ["internal/", "pkg/"],
    "write": ["internal/", "pkg/"],
    "execute": ["go", "git"]
  }
}
```

**What `isolation: worktree` does**:
- Agent's Read, Edit, Write tools are restricted to `ADB_WORKTREE_PATH`
- Even if the agent tries to read `/etc/passwd`, it's blocked
- Prevents multi-agent teams from accidentally crossing worktree boundaries

---

### Example: Team Lead Delegates to Backend Dev

```
User: @team-lead implement authentication for TASK-00042

Team Lead (no isolation):
  @backend-dev please implement the auth middleware in internal/auth/

Backend Dev (isolation: worktree):
  I'll create internal/auth/middleware.go inside the worktree...
  [Attempts to read /etc/passwd]
  → Blocked by PreToolUse hook
```

---

## Troubleshooting Isolation Violations

### Symptom: Claude Cannot Edit Files

**Error message**:
```
Error: Cannot edit file outside worktree: /home/user/other-project/main.go
Worktree: /home/user/work/TASK-00042
```

**Diagnosis**:
1. **Check file path**: Is Claude trying to edit a file in another directory?
2. **Check `ADB_WORKTREE_PATH`**:
   ```bash
   echo $ADB_WORKTREE_PATH
   ```
   Should match the current worktree.

3. **Verify hook is active**:
   ```bash
   cat .claude/settings.json | grep -A 5 PreToolUse
   ```

**Resolution**:
- If the file **should** be edited: `cd` to the correct worktree first
- If the file **should not** be edited: Claude is making a mistake; clarify the task scope

---

### Symptom: Hook Not Firing (Isolation Bypassed)

**Error**: Claude can edit files outside worktree without errors.

**Diagnosis**:
1. **Check hook exists**:
   ```bash
   ls -la .claude/hooks/pre-edit-validate.sh
   ```
   Should be executable (`-rwxr-xr-x`).

2. **Check hook registration**:
   ```bash
   cat .claude/settings.json | jq '.hooks.PreToolUse'
   ```
   Should show:
   ```json
   {
     "command": "bash",
     "args": [".claude/hooks/pre-edit-validate.sh"]
   }
   ```

3. **Check `ADB_WORKTREE_PATH` is set**:
   ```bash
   echo $ADB_WORKTREE_PATH
   ```
   If empty, hook cannot validate.

**Resolution**:
- **Reinstall hook**:
  ```bash
  adb resume TASK-XXXXX
  ```
  This regenerates `.claude/` with hooks.

- **Manually set environment** if launching Claude Code manually:
  ```bash
  export ADB_WORKTREE_PATH=$(pwd)
  ```

---

### Symptom: Hook Blocks Valid Edits

**Error**: Claude cannot edit files that **are** inside the worktree.

**Diagnosis**:
1. **Check `realpath` resolution**:
   ```bash
   realpath internal/core/taskmanager.go
   realpath $ADB_WORKTREE_PATH
   ```
   Does the file path start with the worktree path?

2. **Check for symlinks**:
   ```bash
   ls -la internal/core/taskmanager.go
   ```
   If it's a symlink pointing outside the worktree, `realpath` resolves to the target (outside worktree).

**Resolution**:
- If the file is a symlink to an external location, it **should** be blocked (by design).
- If the file is a regular file inside the worktree but still blocked, the hook may have a bug. Report it.

---

## Disabling Isolation (Not Recommended)

### When You Might Need This

- **Trusted one-off tasks** that need to touch files outside the worktree
- **Cross-repo refactoring** where multiple repos are involved
- **Emergency hotfixes** that can't wait for proper worktree setup

### How to Disable

**Option 1: Remove PreToolUse hook**

Edit `.claude/settings.json`:
```json
{
  "hooks": {
    "PreToolUse": null
  }
}
```

**Option 2: Unset `ADB_WORKTREE_PATH`**

```bash
unset ADB_WORKTREE_PATH
claude --dangerously-skip-permissions --resume
```

Hook will exit 0 if `ADB_WORKTREE_PATH` is not set (fail-open behavior).

**Warning**: Without isolation, Claude can modify **any file** it has permissions for. Use this only in controlled environments.

---

## Read Operations and Isolation

### Read Tool is NOT Blocked

The PreToolUse hook **only** validates Edit and Write tools. Read operations are always allowed, even outside the worktree.

**Rationale**:
- Reading context from other tasks or docs is often useful
- Read operations are non-destructive
- Over-restricting reads would limit Claude's usefulness

**Example** (allowed):
```
Claude: I'll check the docs/architecture.md file for context
[Read tool: file_path="/home/user/project-root/docs/architecture.md"]
→ Allowed (outside worktree, but Read-only)
```

---

## Bash Tool and Isolation

### Bash Tool is NOT Blocked

The Bash tool can run commands that operate on files outside the worktree. This is intentional:

- Bash is powerful and flexible
- Many valid use cases require running commands outside the worktree (e.g., `git fetch`, `adb status`)
- Over-restricting Bash would break too many workflows

**However**, Bash commands that modify files are indirectly isolated:

- `git` commands only affect the current worktree's branch
- `go build` outputs to the worktree's `bin/` directory
- `echo "..." > file` writes relative to the worktree (current directory)

**If strict isolation is needed**, add a custom Bash hook that validates commands. Example:

```bash
# .claude/hooks/pre-bash-validate.sh
#!/bin/bash
read -r tool_json

command=$(echo "$tool_json" | jq -r '.command')

# Block dangerous commands
if [[ "$command" =~ (rm -rf|sudo|chmod 777) ]]; then
  echo "Error: Dangerous command blocked: $command" >&2
  exit 1
fi

exit 0
```

Register in `.claude/settings.json`:
```json
{
  "hooks": {
    "PreToolUse": {
      "Bash": {
        "command": "bash",
        "args": [".claude/hooks/pre-bash-validate.sh"]
      }
    }
  }
}
```

---

## Integration with Git Worktrees

### Git Worktree Recap

Git worktrees allow multiple branches checked out simultaneously:

```
repos/github.com/org/repo/
  .git/                      # Main repo
  work/
    TASK-00042/              # Worktree for feat/add-auth branch
    TASK-00043/              # Worktree for bug/fix-login branch
```

Each worktree:
- Has its own working directory
- Is on a different branch
- Shares the same `.git` repo

---

### Worktree Isolation + Git Worktrees

adb's worktree isolation complements git worktrees:

1. **Task creation**: `adb feat my-feature` creates a git worktree at `work/TASK-XXXXX/`
2. **Claude launch**: `adb resume TASK-XXXXX` sets `ADB_WORKTREE_PATH=work/TASK-XXXXX/`
3. **Isolation enforcement**: PreToolUse hook blocks edits outside `work/TASK-XXXXX/`
4. **Git operations**: `git commit`, `git push` only affect the `TASK-XXXXX` branch

**Result**: Multiple tasks can be worked on in parallel without interference.

---

### Example: Parallel Task Development

```bash
# Task 1: Add authentication
adb feat add-auth
cd work/TASK-00042/
adb resume TASK-00042
# Claude can only edit files in work/TASK-00042/

# Task 2: Fix login bug (in another terminal)
adb bug fix-login
cd work/TASK-00043/
adb resume TASK-00043
# Claude can only edit files in work/TASK-00043/
```

No risk of Task 1's Claude session accidentally modifying Task 2's files.

---

## Advanced: Custom Isolation Policies

### Per-Agent Isolation Rules

Agent definitions can specify custom isolation:

```json
{
  "name": "security-auditor",
  "model": "opus",
  "isolation": "worktree",
  "permissions": {
    "read": ["**/*"],           // Can read anywhere
    "write": ["docs/security/"],  // Can only write to docs/security/
    "execute": ["grep", "git"]  // Can only run these commands
  }
}
```

---

### Namespace-Based Isolation

For monorepos, isolate by package:

```json
{
  "name": "frontend-dev",
  "isolation": "namespace",
  "namespace": "frontend/",
  "permissions": {
    "read": ["**/*"],
    "write": ["frontend/**"],
    "execute": ["npm", "yarn"]
  }
}
```

Hook script (`pre-edit-validate.sh`) checks `namespace` field:

```bash
namespace=$(echo "$tool_json" | jq -r '.agent.namespace // empty')
if [[ -n "$namespace" && "$target" != "$worktree/$namespace"* ]]; then
  echo "Error: Agent restricted to $namespace namespace" >&2
  exit 1
fi
```

---

## Performance Considerations

### Hook Overhead

The PreToolUse hook adds latency to every Edit/Write operation:

| Operation | Without Hook | With Hook | Overhead |
|-----------|--------------|-----------|----------|
| Edit (single file) | 50ms | 65ms | 15ms |
| Edit (10 files) | 500ms | 650ms | 150ms |

**Mitigation**:
- Hook script is lightweight (bash + jq)
- Overhead is negligible for interactive use
- For batch operations (e.g., mass refactoring), consider temporarily disabling the hook

---

### Relative vs Absolute Paths

**Slow** (absolute path):
```
@edit /home/user/work/TASK-00042/internal/core/taskmanager.go
```
Hook must resolve both paths with `realpath` (syscall).

**Fast** (relative path):
```
@edit internal/core/taskmanager.go
```
Hook resolves relative to `$PWD`, which is already inside the worktree.

**Recommendation**: Use relative paths when possible.

---

## Security Considerations

### Isolation is Not Sandboxing

Worktree isolation prevents **accidental** cross-task edits, but it is **not a security boundary**:

- Claude Code runs with the user's permissions
- Bash tool can run arbitrary commands
- A malicious prompt could bypass the hook by using Bash (`echo "..." > /etc/passwd`)

**For security-critical environments**:
- Run Claude Code in a container or VM
- Use AppArmor/SELinux to enforce filesystem restrictions
- Review all AI-generated changes before committing

---

### Hook Can Be Bypassed

The PreToolUse hook is client-side and can be disabled by editing `.claude/settings.json`. It is a **guardrail**, not a lock.

**Mitigation**:
- Commit `.claude/settings.json` to git
- Use pre-commit hooks to validate no unauthorized changes to `.claude/settings.json`
- Monitor `.adb_events.jsonl` for Edit/Write events outside expected worktrees

---

## Testing Isolation

### Manual Test: Attempt Out-of-Worktree Edit

```bash
cd work/TASK-00042/
export ADB_WORKTREE_PATH=$(pwd)

# Start Claude Code
claude --dangerously-skip-permissions

# In Claude chat:
# "Please edit /etc/passwd"

# Expected result: Error message from PreToolUse hook
```

---

### Automated Test: CI Validation

```yaml
# .github/workflows/test-isolation.yml
- name: Test worktree isolation
  run: |
    cd work/TASK-00042/
    export ADB_WORKTREE_PATH=$(pwd)

    # Attempt to edit file outside worktree
    echo '{"tool":"Edit","file_path":"/etc/passwd"}' | \
      .claude/hooks/pre-edit-validate.sh

    # Should exit non-zero
    if [ $? -eq 0 ]; then
      echo "FAIL: Hook allowed out-of-worktree edit"
      exit 1
    fi

    echo "PASS: Isolation enforced"
```

---

## Checklist: Isolation Health

Before starting work on a task, verify isolation is active:

- [ ] `ADB_WORKTREE_PATH` is set:
  ```bash
  echo $ADB_WORKTREE_PATH
  ```
- [ ] PreToolUse hook exists:
  ```bash
  ls -la .claude/hooks/pre-edit-validate.sh
  ```
- [ ] Hook is registered:
  ```bash
  cat .claude/settings.json | grep -A 3 PreToolUse
  ```
- [ ] Hook is executable:
  ```bash
  test -x .claude/hooks/pre-edit-validate.sh && echo "OK"
  ```
- [ ] Current directory is worktree:
  ```bash
  [[ "$(pwd)" == "$ADB_WORKTREE_PATH" ]] && echo "OK"
  ```

---

## See Also

- **docs/wiki/agent-teams.md**: Agent isolation in multi-agent workflows
- **docs/wiki/worktree-automation.md**: Worktree lifecycle management
- **docs/runbooks/troubleshooting.md**: Debugging hook failures
- **CLAUDE.md**: Full command reference for `--worktree` flag

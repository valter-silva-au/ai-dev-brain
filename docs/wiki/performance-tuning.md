# Performance Tuning

This guide covers performance optimization techniques for AI Dev Brain (adb) and Claude Code integration, including fast mode, simple output, and large codebase strategies.

---

## Fast Mode

### Overview

Fast mode enables faster output generation from Claude Opus 4.6 **without changing models**. It uses the same model with optimized streaming parameters.

**Key points**:
- Same model (Opus 4.6)
- Same quality of responses
- Faster token generation (approximately 2-3x)
- Can be toggled on/off mid-conversation

### Enabling Fast Mode

**In Claude Code TUI**:
```
/fast
```

Toggle off:
```
/fast
```

**Via adb exec**:
```bash
adb exec claude -p "/fast run tests and report results"
```

### When to Use Fast Mode

✅ **Use fast mode for**:
- CI/CD pipelines (faster builds)
- Quick iterations during development
- Repetitive tasks (testing, linting, formatting)
- Large batch operations (refactoring multiple files)

❌ **Avoid fast mode for**:
- Complex architectural decisions (take your time)
- Security-critical code reviews (thoroughness > speed)
- First-time problem analysis (need full context)

### Performance Impact

Typical improvements with fast mode enabled:

| Operation | Normal Mode | Fast Mode | Speedup |
|-----------|-------------|-----------|---------|
| Run test suite | 45s | 18s | 2.5x |
| Code review (single file) | 30s | 12s | 2.5x |
| Refactor function | 60s | 25s | 2.4x |
| Generate documentation | 90s | 35s | 2.6x |

*Benchmarks on mid-size Go codebase (50k LOC) with Opus 4.6.*

---

## Simple Output Mode

### Overview

The `CLAUDE_CODE_SIMPLE` environment variable reduces output verbosity by:
- Hiding thinking/reasoning steps
- Omitting metadata in responses
- Suppressing progress indicators
- Producing terser explanations

**Key points**:
- No change in model or quality
- Faster parsing in CI logs
- Less token usage
- Can combine with fast mode

### Enabling Simple Output

**Environment variable** (recommended for CI):
```bash
export CLAUDE_CODE_SIMPLE=1
adb exec claude -p "run tests"
```

**CLI flag** (per-command):
```bash
adb exec claude --simple -p "fix linting errors"
```

**In .taskconfig** (persistent):
```yaml
defaults:
  ai: claude
  simple_output: true
```

### Output Comparison

**Normal output**:
```
Let me run the test suite and analyze the results...

Running tests...
✓ 142 tests passed
✗ 3 tests failed

Let me examine the failures:

1. TestTaskManager_CreateTask failed because...
   This is likely due to...
   I recommend...

2. TestBacklog_FilterTasks failed because...
   ...
```

**Simple output** (`CLAUDE_CODE_SIMPLE=1`):
```
Tests: 142 passed, 3 failed

Failures:
1. TestTaskManager_CreateTask: nil pointer in CreateTask
   Fix: add nil check for config parameter

2. TestBacklog_FilterTasks: index out of range
   Fix: validate filter.Status before accessing

3. ...
```

### When to Use Simple Output

✅ **Use simple output for**:
- CI/CD pipelines (cleaner logs)
- Automated workflows (easier parsing)
- Quick verification tasks
- High-frequency operations

❌ **Avoid simple output for**:
- Learning new codebases (need explanations)
- Debugging complex issues (need reasoning)
- Design discussions (need rationale)

---

## Combining Fast Mode + Simple Output

For maximum performance, combine both optimizations:

**CI pipeline example**:
```yaml
env:
  CLAUDE_CODE_SIMPLE: 1
run: |
  adb exec claude -p "/fast run full test suite and fix failures"
```

**Interactive development**:
```bash
export CLAUDE_CODE_SIMPLE=1
adb exec claude -p "/fast review uncommitted changes"
```

**Expected speedup**: 3-4x faster than normal mode for typical operations.

---

## Large Codebase Strategies

### Problem: Context Window Limits

Claude Opus 4.6 has a 1 million token context window, but large codebases can exceed this when scanning deeply nested directories.

### Solution 1: Scope Commands Explicitly

**Narrow file scope**:
```bash
adb exec claude -p "review internal/core/taskmanager.go for edge cases"
```

**Narrow directory scope**:
```bash
adb exec claude -p "
  Focus only on internal/core/*.go files.
  Check for nil pointer dereferences.
"
```

### Solution 2: Use .claudeignore

Exclude large directories from Claude's context:

```
# .claudeignore
vendor/
node_modules/
.git/
*.min.js
*.generated.go
testdata/
fixtures/
```

**After adding patterns**, Claude Code respects these exclusions automatically.

### Solution 3: Split Operations

For operations that touch many files, split into batches:

```bash
# Batch 1: Core package
adb exec claude -p "refactor internal/core/*.go to use new error handling"

# Batch 2: Storage package
adb exec claude -p "refactor internal/storage/*.go to use new error handling"

# Batch 3: Integration package
adb exec claude -p "refactor internal/integration/*.go to use new error handling"
```

### Solution 4: Close Unused Files

In Claude Code TUI, explicitly close files not relevant to the current task:

```
/close internal/observability/*.go
```

Or reset the conversation to clear context:

```
/reset
```

---

## Memory Management (Claude Code v2.1.50+)

### Memory Leak Fixes

Claude Code v2.1.50 includes fixes for memory leaks in:

1. **LSP server** (language server processes)
2. **Task output panel** (buffer accumulation)
3. **Agent team context** (multi-agent coordination)
4. **Tool call history** (never pruned in older versions)

**Impact**: Sessions can run 4+ hours without restart (previously required restart every 2 hours).

### Monitoring Memory Usage

**Linux/macOS**:
```bash
# Watch Claude process memory
watch 'ps aux | grep claude | awk "{print \$2, \$4, \$11}"'
```

**Expected memory usage**:
- Fresh session: ~500MB - 1GB
- After 2 hours: ~2GB - 3GB (v2.1.50+)
- After 4 hours: ~3GB - 4GB (v2.1.50+)

**Warning signs** (indicates older version or other issue):
- Memory >8GB in any session
- Memory growth >2GB/hour
- Tool responses slowing down over time

**Fix**: Upgrade to Claude Code v2.1.50+:
```bash
curl -fsSL https://install.anthropic.com | sh
```

---

## Agent Team Performance

### Issue: Slow Agent Responses

Multi-agent workflows (e.g., `@team-lead` delegating to `@analyst`) can be slow if:

1. **Memory leaks** (fixed in v2.1.50)
2. **Deeply nested delegation** (3+ levels)
3. **Large context per agent**

### Solution 1: Limit Team Depth

**Avoid**:
```
@team-lead -> @scrum-master -> @analyst -> @researcher
```

**Prefer**:
```
@team-lead -> @analyst
@team-lead -> @scrum-master
```

Maximum recommended depth: 2 levels.

### Solution 2: Use Fast Mode for Teams

```
/fast @team-lead plan this sprint
```

Agent responses will be generated faster without sacrificing quality.

### Solution 3: Restart Long Sessions

For sessions >2 hours with heavy agent usage (Claude Code < v2.1.50):

```bash
# Save work, restart Claude Code
adb resume TASK-XXXXX
```

Sessions automatically resume conversation history.

---

## Worktree Isolation Performance

### Issue: Slow Tool Validation

The `PreToolUse` hook validates every Read/Edit/Write operation to enforce worktree boundaries. For large worktrees, path resolution can be slow.

### Solution: Use Relative Paths

**Slow** (absolute path lookup):
```
@edit /home/user/work/TASK-00042/internal/core/taskmanager.go
```

**Fast** (relative path):
```
@edit internal/core/taskmanager.go
```

Claude Code resolves relative paths from the worktree root without syscalls.

### Solution: Disable Isolation for Trusted Tasks

For tasks where isolation is not critical, disable the `PreToolUse` hook:

```json
// work/TASK-XXXXX/.claude/settings.json
{
  "hooks": {
    "PreToolUse": null
  }
}
```

**Warning**: Only do this for tasks that don't require strict boundary enforcement.

---

## CI/CD Performance Optimizations

### 1. Cache adb Installation

**GitHub Actions**:
```yaml
- uses: actions/cache@v4
  with:
    path: ~/.adb
    key: ${{ runner.os }}-adb-${{ hashFiles('.taskconfig') }}
```

Saves ~30s per job.

### 2. Use Simple Output in CI

```yaml
env:
  CLAUDE_CODE_SIMPLE: 1
```

Reduces log size by 60-70%, speeds up log parsing.

### 3. Set Aggressive Timeouts

```yaml
timeout-minutes: 10  # Fail fast if Claude hangs
```

Prevents stuck jobs from blocking pipelines.

### 4. Limit Context Explicitly

```bash
adb exec claude -p "review only files changed in this PR" --simple
```

Avoids scanning entire codebase in every CI run.

---

## Benchmarking

### Measure adb Command Performance

```bash
# Time a command
time adb exec claude -p "run tests"

# Detailed timing
time adb exec claude --simple -p "/fast run tests"
```

### Measure Claude Code Startup Time

```bash
time claude --version
```

Expected: <1 second for v2.1.50+.

### Measure Tool Call Latency

In Claude Code TUI:
```
/metrics tools
```

Shows average latency per tool (Read, Edit, Write, Bash).

---

## Performance Checklist

Before filing a performance issue, verify:

- [ ] Claude Code version >= v2.1.50 (memory leak fixes)
- [ ] adb version >= v1.7.0 (fast mode, simple output support)
- [ ] `.claudeignore` excludes large directories (`vendor/`, `node_modules/`)
- [ ] Fast mode enabled for repetitive tasks (`/fast`)
- [ ] Simple output enabled for CI (`CLAUDE_CODE_SIMPLE=1`)
- [ ] Commands scoped to relevant files/directories
- [ ] Session duration <4 hours (restart if longer)
- [ ] Agent team depth ≤2 levels
- [ ] Memory usage <8GB per Claude process

---

## Performance Regression Troubleshooting

### Symptom: Tool Calls Taking >10 Seconds

1. **Check Claude Code version**:
   ```bash
   claude --version
   ```
   Upgrade if <v2.1.50.

2. **Check memory usage**:
   ```bash
   ps aux | grep claude
   ```
   If >8GB, restart session.

3. **Enable fast mode**:
   ```
   /fast
   ```

4. **Reduce context**:
   ```
   /close **/*.test.go
   /reset
   ```

### Symptom: CI Jobs Timing Out

1. **Add `CLAUDE_CODE_SIMPLE=1`**:
   ```yaml
   env:
     CLAUDE_CODE_SIMPLE: 1
   ```

2. **Reduce timeout threshold**:
   ```yaml
   timeout-minutes: 10
   ```

3. **Limit file scope**:
   ```bash
   adb exec claude -p "review only PR diff" --simple
   ```

---

## Advanced: Profiling adb Performance

### Enable Event Logging

All adb operations are logged to `.adb_events.jsonl`. Analyze event timestamps to find bottlenecks:

```bash
# Extract tool call latencies
jq -r 'select(.type == "tool.completed") | [.time, .data.tool, .data.duration_ms] | @tsv' \
  .adb_events.jsonl | sort -k3 -n | tail -20
```

### Profile Claude Code

```bash
# Trace syscalls (Linux)
strace -c -p $(pgrep claude)

# Sample CPU usage (macOS)
sample claude 10 -f profile.txt
```

---

## See Also

- **docs/runbooks/troubleshooting.md**: Memory leak fixes and version compatibility
- **docs/runbooks/ci-integration.md**: CI-specific performance optimizations
- **CLAUDE.md**: Full command reference for `adb exec` and flags

# Troubleshooting Claude Code Integration

This runbook covers common issues when integrating AI Dev Brain (adb) with Claude Code, including memory leaks, version compatibility, and session capture failures.

---

## Memory Leak Issues (Claude Code < v2.1.50)

### Symptoms

- Claude Code becomes sluggish after 2+ hours of use
- High memory usage (>8GB) in Claude processes
- Tool call responses slow down or time out
- LSP server crashes or stops responding
- Agent team coordination breaks down (no responses from delegated agents)
- Task output panel stops updating

### Root Causes (Fixed in v2.1.50)

Claude Code versions prior to v2.1.50 had several memory leaks:

1. **LSP Server Memory Leak**: Language server processes accumulated memory over long sessions
2. **Task Output Panel Leak**: Task output buffers were not garbage collected
3. **Agent Team Context Leak**: Multi-agent conversations retained full context indefinitely
4. **Tool Call History Leak**: Tool use history was never pruned

### Resolution

**Upgrade to Claude Code v2.1.50 or later** for automatic fixes.

For older versions, apply these workarounds:

1. **Restart Claude Code every 2 hours** during long sessions:
   ```bash
   # Save work, exit Claude Code, then restart
   claude --dangerously-skip-permissions --resume
   ```

2. **Use `/reset` command** to clear conversation history before switching contexts:
   ```
   /reset
   ```

3. **Avoid deeply nested agent teams** (limit to 2-3 levels) in long sessions

4. **Monitor memory usage**:
   ```bash
   # Linux/macOS
   ps aux | grep claude | awk '{print $2, $4, $11}'

   # Watch for >8GB resident memory
   ```

### Minimum Recommended Version

**Claude Code >= v2.1.50** is strongly recommended for production use with adb.

Check your version:
```bash
claude --version
```

If using an older version, upgrade via:
```bash
# macOS/Linux
curl -fsSL https://install.anthropic.com | sh

# Or via Homebrew
brew upgrade claude
```

---

## Session Capture Failures

### Symptom: Sessions Not Captured

**Issue**: Sessions end but no captured session appears in `sessions/` directory.

**Checklist**:

1. **Verify SessionEnd hook is installed**:
   ```bash
   # Check user-level settings
   cat ~/.claude/settings.json | grep -A 5 SessionEnd
   ```

   Should show:
   ```json
   "SessionEnd": {
     "command": "bash",
     "args": ["-c", "~/.claude/hooks/adb-session-capture.sh"]
   }
   ```

2. **Verify hook script exists and is executable**:
   ```bash
   ls -la ~/.claude/hooks/adb-session-capture.sh
   # Should be -rwxr-xr-x (executable)
   ```

3. **Reinstall hook if missing**:
   ```bash
   adb sync-claude-user
   ```

4. **Check session capture config**:
   ```bash
   # Look for session_capture section in .taskconfig
   cat .taskconfig | grep -A 5 session_capture
   ```

   Should include:
   ```yaml
   session_capture:
     enabled: true
     min_turns_capture: 3  # Adjust as needed
   ```

5. **Test hook manually**:
   ```bash
   # Simulate SessionEnd metadata
   echo '{"session_id":"test-123","started":"2025-01-15T10:00:00Z"}' | \
     ~/.claude/hooks/adb-session-capture.sh

   # Check for error output
   ```

6. **Verify transcript file permissions**:
   ```bash
   # Claude Code writes transcripts to ~/.claude/projects/...
   ls -la ~/.claude/projects/*/transcript.jsonl
   ```

**Common Cause**: Hook script path incorrect in `settings.json`. The hook must use an absolute path or `~/` prefix.

---

### Symptom: Session Captured with Wrong Task ID

**Issue**: Sessions are captured but not linked to the correct task.

**Root Cause**: `ADB_TASK_ID` environment variable not set in the Claude Code session.

**Resolution**:

1. **For `adb resume` workflows** (recommended), the environment is set automatically:
   ```bash
   adb resume TASK-00042
   # Claude Code launches with ADB_TASK_ID=TASK-00042
   ```

2. **For manual Claude Code launch**, set the environment explicitly:
   ```bash
   cd work/TASK-00042
   export ADB_TASK_ID=TASK-00042
   export ADB_BRANCH=$(git branch --show-current)
   export ADB_WORKTREE_PATH=$(pwd)
   export ADB_TICKET_PATH=/path/to/tickets/TASK-00042
   claude --dangerously-skip-permissions --resume
   ```

3. **Verify environment in Claude session**:
   ```
   @terminal echo $ADB_TASK_ID
   ```

---

## Worktree Hook Failures

### Symptom: PreToolUse Hook Not Blocking Invalid Edits

**Issue**: Claude Code can edit files outside the worktree despite hook configuration.

**Checklist**:

1. **Verify hook is registered in worktree's `.claude/settings.json`**:
   ```bash
   cat work/TASK-XXXXX/.claude/settings.json | grep -A 10 PreToolUse
   ```

2. **Check hook script exists in worktree**:
   ```bash
   ls -la work/TASK-XXXXX/.claude/hooks/pre-edit-validate.sh
   ```

3. **Test hook manually**:
   ```bash
   cd work/TASK-XXXXX
   echo '{"tool":"Edit","file_path":"/etc/passwd"}' | \
     ./.claude/hooks/pre-edit-validate.sh

   # Should exit 1 with error message
   ```

4. **Verify `ADB_WORKTREE_PATH` is set**:
   ```bash
   echo $ADB_WORKTREE_PATH
   # Should match work/TASK-XXXXX path
   ```

**Common Cause**: Hook installed at user level (`~/.claude/`) instead of worktree level (`.claude/`). Worktree-specific hooks must live inside the worktree.

---

## Version Mismatch Issues

### Symptom: Features Not Available

**Issue**: Commands like `adb session capture` or `adb exec claude -p` fail with "unknown flag" or "unknown command".

**Root Cause**: Mismatch between adb version and Claude Code version.

**Resolution**:

1. **Check adb version**:
   ```bash
   adb version
   # Should be >= v1.6.0 for session capture
   # Should be >= v1.7.0 for -p/--print flag
   ```

2. **Check Claude Code version**:
   ```bash
   claude --version
   # Should be >= v2.1.50 for memory leak fixes
   ```

3. **Upgrade both if needed**:
   ```bash
   # Upgrade adb
   brew upgrade adb  # or download latest release

   # Upgrade Claude Code
   brew upgrade claude
   ```

4. **Re-sync Claude templates after upgrade**:
   ```bash
   adb sync-claude-user --mcp
   ```

---

## Agent Team Coordination Failures

### Symptom: Agent Does Not Respond to Delegation

**Issue**: Team lead delegates to an agent (e.g., `@analyst`) but gets no response or a generic fallback response.

**Checklist**:

1. **Verify agent definition exists**:
   ```bash
   ls -la ~/.claude/agents/analyst.json
   # Or for project-specific agents
   ls -la .claude/agents/analyst.json
   ```

2. **Check agent registration in settings**:
   ```bash
   cat ~/.claude/settings.json | grep -A 5 '"analyst"'
   ```

3. **Verify memory leak not degrading performance** (Claude Code < v2.1.50):
   - Restart Claude Code if session > 2 hours
   - Check memory usage of Claude processes

4. **Use explicit agent invocation**:
   ```
   @analyst please analyze the requirements in notes.md
   ```

5. **Check agent logs** (if available):
   ```bash
   cat ~/.claude/logs/agent-teams.log
   ```

**Common Cause**: Long-running sessions in Claude Code < v2.1.50 cause agent context to become corrupted. **Upgrade to v2.1.50+** for permanent fix.

---

## Performance Degradation

### Symptom: Slow Tool Responses

**Issue**: Read, Edit, Write tools take >10 seconds to complete.

**Checklist**:

1. **Check Claude Code version** (should be >= v2.1.50 for memory fixes)

2. **Enable fast mode**:
   ```
   /fast
   ```

3. **Use simple output mode**:
   ```bash
   export CLAUDE_CODE_SIMPLE=1
   adb exec claude --simple -p "run tests"
   ```

4. **Reduce context window** for large codebases:
   - Close unrelated files in editor
   - Use `--file` or `--dir` flags to limit scope
   - Avoid deep directory scans with `/directory`

5. **Check LSP server status** (Claude Code >= v2.1.50):
   ```
   /lsp status
   ```

6. **Restart Claude Code** if session > 2 hours (older versions)

---

## CI/CD Integration Issues

### Symptom: Claude Code Hangs in CI Pipeline

**Issue**: `adb exec claude` never completes when run in GitHub Actions or CI.

**Resolution**: Use `-p` / `--print` flag for non-interactive execution:

```yaml
- name: Run adb exec claude in CI
  run: |
    adb exec claude -p "review this PR and suggest improvements"
```

**See also**: `docs/runbooks/ci-integration.md` for full CI setup guide.

---

## Emergency Recovery

### When All Else Fails

1. **Kill all Claude processes**:
   ```bash
   pkill -9 claude
   ```

2. **Clear Claude cache** (loses conversation history):
   ```bash
   rm -rf ~/.claude/cache
   rm -rf ~/.claude/projects/*/transcript.jsonl
   ```

3. **Reinstall Claude Code**:
   ```bash
   curl -fsSL https://install.anthropic.com | sh
   ```

4. **Reinstall adb Claude templates**:
   ```bash
   adb sync-claude-user --mcp
   ```

5. **Resume task with clean slate**:
   ```bash
   adb resume TASK-XXXXX
   # No --resume flag = fresh conversation
   ```

---

## Getting Help

If issues persist after following this guide:

1. **Check GitHub issues**: https://github.com/valter-silva-au/ai-dev-brain/issues
2. **Report version info**:
   ```bash
   adb version
   claude --version
   cat .taskconfig | grep -E "(version|ai)"
   ```
3. **Attach relevant logs**:
   - `.adb_events.jsonl` (last 50 lines)
   - Claude Code logs (if available)
   - Session capture errors from hook execution

---

## Version Compatibility Matrix

| adb Version | Claude Code Version | Session Capture | Memory Leak Fixes | Agent Teams | Worktree Hooks |
|-------------|---------------------|-----------------|-------------------|-------------|----------------|
| v1.6.0      | v2.0.x              | ✅              | ❌                | ⚠️          | ✅             |
| v1.6.0      | v2.1.50+            | ✅              | ✅                | ✅          | ✅             |
| v1.7.0      | v2.1.50+            | ✅              | ✅                | ✅          | ✅             |

**Legend**:
- ✅ Fully supported
- ⚠️ Supported with workarounds (restart every 2 hours)
- ❌ Not supported or unreliable

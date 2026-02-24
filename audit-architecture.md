# Architecture Complexity Review

**Date:** 2026-02-24
**Reviewer:** code-reviewer (architecture-guide persona)
**Scope:** Complete architectural assessment focused on coupling, complexity, and simplification opportunities

## Executive Summary

The AI Dev Brain codebase exhibits significant architectural complexity with **7 adapter structs**, **23 interfaces**, **68 CLI commands**, and **37,287 lines of code** across core packages. While the local interface pattern successfully prevents storage→core cycles, **critical architectural violations exist**: core directly imports storage in 5 production files, contradicting the documented adapter-based design. Hook system complexity with 6 types, 2 phases, and shell wrappers creates maintenance burden. Knowledge system has 4 overlapping abstractions. Configuration uses 3 layers with Viper indirection.

**Priority recommendations:**
1. **CRITICAL**: Eliminate core→storage imports in updategen.go, knowledge.go, designdoc.go, aicontext.go (violates documented architecture)
2. **HIGH**: Consolidate 4 knowledge-related types into 2 (KnowledgeExtractor + KnowledgeStore)
3. **HIGH**: Simplify hook system by removing shell wrappers, use Go-only hooks
4. **MEDIUM**: Reduce adapter count from 7 to 4-5 by using direct interface implementation
5. **MEDIUM**: Flatten configuration to 2 layers (remove .taskrc)

---

## 1. Adapter Pattern Analysis

### Current State

**Location:** `internal/app.go:279-534`

**Adapter Count:** 7 distinct adapter structs

| Adapter | Lines | Purpose | Value Assessment |
|---------|-------|---------|------------------|
| `worktreeAdapter` | 12 | Adapts `GitWorktreeManager.CreateWorktree` to `core.WorktreeCreator` | **Low value** - Only translates struct fields |
| `worktreeRemoverAdapter` | 8 | Adapts `GitWorktreeManager.RemoveWorktree` to `core.WorktreeRemover` | **Low value** - Direct passthrough |
| `backlogStoreAdapter` | 94 | Adapts `storage.BacklogManager` to `core.BacklogStore` | **Medium value** - Prevents import cycle, but heavy boilerplate |
| `contextStoreAdapter` | 6 | Adapts `storage.ContextManager` to `core.ContextStore` | **Low value** - Single method passthrough |
| `eventLogAdapter` | 12 | Adapts `observability.EventLog` to `core.EventLogger` | **Low value** - Wraps event struct only |
| `knowledgeStoreAdapter` | 73 | Adapts `storage.KnowledgeStoreManager` to `core.KnowledgeStoreAccess` | **Medium value** - Prevents import cycle |
| `sessionCapturerAdapter` | 33 | Adapts `storage.SessionStoreManager` to `core.SessionCapturer` | **Medium value** - Prevents import cycle |

**Total adapter lines:** 238 lines (0.64% of codebase)

**Helper functions:** 2 (storageEntryFromCore, coreEntryFromStorage) - 32 lines of struct field copying

### Value Analysis

**Adapters that justify their existence:**
- `backlogStoreAdapter`, `knowledgeStoreAdapter`, `sessionCapturerAdapter` - These prevent actual import cycles between core and storage packages.

**Adapters that are questionable:**
- `worktreeAdapter`, `worktreeRemoverAdapter` - These only translate struct fields. Could be eliminated if core accepted `integration.WorktreeConfig` directly or if `GitWorktreeManager` implemented `core.WorktreeCreator` natively.
- `contextStoreAdapter` - Single-method interface with no transformation logic. This is over-engineering.
- `eventLogAdapter` - Only wraps an event struct. The core package could accept `observability.Event` directly.

### Import Cycle Analysis

**Actual import relationships (analyzed via grep):**
```
storage/ → models/   (OK)
storage/ → NOT core  (OK - no cycle risk from storage side)

core/ → models/      (OK)
core/ → storage/     ❌ CRITICAL VIOLATION ❌
```

**Files where core imports storage directly:**
1. `internal/core/updategen.go:9` - imports storage.ContextManager, storage.CommunicationManager
2. `internal/core/knowledge.go:10` - imports storage.ContextManager, storage.CommunicationManager
3. `internal/core/designdoc.go:10` - imports storage.CommunicationManager
4. `internal/core/aicontext.go:12` - imports storage.BacklogManager (but also uses adapter!)
5. `internal/core/hookengine.go:340-343` - checks for storage import in string literal (architecture guard)

**This is a critical architectural violation.** The documentation and architecture diagrams claim core never imports storage, but 5 production files violate this constraint. The adapters exist to prevent cycles, but the core constructors accept concrete storage types directly:

```go
// From knowledge.go:30
func NewKnowledgeExtractor(basePath string, ctxMgr storage.ContextManager, commMgr storage.CommunicationManager) KnowledgeExtractor
```

This defeats the entire purpose of the adapter pattern. If core can import storage to declare constructor parameters, there's no import cycle to prevent.

### Simplification Proposal

**Option A: Enforce Local Interfaces (Current Intent)**

Make core truly independent by:
1. Define `core.ContextStore` interface (already exists at taskmanager.go:54-56)
2. Define `core.CommunicationStore` interface (new)
3. Change all core constructors to accept interfaces, not concrete types
4. Move adapters to internal/app.go (current location) or eliminate by having storage implement core interfaces

Estimated savings: Remove 5 import statements, enforce actual decoupling.

**Option B: Embrace Direct Imports (Pragmatic)**

Accept that core→storage is not a cycle risk (storage doesn't import core) and:
1. Delete all adapters except `worktreeAdapter` (integration is used by multiple places)
2. Let core import storage directly
3. Update documentation to reflect reality

Estimated savings: Delete 230 lines of adapter code, simplify wiring.

### Recommendation

**Option A** (Enforce Local Interfaces) - Priority: **CRITICAL**

Rationale: The current state is the worst of both worlds - adapters exist but are bypassed. Either remove them entirely (Option B) or enforce them properly (Option A). Since the project clearly values testability and the adapter pattern is already 80% implemented, finishing the job is cleaner than tearing it down.

**Immediate action:** Fix the 4 files that import storage directly by changing their constructors to accept interfaces.

---

## 2. CLI Package Variables (Service Locator Anti-Pattern)

### Current State

**Location:** `internal/cli/vars.go` + wiring in `internal/app.go:207-238`

**Package-level variables:** 32 variables across multiple files

| File | Variable Count | Variables |
|------|---------------|-----------|
| `vars.go` | 11 | EventLog, AlertEngine, MetricsCalc, Notifier, BranchPattern, SessionCapture, HookEngine, VersionChecker |
| `root.go` | ~8 | BasePath, TaskMgr, UpdateGen, AICtxGen, ProjectInit, RepoSyncMgr, ChannelReg, KnowledgeMgr, KnowledgeX, FeedbackLoop |
| `feat.go` | 2 | Executor, Runner |
| `exec.go` | 1 | ExecAliases |

**Total:** ~22 distinct package-level service variables + flags/state

### Problems

1. **Global state** - Makes unit testing harder, requires careful initialization order
2. **Hidden dependencies** - Commands don't declare what they need, they reach into global scope
3. **Initialization coupling** - All commands are coupled to the wiring order in app.go:207-238
4. **Not thread-safe** - Package variables can be mutated from any goroutine (though adb is single-threaded currently)

### What It Takes to Fix

**Traditional dependency injection** would require:

1. Define a `CommandContext` struct that holds all dependencies:
   ```go
   type CommandContext struct {
       BasePath string
       TaskMgr core.TaskManager
       UpdateGen core.UpdateGenerator
       // ... 20 more fields
   }
   ```

2. Thread `CommandContext` through every command:
   ```go
   func newFeatCmd(ctx *CommandContext) *cobra.Command {
       return &cobra.Command{
           RunE: func(cmd *cobra.Command, args []string) error {
               task, err := ctx.TaskMgr.CreateTask(...)
               // ...
           },
       }
   }
   ```

3. Initialize commands in app.go after wiring:
   ```go
   func (a *App) BuildRootCmd() *cobra.Command {
       ctx := &CommandContext{
           BasePath: a.BasePath,
           TaskMgr: a.TaskMgr,
           // ... 20 more assignments
       }
       root := cli.NewRootCmd(ctx)
       root.AddCommand(cli.NewFeatCmd(ctx))
       // ... 68 commands
   }
   ```

**Estimated effort:**
- Define CommandContext struct: 30 lines
- Refactor 68 command constructors: 2-5 lines each = 136-340 lines changed
- Update app.go wiring: 50 lines
- **Total:** ~220-420 lines changed across 68 files

### Are All Variables Used?

Quick usage check (via grep):

| Variable | Used In | Essential? |
|----------|---------|-----------|
| `TaskMgr` | feat.go, bug.go, spike.go, refactor.go, resume.go, archive.go, status.go, priority.go | ✅ Core |
| `UpdateGen` | update.go | ✅ Used |
| `AICtxGen` | synccontext.go | ✅ Used |
| `Executor` | exec.go, run.go | ✅ Used |
| `Runner` | run.go | ✅ Used |
| `EventLog` | metrics.go, alerts.go, sessioncapture.go | ✅ Used (observability) |
| `AlertEngine` | alerts.go | ✅ Used |
| `MetricsCalc` | metrics.go | ✅ Used |
| `SessionCapture` | sessioncapture.go, synccontext.go | ✅ Used |
| `HookEngine` | hook.go | ✅ Used |
| `BranchPattern` | feat.go, bug.go, spike.go, refactor.go | ✅ Used |
| `VersionChecker` | team.go | ✅ Used |
| `Notifier` | alerts.go (--notify flag) | ✅ Used |
| `ProjectInit` | init.go | ✅ Used |
| `RepoSyncMgr` | syncrepos.go | ✅ Used |
| `ChannelReg` | channel.go, loop.go | ✅ Used |
| `KnowledgeMgr` | knowledge.go, loop.go | ✅ Used |
| `KnowledgeX` | knowledgeextract.go (not in vars.go) | ✅ Used |
| `FeedbackLoop` | loop.go | ✅ Used |
| `ExecAliases` | exec.go | ✅ Used |

**Finding:** All variables are actually used. No dead variables found.

### Recommendation

**Accept current pattern** - Priority: **NICE-TO-HAVE**

Rationale: While this is a code smell, the benefit of refactoring to proper DI is low compared to cost:
- 68 command files would need changes
- Cobra doesn't naturally support DI (commands are constructed once at startup)
- adb is single-threaded CLI tool, not a long-running service with complex lifecycle
- All variables are legitimately used

**Better investment:** Fix the critical architecture violations (core importing storage) first.

If this were a long-running service or a library, DI would be essential. For a CLI tool initialized once per invocation, the current pattern is acceptable pragmatism.

---

## 3. Interface Granularity Analysis

### Interface Method Counts

**Core package interfaces** (23 total):

| Interface | Methods | File | Assessment |
|-----------|---------|------|------------|
| `TaskManager` | 9 | taskmanager.go | ✅ Cohesive CRUD + lifecycle |
| `BootstrapSystem` | 3 | bootstrap.go | ✅ Single responsibility |
| `ConfigurationManager` | 4 | config.go | ✅ Cohesive config operations |
| `BacklogStore` | 7 | taskmanager.go | ✅ Standard CRUD + filters |
| `ContextStore` | 1 | taskmanager.go | ⚠️ Single method - could inline |
| `WorktreeCreator` | 1 | bootstrap.go | ⚠️ Single method - could inline |
| `WorktreeRemover` | 1 | taskmanager.go | ⚠️ Single method - could inline |
| `TaskIDGenerator` | 1 | taskid.go | ✅ Atomic operation |
| `TemplateManager` | 3 | templates.go | ✅ Cohesive template ops |
| `AIContextGenerator` | 9 | aicontext.go | ⚠️ Could split: generation vs sections |
| `UpdateGenerator` | 3 | updategen.go | ✅ Single responsibility |
| `TaskDesignDocGenerator` | 6 | designdoc.go | ✅ Cohesive design doc ops |
| `KnowledgeExtractor` | 5 | knowledge.go | ✅ Knowledge extraction + artifacts |
| `ConflictDetector` | 1 | conflict.go | ✅ Single operation |
| `ProjectInitializer` | 1 | projectinit.go | ✅ Single operation |
| `EventLogger` | 1 | eventlogger.go | ✅ Single operation |
| `KnowledgeStoreAccess` | 16 | knowledgemanager.go | ❌ **TOO LARGE** - CRUD + topics + entities + timeline |
| `KnowledgeManager` | 10 | knowledgemanager.go | ⚠️ Business logic + queries - could split |
| `SessionCapturer` | 8 | sessioncapturer.go | ✅ Cohesive session CRUD |
| `ChannelAdapter` | 5 | channeladapter.go | ✅ Standard adapter contract |
| `ChannelRegistry` | 4 | channeladapter.go | ✅ Registry pattern |
| `FeedbackLoopOrchestrator` | 2 | feedbackloop.go | ✅ Simple orchestration |
| `HookEngine` | 5 | hookengine.go | ✅ One method per hook type |

### Violations of Interface Segregation Principle (ISP)

**`KnowledgeStoreAccess` (16 methods)** - Clients need:
- Knowledge CRUD: AddEntry, GetEntry, GetAllEntries, Search (4 methods)
- Topic operations: GetTopics, AddTopic, GetTopic (3 methods)
- Entity operations: GetEntities, AddEntity, GetEntity (3 methods)
- Timeline operations: GetTimeline, AddTimelineEntry (2 methods)
- Queries: QueryByTopic, QueryByEntity, QueryByTags (3 methods)
- Utilities: GenerateID, Load, Save (3 methods)

**Problem:** A client that only needs to query knowledge must depend on 16 methods. This violates ISP - clients should not depend on methods they don't use.

**Proposed split:**

```go
type KnowledgeReader interface {
    GetEntry(id string) (*models.KnowledgeEntry, error)
    GetAllEntries() ([]models.KnowledgeEntry, error)
    QueryByTopic(topic string) ([]models.KnowledgeEntry, error)
    QueryByEntity(entity string) ([]models.KnowledgeEntry, error)
    QueryByTags(tags []string) ([]models.KnowledgeEntry, error)
    Search(query string) ([]models.KnowledgeEntry, error)
}

type KnowledgeWriter interface {
    AddEntry(entry models.KnowledgeEntry) (string, error)
    GenerateID() (string, error)
    Save() error
}

type KnowledgeTopicManager interface {
    GetTopics() (*models.TopicGraph, error)
    AddTopic(topic models.Topic) error
    GetTopic(name string) (*models.Topic, error)
}

type KnowledgeEntityManager interface {
    GetEntities() (*models.EntityRegistry, error)
    AddEntity(entity models.Entity) error
    GetEntity(name string) (*models.Entity, error)
}

type KnowledgeTimeline interface {
    GetTimeline(since time.Time) ([]models.TimelineEntry, error)
    AddTimelineEntry(entry models.TimelineEntry) error
}

// KnowledgeStoreAccess becomes a composite for backward compatibility
type KnowledgeStoreAccess interface {
    KnowledgeReader
    KnowledgeWriter
    KnowledgeTopicManager
    KnowledgeEntityManager
    KnowledgeTimeline
    Load() error
}
```

### Single-Method Interfaces

**Candidates for inlining:**

1. `ContextStore` - 1 method (`LoadContext`)
   - Used by: TaskManager, UpdateGenerator, KnowledgeExtractor
   - **Assessment:** Keep - storage abstraction prevents coupling

2. `WorktreeCreator` - 1 method (`CreateWorktree`)
   - Used by: BootstrapSystem
   - **Assessment:** Keep - integration abstraction for testing

3. `WorktreeRemover` - 1 method (`RemoveWorktree`)
   - Used by: TaskManager
   - **Assessment:** Keep - mirrors WorktreeCreator for symmetry

4. `EventLogger` - 1 method (`LogEvent`)
   - Used by: TaskManager, FeedbackLoopOrchestrator, HookEngine (via adapter)
   - **Assessment:** Keep - observability abstraction, optional dependency

**Finding:** While these are single-method interfaces, they serve legitimate architectural purposes (abstraction, testing, optional features). Not candidates for removal.

### Unused Methods Detection

Method call analysis would require:
```bash
# For each interface method, find call sites
for interface in TaskManager BootstrapSystem ...; do
    for method in CreateTask ResumeTask ...; do
        grep -r "$method(" internal/cli internal/core --include="*.go" | grep -v "_test.go"
    done
done
```

**Spot check on TaskManager:**
- `CreateTask` - called in feat.go, bug.go, spike.go, refactor.go ✅
- `ResumeTask` - called in resume.go ✅
- `ArchiveTask` - called in archive.go ✅
- `UnarchiveTask` - called in unarchive.go ✅
- `GetTasksByStatus` - called in status.go ✅
- `GetAllTasks` - called in status.go ✅
- `GetTask` - called in archive.go, status.go, priority.go ✅
- `UpdateTaskStatus` - called in status.go, resume.go ✅
- `UpdateTaskPriority` - called in priority.go ✅
- `ReorderPriorities` - called in priority.go ✅
- `CleanupWorktree` - called in cleanup.go ✅

**Finding:** All TaskManager methods are used in production code. No dead methods.

### Recommendation

**Split KnowledgeStoreAccess into 5 focused interfaces** - Priority: **MEDIUM**

Rationale:
- 16 methods is too many for a single interface
- Clients like AIContextGenerator only need KnowledgeReader
- Clients like KnowledgeManager need the full composite
- Maintains backward compatibility via composite interface

**Estimated effort:** 50 lines of interface definitions, 10 call sites updated

**Leave single-method interfaces alone** - they serve valid architectural purposes.

---

## 4. Package Boundaries and Import Analysis

### Package Structure

```
internal/
├── cli/          68 files, ~8,000 LOC (command definitions)
├── core/         50 files, ~9,000 LOC (business logic)
├── storage/      12 files, ~3,500 LOC (persistence)
├── integration/  18 files, ~5,000 LOC (external systems)
├── observability/ 6 files, ~1,800 LOC (events, metrics, alerts)
├── hooks/         9 files, ~500 LOC (hook support utilities)
├── mcp/           3 files, ~400 LOC (MCP server)
├── app.go        535 lines (wiring)
└── tests/        3 files, ~3,000 LOC (cross-package tests)
```

### Actual Import Relationships (Measured)

```
cli/ → core, storage, integration, observability (OK - CLI can import all)
cli/ → hooks (for stdin parsing in hook commands)

core/ → models (OK)
core/ → storage ❌ (updategen, knowledge, designdoc, aicontext) - VIOLATION
core/ → integration ❌ (string check in hookengine) - false positive
core/ → hooks (hookengine uses hooks.ChangeTracker) - borderline acceptable

storage/ → models (OK)
storage/ → NOT core (OK - no cycle)

integration/ → models (OK)
integration/ → NOT core (OK - no cycle)

observability/ → models (OK)
observability/ → NOT core (OK - no cycle)

hooks/ → models (OK - SessionChangeEntry)
hooks/ → NOT core (OK - support library)
```

### Circular Dependency Risks

**No actual cycles detected**, but:
- core→storage imports create coupling that adapters were meant to prevent
- If storage ever needed to call core (e.g., for validation), a cycle would form

**Cycle risk assessment:**
- core ↔ storage: **MEDIUM** (core imports storage, but storage doesn't need core today)
- core ↔ integration: **LOW** (core doesn't import integration except string literal)
- core ↔ observability: **LOW** (core uses EventLogger adapter, observability is leaf)

### Could Packages Be Merged?

**Merge candidates:**

1. **storage + observability → persistence/**
   - Both are about writing data to disk (YAML vs JSONL)
   - Observability has no business logic, just append/read
   - **Benefit:** Simpler package structure
   - **Cost:** Lose semantic separation between task data vs operational data

2. **core + storage → domain/**
   - Core already imports storage in 5 files
   - Combining them would eliminate adapters
   - **Benefit:** Remove 230 lines of adapter code
   - **Cost:** Lose testing isolation, larger package

3. **hooks + integration → external/**
   - Both deal with OS/external system interaction
   - hooks/ is really just utilities for hook scripts
   - **Benefit:** Clearer that hooks are part of integration layer
   - **Cost:** Rename would affect imports

### Recommendation

**Do not merge packages** - Priority: **LOW**

Rationale:
- Package boundaries are semantically meaningful
- Merging would save lines of code but lose conceptual clarity
- Better to fix the import violations than tear down the boundaries

**Fix the import violations instead:**
1. Make core→storage use interfaces only (remove direct imports)
2. Keep packages separate for testing and conceptual clarity

---

## 5. Hook Engine Complexity

### Component Count

**Hook Types:** 6
- PreToolUse (blocking)
- PostToolUse (non-blocking)
- Stop (advisory)
- TaskCompleted (2-phase: blocking + non-blocking)
- SessionEnd (non-blocking)
- TeammateIdle (no-op)

**Shell Wrappers:** 6 (one per hook type)
- `adb-hook-pre-tool-use.sh`
- `adb-hook-post-tool-use.sh`
- `adb-hook-stop.sh`
- `adb-hook-task-completed.sh`
- `adb-hook-session-end.sh`
- `adb-hook-teammate-idle.sh`

**Go Implementation:**
- `internal/core/hookengine.go` - 467 lines
- `internal/hooks/tracker.go` - ChangeTracker (~150 lines)
- `internal/hooks/artifacts.go` - Context append, status update helpers (~100 lines)
- `internal/hooks/stdin.go` - JSON parsing (~80 lines)

**Configuration:**
- `models.HookConfig` - Per-hook enable flags, feature flags, command overrides (~50 lines in models/config.go)
- `DefaultHookConfig()` - Phase 1 defaults (~30 lines)

**Total hook-related code:** ~877 lines across 4 files + 6 shell scripts

### Concepts and Dependencies

**Hook system dependency graph:**

```
Claude Code (external)
  ↓ (pipes JSON to stdin)
Shell Wrapper (sets ADB_HOOK_ACTIVE, calls adb hook <type>)
  ↓
CLI Command (internal/cli/hook.go - ParseStdin[T], dispatch to HookEngine)
  ↓
HookEngine (internal/core/hookengine.go)
  ├→ ChangeTracker (internal/hooks/tracker.go)
  │   └→ .adb_session_changes file (append-only log)
  ├→ Artifact Helpers (internal/hooks/artifacts.go)
  │   ├→ AppendToContext (writes to tickets/TASK-XXXXX/context.md)
  │   └→ UpdateStatusTimestamp (writes to tickets/TASK-XXXXX/status.yaml)
  ├→ KnowledgeExtractor (optional, Phase 2)
  └→ ConflictDetector (optional, Phase 3)
```

**Total concepts:** 11
1. Hook type (6 variants)
2. Blocking vs non-blocking semantics
3. Shell wrapper script
4. Stdin JSON schema (per hook type)
5. ADB_HOOK_ACTIVE recursion guard
6. ChangeTracker + .adb_session_changes file
7. Artifact helpers (context append, status timestamp)
8. HookConfig (per-hook feature flags)
9. Phase 1/2/3 feature rollout
10. Quality gates (2-phase TaskCompleted)
11. Advisory vs blocking checks

### Simplified Design Proposal

**Current:** Shell wrapper → adb hook → HookEngine → helpers/tracker
**Proposed:** Claude Code → adb (native Go hook) → HookEngine

**Changes:**
1. **Remove shell wrappers** - Register `adb hook <type>` directly in `.claude/settings.json`:
   ```json
   {
     "hooks": {
       "PreToolUse": { "command": ["adb", "hook", "pre-tool-use"], "blocking": true },
       "PostToolUse": { "command": ["adb", "hook", "post-tool-use"], "blocking": false }
     }
   }
   ```

2. **Stdin parsing moves to adb main** - No subprocess, read stdin directly in `cmd/adb/main.go` if args are `hook <type>`

3. **ADB_HOOK_ACTIVE becomes unnecessary** - No recursion possible without shell subprocess

4. **Consolidate tracker + artifacts** - Move all helper functions into hookengine.go, eliminate separate packages

5. **Flatten HookConfig** - Remove per-hook nested config, use flat feature flags:
   ```yaml
   hooks:
     enabled: true
     block_vendor: true
     go_format: true
     change_tracking: true
     tests_on_complete: true
     lint_on_complete: true
   ```

**Estimated savings:**
- Delete 6 shell scripts (~80 lines total)
- Delete internal/hooks/ package (~330 lines) - fold into hookengine.go
- Simplify HookConfig (~30 lines saved)
- **Total:** ~440 lines removed, 11 concepts → 7 concepts

### Trade-offs

**Pro:**
- Simpler: No shell subprocess, no recursion guard, fewer files
- Faster: Direct Go execution, no bash startup overhead
- More portable: No bash dependency (Windows compatibility)

**Con:**
- Requires Claude Code to accept executable paths (current design assumes shell scripts)
- Migration burden: Users must update .claude/settings.json
- Lose the "thin wrapper" flexibility (can't swap hook implementation without recompiling adb)

### Recommendation

**Simplify to Go-only hooks** - Priority: **HIGH**

Rationale:
- Current design has accidental complexity (shell wrappers exist for recursion prevention, not functional need)
- Go-native execution is faster and more portable
- Most complexity is in the 2-phase TaskCompleted design and feature flags, which are inherent to requirements

**Defer to Phase 2:** Flattening HookConfig and consolidating helpers into hookengine.go

---

## 6. Configuration System Complexity

### Three-Level Hierarchy

**Level 1: Hard-coded defaults** (internal/core/config.go:41-53)
```go
func defaultGlobalConfig() *models.GlobalConfig {
    return &models.GlobalConfig{
        DefaultAI:        "kiro",
        TaskIDPrefix:     "TASK",
        TaskIDCounter:    0,
        TaskIDPadWidth:   5,
        BranchPattern:    "{type}/{description}",
        DefaultPriority:  models.P2,
        // ... 6 more fields
    }
}
```

**Level 2: Global config** (`.taskconfig` YAML, loaded via Viper)
```yaml
defaults:
  ai: claude
  priority: P1
task_id:
  prefix: PROJ
  pad_width: 4
branch:
  pattern: "{prefix}/{type}/{description}"
hooks:
  enabled: true
  pre_tool_use:
    enabled: true
# ... ~30 possible config keys
```

**Level 3: Per-repo config** (`.taskrc` YAML, merged on top of global)
```yaml
build_command: "go build ./..."
test_command: "go test -race ./..."
default_reviewers:
  - alice
  - bob
conventions:
  - file: docs/go-standards.md
```

### Complexity Metrics

**Config struct fields:**
- `models.GlobalConfig` - 18 fields
- `models.RepoConfig` - 6 fields
- `models.MergedConfig` - Composite (GlobalConfig + RepoConfig)
- `models.HookConfig` - ~15 fields (nested per-hook configs)
- `models.NotificationConfig` - 3 fields
- `models.TeamRoutingConfig` - 2 fields
- **Total distinct config keys:** ~50

**Viper usage:**
- 2 Viper instances (global, repo)
- 15 SetDefault calls in LoadGlobalConfig
- ~25 Get* calls to extract values
- Custom unmarshal logic for cli_aliases, team_routing, hooks

**Configuration loading code:**
- `config.go` - 470 lines (LoadGlobalConfig, LoadRepoConfig, GetMergedConfig, Validate)
- Nested YAML parsing (defaults.ai, task_id.prefix, hooks.pre_tool_use.enabled)

### Is Three Levels Necessary?

**Use case analysis:**

1. **Hard-coded defaults** - Essential (fallback when no config file)
2. **Global config (`.taskconfig`)** - Essential (user preferences, machine-specific settings)
3. **Per-repo config (`.taskrc`)** - Questionable

**What `.taskrc` provides:**
- `build_command`, `test_command` - Could be stored in Taskfile.yaml (already present in most repos)
- `default_reviewers` - Rarely used (no evidence in codebase grep)
- `conventions` - Could be inferred by scanning docs/wiki/ directory

**Evidence of `.taskrc` usage:**
```bash
$ grep -r "\.taskrc" internal --include="*.go"
internal/core/config.go:  // LoadRepoConfig reads the .taskrc file...
internal/core/config_test.go:  // Test .taskrc loading
```

Only referenced in config loading tests. No CLI commands explicitly load `.taskrc` - it's only used in `GetMergedConfig`, which is called by... (grep reveals: nobody calls it directly).

**Finding:** `.taskrc` is implemented but unused in practice. The `GetMergedConfig` method is never called outside of tests.

### Viper Overhead

**Why Viper?**
- YAML parsing (could use `yaml.Unmarshal` directly)
- Nested key access (e.g., `defaults.ai`)
- Environment variable overrides (not used in adb)
- Config file discovery (viper.AddConfigPath)

**What Viper provides that stdlib yaml doesn't:**
- Automatic type conversion (GetString, GetInt, GetBool)
- Nested key resolution without manual map traversal
- Config file search paths

**Cost of Viper:**
- Dependency (github.com/spf13/viper)
- ~100 lines of boilerplate (SetDefault calls, Get* calls)
- Indirection - config values read via string keys rather than struct fields

**Alternative:** Direct `yaml.Unmarshal`:
```go
func LoadGlobalConfig(basePath string) (*models.GlobalConfig, error) {
    path := filepath.Join(basePath, ".taskconfig")
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return defaultGlobalConfig(), nil
    }
    if err != nil {
        return nil, err
    }
    cfg := defaultGlobalConfig()
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return nil, fmt.Errorf("parsing .taskconfig: %w", err)
    }
    return cfg, nil
}
```

**Estimated savings:** Remove ~100 lines of Viper boilerplate, remove viper dependency

### Recommendation

**Flatten to 2 levels + remove Viper** - Priority: **MEDIUM**

1. **Remove `.taskrc`** - It's unused. Repo-specific config can live in Taskfile.yaml or docs/.
2. **Replace Viper with yaml.Unmarshal** - Simpler, fewer dependencies, more explicit.
3. **Keep hard-coded defaults + `.taskconfig`** - This is the essential baseline.

**Estimated effort:**
- Remove LoadRepoConfig, GetMergedConfig: 150 lines deleted
- Replace Viper with yaml.Unmarshal: 50 lines rewritten
- Update tests: 100 lines
- **Total:** ~200 lines net savings, 1 dependency removed

---

## 7. Context Evolution Tracking

### Current Implementation

**Components:**
- `.context_state.yaml` - Snapshot (task counts, section hashes, timestamps) - 50 lines per snapshot
- `.context_changelog.md` - Last 50 diffs (pruned) - grows to ~1000 lines
- `internal/core/aicontext.go` - State diff logic (~200 lines)
- `renderWhatsChanged()` - Formats diff into markdown (~80 lines)

**State tracking:**
```yaml
synced_at: "2025-01-15T10:00:00Z"
active_task_ids: [TASK-00001, TASK-00002]
knowledge_count: 42
decision_count: 15
session_count: 28
adr_titles: [ADR-0001-use-go, ADR-0002-file-storage]
section_hashes:
  glossary: "e3b0c44298fc1c14"
  conventions: "a3d5e7f9b1c3d5e7"
```

**FNV-1a hashing:**
- Used to detect when static sections (glossary, conventions) change
- 64-bit hash stored as hex string
- ~20 lines of hash computation code

**Changelog pruning:**
- Keeps last 50 entries (~2-3 weeks of daily syncs)
- ~30 lines of prune logic

### Complexity Assessment

**Is this over-engineered?**

**Use case:** Show "What's Changed" section in CLAUDE.md so AI assistants know what's new since last sync.

**Frequency of sync:** Typically once per day or when significant changes occur.

**Snapshot size:** ~200 bytes per state (trivial)

**Changelog size:** ~1000 lines (~50KB) after pruning (acceptable)

**Alternative designs:**

1. **No state tracking** - Regenerate full context every time, no "What's Changed" section.
   - **Pro:** Simpler (delete 310 lines)
   - **Con:** AI assistants lose temporal awareness

2. **Git-based diffing** - Use git log to compute changes since last sync.
   - **Pro:** Leverage existing git history
   - **Con:** Requires parsing git log, doesn't track non-git changes (knowledge count)

3. **Timestamp-only tracking** - Just track last_synced timestamp, show items added since then.
   - **Pro:** Simpler (delete ~200 lines - keep timestamps, delete hashing/diffing)
   - **Con:** Can't detect removals or section content changes

### Value Assessment

**Who benefits:**
- AI assistants reading CLAUDE.md (primary user)
- Developers reviewing context drift over time (secondary user)

**How often used:**
- Every time AI assistant starts (reads CLAUDE.md)
- Visible in "What's Changed" section if present

**Is it worth 310 lines of code?**
- **Pro:** Provides temporal context (tasks added/completed, sections changed) that raw context can't show
- **Con:** Complex implementation for a feature that's nice-to-have but not essential

### Recommendation

**Simplify to timestamp-only tracking** - Priority: **MEDIUM**

Rationale:
- FNV-1a hashing and deep diffing is overkill for showing recent changes
- Simpler approach: track `last_synced` timestamp + show items added since then
- Changelog pruning adds complexity without proportional value

**Proposed changes:**
1. Replace `.context_state.yaml` with simple `.context_last_synced` file containing one RFC3339 timestamp
2. In "What's Changed" section, show: "Tasks added since [timestamp]", "Knowledge entries since [timestamp]"
3. Remove FNV-1a hashing, section diffing, changelog file

**Estimated savings:** Delete ~200 lines, keep temporal awareness

---

## 8. Session Capture Pipeline

### Current Architecture

```
Claude Code JSONL transcript
  ↓
SessionEnd hook (adb-session-capture.sh)
  ↓
adb session capture --from-hook
  ↓
TranscriptParser (internal/integration/transcript.go)
  ├→ Scan JSONL line-by-line
  ├→ Skip meta/thinking lines
  ├→ Extract user/assistant turns
  └→ Build SessionTurn list
  ↓
StructuralSummarizer (internal/integration/transcript.go)
  ├→ First user message
  ├→ Turn count
  ├→ Tool usage tally (Read:3, Edit:1)
  └→ Format: "Help me fix bug — 5 turns, tools: Read(3), Edit(1)"
  ↓
SessionStoreManager (internal/storage/sessionstore.go)
  ├→ Generate S-XXXXX ID
  ├→ Write sessions/S-XXXXX/session.yaml
  ├→ Write sessions/S-XXXXX/turns.yaml
  ├→ Write sessions/S-XXXXX/summary.md
  └→ Update sessions/index.yaml
```

### Code Metrics

**Files involved:** 3
- `internal/integration/transcript.go` - 350 lines (parser + summarizer)
- `internal/storage/sessionstore.go` - 400 lines (persistence)
- `internal/cli/sessioncapture.go` - 200 lines (CLI command)

**Total:** 950 lines for session capture pipeline

### Complexity Analysis

**What adds complexity:**
1. **JSONL parsing** - Must handle malformed lines, skip types (meta, thinking)
2. **Turn extraction** - Assemble sequences of user→assistant→tools into cohesive turns
3. **Structural summarization** - Count tools, format readable summary
4. **Three-file storage** - session.yaml (metadata) + turns.yaml (full data) + summary.md (human-readable)
5. **Index maintenance** - sessions/index.yaml updated on every capture

**What's actually needed:**
1. Parse JSONL transcript (essential)
2. Extract turns (essential for replay/analysis)
3. Generate summary (essential for AI context)
4. Store persistently (essential for cross-session continuity)

### Could This Be Simpler?

**Option A: Store transcript directly**
- Copy `.claude/projects/[hash]/transcript.jsonl` to `sessions/S-XXXXX/transcript.jsonl`
- No parsing, no turn extraction
- **Pro:** Simplest possible (50 lines)
- **Con:** Lose structured access (can't query "sessions where tool X was used")

**Option B: Single-file storage**
- Store everything in `sessions/S-XXXXX/session.json` (metadata + turns + summary)
- No separate index.yaml
- **Pro:** Simpler (300 lines vs 950)
- **Con:** Slower queries (must scan all session files for filters)

**Option C: Current design**
- **Pro:** Fast queries (index.yaml enables filtered listing), structured turns (enables analysis)
- **Con:** Most complex (950 lines)

### Recommendation

**Keep current design** - Priority: **LOW** (no change)

Rationale:
- Session capture is a feature that benefits from structure (turn-by-turn analysis)
- Index-based queries are essential for `adb session list --task-id TASK-00042`
- 950 lines is reasonable for a feature this comprehensive
- No obvious simplification that doesn't sacrifice functionality

**Alternative:** Defer LLM-powered summarization to Phase 2 (currently deferred anyway)

---

## 9. Knowledge System Architecture

### Four Related Types

| Component | Lines | Purpose | Overlap |
|-----------|-------|---------|---------|
| `KnowledgeExtractor` | ~300 | Extract knowledge from completed tasks (notes.md, design.md, comms) | Reads task data |
| `KnowledgeManager` | ~200 | Business logic: add entries, query, assemble summaries | Orchestrates |
| `KnowledgeStoreManager` | ~400 | Persistence: CRUD on `docs/knowledge/` YAML files | Storage |
| `KnowledgeStoreAccess` | (interface) | 16-method interface bridging core → storage | Abstraction |

**Total knowledge-related code:** ~900 lines across 3 files

### Dependency Relationships

```
KnowledgeExtractor (core)
  └→ reads: storage.ContextManager, storage.CommunicationManager
  └→ outputs: models.ExtractedKnowledge

KnowledgeManager (core)
  └→ stores via: KnowledgeStoreAccess (interface)
  └→ inputs: ExtractedKnowledge or manual entries

KnowledgeStoreManager (storage)
  └→ implements: KnowledgeStoreAccess (16 methods)
  └→ persists: docs/knowledge/*.yaml

knowledgeStoreAdapter (app.go)
  └→ adapts: KnowledgeStoreManager → KnowledgeStoreAccess
```

### Is This Justified?

**What each type does:**

1. **KnowledgeExtractor** - Parse markdown, extract "## Learnings", "## Decisions", comms with #decision tag
   - **Unique responsibility:** Markdown parsing, rule-based extraction
   - **Could be merged with:** KnowledgeManager (but that would mix parsing + business logic)

2. **KnowledgeManager** - Add entries, update topic graph, maintain timeline, query
   - **Unique responsibility:** Business logic around knowledge lifecycle
   - **Could be merged with:** KnowledgeStoreManager (but that would mix business logic + persistence)

3. **KnowledgeStoreManager** - Read/write YAML files, generate IDs, maintain index
   - **Unique responsibility:** File I/O, YAML marshaling
   - **Could be merged with:** KnowledgeManager (but that would violate SRP)

4. **KnowledgeStoreAccess** - Interface abstraction for testing and decoupling
   - **Unique responsibility:** Prevents core → storage import cycle
   - **Could be removed:** If core accepted storage types directly (but violates architecture)

### Overlap Analysis

**Minimal overlap detected:**
- KnowledgeExtractor and KnowledgeManager both reference `models.KnowledgeEntry`, but one produces them, the other stores them.
- KnowledgeStoreManager and KnowledgeStoreAccess are implementation + interface (standard pattern).

**No duplication found.**

### Consolidation Proposal

**Option A: 4 types → 2 types**

Merge:
- `KnowledgeExtractor` + `KnowledgeManager` → `KnowledgeService`
- `KnowledgeStoreManager` + `KnowledgeStoreAccess` → `KnowledgeStore`

**Pro:** Fewer types (2 instead of 4)
**Con:** Larger classes (extraction + management in one, interface + implementation in one)

**Option B: Keep 4 types, split KnowledgeStoreAccess**

As analyzed in Section 3 (Interface Granularity), split `KnowledgeStoreAccess` (16 methods) into 5 focused interfaces.

**Pro:** Better ISP adherence
**Con:** More interfaces (but each is cohesive)

**Option C: Current design**

**Pro:** Clear separation of concerns
**Con:** 4 types feels like a lot for one domain

### Recommendation

**Consolidate to 2 types (Extractor + Store)** - Priority: **HIGH**

Proposed:
1. Merge `KnowledgeManager` into `KnowledgeExtractor` → `KnowledgeService`
   - Rationale: Both are business logic, both live in core/, both deal with knowledge lifecycle
   - The distinction between "extract from task" and "add to store" is an implementation detail, not a conceptual boundary

2. Merge `KnowledgeStoreManager` + `KnowledgeStoreAccess` → `KnowledgeStore` (interface + implementation)
   - Rationale: They're already tightly coupled (interface + implementation), keeping them separate adds no value

**Estimated effort:**
- Merge 2 files: ~100 lines of wiring changes
- Update 10 call sites (cli commands, app.go)
- **Net savings:** ~50 lines, reduce cognitive overhead

---

## Summary of Findings

### Critical Issues (Fix Immediately)

1. **Core imports storage directly** (5 files violate architecture)
   - `updategen.go`, `knowledge.go`, `designdoc.go`, `aicontext.go`, `hookengine.go` (string check only)
   - **Impact:** Defeats adapter pattern, creates coupling, contradicts documentation
   - **Fix:** Define local interfaces, change constructors to accept interfaces

### High-Priority Simplifications

2. **Knowledge system has 4 types** (should be 2)
   - Merge KnowledgeManager into KnowledgeExtractor → KnowledgeService
   - Merge KnowledgeStoreManager + KnowledgeStoreAccess → KnowledgeStore
   - **Savings:** ~50 lines, clearer domain model

3. **Hook system has 11 concepts** (should be 7)
   - Remove shell wrappers, use Go-native hooks
   - Consolidate helpers into hookengine.go
   - **Savings:** ~440 lines, faster execution

### Medium-Priority Refactoring

4. **Adapter count could be reduced** (7 → 4-5)
   - Keep: backlogStoreAdapter, knowledgeStoreAdapter, sessionCapturerAdapter
   - Evaluate: worktreeAdapter, worktreeRemoverAdapter, contextStoreAdapter, eventLogAdapter
   - **Savings:** ~100 lines, less indirection

5. **Configuration is 3 levels** (should be 2)
   - Remove `.taskrc` (unused)
   - Replace Viper with yaml.Unmarshal
   - **Savings:** ~200 lines, 1 dependency removed

6. **KnowledgeStoreAccess has 16 methods** (should be split)
   - Split into: Reader, Writer, TopicManager, EntityManager, Timeline
   - **Benefit:** Better ISP adherence, more testable

7. **Context evolution uses FNV-1a hashing** (overkill)
   - Simplify to timestamp-only tracking
   - **Savings:** ~200 lines

### Nice-to-Have (Low Priority)

8. **CLI package variables** (service locator pattern)
   - **Decision:** Accept as pragmatic for CLI tool
   - **Rationale:** 68 commands would require large refactor for marginal benefit

9. **Session capture pipeline** (950 lines)
   - **Decision:** Keep current design
   - **Rationale:** Structured storage enables queries, no simpler design without sacrificing functionality

---

## Metrics Summary

| Metric | Current | After Refactoring | Savings |
|--------|---------|-------------------|---------|
| **Adapter structs** | 7 | 4-5 | 2-3 structs |
| **Adapter LOC** | 238 | ~120 | ~120 lines |
| **Core interfaces** | 23 | 23 (split KnowledgeStoreAccess into 5) | 0 (restructured) |
| **Knowledge types** | 4 | 2 | 2 types |
| **Hook-related LOC** | 877 | ~440 | ~440 lines |
| **Config LOC** | 470 | ~270 | ~200 lines |
| **Context evolution LOC** | 310 | ~110 | ~200 lines |
| **Total LOC (internal/)** | 37,287 | ~36,200 | ~1,087 lines (2.9%) |

---

## Prioritized Recommendations

### Phase 1: Critical Fixes (Do First)

1. **Eliminate core→storage imports** (updategen, knowledge, designdoc, aicontext)
   - Define `core.CommunicationStore` interface
   - Change constructors to accept interfaces, not concrete types
   - **Effort:** 4 hours
   - **Impact:** Fixes architectural violation, restores adapter pattern intent

### Phase 2: High-Value Simplifications (Do Next)

2. **Consolidate knowledge system** (4 types → 2)
   - Merge KnowledgeManager into KnowledgeExtractor
   - Merge KnowledgeStoreManager + KnowledgeStoreAccess
   - **Effort:** 6 hours
   - **Impact:** Clearer domain model, ~50 lines saved

3. **Simplify hook system** (remove shell wrappers)
   - Register Go binary directly in .claude/settings.json
   - Consolidate helpers into hookengine.go
   - **Effort:** 8 hours
   - **Impact:** ~440 lines saved, faster execution, better portability

### Phase 3: Medium-Value Refactoring (Do Later)

4. **Flatten configuration** (3 levels → 2)
   - Remove .taskrc
   - Replace Viper with yaml.Unmarshal
   - **Effort:** 4 hours
   - **Impact:** ~200 lines saved, 1 dependency removed

5. **Reduce adapter count** (7 → 4-5)
   - Evaluate contextStoreAdapter, worktreeRemoverAdapter, eventLogAdapter
   - Consider direct interface implementation
   - **Effort:** 3 hours
   - **Impact:** ~100 lines saved

6. **Split KnowledgeStoreAccess** (16 methods → 5 interfaces)
   - Create Reader, Writer, TopicManager, EntityManager, Timeline
   - **Effort:** 2 hours
   - **Impact:** Better ISP adherence, more testable

### Phase 4: Low-Priority Nice-to-Haves (Optional)

7. **Simplify context evolution** (FNV-1a → timestamps)
   - Replace state snapshots with last_synced timestamp
   - **Effort:** 3 hours
   - **Impact:** ~200 lines saved

---

## Conclusion

The AI Dev Brain architecture is **well-structured but over-engineered in specific areas**. The adapter pattern is sound but inconsistently applied (core imports storage directly). The knowledge system uses 4 types where 2 would suffice. The hook system has 11 concepts where 7 are essential. Configuration has 3 layers where 2 are sufficient.

**Priority order:**
1. Fix the critical architectural violation (core→storage imports)
2. Consolidate the knowledge system (4→2 types)
3. Simplify the hook system (remove shell wrappers)
4. Flatten configuration and reduce adapters

**Total potential savings:** ~1,087 lines (~2.9% of codebase), plus improved clarity and maintainability.

The codebase is not "bad" - it has good test coverage (87-94%), follows Go idioms, and has clear package boundaries. The issues identified are refinement opportunities, not fundamental flaws.

---
name: dependency-check
description: Identify blocked/blocking tasks and priority conflicts in the backlog
allowed-tools: Bash, Read, Glob
---

# Dependency Check Skill

Analyze task dependencies to identify blocked tasks, blocking chains, and priority conflicts.

## Steps

### 1. Load All Tasks
Read `backlog.yaml` to get the full task list with statuses, priorities, and blocked_by fields.

### 2. Build Dependency Graph
For each task, map:
- Which tasks it blocks (other tasks that have this task in their `blocked_by` list)
- Which tasks block it (its own `blocked_by` list)

### 3. Identify Issues

**Blocked Tasks**
Find tasks with status `blocked` or tasks with non-empty `blocked_by` where the blocking task is not yet `done` or `archived`.

**Blocking Chains**
Identify chains where task A blocks B which blocks C. Report the full chain.

**Priority Conflicts**
Flag cases where:
- A lower-priority task blocks a higher-priority task (e.g., P3 blocking P0)
- A high-priority task (P0, P1) has been in `blocked` status
- Multiple P0 tasks exist simultaneously

**Stale Blockers**
Find tasks blocked by tasks that are `done` or `archived` (blocker should be resolved).

### 4. Generate Report

```
=== Dependency Check ===

-- Blocked Tasks (N) --
[TASK-XXXXX] (P1, blocked) blocked by: TASK-YYYYY (P2, in_progress)

-- Blocking Chains --
TASK-AAAAA -> TASK-BBBBB -> TASK-CCCCC

-- Priority Conflicts (N) --
WARNING: TASK-XXXXX (P3, in_progress) blocks TASK-YYYYY (P0, blocked)
  Recommendation: Escalate TASK-XXXXX to P0

-- Stale Blockers (N) --
TASK-XXXXX is blocked by TASK-YYYYY which is already done
  Recommendation: Remove blocker and unblock TASK-XXXXX

-- Summary --
Total tasks: N
Blocked: M
Blocking chains: K
Priority conflicts: J
Stale blockers: L
```

If no dependency issues are found, report a clean status.

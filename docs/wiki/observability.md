# Observability

## Overview

The observability subsystem provides structured event logging, on-demand metrics derivation, and threshold-based alerting for AI Dev Brain. It operates on an append-only JSONL event log and requires no external services. The design follows adb's core philosophy: single-binary CLI, file-based storage, zero external infrastructure.

The subsystem lives in `internal/observability/` and consists of three interfaces: `EventLog`, `MetricsCalculator`, and `AlertEngine`. All three are optional -- if the event log file cannot be opened, observability is disabled gracefully without affecting core functionality.

## Key Decisions

- **JSONL event log format**: Append-only, one JSON object per line in `.adb_events.jsonl`. Chosen over SQLite (too heavy, requires CGo) and YAML (not line-oriented, not grep-friendly). See ADR-0001.
- **slog (stdlib) for structured logging**: Go's standard library `log/slog` provides JSON output, structured attributes, and log levels without external dependencies. Preferred over zerolog/zap. See ADR-0001.
- **File-based metrics**: Derived on-demand by scanning the event log rather than maintained in a separate store. `adb metrics` reads and aggregates; no database or background process. See ADR-0001.
- **Per-worktree task-context.md**: Generated in `.claude/rules/task-context.md` inside each worktree during bootstrap and resume. Claude Code loads this automatically, providing zero-infrastructure context survival. See ADR-0001.
- **Phased rollout**: Phase 1 (config-only, zero dependencies) through Phase 4 (web dashboard, optional). Currently between Phase 1 and Phase 2. See ADR-0001.

## Event Types

| Type | Trigger | Data Fields |
|------|---------|-------------|
| `task.created` | New task bootstrapped | `task_id`, `type`, `branch` |
| `task.completed` | Task status set to done | `task_id` |
| `task.status_changed` | Any status transition | `task_id`, `old_status`, `new_status` |
| `agent.session_started` | AI agent begins session | `task_id`, `agent` |
| `knowledge.extracted` | Knowledge extracted on archive | `task_id`, `learnings_count`, `decisions_count` |

## Alert Conditions

| Condition | Severity | Default Threshold |
|-----------|----------|-------------------|
| `task_blocked_too_long` | High | 24 hours |
| `task_stale` | Medium | 3 days |
| `review_too_long` | Medium | 5 days |
| `backlog_too_large` | Low | 10 tasks |

Thresholds are configurable via `.taskconfig` under `notifications.alerts`.

## Gotchas

- The JSONL file grows with usage. Lumberjack rotation is configured for 10MB max size with 5 backups and 30-day retention.
- Multiple agents writing to the same event log simultaneously could interleave lines. Append-only writes are atomic for reasonable line lengths, but file locking may be needed if issues arise.
- Derived metrics are only as complete as the events logged. If event logging is not wired into all code paths, metrics will undercount.

## Related

- ADR-0001: Observability Infrastructure for AI Dev Brain
- `internal/observability/eventlog.go` -- EventLog implementation
- `internal/observability/metrics.go` -- MetricsCalculator implementation
- `internal/observability/alerting.go` -- AlertEngine implementation

---
*Sources: TASK-00024 (ADR-0001)*

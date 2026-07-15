package observability

// Canonical adb event schema. Every event adb emits is one of these EventType
// constants, written as a single JSONL line to <ADB_HOME>/.events.jsonl (see
// internal/app.go). Consumers — metrics.go, alerting.go, `adb events`, the
// VS Code webview — rely on this set being the authoritative contract. When a
// new event is added it MUST be:
//
//  1. Declared here (or in eventlog.go's task.* / issue.* block for locality)
//  2. Added to KnownEventTypes below
//  3. Covered by TestKnownEventTypes_CoversEmittedSet in schema_test.go
//
// Payload keys (Event.Data map) per type:
//
//	task.created           task_id, title, type, status, priority
//	task.status_changed    task_id, old_status, new_status
//	task.archived          task_id, archived_at, archived_dir
//	task.unarchived        task_id, unarchived_at
//	task.priority_changed  task_id, old_priority, new_priority
//	task.deleted           task_id, deleted_at
//	task.completed         task_id                                (reserved)
//	worktree.created       task_id, path                          (reserved)
//	worktree.removed       task_id, path
//	knowledge.extracted    task_id, kind, path                    (reserved)
//	agent.session_started  task_id, worktree, bin, args
//	agent.session_active   task_id, worktree, activity
//	agent.session_ended    task_id, worktree, bin, args, error?
//	issue.synced           task_id, repo, provider, action, reason
//	issue.conflict         task_id, repo, provider, action?, reason?, error?
//	issue.skipped          task_id, repo
//	stage.advanced         initiative_id, from, to, overridden
//	stage.override         initiative_id, from, to, reason
//	config.task_context_synced  task_id, trigger
//	serena.effectiveness_recorded  verdict, score, used_for, beat, friction, task_id?
//
// The five task.* and agent.* consts marked as emissions in
// internal/core/taskmanager.go + internal/cli/task_runwith.go were
// previously undeclared — TestKnownEventTypes_CoversEmittedSet is the guard
// that keeps that drift from re-appearing. See the WS-F sub-plan F1 in
// docs/superpowers/plans/2026-07-01-adb-ws-f-observability-dashboard-chat.md
// for the full drift analysis.
const (
	// task.* — every payload carries "task_id".
	EventTaskArchived        EventType = "task.archived"
	EventTaskUnarchived      EventType = "task.unarchived"
	EventTaskPriorityChanged EventType = "task.priority_changed"
	EventTaskDeleted         EventType = "task.deleted"

	// agent.* — payload carries task_id, worktree, bin, args, and (on end) an
	// optional error. session_started was already declared in eventlog.go;
	// session_ended was emitted but undeclared until F1 — this closes the gap.
	EventAgentSessionEnded EventType = "agent.session_ended"

	// session_active is a lightweight heartbeat a live session may emit to
	// signal it is still working (T4 / ADR part D — same-machine live digest).
	// Payload: task_id, worktree, activity (a short human verb like "editing"
	// or "running tests"). The same-machine session digest (sessiondigest.go)
	// consumes started/active/ended to build "what are other sessions doing".
	EventAgentSessionActive EventType = "agent.session_active"

	// stage.* — founder-playbook stage-gate governance events emitted by
	// core.StageManager.AdvanceStage. stage.advanced is emitted on every
	// advance (clean pass OR human override); its `overridden` payload flag
	// distinguishes the two. stage.override is emitted ADDITIONALLY on an
	// override and carries the human-supplied `reason`. Override is human-only
	// (issue #90 / decision D5).
	EventStageAdvanced EventType = "stage.advanced"
	EventStageOverride EventType = "stage.override"

	// config.* — configuration/context maintenance events. task_context_synced
	// is emitted by core.TaskManager.Resume whenever it (re)renders a task's
	// worktree Tier-0 context (.claude/rules/task-context.md). Payload: task_id,
	// trigger (e.g. "resume"). It was emitted but undeclared until #155 — this
	// closes the drift the schema guard is supposed to catch. (core can't import
	// observability per the import-cycle rule, so the emit site keeps the raw
	// string literal in lock-step with this const — the L400 convention.)
	EventConfigTaskContextSynced EventType = "config.task_context_synced"

	// EventSerenaEffectivenessRecorded is emitted by `adb serena record` when an
	// agent self-reports how effective Serena's code-nav was in a session (#203).
	// Payload: verdict (helped|neutral|hindered|unused), score (1..5), used_for,
	// beat, friction, task_id (optional). The `adb serena report` rollup reads
	// these back from the event log — there is no separate store.
	EventSerenaEffectivenessRecorded EventType = "serena.effectiveness_recorded"
)

// KnownEventTypes is the authoritative set of every EventType adb emits or
// reserves. Ordered by lifecycle: task → worktree → knowledge → agent →
// issue-sync. Consumers can use this to build allowlists / dashboards without
// hard-coding the strings.
var KnownEventTypes = []EventType{
	// task lifecycle
	EventTaskCreated,
	EventTaskCompleted,
	EventTaskStatusChanged,
	EventTaskArchived,
	EventTaskUnarchived,
	EventTaskPriorityChanged,
	EventTaskDeleted,
	// worktree
	EventWorktreeCreated,
	EventWorktreeRemoved,
	// knowledge (reserved)
	EventKnowledgeExtracted,
	// agent session
	EventAgentSessionStarted,
	EventAgentSessionActive,
	EventAgentSessionEnded,
	// issue sync (WS-E)
	EventIssueSynced,
	EventIssueConflict,
	EventIssueSkipped,
	// stage-gate governance (#90)
	EventStageAdvanced,
	EventStageOverride,
	// config/context maintenance (#155)
	EventConfigTaskContextSynced,
	// serena effectiveness telemetry (#203)
	EventSerenaEffectivenessRecorded,
}

// IsKnownEventType reports whether e is part of the documented schema.
// Empty and unknown strings return false.
func IsKnownEventType(e EventType) bool {
	if e == "" {
		return false
	}
	for _, k := range KnownEventTypes {
		if k == e {
			return true
		}
	}
	return false
}

package observability

import "testing"

// TestKnownEventTypes_CoversEmittedSet verifies that every event string adb
// currently emits (task lifecycle in taskmanager.go, agent.session_* in
// task_runwith.go, issue.* in the WS-E issue-sync path) is a known member of
// the canonical schema. If adb starts emitting a new event type, add it to
// KnownEventTypes and this list — otherwise metrics/dashboard consumers will
// silently drop it.
func TestKnownEventTypes_CoversEmittedSet(t *testing.T) {
	emitted := []EventType{
		// task lifecycle (internal/core/taskmanager.go)
		EventTaskCreated,
		EventTaskStatusChanged,
		EventTaskArchived,
		EventTaskUnarchived,
		EventTaskPriorityChanged,
		EventTaskDeleted,
		// worktree (internal/core/taskmanager.go: Create emits created;
		// cleanup/archive/delete emit removed) — #206.
		EventWorktreeCreated,
		EventWorktreeRemoved,
		// agent session (internal/cli/task_runwith.go)
		EventAgentSessionStarted,
		EventAgentSessionEnded,
		// issue-sync decisions (internal/integration/issuesync/syncer.go)
		EventIssueSynced,
		EventIssueConflict,
		EventIssueSkipped,
		// stage-gate governance (internal/core/stagemanager.go: AdvanceStage)
		EventStageAdvanced,
		EventStageOverride,
		// config context refresh (internal/core/taskmanager.go: Resume) — emitted
		// on every resume of a task with a worktree; was undeclared until #155.
		EventConfigTaskContextSynced,
		// serena effectiveness telemetry (`adb serena record`, #203)
		EventSerenaEffectivenessRecorded,
	}
	for _, e := range emitted {
		if !IsKnownEventType(e) {
			t.Errorf("emitted event %q is not in the known schema set", e)
		}
	}
}

// TestKnownEventTypes_IncludesReservedDeclared keeps declared-but-unemitted
// types (task.completed, knowledge.extracted) in the schema set. These are
// wired into metrics.go's switch and are reserved for future emissions;
// removing them would silently drop those code paths. (worktree.created
// graduated to an emitted type in #206.)
func TestKnownEventTypes_IncludesReservedDeclared(t *testing.T) {
	reserved := []EventType{
		EventTaskCompleted,
		EventKnowledgeExtracted,
	}
	for _, e := range reserved {
		if !IsKnownEventType(e) {
			t.Errorf("reserved event %q must remain in the schema set", e)
		}
	}
}

func TestIsKnownEventType_RejectsGarbage(t *testing.T) {
	if IsKnownEventType("totally.made.up") {
		t.Error("expected unknown event type to be rejected")
	}
	if IsKnownEventType("") {
		t.Error("expected empty event type to be rejected")
	}
}

// TestKnownEventTypes_NoDuplicates guards against a copy-paste that adds the
// same const twice into KnownEventTypes.
func TestKnownEventTypes_NoDuplicates(t *testing.T) {
	seen := make(map[EventType]int, len(KnownEventTypes))
	for _, e := range KnownEventTypes {
		seen[e]++
	}
	for e, n := range seen {
		if n > 1 {
			t.Errorf("duplicate entry in KnownEventTypes: %q appears %d times", e, n)
		}
	}
}

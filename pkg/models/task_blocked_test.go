package models

import "testing"

// TestIsBlocked_ParityAcrossMigration pins that IsBlocked() gives the SAME
// answer before and after BlockedBy is folded onto the generic edge model
// (issue #110): a dependency expressed as a legacy BlockedBy entry and the same
// dependency expressed as a depends_on link both count as blocked.
func TestIsBlocked_ParityAcrossMigration(t *testing.T) {
	cases := []struct {
		name string
		task Task
		want bool
	}{
		{"legacy blocked_by", Task{Status: TaskStatusBacklog, BlockedBy: []string{"TASK-2"}}, true},
		{"migrated depends_on", Task{Status: TaskStatusBacklog, Links: []Link{{Type: EdgeDependsOn, Target: "TASK-2"}}}, true},
		{"status blocked, no deps", Task{Status: TaskStatusBlocked}, true},
		{"no deps at all", Task{Status: TaskStatusBacklog}, false},
		// A non-dependency link (relates_to) does NOT block.
		{"relates_to only", Task{Status: TaskStatusBacklog, Links: []Link{{Type: EdgeRelatesTo, Target: "TASK-2"}}}, false},
		// A `blocks` edge means the task blocks OTHERS — it is not itself blocked.
		{"blocks edge only", Task{Status: TaskStatusBacklog, Links: []Link{{Type: EdgeBlocks, Target: "TASK-2"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.task.IsBlocked(); got != tc.want {
				t.Errorf("IsBlocked()=%v want %v", got, tc.want)
			}
		})
	}
}

// TestMigrateBlockedByToLinks_Basic folds a two-entry BlockedBy onto depends_on
// links and clears BlockedBy.
func TestMigrateBlockedByToLinks_Basic(t *testing.T) {
	task := Task{ID: "TASK-1", BlockedBy: []string{"TASK-2", "TASK-3"}}
	if !task.MigrateBlockedByToLinks() {
		t.Fatal("expected migration to report a change")
	}
	if len(task.BlockedBy) != 0 {
		t.Errorf("BlockedBy not cleared: %v", task.BlockedBy)
	}
	deps := task.DependsOn()
	if len(deps) != 2 || deps[0] != "TASK-2" || deps[1] != "TASK-3" {
		t.Errorf("DependsOn()=%v want [TASK-2 TASK-3]", deps)
	}
}

// TestMigrateBlockedByToLinks_Dedup does not duplicate a dependency already
// expressed as a depends_on link (a partially-migrated backlog).
func TestMigrateBlockedByToLinks_Dedup(t *testing.T) {
	task := Task{
		ID:        "TASK-1",
		BlockedBy: []string{"TASK-2", "TASK-3"},
		Links:     []Link{{Type: EdgeDependsOn, Target: "TASK-2"}},
	}
	if !task.MigrateBlockedByToLinks() {
		t.Fatal("expected a change (BlockedBy cleared)")
	}
	deps := task.DependsOn()
	if len(deps) != 2 {
		t.Errorf("DependsOn()=%v want exactly 2 (no dup of TASK-2)", deps)
	}
}

// TestMigrateBlockedByToLinks_Idempotent: a second run finds nothing to change.
func TestMigrateBlockedByToLinks_Idempotent(t *testing.T) {
	task := Task{ID: "TASK-1", BlockedBy: []string{"TASK-2"}}
	task.MigrateBlockedByToLinks()
	if task.MigrateBlockedByToLinks() {
		t.Error("second migration should be a no-op")
	}
}

// TestMigrateBlockedByToLinks_NoBlockedBy leaves a task with no BlockedBy
// untouched (returns false).
func TestMigrateBlockedByToLinks_NoBlockedBy(t *testing.T) {
	task := Task{ID: "TASK-1", Links: []Link{{Type: EdgeRelatesTo, Target: "TASK-9"}}}
	if task.MigrateBlockedByToLinks() {
		t.Error("no BlockedBy → no change expected")
	}
	if len(task.Links) != 1 {
		t.Errorf("existing links disturbed: %v", task.Links)
	}
}

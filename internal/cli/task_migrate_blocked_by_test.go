package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// seedBacklogWithBlockedBy seeds a backlog with one task carrying a two-entry
// blocked_by list plus an unrelated task with none, so the migration has
// exactly one row to rewrite.
func seedBacklogWithBlockedBy(t *testing.T) {
	t.Helper()
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load backlog: %v", err)
	}
	backlog.Tasks = []models.Task{
		{
			ID:        "TASK-00001",
			Title:     "blocked-task",
			Type:      models.TaskTypeFeat,
			Status:    models.TaskStatusBacklog,
			Priority:  models.PriorityP2,
			BlockedBy: []string{"TASK-00002", "TASK-00003"},
			Created:   time.Now().UTC(),
			Updated:   time.Now().UTC(),
		},
		{
			ID:       "TASK-00002",
			Title:    "no-deps",
			Type:     models.TaskTypeFeat,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
	}
	if err := App.BacklogManager.Save(backlog); err != nil {
		t.Fatalf("Save backlog: %v", err)
	}
}

func TestMigrateBlockedBy_DryRun(t *testing.T) {
	app, err := internal.NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app
	seedBacklogWithBlockedBy(t)

	cmd := newTaskMigrateBlockedByCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "TASK-00001") || !strings.Contains(out, "would change") {
		t.Errorf("expected TASK-00001 + 'would change', got:\n%s", out)
	}
	// Disk untouched — blocked_by still present.
	backlog, _ := App.BacklogManager.Load()
	if len(backlog.Tasks[0].BlockedBy) != 2 {
		t.Errorf("dry-run mutated disk: BlockedBy=%v", backlog.Tasks[0].BlockedBy)
	}
}

func TestMigrateBlockedBy_Apply(t *testing.T) {
	app, err := internal.NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app
	seedBacklogWithBlockedBy(t)

	cmd := newTaskMigrateBlockedByCmd()
	_ = cmd.Flags().Set("apply", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Migrated 1") {
		t.Errorf("expected 'Migrated 1', got:\n%s", buf.String())
	}

	backlog, _ := App.BacklogManager.Load()
	got := backlog.Tasks[0]
	if len(got.BlockedBy) != 0 {
		t.Errorf("BlockedBy not cleared: %v", got.BlockedBy)
	}
	deps := got.DependsOn()
	if len(deps) != 2 || deps[0] != "TASK-00002" || deps[1] != "TASK-00003" {
		t.Errorf("DependsOn()=%v want [TASK-00002 TASK-00003]", deps)
	}
	// IsBlocked() parity: still blocked after migration.
	if !got.IsBlocked() {
		t.Error("migrated task should still be IsBlocked()")
	}
}

// TestMigrateBlockedBy_Idempotent: a second apply is a no-op.
func TestMigrateBlockedBy_Idempotent(t *testing.T) {
	app, err := internal.NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app
	seedBacklogWithBlockedBy(t)

	first := newTaskMigrateBlockedByCmd()
	_ = first.Flags().Set("apply", "true")
	var b1 bytes.Buffer
	first.SetOut(&b1)
	first.SetErr(&b1)
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	second := newTaskMigrateBlockedByCmd()
	_ = second.Flags().Set("apply", "true")
	var b2 bytes.Buffer
	second.SetOut(&b2)
	second.SetErr(&b2)
	if err := second.Execute(); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !strings.Contains(b2.String(), "No blocked_by entries need migrating") {
		t.Errorf("second run should be a no-op, got:\n%s", b2.String())
	}
}

func TestMigrateBlockedBy_Registered(t *testing.T) {
	rootCmd := NewRootCmd()
	taskCmd := findCobraSub(rootCmd, "task")
	if taskCmd == nil {
		t.Fatal("task not registered")
	}
	if findCobraSub(taskCmd, "migrate-blocked-by") == nil {
		t.Fatal("task migrate-blocked-by not registered")
	}
}

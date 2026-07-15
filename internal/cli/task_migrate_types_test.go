package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// seedBacklogWithBugType seeds a backlog holding one legacy bug-typed task
// alongside a fix-typed and a feat-typed task, so migrate-types has exactly
// one row to rewrite.
func seedBacklogWithBugType(t *testing.T) {
	t.Helper()
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load backlog: %v", err)
	}
	backlog.Tasks = []models.Task{
		{
			ID:       "TASK-00001",
			Title:    "legacy-bug-task",
			Type:     models.TaskTypeBug,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
		{
			ID:       "TASK-00002",
			Title:    "already-fix",
			Type:     models.TaskTypeFix,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
		{
			ID:       "TASK-00003",
			Title:    "a-feature",
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

func TestMigrateTypes_DryRun(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	seedBacklogWithBugType(t)

	cmd := newTaskMigrateTypesCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "TASK-00001") || !strings.Contains(out, "would change") {
		t.Errorf("expected TASK-00001 + 'would change' in dry-run output, got:\n%s", out)
	}
	// Disk untouched.
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if backlog.Tasks[0].Type != models.TaskTypeBug {
		t.Errorf("dry-run mutated disk: Tasks[0].Type = %q, want bug", backlog.Tasks[0].Type)
	}
}

func TestMigrateTypes_Apply(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	seedBacklogWithBugType(t)

	cmd := newTaskMigrateTypesCmd()
	_ = cmd.Flags().Set("apply", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Rewrote 1") {
		t.Errorf("expected 'Rewrote 1' in apply output, got:\n%s", buf.String())
	}

	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if backlog.Tasks[0].Type != models.TaskTypeFix {
		t.Errorf("Tasks[0].Type = %q, want fix", backlog.Tasks[0].Type)
	}
	if backlog.Tasks[1].Type != models.TaskTypeFix {
		t.Errorf("Tasks[1].Type = %q, want fix (unchanged)", backlog.Tasks[1].Type)
	}
	if backlog.Tasks[2].Type != models.TaskTypeFeat {
		t.Errorf("Tasks[2].Type = %q, want feat (unchanged)", backlog.Tasks[2].Type)
	}
}

// TestMigrateTypes_Idempotent: a second apply run finds nothing left to change.
func TestMigrateTypes_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	seedBacklogWithBugType(t)

	// First apply.
	first := newTaskMigrateTypesCmd()
	_ = first.Flags().Set("apply", "true")
	var b1 bytes.Buffer
	first.SetOut(&b1)
	first.SetErr(&b1)
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	// Second apply — idempotent no-op.
	second := newTaskMigrateTypesCmd()
	_ = second.Flags().Set("apply", "true")
	var b2 bytes.Buffer
	second.SetOut(&b2)
	second.SetErr(&b2)
	if err := second.Execute(); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !strings.Contains(b2.String(), "No task types need migrating") {
		t.Errorf("second run should be a no-op, got:\n%s", b2.String())
	}
}

func TestMigrateTypes_Registered(t *testing.T) {
	rootCmd := NewRootCmd()
	taskCmd := findCobraSub(rootCmd, "task")
	if taskCmd == nil {
		t.Fatal("task not registered")
	}
	if findCobraSub(taskCmd, "migrate-types") == nil {
		t.Fatal("task migrate-types not registered")
	}
}

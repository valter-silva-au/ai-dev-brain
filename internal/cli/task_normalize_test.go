package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func seedBacklogWithDoubledTitles(t *testing.T) *models.Backlog {
	t.Helper()
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load backlog: %v", err)
	}
	backlog.Tasks = []models.Task{
		{
			ID:       "TASK-00001",
			Title:    "[feat] double-prefix-feat",
			Type:     models.TaskTypeFeat,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
		{
			ID:       "TASK-00002",
			Title:    "[bug] already-raw-title", // deliberate: prefix ≠ type, should be left alone
			Type:     models.TaskTypeFeat,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
		{
			ID:       "TASK-00003",
			Title:    "clean-title-no-prefix",
			Type:     models.TaskTypeBug,
			Status:   models.TaskStatusBacklog,
			Priority: models.PriorityP2,
			Created:  time.Now().UTC(),
			Updated:  time.Now().UTC(),
		},
	}
	if err := App.BacklogManager.Save(backlog); err != nil {
		t.Fatalf("Save backlog: %v", err)
	}
	return backlog
}

func TestNormalizeTitles_DryRun(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	seedBacklogWithDoubledTitles(t)

	cmd := newTaskNormalizeTitlesCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "TASK-00001") {
		t.Errorf("expected TASK-00001 in dry-run output, got:\n%s", out)
	}
	if !strings.Contains(out, "would change") {
		t.Errorf("expected 'would change' in dry-run output, got:\n%s", out)
	}

	// Backlog on disk is untouched.
	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if backlog.Tasks[0].Title != "[feat] double-prefix-feat" {
		t.Errorf("dry-run mutated disk: Tasks[0].Title = %q", backlog.Tasks[0].Title)
	}
}

func TestNormalizeTitles_Apply(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	seedBacklogWithDoubledTitles(t)

	cmd := newTaskNormalizeTitlesCmd()
	_ = cmd.Flags().Set("apply", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Rewrote 1 title(s)") {
		t.Errorf("expected 'Rewrote 1' in apply output, got:\n%s", out)
	}

	backlog, err := App.BacklogManager.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Task 1: had "[feat]" prefix matching Type=feat → stripped.
	if backlog.Tasks[0].Title != "double-prefix-feat" {
		t.Errorf("Tasks[0].Title = %q, want %q", backlog.Tasks[0].Title, "double-prefix-feat")
	}
	// Task 2: "[bug] ..." with Type=feat → prefix doesn't match type, left alone.
	if backlog.Tasks[1].Title != "[bug] already-raw-title" {
		t.Errorf("Tasks[1].Title = %q, want unchanged", backlog.Tasks[1].Title)
	}
	// Task 3: no prefix → untouched.
	if backlog.Tasks[2].Title != "clean-title-no-prefix" {
		t.Errorf("Tasks[2].Title = %q, want unchanged", backlog.Tasks[2].Title)
	}
}

func TestNormalizeTitles_NothingToDo(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	// Empty backlog.
	cmd := newTaskNormalizeTitlesCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "No titles need normalising") {
		t.Errorf("expected 'No titles need normalising', got:\n%s", buf.String())
	}
}

func TestNormalizeTitles_Registered(t *testing.T) {
	rootCmd := NewRootCmd()
	taskCmd := findCobraSub(rootCmd, "task")
	if taskCmd == nil {
		t.Fatal("task not registered")
	}
	if findCobraSub(taskCmd, "normalize-titles") == nil {
		t.Fatal("task normalize-titles not registered")
	}
}

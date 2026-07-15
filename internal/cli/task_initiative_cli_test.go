package cli

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestTaskCLI_InitiativeAssociation drives the ticket↔initiative association
// through the real root command (issue #88): a repo-less task can be associated
// with an initiative at create time, an unknown initiative is rejected, and a
// task with no initiative still works. Association is metadata in backlog.yaml.
func TestTaskCLI_InitiativeAssociation(t *testing.T) {
	// Repo-less tasks never launch Claude, but set the guard for belt-and-braces
	// so the create path is unambiguously non-interactive.
	t.Setenv("ADB_NO_LAUNCH", "1")

	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	if err := runADB(t, "org", "create", "Acme", "--git-host", "github.com"); err != nil {
		t.Fatalf("org create: %v", err)
	}
	if err := runADB(t, "initiative", "create", "Widget Launcher", "--org", "acme"); err != nil {
		t.Fatalf("initiative create: %v", err)
	}

	// Create a repo-less task associated with the initiative.
	if err := runADB(t, "task", "create", "Build the widget", "--initiative", "widget-launcher"); err != nil {
		t.Fatalf("task create --initiative: %v", err)
	}

	backlog, err := app.BacklogManager.Load()
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	var found bool
	for _, task := range backlog.Tasks {
		if task.Title == "Build the widget" {
			found = true
			if task.Initiative != "widget-launcher" {
				t.Errorf("task initiative = %q, want widget-launcher", task.Initiative)
			}
		}
	}
	if !found {
		t.Fatal("associated task not found in backlog")
	}

	// An unknown initiative is rejected at create.
	if err := runADB(t, "task", "create", "Orphan", "--initiative", "ghost"); err == nil {
		t.Error("task create with unknown initiative should fail")
	}

	// A task with no initiative still works (association is optional).
	if err := runADB(t, "task", "create", "Plain task"); err != nil {
		t.Fatalf("task create without initiative: %v", err)
	}

	// `adb task update --initiative` re-associates an existing task; and `""`
	// clears it.
	if err := runADB(t, "task", "create", "Later associate"); err != nil {
		t.Fatalf("task create: %v", err)
	}
	backlog, _ = app.BacklogManager.Load()
	var laterID string
	for _, task := range backlog.Tasks {
		if task.Title == "Later associate" {
			laterID = task.ID
		}
	}
	if laterID == "" {
		t.Fatal("could not find 'Later associate' task")
	}
	if err := runADB(t, "task", "update", laterID, "--initiative", "widget-launcher"); err != nil {
		t.Fatalf("task update --initiative: %v", err)
	}
	if got, _ := app.BacklogManager.GetTask(laterID); got.Initiative != "widget-launcher" {
		t.Errorf("after update, initiative = %q, want widget-launcher", got.Initiative)
	}
	if err := runADB(t, "task", "update", laterID, "--initiative", ""); err != nil {
		t.Fatalf("task update --initiative '': %v", err)
	}
	if got, _ := app.BacklogManager.GetTask(laterID); got.Initiative != "" {
		t.Errorf("after clear, initiative = %q, want empty", got.Initiative)
	}
}

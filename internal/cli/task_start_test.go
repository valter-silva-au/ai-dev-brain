package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// newStartWorkspace gives each test its own isolated App over a temp workspace.
func newStartWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "backlog.yaml"), []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("write backlog: %v", err)
	}
	app, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	oldApp := App
	App = app
	t.Cleanup(func() { App = oldApp })
	return dir
}

func runTaskStart(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut strings.Builder
	cmd := newTaskStartCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// `task start` must promote the task. --no-launch keeps the test from trying to
// exec a real coding agent, which is also exactly what CI and scripts want.
func TestTaskStart_PromotesWithNoLaunch(t *testing.T) {
	newStartWorkspace(t)

	task, err := App.TaskManager.Create(core.CreateTaskOpts{
		Title:    "add retry",
		TaskType: models.TaskTypeFeat,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != models.TaskStatusBacklog {
		t.Fatalf("new task status = %q, want backlog", task.Status)
	}

	if _, _, err := runTaskStart(t, task.ID, "--no-launch"); err != nil {
		t.Fatalf("task start: %v", err)
	}

	got, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if got.Status != models.TaskStatusInProgress {
		t.Errorf("status = %q, want in_progress", got.Status)
	}
}

// ADB_NO_LAUNCH=1 must suppress the launch just like --no-launch, so a CI
// environment can opt out globally without editing every call site.
func TestTaskStart_HonoursADBNoLaunchEnv(t *testing.T) {
	newStartWorkspace(t)
	t.Setenv("ADB_NO_LAUNCH", "1")

	task, err := App.TaskManager.Create(core.CreateTaskOpts{
		Title:    "env opt out",
		TaskType: models.TaskTypeFeat,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, _, err := runTaskStart(t, task.ID); err != nil {
		t.Fatalf("task start: %v", err)
	}

	got, err := App.BacklogManager.GetTask(task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if got.Status != models.TaskStatusInProgress {
		t.Errorf("status = %q, want in_progress", got.Status)
	}
}

// The flags are the contract `start` gained: without them it cannot launch a
// session the way `resume` can, which was the whole defect.
func TestTaskStart_HasLaunchFlags(t *testing.T) {
	cmd := newTaskStartCmd()
	// "here" is deliberately absent: it existed only to bypass the VS Code
	// launch-file hand-off, which is gone, so launching in place is unconditional.
	for _, name := range []string{"agent", "no-launch"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("task start is missing the --%s flag", name)
		}
	}
}

// The help text must not still claim `start` never launches — that sentence is
// what sent people to `resume` for a fresh session.
func TestTaskStart_HelpDoesNotClaimNoLaunch(t *testing.T) {
	cmd := newTaskStartCmd()
	if strings.Contains(cmd.Short, "no session launch") {
		t.Errorf("stale Short still says start does not launch: %q", cmd.Short)
	}
	if !strings.Contains(cmd.Long, "NEW") {
		t.Errorf("Long should distinguish start's new session from resume: %q", cmd.Long)
	}
}

func TestLaunchDirFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task *models.Task
		want string
	}{
		{"nil task", nil, ""},
		{"worktree wins", &models.Task{WorktreePath: "/w", TicketPath: "/t"}, "/w"},
		{"ticket dir when no worktree", &models.Task{TicketPath: "/t"}, "/t"},
		{"neither", &models.Task{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := launchDirFor(tc.task); got != tc.want {
				t.Errorf("launchDirFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPriorSessionExists_EmptyPathIsFalse(t *testing.T) {
	t.Parallel()
	if priorSessionExists("claude", "") {
		t.Error("an empty path cannot have a prior session")
	}
	if priorSessionExists("pi", "") {
		t.Error("an empty path cannot have a prior session")
	}
}

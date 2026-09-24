package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalapp "github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ADB_NO_LAUNCH=1 is a SESSION-WIDE opt-out: you export it once and every adb
// command in that shell stops opening interactive agent sessions. `task create`
// honours it (via suppressLaunch) and `task start` honours it (an inline check),
// but `task resume` read neither — so a script that exported it and called resume
// got an interactive session it had explicitly disabled.
//
// TASK-00039 Batch 4, follow-up 2. The DECISION recorded there: resume honours the
// env var but gets NO --no-launch flag. "Promote without launching" is already
// spelled `adb task start --no-launch`, and a --no-launch on resume would reduce to
// `adb task update --status in_progress` — a flag whose meaning is another command.
// The env var is different in kind: it is a global the user already set, and
// silently ignoring one of three launch paths is the actual inconsistency.
//
// Note this test cannot use t.Parallel(): t.Setenv forbids it, and the env var IS
// the subject here.

// resumeTestApp builds a hermetic App with one repo-less in-progress task and
// returns the App plus the task id. Repo-less is deliberate: launchDirFor falls
// back to the TICKET dir, so the launch path is still reached (a task with no
// launch dir at all would make this test vacuous — it would pass without the fix
// because nothing would launch either way).
func resumeTestApp(t *testing.T) (*internalapp.App, string) {
	t.Helper()

	dir := t.TempDir()
	app, err := internalapp.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}

	task, err := app.TaskManager.Create(core.CreateTaskOpts{
		Title:    "resume env probe",
		TaskType: models.TaskTypeDocs,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Give it a ticket dir on disk so launchDirFor resolves to something.
	if task.TicketPath != "" {
		if mkErr := os.MkdirAll(task.TicketPath, 0o755); mkErr != nil {
			t.Fatalf("mkdir ticket path: %v", mkErr)
		}
	}
	return app, task.ID
}

func TestTaskResume_HonoursADBNoLaunch(t *testing.T) {
	prev := App
	t.Cleanup(func() { App = prev })

	app, taskID := resumeTestApp(t)
	App = app

	// ADB_NO_LAUNCH=1 must suppress the launch. If it does not, the launcher runs
	// and tries to exec a coding agent — so we also pin ADB_TMUX=0 and point
	// ADB_AGENT at a binary that cannot exist, making a leaked launch attempt
	// surface as an error rather than as a hung interactive session.
	t.Setenv("ADB_NO_LAUNCH", "1")
	t.Setenv("ADB_TMUX", "0")

	var stdout, stderr bytes.Buffer
	cmd := newTaskResumeCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{taskID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("resume with ADB_NO_LAUNCH=1 returned an error: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}

	out := stdout.String() + stderr.String()

	// The status flip must still happen — suppressing the launch is not skipping
	// the promotion.
	task, err := app.BacklogManager.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != models.TaskStatusInProgress {
		t.Errorf("status = %q, want %q — ADB_NO_LAUNCH must suppress the LAUNCH, not the promotion",
			task.Status, models.TaskStatusInProgress)
	}

	// And it must say it skipped, rather than going quiet: a user who forgot the
	// var is exported needs to know why no session opened.
	if !strings.Contains(strings.ToLower(out), "adb_no_launch") {
		t.Errorf("output does not mention ADB_NO_LAUNCH, so a user who forgot it is "+
			"exported cannot tell why no session opened; got:\n%s", out)
	}

	// The launcher must not have run. launchWorkflow emits agent.session_started
	// through the event log, so its absence is the machine-checkable proof.
	if launchWasAttempted(t, app) {
		t.Error("agent.session_started was logged, so the launch was NOT suppressed")
	}
}

// TestTaskResume_LaunchesWithoutTheEnvVar is the counterpart. Without it, the test
// above could be satisfied by a resume that never launches under any circumstances.
func TestTaskResume_StillPromotesWithoutTheEnvVar(t *testing.T) {
	prev := App
	t.Cleanup(func() { App = prev })

	app, taskID := resumeTestApp(t)
	App = app

	// Explicitly UNSET, so an exported value in the developer's own shell cannot
	// make this pass for the wrong reason.
	t.Setenv("ADB_NO_LAUNCH", "")
	t.Setenv("ADB_TMUX", "0")
	// A deliberately absent binary: the launch is EXPECTED to be attempted here, and
	// this makes the attempt fail fast instead of opening a real session.
	t.Setenv("ADB_AGENT", "claude")

	var stdout, stderr bytes.Buffer
	cmd := newTaskResumeCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{taskID})

	// The launch may succeed or fail depending on what is installed; either is fine.
	// What must hold is that the promotion happened and the ADB_NO_LAUNCH notice did
	// NOT appear.
	_ = cmd.Execute() //nolint:errcheck // the launch outcome is environment-dependent; the assertions below are the contract

	task, err := app.BacklogManager.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != models.TaskStatusInProgress {
		t.Errorf("status = %q, want %q", task.Status, models.TaskStatusInProgress)
	}

	out := strings.ToLower(stdout.String() + stderr.String())
	if strings.Contains(out, "adb_no_launch") {
		t.Errorf("reported an ADB_NO_LAUNCH skip when the var was unset; got:\n%s", out)
	}
}

// launchWasAttempted reports whether launchWorkflow ran, by looking for the
// agent.session_started event it emits. Reading the event log is what makes this
// checkable without executing an agent.
func launchWasAttempted(t *testing.T, app *internalapp.App) bool {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(app.BasePath, ".adb", "events.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		t.Fatalf("read the event log: %v", err)
	}
	return strings.Contains(string(data), "agent.session_started")
}

package cli

import (
	"errors"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// TestLaunchWorkflowEmitsAgentSessionEvents is the reason `agent.session_*`
// still means anything.
//
// Those three event types were declared, documented with payload keys, counted
// by `metrics.AgentSessions`, and reduced by `adb events digest` — but the ONLY
// thing that ever emitted them was `adb task run-with-ruflo`, a ruflo-specific
// launcher removed in TASK-00039. Removing it would have left a whole
// subcommand and a metric permanently reporting nothing, with a schema comment
// claiming otherwise.
//
// So emission moved to `launchWorkflow`, the single seam every `adb task
// create`/`start`/`resume` launch goes through — which is where an agent
// session actually begins and ends.
func TestLaunchWorkflowEmitsAgentSessionEvents(t *testing.T) {
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app
	t.Cleanup(func() { App = nil })

	launched := 0
	restore := swapAgentLauncher(func(agent, path string, resume bool) error {
		launched++
		if agent != "pi" {
			t.Errorf("launcher got agent %q, want pi", agent)
		}
		if path != "/tmp/wt" {
			t.Errorf("launcher got path %q, want /tmp/wt", path)
		}
		return nil
	})
	defer restore()

	if err := launchWorkflow(taskLaunchInfo{
		TaskID:       "TASK-00042",
		TaskType:     "feat",
		Priority:     "P1",
		WorktreePath: "/tmp/wt",
		Agent:        "pi",
		Resume:       true,
	}); err != nil {
		t.Fatalf("launchWorkflow: %v", err)
	}
	if launched != 1 {
		t.Fatalf("launcher called %d times, want 1", launched)
	}

	events := readAgentSessionEvents(t, app)
	if len(events) != 2 {
		t.Fatalf("emitted %d agent.session_* events, want 2: %#v", len(events), events)
	}
	if events[0].Type != observability.EventAgentSessionStarted {
		t.Fatalf("first event = %q, want started", events[0].Type)
	}
	if events[1].Type != observability.EventAgentSessionEnded {
		t.Fatalf("second event = %q, want ended", events[1].Type)
	}

	// The payload keys are a documented contract (schema.go) that `events
	// digest` and the metrics reducer both read. Keep them identical to what
	// run-with-ruflo emitted, so an existing log stays readable.
	for _, event := range events {
		for _, key := range []string{"task_id", "worktree", "bin"} {
			if _, ok := event.Data[key]; !ok {
				t.Fatalf("%s payload missing %q: %#v", event.Type, key, event.Data)
			}
		}
		if event.Data["task_id"] != "TASK-00042" {
			t.Fatalf("task_id = %v", event.Data["task_id"])
		}
		if event.Data["worktree"] != "/tmp/wt" {
			t.Fatalf("worktree = %v", event.Data["worktree"])
		}
		if event.Data["bin"] != "pi" {
			t.Fatalf("bin = %v, want the resolved agent", event.Data["bin"])
		}
	}
	if _, ok := events[1].Data["error"]; ok {
		t.Fatalf("successful launch recorded an error: %#v", events[1].Data)
	}
}

// TestLaunchWorkflowRecordsAFailedSession pins that a failed launch is still a
// session that started and ended. Emitting only on success would make the
// digest silently under-report exactly the cases worth investigating.
func TestLaunchWorkflowRecordsAFailedSession(t *testing.T) {
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()
	App = app
	t.Cleanup(func() { App = nil })

	restoreLauncher := swapAgentLauncher(func(string, string, bool) error {
		return errors.New("claude: command not found")
	})
	defer restoreLauncher()
	// The failure path drops into an interactive shell, which would block a
	// test; stub it out and record that it was reached.
	fellBack := false
	restoreShell := swapInteractiveShell(func(taskID, path string) error {
		fellBack = true
		return nil
	})
	defer restoreShell()

	if err := launchWorkflow(taskLaunchInfo{
		TaskID:       "TASK-00007",
		WorktreePath: "/tmp/wt",
		Agent:        "claude",
	}); err != nil {
		t.Fatalf("launchWorkflow should fall back, not fail: %v", err)
	}
	if !fellBack {
		t.Fatal("a failed launch did not fall back to the interactive shell")
	}

	events := readAgentSessionEvents(t, app)
	if len(events) != 2 {
		t.Fatalf("emitted %d agent.session_* events, want 2", len(events))
	}
	message, ok := events[1].Data["error"].(string)
	if !ok || message == "" {
		t.Fatalf("failed session recorded no error: %#v", events[1].Data)
	}
	if message != "claude: command not found" {
		t.Fatalf("error = %q, want the launcher's own message", message)
	}
}

// TestLaunchWorkflowSurvivesAnUnavailableEventLog keeps observability
// non-fatal: telemetry must never be the reason a launch fails.
func TestLaunchWorkflowSurvivesAnUnavailableEventLog(t *testing.T) {
	App = nil
	t.Cleanup(func() { App = nil })

	launched := false
	restore := swapAgentLauncher(func(string, string, bool) error {
		launched = true
		return nil
	})
	defer restore()

	if err := launchWorkflow(taskLaunchInfo{
		TaskID:       "TASK-00001",
		WorktreePath: "/tmp/wt",
	}); err != nil {
		t.Fatalf("launchWorkflow failed with no App: %v", err)
	}
	if !launched {
		t.Fatal("no-App path skipped the launch entirely")
	}
}

func readAgentSessionEvents(
	t *testing.T,
	app *internal.App,
) []observability.Event {
	t.Helper()

	all, err := app.EventLog.ReadAll()
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}
	var out []observability.Event
	for _, event := range all {
		// An `if` rather than a two-case switch: this filters two event types out
		// of the whole schema, so it is not a switch that ought to be exhaustive
		// over every EventType (and the linter is right to say a switch here
		// would be).
		if event.Type == observability.EventAgentSessionStarted ||
			event.Type == observability.EventAgentSessionEnded {
			out = append(out, event)
		}
	}
	return out
}

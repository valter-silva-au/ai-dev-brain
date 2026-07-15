package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// appendToEventLog appends raw JSONL bytes to the workspace's event log.
// Used to inject events with hand-picked timestamps for --since testing. The
// log lives under .adb/ as of #186/#190 (App.EventLog reads .adb/events.jsonl),
// so seed there; NewApp's ensureStateDir already created the .adb/ dir.
func appendToEventLog(t *testing.T, basePath string, jsonl []byte) error {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(basePath, ".adb", "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(jsonl)
	return err
}

// setupEventsTest wires a temp workspace + global App the way sync_issues_test.go
// does, so `App.EventLog` writes to <tmp>/.adb/events.jsonl. Returns the wired App
// (for direct .EventLog.Log seeding) + a cleanup that restores the global App.
func setupEventsTest(t *testing.T) (*internal.App, func()) {
	t.Helper()
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	oldApp := App
	App = app
	return app, func() {
		App = oldApp
		app.Cleanup()
	}
}

// TestEventsQuery_FiltersByType_JSON: two events seeded, --type task.created
// filters to one, --json emits a valid JSON array.
func TestEventsQuery_FiltersByType_JSON(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})
	app.EventLog.Log(observability.EventTaskDeleted, map[string]interface{}{"task_id": "TASK-1"})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"query", "--type", "task.created", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var got []observability.Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out.String())
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 filtered event, got %d: %s", len(got), out.String())
	}
	if got[0].Type != observability.EventTaskCreated {
		t.Errorf("expected task.created, got %s", got[0].Type)
	}
}

// TestEventsQuery_FiltersByTaskID: --task filters on data.task_id.
func TestEventsQuery_FiltersByTaskID(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})
	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-2"})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"query", "--task", "TASK-2", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got []observability.Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event for TASK-2, got %d", len(got))
	}
	if id, _ := got[0].Data["task_id"].(string); id != "TASK-2" {
		t.Errorf("expected task_id=TASK-2, got %v", got[0].Data["task_id"])
	}
}

// TestEventsQuery_HumanReadableFormat: without --json, one line per event
// with RFC3339 timestamp + type.
func TestEventsQuery_HumanReadableFormat(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"query"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "task.created") {
		t.Errorf("expected 'task.created' in output, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "TASK-1") {
		t.Errorf("expected 'TASK-1' in output, got:\n%s", out.String())
	}
}

// TestEventsQuery_SinceFilter: --since 24h drops older events. Uses a
// hand-crafted event log so we control timestamps precisely.
func TestEventsQuery_SinceFilter(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	// Log one event now, then rewrite one JSONL line by hand for the "old"
	// event so its timestamp is guaranteed pre-cutoff.
	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-recent"})

	// Append an old event by writing the file directly.
	oldEvent := observability.Event{
		Timestamp: time.Now().UTC().Add(-48 * time.Hour),
		Type:      observability.EventTaskCreated,
		Data:      map[string]interface{}{"task_id": "TASK-old"},
	}
	oldJSON, _ := json.Marshal(oldEvent)
	oldJSON = append(oldJSON, '\n')
	// Reuse EventLog's file (same path).
	if err := appendToEventLog(t, app.BasePath, oldJSON); err != nil {
		t.Fatal(err)
	}

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"query", "--since", "24h", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got []observability.Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	// Only the recent event should survive --since 24h.
	for _, e := range got {
		if id, _ := e.Data["task_id"].(string); id == "TASK-old" {
			t.Errorf("--since 24h should have dropped TASK-old, got: %+v", got)
		}
	}
}

// TestEventsTail_JSONOnce: --json (no --follow) emits one JSON per line for
// each event currently in the log. This is the JSONL shape the F3 webview
// spawns and consumes.
func TestEventsTail_JSONOnce(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventTaskCreated, map[string]interface{}{"task_id": "TASK-1"})
	app.EventLog.Log(observability.EventTaskDeleted, map[string]interface{}{"task_id": "TASK-1"})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"tail", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// One JSON object per line — parse them individually.
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		var e observability.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d not JSON: %v — %s", i, err, line)
		}
	}
}

// TestEventsCommand_Registered: NewEventsCmd exposes 'query' and 'tail'
// subcommands (regression guard for the AddCommand wiring).
func TestEventsCommand_Registered(t *testing.T) {
	cmd := NewEventsCmd()
	subs := map[string]bool{}
	for _, c := range cmd.Commands() {
		subs[c.Name()] = true
	}
	for _, want := range []string{"query", "tail", "digest"} {
		if !subs[want] {
			t.Errorf("expected 'events %s' subcommand", want)
		}
	}
}

// TestEventsDigest_Human: seeds two other sessions + self; the digest excludes
// self and renders one compact line per other task with an age and activity.
func TestEventsDigest_Human(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventAgentSessionActive, map[string]interface{}{
		"task_id": "TASK-00081", "worktree": "/w/81", "activity": "editing",
	})
	app.EventLog.Log(observability.EventAgentSessionActive, map[string]interface{}{
		"task_id": "TASK-00088", "worktree": "/w/88", "activity": "self work",
	})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"digest", "--self", "TASK-00088"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "TASK-00081") {
		t.Errorf("expected TASK-00081 in digest, got:\n%s", got)
	}
	if !strings.Contains(got, "editing") {
		t.Errorf("expected activity 'editing' in digest, got:\n%s", got)
	}
	if !strings.Contains(got, "ago") {
		t.Errorf("expected an age token in digest, got:\n%s", got)
	}
	if strings.Contains(got, "TASK-00088") {
		t.Errorf("digest should exclude --self TASK-00088, got:\n%s", got)
	}
}

// TestEventsDigest_JSON: --json emits a structured array of lines the KB skill
// can parse; self is excluded and each line carries task_id + activity.
func TestEventsDigest_JSON(t *testing.T) {
	app, cleanup := setupEventsTest(t)
	defer cleanup()

	app.EventLog.Log(observability.EventAgentSessionActive, map[string]interface{}{
		"task_id": "TASK-00001", "activity": "running tests",
	})
	app.EventLog.Log(observability.EventAgentSessionStarted, map[string]interface{}{
		"task_id": "TASK-00002",
	})

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"digest", "--self", "TASK-00099", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var got []observability.SessionLine
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 digest lines, got %d: %s", len(got), out.String())
	}
	seen := map[string]string{}
	for _, l := range got {
		seen[l.TaskID] = l.Activity
	}
	if _, ok := seen["TASK-00001"]; !ok {
		t.Errorf("expected TASK-00001 in JSON digest, got: %s", out.String())
	}
	if seen["TASK-00001"] != "running tests" {
		t.Errorf("expected TASK-00001 activity 'running tests', got %q", seen["TASK-00001"])
	}
}

// TestEventsDigest_Empty: no other sessions -> friendly note (human) and an
// empty JSON array (--json), never a nil/null.
func TestEventsDigest_Empty(t *testing.T) {
	_, cleanup := setupEventsTest(t)
	defer cleanup()

	cmd := NewEventsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"digest", "--self", "TASK-00099", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("expected empty JSON array, got: %q", out.String())
	}
}

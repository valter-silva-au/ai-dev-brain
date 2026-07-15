package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestParseTaskType_AcceptsConventionalSet(t *testing.T) {
	valid := map[string]models.TaskType{
		"feat":     models.TaskTypeFeat,
		"fix":      models.TaskTypeFix,
		"refactor": models.TaskTypeRefactor,
		"docs":     models.TaskTypeDocs,
		"chore":    models.TaskTypeChore,
		"test":     models.TaskTypeTest,
		"perf":     models.TaskTypePerf,
		"spike":    models.TaskTypeSpike,
	}
	for in, want := range valid {
		t.Run(in, func(t *testing.T) {
			got, err := parseTaskType(in)
			if err != nil {
				t.Fatalf("parseTaskType(%q) unexpected error: %v", in, err)
			}
			if got != want {
				t.Errorf("parseTaskType(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestParseTaskType_BugRejectedWithHint(t *testing.T) {
	_, err := parseTaskType("bug")
	if err == nil {
		t.Fatal("parseTaskType(\"bug\") = nil error, want a rejection")
	}
	if !strings.Contains(err.Error(), "fix") {
		t.Errorf("bug rejection %q should hint to use `fix`", err.Error())
	}
}

func TestParseTaskType_UnknownRejected(t *testing.T) {
	if _, err := parseTaskType("banana"); err == nil {
		t.Error("parseTaskType(\"banana\") = nil error, want a rejection")
	}
}

// startResultText calls the adb_task_start handler for id and returns its text.
func startResultText(t *testing.T, app *internal.App, id string) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"task_id": id}
	res, err := handleStart(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("handleStart transport error: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("start result has no content")
	}
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("start content[0] is not text: %T", res.Content[0])
	}
	return tc.Text
}

// TestHandleStart_ReportsActualStatus guards #161: adb_task_start reported
// "is now in_progress" unconditionally, but TaskManager.Resume only promotes a
// BACKLOG task — for done/review/blocked it is a no-op. The MCP client must be
// told the real status, not a false success.
func TestHandleStart_ReportsActualStatus(t *testing.T) {
	app, err := internal.NewApp(t.TempDir())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()

	// A backlog task IS promoted — the honest success message.
	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "start me", Type: models.TaskTypeFeat,
		Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if got := startResultText(t, app, "TASK-00001"); !strings.Contains(got, "is now in_progress") {
		t.Errorf("backlog start = %q, want 'is now in_progress'", got)
	}

	// A done task is a no-op: the message must NOT falsely claim in_progress, and
	// the task's real status must be unchanged.
	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00002", Title: "already done", Type: models.TaskTypeFeat,
		Status: models.TaskStatusDone, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	got := startResultText(t, app, "TASK-00002")
	if strings.Contains(got, "is now in_progress") {
		t.Errorf("done-task start falsely claims promotion: %q", got)
	}
	if !strings.Contains(got, "not promoted") || !strings.Contains(got, "done") {
		t.Errorf("done-task start = %q, want a 'not promoted; it is done' message", got)
	}
	if tk, _ := app.BacklogManager.GetTask("TASK-00002"); tk.Status != models.TaskStatusDone {
		t.Errorf("done task status changed to %q after adb_task_start", tk.Status)
	}
}

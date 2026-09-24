package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func validateResultText(
	t *testing.T,
	app *internal.App,
	id string,
) string {
	t.Helper()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"task_id": id}
	res, err := handleTaskValidate(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("handleTaskValidate transport error: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("validate result has no content")
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("validate content[0] is not text: %T", res.Content[0])
	}
	return text.Text
}

// isolatedApp mirrors the isolation note in server_test.go: a t.TempDir()
// basePath pins only the REPO config tier, so the App is built isolated to keep
// the developer's ~/.taskconfig out of it.
func isolatedApp(t *testing.T) (*internal.App, string) {
	t.Helper()

	base := t.TempDir()
	app, err := internal.NewAppIsolated(base)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Cleanup(); err != nil {
			t.Errorf("app cleanup: %v", err)
		}
	})
	return app, base
}

// TestHandleTaskValidate_MissingTaskReportsNotFound pins the answer that is
// genuinely "not found".
func TestHandleTaskValidate_MissingTaskReportsNotFound(t *testing.T) {
	t.Parallel()

	app, _ := isolatedApp(t)
	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "present", Type: models.TaskTypeFeat,
		Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	got := validateResultText(t, app, "TASK-09999")
	if !strings.Contains(got, "not found") {
		t.Errorf("absent task = %q, want a 'not found' message", got)
	}
}

// TestHandleTaskValidate_UnreadableBacklogIsNotReportedAsNotFound is the real
// defect. BacklogManager.GetTask returns an error both when the id is absent and
// when backlog.yaml cannot be read or parsed; adb_task_validate collapsed both
// into "task X not found", so a corrupt workspace told an agent its ticket did
// not exist. Every other GetTask caller in this package reports a load failure
// as a load failure, and the MCP surface is supposed to stay behaviourally
// identical to the CLI.
func TestHandleTaskValidate_UnreadableBacklogIsNotReportedAsNotFound(
	t *testing.T,
) {
	t.Parallel()

	app, base := isolatedApp(t)
	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "present", Type: models.TaskTypeFeat,
		Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(base, "backlog.yaml"),
		[]byte("tasks: [this is: not, valid: yaml\n"),
		0o644,
	); err != nil {
		t.Fatalf("corrupt backlog: %v", err)
	}

	got := validateResultText(t, app, "TASK-00001")
	if strings.Contains(got, "not found") {
		t.Errorf(
			"unreadable backlog reported as %q; a corrupt backlog is not a "+
				"missing ticket",
			got,
		)
	}
	if !strings.Contains(got, "TASK-00001") {
		t.Errorf("load failure %q does not name the task asked for", got)
	}
	if !strings.Contains(strings.ToLower(got), "backlog") {
		t.Errorf("load failure %q does not surface the underlying cause", got)
	}
}

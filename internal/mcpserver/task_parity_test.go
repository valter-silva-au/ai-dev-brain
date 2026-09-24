package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// cliTaskJSONTags is the json tag set (in order) of internal/cli's
// taskStatusJSON — the shape `adb task list --json` emits.
//
// It is a literal copy rather than a reflective assertion over the real type
// for two reasons, both hard rather than stylistic:
//
//  1. internal/cli imports internal/mcpserver (cli/mcp_serve.go wires
//     `adb mcp serve`), so importing internal/cli from this in-package test
//     file is an import cycle and does not compile.
//  2. taskStatusJSON is unexported, so even an external mcpserver_test package
//     — which may legally close that cycle — could not name it.
//
// So the two shapes are pinned by a copied constant on each side. If this list
// and internal/cli's struct ever disagree, THIS test is the thing that has to
// be updated deliberately, which is the point: the drift becomes a decision
// instead of a silent divergence. The comment above taskStatusJSON claimed the
// shapes mirrored each other while four fields were missing here, which is the
// defect this test exists to prevent recurring.
var cliTaskJSONTags = []string{
	"id",
	"title",
	"type",
	"status",
	"priority",
	"owner,omitempty",
	"tags,omitempty",
	"repo,omitempty",
	"worktree_path,omitempty",
	"ticket_path,omitempty",
	"branch,omitempty",
	"initiative,omitempty",
}

// TestTaskView_TagsMatchCLIShape asserts the exact json tag STRINGS, not merely
// that fields exist: `omitempty` and the snake_case spelling are the wire
// contract an MCP client parses, so a field named right and tagged wrong is
// still a break.
func TestTaskView_TagsMatchCLIShape(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(taskView{})
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag, ok := typ.Field(i).Tag.Lookup("json")
		if !ok {
			t.Errorf("taskView.%s has no json tag", typ.Field(i).Name)
			continue
		}
		got = append(got, tag)
	}

	if !reflect.DeepEqual(got, cliTaskJSONTags) {
		t.Errorf("taskView json tags =\n  %v\nwant (cli taskStatusJSON) =\n  %v", got, cliTaskJSONTags)
	}
}

// TestToView_PopulatesPathFields covers the actual cost of the missing fields:
// an agent driving adb over MCP could not find a task's worktree, which is
// where the work happens.
func TestToView_PopulatesPathFields(t *testing.T) {
	t.Parallel()

	task := models.Task{
		ID:           "TASK-00042",
		Title:        "wire the thing",
		Type:         models.TaskTypeFeat,
		Status:       models.TaskStatusInProgress,
		Priority:     models.PriorityP1,
		Owner:        "valter",
		Tags:         []string{"mcp"},
		Repo:         "github.com/valter-silva-au/ai-dev-brain",
		Branch:       "feat/wire-the-thing",
		WorktreePath: "/ws/work/github.com/valter-silva-au/ai-dev-brain/TASK-00042-wire-the-thing",
		TicketPath:   "/ws/tickets/github.com/valter-silva-au/ai-dev-brain/TASK-00042-wire-the-thing",
		Initiative:   "ai-dev-brain",
	}

	view := toView(task)

	for _, tc := range []struct {
		field string
		got   string
		want  string
	}{
		{"WorktreePath", view.WorktreePath, task.WorktreePath},
		{"TicketPath", view.TicketPath, task.TicketPath},
		{"Branch", view.Branch, task.Branch},
		{"Initiative", view.Initiative, task.Initiative},
	} {
		if tc.got != tc.want {
			t.Errorf("toView().%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}

	// And they must reach the wire under the CLI's key names.
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal taskView: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal taskView: %v", err)
	}
	for key, want := range map[string]string{
		"worktree_path": task.WorktreePath,
		"ticket_path":   task.TicketPath,
		"branch":        task.Branch,
		"initiative":    task.Initiative,
	} {
		if wire[key] != want {
			t.Errorf("marshalled taskView[%q] = %v, want %q", key, wire[key], want)
		}
	}
}

// TestToView_OmitsEmptyPathFields pins the `omitempty` half of the tags: a
// repo-less task has no worktree, and the key must be absent rather than empty.
func TestToView_OmitsEmptyPathFields(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(toView(models.Task{
		ID: "TASK-00001", Title: "local only", Type: models.TaskTypeWork,
		Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
	}))
	if err != nil {
		t.Fatalf("marshal taskView: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal taskView: %v", err)
	}
	for _, key := range []string{"worktree_path", "ticket_path", "branch", "initiative"} {
		if _, present := wire[key]; present {
			t.Errorf("empty %q should be omitted, got %v", key, wire[key])
		}
	}
}

// closeResultText calls the adb_task_close handler for id and returns its text.
func closeResultText(t *testing.T, app *internal.App, id string) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"task_id": id}
	res, err := handleClose(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("handleClose transport error: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatal("close result has no content")
	}
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("close content[0] is not text: %T", res.Content[0])
	}
	return tc.Text
}

// TestHandleStart_PromotesBacklogTask asserts the STORED status, not just the
// reported one: handleStart now goes through core.TaskManager.Start (the same
// method `adb task start` uses), so the promotion must actually persist.
func TestHandleStart_PromotesBacklogTask(t *testing.T) {
	t.Parallel()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "start me", Type: models.TaskTypeFeat,
		Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	if got := startResultText(t, app, "TASK-00001"); !strings.Contains(got, "is now in_progress") {
		t.Errorf("backlog start = %q, want 'is now in_progress'", got)
	}
	task, err := app.BacklogManager.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != models.TaskStatusInProgress {
		t.Errorf("stored status = %q, want in_progress", task.Status)
	}
}

// TestHandleStart_ArchivedTaskIsReportedNotErrored is the one case where
// core.TaskManager.Start and .Resume are observably different, and therefore the
// only test that actually pins the switch from Resume to Start:
//
//	Resume: returns an error, "cannot resume archived task TASK-…"
//	Start:  no-ops (it is silently idempotent for any non-backlog status)
//
// Start is the correct one for a tool named `start`, because it is what
// `adb task start` calls — the two entry points must not disagree about what
// starting an archived ticket does. The #161 contract still holds: the caller is
// told the ACTUAL status rather than a false promotion.
func TestHandleStart_ArchivedTaskIsReportedNotErrored(t *testing.T) {
	t.Parallel()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "retired", Type: models.TaskTypeFeat,
		Status: models.TaskStatusArchived, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"task_id": "TASK-00001"}
	res, err := handleStart(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("handleStart transport error: %v", err)
	}
	if res.IsError {
		t.Errorf("archived start returned a tool error; want a truthful status report (Resume's behaviour, not Start's)")
	}
	got := startResultText(t, app, "TASK-00001")
	if !strings.Contains(got, "not promoted") || !strings.Contains(got, "archived") {
		t.Errorf("archived start = %q, want a 'not promoted; it is archived' message", got)
	}
	if task, _ := app.BacklogManager.GetTask("TASK-00001"); task.Status != models.TaskStatusArchived {
		t.Errorf("archived task status changed to %q after adb_task_start", task.Status)
	}
}

// TestHandleClose_MarksTaskDone covers the routing through
// core.TaskManager.Close, so `adb task close` and adb_task_close are provably
// the same operation.
func TestHandleClose_MarksTaskDone(t *testing.T) {
	t.Parallel()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	if err := app.BacklogManager.AddTask(models.Task{
		ID: "TASK-00001", Title: "close me", Type: models.TaskTypeFeat,
		Status: models.TaskStatusInProgress, Priority: models.PriorityP2,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	if got := closeResultText(t, app, "TASK-00001"); !strings.Contains(got, "marked done") {
		t.Errorf("close = %q, want 'marked done'", got)
	}
	task, err := app.BacklogManager.GetTask("TASK-00001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != models.TaskStatusDone {
		t.Errorf("stored status = %q, want done", task.Status)
	}
}

// TestHandleClose_UnknownIDErrors pins the deliberate asymmetry documented on
// core.TaskManager.Close: unlike Start, it is not silently idempotent, so
// closing a typo'd id must not report success.
func TestHandleClose_UnknownIDErrors(t *testing.T) {
	t.Parallel()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"task_id": "TASK-99999"}
	res, err := handleClose(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("handleClose transport error: %v", err)
	}
	if !res.IsError {
		t.Error("closing an unknown task id reported success; want a tool error")
	}
}

// toolParamDescription returns the description of one input-schema property of a
// registered tool, so a test can assert what a CLIENT is actually told rather
// than what the source comment nearby claims.
func toolParamDescription(t *testing.T, tool, param string) string {
	t.Helper()
	registered := New(nil, "test").GetTool(tool)
	if registered == nil {
		t.Fatalf("tool %q is not registered", tool)
	}
	prop, ok := registered.Tool.InputSchema.Properties[param]
	if !ok {
		t.Fatalf("tool %q has no %q property", tool, param)
	}
	spec, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("tool %q property %q is %T, want map[string]any", tool, param, prop)
	}
	desc, _ := spec["description"].(string)
	return desc
}

// TestCreateToolDescribesEveryValidTaskType is a drift guard on discoverability.
// parseTaskType accepts the full models.ValidTaskTypes set, but the tool's `type`
// description listed only the 8 Conventional code types — so `work` and
// `prototype` were accepted and undocumented, which to a client is
// indistinguishable from unsupported. The schema description is the only place an
// MCP client can learn the accepted values, so adding a task type must update it.
func TestCreateToolDescribesEveryValidTaskType(t *testing.T) {
	t.Parallel()
	desc := toolParamDescription(t, "adb_task_create", "type")
	for _, tt := range models.ValidTaskTypes {
		if !strings.Contains(desc, string(tt)) {
			t.Errorf("adb_task_create `type` description omits the valid type %q: %s", tt, desc)
		}
	}
	// The retired `bug` alias may be MENTIONED — telling a client up front that
	// it is rejected saves a failed call, and parseTaskType's error says the same
	// thing — but it must never read as an accepted value.
	if strings.Contains(desc, "bug") && !strings.Contains(desc, "retired") {
		t.Errorf("adb_task_create `type` description names `bug` without marking it retired: %s", desc)
	}
}

// TestListToolDescribesReturnedFields keeps the adb_task_list description
// honest about the shape taskView actually returns — the four path fields were
// added to the struct, and a description that still lists eight keys tells a
// client the worktree it needs is not there.
func TestListToolDescribesReturnedFields(t *testing.T) {
	t.Parallel()
	registered := New(nil, "test").GetTool("adb_task_list")
	if registered == nil {
		t.Fatal("adb_task_list is not registered")
	}
	desc := registered.Tool.Description
	for _, key := range []string{"worktree_path", "ticket_path", "branch", "initiative"} {
		if !strings.Contains(desc, key) {
			t.Errorf("adb_task_list description omits returned field %q: %s", key, desc)
		}
	}
}

// TestInstructions_NameNoRemovedCommand guards the one string in this package
// that every MCP client is handed. It named `task start-all` / `close-all`
// after both were removed, which is worse than saying nothing: an agent
// following it runs a command that does not exist.
func TestInstructions_NameNoRemovedCommand(t *testing.T) {
	t.Parallel()
	for _, gone := range []string{
		"start-all",
		"close-all",
		"task status", // renamed to `adb task list`
		"task delete", // renamed to `adb task remove`
		"dashboard",
		"adb chat",
		"adb session",
		"adb sync",
		"adb work",
		"adb plugin",
		"adb memory",
	} {
		if strings.Contains(instructions, gone) {
			t.Errorf("instructions still name the removed/renamed %q", gone)
		}
	}
}

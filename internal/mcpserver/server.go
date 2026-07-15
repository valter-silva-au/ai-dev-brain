// Package mcpserver implements a Model Context Protocol (MCP) server that
// exposes AI Dev Brain's task lifecycle over stdio so that MCP clients
// (Claude Code, Claude Desktop, etc.) can drive ADB natively as tool calls
// instead of shelling out to the CLI.
//
// The server is a thin adapter: every tool delegates to the same
// internal.App subsystems the CLI uses (TaskManager, BacklogManager), so
// behaviour and storage are identical regardless of entry point.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

const (
	serverName = "ai-dev-brain"

	instructions = `AI Dev Brain (adb) task tracker. Use these tools to manage the developer's
tasks/tickets in the current workspace: list and filter tasks, create new ones,
start (promote to in_progress) and close (mark done) individual or all tasks,
and update status/priority/owner. Task IDs look like TASK-00001. Statuses:
backlog, in_progress, blocked, review, done, archived. Prefer adb_task_list to
discover IDs before mutating. Bulk operations (adb_task_start_all,
adb_task_close_all) act on every eligible task at once.

Graph + knowledge tools traverse the workspace's typed entity graph and vector
memory: graph_neighbors (edges incident to an entity), related_tickets (tickets
linked to a ticket), get_initiative (an initiative's stage + gate), and
search_knowledge (semantic search; degrades gracefully when memory is
unconfigured).`
)

// New builds an MCP server backed by the given App. The version string is
// surfaced to clients in the initialize handshake.
func New(app *internal.App, version string) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
		server.WithInstructions(instructions),
	)
	registerTaskTools(s, app)
	registerGraphTools(s, app)
	return s
}

// Serve builds the server and serves it over stdio, blocking until the
// client disconnects or the context is cancelled.
func Serve(app *internal.App, version string) error {
	return server.ServeStdio(New(app, version))
}

// registerTaskTools wires every task tool onto the server.
func registerTaskTools(s *server.MCPServer, app *internal.App) {
	s.AddTool(mcp.NewTool("adb_task_list",
		mcp.WithDescription("List tasks in the workspace, optionally filtered by status. Returns JSON with id, title, type, status, priority, owner, tags for each task."),
		mcp.WithString("status",
			mcp.Description("Optional status filter: backlog, in_progress, blocked, review, done, or archived. Omit for all tasks."),
		),
	), handleList(app))

	s.AddTool(mcp.NewTool("adb_task_create",
		mcp.WithDescription("Create a new task in the backlog. Returns the minted task ID (e.g. TASK-00001)."),
		mcp.WithString("title", mcp.Required(),
			mcp.Description("Short task title / branch name."),
		),
		mcp.WithString("type",
			mcp.Description("Task type: feat (default), fix, refactor, docs, chore, test, perf, or spike."),
		),
		mcp.WithString("priority",
			mcp.Description("Priority: P0, P1, P2 (default), or P3."),
		),
		mcp.WithString("description",
			mcp.Description("Optional longer description of the task."),
		),
		mcp.WithString("owner",
			mcp.Description("Optional task owner."),
		),
	), handleCreate(app))

	s.AddTool(mcp.NewTool("adb_task_start",
		mcp.WithDescription("Start a single task: promote it from backlog to in_progress."),
		mcp.WithString("task_id", mcp.Required(),
			mcp.Description("The task ID to start, e.g. TASK-00001."),
		),
	), handleStart(app))

	s.AddTool(mcp.NewTool("adb_task_close",
		mcp.WithDescription("Close a single task: mark it done. Does not archive or remove the worktree."),
		mcp.WithString("task_id", mcp.Required(),
			mcp.Description("The task ID to close, e.g. TASK-00001."),
		),
	), handleClose(app))

	s.AddTool(mcp.NewTool("adb_task_update",
		mcp.WithDescription("Update a task's status, priority, and/or owner."),
		mcp.WithString("task_id", mcp.Required(),
			mcp.Description("The task ID to update."),
		),
		mcp.WithString("status",
			mcp.Description("New status: backlog, in_progress, blocked, review, done, or archived."),
		),
		mcp.WithString("priority",
			mcp.Description("New priority: P0, P1, P2, or P3."),
		),
		mcp.WithString("owner",
			mcp.Description("New owner."),
		),
	), handleUpdate(app))

	s.AddTool(mcp.NewTool("adb_task_start_all",
		mcp.WithDescription("Start every backlog task at once (promote all to in_progress). Idempotent. Returns a per-task summary."),
	), handleStartAll(app))

	s.AddTool(mcp.NewTool("adb_task_close_all",
		mcp.WithDescription("Close every active task at once (mark all in_progress/blocked/review tasks done). Returns a per-task summary."),
	), handleCloseAll(app))
}

// taskView is the JSON shape returned to clients for a task.
type taskView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Owner    string   `json:"owner,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Repo     string   `json:"repo,omitempty"`
}

func toView(t models.Task) taskView {
	return taskView{
		ID:       t.ID,
		Title:    t.Title,
		Type:     string(t.Type),
		Status:   string(t.Status),
		Priority: string(t.Priority),
		Owner:    t.Owner,
		Tags:     t.Tags,
		Repo:     t.Repo,
	}
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to encode result", err), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func handleList(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		backlog, err := app.BacklogManager.Load()
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to load backlog", err), nil
		}
		filter := req.GetString("status", "")
		views := make([]taskView, 0, len(backlog.Tasks))
		for _, t := range backlog.Tasks {
			if filter != "" && string(t.Status) != filter {
				continue
			}
			views = append(views, toView(t))
		}
		return jsonResult(map[string]any{"count": len(views), "tasks": views})
	}
}

func handleCreate(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}

		tt, err := parseTaskType(req.GetString("type", "feat"))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid type", err), nil
		}
		pr, err := parsePriority(req.GetString("priority", "P2"))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid priority", err), nil
		}

		task, err := app.TaskManager.Create(core.CreateTaskOpts{
			Title:       title,
			Description: req.GetString("description", ""),
			TaskType:    tt,
			Priority:    pr,
			Owner:       req.GetString("owner", ""),
		})
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to create task", err), nil
		}
		return jsonResult(map[string]any{
			"created": toView(*task),
			"message": fmt.Sprintf("Task %s created in backlog", task.ID),
		})
	}
}

func handleStart(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		task, err := app.TaskManager.Resume(id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to start task", err), nil
		}
		// Resume only promotes a backlog task; for done/review/blocked it is a
		// no-op that returns the unchanged task. Report the ACTUAL status so an
		// agent isn't falsely told a done/review task became in_progress (#161).
		if task.Status == models.TaskStatusInProgress {
			return mcp.NewToolResultText(fmt.Sprintf("Task %s is now in_progress", id)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Task %s was not promoted; it is %s (only a backlog task can be started)", id, task.Status)), nil
	}
}

func handleClose(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := app.TaskManager.UpdateStatus(id, models.TaskStatusDone); err != nil {
			return mcp.NewToolResultErrorFromErr("failed to close task", err), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Task %s marked done", id)), nil
	}
}

func handleUpdate(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}

		changed := []string{}

		if status := req.GetString("status", ""); status != "" {
			st, err := parseStatus(status)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("invalid status", err), nil
			}
			if err := app.TaskManager.UpdateStatus(id, st); err != nil {
				return mcp.NewToolResultErrorFromErr("failed to update status", err), nil
			}
			changed = append(changed, "status="+status)
		}

		if priority := req.GetString("priority", ""); priority != "" {
			pr, err := parsePriority(priority)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("invalid priority", err), nil
			}
			if err := app.TaskManager.UpdatePriority(id, pr); err != nil {
				return mcp.NewToolResultErrorFromErr("failed to update priority", err), nil
			}
			changed = append(changed, "priority="+priority)
		}

		if owner := req.GetString("owner", ""); owner != "" {
			task, err := app.BacklogManager.GetTask(id)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("failed to load task", err), nil
			}
			task.Owner = owner
			task.UpdateTimestamp()
			if err := app.BacklogManager.UpdateTask(*task); err != nil {
				return mcp.NewToolResultErrorFromErr("failed to update owner", err), nil
			}
			changed = append(changed, "owner="+owner)
		}

		if len(changed) == 0 {
			return mcp.NewToolResultError("no updates specified; provide status, priority, and/or owner"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Task %s updated: %v", id, changed)), nil
	}
}

func handleStartAll(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		results, err := app.TaskManager.StartAll()
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to start tasks", err), nil
		}
		return jsonResult(summarizeBulk("start", results))
	}
}

func handleCloseAll(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		results, err := app.TaskManager.CloseAll()
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to close tasks", err), nil
		}
		return jsonResult(summarizeBulk("close", results))
	}
}

func summarizeBulk(verb string, results []core.BulkResult) map[string]any {
	type item struct {
		TaskID    string `json:"task_id"`
		OldStatus string `json:"old_status"`
		NewStatus string `json:"new_status"`
		Error     string `json:"error,omitempty"`
	}
	items := make([]item, 0, len(results))
	failures := 0
	for _, r := range results {
		it := item{TaskID: r.TaskID, OldStatus: string(r.OldStatus), NewStatus: string(r.NewStatus)}
		if r.Err != nil {
			it.Error = r.Err.Error()
			failures++
		}
		items = append(items, it)
	}
	return map[string]any{
		"verb":      verb,
		"total":     len(results),
		"succeeded": len(results) - failures,
		"failed":    failures,
		"results":   items,
	}
}

// parseTaskType validates an MCP-supplied type against the canonical
// Conventional set (models.ValidTaskTypes), matching the CLI create path. The
// legacy "bug" alias is rejected with a hint to use "fix"; any other unknown
// value is rejected with the accepted list.
func parseTaskType(s string) (models.TaskType, error) {
	if s == string(models.TaskTypeBug) {
		return "", fmt.Errorf("task type %q is retired; use `fix` instead", s)
	}
	tt := models.TaskType(s)
	if !tt.IsValid() {
		parts := make([]string, len(models.ValidTaskTypes))
		for i, t := range models.ValidTaskTypes {
			parts[i] = string(t)
		}
		return "", fmt.Errorf("invalid task type %q (must be one of %s)", s, strings.Join(parts, ", "))
	}
	return tt, nil
}

func parsePriority(s string) (models.Priority, error) {
	switch s {
	case "P0":
		return models.PriorityP0, nil
	case "P1":
		return models.PriorityP1, nil
	case "P2":
		return models.PriorityP2, nil
	case "P3":
		return models.PriorityP3, nil
	default:
		return "", fmt.Errorf("invalid priority %q (must be P0, P1, P2, or P3)", s)
	}
}

func parseStatus(s string) (models.TaskStatus, error) {
	switch s {
	case "backlog":
		return models.TaskStatusBacklog, nil
	case "in_progress":
		return models.TaskStatusInProgress, nil
	case "blocked":
		return models.TaskStatusBlocked, nil
	case "review":
		return models.TaskStatusReview, nil
	case "done":
		return models.TaskStatusDone, nil
	case "archived":
		return models.TaskStatusArchived, nil
	default:
		return "", fmt.Errorf("invalid status %q", s)
	}
}

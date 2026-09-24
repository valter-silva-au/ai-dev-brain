// Package mcpserver implements a Model Context Protocol (MCP) server that
// exposes AI Dev Brain capabilities over stdio so that MCP clients
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
	"github.com/valter-silva-au/ai-dev-brain/internal/foundation"
	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
	"github.com/valter-silva-au/ai-dev-brain/internal/v3mcp"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

const (
	serverName = "ai-dev-brain"

	// instructions is handed to every MCP client in the initialize handshake, so
	// it must name only things that exist. It deliberately names TOOLS rather
	// than CLI commands: a tool name is verified by this package's registration,
	// whereas a CLI spelling here is an unchecked copy that goes stale the next
	// time the command tree is reorganised — which is exactly how the shipped
	// build came to advertise `task start-all` / `close-all` months after both
	// were removed. TestInstructions_NameNoRemovedCommand guards the regression.
	// The one CLI command named below (`adb task resume`) is there because no
	// tool replaces it, and it is asserted to still exist by that same test's
	// denylist staying in sync with the command tree.
	instructions = `AI Dev Brain (adb) task tracker. Use these tools to manage the developer's
tasks/tickets in the current workspace: list and filter tasks, create new ones,
start (promote to in_progress) and close (mark done) individual tasks, and
update a task's status/priority/owner. Task IDs look like TASK-00001. Statuses:
backlog, in_progress, blocked, review, done, archived. Prefer adb_task_list to
discover IDs before mutating. Bulk operations are intentionally NOT on this
surface: act on one task at a time so the caller stays explicit about which
tickets moved.

Every task returned by adb_task_list and adb_task_create carries worktree_path
and ticket_path, so a task tells you where its work happens and where its notes
live without re-reading backlog.yaml. adb_task_start only promotes the ticket:
it launches no coding-agent session and does not refresh the worktree's Tier-0
context file. When a live session is what is wanted, ask the developer to run
"adb task resume <id>" in a terminal. Starting a ticket that is not in backlog
is a no-op and is reported as its actual status, never as a promotion.

Process tools let an agent follow the workspace's document programs itself:
adb_program_list (which packs exist), adb_program_status (gate state),
adb_program_next (what is draftable right now), and adb_task_validate (DoD
self-check for duplicate-seed and stale-template drift). Programs surface only
where provisioned; artifact execution stays CLI/human.

Graph + knowledge tools traverse the workspace's typed entity graph and vector
memory: graph_neighbors (edges incident to an entity), related_tickets (tickets
linked to a ticket), get_initiative (an initiative's stage + gate), and
search_knowledge (semantic search; degrades gracefully when memory is
unconfigured).

Workspace foundation tools use the v3 .aidb boundary:
adb_workspace_initialize plans by default and mutates only with apply: true;
adb_workspace_doctor is strictly read-only and reports typed findings. Both
return the same versioned result envelope as their CLI counterparts.

Organization tools manage v3 trust scopes through portable manifests and a
rebuildable projection. Initialize, update, adopt, move, and archive plan by
default and mutate only with apply: true; list, show, and validate are
read-only.

Repository tools manage v3 repository identity, canonical clones, health,
safe fast-forward updates, archive state, and registered worktrees. Mutations
plan by default and require apply: true; worktree prune also requires an exact
path and refuses active, dirty, ahead, diverged, or unknown worktrees.`
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
	registerProgramTools(s, app)
	foundationService, err := foundation.NewService(foundation.Options{})
	if err != nil {
		panic(fmt.Sprintf("construct default v3 foundation service: %v", err))
	}
	organizationService, err := organization.NewService(organization.Options{})
	if err != nil {
		panic(fmt.Sprintf("construct default v3 organization service: %v", err))
	}
	repositoryService, err := repository.NewService(repository.Options{})
	if err != nil {
		panic(fmt.Sprintf("construct default v3 repository service: %v", err))
	}
	v3mcp.Register(s, foundationService)
	v3mcp.RegisterOrganization(s, organizationService)
	v3mcp.RegisterRepository(s, repositoryService)
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
		mcp.WithDescription("List tasks in the workspace, optionally filtered by status. Returns JSON with id, title, type, status, priority, owner, tags, repo, worktree_path, ticket_path, branch, and initiative for each task — the same field set as `adb task list --json`. Optional fields are omitted when empty (a repo-less ticket has no worktree_path or branch)."),
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
			mcp.Description("Task type. Code types: feat (default), fix, refactor, docs, chore, test, perf, spike. Non-code types: work (an artifact/graph deliverable — no worktree or branch is created) and prototype (a time-boxed experiment). The retired `bug` alias is rejected; use fix."),
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
		mcp.WithDescription("Start a single task: promote it from backlog to in_progress. Launches no coding-agent session and does not refresh the worktree's context file (that is `adb task resume` in a terminal). A task not in backlog is left unchanged and its actual status is reported back."),
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
}

// taskView is the JSON shape returned to clients for a task.
//
// Its field set and json tags are IDENTICAL to internal/cli's taskStatusJSON —
// the shape `adb task list --json` emits — so a consumer can parse either entry
// point with one decoder. That parity is asserted by
// TestTaskView_TagsMatchCLIShape, which explains there why it pins a copied tag
// list rather than reflecting over the CLI type (internal/cli imports this
// package, so naming it here is an import cycle).
type taskView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Owner    string   `json:"owner,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Repo     string   `json:"repo,omitempty"`
	// WorktreePath and TicketPath are what make a returned task actionable to an
	// agent: the worktree is where the work happens, and without it an MCP
	// client had to shell out to the CLI to find the directory it was already
	// being told about. Branch and Initiative complete the CLI's shape.
	WorktreePath string `json:"worktree_path,omitempty"`
	TicketPath   string `json:"ticket_path,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Initiative   string `json:"initiative,omitempty"`
}

func toView(t models.Task) taskView {
	return taskView{
		ID:           t.ID,
		Title:        t.Title,
		Type:         string(t.Type),
		Status:       string(t.Status),
		Priority:     string(t.Priority),
		Owner:        t.Owner,
		Tags:         t.Tags,
		Repo:         t.Repo,
		WorktreePath: t.WorktreePath,
		TicketPath:   t.TicketPath,
		Branch:       t.Branch,
		Initiative:   t.Initiative,
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
		// Start, not Resume: the tool is named `start`, and Start is the method
		// `adb task start` calls, so the CLI and this tool cannot come to mean
		// different things by the same word.
		//
		// DELIBERATELY LOST in that switch, recorded rather than left to be
		// rediscovered: Resume additionally re-renders the worktree's Tier-0
		// .claude/rules/task-context.md and re-provisions the per-worktree Serena
		// config (see core.TaskManager.Resume). Start does neither. That is
		// correct for `start` — the CLI has the same split, and refreshing a
		// worktree's context is a resume concern — but it is a real behaviour
		// change: an MCP client that relied on adb_task_start to freshen a
		// worktree's context file no longer gets it, and should ask the developer
		// for `adb task resume <id>` instead.
		//
		// The other observable difference: Resume ERRORS on an archived task,
		// while Start is silently idempotent for any non-backlog status. So an
		// archived ticket now reports its real status below instead of failing.
		if err := app.TaskManager.Start(id); err != nil {
			return mcp.NewToolResultErrorFromErr("failed to start task", err), nil
		}
		// Start returns only an error, so re-read the task to report the truth.
		// It promotes ONLY a backlog task; for done/review/blocked/archived it is
		// a no-op. Report the ACTUAL status so an agent isn't falsely told a
		// done/review task became in_progress (#161).
		task, err := app.BacklogManager.GetTask(id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to load task", err), nil
		}
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
		// Close, not a hand-rolled UpdateStatus(id, done): `adb task close` goes
		// through the same core method, so the two spellings of "close a task"
		// are provably one operation. Unlike Start, Close is NOT silently
		// idempotent — an unknown id errors, so closing a typo cannot report a
		// ticket closed that was never touched.
		if err := app.TaskManager.Close(id); err != nil {
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

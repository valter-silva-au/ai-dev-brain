package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// This file registers the READ-ONLY PROCESS tools (TASK-00031 phase 5): agents
// follow the process themselves — discovery (program_list), gate state
// (program_status), what is draftable right now (program_next) and the DoD
// self-check (task_validate). Write-side operations (scaffold, --force) stay
// CLI/human: the MCP surface stays read-mostly, and program manifest
// human_review gates remain human-checked.
//
// Programs surface only where provisioned: with no program provisioned the
// tools return a clear "no program provisioned" result (not an error) so an
// agent does nothing rather than inventing a process.

// registerProgramTools wires the read-only program/validate tools onto the
// server. Every handler delegates to the same core functions the CLI uses —
// thin wrappers, no new business logic.
func registerProgramTools(s *server.MCPServer, app *internal.App) {
	s.AddTool(mcp.NewTool("adb_program_list",
		mcp.WithDescription("List available document programs (phase-ordered artifact template packs): embedded defaults plus any provisioned via programs_search_paths."),
	), handleProgramList(app))

	s.AddTool(mcp.NewTool("adb_program_status",
		mcp.WithDescription("Full gate classification for a program's templates: generated / ready / blocked / pending, with resolved output paths."),
		mcp.WithString("program_id", mcp.Required(),
			mcp.Description("The program pack id, e.g. technical-design."),
		),
	), handleProgramStatus(app))

	s.AddTool(mcp.NewTool("adb_program_next",
		mcp.WithDescription("What artifacts can be drafted RIGHT NOW for a program: ready templates with their resolved from:/write: paths. Re-call after drafting to see what unblocked."),
		mcp.WithString("program_id", mcp.Required(),
			mcp.Description("The program pack id, e.g. technical-design."),
		),
	), handleProgramNext(app))

	s.AddTool(mcp.NewTool("adb_task_validate",
		mcp.WithDescription("Check a ticket for drift before claiming done: duplicate description seed in notes.md, stale template_version vs the resolved template."),
		mcp.WithString("task_id", mcp.Required(),
			mcp.Description("The task ID to validate, e.g. TASK-00001."),
		),
	), handleTaskValidate(app))
}

// programSource locates one program pack: an fs.FS plus the packs ROOT within
// it, so callers can hand (fsys, root, id) straight to core.LoadProgram.
type programSource struct {
	fsys fs.FS
	root string
	id   string
}

// resolveProgramSource locates a program by id: the embedded packs first — the
// shipped pack is the reference definition and wins an id collision — then each
// configured search path. Mirrors the CLI's resolveProgramSource so the two
// entry points resolve identically.
func resolveProgramSource(app *internal.App, id string) (programSource, error) {
	if app == nil {
		return programSource{}, fmt.Errorf("app not initialized")
	}
	candidates := []programSource{{fsys: templates.FS, root: core.ProgramsRoot, id: id}}
	for _, dir := range core.ProgramSearchPaths(app.MergedConfig, app.BasePath) {
		candidates = append(candidates, programSource{fsys: os.DirFS(dir), root: ".", id: id})
	}
	var lastErr error
	for _, c := range candidates {
		if _, err := core.LoadProgram(c.fsys, path.Join(c.root, c.id)); err == nil {
			return programSource{fsys: c.fsys, root: c.root, id: c.id}, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("program %q not found", id)
	}
	return programSource{}, lastErr
}

func handleProgramList(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ids, err := core.ListPrograms(templates.FS, core.ProgramsRoot, core.ProgramSearchPaths(app.MergedConfig, app.BasePath))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to list programs", err), nil
		}
		if len(ids) == 0 {
			return mcp.NewToolResultText("No document programs available (none embedded or provisioned for this workspace) — not an error; nothing to follow."),
				nil
		}
		return jsonResult(map[string]any{"count": len(ids), "programs": ids})
	}
}

func handleProgramStatus(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("program_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		src, err := resolveProgramSource(app, id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to resolve program", err), nil
		}
		p, err := core.LoadProgram(src.fsys, path.Join(src.root, src.id))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to load program", err), nil
		}
		states, err := core.ProgramStatus(p, app.BasePath)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to classify program status", err), nil
		}
		return jsonResult(map[string]any{"program_id": id, "templates": states})
	}
}

func handleProgramNext(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("program_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		src, err := resolveProgramSource(app, id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to resolve program", err), nil
		}
		p, err := core.LoadProgram(src.fsys, path.Join(src.root, src.id))
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to load program", err), nil
		}
		next, err := core.NextTemplates(p, app.BasePath)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to compute next templates", err), nil
		}
		if len(next) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No artifacts are draftable right now for %q (gates closed or everything generated) — not an error.", id)), nil
		}
		return jsonResult(map[string]any{"program_id": id, "next": next})
	}
}

func handleTaskValidate(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		// GetTask returns an error both when the id is absent AND when
		// backlog.yaml cannot be read or parsed. Collapsing the two into "not
		// found" told an agent its ticket did not exist when the workspace was
		// merely corrupt — and it diverged from every other GetTask caller in
		// this package, which report a load failure as one.
		task, err := app.BacklogManager.GetTask(id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr(
				fmt.Sprintf("failed to load task %s", id),
				err,
			), nil
		}
		if task == nil {
			return mcp.NewToolResultError(fmt.Sprintf("task %s not found", id)), nil
		}
		lm, err := core.NewLayeredTemplateManager(appWorkspaceTemplates(app), templates.FS)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to build template manager", err), nil
		}
		dir := ticketDir(app, task)
		if dir == "" {
			return mcp.NewToolResultError(fmt.Sprintf("ticket directory for %s not resolved", id)), nil
		}
		description := ticketDescriptionFrom(dir)
		findings := core.InspectTicket(dir, description, lm)
		clean := len(findings) == 0
		return jsonResult(map[string]any{
			"task_id":  id,
			"clean":    clean,
			"findings": findings,
			"message": map[bool]string{
				true:  fmt.Sprintf("✓ %s: ticket clean", id),
				false: fmt.Sprintf("⚠ %s: %d finding(s) — resolve before claiming done", id, len(findings)),
			}[clean],
		})
	}
}

// ticketDescriptionFrom extracts the description from the ticket's context.md
// (## Description) — the ground truth the duplicate seed copied.
func ticketDescriptionFrom(dir string) string {
	b, err := os.ReadFile(fmt.Sprintf("%s/context.md", dir))
	if err != nil {
		return ""
	}
	return core.ExtractSection(string(b), "Description")
}

// appWorkspaceTemplates is the workspace template layer ($ADB_HOME/templates).
func appWorkspaceTemplates(app *internal.App) string {
	return fmt.Sprintf("%s/templates", app.BasePath)
}

// ticketDir resolves a task's ticket directory: the persisted TicketPath when
// set, else the resolver (nested/local/legacy layouts).
func ticketDir(app *internal.App, task *models.Task) string {
	if task.TicketPath != "" {
		return task.TicketPath
	}
	if d, err := core.ResolveTicketDir(fmt.Sprintf("%s/tickets", app.BasePath), task.ID); err == nil {
		return d
	}
	return ""
}

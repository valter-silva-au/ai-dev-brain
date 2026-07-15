package mcpserver

import (
	"context"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// registerGraphTools wires the graph + knowledge tools (decision D9) onto the
// server. Like the task tools, each is a thin adapter over the same App
// subsystems the CLI uses (GraphManager, StageManager, the memory store), so
// there is no divergent logic between the CLI and MCP entry points.
func registerGraphTools(s *server.MCPServer, app *internal.App) {
	s.AddTool(mcp.NewTool("graph_neighbors",
		mcp.WithDescription("List the graph edges incident to an entity (both directions: edges it declares and edges declared toward it), optionally filtered by edge type. Entity ids look like TASK-00001 or an initiative id."),
		mcp.WithString("id", mcp.Required(),
			mcp.Description("The entity id, e.g. TASK-00001 or an initiative id."),
		),
		mcp.WithString("type",
			mcp.Description("Optional edge-type filter: relates_to, part_of, blocks, depends_on, duplicates."),
		),
	), handleGraphNeighbors(app))

	s.AddTool(mcp.NewTool("related_tickets",
		mcp.WithDescription("List the tickets directly linked to a ticket in the graph, each with the relationship type and direction (outgoing = this ticket declares it; incoming = the other declares it)."),
		mcp.WithString("id", mcp.Required(),
			mcp.Description("The ticket id, e.g. TASK-00001."),
		),
	), handleRelatedTickets(app))

	s.AddTool(mcp.NewTool("get_initiative",
		mcp.WithDescription("Get a founder-playbook initiative: its org, stage (Idea/MVP/Launch/Scale), and most recent stage-gate state."),
		mcp.WithString("id", mcp.Required(),
			mcp.Description("The initiative id (slug)."),
		),
	), handleGetInitiative(app))

	s.AddTool(mcp.NewTool("search_knowledge",
		mcp.WithDescription("Semantic search over the workspace's vector memory. Degrades gracefully — returns a clear notice, never an error — when memory is not configured for the workspace."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("The natural-language search query."),
		),
		mcp.WithString("namespace",
			mcp.Description("Optional namespace to scope the search (e.g. tickets/TASK-00001). Omit to search across all namespaces."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max hits to return (default 5)."),
		),
	), handleSearchKnowledge(app))
}

// edgeViews normalises a possibly-nil edge slice to a non-nil one so it
// marshals as [] rather than null.
func edgeViews(edges []models.GraphEdge) []models.GraphEdge {
	if edges == nil {
		return []models.GraphEdge{}
	}
	return edges
}

func handleGraphNeighbors(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		var edges []models.GraphEdge
		if ft := req.GetString("type", ""); ft != "" {
			edges, err = app.GraphManager.NeighborsByType(id, models.EdgeType(ft))
		} else {
			edges, err = app.GraphManager.Neighbors(id)
		}
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to read neighbours", err), nil
		}
		return jsonResult(map[string]any{"id": id, "count": len(edges), "edges": edgeViews(edges)})
	}
}

func handleRelatedTickets(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		edges, err := app.GraphManager.Neighbors(id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to read neighbours", err), nil
		}
		type relatedTicket struct {
			TicketID  string `json:"ticket_id"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			Type      string `json:"type"`
			EdgeType  string `json:"edge_type"`
			Direction string `json:"direction"` // outgoing | incoming
		}
		related := make([]relatedTicket, 0, len(edges))
		for _, e := range edges {
			other, dir := e.To, "outgoing"
			if e.To == id {
				other, dir = e.From, "incoming"
			}
			// Only surface neighbours that are known tickets in the backlog —
			// the "other end" may be an initiative or an as-yet-unknown ref.
			task, gerr := app.BacklogManager.GetTask(other)
			if gerr != nil {
				continue
			}
			related = append(related, relatedTicket{
				TicketID:  task.ID,
				Title:     task.Title,
				Status:    string(task.Status),
				Type:      string(task.Type),
				EdgeType:  string(e.Type),
				Direction: dir,
			})
		}
		return jsonResult(map[string]any{"id": id, "count": len(related), "related": related})
	}
}

func handleGetInitiative(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		init, err := app.StageManager.GetInitiative(id)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to get initiative", err), nil
		}
		// models.Initiative already carries json tags for id/name/org_id/stage/gate.
		return jsonResult(init)
	}
}

// knowledgeHit is the JSON shape returned for one search_knowledge result.
type knowledgeHit struct {
	Namespace string            `json:"namespace"`
	Key       string            `json:"key"`
	Score     float32           `json:"score"`
	Content   string            `json:"content"`
	Meta      map[string]string `json:"meta,omitempty"`
}

func handleSearchKnowledge(app *internal.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		limit := req.GetInt("limit", 5)
		if limit <= 0 {
			limit = 5
		}
		ns := req.GetString("namespace", "")

		store, configured, err := app.OpenMemoryStore(ctx)
		if err != nil {
			// Never a hard error — surface a clear notice so the agent can move on.
			return jsonResult(map[string]any{
				"configured": false,
				"notice":     fmt.Sprintf("vector memory unavailable: %v", err),
				"count":      0,
				"hits":       []knowledgeHit{},
			})
		}
		if !configured {
			return jsonResult(map[string]any{
				"configured": false,
				"notice":     "vector memory is not configured for this workspace (no knowledge base found — enable hooks.memory or add records with `adb memory store`)",
				"count":      0,
				"hits":       []knowledgeHit{},
			})
		}
		defer store.Close()

		hits, err := searchKnowledge(ctx, store, ns, query, limit)
		if err != nil {
			return jsonResult(map[string]any{
				"configured": true,
				"notice":     fmt.Sprintf("search failed: %v", err),
				"count":      0,
				"hits":       []knowledgeHit{},
			})
		}
		views := make([]knowledgeHit, 0, len(hits))
		for _, h := range hits {
			views = append(views, knowledgeHit{
				Namespace: h.Namespace, Key: h.Key, Score: h.Score, Content: h.Content, Meta: h.Meta,
			})
		}
		return jsonResult(map[string]any{"configured": true, "count": len(views), "hits": views})
	}
}

// searchKnowledge searches one namespace when ns is set, otherwise every
// namespace, merging the hits and returning the top `limit` by score.
func searchKnowledge(ctx context.Context, store memory.Store, ns, query string, limit int) ([]memory.Hit, error) {
	if ns != "" {
		return store.Search(ctx, ns, query, limit)
	}
	names, err := store.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	var all []memory.Hit
	for _, n := range names {
		hits, err := store.Search(ctx, n, query, limit)
		if err != nil {
			return nil, err
		}
		all = append(all, hits...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/memory"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ===== Isolation in this package =============================================
//
// Every App built here comes from internal.NewAppIsolated, never internal.NewApp.
// A t.TempDir() workspace is NOT enough on its own: it pins only the REPO tier
// (<workspace>/.taskrc), while the GLOBAL tier still defaults to
// $HOME/.taskconfig, the terminal-state file to $HOME/.adb_terminal_state.json,
// and the active org to $ADB_ORG.
//
// That matters for the memory tools specifically. App.OpenMemoryStore resolves
// its embedder from MergedConfig.ResolvedHooks().Memory, and this repo's own
// workstation config sets provider: ollama / nomic-embed-text — so
// TestSearchKnowledge_Configured_ReturnsHits, which seeds the store with
// memory.NewFakeEmbedder(64), failed with
// `embedder mismatch: store was created with "fake", opened with
// "ollama/nomic-embed-text"`. TestSearchKnowledge_NoMemory_Graceful is exposed
// the same way: a global hooks.memory.db_path pointing at a real database would
// make its "no knowledge base here" premise false.
//
// NewAppIsolated points the global tier at <workspace>/.taskconfig — absent in
// every test here, so GetGlobalConfig returns models.DefaultGlobalConfig() and the
// Memory block stays zero-valued, which is what makes the fake/64 default true.
// It also ignores $ADB_ORG. This replaced a t.Setenv("HOME", …) helper: the
// constructor cannot be forgotten by the next author, and unlike t.Setenv it does
// not forbid t.Parallel — which is why every test in this package is now parallel.
//
// Note the leak was never via ADB_HOME: that is read only by cmd/adb/main.go when
// it resolves a base path to hand to NewApp, so it has no effect on an in-process
// test that passes the path directly.

// seededGraphApp builds an isolated App on a TempDir workspace with a small graph:
// TASK-00001 --depends_on--> TASK-00002, --relates_to--> widget-launch (an
// initiative), --mentions--> TASK-99999 (an unknown ref); plus org acme and
// initiative widget-launch.
func seededGraphApp(t *testing.T) *internal.App {
	t.Helper()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })

	if _, err := app.StageManager.CreateOrganization("Acme", "github.com"); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := app.StageManager.CreateInitiative("Widget Launch", "acme"); err != nil {
		t.Fatalf("CreateInitiative: %v", err)
	}

	tasks := []models.Task{
		{
			ID: "TASK-00001", Title: "alpha", Type: models.TaskTypeFeat,
			Status: models.TaskStatusBacklog, Priority: models.PriorityP2,
			Links: []models.Link{
				{Type: models.EdgeDependsOn, Target: "TASK-00002"},
				{Type: models.EdgeRelatesTo, Target: "widget-launch"},
				{Type: models.EdgeType("mentions"), Target: "TASK-99999"},
			},
		},
		{ID: "TASK-00002", Title: "beta", Type: models.TaskTypeFeat, Status: models.TaskStatusBacklog, Priority: models.PriorityP2},
	}
	for _, tk := range tasks {
		if err := app.BacklogManager.AddTask(tk); err != nil {
			t.Fatalf("AddTask %s: %v", tk.ID, err)
		}
	}
	return app
}

func callTool(t *testing.T, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) map[string]any {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned an error result: %s", resultText(t, res))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("result is not JSON object: %v\n%s", err, resultText(t, res))
	}
	return out
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("content[0] is not text: %T", res.Content[0])
	}
	return tc.Text
}

func TestGraphNeighbors_Tool(t *testing.T) {
	t.Parallel()
	app := seededGraphApp(t)

	out := callTool(t, handleGraphNeighbors(app), map[string]any{"id": "TASK-00001"})
	if int(out["count"].(float64)) != 3 {
		t.Errorf("neighbours count=%v want 3\n%v", out["count"], out)
	}

	// Type filter narrows to the single depends_on edge.
	filtered := callTool(t, handleGraphNeighbors(app), map[string]any{"id": "TASK-00001", "type": "depends_on"})
	if int(filtered["count"].(float64)) != 1 {
		t.Errorf("depends_on count=%v want 1\n%v", filtered["count"], filtered)
	}

	// TASK-00002 sees the incoming depends_on from TASK-00001.
	incoming := callTool(t, handleGraphNeighbors(app), map[string]any{"id": "TASK-00002"})
	if int(incoming["count"].(float64)) != 1 {
		t.Errorf("TASK-00002 incoming count=%v want 1\n%v", incoming["count"], incoming)
	}
}

func TestRelatedTickets_Tool(t *testing.T) {
	t.Parallel()
	app := seededGraphApp(t)

	out := callTool(t, handleRelatedTickets(app), map[string]any{"id": "TASK-00001"})
	// Only TASK-00002 is a known backlog ticket; the initiative and the unknown
	// TASK-99999 ref are excluded.
	if int(out["count"].(float64)) != 1 {
		t.Fatalf("related count=%v want 1\n%v", out["count"], out)
	}
	related := out["related"].([]any)
	first := related[0].(map[string]any)
	if first["ticket_id"] != "TASK-00002" || first["edge_type"] != "depends_on" || first["direction"] != "outgoing" {
		t.Errorf("related[0]=%v want TASK-00002/depends_on/outgoing", first)
	}
	if first["title"] != "beta" {
		t.Errorf("related[0].title=%v want beta", first["title"])
	}
}

func TestGetInitiative_Tool(t *testing.T) {
	t.Parallel()
	app := seededGraphApp(t)

	out := callTool(t, handleGetInitiative(app), map[string]any{"id": "widget-launch"})
	if out["id"] != "widget-launch" || out["org_id"] != "acme" {
		t.Errorf("initiative id/org mismatch: %v", out)
	}
	if out["stage"] != "Idea" {
		t.Errorf("stage=%v want Idea", out["stage"])
	}

	// Unknown initiative is a clean error result, not a panic.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "nope"}
	res, err := handleGetInitiative(app)(context.Background(), req)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError {
		t.Errorf("unknown initiative should yield an error result")
	}
}

func TestSearchKnowledge_NoMemory_Graceful(t *testing.T) {
	t.Parallel()
	app := seededGraphApp(t) // fresh workspace: no .adb_memory.sqlite

	out := callTool(t, handleSearchKnowledge(app), map[string]any{"query": "anything"})
	if out["configured"] != false {
		t.Errorf("configured=%v want false in a workspace with no memory db", out["configured"])
	}
	if _, ok := out["notice"]; !ok {
		t.Errorf("expected a graceful notice, got %v", out)
	}
	if int(out["count"].(float64)) != 0 {
		t.Errorf("count=%v want 0", out["count"])
	}
	hits, ok := out["hits"].([]any)
	if !ok || len(hits) != 0 {
		t.Errorf("hits=%v want empty array", out["hits"])
	}
}

func TestSearchKnowledge_Configured_ReturnsHits(t *testing.T) {
	t.Parallel()
	// Load-bearing: OpenMemoryStore's embedder comes from the merged config, and
	// for an un-isolated App the global tier is the developer's ~/.taskconfig.
	// Under plain NewApp the seeded fake/64 store is opened with whatever embedder
	// that file names, and OpenSQLiteStore rejects the mismatch. See the isolation
	// note at the top of this file.
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })
	// Seed a memory store at the default path with the default fake embedder
	// (dim 64) so App.OpenMemoryStore — which also defaults to fake/64 once the
	// config tiers are isolated — can open and search it.
	ctx := context.Background()
	// Seed at the default path OpenMemoryStore resolves — under .adb/ (#186/#189).
	dbPath := app.StatePath("memory.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir .adb: %v", err)
	}
	store, err := memory.OpenSQLiteStore(ctx, dbPath, memory.NewFakeEmbedder(64))
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := store.Upsert(ctx, "tickets/TASK-00001", "note1", "the ranking algorithm favours recency", map[string]string{"kind": "note"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_ = store.Close()

	out := callTool(t, handleSearchKnowledge(app), map[string]any{"query": "ranking recency"})
	if out["configured"] != true {
		t.Fatalf("configured=%v want true\n%v", out["configured"], out)
	}
	if int(out["count"].(float64)) != 1 {
		t.Fatalf("count=%v want 1\n%v", out["count"], out)
	}
	hit := out["hits"].([]any)[0].(map[string]any)
	if hit["namespace"] != "tickets/TASK-00001" || hit["key"] != "note1" {
		t.Errorf("hit=%v want tickets/TASK-00001/note1", hit)
	}
}

func TestGraphAndTaskTools_Registered(t *testing.T) {
	t.Parallel()
	app, err := internal.NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })
	// New() must wire both tool sets without panicking.
	if s := New(app, "test"); s == nil {
		t.Fatal("New returned nil server")
	}
}

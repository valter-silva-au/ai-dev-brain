package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// KnowledgeIndexer connects the knowledge/graph pipeline to the vector-memory
// substrate (issue #121): it indexes ticket knowledge (context/notes/design/
// decisions) and the graph's typed edges into the store, so the MCP
// search_knowledge tool (#113) surfaces real workspace content rather than an
// empty index. It complements the memory HOOK (which indexes one ticket on
// completion) with an on-demand FULL-workspace pass driven by `adb memory index`.
//
// It depends only on the MemoryIndexer Upsert seam plus the backlog + graph it
// already reads elsewhere, so it stays testable with fakes and never opens the
// store itself (the caller passes an already-opened, opt-in store).
type KnowledgeIndexer struct {
	mem        MemoryIndexer
	backlog    BacklogStore
	graph      GraphManager
	ticketsDir string
}

// NewKnowledgeIndexer wires the indexer. mem is the (already-opened) vector
// store's Upsert surface; ticketsDir is the workspace tickets/ root used to
// resolve a ticket's directory when a task has no recorded TicketPath.
func NewKnowledgeIndexer(mem MemoryIndexer, backlog BacklogStore, graph GraphManager, ticketsDir string) *KnowledgeIndexer {
	return &KnowledgeIndexer{mem: mem, backlog: backlog, graph: graph, ticketsDir: ticketsDir}
}

// KnowledgeIndexStats reports what an index pass wrote.
type KnowledgeIndexStats struct {
	Tickets int // tickets that contributed at least one indexed file
	Files   int // ticket knowledge files indexed
	Edges   int // graph edges indexed
}

// ticketKnowledgeFiles are the per-ticket artifacts worth indexing for semantic
// recall (paths relative to the ticket dir).
var ticketKnowledgeFiles = []string{"context.md", "notes.md", "design.md", filepath.Join("knowledge", "decisions.yaml")}

// IndexWorkspace indexes every ticket's knowledge files under namespace
// tickets/<id> and every graph edge under namespace "graph". It is idempotent:
// Upsert replaces a record at (ns, key), so re-running refreshes rather than
// duplicates. A missing/empty file is skipped, not an error.
func (ki *KnowledgeIndexer) IndexWorkspace(ctx context.Context) (KnowledgeIndexStats, error) {
	var stats KnowledgeIndexStats
	if ki.mem == nil {
		return stats, fmt.Errorf("no memory indexer wired")
	}

	backlog, err := ki.backlog.Load()
	if err != nil {
		return stats, fmt.Errorf("load backlog: %w", err)
	}
	for _, t := range backlog.Tasks {
		n, err := ki.indexTaskFiles(ctx, t)
		if err != nil {
			return stats, err
		}
		if n > 0 {
			stats.Tickets++
			stats.Files += n
		}
	}

	if ki.graph != nil {
		g, err := ki.graph.Graph()
		if err != nil {
			return stats, fmt.Errorf("build graph: %w", err)
		}
		for _, e := range g.Index().Edges {
			key := fmt.Sprintf("%s|%s|%s", e.From, e.Type, e.To)
			content := fmt.Sprintf("%s %s %s", e.From, e.Type, e.To)
			if err := ki.mem.Upsert(ctx, "graph", key, content, map[string]string{"source": "graph-edge"}); err != nil {
				return stats, fmt.Errorf("index edge %s: %w", key, err)
			}
			stats.Edges++
		}
	}
	return stats, nil
}

// indexTaskFiles indexes a single ticket's knowledge files, returning how many
// were indexed. It resolves the ticket dir from the task's TicketPath, falling
// back to a tickets/ walk (the nested correlation layout means a task id alone
// is not a directory).
func (ki *KnowledgeIndexer) indexTaskFiles(ctx context.Context, t models.Task) (int, error) {
	dir := t.TicketPath
	if dir == "" {
		resolved, err := ResolveTicketDir(ki.ticketsDir, t.ID)
		if err != nil {
			// Per-ticket indexing is best-effort: a task with no resolvable ticket
			// dir (none created yet, or an unreadable tree) contributes nothing but
			// must not fail the whole workspace pass. The missing content simply
			// isn't searchable until the dir exists.
			return 0, nil
		}
		dir = resolved
	}
	ns := "tickets/" + t.ID
	indexed := 0
	for _, rel := range ticketKnowledgeFiles {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || len(data) == 0 {
			continue
		}
		key := filepath.ToSlash(rel)
		meta := map[string]string{"source": "ticket-knowledge", "task_id": t.ID}
		if err := ki.mem.Upsert(ctx, ns, key, string(data), meta); err != nil {
			return indexed, fmt.Errorf("index %s/%s: %w", ns, key, err)
		}
		indexed++
	}
	return indexed, nil
}
